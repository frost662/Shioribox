package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuides(t *testing.T) {
	d := t.TempDir()
	st, _ := newStore(d)
	a := &App{store: st, dataDir: d}
	st.Update(func(l *Library) error { l.Games = append(l.Games, &Game{ID: "g1", Title: "A"}); return nil })

	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	g, err := a.addGuide("g1", `C:\walk\route 1.png`, png)
	if err != nil || g.Kind != "image" || g.Name != "route 1.png" {
		t.Fatalf("%+v %v", g, err)
	}
	sj := []byte{0x93, 0xFA, 0x96, 0x7B, 0x8C, 0xEA} // 日本語 in Shift-JIS
	tg, err := a.addGuide("g1", "かいせつ.txt", sj)
	if err != nil || tg.Kind != "text" {
		t.Fatalf("%+v %v", tg, err)
	}
	if txt, err := a.guideText("g1", tg.ID); err != nil || txt != "日本語" {
		t.Fatalf("decoded %q %v", txt, err)
	}
	if _, err := a.addGuide("g1", "x.exe", []byte("MZ\x00\x00binary")); err == nil {
		t.Fatal("binary accepted")
	}
	if _, err := a.addGuide("g1", "page.html", []byte("<script>x</script>")); err == nil {
		t.Fatal("html accepted")
	}
	if _, err := a.addGuide("missing", "a.txt", []byte("hi")); err == nil {
		t.Fatal("unknown game accepted")
	}
	if _, err := os.Stat(filepath.Join(a.guideDir("g1"), g.File)); err != nil {
		t.Fatal(err)
	}
	a.removeGuide("g1", g.ID)
	if _, err := os.Stat(filepath.Join(a.guideDir("g1"), g.File)); err == nil {
		t.Fatal("file not removed")
	}
	st.View(func(l *Library) {
		if len(l.Games[0].Guides) != 1 {
			t.Fatal("record not removed")
		}
	})
}

func TestDecodeText(t *testing.T) {
	if decodeText([]byte("\xEF\xBB\xBFhello")) != "hello" {
		t.Fatal("bom")
	}
	if decodeText([]byte("日本")) != "日本" {
		t.Fatal("utf8")
	}
}
