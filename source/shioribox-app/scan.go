package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ScanExe struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ScanItem struct {
	Title  string    `json:"title"`
	Dir    string    `json:"dir"`
	Exes   []ScanExe `json:"exes"`
	Chosen string    `json:"chosen"`
	Exists bool      `json:"exists"`
}

type ScanResult struct {
	Items   []ScanItem `json:"items"`
	Dirs    int        `json:"dirs"`    // folders looked into
	Exes    int        `json:"exes"`    // .exe files seen in total
	Skipped []string   `json:"skipped"` // names ignored as installers/tools
	Root    string     `json:"root"`
}

const (
	maxScanDepth = 6
	maxScanDirs  = 30000
)

var junkExe = []string{"unins", "uninst", "uninstall", "vcredist", "vc_redist", "dxsetup", "dxwebsetup",
	"directx", "crashhandler", "crashpad", "crashreport", "notification_helper", "updater", "dotnet",
	"ndp4", "oalinst", "_redist", "redist"}

// tools that are usually not the game itself but may be the only exe present
var toolExe = []string{"config", "setting", "launcher_config", "setup"}

var genericDirs = map[string]bool{"bin": true, "bin32": true, "bin64": true, "x86": true, "x64": true, "win32": true,
	"win64": true, "windows": true, "game": true, "app": true, "release": true, "program": true, "launcher": true,
	"binaries": true, "client": true, "exe": true}

var skipDirs = map[string]bool{"system volume information": true, "$recycle.bin": true, "windows": true,
	"node_modules": true, "__macosx": true}

func hasAny(name string, words []string) bool {
	n := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	for _, j := range words {
		if strings.Contains(n, j) {
			return true
		}
	}
	return false
}

func normPath(p string) string { return strings.ToLower(filepath.Clean(p)) }

// listExes returns the .exe files in dir and (up to relDepth levels) below it.
func listExes(dir string, relDepth int, res *ScanResult) (good, tools []ScanExe) {
	base := strings.Count(filepath.Clean(dir), string(os.PathSeparator))
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(p, string(os.PathSeparator))-base >= relDepth+1 {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".exe") {
			return nil
		}
		var sz int64
		if info, err := d.Info(); err == nil {
			sz = info.Size()
		}
		x := ScanExe{Path: p, Size: sz}
		switch {
		case hasAny(d.Name(), junkExe):
			if len(res.Skipped) < 12 {
				res.Skipped = append(res.Skipped, d.Name())
			}
		case hasAny(d.Name(), toolExe):
			tools = append(tools, x)
		default:
			good = append(good, x)
		}
		return nil
	})
	return
}

// scanLibrary finds games below root. A folder that directly contains an executable is a game;
// folders without one are searched deeper (Developer\Game\game.exe, Game\bin\game.exe, ...).
// If root itself is a single game folder, it is returned as one game.
func scanLibrary(root string, known map[string]bool) (*ScanResult, error) {
	root = filepath.Clean(root)
	if _, err := os.ReadDir(root); err != nil {
		return nil, err
	}
	res := &ScanResult{Items: []ScanItem{}, Root: root, Skipped: []string{}}
	seen := map[string]bool{}

	addGame := func(dir string) {
		title := filepath.Base(dir)
		for genericDirs[strings.ToLower(title)] && filepath.Dir(dir) != dir && normPath(dir) != normPath(root) {
			dir = filepath.Dir(dir)
			title = filepath.Base(dir)
		}
		if seen[normPath(dir)] {
			return
		}
		good, tools := listExes(dir, 2, res)
		exes := good
		if len(exes) == 0 {
			exes = tools
		} else {
			exes = append(exes, tools...)
		}
		if len(exes) == 0 {
			return
		}
		seen[normPath(dir)] = true
		sort.SliceStable(exes, func(i, j int) bool {
			// non-tool exes first, then biggest
			ti, tj := hasAny(filepath.Base(exes[i].Path), toolExe), hasAny(filepath.Base(exes[j].Path), toolExe)
			if ti != tj {
				return !ti
			}
			return exes[i].Size > exes[j].Size
		})
		chosen := exes[0].Path
		ltitle := strings.ToLower(title)
		for _, x := range exes {
			b := strings.ToLower(strings.TrimSuffix(filepath.Base(x.Path), filepath.Ext(x.Path)))
			if b != "" && !hasAny(b, toolExe) && (strings.Contains(ltitle, b) || strings.Contains(b, ltitle)) {
				chosen = x.Path
				break
			}
		}
		it := ScanItem{Title: title, Dir: dir, Exes: exes, Chosen: chosen}
		for _, x := range exes {
			if known[normPath(x.Path)] {
				it.Exists = true
			}
		}
		res.Items = append(res.Items, it)
	}

	var visit func(dir string, depth int)
	visit = func(dir string, depth int) {
		if res.Dirs >= maxScanDirs {
			return
		}
		res.Dirs++
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		var subs []string
		direct := false
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() {
				if strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)] {
					continue
				}
				subs = append(subs, filepath.Join(dir, name))
			} else if strings.EqualFold(filepath.Ext(name), ".exe") {
				res.Exes++
				if !hasAny(name, junkExe) {
					direct = true
				}
			}
		}
		if direct && depth > 0 {
			addGame(dir)
			return
		}
		if depth >= maxScanDepth {
			return
		}
		for _, s := range subs {
			visit(s, depth+1)
		}
	}
	visit(root, 0)
	// the chosen folder may itself be one game
	if len(res.Items) == 0 {
		if ents, err := os.ReadDir(root); err == nil {
			for _, e := range ents {
				if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
					addGame(root)
					break
				}
			}
		}
	}
	sort.Slice(res.Items, func(i, j int) bool { return strings.ToLower(res.Items[i].Title) < strings.ToLower(res.Items[j].Title) })
	return res, nil
}
