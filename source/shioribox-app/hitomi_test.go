package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const ggFixture = `'use strict';
gg = { m: function(g) { var o = 1; switch (g) {
case 1234:
case 99:
o = 0; break;
case 5: o = 0; break;
} return o; },
s: function(h) { var m = /(..)(.)$/.exec(h); return parseInt(m[2]+m[1], 16).toString(10); },
b: '1731000000/'
};`

func nozomi(ids ...int) []byte {
	b := make([]byte, 4*len(ids))
	for i, id := range ids {
		binary.BigEndian.PutUint32(b[i*4:], uint32(id))
	}
	return b
}

func TestParseGGAndURLs(t *testing.T) {
	gg, err := parseGG(ggFixture)
	if err != nil {
		t.Fatal(err)
	}
	if gg.b != "1731000000/" || gg.def != 1 || gg.m(1234) != 0 || gg.m(99) != 0 || gg.m(5) != 0 || gg.m(7) != 1 {
		t.Fatalf("%+v", gg)
	}
	hash := strings.Repeat("0", 61) + "b2c" // last three "b2c" -> "c"+"b2" = 0xcb2 = 3250
	if n, _ := hitomiSlot(hash); n != 0xcb2 {
		t.Fatalf("slot %d", n)
	}
	urls := hitomiImageURLs(gg, "gold-usergeneratedcontent.net", hash, "01.png", true)
	if len(urls) != 2 ||
		urls[0] != "https://w2.gold-usergeneratedcontent.net/1731000000/3250/"+hash+".webp" ||
		urls[1] != "https://ba.gold-usergeneratedcontent.net/images/1731000000/3250/"+hash+".png" {
		t.Fatalf("%v", urls)
	}
	if got := hitomiImageURLs(gg, "x.net", hash, "a.jpg", false); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if _, err := parseGG("nothing here"); err == nil {
		t.Fatal("garbage gg.js accepted")
	}
	if got := hitomiThumbURL("x.net", "00000000000000000000000000000000000000000000000000000000000abc", "webpbigtn"); got != "https://tn.x.net/webpbigtn/c/ab/00000000000000000000000000000000000000000000000000000000000abc.webp" {
		t.Fatalf("%s", got)
	}
}

func TestNozomiMatchingAndSeriesNames(t *testing.T) {
	ids := parseNozomi(nozomi(3000000, 42, 7))
	if len(ids) != 3 || ids[0] != 3000000 || ids[2] != 7 {
		t.Fatalf("%v", ids)
	}
	for _, c := range []struct {
		q, t string
		want bool
	}{
		{"Clannad", "Clannad CG Collection", true},
		{"clannad after story", "CLANNAD ~After Story~ CG", true},
		{"fate stay", "Fate/hollow ataraxia", false},
		{"車輪の国", "車輪の国、向日葵の少女 CG", true},
		{"", "anything", true},
	} {
		if got := titleMatches(c.q, c.t); got != c.want {
			t.Errorf("titleMatches(%q,%q)=%v want %v", c.q, c.t, got, c.want)
		}
	}
	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	n := hitomiSeriesNames("Steins;Gate", "CLANNAD ~After Story~", "Fate/stay night")
	for _, want := range []string{"steins;gate", "steins gate", "steinsgate", "clannad ~after story~", "clannad", "fate/stay night", "fate stay night"} {
		if !has(n, want) {
			t.Errorf("missing series spelling %q in %v", want, n)
		}
	}
	if len(hitomiSeriesNames("")) != 0 {
		t.Error("empty title produced names")
	}
}

type hitomiMock struct {
	srv    *httptest.Server
	blocks atomic.Int32
}

// newHitomiMock serves a tiny Hitomi: three Game CG galleries (333 newest), a "clannad" series that also lists a
// non-CG gallery, and index files under /n/ only (so the prefix fallback is exercised).
func newHitomiMock(t *testing.T) *hitomiMock {
	m := &hitomiMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://hitomi.la/" {
			http.Error(w, "no referer", 403)
			return
		}
		switch r.URL.Path {
		case "/n/type/gamecg-all.nozomi":
			w.Write(nozomi(333, 222, 111))
		case "/n/series/clannad-all.nozomi":
			w.Write(nozomi(111, 999))
		case "/gg.js":
			w.Write([]byte(ggFixture))
		case "/galleryblock/111.html":
			m.blocks.Add(1)
			w.Write([]byte(`<div class="gallery-content"><div class="dj-img1"><picture><img data-src="//tn.gold-usergeneratedcontent.net/webpbigtn/a/bc/hash.webp"></picture></div><h1 class="lillie"><a href="/gamecg/clannad-111.html">Clannad CG</a></h1><div>12 pages</div></div>`))
		case "/galleryblock/222.html":
			m.blocks.Add(1)
			w.Write([]byte(`<h1 class="lillie"><a href="/gamecg/fate-222.html">Fate &amp; Stay</a></h1>`))
		case "/galleryblock/333.html":
			m.blocks.Add(1)
			w.Write([]byte(`<h1 class="lillie"><a href="/gamecg/umineko-333.html">Umineko CG</a></h1>`))
		case "/galleries/111.js":
			w.Write([]byte(`var galleryinfo = {"title":"Clannad CG","files":[{"hash":"` + strings.Repeat("0", 61) + `b2c","name":"01.png","haswebp":1,"width":1,"height":1}]};`))
		default:
			http.NotFound(w, r)
		}
	}))
	return m
}

func resetHitomi(t *testing.T, srvURL string) string {
	old := hitomiHosts
	hitomiHosts = []struct{ ltn, domain string }{{srvURL, "example.test"}}
	cache := filepath.Join(t.TempDir(), "cache", "hitomi-index.json")
	hitomiMu.Lock()
	hitomiIDs, hitomiIDSet, hitomiGGc, hitomiPair = nil, nil, nil, [2]int{-1, -1}
	hitomiGals, hitomiIdx, hitomiIdxLoaded, hitomiIdxDirty, hitomiCacheFile = map[int]*hitomiGallery{}, map[int]hitomiEntry{}, false, 0, cache
	hitomiMu.Unlock()
	t.Cleanup(func() { hitomiHosts = old })
	return cache
}

func TestHitomiEndToEndMock(t *testing.T) {
	m := newHitomiMock(t)
	defer m.srv.Close()
	cache := resetHitomi(t, m.srv.URL)

	// 1. series hit: found without scanning the whole list; the non-CG gallery 999 is dropped
	res, err := hitomiSearch("Clannad", "", 0, false)
	if err != nil || len(res.Items) != 1 || res.Items[0].Ref != "111" || res.Items[0].Pages != 12 || res.Total != 3 {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(res.Items[0].Thumb, "webpbigtn") {
		t.Fatalf("thumb %q", res.Items[0].Thumb)
	}

	// 2. no series: found by scanning titles ("&amp;" is decoded); results are remembered on disk
	res, err = hitomiSearch("fate stay", "", 0, false)
	if err != nil || len(res.Items) != 1 || res.Items[0].Ref != "222" || res.Items[0].Title != "Fate & Stay" || res.Next != -1 {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("title index not saved: %v", err)
	}

	// 3. a fresh process (empty memory, same cache file) answers from the saved index without fetching any block again
	before := m.blocks.Load()
	hitomiMu.Lock()
	hitomiIdx, hitomiIdxLoaded = map[int]hitomiEntry{}, false
	hitomiMu.Unlock()
	res, err = hitomiSearch("umineko", "", 0, false)
	if err != nil || len(res.Items) != 1 || res.Items[0].Ref != "333" {
		t.Fatalf("%v %+v", err, res)
	}
	res, err = hitomiSearch("fate", "", 0, false)
	if err != nil || len(res.Items) != 1 || m.blocks.Load() != before { // nothing is fetched twice
		t.Fatalf("%v %+v blocks %d->%d", err, res, before, m.blocks.Load())
	}

	// 4. alternative (original) title also matches
	res, _ = hitomiSearch("nothing like it", "umineko", 0, false)
	if len(res.Items) != 1 || res.Items[0].Ref != "333" {
		t.Fatalf("%+v", res)
	}

	// 5. empty query lists newest first
	all, err := hitomiSearch("", "", 0, false)
	if err != nil || len(all.Items) != 3 || all.Items[0].Ref != "333" {
		t.Fatalf("%v %+v", err, all)
	}

	// 6. gallery + image proxy
	g, err := hitomiGallerySummary(111)
	if err != nil || g.Count != 1 || g.PerPage != 1 || g.Images[0].URL != "/api/cg/img?site=hitomi&id=111&i=0" || !strings.HasSuffix(g.Images[0].Thumb, "&th=1") {
		t.Fatalf("%v %+v", err, g)
	}
	urls, ref, err := cgImageRequest(map[string][]string{"site": {"hitomi"}, "id": {"111"}, "i": {"0"}})
	if err != nil || ref != "https://hitomi.la/" || len(urls) != 4 || !strings.HasPrefix(urls[0], "https://w2.example.test/1731000000/3250/") {
		t.Fatalf("%v %v %v", err, ref, urls)
	}
	turls, _, err := cgImageRequest(map[string][]string{"site": {"hitomi"}, "id": {"111"}, "i": {"0"}, "th": {"1"}})
	if err != nil || len(turls) != 4 || !strings.Contains(turls[0], "/webpsmalltn/") || !strings.Contains(turls[1], "/webpbigtn/") {
		t.Fatalf("%v %v", err, turls)
	}
	if _, _, err := cgImageRequest(map[string][]string{"site": {"hitomi"}, "id": {"111"}, "i": {"5"}}); err == nil {
		t.Fatal("out-of-range image accepted")
	}
}

func TestHitomiOffline(t *testing.T) {
	resetHitomi(t, "http://127.0.0.1:1")
	if _, err := hitomiSearch("clannad", "", 0, false); err == nil {
		t.Fatal("expected an error when Hitomi is unreachable")
	}
}

// buildHitomiIndex makes a two-node B-tree (root at 0 holds the smaller key, its right child sits at 464) plus the
// data file, in Hitomi's layout.
func buildHitomiIndex(words map[string][]int) (index, data []byte) {
	type ent struct {
		key []byte
		off int64
		ln  int32
	}
	var es []ent
	for w, ids := range words {
		sum := sha256.Sum256([]byte(w))
		off := int64(len(data))
		d := make([]byte, 4+4*len(ids))
		binary.BigEndian.PutUint32(d, uint32(len(ids)))
		for i, id := range ids {
			binary.BigEndian.PutUint32(d[4+4*i:], uint32(id))
		}
		data = append(data, d...)
		es = append(es, ent{sum[:4], off, int32(len(d))})
	}
	sort.Slice(es, func(i, j int) bool { return bytes.Compare(es[i].key, es[j].key) < 0 })
	node := func(e ent, sub1 int64) []byte {
		b := make([]byte, hitomiNodeSize)
		p := 0
		put32 := func(v int32) { binary.BigEndian.PutUint32(b[p:], uint32(v)); p += 4 }
		put32(1)
		put32(4)
		copy(b[p:], e.key)
		p += 4
		put32(1)
		binary.BigEndian.PutUint64(b[p:], uint64(e.off))
		p += 8
		put32(e.ln)
		p += 8 // sub 0: none
		binary.BigEndian.PutUint64(b[p:], uint64(sub1))
		return b
	}
	index = append(index, node(es[0], hitomiNodeSize)...)
	index = append(index, node(es[1], 0)...)
	return
}

func TestHitomiWordIndexMock(t *testing.T) {
	idx, data := buildHitomiIndex(map[string][]int{"clannad": {111, 999}, "cg": {111, 222, 333}})
	m := newHitomiMock(t)
	defer m.srv.Close()
	// a second server in front: serves the index with Range support, everything else goes to the base mock
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/galleriesindex/version":
			w.Write([]byte("123\n"))
		case "/galleriesindex/galleries.123.index":
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(idx))
		case "/galleriesindex/galleries.123.data":
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
		default:
			m.srv.Config.Handler.ServeHTTP(w, r)
		}
	}))
	defer front.Close()
	resetHitomi(t, front.URL)
	hitomiMu.Lock()
	hitomiIdxVer, hitomiRangeOK = "", true
	hitomiMu.Unlock()

	res, err := hitomiSearch("Clannad CG", "", 0, false)
	if err != nil || !res.Indexed || len(res.Items) != 1 || res.Items[0].Ref != "111" || res.Next != -1 {
		t.Fatalf("%v %+v", err, res)
	}
	if m.blocks.Load() > 3 {
		t.Fatalf("scanned %d blocks", m.blocks.Load())
	}
	// a title with a word that is not in the title finds nothing, and does not trigger a scan
	res, err = hitomiSearch("clannad zzzzunknown", "", 0, false)
	if err != nil || !res.Indexed || len(res.Items) != 0 || res.Next != -1 {
		t.Fatalf("%v %+v", err, res)
	}
}
