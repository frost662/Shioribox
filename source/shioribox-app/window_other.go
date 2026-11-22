//go:build !windows

package main

// runUI shows the app. Off Windows (development only) we use a browser window.
func runUI(app *App, url, dataDir string) { runBrowserUI(app, url, dataDir) }
