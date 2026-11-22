package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type fsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type fsPlace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type fsResp struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []fsEntry `json:"entries"`
	Places  []fsPlace `json:"places"`
}

func fsPlaces() []fsPlace {
	var out []fsPlace
	home, _ := os.UserHomeDir()
	if home != "" {
		out = append(out, fsPlace{"Home", home})
		for _, n := range []string{"Desktop", "Documents", "Downloads"} {
			p := filepath.Join(home, n)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				out = append(out, fsPlace{n, p})
			}
		}
	}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			root := string(c) + ":\\"
			if _, err := os.Stat(root); err == nil {
				out = append(out, fsPlace{string(c) + ": drive", root})
			}
		}
	} else {
		out = append(out, fsPlace{"Root /", "/"})
	}
	return out
}

// listFS lists a directory. exts (lowercase, with dot) filters files; nil shows none when dirsOnly.
func listFS(path string, exts map[string]bool, dirsOnly bool) (*fsResp, error) {
	resp := &fsResp{Places: fsPlaces(), Entries: []fsEntry{}}
	if path == "" {
		if runtime.GOOS == "windows" {
			for _, p := range resp.Places {
				if strings.HasSuffix(p.Name, "drive") {
					resp.Entries = append(resp.Entries, fsEntry{Name: p.Path, Path: p.Path, IsDir: true})
				}
			}
			return resp, nil
		}
		if h, err := os.UserHomeDir(); err == nil {
			path = h
		} else {
			path = "/"
		}
	}
	path = filepath.Clean(path)
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	resp.Path = path
	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}
	resp.Parent = parent
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
			continue
		}
		full := filepath.Join(path, name)
		isDir := e.IsDir()
		if !isDir {
			if dirsOnly {
				continue
			}
			if exts != nil && !exts[strings.ToLower(filepath.Ext(name))] {
				continue
			}
		}
		var size int64
		if !isDir {
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
		}
		resp.Entries = append(resp.Entries, fsEntry{Name: name, Path: full, IsDir: isDir, Size: size})
	}
	sort.Slice(resp.Entries, func(i, j int) bool {
		a, b := resp.Entries[i], resp.Entries[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return resp, nil
}
