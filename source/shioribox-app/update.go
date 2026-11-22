package main

// Update check. ShioriBox compares its own version with the newest one published at an address you set in Settings:
//   - "owner/repo": the latest GitHub release of that repository (its tag is the version, an attached .exe is the installer), or
//   - an https address of a small JSON file: {"version":"1.20.0","notes":"...","page":"https://...","download":"https://.../ShioriBox-Update-1.20.0.exe"}
// Nothing is checked until an address is set.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// updateSource is where new versions are published: the GitHub repository that holds ShioriBox's releases.
// (A variable only so tests can point it elsewhere.)
var updateSource = "frost662/Shioribox"

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Skipped   bool   `json:"skipped"`
	Notes     string `json:"notes"`
	Page      string `json:"page"`
	Download  string `json:"download"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Checked   string `json:"checked"`
	Error     string `json:"error"`
}

type updState struct {
	mu   sync.Mutex
	info UpdateInfo
}

var reGitHubRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// versionNewer reports whether a is a higher version than b ("1.20.0" > "1.19.3").
func versionNewer(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(v)), "v")
	var out []int
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' || r == '_' }) {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

func updateGetJSON(u string, v any) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ShioriBox/"+version)
	req.Header.Set("Accept", "application/vnd.github+json, application/json")
	resp, err := cgHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach the update address: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return errors.New("nothing is published at that address yet (404)")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("the update address answered %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(v)
}

// pickInstaller chooses the update installer among a release's files: only ShioriBox-Update-<version>.exe, never the
// first-time Setup installer or the portable zip. -1 when the release has none.
func pickInstaller(names []string) int {
	for i, n := range names {
		l := strings.ToLower(n)
		if strings.HasSuffix(l, ".exe") && strings.Contains(l, "update") && !strings.Contains(l, "setup") {
			return i
		}
	}
	return -1
}

// fetchLatest reads the newest published version from the source.
func fetchLatest(source string) (latest, notes, page, download, name string, size int64, err error) {
	source = strings.TrimSpace(source)
	switch {
	case reGitHubRepo.MatchString(source):
		var rel struct {
			Tag    string `json:"tag_name"`
			Name   string `json:"name"`
			Body   string `json:"body"`
			URL    string `json:"html_url"`
			Assets []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
				Size int64  `json:"size"`
			} `json:"assets"`
		}
		if err = updateGetJSON("https://api.github.com/repos/"+source+"/releases/latest", &rel); err != nil {
			return
		}
		latest, notes, page = strings.TrimPrefix(rel.Tag, "v"), rel.Body, rel.URL
		var names []string
		for _, a := range rel.Assets {
			names = append(names, a.Name)
		}
		if i := pickInstaller(names); i >= 0 {
			download, name, size = rel.Assets[i].URL, rel.Assets[i].Name, rel.Assets[i].Size
		}
	case strings.HasPrefix(strings.ToLower(source), "https://"):
		var m struct {
			Version  string `json:"version"`
			Notes    string `json:"notes"`
			Page     string `json:"page"`
			Download string `json:"download"`
		}
		if err = updateGetJSON(source, &m); err != nil {
			return
		}
		latest, notes, page, download = strings.TrimPrefix(m.Version, "v"), m.Notes, m.Page, m.Download
		if download != "" {
			if u, e := neturl.Parse(download); e == nil {
				name = filepath.Base(u.Path)
			}
		}
	default:
		err = errors.New("use a GitHub repository like owner/repo, or an https address")
		return
	}
	if len(versionParts(latest)) == 0 {
		err = errors.New("the published version number couldn't be read")
	}
	if len(notes) > 4000 {
		notes = notes[:4000] + "…"
	}
	return
}

func (a *App) updateSource() (src string, skip string) {
	a.store.View(func(l *Library) { skip = l.Settings.UpdateSkip })
	return updateSource, skip
}

// updateCheck asks the source and remembers the answer.
func (a *App) updateCheck() UpdateInfo {
	src, skip := a.updateSource()
	info := UpdateInfo{Current: version, Checked: time.Now().Format(time.RFC3339)}
	if src == "" {
		info.Error = "No update address is configured."
	} else if latest, notes, page, dl, name, size, err := fetchLatest(src); err != nil {
		info.Error = err.Error()
	} else {
		info.Latest, info.Notes, info.Page, info.Download, info.Name, info.Size = latest, notes, page, dl, name, size
		info.Available = versionNewer(latest, version)
		info.Skipped = info.Available && skip == latest
	}
	a.upd.mu.Lock()
	a.upd.info = info
	a.upd.mu.Unlock()
	return info
}

func (a *App) updateLoop() {
	time.Sleep(3 * time.Second) // right after launch
	for {
		a.updateCheck()
		time.Sleep(6 * time.Hour) // and now and then while the app stays open
	}
}

func (a *App) updateStatus() map[string]any {
	src, skip := a.updateSource()
	a.upd.mu.Lock()
	info := a.upd.info
	a.upd.mu.Unlock()
	info.Current = version
	info.Skipped = info.Available && skip == info.Latest
	return map[string]any{"source": src, "info": info, "canInstall": runtime.GOOS == "windows"}
}

// downloadInstaller fetches the update installer into the data folder. Only https, only from the places releases live.
func (a *App) downloadInstaller(info UpdateInfo, src string) (string, error) {
	u, err := neturl.Parse(info.Download)
	if err != nil || u.Scheme != "https" {
		return "", errors.New("this release has no installer to download")
	}
	host := strings.ToLower(u.Hostname())
	ok := host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
	if su, e := neturl.Parse(src); e == nil && su.Hostname() != "" && strings.EqualFold(su.Hostname(), host) {
		ok = true
	}
	if !ok {
		return "", errors.New("the installer is hosted somewhere other than the update address; open the release page instead")
	}
	if _, err := cgPublicURL(info.Download); err != nil {
		return "", err
	}
	name := filepath.Base(u.Path)
	if !strings.HasSuffix(strings.ToLower(name), ".exe") || strings.ContainsAny(name, `\/:*?"<>|`) {
		return "", errors.New("the download isn't an installer (.exe)")
	}
	dir := filepath.Join(a.dataDir, "update")
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	req, _ := http.NewRequest("GET", info.Download, nil)
	req.Header.Set("User-Agent", "ShioriBox/"+version)
	client := &http.Client{Timeout: 15 * time.Minute, Transport: cgTransport()}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed (%d)", resp.StatusCode)
	}
	if resp.ContentLength > 600<<20 {
		return "", errors.New("the installer is unexpectedly large")
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 600<<20))
	f.Close()
	if err != nil {
		os.Remove(path)
		return "", err
	}
	if info.Size > 0 && n != info.Size {
		os.Remove(path)
		return "", errors.New("the download was cut short; try again")
	}
	return path, nil
}

func (a *App) updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.updateStatus()) })
	mux.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.updateCheck()
		writeJSON(w, 200, a.updateStatus())
	})
	// "Skip this version": no popup for it any more (a newer one still shows)
	mux.HandleFunc("PUT /api/update/skip", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Version string `json:"version"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		a.store.Update(func(l *Library) error { l.Settings.UpdateSkip = strings.TrimSpace(in.Version); return nil })
		writeJSON(w, 200, a.updateStatus())
	})
	mux.HandleFunc("POST /api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if runtime.GOOS != "windows" {
			fail(w, 400, "installing updates works in the Windows app only")
			return
		}
		src, _ := a.updateSource()
		a.upd.mu.Lock()
		info := a.upd.info
		a.upd.mu.Unlock()
		if !info.Available || info.Download == "" {
			fail(w, 400, "this release has no ShioriBox-Update installer attached; open the release page instead")
			return
		}
		path, err := a.downloadInstaller(info, src)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		ok(w)
		go func() { // answer first, then hand over to the installer and get out of its way
			time.Sleep(600 * time.Millisecond)
			if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start(); err != nil {
				logf("update: could not start installer: %v", err)
				return
			}
			time.Sleep(1500 * time.Millisecond)
			os.Exit(0)
		}()
	})
}
