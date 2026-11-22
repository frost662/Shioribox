package main

import (
	"os"
	"path/filepath"
)

// The embedded browser (WebView2) keeps an HTTP/image/shader cache in data\webview. Browsing Discover
// pulls many remote cover images and screenshots into it, so it can grow by hundreds of MB. None of it
// is needed between runs, so it is emptied at start-up and capped while the app is open.

const webCacheArgs = "--disk-cache-size=26214400 --media-cache-size=1 --disable-background-networking --disable-component-update"

var webCacheDirs = []string{
	filepath.Join("EBWebView", "Default", "Cache"),
	filepath.Join("EBWebView", "Default", "Code Cache"),
	filepath.Join("EBWebView", "Default", "GPUCache"),
	filepath.Join("EBWebView", "Default", "DawnCache"),
	filepath.Join("EBWebView", "Default", "Service Worker"),
	filepath.Join("EBWebView", "GrShaderCache"),
	filepath.Join("EBWebView", "ShaderCache"),
	filepath.Join("EBWebView", "GraphiteDawnCache"),
}

// clearWebCache deletes the cache folders under webviewDir and returns how many it removed.
func clearWebCache(webviewDir string) int {
	n := 0
	for _, d := range webCacheDirs {
		p := filepath.Join(webviewDir, d)
		if _, err := os.Stat(p); err == nil && os.RemoveAll(p) == nil {
			n++
		}
	}
	return n
}
