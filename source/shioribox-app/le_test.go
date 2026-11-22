package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = "\ufeff" + `<?xml version="1.0" encoding="utf-8"?>
<LEConfig>
  <Profiles>
    <Profile Name="Run in Chinese" Guid="aaaa-1" MainMenu="false"><Parameter></Parameter><Location>zh-CN</Location><Timezone>China Standard Time</Timezone></Profile>
    <Profile Name="Run in Japanese" Guid="c0ffee-ja" MainMenu="true"><Parameter></Parameter><Location>ja-JP</Location><Timezone>Tokyo Standard Time</Timezone></Profile>
  </Profiles>
</LEConfig>`

func TestReadJapaneseProfile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := readJapaneseProfile(dir); ok {
		t.Fatal("no config should give no profile")
	}
	os.WriteFile(filepath.Join(dir, "LEConfig.xml"), []byte(sampleConfig), 0o644)
	p, ok := readJapaneseProfile(dir)
	if !ok || p.Guid != "c0ffee-ja" || p.Name != "Run in Japanese" {
		t.Fatalf("got %+v ok=%v", p, ok)
	}
}

func TestLeCommand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, leExeName), []byte("x"), 0o755)
	st := LEStatus{Available: true, Dir: dir}
	prog, args := leCommand(st, "/g/game.exe", nil)
	if filepath.Base(prog) != leExeName || len(args) != 1 || args[0] != "/g/game.exe" {
		t.Fatalf("no-profile command wrong: %v %v", prog, args)
	}
	os.WriteFile(filepath.Join(dir, "LEConfig.xml"), []byte(sampleConfig), 0o644)
	_, args = leCommand(st, "/g/game.exe", nil)
	if strings.Join(args, "|") != "-runas|c0ffee-ja|/g/game.exe" {
		t.Fatalf("profile command wrong: %v", args)
	}
}

func TestStatusConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	if s := leStatus(Settings{LEPath: dir}); s.Available || s.Problem == "" {
		t.Fatalf("empty folder must be unavailable: %+v", s)
	}
	os.WriteFile(filepath.Join(dir, leExeName), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(dir, "LEConfig.xml"), []byte(sampleConfig), 0o644)
	s := leStatus(Settings{LEPath: dir})
	if !s.Available || s.Profile != "Run in Japanese" || s.Auto {
		t.Fatalf("configured folder not recognised: %+v", s)
	}
}

func TestFindInTree(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "Locale.Emulator.2.5", "x")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(root, "Locale.Emulator.2.5", leExeName), []byte("x"), 0o755)
	if got := findInTree(root, 2); filepath.Base(got) != "Locale.Emulator.2.5" {
		t.Fatalf("got %q", got)
	}
	if got := findInTree(root, 0); got != "" {
		t.Fatalf("depth 0 should not descend, got %q", got)
	}
}

func TestExeBitsAndCheck(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notpe.exe")
	os.WriteFile(f, []byte("hello"), 0o644)
	if exeBits(f) != 0 || checkLEGame(f) != nil {
		t.Fatal("non-PE files must not be rejected")
	}
}

func TestLeCommandForwardsArgs(t *testing.T) {
	dir := t.TempDir()
	st := LEStatus{Available: true, Dir: dir}
	_, args := leCommand(st, "/g/game.exe", []string{"-windowed", "my folder"})
	if strings.Join(args, "|") != "/g/game.exe|-windowed|\"my folder\"" {
		t.Fatalf("args wrong: %q", args)
	}
}
