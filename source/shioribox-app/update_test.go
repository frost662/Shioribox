package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionCompare(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{{"1.20.0", "1.19.0", true}, {"v1.19.1", "1.19.0", true}, {"1.19.0", "1.19.0", false}, {"1.9.0", "1.19.0", false}, {"2", "1.99.9", true}, {"1.19", "1.19.0", false}, {"1.19.0-beta", "1.19.0", false}} {
		if versionNewer(c.a, c.b) != c.newer {
			t.Errorf("%s vs %s", c.a, c.b)
		}
	}
	names := []string{"source.zip", "ShioriBox-Setup-1.20.0.exe", "ShioriBox-Update-1.20.0.exe"}
	if pickInstaller(names) != 2 || pickInstaller([]string{"a.zip"}) != -1 || pickInstaller(names[:2]) != -1 || pickInstaller([]string{"ShioriBox_Portable.zip", "ShioriBox-Setup-1.exe"}) != -1 {
		t.Fatal("pickInstaller")
	}
}

func TestUpdateCheckGitHubAndManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/me/shiori/releases/latest"):
			w.Write([]byte(`{"tag_name":"v99.0.0","body":"New things","html_url":"https://github.com/me/shiori/releases/tag/v99.0.0","assets":[{"name":"ShioriBox-Setup-99.0.0.exe","browser_download_url":"https://github.com/me/shiori/releases/download/v99.0.0/ShioriBox-Setup-99.0.0.exe","size":5},{"name":"ShioriBox-Update-99.0.0.exe","browser_download_url":"https://github.com/me/shiori/releases/download/v99.0.0/ShioriBox-Update-99.0.0.exe","size":7}]}`))
		case r.URL.Path == "/latest.json":
			w.Write([]byte(`{"version":"0.0.1","notes":"old","page":"https://example.com/p"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	d := t.TempDir()
	st, _ := newStore(d)
	a := &App{store: st, dataDir: d}

	oldSrc := updateSource
	defer func() { updateSource = oldSrc }()
	if oldSrc != "frost662/Shioribox" {
		t.Fatal("the update address must be frost662/Shioribox")
	}
	updateSource = ""
	if info := a.updateCheck(); info.Error == "" || info.Available {
		t.Fatalf("no source should be an error: %+v", info)
	}
	updateSource = "me/shiori"
	info := a.updateCheck()
	if info.Error != "" || !info.Available || info.Latest != "99.0.0" || info.Name != "ShioriBox-Update-99.0.0.exe" || info.Size != 7 || info.Notes != "New things" {
		t.Fatalf("%+v", info)
	}
	st.Update(func(l *Library) error { l.Settings.UpdateSkip = "99.0.0"; return nil })
	if s := a.updateStatus()["info"].(UpdateInfo); !s.Skipped {
		t.Fatal("skip not honoured")
	}
	updateSource = "https://example.com/latest.json"
	if info := a.updateCheck(); info.Error != "" || info.Available || info.Latest != "0.0.1" {
		t.Fatalf("%+v", info)
	}
	updateSource = "me/missing"
	if info := a.updateCheck(); info.Error == "" || !strings.Contains(info.Error, "404") {
		t.Fatalf("%+v", info)
	}
	// downloads only from where releases live
	if _, err := a.downloadInstaller(UpdateInfo{Download: "https://evil.example/x.exe"}, "me/shiori"); err == nil {
		t.Fatal("foreign host accepted")
	}
	if _, err := a.downloadInstaller(UpdateInfo{Download: "http://github.com/x.exe"}, "me/shiori"); err == nil {
		t.Fatal("plain http accepted")
	}
	if _, err := a.downloadInstaller(UpdateInfo{Download: "https://github.com/me/x/releases/download/v1/readme.txt"}, "me/shiori"); err == nil {
		t.Fatal("non-exe accepted")
	}
}
