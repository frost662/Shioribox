package main

import (
	"os"
	"testing"
)

func TestSnapshots(t *testing.T) {
	d := t.TempDir()
	st, err := newStore(d)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: st, dataDir: d}
	st.Update(func(l *Library) error { l.Games = append(l.Games, &Game{ID: "a", Title: "A"}); return nil })
	n, err := a.snapshot("manual", false)
	if err != nil || n == "" {
		t.Fatal(n, err)
	}
	if again, _ := a.snapshot("auto", true); again != "" {
		t.Fatal("identical library should not be snapshotted again")
	}
	st.Update(func(l *Library) error { l.Games = nil; return nil })
	if err := a.restoreSnapshot(n); err != nil {
		t.Fatal(err)
	}
	st.View(func(l *Library) {
		if len(l.Games) != 1 || l.Games[0].Title != "A" {
			t.Fatal("restore failed")
		}
	})
	if len(a.listSnapshots()) < 2 {
		t.Fatal("pre-restore snapshot missing")
	}
	if a.restoreSnapshot("../x.json") == nil {
		t.Fatal("bad name accepted")
	}
	os.RemoveAll(d)
}
