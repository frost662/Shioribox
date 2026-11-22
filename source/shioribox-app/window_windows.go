//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

// webview2Installed reports whether the Microsoft Edge WebView2 runtime is present
// (it ships with Windows 11 and current Windows 10).
func webview2Installed() bool {
	const guid = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, c := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{registry.LOCAL_MACHINE, guid},
		{registry.CURRENT_USER, guid},
	} {
		k, err := registry.OpenKey(c.root, c.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("pv")
		k.Close()
		if err == nil && v != "" && v != "0.0.0.0" {
			return true
		}
	}
	return false
}

// runUI opens ShioriBox in its own native window (embedded WebView2) and blocks
// until the window is closed.
func runUI(app *App, url, dataDir string) {
	if !webview2Installed() {
		logf("WebView2 runtime not found; using fallback window")
		runBrowserUI(app, url, dataDir)
		return
	}
	wvDir := filepath.Join(dataDir, "webview")
	clearWebCache(wvDir)
	if os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", webCacheArgs)
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  wvDir,
		WindowOptions: webview2.WindowOptions{
			Title: "ShioriBox", Width: 1360, Height: 860, Center: true, IconId: 1,
		},
	})
	if w == nil {
		logf("could not create native window; using fallback window")
		runBrowserUI(app, url, dataDir)
		return
	}
	defer w.Destroy()
	w.SetSize(960, 600, webview2.HintMin)
	startPanic(app, w)
	w.Navigate(url)
	w.Run()
}
