package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Library snapshots: small timestamped copies of library.json (game details, playtime, collections).
// They are taken automatically, before risky changes, and on demand.

const snapshotKeep = 30

var snapName = regexp.MustCompile(`^library-\d{8}-\d{6}(-[a-z]+)?\.json$`)

type SnapshotInfo struct {
	Name  string    `json:"name"`
	Time  time.Time `json:"time"`
	Size  int64     `json:"size"`
	Games int       `json:"games"`
	Note  string    `json:"note"`
}

func (a *App) snapDir() string { return filepath.Join(a.dataDir, "library-backups") }

func (a *App) listSnapshots() []SnapshotInfo {
	ents, _ := os.ReadDir(a.snapDir())
	out := []SnapshotInfo{}
	for _, e := range ents {
		if e.IsDir() || !snapName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		si := SnapshotInfo{Name: e.Name(), Time: info.ModTime(), Size: info.Size()}
		if m := snapName.FindStringSubmatch(e.Name()); m[1] != "" {
			si.Note = m[1][1:]
		}
		if b, err := os.ReadFile(filepath.Join(a.snapDir(), e.Name())); err == nil {
			var l struct{ Games []json.RawMessage }
			if json.Unmarshal(b, &l) == nil {
				si.Games = len(l.Games)
			}
		}
		out = append(out, si)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

// snapshot writes a copy of the current library. note is "" (manual/auto) or a short lowercase tag.
// With onlyIfChanged it does nothing when the newest snapshot is identical.
func (a *App) snapshot(note string, onlyIfChanged bool) (string, error) {
	cur, err := os.ReadFile(a.store.path())
	if err != nil {
		return "", err
	}
	if onlyIfChanged {
		if l := a.listSnapshots(); len(l) > 0 {
			if last, err := os.ReadFile(filepath.Join(a.snapDir(), l[0].Name)); err == nil && bytes.Equal(last, cur) {
				return "", nil
			}
		}
	}
	if err := os.MkdirAll(a.snapDir(), 0o755); err != nil {
		return "", err
	}
	name := "library-" + time.Now().Format("20060102-150405")
	if note != "" {
		name += "-" + note
	}
	name += ".json"
	if err := os.WriteFile(filepath.Join(a.snapDir(), name), cur, 0o644); err != nil {
		return "", err
	}
	for i, s := range a.listSnapshots() {
		if i >= snapshotKeep {
			os.Remove(filepath.Join(a.snapDir(), s.Name))
		}
	}
	return name, nil
}

func (a *App) restoreSnapshot(name string) error {
	if !snapName.MatchString(name) {
		return errors.New("bad snapshot name")
	}
	b, err := os.ReadFile(filepath.Join(a.snapDir(), name))
	if err != nil {
		return errors.New("snapshot not found")
	}
	var lib Library
	if err := json.Unmarshal(b, &lib); err != nil {
		return errors.New("that snapshot is damaged")
	}
	a.snapshot("prerestore", true)
	return a.store.Update(func(l *Library) error { *l = lib; return nil })
}

// startSnapshots takes a snapshot at start-up and then every few hours while the app runs.
func (a *App) startSnapshots() {
	go func() {
		a.snapshot("auto", true)
		for range time.Tick(4 * time.Hour) {
			a.snapshot("auto", true)
		}
	}()
}
