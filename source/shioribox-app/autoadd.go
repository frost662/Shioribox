package main

// Auto-add: folders listed in Settings are scanned for games now and then; anything new goes into the library by itself.
// A game that was added this way and later removed by the user is not added again (its programs stay in AutoSeen).

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const autoScanEvery = 15 * time.Minute

type autoState struct {
	mu     sync.Mutex
	scan   sync.Mutex // one scan at a time
	seq    int        // bumps whenever a scan added games
	last   time.Time
	recent []*Game
	err    string
	busy   bool
}

// autoScan scans every watched folder and adds the games it has not seen before. It returns the games it added.
func (a *App) autoScan() []*Game {
	a.auto.scan.Lock()
	defer a.auto.scan.Unlock()
	a.auto.mu.Lock()
	a.auto.busy = true
	a.auto.mu.Unlock()

	var folders []string
	known := map[string]bool{}
	seen := map[string]bool{}
	var locale string
	a.store.View(func(l *Library) {
		folders = append(folders, l.Settings.WatchFolders...)
		locale = l.Settings.DefaultLocale
		for _, g := range l.Games {
			if g.ExePath != "" {
				known[normPath(g.ExePath)] = true
			}
		}
		for _, p := range l.Settings.AutoSeen {
			seen[normPath(p)] = true
		}
	})
	type found struct {
		title, exe string
		all        []string
	}
	var add []found
	var errs []string
	for _, f := range folders {
		res, err := scanLibrary(f, known)
		if err != nil {
			errs = append(errs, filepath.Base(f)+": "+err.Error())
			continue
		}
		for _, it := range res.Items {
			if it.Exists || it.Chosen == "" || seen[normPath(it.Chosen)] || known[normPath(it.Chosen)] {
				continue
			}
			fd := found{title: it.Title, exe: it.Chosen}
			for _, x := range it.Exes {
				fd.all = append(fd.all, x.Path)
			}
			add = append(add, fd)
			known[normPath(it.Chosen)] = true // two watched folders may overlap
		}
	}
	var created []*Game
	if len(add) > 0 {
		a.store.Update(func(l *Library) error {
			for _, fd := range add {
				g := &Game{ID: newID(), Title: strings.TrimSpace(fd.title), ExePath: fd.exe, Locale: locale, AddedAt: time.Now()}
				normalize(g)
				l.Games = append(l.Games, g)
				created = append(created, g)
				l.Settings.AutoSeen = append(l.Settings.AutoSeen, fd.all...)
			}
			return nil
		})
	}
	a.auto.mu.Lock()
	a.auto.busy = false
	a.auto.last = time.Now()
	a.auto.err = strings.Join(errs, "; ")
	if len(created) > 0 {
		a.auto.seq++
		a.auto.recent = created
	}
	a.auto.mu.Unlock()
	return created
}

func (a *App) autoLoop() {
	time.Sleep(6 * time.Second) // let the app finish starting first
	for {
		var n int
		a.store.View(func(l *Library) { n = len(l.Settings.WatchFolders) })
		if n > 0 {
			if c := a.autoScan(); len(c) > 0 {
				logf("auto-add: added %d game(s)", len(c))
			}
		}
		time.Sleep(autoScanEvery)
	}
}

func (a *App) autoStatus() map[string]any {
	var folders []string
	var off bool
	a.store.View(func(l *Library) {
		folders = append([]string{}, l.Settings.WatchFolders...)
		off = l.Settings.AutoLookupOff
	})
	if folders == nil {
		folders = []string{}
	}
	a.auto.mu.Lock()
	defer a.auto.mu.Unlock()
	last := ""
	if !a.auto.last.IsZero() {
		last = a.auto.last.Format(time.RFC3339)
	}
	recent := a.auto.recent
	if recent == nil {
		recent = []*Game{}
	}
	return map[string]any{"folders": folders, "lookupOff": off, "last": last, "seq": a.auto.seq, "recent": recent, "error": a.auto.err, "busy": a.auto.busy}
}

func (a *App) autoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auto-add", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.autoStatus()) })
	mux.HandleFunc("PUT /api/auto-add", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Folders   []string `json:"folders"`
			LookupOff bool     `json:"lookupOff"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		var clean []string
		have := map[string]bool{}
		for _, f := range in.Folders {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			fi, err := os.Stat(f)
			if err != nil || !fi.IsDir() {
				fail(w, 400, "that folder doesn't exist: "+f)
				return
			}
			if k := normPath(f); !have[k] {
				have[k] = true
				clean = append(clean, filepath.Clean(f))
			}
		}
		a.store.Update(func(l *Library) error {
			l.Settings.WatchFolders, l.Settings.AutoLookupOff = clean, in.LookupOff
			return nil
		})
		writeJSON(w, 200, a.autoStatus())
	})
	// scan now and wait for the result
	mux.HandleFunc("POST /api/auto-add/scan", func(w http.ResponseWriter, r *http.Request) {
		created := a.autoScan()
		st := a.autoStatus()
		st["added"] = len(created)
		writeJSON(w, 200, st)
	})
}
