package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type VNDBResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	AltTitle    string   `json:"altTitle"`
	Released    string   `json:"released"`
	Description string   `json:"description"`
	Image       string   `json:"image"`
	Developers  []string `json:"developers"`
	Rating      float64  `json:"rating"` // 10-100 on VNDB
	Length      int      `json:"lengthMinutes"`
	Tags        []string `json:"tags"`
	Lang        string   `json:"lang"`      // original language
	Platforms   []string `json:"platforms"` // platform codes
	Votes       int      `json:"votes"`
	Sexual      float64  `json:"sexual"` // cover flag: 0 safe .. 2 explicit
}

var (
	reSpoiler    = regexp.MustCompile(`(?is)\[spoiler\].*?\[/spoiler\]`)
	reURLTag     = regexp.MustCompile(`(?is)\[url=[^\]]*\](.*?)\[/url\]`)
	reTag        = regexp.MustCompile(`(?i)\[/?(b|i|u|s|quote|raw|code|url)\]`)
	reVNID       = regexp.MustCompile(`(?i)^v\d+$`)
	reProducerID = regexp.MustCompile(`(?i)^p\d{1,7}$`)
	httpc        = &http.Client{Timeout: 20 * time.Second}
	vndbBase     = "https://api.vndb.org/kana"
)

func cleanDesc(s string) string {
	s = reSpoiler.ReplaceAllString(s, "")
	s = reURLTag.ReplaceAllString(s, "$1")
	s = reTag.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func vndbSearch(q string) ([]VNDBResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("empty query")
	}
	filters := any([]any{"search", "=", q})
	sortBy := "searchrank"
	if reVNID.MatchString(q) {
		filters = []any{"id", "=", strings.ToLower(q)}
		sortBy = "id"
	}
	res, _, err := vndbVN(filters, sortBy, false, 1, 12)
	return res, err
}

const vnFields = "id,title,alttitle,released,description,image.url,image.sexual,developers.name,rating,length_minutes,olang,platforms,votecount,tags.name,tags.rating,tags.spoiler"

// vndbPost sends a JSON query to a VNDB API endpoint and returns the raw reply.
func vndbPost(path string, payload map[string]any) ([]byte, error) {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", vndbBase+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ShioriBox/"+version)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach VNDB: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("VNDB returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// vndbVN runs one /vn query; the bool says whether another page exists.
func vndbVN(filters any, sortBy string, reverse bool, page, n int) ([]VNDBResult, bool, error) {
	raw, err := vndbPost("/vn", map[string]any{
		"filters": filters, "fields": vnFields, "sort": sortBy, "reverse": reverse, "page": page, "results": n,
	})
	if err != nil {
		return nil, false, err
	}
	var parsed struct {
		More    bool `json:"more"`
		Results []struct {
			ID          string   `json:"id"`
			Title       string   `json:"title"`
			AltTitle    *string  `json:"alttitle"`
			Released    *string  `json:"released"`
			Description *string  `json:"description"`
			Rating      *float64 `json:"rating"`
			Length      *int     `json:"length_minutes"`
			OLang       string   `json:"olang"`
			Platforms   []string `json:"platforms"`
			Votes       int      `json:"votecount"`
			Image       *struct {
				URL    string   `json:"url"`
				Sexual *float64 `json:"sexual"`
			} `json:"image"`
			Developers []struct {
				Name string `json:"name"`
			} `json:"developers"`
			Tags []struct {
				Name    string  `json:"name"`
				Rating  float64 `json:"rating"`
				Spoiler int     `json:"spoiler"`
			} `json:"tags"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, false, err
	}
	out := []VNDBResult{}
	for _, r := range parsed.Results {
		v := VNDBResult{ID: r.ID, Title: r.Title, Developers: []string{}, Tags: []string{}, Lang: r.OLang, Platforms: r.Platforms, Votes: r.Votes}
		if v.Platforms == nil {
			v.Platforms = []string{}
		}
		if r.AltTitle != nil {
			v.AltTitle = *r.AltTitle
		}
		if r.Released != nil {
			v.Released = *r.Released
		}
		if r.Description != nil {
			v.Description = cleanDesc(*r.Description)
		}
		if r.Rating != nil {
			v.Rating = *r.Rating
		}
		if r.Length != nil {
			v.Length = *r.Length
		}
		if r.Image != nil {
			v.Image = r.Image.URL
			if r.Image.Sexual != nil {
				v.Sexual = *r.Image.Sexual
			}
		}
		for _, d := range r.Developers {
			v.Developers = append(v.Developers, d.Name)
		}
		sort.Slice(r.Tags, func(i, j int) bool { return r.Tags[i].Rating > r.Tags[j].Rating })
		for _, t := range r.Tags {
			if t.Spoiler == 0 && len(v.Tags) < 10 {
				v.Tags = append(v.Tags, t.Name)
			}
		}
		out = append(out, v)
	}
	return out, parsed.More, nil
}

// fetchImage downloads an image from VNDB's image host only.
func fetchImage(raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !(u.Hostname() == "vndb.org" || strings.HasSuffix(u.Hostname(), ".vndb.org")) {
		return nil, "", fmt.Errorf("only https://*.vndb.org image URLs are allowed")
	}
	req, _ := http.NewRequest("GET", raw, nil)
	req.Header.Set("User-Agent", "ShioriBox/"+version)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("image download returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	return b, resp.Header.Get("Content-Type"), nil
}

// DiscoverQuery is the Discover page's search form.
type DiscoverQuery struct {
	Q        string   `json:"q"`
	Tags     []string `json:"tags"` // VNDB tag ids such as "g505"
	Platform string   `json:"platform"`
	Lang     string   `json:"lang"`  // a language the VN is available in
	OLang    string   `json:"olang"` // the language it was originally written in
	Sort     string   `json:"sort"`
	Reverse  bool     `json:"reverse"`
	Page     int      `json:"page"`
	Producer string   `json:"producer"` // VNDB producer id such as "p98": developer or publisher
}

var (
	reTagID    = regexp.MustCompile(`^g\d{1,6}$`)
	reCode     = regexp.MustCompile(`^[a-z0-9-]{2,12}$`)
	sortFields = map[string]bool{"searchrank": true, "rating": true, "votecount": true, "released": true, "title": true}
)

// buildFilters turns the form into a VNDB filter expression (nil = no filter at all).
func buildFilters(d DiscoverQuery) (any, error) {
	var f []any
	if q := strings.TrimSpace(d.Q); q != "" {
		if reVNID.MatchString(q) {
			f = append(f, []any{"id", "=", strings.ToLower(q)})
		} else {
			f = append(f, []any{"search", "=", q})
		}
	}
	for _, t := range d.Tags {
		if !reTagID.MatchString(t) {
			return nil, fmt.Errorf("bad tag id %q", t)
		}
		f = append(f, []any{"tag", "=", t})
	}
	if d.Producer != "" {
		if !reProducerID.MatchString(d.Producer) {
			return nil, fmt.Errorf("bad producer id %q", d.Producer)
		}
		f = append(f, []any{"release", "=", []any{"producer", "=", []any{"id", "=", strings.ToLower(d.Producer)}}})
	}
	for _, p := range []struct{ name, val string }{{"platform", d.Platform}, {"lang", d.Lang}, {"olang", d.OLang}} {
		if p.val == "" {
			continue
		}
		if !reCode.MatchString(p.val) {
			return nil, fmt.Errorf("bad %s %q", p.name, p.val)
		}
		f = append(f, []any{p.name, "=", p.val})
	}
	switch len(f) {
	case 0:
		return []any{}, nil
	case 1:
		return f[0], nil
	}
	return append([]any{"and"}, f...), nil
}

func vndbDiscover(d DiscoverQuery) ([]VNDBResult, bool, error) {
	f, err := buildFilters(d)
	if err != nil {
		return nil, false, err
	}
	sortBy := d.Sort
	if !sortFields[sortBy] {
		sortBy = "searchrank"
	}
	if sortBy == "searchrank" && strings.TrimSpace(d.Q) == "" {
		sortBy, d.Reverse = "votecount", true // nothing to rank by: show the popular ones
	}
	if d.Page < 1 {
		d.Page = 1
	}
	return vndbVN(f, sortBy, d.Reverse, d.Page, 24)
}

type VNDBTag struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

func vndbTags(q string) ([]VNDBTag, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []VNDBTag{}, nil
	}
	raw, err := vndbPost("/tag", map[string]any{
		"filters": []any{"search", "=", q}, "fields": "id,name,category,vn_count,searchable",
		"sort": "searchrank", "results": 15,
	})
	if err != nil {
		return nil, err
	}
	var p struct {
		Results []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Category   string `json:"category"`
			Count      int    `json:"vn_count"`
			Searchable bool   `json:"searchable"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	out := []VNDBTag{}
	for _, t := range p.Results {
		if t.Searchable {
			out = append(out, VNDBTag{t.ID, t.Name, t.Category, t.Count})
		}
	}
	return out, nil
}

type VNDBProducer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Original string `json:"original"`
	Type     string `json:"type"` // co company, in individual, ng amateur group
}

// vndbProducers searches developers and publishers by name.
func vndbProducerSearch(q string) ([]VNDBProducer, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []VNDBProducer{}, nil
	}
	raw, err := vndbPost("/producer", map[string]any{
		"filters": []any{"search", "=", q}, "fields": "id,name,original,type", "sort": "searchrank", "results": 12,
	})
	if err != nil {
		return nil, err
	}
	var p struct {
		Results []VNDBProducer `json:"results"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Results == nil {
		p.Results = []VNDBProducer{}
	}
	return p.Results, nil
}
