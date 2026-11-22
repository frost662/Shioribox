package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseHotkey(t *testing.T) {
	for _, c := range []struct {
		in, norm string
		mods, vk uint32
		bad      bool
	}{
		{"ctrl+alt+h", "Ctrl+Alt+H", modCtrl | modAlt, 'H', false},
		{"Shift + Ctrl + F9", "Ctrl+Shift+F9", modCtrl | modShift, 0x78, false},
		{"F12", "F12", 0, 0x7B, false},
		{"Win+`", "Win+`", modWin, 0xC0, false},
		{"Alt+pageup", "Alt+PageUp", modAlt, 0x21, false},
		{"", "", 0, 0, false},
		{"H", "", 0, 0, true},
		{"Ctrl+Alt", "", 0, 0, true},
		{"Ctrl+A+B", "", 0, 0, true},
		{"Ctrl+Nope", "", 0, 0, true},
	} {
		m, v, n, err := parseHotkey(c.in)
		if (err != nil) != c.bad || (!c.bad && (n != c.norm || m != c.mods || v != c.vk)) {
			t.Errorf("%q -> %v %v %q %v", c.in, m, v, n, err)
		}
	}
}

type fakePanic struct {
	mods, vk uint32
	tapVK    uint32
	taps     int
	err      error
	hits     chan bool
}

func (f *fakePanic) Register(m, v uint32) error { f.mods, f.vk = m, v; return f.err }
func (f *fakePanic) Trigger()                   { f.hits <- true }
func (f *fakePanic) RegisterTaps(vk uint32, n int) error {
	f.tapVK, f.taps = vk, n
	return f.err
}

func TestPanicEndpoints(t *testing.T) {
	d := t.TempDir()
	st, _ := newStore(d)
	a := &App{store: st, dataDir: d}
	mux := http.NewServeMux()
	a.panicRoutes(mux)
	f := &fakePanic{hits: make(chan bool, 1)}
	panicMu.Lock()
	panicUI = f
	panicMu.Unlock()
	defer func() { panicMu.Lock(); panicUI = nil; panicMu.Unlock() }()
	do := func(method, url, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	code, m := do("PUT", "/api/panic/key", `{"key":"ctrl+alt+h"}`)
	if code != 200 || m["key"] != "Ctrl+Alt+H" || m["registered"] != true || f.vk != 'H' || f.mods != modCtrl|modAlt {
		t.Fatalf("%d %v %+v", code, m, f)
	}
	if code, _ := do("PUT", "/api/panic/key", `{"key":"H"}`); code != 400 {
		t.Fatal("bare key accepted")
	}
	f.err = errors.New("taken")
	code, m = do("PUT", "/api/panic/key", `{"key":"Ctrl+Alt+J"}`)
	if code != 200 || m["registered"] != false || m["error"] != "taken" {
		t.Fatalf("%v", m)
	}
	f.err = nil
	do("PUT", "/api/panic/key", `{"key":""}`)
	if f.vk != 0 {
		t.Fatal("hotkey not removed")
	}
	if code, _ := do("POST", "/api/panic/now", ""); code != 200 {
		t.Fatal(code)
	}
	<-f.hits
	var key string
	st.View(func(l *Library) { key = l.Settings.PanicKey })
	if key != "" {
		t.Fatal(key)
	}
}

func TestTapDetector(t *testing.T) {
	d := tapDetector{vk: 0x1B, need: 3, gap: tapGapMs}
	press := func(vk uint32, at uint32) bool {
		a := d.Event(vk, true, at)
		d.Event(vk, false, at+20)
		return a
	}
	if press(0x1B, 0) || press(0x1B, 200) || !press(0x1B, 400) {
		t.Fatal("three quick presses should fire on the third")
	}
	if press(0x1B, 1000) || press(0x1B, 1200) || press(0x1B, 1900) { // too slow between 2nd and 3rd
		t.Fatal("slow presses fired")
	}
	d.n = 0
	if press(0x1B, 3000) || press(0x41, 3100) || press(0x1B, 3200) || press(0x1B, 3300) { // another key resets the count
		t.Fatal("interrupted sequence fired")
	}
	d.n = 0
	d.Event(0x1B, true, 5000)
	if d.Event(0x1B, true, 5050) || d.Event(0x1B, true, 5100) { // key held: auto-repeat counts once
		t.Fatal("auto-repeat counted")
	}
	if (&tapDetector{}).Event(0x1B, true, 1) {
		t.Fatal("unset detector fired")
	}
	if vk, n, err := parseTapKey("esc"); vk != 0x1B || n != "Esc" || err != nil {
		t.Fatal(vk, n, err)
	}
	if _, _, err := parseTapKey("Nope"); err == nil {
		t.Fatal("bad key accepted")
	}
}

func TestTapEndpoint(t *testing.T) {
	d := t.TempDir()
	st, _ := newStore(d)
	a := &App{store: st, dataDir: d}
	mux := http.NewServeMux()
	a.panicRoutes(mux)
	f := &fakePanic{hits: make(chan bool, 1)}
	panicMu.Lock()
	panicUI = f
	panicMu.Unlock()
	defer func() { panicMu.Lock(); panicUI = nil; panicMu.Unlock() }()
	put := func(body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/panic/taps", strings.NewReader(body)))
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	code, m := put(`{"key":"esc","count":3}`)
	if code != 200 || m["tapKey"] != "Esc" || m["taps"] != float64(3) || m["tapsRegistered"] != true || f.tapVK != 0x1B || f.taps != 3 {
		t.Fatalf("%d %v %+v", code, m, f)
	}
	if code, _ := put(`{"key":"Esc","count":1}`); code != 400 {
		t.Fatal("1 press accepted")
	}
	put(`{"key":"","count":3}`)
	if f.tapVK != 0 {
		t.Fatal("not removed")
	}
}
