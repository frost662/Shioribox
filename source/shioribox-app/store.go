package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	Start   time.Time `json:"start"`
	Seconds int       `json:"seconds"`
}

type Game struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	OriginalTitle string    `json:"originalTitle"`
	Developer     string    `json:"developer"`
	ReleaseDate   string    `json:"releaseDate"`
	Description   string    `json:"description"`
	Cover         string    `json:"cover"`
	ExePath       string    `json:"exePath"`
	LaunchArgs    string    `json:"launchArgs"`
	UseWrapper    bool      `json:"useWrapper"`
	Locale        string    `json:"locale"` // "" = system, "ja" = start through a Japanese locale emulator
	SavePath      string    `json:"savePath"`
	NSFW          bool      `json:"nsfw"` // blur this game's cover in the app
	Status        string    `json:"status"`
	Rating        float64   `json:"rating"` // 0-10
	Tags          []string  `json:"tags"`
	Categories    []string  `json:"categories"`
	Notes         string    `json:"notes"`
	Progress      string    `json:"progress"` // short "where I am" note shown on the card
	Guides        []Guide   `json:"guides"`
	VNDBID        string    `json:"vndbId"`
	Favorite      bool      `json:"favorite"`
	AddedAt       time.Time `json:"addedAt"`
	LastPlayed    time.Time `json:"lastPlayed"`
	Sessions      []Session `json:"sessions"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Settings struct {
	Theme         string `json:"theme"` // dark | light
	WrapperPath   string `json:"wrapperPath"`
	WrapperArgs   string `json:"wrapperArgs"`
	AutoBackup    bool   `json:"autoBackup"`
	BackupKeep    int    `json:"backupKeep"`
	MinSessionSec int    `json:"minSessionSec"`

	// Japanese locale launching (through a locale emulator the user installs)
	LEPath        string `json:"lePath"`        // folder holding LEProc.exe ("" = detect automatically)
	DefaultLocale string `json:"defaultLocale"` // applied to newly added games
	LocaleEngine  string `json:"localeEngine"`  // "jle" (Shiori Locale, default), "le" (Locale Emulator), "custom"
	BgImage       string `json:"bgImage"`       // file in data/background ("" = none)
	BgBlur        int    `json:"bgBlur"`        // px, 0-40
	BgOverlay     int    `json:"bgOverlay"`     // % of theme colour laid over the picture, 0-95
	BgPanel       int    `json:"bgPanel"`       // % opacity of cards and sidebar, 30-100
	BgPos         string `json:"bgPos"`         // center | top | bottom
	RevealOnHover bool   `json:"revealOnHover"` // blurred covers clear up while the pointer is over them
	LocalePath    string `json:"localePath"`    // custom engine: program that takes the game as its first argument
	LocaleArgs    string `json:"localeArgs"`    // custom engine arguments, {exe} = the game ("" = just the game)
	PanicKey      string `json:"panicKey"`      // privacy hotkey, e.g. "Ctrl+Alt+H" ("" = off); managed by /api/panic/key
	PanicTapKey   string `json:"panicTapKey"`   // privacy quick taps: this key ...
	PanicTaps     int    `json:"panicTaps"`     // ... pressed this many times quickly (2-6)

	// auto-add: folders scanned for new games (see autoadd.go)
	WatchFolders  []string `json:"watchFolders"`
	AutoSeen      []string `json:"autoSeen"`      // programs auto-add has already handled, so a removed game is not added again
	AutoLookupOff bool     `json:"autoLookupOff"` // true = don't look up VNDB details for auto-added games

	// update check (see update.go)
	UpdateSource string `json:"updateSource"` // "owner/repo" on GitHub, or an https address of a version file
	UpdateOff    bool   `json:"updateOff"`    // true = no automatic daily check
	UpdateSkip   string `json:"updateSkip"`   // a version the user chose to ignore
}

type Library struct {
	Games      []*Game     `json:"games"`
	Categories []*Category `json:"categories"`
	Settings   Settings    `json:"settings"`
}

type Store struct {
	mu  sync.Mutex
	dir string
	lib Library
}

var validStatus = map[string]bool{"planned": true, "playing": true, "completed": true, "onhold": true, "dropped": true}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultSettings() Settings {
	return Settings{BgBlur: 6, BgOverlay: 55, BgPanel: 80, BgPos: "center", RevealOnHover: true, Theme: "dark", WrapperArgs: "-run {exe}", AutoBackup: true, BackupKeep: 10, MinSessionSec: 5}
}

func normalize(g *Game) {
	if g.Guides == nil {
		g.Guides = []Guide{}
	}
	if g.Tags == nil {
		g.Tags = []string{}
	}
	if g.Categories == nil {
		g.Categories = []string{}
	}
	if g.Sessions == nil {
		g.Sessions = []Session{}
	}
	if g.Locale != "ja" {
		g.Locale = ""
	}
	if !validStatus[g.Status] {
		g.Status = "planned"
	}
	if g.Rating < 0 {
		g.Rating = 0
	}
	if g.Rating > 10 {
		g.Rating = 10
	}
}

func newStore(dir string) (*Store, error) {
	for _, sub := range []string{"", "covers", "backups"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	s := &Store{dir: dir, lib: Library{Settings: defaultSettings()}}
	b, err := os.ReadFile(s.path())
	if err == nil {
		if err := json.Unmarshal(b, &s.lib); err != nil {
			// keep the unreadable file around instead of overwriting it
			os.Rename(s.path(), s.path()+".corrupt-"+time.Now().Format("20060102-150405"))
			s.lib = Library{Settings: defaultSettings()}
		}
	}
	s.fixup()
	return s, nil
}

func (s *Store) fixup() {
	if s.lib.Games == nil {
		s.lib.Games = []*Game{}
	}
	if s.lib.Categories == nil {
		s.lib.Categories = []*Category{}
	}
	for _, g := range s.lib.Games {
		normalize(g)
	}
	st := &s.lib.Settings
	if st.Theme != "light" {
		st.Theme = "dark"
	}
	st.BgBlur = clampInt(st.BgBlur, 0, 40)
	st.BgOverlay = clampInt(st.BgOverlay, 0, 95)
	st.BgPanel = clampInt(st.BgPanel, 30, 100)
	if st.BgPos != "top" && st.BgPos != "bottom" {
		st.BgPos = "center"
	}
	if st.MinSessionSec <= 0 {
		st.MinSessionSec = 5
	}
	switch st.LocaleEngine {
	case "le", "custom":
	case "classic", "", "jle":
		// Migrate removed Classic settings to the supported Shiori Locale engine.
		st.LocaleEngine = "jle"
	default:
		st.LocaleEngine = "jle"
	}
	if st.DefaultLocale != "ja" {
		st.DefaultLocale = ""
	}
	if st.WrapperArgs == "" {
		st.WrapperArgs = "-run {exe}"
	}
}

func (s *Store) path() string { return filepath.Join(s.dir, "library.json") }

func (s *Store) save() error {
	b, err := json.MarshalIndent(&s.lib, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// Update runs fn under the lock and persists if fn succeeds.
func (s *Store) Update(fn func(l *Library) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.lib); err != nil {
		return err
	}
	s.fixup()
	return s.save()
}

func (s *Store) View(fn func(l *Library)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.lib)
}

func findGame(l *Library, id string) *Game {
	for _, g := range l.Games {
		if g.ID == id {
			return g
		}
	}
	return nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
