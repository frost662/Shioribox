package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHentaiEngineParsing(t *testing.T) {
	s := &heSite{"hentaiera", "HentaiEra", "https://hentaiera.com"}
	list := `<div class="thumb"><div class="inner_thumb"><a href="/gallery/101/"><img class="lazy" data-src="//cdn.hentaiera.com/001/101/cover.jpg" src="data:image/gif;base64,AAA" alt="Alt Title"></a>
	<div class="caption"><a href="/gallery/101/">Clannad CG &amp; More</a></div></div><span>45 pages</span></div>
	<div class="thumb"><a href="/gallery/202/"><img src="/t/202.jpg" alt="Other"></a><h2 class="gallery_title"><a href="/gallery/202/">Fate CG</a></h2></div>
	<a href="/search/?key=clannad&page=2">2</a>`
	items, more := heParseList(s, list, 0)
	if len(items) != 2 || items[0].Ref != "101" || items[0].Title != "Clannad CG & More" || items[0].Pages != 45 || !strings.Contains(items[0].Thumb, "cdn.hentaiera.com") || items[1].Title != "Fate CG" || !more {
		t.Fatalf("%+v %v", items, more)
	}
	if !cgAllowedThumbHost("cdn.hentaiera.com") || cgAllowedThumbHost("evil.example") {
		t.Fatal("host allow-list")
	}
	gal := `<h1>Clannad CG</h1><div>Pages: 3</div>
	<div class="gthumb"><a href="/view/101/1/"><img data-src="//m1.hentaiera.com/002/101/1t.jpg"></a></div>
	<div class="gthumb"><a href="/view/101/2/"><img data-src="//m1.hentaiera.com/002/101/2t.png"></a></div>
	<div class="gthumb"><a href="/view/101/3/"><img data-src="//m1.hentaiera.com/002/101/3t.jpg"></a></div>`
	g := heParseGallery(s, "101", gal)
	if g.Title != "Clannad CG" || len(g.Images) != 3 || g.Count != 3 || !strings.Contains(g.Images[1].URL, "%2F2.png") || !strings.Contains(g.Images[1].Thumb, "2t.png") {
		t.Fatalf("%+v", g)
	}
}

func TestHentaiSearchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/" && r.URL.Query().Get("key") == "clannad" {
			w.Write([]byte(`<div><a href="/gallery/9/"><img src="/x.jpg"></a><h2 class="gallery_title">Clannad</h2></div>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	s := &heSite{"hentaiera", "HentaiEra", srv.URL}
	res, err := heSearch(s, "clannad", 0)
	if err != nil || len(res.Items) != 1 || res.Items[0].Title != "Clannad" || res.Next != -1 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestHentaiFillsMissingPreviews(t *testing.T) {
	s := &heSite{"hentaienvy", "HentaiEnvy", "https://hentaienvy.com"}
	page := `<h1>Big</h1><div>Pages: 20</div>` +
		`<img data-src="//m1.hentaienvy.com/001/55/1t.jpg"><img data-src="//m1.hentaienvy.com/001/55/2t.jpg">`
	g := heParseGallery(s, "55", page)
	if len(g.Images) != 20 || g.Count != 20 || g.PerPage != 20 || g.Images[19].N != 20 {
		t.Fatalf("%d %+v", len(g.Images), g.Images[:3])
	}
	last := g.Images[19]
	if !strings.Contains(last.URL, "%2F20.jpg") || !strings.Contains(last.Thumb, "%2F20t.jpg") || !strings.Contains(last.URL, "&e=") {
		t.Fatalf("%+v", last)
	}
	urls, _, err := cgImageRequest(map[string][]string{"t": {"https://m1.hentaienvy.com/001/55/20.jpg"}, "e": {"jpg,png,webp"}})
	if err != nil || len(urls) != 3 || !strings.HasSuffix(urls[1], "/20.png") {
		t.Fatalf("%v %v", err, urls)
	}
}

func TestImageProxyRetries(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			http.Error(w, "busy", 503)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	u := "http://cdn.hentaiera.com"
	rec := httptest.NewRecorder()
	cgServeImage(rec, httptest.NewRequest("GET", "/api/cg/img?t="+url.QueryEscape(u+"/a.png"), nil))
	if rec.Code != 200 || n != 3 {
		t.Fatalf("code %d after %d tries", rec.Code, n)
	}
}

// rewriteTransport sends every request to the test server, whatever host it names.
type rewriteTransport struct{ addr string }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	orig := req
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = "http", r.addr
	resp, err := http.DefaultTransport.RoundTrip(req)
	if resp != nil {
		resp.Request = orig // callers see the address they asked for
	}
	return resp, err
}

func TestImageProxyViewPageAndOctetStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/view/9/2/":
			w.Write([]byte(`<img id="gimg" data-src="//cdn.hentaiera.com/real/2.jpg">`))
		case "/real/2.jpg":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("jpegdata"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	full := heFull(heSites["hentaiera"], "9", 2, "https://cdn.hentaiera.com/guess/2.jpg", "https://cdn.hentaiera.com/guess/2t.jpg", false)
	rec := httptest.NewRecorder()
	cgServeImage(rec, httptest.NewRequest("GET", full, nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.String() != "jpegdata" {
		t.Fatalf("%d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
