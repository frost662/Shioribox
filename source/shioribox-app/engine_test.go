package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLauncherOutput(t *testing.T) {
	if p, err := parseLauncherOutput("PID 1234\r\n"); err != nil || p != 1234 {
		t.Fatalf("got %d %v", p, err)
	}
	if _, err := parseLauncherOutput("ERR boom\n"); err == nil || err.Error() != "boom" {
		t.Fatalf("got %v", err)
	}
	if _, err := parseLauncherOutput(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestPrepareEngine(t *testing.T) {
	d := t.TempDir()
	dir, err := prepareEngine(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"sbl32.dll", "sbl64.dll", "sbl_launch32.exe", "sbl_launch64.exe", "jle.exe"} {
		if st, err := os.Stat(filepath.Join(dir, n)); err != nil || st.Size() < 10000 {
			t.Fatalf("%s missing or tiny", n)
		}
	}
	os.WriteFile(filepath.Join(dir, "sbl32.dll"), []byte("x"), 0o644)
	prepareEngine(d)
	if st, _ := os.Stat(filepath.Join(dir, "sbl32.dll")); st.Size() < 10000 {
		t.Fatal("modified file not restored")
	}
}

func TestCustomLocaleArgs(t *testing.T) {
	got := customLocaleArgs("", `C:\g\a.exe`, []string{"x y"})
	if len(got) != 2 || got[0] != `C:\g\a.exe` || got[1] != `"x y"` {
		t.Fatalf("%v", got)
	}
	got = customLocaleArgs("-runas ja {exe}", `C:\g\a.exe`, nil)
	if len(got) != 3 || got[2] != `C:\g\a.exe` {
		t.Fatalf("%v", got)
	}
}

func TestIniKept(t *testing.T) {
	d := t.TempDir()
	dir, _ := prepareEngine(d)
	p := filepath.Join(dir, "jle.ini")
	os.WriteFile(p, []byte("dbcs=1"), 0o644)
	prepareEngine(d)
	if b, _ := os.ReadFile(p); string(b) != "dbcs=1" {
		t.Fatal("user ini overwritten")
	}
}
