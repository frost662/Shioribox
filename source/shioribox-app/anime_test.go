package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnimeParsing(t *testing.T) {
	s := animeSites[0]
	search := `<nav><a href="/">Home</a><a href="/genre/ecchi/">Ecchi</a></nav>
	<article><a href="/tvshows/clannad/"><img data-src="https://anihentai.com/wp-content/up/clannad.jpg" alt="Clannad poster"></a>
	<div class="title"><a href="https://anihentai.com/tvshows/clannad/">Clannad</a></div></article>
	<article><a href="/episodes/other-episode-1/" title="Other Show Episode 1">x</a></article>
	<a href="https://elsewhere.example/clannad/">Clannad ad</a>
	<a href="/episodes/clannad-episode-2/">Clannad Episode 2</a>`
	items := animeLinks(s, search)
	var got []string
	for _, it := range items {
		got = append(got, it.Title)
	}
	if len(items) != 3 || items[0].Title != "Clannad" || items[0].Episode || !strings.Contains(items[0].Thumb, "clannad.jpg") {
		t.Fatalf("%q %+v", got, items)
	}
	if !items[2].Episode || items[2].N != 2 {
		t.Fatalf("%+v", items[2])
	}
	if !cgAllowedThumbHost("anihentai.com") {
		t.Fatal("thumb host not registered")
	}
}

func TestAnimeSearchAndEpisodesMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("s") == "Clannad":
			w.Write([]byte(`<a href="/series/clannad/"><img src="/x.jpg" alt="Clannad"></a><a href="/series/clannad/">Clannad</a><a href="/series/fate/">Fate</a>`))
		case r.URL.Path == "/series/clannad/":
			w.Write([]byte(`<a href="/videos/clannad-episode-2/">Clannad Episode 2</a><a href="/videos/clannad-episode-1/">Clannad Episode 1</a><a href="/series/clannad/">Clannad</a>`))
		case r.URL.Path == "/":
			w.Write([]byte(`<p>No results</p>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := &animeSite{"t", "Test", "https://anihentai.com", nil}
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	r := animeSearchSite(s, []string{"Clannad"})
	if r.Error != "" || len(r.Items) != 1 || r.Items[0].URL != "https://anihentai.com/series/clannad/" {
		t.Fatalf("%+v", r)
	}
	none := animeSearchSite(s, []string{"Unknown Title"})
	if none.Error != "" || len(none.Items) != 0 {
		t.Fatalf("%+v", none)
	}
	eps, err := animeEpisodes(s, "https://anihentai.com/series/clannad/")
	if err != nil || len(eps) != 2 || eps[0].N != 1 || eps[1].N != 2 {
		t.Fatalf("%v %+v", err, eps)
	}
	if _, err := animeEpisodes(s, "https://evil.example/x"); err == nil {
		t.Fatal("foreign page accepted")
	}
	if !cgOpenAllowed("hentaiera.com") || !cgOpenAllowed("www.anihentai.com") || cgOpenAllowed("evil.example") {
		t.Fatal("open allow-list")
	}
}

func TestAnimeEpisodesNamedFromAddressAndLooseMatch(t *testing.T) {
	s := animeSites[0]
	page := `<ul><li><a href="/videos/mama-holic-episode-1/">Watch Now</a></li><li><a href="/videos/mama-holic-episode-2/"><img src="/t.jpg"></a></li><li><a href="/videos/mama-holic-episode-3/">3</a></li></ul>`
	items := animeLinks(s, page)
	if len(items) != 3 || items[0].N != 1 || items[2].N != 3 || !items[1].Episode || items[0].Title != "Episode 1" {
		t.Fatalf("%+v", items)
	}
	if !animeMatch("Mama X holic Imouto", "Mamax Holic Episode 1") && animeMatch("Mama X holic", "Totally Different") {
		t.Fatal("matching")
	}
	if animeMatch("Clannad After Story", "Fate Zero Episode 1") {
		t.Fatal("false match")
	}
	if !animeMatch("Mama X holic Imouto", "Mama x Holic Episode 1") {
		t.Fatal("loose match failed")
	}
}

func TestAnimeBrowseFrontPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Write([]byte(`<a href="/series/clannad/"><img src="/x.jpg" alt="Clannad"></a><a href="/series/clannad/">Clannad</a><a href="/videos/fate-episode-1/">Fate Episode 1</a>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	animeMu.Lock()
	delete(animeCache, "\x00browse")
	animeMu.Unlock()
	res := animeBrowse()
	if len(res) < 1 || res[0].Error != "" || len(res[0].Items) < 2 {
		t.Fatalf("%+v", res)
	}
}

func TestAnimeListFiltersRandomNext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/random/":
			http.Redirect(w, r, "/hentai/some-title/", 302)
		case r.URL.Path == "/browse/":
			w.Write([]byte(`<form action="/browse/" method="get"><select name="genre"><option value="">All genres</option><option value="milf">Milf</option></select><select name="studio"><option value="">All studios</option><option value="pink">Pink Pineapple</option></select></form>`))
		case r.URL.Path == "/hentai/":
			w.Write([]byte(`<a href="#content">Skip to content</a><a href="/hentai/">Series</a><a href="/browse/">Browse</a><a href="/random/">Surprise me</a>
<article><img data-src="/t/a.jpg" alt=""><a href="/hentai/alpha/" title="Alpha">Alpha</a></article>
<li><a href="/hentai/beta/"><img src="/t/b.jpg" srcset="/t/b.jpg 1x, /t/b2.jpg 2x" alt="Beta"></a></li>
<a href="/hentai/page/2/" rel="next">Next</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	s := animeSites[0]
	out, err := animeListPage(s, s.base+"/hentai/")
	if err != nil || len(out.Items) != 2 || out.Items[0].Title != "Alpha" || out.Items[0].Thumb == "" || out.Items[1].Thumb == "" || out.Next != "https://anihentai.com/hentai/page/2/" {
		t.Fatalf("%v %+v", err, out)
	}
	act, f, err := animeFilters(s)
	if err != nil || act != "/browse/" || len(f) != 2 || f[0].Name != "genre" || f[0].Label != "genres" || f[1].Options[0].V != "pink" {
		t.Fatalf("%v %q %+v", err, act, f)
	}
	it, err := animeRandom(s)
	if err != nil || it.URL != "https://anihentai.com/hentai/some-title/" || it.Title != "Some Title" || it.Episode {
		t.Fatalf("%v %+v", err, it)
	}
}

func TestAnimeCardTitlesAndInfo(t *testing.T) {
	s := animeSites[0]
	page := `<a href="/hentai/kan-ochi-x-netorare/"><div class="poster"><img src="/t/a.jpg" alt="Cover unavailable"></div><span>Big Boobs</span><span>Blow Job</span><span>Censored</span><div class="x"><b>EP</b></div></a>
<a href="/hentai/with-heading/"><img src="/t/b.jpg"><h3>Real Heading</h3><span>Tag</span><span>Tag2</span></a>`
	its := animeLinks(s, page)
	if len(its) != 2 || its[0].Title != "Kan Ochi X Netorare" || its[1].Title != "Real Heading" {
		t.Fatalf("%+v", its)
	}
	series := `<head><meta property="og:image" content="/up/poster.jpg"><meta property="og:description" content="Short."></head>
<h1>So Low Series</h1><p>` + strings.Repeat("A long synopsis sentence. ", 6) + `</p>
<div>Studio: <a href="/studio/pink/">Pink Pineapple</a></div><div>Status</div><div>Completed</div><a href="/genre/milf/">Milf</a><a href="/year/2024/">2024</a>
<a href="/episodes/so-low-episode-1/">Episode 1 Ahegao Big Boobs</a><a href="/episodes/so-low-2/">Watch</a><a href="/episodes/so-low-episode-3/"><img src="/x.jpg"></a><a href="/episodes/">Episodes</a>`
	in := animeInfoParse(s, series, "https://anihentai.com/hentai/so-low/")
	if in.Title != "So Low Series" || in.Poster == "" || !strings.Contains(in.Synopsis, "synopsis sentence") || len(in.Genres) != 1 || in.Genres[0].V != "/genre/milf/" ||
		len(in.Studios) != 1 || in.Year != "2024" || len(in.Episodes) != 3 || in.Episodes[1].N != 2 || in.Episodes[2].Title != "Episode 3" {
		t.Fatalf("%+v", in)
	}
	var st string
	for _, f := range in.Facts {
		if f.K == "Status" {
			st = f.V
		}
	}
	if st != "Completed" {
		t.Fatalf("facts %+v", in.Facts)
	}
}

func TestAkiHListSearchTagRandom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cards := `<a href="/latest/">Latest Updated</a><a href="/popular/">Most Popular</a><a href="/az-list/">A-Z</a><a href="/genre/3d/">3D</a><a href="/random/">Random</a>
<a href="/sister-breeder/" title="Sister Breeder"><img data-src="https://i.aki-h.com/poster/a.webp"><span>EP 1-8</span></a>
<a href="/mankitsu-happening/" title="Mankitsu Happening"><img src="https://i.aki-h.com/poster/b.webp"></a>`
		switch {
		case r.URL.Path == "/search" && r.URL.Query().Get("keyword") == "sister":
			w.Write([]byte(`<a href="/sister-breeder/" title="Sister Breeder"><img src="https://i.aki-h.com/poster/a.webp"></a>`))
		case r.URL.Path == "/latest/", r.URL.Path == "/genre/milf/":
			w.Write([]byte(cards))
		case r.URL.Path == "/random/":
			http.Redirect(w, r, "/mankitsu-happening/", 302)
		case r.URL.Path == "/mankitsu-happening/":
			w.Write([]byte(`<h1>Mankitsu Happening</h1><a href="/genre/3d/">3D</a><a href="/episode/mankitsu-happening/?ep=1">Ep 1</a><a href="/episode/mankitsu-happening/?ep=2">Ep 2</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	s := animeSiteByID("akih")
	if s == nil {
		t.Fatal("no akih site")
	}
	out, err := animeListPage(s, s.base+"/latest/")
	if err != nil || len(out.Items) != 2 || out.Items[0].Title != "Sister Breeder" || out.Items[0].Thumb == "" || out.Next != "https://aki-h.com/latest/?page=2" {
		t.Fatalf("%v %+v", err, out)
	}
	res := animeSearchSite(s, []string{"sister"})
	if len(res.Items) != 1 || res.Items[0].Title != "Sister Breeder" {
		t.Fatalf("%+v", res)
	}
	if it, err := animeRandom(s); err != nil || it.URL != "https://aki-h.com/mankitsu-happening/" || it.Site != "akih" {
		t.Fatalf("%v %+v", err, it)
	}
	if s.genrePath("milf") != "/genre/milf/" {
		t.Fatal(s.genrePath("milf"))
	}
	in, err := animeInfoFor(s, "https://aki-h.com/mankitsu-happening/")
	if err != nil || in.Title != "Mankitsu Happening" || len(in.Episodes) != 2 || in.Episodes[1].N != 2 || !strings.HasSuffix(in.Episodes[1].URL, "?ep=2") || in.Site != "akih" {
		t.Fatalf("%v %+v", err, in)
	}
}

func TestAnimePageReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<h1>Some Show</h1><iframe src="https://aki-h.com/embed/1"></iframe><a href="/episode/some-show/?ep=1">1</a>`))
	}))
	defer srv.Close()
	old := cgHTTP
	cgHTTP = &http.Client{Transport: rewriteTransport{srv.Listener.Addr().String()}}
	defer func() { cgHTTP = old }()
	mux := http.NewServeMux()
	(&App{}).animeListRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/anime/dump?site=akih&url=https%3A%2F%2Faki-h.com%2Fsome-show%2F", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `title="Some Show"`) || !strings.Contains(body, "episode 1:") || !strings.Contains(body, "===== page source =====") || !strings.Contains(body, "player:") {
		t.Fatalf("%d %s", rec.Code, body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/anime/dump?site=akih&url=https%3A%2F%2Fevil.example%2Fx", nil))
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestAkiHWatchEpisodes(t *testing.T) {
	s := animeSiteByID("akih")
	page := `<a href="https://aki-h.com/watch/3S5dfOZrM6/" class="ak-btn">Watch now</a><a href="https://aki-h.com/episode/solow-sub/" class="ak-btn">All episodes</a>
<div class="ak-ep"><a class="ak-ep__thumb" href="https://aki-h.com/watch/3S5dfOZrM6/" title="So_low Vol 1 Sub-Eng"><img data-src="x"></a><h3><a href="https://aki-h.com/watch/3S5dfOZrM6/">So_low Vol 1 Sub-Eng</a></h3></div>
<div class="ak-ep"><a class="ak-ep__thumb" href="https://aki-h.com/watch/8MoiIRzCT2/" title="So_low ตอนที่ 1 ซับไทย"></a></div>`
	eps := animeEpisodeLinks(s, page, "https://aki-h.com/solow-sub/")
	if len(eps) != 2 || eps[0].URL != "https://aki-h.com/watch/3S5dfOZrM6/" || eps[0].Title != "So_low Vol 1 Sub-Eng" || eps[1].N != 2 || !strings.Contains(eps[1].Title, "ซับไทย") {
		t.Fatalf("%+v", eps)
	}
}

func TestAkiHPosterNotAd(t *testing.T) {
	s := animeSiteByID("akih")
	page := `<meta property="og:image" content=""><h1>X</h1><img data-src="https://image.cdend.com/wp-content/uploads/2026/02/ad.gif" class="lazyload">
<div class="ak-detail__poster"><img src="https://i.aki-h.com/images/2t5GhTV.webp" alt="x"></div>`
	in := animeInfoParse(s, page, "https://aki-h.com/x/")
	if in.Poster == "" || strings.Contains(in.Poster, "cdend") || !strings.Contains(in.Poster, "aki-h.com") {
		t.Fatalf("%q", in.Poster)
	}
}
