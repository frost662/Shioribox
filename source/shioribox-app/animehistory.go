package main

import (
	"encoding/json"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HistEntry is one anime in the watch history: the last episode played and every episode played so far.
type HistEntry struct {
	Site    string   `json:"site"`
	URL     string   `json:"url"` // the series page (or the episode itself when no series page is known)
	Title   string   `json:"title"`
	Thumb   string   `json:"thumb,omitempty"`
	EpURL   string   `json:"epUrl"`
	EpTitle string   `json:"epTitle,omitempty"`
	EpN     int      `json:"epN,omitempty"`
	At      int64    `json:"at"`
	Seen    []string `json:"seen"`
}

const (
	histMaxEntries = 300
	histMaxSeen    = 400
)

var histMu sync.Mutex

func histPath(dir string) string { return filepath.Join(dir, "anime-history.json") }

func histLoad(dir string) []HistEntry {
	b, err := os.ReadFile(histPath(dir))
	if err != nil {
		return []HistEntry{}
	}
	var l []HistEntry
	if json.Unmarshal(b, &l) != nil || l == nil {
		return []HistEntry{}
	}
	return l
}

func histSave(dir string, l []HistEntry) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := histPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, histPath(dir))
}

// histRecord notes that an episode was played: the series moves to the top and the episode is marked as seen.
func histRecord(l []HistEntry, in HistEntry, now int64) []HistEntry {
	var cur HistEntry
	out := make([]HistEntry, 0, len(l)+1)
	for _, e := range l {
		if e.Site == in.Site && e.URL == in.URL {
			cur = e
			continue
		}
		out = append(out, e)
	}
	if in.Title == "" {
		in.Title = cur.Title
	}
	if in.Thumb == "" {
		in.Thumb = cur.Thumb
	}
	in.At = now
	in.Seen = cur.Seen
	has := false
	for _, u := range in.Seen {
		if u == in.EpURL {
			has = true
		}
	}
	if !has && in.EpURL != "" {
		in.Seen = append(in.Seen, in.EpURL)
	}
	if len(in.Seen) > histMaxSeen {
		in.Seen = in.Seen[len(in.Seen)-histMaxSeen:]
	}
	if in.Seen == nil {
		in.Seen = []string{}
	}
	out = append([]HistEntry{in}, out...)
	if len(out) > histMaxEntries {
		out = out[:histMaxEntries]
	}
	return out
}

func (a *App) animeHistoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/anime/history", func(w http.ResponseWriter, r *http.Request) {
		histMu.Lock()
		l := histLoad(a.dataDir)
		histMu.Unlock()
		// covers go through the image proxy, which only serves hosts it has seen: tell it about these again
		for _, e := range l {
			if s := animeSiteByID(e.Site); s != nil && strings.HasPrefix(e.Thumb, "/api/cg/img?t=") {
				if raw, err := neturl.QueryUnescape(strings.SplitN(strings.TrimPrefix(e.Thumb, "/api/cg/img?t="), "&", 2)[0]); err == nil {
					heRegisterHost(raw, s.base+"/")
				}
			}
		}
		writeJSON(w, 200, map[string]any{"items": l})
	})
	mux.HandleFunc("POST /api/anime/history", func(w http.ResponseWriter, r *http.Request) {
		var in HistEntry
		if err := decode(r, &in); err != nil || in.URL == "" || in.EpURL == "" || (animeSiteByID(in.Site) == nil && in.Site != hstreamID) {
			fail(w, 400, "bad request")
			return
		}
		histMu.Lock()
		defer histMu.Unlock()
		l := histRecord(histLoad(a.dataDir), in, time.Now().Unix())
		if err := histSave(a.dataDir, l); err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"items": l})
	})
	// one anime (?site=&url=) or, with no url, the whole history
	mux.HandleFunc("DELETE /api/anime/history", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		histMu.Lock()
		defer histMu.Unlock()
		l := []HistEntry{}
		if u := q.Get("url"); u != "" {
			for _, e := range histLoad(a.dataDir) {
				if !(e.Site == q.Get("site") && e.URL == u) {
					l = append(l, e)
				}
			}
		}
		if err := histSave(a.dataDir, l); err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"items": l})
	})
}
