package main

import (
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"
)

func TestFindMedia(t *testing.T) {
	page := `<video><source src="/v/ep1.mp4" type="video/mp4"></video>
	<script>var p = {"file":"https:\/\/cdn.example.com\/hls\/master.m3u8?t=1&e=2"}; jw.setup({sources:[{file:"//cdn2.example.com/a.webm"}]});</script>
	<iframe data-src="https://player.example.com/embed/123"></iframe><img src="x.png">`
	media, frames := findMedia("https://anihentai.com/videos/x/", page)
	want := map[string]bool{"https://anihentai.com/v/ep1.mp4": true, "https://cdn.example.com/hls/master.m3u8?t=1&e=2": true, "https://cdn2.example.com/a.webm": true}
	if len(media) != 3 {
		t.Fatalf("%v", media)
	}
	for _, m := range media {
		if !want[m] {
			t.Fatalf("unexpected %s in %v", m, media)
		}
	}
	if len(frames) != 1 || frames[0] != "https://player.example.com/embed/123" {
		t.Fatalf("%v", frames)
	}
	if mediaType("https://a/b.m3u8?x=1") != "hls" || mediaType("https://a/b.mp4") != "mp4" {
		t.Fatal("mediaType")
	}
}

func TestRewriteM3U8(t *testing.T) {
	base, _ := neturl.Parse("https://cdn.example.com/hls/master.m3u8")
	out := rewriteM3U8("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXTINF:4,\nseg1.ts\n/abs/seg2.ts\n", base, "https://player.example.com/")
	if strings.Contains(out, "seg1.ts") || strings.Contains(out, "key.bin") || strings.Count(out, "/api/anime/stream?k=") != 3 || !strings.HasPrefix(out, "#EXTM3U") {
		t.Fatal(out)
	}
	k := out[strings.Index(out, "?k=")+3:]
	k = k[:strings.IndexAny(k, "\"\n")]
	e, ok := streamLookup(k)
	if !ok || e.url != "https://cdn.example.com/hls/key.bin" || e.referer != "https://player.example.com/" {
		t.Fatalf("%+v %v", e, ok)
	}
}

func TestAnimeResolveAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videos/x/":
			w.Write([]byte(`<iframe src="https://player.example.com/embed/1"></iframe>`))
		case "/embed/1":
			if r.Header.Get("Referer") != "https://anihentai.com/videos/x/" {
				http.Error(w, "referer", 403)
				return
			}
			w.Write([]byte(`sources:[{file:"https://media.example.com/ep.mp4"},{file:"https://media.example.com/list.m3u8"}]`))
		case "/ep.mp4":
			if r.Header.Get("Referer") != "https://player.example.com/embed/1" {
				http.Error(w, "referer", 403)
				return
			}
			http.ServeContent(w, r, "ep.mp4", time.Time{}, strings.NewReader("0123456789"))
		case "/list.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXTINF:4,\nseg.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old, oldS := cgHTTP, streamHTTP
	cl := &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	cgHTTP, streamHTTP = cl, cl
	defer func() { cgHTTP, streamHTTP = old, oldS }()

	src, embeds, err := animeResolve(animeSiteByID("anihentai"), "https://anihentai.com/videos/x/")
	if err != nil || len(src) != 2 || len(embeds) != 1 || src[0].Type != "mp4" || src[1].Type != "hls" {
		t.Fatalf("%v %+v %v", err, src, embeds)
	}
	// Range request through the proxy: seeking works and the Referer is added
	req := httptest.NewRequest("GET", src[0].URL, nil)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	animeStream(rec, req)
	if rec.Code != 206 || rec.Body.String() != "2345" {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	// playlist: segments come back as proxy links
	rec = httptest.NewRecorder()
	animeStream(rec, httptest.NewRequest("GET", src[1].URL, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/api/anime/stream?k=") || strings.Contains(rec.Body.String(), "seg.ts") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	// unknown token
	rec = httptest.NewRecorder()
	animeStream(rec, httptest.NewRequest("GET", "/api/anime/stream?k=nope", nil))
	if rec.Code != 410 {
		t.Fatal(rec.Code)
	}
	if _, _, err := animeResolve(animeSiteByID("anihentai"), "https://evil.example/x"); err == nil {
		t.Fatal("foreign page accepted")
	}
}

func TestAdsAreSkippedAndEpisodeComesFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videos/x/":
			w.Write([]byte(`<source src="https://cdn.example.com/clip.mp4"><script>file:"https://cdn.example.com/show.m3u8"; file:"https://ads.example.com/vast/pre.mp4"; file:"https://cdn.example.com/trailer.mp4"</script>`))
		case "/clip.mp4":
			w.Header().Set("Content-Range", "bytes 0-0/900000")
			w.WriteHeader(206)
			w.Write([]byte("x"))
		case "/trailer.mp4":
			w.Header().Set("Content-Range", "bytes 0-0/400000000")
			w.WriteHeader(206)
			w.Write([]byte("x"))
		case "/show.m3u8":
			w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv.m3u8\n"))
		case "/v.m3u8":
			w.Write([]byte("#EXTM3U\n#EXTINF:600.0,\na.ts\n#EXTINF:600.0,\nb.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old, oldS := cgHTTP, streamHTTP
	cl := &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	cgHTTP, streamHTTP = cl, cl
	defer func() { cgHTTP, streamHTTP = old, oldS }()
	src, _, err := animeResolve(animeSiteByID("anihentai"), "https://anihentai.com/videos/x/")
	// ad host dropped, 0.9 MB clip dropped as an advert; the 20-minute stream leads, the 400 MB file follows
	if err != nil || len(src) != 2 || src[0].Type != "hls" || !strings.HasPrefix(src[0].Label, "20 min") || src[1].Type != "mp4" || !strings.HasPrefix(src[1].Label, "381 MB") {
		t.Fatalf("%v %+v", err, src)
	}
}
