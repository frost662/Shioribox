package main

// HStream (hstream.moe) as a second source for the Anime page. Its pages are ordinary server-rendered HTML:
//   /search?order=recently-uploaded[&search=<words>][&tags[0]=<tag>][&page=N]   listing
//   /hentai/<series>-<n>                                                       one episode
// The site's own video player signs its requests inside the page, so an episode is played by embedding the site's page
// (its player runs in the app's window) rather than by fetching the stream here.

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const hstreamID = "hstream"

var hstreamSite = &animeSite{hstreamID, "HStream", "https://hstream.moe", nil}

var (
	reHSTail    = regexp.MustCompile(`^(.*?)-(\d{1,3})$`)
	reHSTitleEp = regexp.MustCompile(`[\s-](\d{1,3})$`)
	reHSTag     = regexp.MustCompile(`(?i)[?&](tags|studios|brands|genres)(?:\[\d*\]|%5B\d*%5D)=([^&"'#]+)`)
)

func hstreamListURL(q, tag string, page int) string {
	v := neturl.Values{}
	v.Set("order", "recently-uploaded")
	if q != "" {
		v.Set("search", q)
	}
	if tag != "" {
		v.Set("tags[0]", tag)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	return hstreamSite.base + "/search?" + v.Encode()
}

func hstreamEpisodePath(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	return len(segs) == 2 && strings.EqualFold(segs[0], "hentai")
}

func hstreamPageURL(raw string) (string, error) {
	if !animeSameHost(hstreamSite, raw) || !hstreamEpisodePath(raw) {
		return "", errors.New("that page isn't on HStream")
	}
	u, _ := neturl.Parse(raw)
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// hstreamList returns the cards of one listing page. next is the following page number, or "" when this page was empty.
func hstreamList(q, tag string, page int) ([]AnimeItem, string, error) {
	if page < 1 {
		page = 1
	}
	out, err := animeListPage(hstreamSite, hstreamListURL(q, tag, page))
	if err != nil {
		return nil, "", err
	}
	items := []AnimeItem{}
	for _, it := range out.Items {
		if !hstreamEpisodePath(it.URL) {
			continue
		}
		it.Site, it.Episode, it.N = hstreamID, false, 0
		if m := reHSTail.FindStringSubmatch(animeSlugKey(it.URL)); m != nil {
			it.Title = hstreamTrimEp(it.Title, m[2])
		}
		items = append(items, it)
	}
	next := ""
	if len(items) > 0 {
		next = strconv.Itoa(page + 1)
	}
	return items, next, nil
}

// hstreamTrimEp drops the trailing episode number ("Sister Breeder - 6", "Sister Breeder 6") from a title.
func hstreamTrimEp(title, n string) string {
	t := strings.TrimSpace(title)
	if strings.HasSuffix(t, n) {
		t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(t, n)), "-"))
	}
	return t
}

func animeSlugKey(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

func hstreamInfo(pageURL string) (AnimeInfo, error) {
	pageURL, err := hstreamPageURL(pageURL)
	if err != nil {
		return AnimeInfo{}, err
	}
	b, err := animeGet(hstreamSite, pageURL)
	if err != nil {
		return AnimeInfo{}, err
	}
	page := string(b)
	info := animeInfoParse(hstreamSite, page, pageURL)
	info.Site = hstreamID
	// tags and studios are links back to the search page: /search?...&tags[0]=big-boobs
	have := map[string]bool{}
	baseU, _ := neturl.Parse(hstreamSite.base)
	for _, m := range reAAnchor.FindAllStringSubmatch(page, -1) {
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		raw := html.UnescapeString(strings.TrimSpace(hm[1]))
		if u, e := baseU.Parse(raw); e == nil {
			raw = u.String()
		}
		tm := reHSTag.FindStringSubmatch(raw)
		if tm == nil {
			continue
		}
		v, _ := neturl.QueryUnescape(tm[2])
		name := unesc(m[2])
		if name == "" || len(name) > 50 || have[tm[1]+v] {
			continue
		}
		have[tm[1]+v] = true
		if strings.EqualFold(tm[1], "tags") || strings.EqualFold(tm[1], "genres") {
			info.Genres = append(info.Genres, AnimeFilterOpt{V: v, L: name})
		} else {
			info.Studios = append(info.Studios, AnimeFilterOpt{V: v, L: name})
		}
	}
	// episodes of the same series: /hentai/<series>-<n>
	self := animeSlugKey(pageURL)
	base := self
	if m := reHSTail.FindStringSubmatch(self); m != nil {
		base = m[1]
	}
	seen := map[string]bool{}
	var eps []AnimeItem
	add := func(raw string) {
		u, e := baseU.Parse(html.UnescapeString(strings.TrimSpace(raw)))
		if e != nil || !animeSameHost(hstreamSite, u.String()) {
			return
		}
		u.RawQuery, u.Fragment = "", ""
		if !hstreamEpisodePath(u.String()) || seen[u.String()] {
			return
		}
		m := reHSTail.FindStringSubmatch(animeSlugKey(u.String()))
		if m == nil || m[1] != base {
			return
		}
		seen[u.String()] = true
		n, _ := strconv.Atoi(m[2])
		eps = append(eps, AnimeItem{Title: "Episode " + strconv.Itoa(n), URL: u.String(), Episode: true, N: n, Site: hstreamID})
	}
	add(pageURL)
	for _, m := range reAAnchor.FindAllStringSubmatch(page, -1) {
		if hm := reAHref.FindStringSubmatch(m[1]); hm != nil {
			add(hm[1])
		}
	}
	if len(eps) == 0 {
		eps = []AnimeItem{{Title: "Episode 1", URL: pageURL, Episode: true, N: 1, Site: hstreamID}}
	}
	sort.SliceStable(eps, func(i, j int) bool { return eps[i].N < eps[j].N })
	info.Episodes = eps
	if m := reHSTitleEp.FindStringSubmatch(info.Title); m != nil {
		info.Title = hstreamTrimEp(info.Title, m[1])
	}
	return info, nil
}

func hstreamRandom() (AnimeItem, error) {
	c := *cgHTTP
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if req, err := http.NewRequest("GET", hstreamSite.base+"/random", nil); err == nil {
		req.Header.Set("User-Agent", cgUA)
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
			if loc := resp.Header.Get("Location"); loc != "" {
				if u, e := neturl.Parse(hstreamSite.base); e == nil {
					if t, e := u.Parse(loc); e == nil && hstreamEpisodePath(t.String()) && animeSameHost(hstreamSite, t.String()) {
						return AnimeItem{Title: animeSlugTitle(t.String()), URL: t.String(), Site: hstreamID}, nil
					}
				}
			}
		}
	}
	p, _ := crand.Int(crand.Reader, big.NewInt(8))
	items, _, err := hstreamList("", "", int(p.Int64())+1)
	if err != nil || len(items) == 0 {
		if items, _, err = hstreamList("", "", 1); err != nil || len(items) == 0 {
			return AnimeItem{}, errors.New("HStream's random pick isn't available right now")
		}
	}
	i, _ := crand.Int(crand.Reader, big.NewInt(int64(len(items))))
	return items[i.Int64()], nil
}

// hstreamEmbed: the episode page itself, played by the site's own player inside the app.
func hstreamEmbed(pageURL string) ([]string, error) {
	u, err := hstreamPageURL(pageURL)
	if err != nil {
		return nil, err
	}
	return []string{u}, nil
}

var _ = fmt.Sprint
