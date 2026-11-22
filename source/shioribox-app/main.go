package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

const version = "1.20.0"

type App struct {
	auto    autoState
	upd     updState
	store   *Store
	dataDir string
	port    int
	run     *Runner
}

func logf(format string, a ...any) { log.Printf(format, a...) }

func writable(dir string) bool {
	if os.MkdirAll(dir, 0o755) != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".w")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

func resolveDataDir(flagVal string) string {
	if flagVal != "" {
		p, _ := filepath.Abs(flagVal)
		return p
	}
	if e := os.Getenv("SHIORIBOX_DATA"); e != "" {
		p, _ := filepath.Abs(e)
		return p
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "data")
		if writable(d) {
			return d
		}
	}
	cfg, _ := os.UserConfigDir()
	if _, err := os.Stat(filepath.Join(cfg, "ShioriBox")); err != nil {
		if _, err := os.Stat(filepath.Join(cfg, "NovelShelf")); err == nil {
			return filepath.Join(cfg, "NovelShelf") // keep using data from before the rename
		}
	}
	return filepath.Join(cfg, "ShioriBox")
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Start()
}

// findAppBrowser locates a Chromium-based browser (Edge ships with Windows 10/11)
// that can show the page as a standalone app window.
func findAppBrowser() string {
	if p := os.Getenv("SHIORIBOX_BROWSER"); p != "" {
		return p
	}
	var cands []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			cands = append(cands,
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `BraveSoftware\Brave-Browser\Application\brave.exe`))
		}
	} else {
		for _, n := range []string{"microsoft-edge", "google-chrome", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(n); err == nil {
				cands = append(cands, p)
			}
		}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// openAppWindow shows url in a chrome-less window using a private browser profile.
// It returns the started process, or nil if no suitable browser exists.
func openAppWindow(u, dataDir string) *exec.Cmd {
	b := findAppBrowser()
	if b == "" {
		return nil
	}
	cmd := exec.Command(b, "--app="+u, "--user-data-dir="+filepath.Join(dataDir, "window"),
		"--window-size=1360,860", "--no-first-run", "--no-default-browser-check",
		"--disable-features=Translate,msEdgeSidebarV2")
	if err := cmd.Start(); err != nil {
		logf("could not open app window with %s: %v", b, err)
		return nil
	}
	return cmd
}

func main() {
	port := flag.Int("port", 17321, "port to listen on (localhost only)")
	dataFlag := flag.String("data", "", "data directory (default: ./data next to the program)")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser on start")
	flag.Parse()

	dataDir := resolveDataDir(*dataFlag)
	os.MkdirAll(dataDir, 0o755)
	if lf, err := os.OpenFile(filepath.Join(dataDir, "shioribox.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(lf)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", *port)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		// another copy is already running: open a window onto it and leave it in charge
		if !*noBrowser {
			runUI(nil, url, dataDir)
		}
		return
	}

	st, err := newStore(dataDir)
	if err != nil {
		log.Fatalf("cannot open data dir %s: %v", dataDir, err)
	}
	app := &App{store: st, dataDir: dataDir, port: *port, run: newRunner()}
	if v := os.Getenv("SHIORIBOX_VNDB_API"); v != "" {
		vndbBase = v // testing aid
	}
	app.startSnapshots()
	logf("ShioriBox %s serving %s (data: %s)", version, url, dataDir)
	srv := &http.Server{Handler: app.guard(app.routes())}
	go app.autoLoop()
	go app.updateLoop()
	go func() { log.Fatal(srv.Serve(ln)) }()
	if *noBrowser {
		select {}
	}
	runUI(app, url, dataDir) // blocks until the window is closed
	waitForGames(app)
	logf("window closed, exiting")
}

// waitForGames keeps the process alive while games are still being timed.
func waitForGames(app *App) {
	if app == nil {
		return
	}
	for len(app.run.Snapshot()) > 0 {
		time.Sleep(2 * time.Second)
	}
}

// runBrowserUI is the fallback when no embedded window is available: a chrome-less
// Edge/Chrome window if possible, otherwise the default browser (stays up until Quit).
func runBrowserUI(app *App, url, dataDir string) {
	t0 := time.Now()
	cmd := openAppWindow(url, dataDir)
	if cmd == nil {
		openBrowser(url)
		if app != nil {
			select {}
		}
		return
	}
	cmd.Wait()
	if time.Since(t0) < 3*time.Second {
		logf("app window exited too quickly; falling back to default browser")
		openBrowser(url)
		if app != nil {
			select {}
		}
	}
}

// guard blocks DNS-rebinding and cross-site requests: this server can launch programs.
func (a *App) guard(next http.Handler) http.Handler {
	ok := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", a.port): true,
		fmt.Sprintf("localhost:%d", a.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-ShioriBox") == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(b)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v)
}

func ok(w http.ResponseWriter) { writeJSON(w, 200, map[string]bool{"ok": true}) }

func imageExt(b []byte) (string, bool) {
	switch http.DetectContentType(b) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	}
	return "", false
}

func (a *App) setCover(id string, data []byte) error {
	ext, good := imageExt(data)
	if !good {
		return errors.New("that file is not a PNG, JPEG, GIF or WebP image")
	}
	name := fmt.Sprintf("%s-%d%s", id, time.Now().UnixNano(), ext)
	if err := os.WriteFile(filepath.Join(a.dataDir, "covers", name), data, 0o644); err != nil {
		return err
	}
	var old string
	err := a.store.Update(func(l *Library) error {
		g := findGame(l, id)
		if g == nil {
			return errors.New("game not found")
		}
		old, g.Cover = g.Cover, name
		return nil
	})
	if err != nil {
		os.Remove(filepath.Join(a.dataDir, "covers", name))
		return err
	}
	if old != "" {
		os.Remove(filepath.Join(a.dataDir, "covers", filepath.Base(old)))
	}
	return nil
}

func copyEditable(dst, src *Game) {
	dst.Title = strings.TrimSpace(src.Title)
	dst.OriginalTitle = src.OriginalTitle
	dst.Developer = src.Developer
	dst.ReleaseDate = src.ReleaseDate
	dst.Description = src.Description
	dst.ExePath = src.ExePath
	dst.LaunchArgs = src.LaunchArgs
	dst.UseWrapper = src.UseWrapper
	dst.Locale = src.Locale
	dst.NSFW = src.NSFW
	dst.Progress = strings.TrimSpace(src.Progress)
	dst.SavePath = src.SavePath
	dst.Status = src.Status
	dst.Rating = src.Rating
	dst.Tags = src.Tags
	dst.Categories = src.Categories
	dst.Notes = src.Notes
	dst.VNDBID = src.VNDBID
	dst.Favorite = src.Favorite
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	covers := http.StripPrefix("/covers/", http.FileServer(http.Dir(filepath.Join(a.dataDir, "covers"))))
	mux.HandleFunc("GET /covers/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store") // local files: keep them out of the browser cache
		covers.ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /api/library", func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		a.store.View(func(l *Library) {
			b, _ = json.Marshal(map[string]any{
				"games": l.Games, "categories": l.Categories, "settings": l.Settings,
				"running": a.run.Snapshot(),
				"info":    map[string]any{"version": version, "dataDir": a.dataDir, "os": runtime.GOOS, "le": leStatus(l.Settings), "engine": engineStatus(a.dataDir)},
			})
		})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	mux.HandleFunc("GET /api/running", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, a.run.Snapshot())
	})

	// ---- games
	mux.HandleFunc("POST /api/games", func(w http.ResponseWriter, r *http.Request) {
		var in Game
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		if strings.TrimSpace(in.Title) == "" {
			fail(w, 400, "title is required")
			return
		}
		g := &Game{ID: newID(), AddedAt: time.Now()}
		copyEditable(g, &in)
		normalize(g)
		if err := a.store.Update(func(l *Library) error { l.Games = append(l.Games, g); return nil }); err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, g)
	})
	mux.HandleFunc("PUT /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in Game
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		if strings.TrimSpace(in.Title) == "" {
			fail(w, 400, "title is required")
			return
		}
		var out Game
		err := a.store.Update(func(l *Library) error {
			g := findGame(l, r.PathValue("id"))
			if g == nil {
				return errors.New("game not found")
			}
			copyEditable(g, &in)
			normalize(g)
			out = *g
			return nil
		})
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("DELETE /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var cover string
		err := a.store.Update(func(l *Library) error {
			for i, g := range l.Games {
				if g.ID == id {
					cover = g.Cover
					l.Games = append(l.Games[:i], l.Games[i+1:]...)
					return nil
				}
			}
			return errors.New("game not found")
		})
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		if cover != "" {
			os.Remove(filepath.Join(a.dataDir, "covers", filepath.Base(cover)))
		}
		if r.URL.Query().Get("keepBackups") != "1" {
			os.RemoveAll(a.backupDir(id))
		}
		os.RemoveAll(a.guideDir(id))
		ok(w)
	})
	mux.HandleFunc("POST /api/games/{id}/launch", func(w http.ResponseWriter, r *http.Request) {
		if err := a.launch(r.PathValue("id")); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("POST /api/games/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := a.stop(r.PathValue("id")); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("POST /api/games/{id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		var in Session
		if err := decode(r, &in); err != nil || in.Seconds <= 0 || in.Seconds > 7*24*3600 {
			fail(w, 400, "seconds must be between 1 and 604800")
			return
		}
		if in.Start.IsZero() {
			in.Start = time.Now()
		}
		err := a.store.Update(func(l *Library) error {
			g := findGame(l, r.PathValue("id"))
			if g == nil {
				return errors.New("game not found")
			}
			g.Sessions = append(g.Sessions, in)
			return nil
		})
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("DELETE /api/games/{id}/sessions/{idx}", func(w http.ResponseWriter, r *http.Request) {
		var idx int
		if _, err := fmt.Sscanf(r.PathValue("idx"), "%d", &idx); err != nil {
			fail(w, 400, "bad index")
			return
		}
		err := a.store.Update(func(l *Library) error {
			g := findGame(l, r.PathValue("id"))
			if g == nil || idx < 0 || idx >= len(g.Sessions) {
				return errors.New("session not found")
			}
			g.Sessions = append(g.Sessions[:idx], g.Sessions[idx+1:]...)
			return nil
		})
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		ok(w)
	})

	// ---- background picture
	bgDir := filepath.Join(a.dataDir, "background")
	bgFiles := http.StripPrefix("/background/", http.FileServer(http.Dir(bgDir)))
	mux.HandleFunc("GET /background/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		bgFiles.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /api/background", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 30<<20))
		if err != nil {
			fail(w, 400, "could not read upload")
			return
		}
		ext, good := imageExt(data)
		if !good {
			fail(w, 400, "that file is not a PNG, JPEG, GIF or WebP image")
			return
		}
		os.MkdirAll(bgDir, 0o755)
		name := fmt.Sprintf("bg-%d%s", time.Now().UnixNano(), ext)
		if err := os.WriteFile(filepath.Join(bgDir, name), data, 0o644); err != nil {
			fail(w, 500, err.Error())
			return
		}
		var old string
		a.store.Update(func(l *Library) error { old, l.Settings.BgImage = l.Settings.BgImage, name; return nil })
		if old != "" {
			os.Remove(filepath.Join(bgDir, filepath.Base(old)))
		}
		writeJSON(w, 200, map[string]string{"name": name})
	})
	mux.HandleFunc("DELETE /api/background", func(w http.ResponseWriter, r *http.Request) {
		var old string
		a.store.Update(func(l *Library) error { old, l.Settings.BgImage = l.Settings.BgImage, ""; return nil })
		if old != "" {
			os.Remove(filepath.Join(bgDir, filepath.Base(old)))
		}
		ok(w)
	})
	mux.HandleFunc("PUT /api/background/settings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Blur, Overlay, Panel int
			Pos                  string
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		a.store.Update(func(l *Library) error {
			l.Settings.BgBlur, l.Settings.BgOverlay, l.Settings.BgPanel, l.Settings.BgPos = in.Blur, in.Overlay, in.Panel, in.Pos
			return nil
		})
		ok(w)
	})

	// ---- guides (walkthrough images / text files)
	guideFiles := http.StripPrefix("/guides/", http.FileServer(http.Dir(filepath.Join(a.dataDir, "guides"))))
	mux.HandleFunc("GET /guides/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		guideFiles.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /api/games/{id}/guides", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxGuideImage+1))
		if err != nil || len(data) == 0 {
			fail(w, 400, "could not read upload")
			return
		}
		g, err := a.addGuide(r.PathValue("id"), r.URL.Query().Get("name"), data)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, g)
	})
	mux.HandleFunc("GET /api/games/{id}/guides/{gid}/text", func(w http.ResponseWriter, r *http.Request) {
		t, err := a.guideText(r.PathValue("id"), r.PathValue("gid"))
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"text": t})
	})
	mux.HandleFunc("GET /api/games/{id}/guides/{gid}/marks", func(w http.ResponseWriter, r *http.Request) {
		b, err := a.readMarks(r.PathValue("id"), r.PathValue("gid"))
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	mux.HandleFunc("PUT /api/games/{id}/guides/{gid}/marks", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxMarksBytes+1))
		if err != nil {
			fail(w, 400, "could not read marks")
			return
		}
		if err := a.writeMarks(r.PathValue("id"), r.PathValue("gid"), data); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("DELETE /api/games/{id}/guides/{gid}", func(w http.ResponseWriter, r *http.Request) {
		a.removeGuide(r.PathValue("id"), r.PathValue("gid"))
		ok(w)
	})

	// ---- covers
	mux.HandleFunc("POST /api/games/{id}/cover", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 12<<20))
		if err != nil {
			fail(w, 400, "could not read upload")
			return
		}
		if err := a.setCover(r.PathValue("id"), data); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("POST /api/games/{id}/cover-from-url", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		data, _, err := fetchImage(in.URL)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if err := a.setCover(r.PathValue("id"), data); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("DELETE /api/games/{id}/cover", func(w http.ResponseWriter, r *http.Request) {
		var old string
		a.store.Update(func(l *Library) error {
			if g := findGame(l, r.PathValue("id")); g != nil {
				old, g.Cover = g.Cover, ""
			}
			return nil
		})
		if old != "" {
			os.Remove(filepath.Join(a.dataDir, "covers", filepath.Base(old)))
		}
		ok(w)
	})

	// ---- metadata
	mux.HandleFunc("POST /api/vndb/discover", func(w http.ResponseWriter, r *http.Request) {
		var q DiscoverQuery
		if err := decode(r, &q); err != nil {
			fail(w, 400, "bad request")
			return
		}
		res, more, err := vndbDiscover(q)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"results": res, "more": more})
	})
	mux.HandleFunc("GET /api/vndb/recommend", func(w http.ResponseWriter, r *http.Request) {
		var games []*Game
		a.store.View(func(l *Library) { games = append(games, l.Games...) })
		rec, err := recommend(games)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, rec)
	})
	mux.HandleFunc("GET /api/vndb/vn/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, err := vndbDetail(r.PathValue("id"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("GET /api/vndb/char/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, err := vndbCharDetail(r.PathValue("id"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("GET /api/vndb/producers", func(w http.ResponseWriter, r *http.Request) {
		res, err := vndbProducerSearch(r.URL.Query().Get("q"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("GET /api/vndb/tags", func(w http.ResponseWriter, r *http.Request) {
		res, err := vndbTags(r.URL.Query().Get("q"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("GET /api/vndb/search", func(w http.ResponseWriter, r *http.Request) {
		res, err := vndbSearch(r.URL.Query().Get("q"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, res)
	})

	// ---- files, scan, import
	mux.HandleFunc("GET /api/fs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var exts map[string]bool
		if e := q.Get("exts"); e != "" {
			exts = map[string]bool{}
			for _, x := range strings.Split(strings.ToLower(e), ",") {
				exts[x] = true
			}
		}
		res, err := listFS(q.Get("path"), exts, q.Get("dirsOnly") == "1")
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
		}
		if err := decode(r, &in); err != nil || in.Path == "" {
			fail(w, 400, "path is required")
			return
		}
		known := map[string]bool{}
		a.store.View(func(l *Library) {
			for _, g := range l.Games {
				if g.ExePath != "" {
					known[normPath(g.ExePath)] = true
				}
			}
		})
		res, err := scanLibrary(in.Path, known)
		if err != nil {
			fail(w, 400, "cannot read that folder: "+err.Error())
			return
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []struct {
				Title   string `json:"title"`
				ExePath string `json:"exePath"`
				Locale  string `json:"locale"`
			} `json:"items"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		created := []*Game{}
		err := a.store.Update(func(l *Library) error {
			for _, it := range in.Items {
				if strings.TrimSpace(it.Title) == "" {
					continue
				}
				g := &Game{ID: newID(), Title: strings.TrimSpace(it.Title), ExePath: it.ExePath, Locale: it.Locale, AddedAt: time.Now()}
				normalize(g)
				l.Games = append(l.Games, g)
				created = append(created, g)
			}
			return nil
		})
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, created)
	})

	// ---- categories
	mux.HandleFunc("POST /api/categories", func(w http.ResponseWriter, r *http.Request) {
		var in Category
		if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
			fail(w, 400, "name is required")
			return
		}
		c := &Category{ID: newID(), Name: strings.TrimSpace(in.Name)}
		a.store.Update(func(l *Library) error { l.Categories = append(l.Categories, c); return nil })
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("PUT /api/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in Category
		if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
			fail(w, 400, "name is required")
			return
		}
		a.store.Update(func(l *Library) error {
			for _, c := range l.Categories {
				if c.ID == r.PathValue("id") {
					c.Name = strings.TrimSpace(in.Name)
				}
			}
			return nil
		})
		ok(w)
	})
	mux.HandleFunc("DELETE /api/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a.store.Update(func(l *Library) error {
			cs := l.Categories[:0]
			for _, c := range l.Categories {
				if c.ID != id {
					cs = append(cs, c)
				}
			}
			l.Categories = cs
			for _, g := range l.Games {
				keep := g.Categories[:0]
				for _, c := range g.Categories {
					if c != id {
						keep = append(keep, c)
					}
				}
				g.Categories = keep
			}
			return nil
		})
		ok(w)
	})

	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		if in.BackupKeep < 0 {
			in.BackupKeep = 0
		}
		a.store.Update(func(l *Library) error {
			// the background is managed by its own endpoints
			in.BgImage, in.BgBlur, in.BgOverlay, in.BgPanel, in.BgPos = l.Settings.BgImage, l.Settings.BgBlur, l.Settings.BgOverlay, l.Settings.BgPanel, l.Settings.BgPos
			in.PanicKey, in.PanicTapKey, in.PanicTaps = l.Settings.PanicKey, l.Settings.PanicTapKey, l.Settings.PanicTaps // own endpoints
			in.WatchFolders, in.AutoSeen, in.AutoLookupOff = l.Settings.WatchFolders, l.Settings.AutoSeen, l.Settings.AutoLookupOff
			in.UpdateSource, in.UpdateOff, in.UpdateSkip = l.Settings.UpdateSource, l.Settings.UpdateOff, l.Settings.UpdateSkip
			l.Settings = in
			return nil
		})
		ok(w)
	})

	// ---- save backups
	mux.HandleFunc("GET /api/games/{id}/backups", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, a.listBackups(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/games/{id}/backups", func(w http.ResponseWriter, r *http.Request) {
		name, err := a.backupGame(r.PathValue("id"))
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"name": name})
	})
	mux.HandleFunc("POST /api/games/{id}/backups/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		id, name := r.PathValue("id"), r.PathValue("name")
		if !safeName(name) {
			fail(w, 400, "bad backup name")
			return
		}
		if _, running := a.run.Snapshot()[id]; running {
			fail(w, 400, "close the game before restoring a save")
			return
		}
		var savePath string
		a.store.View(func(l *Library) {
			if g := findGame(l, id); g != nil {
				savePath = g.SavePath
			}
		})
		if savePath == "" {
			fail(w, 400, "no save location set for this game")
			return
		}
		if _, err := os.Stat(savePath); err == nil {
			pre := filepath.Join(a.backupDir(id), "pre-restore-"+time.Now().Format("20060102-150405")+".zip")
			if err := zipPath(savePath, pre); err != nil {
				fail(w, 500, "could not snapshot current save first: "+err.Error())
				return
			}
		}
		if err := restoreZip(filepath.Join(a.backupDir(id), name), savePath); err != nil {
			fail(w, 500, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("DELETE /api/games/{id}/backups/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !safeName(r.PathValue("name")) {
			fail(w, 400, "bad backup name")
			return
		}
		os.Remove(filepath.Join(a.backupDir(r.PathValue("id")), r.PathValue("name")))
		ok(w)
	})

	// ---- whole-library export / import
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		var lib []byte
		a.store.View(func(l *Library) { lib, _ = json.MarshalIndent(l, "", "  ") })
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="shioribox-backup-`+time.Now().Format("2006-01-02")+`.zip"`)
		zw := zip.NewWriter(w)
		f, _ := zw.Create("library.json")
		f.Write(lib)
		ents, _ := os.ReadDir(filepath.Join(a.dataDir, "covers"))
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(a.dataDir, "covers", e.Name()))
			if err != nil {
				continue
			}
			cf, _ := zw.Create("covers/" + e.Name())
			cf.Write(b)
		}
		filepath.WalkDir(filepath.Join(a.dataDir, "guides"), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(filepath.Join(a.dataDir, "guides"), p)
			if b, e := os.ReadFile(p); rerr == nil && e == nil {
				if cf, e := zw.Create("guides/" + filepath.ToSlash(rel)); e == nil {
					cf.Write(b)
				}
			}
			return nil
		})
		zw.Close()
	})
	mux.HandleFunc("POST /api/import-library", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 512<<20))
		if err != nil {
			fail(w, 400, "could not read upload")
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			fail(w, 400, "that is not a ShioriBox backup zip")
			return
		}
		var lib Library
		have := false
		for _, f := range zr.File {
			if f.Name == "library.json" {
				rc, _ := f.Open()
				b, _ := io.ReadAll(io.LimitReader(rc, 64<<20))
				rc.Close()
				if json.Unmarshal(b, &lib) != nil {
					fail(w, 400, "library.json in the backup is damaged")
					return
				}
				have = true
			}
		}
		if !have {
			fail(w, 400, "backup does not contain library.json")
			return
		}
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "covers/") && !f.FileInfo().IsDir() {
				name := filepath.Base(f.Name)
				rc, _ := f.Open()
				b, _ := io.ReadAll(io.LimitReader(rc, 12<<20))
				rc.Close()
				os.WriteFile(filepath.Join(a.dataDir, "covers", name), b, 0o644)
			}
			if parts := strings.Split(f.Name, "/"); len(parts) == 3 && parts[0] == "guides" && !f.FileInfo().IsDir() && parts[1] != "" && parts[2] != "" && parts[1] != ".." && parts[2] != ".." {
				rc, _ := f.Open()
				b, _ := io.ReadAll(io.LimitReader(rc, maxGuideImage+1))
				rc.Close()
				dir := a.guideDir(parts[1])
				os.MkdirAll(dir, 0o755)
				os.WriteFile(filepath.Join(dir, filepath.Base(parts[2])), b, 0o644)
			}
		}
		if b, err := os.ReadFile(a.store.path()); err == nil {
			os.WriteFile(a.store.path()+".before-import-"+time.Now().Format("20060102-150405"), b, 0o644)
		}
		a.store.Update(func(l *Library) error { *l = lib; return nil })
		ok(w)
	})

	mux.HandleFunc("GET /api/snapshots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, a.listSnapshots())
	})
	mux.HandleFunc("POST /api/snapshots", func(w http.ResponseWriter, r *http.Request) {
		note := "manual"
		if r.URL.Query().Get("note") == "bulk" {
			note = "bulk"
		}
		name, err := a.snapshot(note, false)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"name": name})
	})
	mux.HandleFunc("POST /api/snapshots/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		if err := a.restoreSnapshot(r.PathValue("name")); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("DELETE /api/snapshots/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !snapName.MatchString(r.PathValue("name")) {
			fail(w, 400, "bad snapshot name")
			return
		}
		os.Remove(filepath.Join(a.snapDir(), r.PathValue("name")))
		ok(w)
	})
	mux.HandleFunc("POST /api/open-snapshots", func(w http.ResponseWriter, r *http.Request) {
		os.MkdirAll(a.snapDir(), 0o755)
		if err := openFolder(a.snapDir()); err != nil {
			fail(w, 500, err.Error())
			return
		}
		ok(w)
	})

	a.cgRoutes(mux)
	a.animeRoutes(mux)
	a.streamRoutes(mux)
	a.panicRoutes(mux)
	a.autoRoutes(mux)
	a.updateRoutes(mux)

	mux.HandleFunc("POST /api/open-url", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		u, err := neturl.Parse(in.URL)
		okHost := err == nil && u.Scheme == "https" && (u.Hostname() == "vndb.org" || strings.HasSuffix(u.Hostname(), ".vndb.org"))
		okLE := err == nil && u.Scheme == "https" && u.Hostname() == "github.com" && strings.HasPrefix(u.Path, "/xupefei/Locale-Emulator")
		okCG := err == nil && u.Scheme == "https" && (cgOpenAllowed(u.Hostname()) ||
			(u.Hostname() == "www.google.com" && u.Path == "/search") || u.Hostname() == "www.pixiv.net")
		if !okHost && !okLE && !okCG {
			fail(w, 400, "that link can't be opened from here")
			return
		}
		openBrowser(in.URL)
		ok(w)
	})
	mux.HandleFunc("POST /api/open-region", func(w http.ResponseWriter, r *http.Request) {
		if err := openControlPanel("intl.cpl"); err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w)
	})
	mux.HandleFunc("POST /api/quit", func(w http.ResponseWriter, r *http.Request) {
		ok(w)
		go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
	})
	return mux
}
