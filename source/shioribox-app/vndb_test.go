package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildFilters(t *testing.T) {
	f, err := buildFilters(DiscoverQuery{Q: "clannad", Tags: []string{"g505"}, Platform: "win", Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(f)
	want := `["and",["search","=","clannad"],["tag","=","g505"],["platform","=","win"],["lang","=","en"]]`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	f, _ = buildFilters(DiscoverQuery{Lang: "ja"})
	if b, _ := json.Marshal(f); string(b) != `["lang","=","ja"]` {
		t.Fatalf("single filter: %s", b)
	}
	if _, err := buildFilters(DiscoverQuery{Tags: []string{"x1"}}); err == nil {
		t.Fatal("bad tag accepted")
	}
	if _, err := buildFilters(DiscoverQuery{Platform: "win\"]"}); err == nil {
		t.Fatal("bad platform accepted")
	}
}

func TestDiscoverAgainstMock(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		if r.URL.Path == "/tag" {
			w.Write([]byte(`{"results":[{"id":"g1","name":"School","category":"cont","vn_count":9,"searchable":true},{"id":"g2","name":"X","category":"ero","vn_count":1,"searchable":false}]}`))
			return
		}
		w.Write([]byte(`{"more":true,"results":[{"id":"v1","title":"T","olang":"ja","platforms":["win"],"votecount":5,"image":{"url":"https://t.vndb.org/x.jpg","sexual":1.5},"developers":[{"name":"D"}],"tags":[]}]}`))
	}))
	defer srv.Close()
	old := vndbBase
	vndbBase = srv.URL
	defer func() { vndbBase = old }()
	res, more, err := vndbDiscover(DiscoverQuery{Lang: "ja"})
	if err != nil || !more || len(res) != 1 || res[0].Sexual != 1.5 || res[0].Platforms[0] != "win" || res[0].Lang != "ja" {
		t.Fatalf("%v %v %+v", err, more, res)
	}
	if got["sort"] != "votecount" || got["reverse"] != true {
		t.Fatalf("default sort wrong: %v", got)
	}
	tags, err := vndbTags("school")
	if err != nil || len(tags) != 1 || tags[0].ID != "g1" {
		t.Fatalf("%v %v", tags, err)
	}
}

func TestRecommend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var q map[string]any
		json.Unmarshal(b, &q)
		if q["fields"] != vnFields { // the "what do you like" query
			w.Write([]byte(`{"results":[
			 {"id":"v1","tags":[{"id":"g1","name":"Drama","rating":2.5,"spoiler":0,"category":"cont"},{"id":"g2","name":"Mystery","rating":2,"spoiler":0,"category":"cont"},{"id":"g9","name":"ADV","rating":3,"spoiler":0,"category":"tech"}]},
			 {"id":"v2","tags":[{"id":"g1","name":"Drama","rating":2,"spoiler":0,"category":"cont"},{"id":"g2","name":"Mystery","rating":2.5,"spoiler":0,"category":"cont"},{"id":"g3","name":"Spoilery","rating":3,"spoiler":2,"category":"cont"}]}]}`))
			return
		}
		w.Write([]byte(`{"more":false,"results":[{"id":"v1","title":"Own","tags":[]},{"id":"v7","title":"New","rating":85,"tags":[{"name":"Drama"},{"name":"Mystery"}],"developers":[]}]}`))
	}))
	defer srv.Close()
	old := vndbBase
	vndbBase = srv.URL
	defer func() { vndbBase = old }()
	games := []*Game{{VNDBID: "v1", Title: "A", Rating: 9}, {VNDBID: "v2", Title: "B", Rating: 8, Favorite: true}, {Title: "no link", Rating: 10}}
	rec, err := recommend(games)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Results) != 1 || rec.Results[0].ID != "v7" {
		t.Fatalf("own game must be excluded: %+v", rec.Results)
	}
	if len(rec.Tags) != 2 || rec.Tags[0] == "ADV" || rec.Tags[0] == "Spoilery" {
		t.Fatalf("tags %v", rec.Tags)
	}
	if _, err := recommend(games[:1]); err == nil {
		t.Fatal("expected not-enough-data error")
	}
}

func TestVNDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/character" {
			w.Write([]byte(`{"results":[{"id":"c2","name":"Side","vns":[{"id":"v1","role":"side"}]},{"id":"c1","name":"Hero","original":"主","image":{"url":"u","sexual":0},"vns":[{"id":"v9","role":"main"},{"id":"v1","role":"main"}]}]}`))
			return
		}
		if r.URL.Path == "/release" {
			w.Write([]byte(`{"results":[{"producers":[{"id":"p2","name":"Pub","developer":false,"publisher":true},{"id":"p1","name":"Dev","developer":true,"publisher":false}]},{"producers":[{"id":"p1","name":"Dev","developer":true,"publisher":true}]}]}`))
			return
		}
		w.Write([]byte(`{"results":[{"id":"v1","title":"T","alttitle":"A","titles":[{"lang":"ja","title":"X","official":true}],"devstatus":0,"released":"2010-01-01","length":3,"length_minutes":1200,"rating":80,"votecount":10,"image":{"url":"i","sexual":2},"developers":[{"name":"D"}],"tags":[{"id":"g1","name":"Drama","rating":2,"spoiler":0,"category":"cont"}],"screenshots":[{"url":"s","thumbnail":"t","sexual":0}],"relations":[{"id":"v2","title":"Sequel","relation":"seq","relation_official":true}],"extlinks":[{"label":"Steam","url":"https://s"}],"staff":[{"name":"W","role":"scenario","note":""}]}]}`))
	}))
	defer srv.Close()
	old := vndbBase
	vndbBase = srv.URL
	defer func() { vndbBase = old }()
	d, err := vndbDetail("V1")
	if err != nil || d.Title != "T" || d.LengthMinutes != 1200 || d.Sexual != 2 || len(d.Relations) != 1 || len(d.Staff) != 1 {
		t.Fatalf("%v %+v", err, d)
	}
	if len(d.Characters) != 2 || d.Characters[0].Name != "Hero" || d.Characters[0].Role != "main" {
		t.Fatalf("characters %+v", d.Characters)
	}
	if len(d.Producers) != 2 || d.Producers[0].ID != "p1" || !d.Producers[0].Publisher || !d.Producers[0].Developer {
		t.Fatalf("producers %+v", d.Producers)
	}
	if _, err := buildFilters(DiscoverQuery{Producer: "p1; x"}); err == nil {
		t.Fatal("bad producer accepted")
	}
	if f, err := buildFilters(DiscoverQuery{Producer: "P7"}); err != nil || fmt.Sprint(f) != "[release = [producer = [id = p7]]]" {
		t.Fatalf("%v %v", f, err)
	}
	if _, err := vndbDetail("x; drop"); err == nil {
		t.Fatal("bad id accepted")
	}
}

func TestCharDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"id":"c1","name":"Hero","aliases":["H"],"description":"d","birthday":[3,14],"sex":["m","m"],"height":170,"blood_type":"a","traits":[{"id":"i1","name":"Kind","group_name":"Personality","spoiler":0}],"vns":[{"id":"v1","title":"T","role":"main","spoiler":0}]}]}`))
	}))
	defer srv.Close()
	old := vndbBase
	vndbBase = srv.URL
	defer func() { vndbBase = old }()
	d, err := vndbCharDetail("C1")
	if err != nil || d.Name != "Hero" || d.Birthday != "3/14" || d.Sex != "m" || d.Height != 170 || len(d.Traits) != 1 || len(d.VNs) != 1 {
		t.Fatalf("%v %+v", err, d)
	}
	if _, err := vndbCharDetail("c1;x"); err == nil {
		t.Fatal("bad id accepted")
	}
}
