package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearWebCache(t *testing.T) {
	d := t.TempDir()
	cache := filepath.Join(d, "EBWebView", "Default", "Cache", "Cache_Data")
	keep := filepath.Join(d, "EBWebView", "Default", "Local Storage")
	os.MkdirAll(cache, 0o755)
	os.MkdirAll(keep, 0o755)
	os.WriteFile(filepath.Join(cache, "f_000001"), []byte("x"), 0o644)
	if n := clearWebCache(d); n != 1 {
		t.Fatalf("removed %d", n)
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("cache still there")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("local storage must survive")
	}
	if clearWebCache(filepath.Join(d, "nope")) != 0 {
		t.Fatal("nothing to remove")
	}
}
