package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Guides: walkthrough images and text files attached to a game. Files live in data/guides/<game id>/.

const (
	maxGuideImage = 25 << 20
	maxGuideText  = 5 << 20
	maxGuides     = 200
)

type Guide struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	File  string    `json:"file"`
	Kind  string    `json:"kind"` // image | text
	Size  int       `json:"size"`
	Added time.Time `json:"added"`
}

func (a *App) guideDir(gameID string) string {
	return filepath.Join(a.dataDir, "guides", filepath.Base(gameID))
}

var textExts = map[string]bool{".txt": true, ".md": true, ".text": true, ".log": true}

// classifyGuide decides whether an upload is an image or text file (by content, then by extension).
func classifyGuide(name string, data []byte) (kind, ext string, err error) {
	if e, good := imageExt(data); good {
		if len(data) > maxGuideImage {
			return "", "", errors.New("that image is larger than 25 MB")
		}
		return "image", e, nil
	}
	e := strings.ToLower(filepath.Ext(name))
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	if textExts[e] && !bytes.Contains(head, []byte{0}) || (e == "" && utf8.Valid(head) && !bytes.Contains(head, []byte{0})) {
		if len(data) > maxGuideText {
			return "", "", errors.New("that text file is larger than 5 MB")
		}
		if e == "" || !textExts[e] {
			e = ".txt"
		}
		return "text", e, nil
	}
	return "", "", errors.New("only images (PNG, JPEG, GIF, WebP) and text files (.txt, .md) can be attached")
}

func (a *App) addGuide(gameID, name string, data []byte) (*Guide, error) {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		name = "guide"
	}
	kind, ext, err := classifyGuide(name, data)
	if err != nil {
		return nil, err
	}
	dir := a.guideDir(gameID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	g := &Guide{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: name, Kind: kind, Size: len(data), Added: time.Now()}
	g.File = g.ID + ext
	if err := os.WriteFile(filepath.Join(dir, g.File), data, 0o644); err != nil {
		return nil, err
	}
	err = a.store.Update(func(l *Library) error {
		gm := findGame(l, gameID)
		if gm == nil {
			return errors.New("game not found")
		}
		if len(gm.Guides) >= maxGuides {
			return errors.New("too many attachments for one game")
		}
		gm.Guides = append(gm.Guides, *g)
		return nil
	})
	if err != nil {
		os.Remove(filepath.Join(dir, g.File))
		return nil, err
	}
	return g, nil
}

func (a *App) removeGuide(gameID, gid string) {
	var file string
	a.store.Update(func(l *Library) error {
		if gm := findGame(l, gameID); gm != nil {
			for i, g := range gm.Guides {
				if g.ID == gid {
					file = g.File
					gm.Guides = append(gm.Guides[:i], gm.Guides[i+1:]...)
					break
				}
			}
		}
		return nil
	})
	if file != "" {
		os.Remove(filepath.Join(a.guideDir(gameID), filepath.Base(file)))
		os.Remove(filepath.Join(a.guideDir(gameID), filepath.Base(file)+".marks.json"))
	}
}

func (a *App) guideText(gameID, gid string) (string, error) {
	var file string
	a.store.View(func(l *Library) {
		if gm := findGame(l, gameID); gm != nil {
			for _, g := range gm.Guides {
				if g.ID == gid && g.Kind == "text" {
					file = g.File
				}
			}
		}
	})
	if file == "" {
		return "", errors.New("attachment not found")
	}
	b, err := os.ReadFile(filepath.Join(a.guideDir(gameID), filepath.Base(file)))
	if err != nil {
		return "", errors.New("the file is missing from the data folder")
	}
	return decodeText(b), nil
}

// Marks: the brush strokes a person draws over a guide image. They live in a small JSON file next to the image
// (the image itself is never changed) in coordinates relative to the image size, so they survive zoom and resize.
const maxMarksBytes = 4 << 20

func (a *App) marksFile(gameID, gid string) string {
	var file string
	a.store.View(func(l *Library) {
		if gm := findGame(l, gameID); gm != nil {
			for _, x := range gm.Guides {
				if x.ID == gid && x.Kind == "image" {
					file = x.File
				}
			}
		}
	})
	if file == "" {
		return ""
	}
	return filepath.Join(a.guideDir(gameID), filepath.Base(file)+".marks.json")
}

func (a *App) readMarks(gameID, gid string) ([]byte, error) {
	p := a.marksFile(gameID, gid)
	if p == "" {
		return nil, errors.New("image not found")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return []byte(`{"strokes":[]}`), nil
	}
	return b, nil
}

func (a *App) writeMarks(gameID, gid string, data []byte) error {
	p := a.marksFile(gameID, gid)
	if p == "" {
		return errors.New("image not found")
	}
	var m struct {
		Strokes []json.RawMessage `json:"strokes"`
	}
	if len(data) > maxMarksBytes || json.Unmarshal(data, &m) != nil {
		return errors.New("invalid marks")
	}
	if len(m.Strokes) == 0 {
		os.Remove(p)
		return nil
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
