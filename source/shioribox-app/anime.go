package main

// Anime tab: looks a visual novel's title up on a few anime streaming sites and lists the episodes it finds. The sites
// are ordinary WordPress sites, so search is /?s=<title>. Pages are read with tolerant patterns (links, titles and
// pictures), and episodes are opened on the site itself in the user's browser.

import (
	"fmt"
	"html"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type animeSite struct {
	id, name, base string
	cfg            *animeCfg // nil = the common WordPress-style layout (AniHentai)
}

// animeCfg describes how a site differs from the common layout.
type animeCfg struct {
	latest []string                 // listing paths tried for "latest"
	search []string                 // search paths tried in order; %s is the escaped query
	isCard func(segs []string) bool // does a link path look like a series card
	random string                   // random-pick path
	pageQ  string                   // query key for paging a listing that has no "next" link
	genre  string                   // path of a tag page; %s is the tag
}

var akihReserved = map[string]bool{"latest": true, "popular": true, "az-list": true, "filter": true, "random": true, "genre": true, "episode": true,
	"search": true, "page": true, "assets": true, "login": true, "register": true, "contact": true, "dmca": true, "privacy": true, "terms": true,
	"about": true, "news": true, "tag": true, "studio": true, "year": true, "category": true, "wp-content": true, "wp-json": true, "feed": true}

var akihCfg = &animeCfg{
	latest: []string{"/latest/", "/"},
	search: []string{"/search?keyword=%s", "/filter/?keyword=%s", "/?s=%s", "/search?q=%s"},
	isCard: func(segs []string) bool {
		return len(segs) == 1 && !akihReserved[strings.ToLower(segs[0])] && !strings.Contains(segs[0], ".")
	},
	random: "/random/",
	pageQ:  "page",
	genre:  "/genre/%s/",
}

var animeSites = []*animeSite{
	{"anihentai", "AniHentai", "https://anihentai.com", nil},
	{"akih", "Aki-H", "https://aki-h.com", akihCfg},
}

func (s *animeSite) cardOK(raw string) bool {
	if s.cfg == nil || s.cfg.isCard == nil {
		return animeCardOK(raw)
	}
	u, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	return s.cfg.isCard(segs)
}

func (s *animeSite) latestPaths() []string {
	if s.cfg != nil && len(s.cfg.latest) > 0 {
		return s.cfg.latest
	}
	return []string{"/hentai/", "/"}
}

func (s *animeSite) searchPaths(q string) []string {
	if s.cfg != nil && len(s.cfg.search) > 0 {
		var out []string
		for _, p := range s.cfg.search {
			out = append(out, strings.Replace(p, "%s", neturl.QueryEscape(q), 1))
		}
		return out
	}
	return []string{"/?s=" + neturl.QueryEscape(q), "/search/" + neturl.PathEscape(q) + "/"}
}

func (s *animeSite) randomPath() string {
	if s.cfg != nil && s.cfg.random != "" {
		return s.cfg.random
	}
	return "/random/"
}

func (s *animeSite) genrePath(tag string) string {
	if s.cfg != nil && s.cfg.genre != "" {
		return fmt.Sprintf(s.cfg.genre, neturl.PathEscape(tag))
	}
	return "/genre/" + neturl.PathEscape(tag) + "/"
}

func animeSiteByID(id string) *animeSite {
	for _, s := range animeSites {
		if s.id == id {
			return s
		}
	}
	return nil
}

// cgOpenAllowed lists the sites the "open in browser" button may open.
func cgOpenAllowed(host string) bool {
	host = strings.ToLower(host)
	hosts := []string{"e-hentai.org", "hitomi.la", "hentaiera.com", "hentaienvy.com", "hstream.moe"}
	for _, s := range animeSites {
		hosts = append(hosts, strings.TrimPrefix(s.base, "https://"))
	}
	for _, d := range hosts {
		if host == d || host == "www."+d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

type AnimeItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Thumb   string `json:"thumb,omitempty"`
	Episode bool   `json:"episode"` // a single episode page rather than a series page
	N       int    `json:"n,omitempty"`
	Site    string `json:"site,omitempty"` // which source this came from
}

// animeMatch: every word of the search is in the title, or at least 60% of its meaningful words (titles on these
// sites are often shortened or romanised differently).
func animeMatch(q, title string) bool {
	if titleMatches(q, title) {
		return true
	}
	nt := " " + normTitle(title) + " "
	flat := strings.ReplaceAll(nt, " ", "")
	var words []string
	for _, w := range strings.Fields(normTitle(q)) {
		if len([]rune(w)) >= 3 && w != "the" && w != "and" && w != "no" && w != "wa" && w != "ga" {
			words = append(words, w)
		}
	}
	if len(words) < 2 {
		return false
	}
	hit := 0
	for _, w := range words {
		if strings.Contains(nt, " "+w) || strings.Contains(flat, w) {
			hit++
		}
	}
	return float64(hit) >= 0.6*float64(len(words)) && hit >= 2
}

type AnimeSiteResult struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Base  string      `json:"base"`
	Items []AnimeItem `json:"items"`
	Error string      `json:"error,omitempty"`
	Debug []string    `json:"debug,omitempty"` // what was asked and how many links came back, for troubleshooting
}

var reAJunkAlt = regexp.MustCompile(`(?i)cover|unavailable|poster|thumbnail`)

var (
	reAAnchor  = regexp.MustCompile(`(?is)<a\b([^>]*?)>(.*?)</a>`)
	reAHref    = regexp.MustCompile(`(?is)\bhref\s*=\s*"([^"]+)"`)
	reATitle   = regexp.MustCompile(`(?is)\btitle\s*=\s*"([^"]+)"`)
	reAEpNum   = regexp.MustCompile(`(?i)(?:episode|\bep\.?)[\s_-]*(\d{1,4})`)
	reAEpURL   = regexp.MustCompile(`(?i)(?:episode|-ep)-?(\d{1,4})`)
	reASkipURL = regexp.MustCompile(`(?i)^/(?:$|genres?/|tag/|category/|page/|author/|feed|wp-|contact|dmca|privacy|login|register|search|\?|az-list|a-z|latest|popular|trending|most|release|year|studio|brand|blog|news|forum|requests?)`)
	reAGeneric = regexp.MustCompile(`(?i)^(?:watch(?: now| episode| online)?|play|read more|more|view|home|next|prev(?:ious)?|\d+|»|«)$`)
)

// animeCardImg finds the picture of the card (<article> or <li>) an anchor sits in, for sites that put the picture
// outside the title link.
func animeCardImg(page string, from, to int) string {
	start := strings.LastIndex(page[:from], "<article")
	if i := strings.LastIndex(page[:from], "<li"); i > start {
		start = i
	}
	if start < 0 || from-start > 3000 {
		return ""
	}
	end := to + 1500
	if end > len(page) {
		end = len(page)
	}
	region := page[start:end]
	for _, tag := range []string{"</article>", "</li>"} {
		if i := strings.Index(region[to-start:], tag); i >= 0 {
			region = region[:to-start+i]
		}
	}
	if im := reHEImg.FindString(region); im != "" {
		src, _ := heImgSrc(im)
		return src
	}
	return ""
}

// animeLinks collects every internal link of a page with the best title and picture found for it.
func animeLinks(s *animeSite, page string) []AnimeItem {
	baseU, _ := neturl.Parse(s.base)
	var order []string
	by := map[string]*AnimeItem{}
	textTitle := map[string]bool{}
	for _, l := range reAAnchor.FindAllStringSubmatchIndex(page, -1) {
		m := []string{page[l[0]:l[1]], page[l[2]:l[3]], page[l[4]:l[5]]}
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		raw := html.UnescapeString(strings.TrimSpace(hm[1]))
		u, err := baseU.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		if host != strings.TrimPrefix(baseU.Hostname(), "www.") || u.EscapedPath() == "" || reASkipURL.MatchString(u.EscapedPath()+map[bool]string{true: "?", false: ""}[u.RawQuery != ""]) {
			continue
		}
		u.Fragment, u.RawQuery = "", ""
		key := u.String()
		it := by[key]
		if it == nil {
			it = &AnimeItem{URL: key}
			by[key] = it
			order = append(order, key)
		}
		inner := m[2]
		var alt, src string
		if im := reHEImg.FindString(inner); im != "" {
			src, alt = heImgSrc(im)
		}
		if src == "" && it.Thumb == "" {
			src = animeCardImg(page, l[0], l[1])
		}
		if f := strings.Fields(src); len(f) > 1 {
			src = strings.TrimSuffix(f[0], ",")
		}
		if src != "" && it.Thumb == "" {
			full := heAbs(s.base, src)
			heRegisterHost(full, s.base+"/")
			it.Thumb = heProxy(full)
		}
		title := ""
		if t := reATitle.FindStringSubmatch(m[1]); t != nil {
			title = unesc(t[1])
		}
		if title == "" {
			if t, ok := animeCardTitle(inner); ok {
				title = t
			} else if txt := unesc(reHEImg.ReplaceAllString(inner, "")); len([]rune(txt)) > 2 && !reAGeneric.MatchString(txt) {
				// plain text link: use it; a card full of badges and tags: name it from its address instead
				if strings.Count(inner, "<") <= 4 {
					title = txt
				} else if t := animeSlugTitle(key); t != "" {
					title = t
				}
			}
		}
		if title != "" && (!textTitle[key] || len(title) > len(it.Title)) {
			it.Title, textTitle[key] = title, true
		} else if it.Title == "" && alt != "" && !reAJunkAlt.MatchString(alt) {
			it.Title = unesc(alt)
		}
	}
	var out []AnimeItem
	for _, k := range order {
		it := by[k]
		urlEp := reAEpURL.FindStringSubmatch(it.URL)
		if it.Title == "" || reAGeneric.MatchString(it.Title) {
			// "Watch now", "1", a bare picture… on an episode address: name it from the address
			if urlEp == nil {
				continue
			}
			it.Title = "Episode " + urlEp[1]
		}
		if m := reAEpNum.FindStringSubmatch(it.Title); m != nil {
			it.N, _ = strconv.Atoi(m[1])
		} else if urlEp != nil {
			it.N, _ = strconv.Atoi(urlEp[1])
		}
		it.Episode = it.N > 0 || strings.Contains(strings.ToLower(it.URL), "episode")
		out = append(out, *it)
	}
	return out
}

var (
	animeMu    sync.Mutex
	animeCache = map[string]animeCached{}
)

type animeCached struct {
	at  time.Time
	res []AnimeSiteResult
}

func animeGet(s *animeSite, u string) ([]byte, error) {
	b, _, err := cgGet(u, map[string]string{"Referer": s.base + "/", "Accept": "text/html,application/xhtml+xml"}, cgMaxPageBody)
	return b, err
}

func animeSearchSite(s *animeSite, queries []string) AnimeSiteResult {
	res := AnimeSiteResult{ID: s.id, Name: s.name, Base: s.base, Items: []AnimeItem{}}
	seen := map[string]bool{}
	var lastErr error
	okAny := false
	for _, q := range queries {
		// WordPress sites answer /?s=...; some also have /search/<words>/
		for _, path := range s.searchPaths(q) {
			b, err := animeGet(s, s.base+path)
			if err != nil {
				lastErr = err
				res.Debug = append(res.Debug, fmt.Sprintf("%s → %v", path, err))
				continue
			}
			okAny = true
			links := animeLinks(s, string(b))
			matched := 0
			for _, it := range links {
				if seen[it.URL] || !animeMatch(q, it.Title) {
					continue
				}
				seen[it.URL] = true
				matched++
				res.Items = append(res.Items, it)
			}
			res.Debug = append(res.Debug, fmt.Sprintf("%s → %d links, %d matched", path, len(links), matched))
			if matched > 0 {
				break
			}
		}
		if len(res.Items) > 0 {
			break
		}
	}
	if !okAny && lastErr != nil {
		res.Error = lastErr.Error()
	}
	if len(res.Items) > 24 {
		res.Items = res.Items[:24]
	}
	return res
}

// animeSearch asks every site at once. Titles tried: the given ones, then without a subtitle.
func animeSearch(q, alt string) []AnimeSiteResult {
	var queries []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		for _, x := range queries {
			if strings.EqualFold(x, t) {
				return
			}
		}
		queries = append(queries, t)
	}
	add(q)
	add(alt)
	for _, t := range []string{q, alt} {
		if loc := reSubtitleCut.FindStringIndex(t); loc != nil && loc[0] > 2 {
			add(t[:loc[0]])
		}
	}
	// last resort: the first two words ("Mama X holic" -> "Mama X")
	if f := strings.Fields(q); len(f) > 2 {
		add(strings.Join(f[:2], " "))
	}
	key := strings.Join(queries, "\x00")
	animeMu.Lock()
	if c, ok := animeCache[key]; ok && time.Since(c.at) < 10*time.Minute {
		animeMu.Unlock()
		return c.res
	}
	animeMu.Unlock()
	out := make([]AnimeSiteResult, len(animeSites))
	var wg sync.WaitGroup
	for i, s := range animeSites {
		wg.Add(1)
		go func(i int, s *animeSite) {
			defer wg.Done()
			out[i] = animeSearchSite(s, queries)
		}(i, s)
	}
	wg.Wait()
	animeMu.Lock()
	animeCache[key] = animeCached{time.Now(), out}
	animeMu.Unlock()
	return out
}

// animeBrowse lists what a site's front page shows (latest series and episodes) for the Anime page.
func animeBrowse() []AnimeSiteResult {
	animeMu.Lock()
	if c, ok := animeCache["\x00browse"]; ok && time.Since(c.at) < 10*time.Minute {
		animeMu.Unlock()
		return c.res
	}
	animeMu.Unlock()
	out := make([]AnimeSiteResult, len(animeSites))
	var wg sync.WaitGroup
	for i, s := range animeSites {
		wg.Add(1)
		go func(i int, s *animeSite) {
			defer wg.Done()
			res := AnimeSiteResult{ID: s.id, Name: s.name, Base: s.base, Items: []AnimeItem{}}
			b, err := animeGet(s, s.base+"/")
			if err != nil {
				res.Error = err.Error()
				out[i] = res
				return
			}
			links := animeLinks(s, string(b))
			res.Items = links
			if len(res.Items) > 48 {
				res.Items = res.Items[:48]
			}
			res.Debug = append(res.Debug, fmt.Sprintf("front page → %d links", len(links)))
			out[i] = res
		}(i, s)
	}
	wg.Wait()
	failed := false
	for _, r := range out {
		if r.Error != "" {
			failed = true
		}
	}
	if !failed {
		animeMu.Lock()
		animeCache["\x00browse"] = animeCached{time.Now(), out}
		animeMu.Unlock()
	}
	return out
}

// animeEpisodes reads a series page and returns its episode links, lowest number first.
func animeEpisodes(s *animeSite, pageURL string) ([]AnimeItem, error) {
	u, err := neturl.Parse(pageURL)
	if err != nil || u.Scheme != "https" || strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != strings.TrimPrefix(strings.TrimPrefix(s.base, "https://"), "www.") {
		return nil, fmt.Errorf("that page isn't on %s", s.name)
	}
	b, err := animeGet(s, pageURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.name, err)
	}
	eps := animeEpisodeLinks(s, string(b), pageURL)
	sort.SliceStable(eps, func(i, j int) bool { return eps[i].N < eps[j].N })
	if len(eps) > 200 {
		eps = eps[:200]
	}
	return eps, nil
}

func (a *App) animeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/anime/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if strings.TrimSpace(q.Get("q")) == "" {
			fail(w, 400, "missing title")
			return
		}
		writeJSON(w, 200, map[string]any{"sites": animeSearch(q.Get("q"), q.Get("alt"))})
	})
	mux.HandleFunc("GET /api/anime/browse", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"sites": animeBrowse()})
	})
	mux.HandleFunc("GET /api/anime/episodes", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s := animeSiteByID(q.Get("site"))
		if s == nil {
			fail(w, 400, "unknown site")
			return
		}
		eps, err := animeEpisodes(s, q.Get("url"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if eps == nil {
			eps = []AnimeItem{}
		}
		writeJSON(w, 200, map[string]any{"episodes": eps})
	})
}
