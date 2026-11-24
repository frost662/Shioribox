//go:build !windows

package main

// The in-app browser window exists on Windows only; elsewhere the default browser is used.
func openAppBrowser(url, title string)                       { openBrowser(url) }
func closeAppBrowser()                                       {}
func runBrowseWindow(url, title, dataDir string, parent int) { openBrowser(url) }
