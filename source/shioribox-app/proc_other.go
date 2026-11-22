//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Non-Windows versions exist only so the program can be developed and tested off Windows.

func hideWindow(cmd *exec.Cmd) {}

// No window focus tracking off Windows: the game always counts as on top.
func foregroundIsGame(exe string) bool { return true }

func findGameProcess(exe string, notBefore time.Time) uint32 {
	want := filepath.Clean(exe)
	dir := filepath.Dir(want) + "/"
	ents, _ := os.ReadDir("/proc")
	var exactPID, dirPID uint32
	for _, e := range ents {
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		p, err := os.Readlink("/proc/" + e.Name() + "/exe")
		if err != nil {
			continue
		}
		p = strings.TrimSuffix(p, " (deleted)")
		if p == want {
			exactPID = uint32(n)
		} else if strings.HasPrefix(p, dir) {
			dirPID = uint32(n)
		}
	}
	if exactPID != 0 {
		return exactPID
	}
	return dirPID
}

func alive(pid uint32) bool {
	if syscall.Kill(int(pid), 0) != nil {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(int(pid)) + "/stat")
	if err != nil {
		return false
	}
	if i := strings.LastIndex(string(b), ") "); i >= 0 && i+2 < len(b) && b[i+2] == 'Z' {
		return false // zombie: already finished
	}
	return true
}

func waitPID(pid uint32) {
	for alive(pid) {
		time.Sleep(300 * time.Millisecond)
	}
}

func killPID(pid uint32) error { return syscall.Kill(int(pid), syscall.SIGKILL) }

func openControlPanel(applet string) error { return errors.New("only available on Windows") }
func openFolder(dir string) error          { return errors.New("only available on Windows") }
