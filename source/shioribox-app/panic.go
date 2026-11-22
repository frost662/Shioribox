package main

// Privacy hotkey: one global key combination (set in Settings) that blurs the app and hides it to the tray in an
// instant, and brings it back when pressed again. The app keeps running, so nothing is lost: the library, the open
// page, running games and their play timers all stay exactly as they were.

import (
	"errors"
	"net/http"
	"strings"
	"sync"
)

// panicBackend is the native side (Windows only): it owns the global hotkey, the window and the tray icon.
type panicBackend interface {
	Register(mods, vk uint32) error          // vk == 0 removes the hotkey
	RegisterTaps(vk uint32, count int) error // press one key count times quickly; vk == 0 removes it
	Trigger()                                // hide if visible, show if hidden
}

var (
	panicUI  panicBackend
	panicMu  sync.Mutex
	panicErr string // why the hotkey could not be registered ("" = fine)
	panicOn  bool   // a hotkey is currently registered

	panicTapErr string
	panicTapOn  bool
)

const (
	tapGapMs = 500 // longest pause between two presses that still count as one quick sequence
	minTaps  = 2
	maxTaps  = 6
)

// tapDetector recognises "press this key N times quickly". Any other key pressed in between starts the count over,
// and a key held down (auto-repeat) counts once. It sees every key but never swallows one.
type tapDetector struct {
	vk   uint32
	need int
	gap  uint32
	n    int
	last uint32
	held bool
}

// Event takes one key event (time in milliseconds) and reports true when the sequence has just been completed.
func (d *tapDetector) Event(vk uint32, down bool, t uint32) bool {
	if d.vk == 0 || d.need < minTaps {
		return false
	}
	if vk != d.vk {
		if down {
			d.n = 0
		}
		return false
	}
	if !down {
		d.held = false
		return false
	}
	if d.held {
		return false
	}
	d.held = true
	if d.n > 0 && t-d.last > d.gap {
		d.n = 0
	}
	d.n++
	d.last = t
	if d.n >= d.need {
		d.n = 0
		return true
	}
	return false
}

// parseTapKey reads a single key name ("Esc", "F8", "A") with no modifiers.
func parseTapKey(s string) (vk uint32, norm string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", nil
	}
	if len(s) == 1 {
		s = strings.ToUpper(s)
	}
	for k, v := range hotkeyVK {
		if strings.EqualFold(k, s) {
			return v, k, nil
		}
	}
	return 0, "", errors.New("that key can't be used")
}

const (
	modAlt   = 0x1
	modCtrl  = 0x2
	modShift = 0x4
	modWin   = 0x8
)

var hotkeyVK = func() map[string]uint32 {
	m := map[string]uint32{
		"Space": 0x20, "Tab": 0x09, "Backspace": 0x08, "Enter": 0x0D, "Esc": 0x1B,
		"PageUp": 0x21, "PageDown": 0x22, "End": 0x23, "Home": 0x24, "Left": 0x25, "Up": 0x26, "Right": 0x27, "Down": 0x28,
		"Insert": 0x2D, "Delete": 0x2E, "`": 0xC0, "-": 0xBD, "=": 0xBB, "[": 0xDB, "]": 0xDD, "\\": 0xDC, ";": 0xBA, "'": 0xDE, ",": 0xBC, ".": 0xBE, "/": 0xBF,
	}
	for c := 'A'; c <= 'Z'; c++ {
		m[string(c)] = uint32(c)
	}
	for c := '0'; c <= '9'; c++ {
		m[string(c)] = uint32(c)
	}
	for i := 1; i <= 12; i++ {
		m["F"+itoa(i)] = uint32(0x6F + i)
	}
	return m
}()

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// parseHotkey reads "Ctrl+Alt+H". An empty string means "no hotkey". A modifier is required unless the key is F1-F12.
func parseHotkey(s string) (mods, vk uint32, norm string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, "", nil
	}
	var key string
	for _, part := range strings.Split(s, "+") {
		p := strings.TrimSpace(part)
		switch strings.ToLower(p) {
		case "ctrl", "control":
			mods |= modCtrl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "win", "meta", "super":
			mods |= modWin
		case "":
			if part == "" && strings.HasSuffix(s, "++") { // the "+" key itself is not supported
				return 0, 0, "", errors.New("that key can't be used")
			}
		default:
			if key != "" {
				return 0, 0, "", errors.New("use one key plus modifiers, like Ctrl+Alt+H")
			}
			key = p
			if len(p) == 1 {
				key = strings.ToUpper(p)
			}
		}
	}
	code, ok := hotkeyVK[key]
	if !ok {
		for k, v := range hotkeyVK { // names are matched without regard to case ("pageup" -> "PageUp")
			if strings.EqualFold(k, key) {
				key, code, ok = k, v, true
				break
			}
		}
	}
	if !ok {
		return 0, 0, "", errors.New("that key can't be used")
	}
	isF := len(key) >= 2 && key[0] == 'F' && key[1] >= '0' && key[1] <= '9'
	if mods == 0 && !isF {
		return 0, 0, "", errors.New("add Ctrl, Alt, Shift or Win so you don't lose a normal key")
	}
	var parts []string
	if mods&modCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&modAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&modShift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&modWin != 0 {
		parts = append(parts, "Win")
	}
	return mods, code, strings.Join(append(parts, key), "+"), nil
}

// applyPanic (re)registers the configured hotkey with the native side.
func (a *App) applyPanic() {
	var key string
	a.store.View(func(l *Library) { key = l.Settings.PanicKey })
	mods, vk, _, err := parseHotkey(key)
	panicMu.Lock()
	ui := panicUI
	panicMu.Unlock()
	msg := ""
	switch {
	case err != nil:
		msg = err.Error()
	case ui == nil:
		if vk != 0 {
			msg = "The global hotkey works in the Windows app only."
		}
	default:
		if e := ui.Register(mods, vk); e != nil {
			msg = e.Error()
		}
	}
	panicMu.Lock()
	panicErr, panicOn = msg, msg == "" && vk != 0 && ui != nil
	panicMu.Unlock()

	// quick taps
	var tapKey string
	var taps int
	a.store.View(func(l *Library) { tapKey, taps = l.Settings.PanicTapKey, l.Settings.PanicTaps })
	tvk, _, terr := parseTapKey(tapKey)
	if taps < minTaps || taps > maxTaps {
		taps = 3
	}
	tmsg := ""
	switch {
	case terr != nil:
		tmsg = terr.Error()
	case ui == nil:
		if tvk != 0 {
			tmsg = "Quick taps work in the Windows app only."
		}
	default:
		if e := ui.RegisterTaps(tvk, taps); e != nil {
			tmsg = e.Error()
		}
	}
	panicMu.Lock()
	panicTapErr, panicTapOn = tmsg, tmsg == "" && tvk != 0 && ui != nil
	panicMu.Unlock()
}

func (a *App) panicRoutes(mux *http.ServeMux) {
	status := func() map[string]any {
		var key, tapKey string
		var taps int
		a.store.View(func(l *Library) {
			key, tapKey, taps = l.Settings.PanicKey, l.Settings.PanicTapKey, l.Settings.PanicTaps
		})
		if taps < minTaps || taps > maxTaps {
			taps = 3
		}
		panicMu.Lock()
		defer panicMu.Unlock()
		return map[string]any{"key": key, "native": panicUI != nil, "registered": panicOn, "error": panicErr,
			"tapKey": tapKey, "taps": taps, "tapsRegistered": panicTapOn, "tapsError": panicTapErr}
	}
	mux.HandleFunc("GET /api/panic", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, status()) })
	mux.HandleFunc("PUT /api/panic/key", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Key string `json:"key"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		_, _, norm, err := parseHotkey(in.Key)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		a.store.Update(func(l *Library) error { l.Settings.PanicKey = norm; return nil })
		a.applyPanic()
		writeJSON(w, 200, status())
	})
	mux.HandleFunc("PUT /api/panic/taps", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Key   string `json:"key"`
			Count int    `json:"count"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, 400, "bad request")
			return
		}
		_, norm, err := parseTapKey(in.Key)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if in.Count < minTaps || in.Count > maxTaps {
			fail(w, 400, "choose between 2 and 6 presses")
			return
		}
		a.store.Update(func(l *Library) error { l.Settings.PanicTapKey, l.Settings.PanicTaps = norm, in.Count; return nil })
		a.applyPanic()
		writeJSON(w, 200, status())
	})
	// "Try it": hide now, exactly as the hotkey would.
	mux.HandleFunc("POST /api/panic/now", func(w http.ResponseWriter, r *http.Request) {
		panicMu.Lock()
		ui := panicUI
		panicMu.Unlock()
		if ui == nil {
			fail(w, 400, "hiding to the tray works in the Windows app only")
			return
		}
		go ui.Trigger()
		ok(w)
	})
}
