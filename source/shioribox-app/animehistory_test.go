package main

import "testing"

func TestHistRecord(t *testing.T) {
	var l []HistEntry
	l = histRecord(l, HistEntry{Site: "akih", URL: "s1", Title: "One", EpURL: "e1", EpN: 1}, 10)
	l = histRecord(l, HistEntry{Site: "akih", URL: "s2", Title: "Two", EpURL: "x1"}, 20)
	l = histRecord(l, HistEntry{Site: "akih", URL: "s1", EpURL: "e2", EpN: 2}, 30)
	l = histRecord(l, HistEntry{Site: "akih", URL: "s1", EpURL: "e2", EpN: 2}, 40)
	if len(l) != 2 || l[0].URL != "s1" || l[0].Title != "One" || l[0].EpN != 2 || len(l[0].Seen) != 2 || l[0].At != 40 || l[1].URL != "s2" {
		t.Fatalf("%+v", l)
	}
	d := t.TempDir()
	if err := histSave(d, l); err != nil || len(histLoad(d)) != 2 {
		t.Fatal("save/load", err)
	}
}
