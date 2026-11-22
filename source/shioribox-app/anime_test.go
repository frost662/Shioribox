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
	s := &animeSite{"t", "Test", "https://anihentai.com"}
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
