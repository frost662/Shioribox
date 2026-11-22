package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const fileMarker = "shioribox:file"
const oldFileMarker = "novelshelf:file" // backups made before the rename

type BackupInfo struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
}

func (a *App) backupDir(id string) string { return filepath.Join(a.dataDir, "backups", id) }

func zipPath(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	addFile := func(full, name string) error {
		w, err := zw.Create(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		in, err := os.Open(full)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	}
	if st.IsDir() {
		err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(src, p)
			return addFile(p, rel)
		})
	} else {
		zw.SetComment(fileMarker)
		err = addFile(src, filepath.Base(src))
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}

func restoreZip(zp, target string) error {
	zr, err := zip.OpenReader(zp)
	if err != nil {
		return err
	}
	defer zr.Close()
	extract := func(f *zip.File, dest string) error {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	}
	if zr.Comment == fileMarker || zr.Comment == oldFileMarker {
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() {
				return extract(f, target)
			}
		}
		return errors.New("empty backup")
	}
	base := filepath.Clean(target)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest := filepath.Join(base, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(dest, base+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in backup: %s", f.Name)
		}
		if err := extract(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) listBackups(id string) []BackupInfo {
	out := []BackupInfo{}
	ents, err := os.ReadDir(a.backupDir(id))
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, BackupInfo{Name: e.Name(), Size: info.Size(), Time: info.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func (a *App) backupGame(id string) (string, error) {
	var savePath string
	var keep int
	a.store.View(func(l *Library) {
		if g := findGame(l, id); g != nil {
			savePath = g.SavePath
		}
		keep = l.Settings.BackupKeep
	})
	if savePath == "" {
		return "", errors.New("no save location set for this game")
	}
	if err := os.MkdirAll(a.backupDir(id), 0o755); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405") + ".zip"
	if err := zipPath(savePath, filepath.Join(a.backupDir(id), name)); err != nil {
		return "", err
	}
	if keep > 0 {
		var auto []BackupInfo
		for _, b := range a.listBackups(id) {
			if !strings.HasPrefix(b.Name, "pre-restore-") {
				auto = append(auto, b)
			}
		}
		for i := keep; i < len(auto); i++ {
			os.Remove(filepath.Join(a.backupDir(id), auto[i].Name))
		}
	}
	return name, nil
}

func safeName(n string) bool {
	return n != "" && n == filepath.Base(n) && !strings.ContainsAny(n, `/\`) && strings.HasSuffix(n, ".zip")
}
