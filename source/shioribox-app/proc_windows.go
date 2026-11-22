//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// foregroundIsGame reports whether the window on top belongs to the game: its program is the game's exe
// or lives in the game's folder (launchers, helper processes). Anything else — another app, the desktop,
// the lock screen, ShioriBox itself — counts as "not playing".
func foregroundIsGame(exe string) bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}
	p, _ := imageInfo(pid)
	if p == "" {
		return false
	}
	np := lc(p)
	return np == lc(exe) || strings.HasPrefix(np, lc(filepath.Dir(exe))+`\`)
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func listPIDs() []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []uint32
	if windows.Process32First(snap, &pe) != nil {
		return nil
	}
	for {
		out = append(out, pe.ProcessID)
		if windows.Process32Next(snap, &pe) != nil {
			break
		}
	}
	return out
}

func imageInfo(pid uint32) (string, time.Time) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", time.Time{}
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return "", time.Time{}
	}
	var c, e, k, u windows.Filetime
	var created time.Time
	if windows.GetProcessTimes(h, &c, &e, &k, &u) == nil {
		created = time.Unix(0, c.Nanoseconds())
	}
	return windows.UTF16ToString(buf[:n]), created
}

func lc(p string) string { return strings.ToLower(filepath.Clean(p)) }

// findGameProcess finds the process that a launcher started for exe: first an exact match of
// the program, otherwise any program from the same folder (for games that start through a
// small launcher). Only processes created after notBefore count.
func findGameProcess(exe string, notBefore time.Time) uint32 {
	want := lc(exe)
	dir := lc(filepath.Dir(exe)) + `\`
	var best uint32
	var bestExact bool
	var bestTime time.Time
	for _, pid := range listPIDs() {
		p, created := imageInfo(pid)
		if p == "" || created.Before(notBefore.Add(-3*time.Second)) {
			continue
		}
		np := lc(p)
		exact := np == want
		if !exact && !strings.HasPrefix(np, dir) {
			continue
		}
		if best == 0 || (exact && !bestExact) || (exact == bestExact && created.After(bestTime)) {
			best, bestExact, bestTime = pid, exact, created
		}
	}
	return best
}

func pidAlive(pid uint32) bool {
	for _, p := range listPIDs() {
		if p == pid {
			return true
		}
	}
	return false
}

func waitPID(pid uint32) {
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid); err == nil {
		windows.WaitForSingleObject(h, windows.INFINITE)
		windows.CloseHandle(h)
		return
	}
	for pidAlive(pid) { // e.g. an elevated game we may not open: poll instead
		time.Sleep(2 * time.Second)
	}
}

func killPID(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

func openControlPanel(applet string) error {
	return exec.Command("control.exe", applet).Start()
}

func openFolder(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}
