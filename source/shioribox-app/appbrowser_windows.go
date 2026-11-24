//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	abMu  sync.Mutex
	abCmd *exec.Cmd // the browser window's process, or nil
	abDir string    // data folder (set by runUI)
)

// openAppBrowser shows a page in a separate ShioriBox browser window. The window runs in its own process (this same
// program started with -browse), so closing it can never disturb the main window. A new call replaces the old window.
func openAppBrowser(url, title string) {
	exe, err := os.Executable()
	if err != nil || abDir == "" {
		openBrowser(url)
		return
	}
	abMu.Lock()
	defer abMu.Unlock()
	if abCmd != nil && abCmd.Process != nil {
		abCmd.Process.Kill()
	}
	cmd := exec.Command(exe, "-browse", url, "-title", title, "-parent", strconv.Itoa(os.Getpid()), "-data", abDir)
	if err := cmd.Start(); err != nil {
		abCmd = nil
		openBrowser(url)
		return
	}
	abCmd = cmd
	go func() {
		cmd.Wait()
		abMu.Lock()
		if abCmd == cmd {
			abCmd = nil
		}
		abMu.Unlock()
	}()
}

// closeAppBrowser closes the browser window (used by the privacy hotkey).
func closeAppBrowser() {
	abMu.Lock()
	defer abMu.Unlock()
	if abCmd != nil && abCmd.Process != nil {
		abCmd.Process.Kill()
	}
}

// runBrowseWindow is the browser window's own process: one WebView2 window showing url until it is closed or the main
// ShioriBox process ends.
func runBrowseWindow(url, title, dataDir string, parent int) {
	if !webview2Installed() {
		openBrowser(url)
		return
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(dataDir, "webview-browser"),
		WindowOptions: webview2.WindowOptions{
			Title: title, Width: 1200, Height: 800, Center: true, IconId: 1,
		},
	})
	if w == nil {
		openBrowser(url)
		return
	}
	defer w.Destroy()
	w.SetSize(640, 420, webview2.HintMin)
	w.Init(`window.open=function(){return null};`) // no pop-up windows or ads
	if parent > 0 {
		go func() {
			h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parent))
			if err != nil {
				return
			}
			windows.WaitForSingleObject(h, windows.INFINITE)
			w.Destroy() // the main app ended: close with it
		}()
	}
	w.Navigate(url)
	w.Run()
}
