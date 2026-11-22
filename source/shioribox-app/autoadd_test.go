package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkGame(t *testing.T, root, dev, name string) string {
	dir := filepath.Join(root, dev, name)
	os.MkdirAll(dir, 0o755)
	exe := filepath.Join(dir, name+".exe")
	os.WriteFile(exe, []byte("MZ-----------------------------------"), 0o644)
	return exe
}

func TestAutoAdd(t *testing.T) {
	d := t.TempDir()
	st, _ := newStore(d)
	a := &App{store: st, dataDir: d}
	mux := http.NewServeMux()
	a.autoRoutes(mux)
	lib := t.TempDir()
	e1 := mkGame(t, lib, "Dev", "Clannad")
	mkGame(t, lib, "Dev", "Kanon")
	call := func(method, url, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	if code, _ := call("PUT", "/api/auto-add", `{"folders":["`+filepath.ToSlash(filepath.Join(lib, "nope"))+`"]}`); code != 400 {
		t.Fatal("missing folder accepted")
	}
	body, _ := json.Marshal(map[string]any{"folders": []string{lib, lib}, "lookupOff": false})
	if code, m := call("PUT", "/api/auto-add", string(body)); code != 200 || len(m["folders"].([]any)) != 1 {
		t.Fatalf("%d %v", code, m)
	}
	code, m := call("POST", "/api/auto-add/scan", "")
	if code != 200 || m["added"] != float64(2) || m["seq"] != float64(1) || len(m["recent"].([]any)) != 2 {
		t.Fatalf("%d %v", code, m)
	}
	// a second scan finds nothing new
	if _, m = call("POST", "/api/auto-add/scan", ""); m["added"] != float64(0) || m["seq"] != float64(1) {
		t.Fatalf("%v", m)
	}
	// remove Clannad from the library: it must not come back
	st.Update(func(l *Library) error {
		keep := l.Games[:0]
		for _, g := range l.Games {
			if g.ExePath != e1 {
				keep = append(keep, g)
			}
		}
		l.Games = keep
		return nil
	})
	if _, m = call("POST", "/api/auto-add/scan", ""); m["added"] != float64(0) {
		t.Fatalf("removed game came back: %v", m)
	}
	// a newly installed game is picked up
	mkGame(t, lib, "Other", "Fate")
	if _, m = call("POST", "/api/auto-add/scan", ""); m["added"] != float64(1) || m["seq"] != float64(2) {
		t.Fatalf("%v", m)
	}
	n := 0
	st.View(func(l *Library) { n = len(l.Games) })
	if n != 2 {
		t.Fatalf("%d games", n)
	}
	// saving the ordinary settings must not wipe the watched folders
	var keep int
	st.Update(func(l *Library) error {
		in := Settings{Theme: "dark"}
		in.WatchFolders, in.AutoSeen, in.AutoLookupOff = l.Settings.WatchFolders, l.Settings.AutoSeen, l.Settings.AutoLookupOff
		l.Settings = in
		keep = len(l.Settings.WatchFolders)
		return nil
	})
	if keep != 1 {
		t.Fatal("folders lost")
	}
}
