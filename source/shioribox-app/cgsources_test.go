package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const ehListFixture = `<table class="itg gltc"><tr><th>x</th></tr>
<tr><td class="gl1c glcat"><div class="cs ct5">Game CG</div></td>
<td class="gl2c"><div class="glthumb"><div><img style="height:200px" alt="Clannad CG" title="Clannad CG" data-src="https://ehgt.org/ab/cd/abcd-1-200-300-jpg_250.jpg" src="data:image/gif;base64,R0"></div><div>34 pages</div></div></td>
<td class="gl3c glname"><a href="https://e-hentai.org/g/1234567/abcdef0123/"><div class="glink">[Key] Clannad &amp; More CG Set</div></a></td></tr>
<tr><td class="gl3c glname"><a href="https://e-hentai.org/g/7654321/0123abcdef/"><div class="glink">Fate CG</div></a></td><td>120 pages</td></tr>
</table><a id="dnext" href="https://e-hentai.org/?page=1">&gt;</a>`

func TestEHParseList(t *testing.T) {
	items, more := ehParseList(ehListFixture)
	if !more || len(items) != 2 {
		t.Fatalf("more=%v items=%+v", more, items)
	}
	if items[0].Ref != "1234567/abcdef0123" || items[0].Title != "[Key] Clannad & More CG Set" || items[0].Pages != 34 {
		t.Fatalf("%+v", items[0])
	}
	if !strings.Contains(items[0].Thumb, "ehgt.org") || items[1].Pages != 120 || items[1].Title != "Fate CG" {
		t.Fatalf("%+v", items)
	}
}

const ehGalleryFixture = `<h1 id="gn">Clannad CG Set</h1><table><tr><td class="gdt1">Length:</td><td class="gdt2">3 pages</td></tr></table>
<div id="gdt"><div class="gdtm"><div><a href="https://e-hentai.org/s/aaaaaaaaaa/1234567-1"><img alt="1"></a></div></div>
<div class="gdtm"><a href="https://e-hentai.org/s/bbbbbbbbbb/1234567-2"></a></div>
<a href="https://e-hentai.org/s/bbbbbbbbbb/1234567-2"></a></div>`

func TestEHParseGallery(t *testing.T) {
	title, count, shots := ehParseGallery(ehGalleryFixture)
	if title != "Clannad CG Set" || count != 3 || len(shots) != 2 {
		t.Fatalf("%q %d %+v", title, count, shots)
	}
	if shots[1].N != 2 || shots[1].URL != "/api/cg/img?s=bbbbbbbbbb/1234567-2" {
		t.Fatalf("%+v", shots)
	}
}

func TestEHEndToEndMock(t *testing.T) {
	var gotCookie, gotQuery, gotGalleryQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		switch {
		case r.URL.Path == "/":
			gotQuery = r.URL.RawQuery
			w.Write([]byte(ehListFixture))
		case strings.HasPrefix(r.URL.Path, "/g/"):
			gotGalleryQuery = r.URL.RawQuery
			if r.URL.Query().Get("p") == "1" {
				w.Write([]byte(`<h1 id="gn">Clannad CG Set</h1><table><tr><td>Length:</td><td>3 pages</td></tr></table><a href="/s/cccccccccc/1234567-3"></a>`))
			} else {
				w.Write([]byte(ehGalleryFixture))
			}
		case strings.HasPrefix(r.URL.Path, "/s/"):
			w.Write([]byte(`<img src="http://` + r.Host + `/full/pic.jpg" style="x" id="img">`))
		}
	}))
	defer srv.Close()
	old := ehBase
	ehBase = srv.URL
	defer func() { ehBase = old }()

	res, err := ehSearch("clannad", 0)
	if err != nil || len(res.Items) != 2 || res.Next != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(gotQuery, "f_cats=1007") || !strings.Contains(gotQuery, "f_search=clannad") || gotCookie != "nw=1" {
		t.Fatalf("query=%q cookie=%q", gotQuery, gotCookie)
	}
	g, err := ehGallery("1234567/abcdef0123", 0)
	if err != nil || len(g.Images) != 2 || g.Title != "Clannad CG Set" || !g.More {
		t.Fatalf("%v %+v", err, g)
	}
	g2, err := ehGallery("1234567/abcdef0123", 1)
	if err != nil || len(g2.Images) != 1 || g2.Images[0].N != 3 || g2.More {
		t.Fatalf("page 2: %v %+v", err, g2)
	}
	if gotGalleryQuery != "p=1" {
		t.Fatalf("gallery pagination query = %q, want p=1", gotGalleryQuery)
	}
	u, err := ehResolveImage("aaaaaaaaaa/1234567-1")
	if err != nil || !strings.HasSuffix(u, "/full/pic.jpg") {
		t.Fatalf("%v %q", err, u)
	}
	if _, err := ehResolveImage("../../etc/passwd"); err == nil {
		t.Fatal("bad image id accepted")
	}
	if _, err := ehGallery("1/../../x", 0); err == nil {
		t.Fatal("bad gallery ref accepted")
	}
}

func TestImageProxyAllowList(t *testing.T) {
	for _, bad := range []string{
		"http://127.0.0.1/x.jpg", "https://evil.example/x.jpg", "http://localhost/x", "https://ehgt.org.evil.com/x", "file:///etc/passwd", "http://192.168.1.5/x",
	} {
		if _, _, err := cgImageRequest(map[string][]string{"t": {bad}}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, _, err := cgImageRequest(map[string][]string{"t": {"https://ehgt.org/a/b.jpg"}}); err != nil {
		t.Error(err)
	}
	if _, ref, err := cgImageRequest(map[string][]string{"t": {"https://tn.gold-usergeneratedcontent.net/webpbigtn/a.webp"}}); err != nil || ref != "https://hitomi.la/" {
		t.Error("Hitomi preview rejected or sent without its Referer", err, ref)
	}
	if _, err := cgPublicURL("http://10.0.0.1/a"); err == nil {
		t.Error("private IP accepted")
	}
	b, _ := json.Marshal(CGSearchResult{})
	_ = b
}
