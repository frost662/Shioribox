package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHStreamListInfoEmbedRandom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search":
			if r.URL.Query().Get("tags[0]") != "big-boobs" && r.URL.Query().Get("tags[0]") != "" {
				t.Errorf("tag %q", r.URL.Query().Get("tags[0]"))
			}
			w.Write([]byte(`<a href="/search?order=view-count">Most viewed</a><a href="/random">Random</a>
<a href="/hentai/sister-breeder-6"><img data-src="/img/sb6.jpg" alt=""><span>Sister Breeder - 6</span><span>4k</span><span>48fps</span></a>
<a href="/hentai/maid-cafe-1"><img src="/img/mc1.jpg"><h3>Maid Cafe - 1</h3></a>`))
		case r.URL.Path == "/random":
			http.Redirect(w, r, "/hentai/maid-cafe-2", 302)
		case r.URL.Path == "/hentai/sister-breeder-6":
			w.Write([]byte(`<head><meta property="og:image" content="/img/sb6.jpg"></head><h1>Sister Breeder - 6</h1><p>` + strings.Repeat("A synopsis sentence. ", 6) + `</p>
<a href="/search?tags%5B0%5D=big-boobs">Big Boobs</a><a href="/search?tags[0]=milf">Milf</a>
<a href="/hentai/sister-breeder-5">Ep 5</a><a href="/hentai/sister-breeder-6">Ep 6</a><a href="/hentai/other-1">Other</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	items, next, err := hstreamList("", "big-boobs", 1)
	if err != nil || len(items) != 2 || next != "2" || items[0].Site != "hstream" || items[0].Title != "Sister Breeder" || items[0].Thumb == "" || items[1].Title != "Maid Cafe" {
		t.Fatalf("%v %q %+v", err, next, items)
	}
	info, err := hstreamInfo("https://hstream.moe/hentai/sister-breeder-6")
	if err != nil || info.Title != "Sister Breeder" || len(info.Genres) != 2 || info.Genres[0].V != "big-boobs" || len(info.Episodes) != 2 || info.Episodes[0].N != 5 || info.Poster == "" || info.Site != "hstream" {
		t.Fatalf("%v %+v", err, info)
	}
	if _, err := hstreamInfo("https://evil.example/hentai/x-1"); err == nil {
		t.Fatal("foreign page accepted")
	}
	if e, err := hstreamEmbed("https://hstream.moe/hentai/sister-breeder-6"); err != nil || len(e) != 1 {
		t.Fatalf("%v %v", err, e)
	}
	if it, err := hstreamRandom(); err != nil || !strings.Contains(it.URL, "/hentai/maid-cafe-2") {
		t.Fatalf("%v %+v", err, it)
	}
}
