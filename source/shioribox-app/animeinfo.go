package main

// Anime detail: everything a series page tells us (cover, synopsis, genres, studio, year and any "Label: value" facts)
// plus a complete episode list. Read tolerantly: meta tags, headings, taxonomy links and labelled lines.

import (
	"errors"
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

type AnimeFact struct {
	K string `json:"k"`
	V string `json:"v"`
}
type AnimeInfo struct {
	Site     string           `json:"site"`
	Title    string           `json:"title"`
	Poster   string           `json:"poster,omitempty"`
	Synopsis string           `json:"synopsis,omitempty"`
	Genres   []AnimeFilterOpt `json:"genres"`  // V = site path, L = name
	Studios  []AnimeFilterOpt `json:"studios"` // same
	Year     string           `json:"year,omitempty"`
	Facts    []AnimeFact      `json:"facts"`
	Episodes []AnimeItem      `json:"episodes"`
}

var (
	reAMeta     = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	reAMetaKey  = regexp.MustCompile(`(?is)\b(?:property|name)\s*=\s*["']([^"']+)["']`)
	reAMetaVal  = regexp.MustCompile(`(?is)\bcontent\s*=\s*["']([^"']*)["']`)
	reAH1       = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	reAPara     = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p>`)
	reAScript   = regexp.MustCompile(`(?is)<(?:script|style|noscript)\b.*?</(?:script|style|noscript)>`)
	reAHeading  = regexp.MustCompile(`(?is)<h[1-6]\b[^>]*>(.*?)</h[1-6]>`)
	reATitleCl  = regexp.MustCompile(`(?is)<(?:div|span|p|strong|b)\b[^>]*class\s*=\s*["'][^"']*\b(?:title|name|entry-title)\b[^"']*["'][^>]*>(.*?)</(?:div|span|p|strong|b)>`)
	reAEpTail   = regexp.MustCompile(`(?i)(?:episode|ep)?[-_ ]?(\d{1,4})/?$`)
	reAFactBare = regexp.MustCompile(`(?i)^(studio|released|release date|aired|status|duration|rating|censorship|source|country|season|original title|japanese)$`)
	reAFactLine = regexp.MustCompile(`(?i)^(studios?|released?|release date|year|aired|status|type|duration|episodes?|rating|score|censorship|censored|source|original title|japanese|alternative(?: titles?)?|synonyms|producers?|language|subtitles?|quality|country|season|views|uploaded|updated)\s*[:：]\s*(.*)$`)
)

func animeMetaContent(page, key string) string {
	for _, m := range reAMeta.FindAllString(page, -1) {
		k := reAMetaKey.FindStringSubmatch(m)
		if k == nil || !strings.EqualFold(k[1], key) {
			continue
		}
		if v := reAMetaVal.FindStringSubmatch(m); v != nil {
			return strings.TrimSpace(html.UnescapeString(v[1]))
		}
	}
	return ""
}

func animeSlugTitle(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(segs) == 0 {
		return ""
	}
	return strings.Title(strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(segs[len(segs)-1])), " "))
}

// animeCardTitle picks the real title out of a card's markup (a heading or "title" element); the tag and badge text
// that sits in the same link is ignored. ok is false when nothing reliable was found.
func animeCardTitle(inner string) (string, bool) {
	for _, re := range []*regexp.Regexp{reAHeading, reATitleCl} {
		if m := re.FindStringSubmatch(inner); m != nil {
			if t := unesc(m[1]); len([]rune(t)) > 1 && len(t) < 200 {
				return t, true
			}
		}
	}
	return "", false
}

func animeEpisodeNumber(u, text string) int {
	if m := reAEpNum.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if p, err := neturl.Parse(u); err == nil {
		if m := reAEpTail.FindStringSubmatch(strings.TrimSuffix(p.Path, "/") + "/"); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n
		}
	}
	return 0
}

// animeEpisodeLinks lists every /episodes/… link of a series page (whatever its link text) next to the generic reader.
// animeWatchLinks reads sites that list each episode as /watch/<id>/ (or /videos/<id>/), such as Aki-H,
// keeping the title the site gives each one ("So_low Vol 1 Sub-Eng").
func animeWatchLinks(s *animeSite, page string, baseU *neturl.URL) []AnimeItem {
	idx := map[string]int{}
	var out []AnimeItem
	for _, m := range reAAnchor.FindAllStringSubmatch(page, -1) {
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		u, err := baseU.Parse(html.UnescapeString(strings.TrimSpace(hm[1])))
		if err != nil || !animeSameHost(s, u.String()) {
			continue
		}
		segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		if len(segs) != 2 || !(strings.EqualFold(segs[0], "watch") || strings.EqualFold(segs[0], "videos")) {
			continue
		}
		u.Fragment, u.RawQuery = "", ""
		title := ""
		if tm := reATitle.FindStringSubmatch(m[1]); tm != nil {
			title = strings.TrimSpace(unesc(tm[1]))
		}
		if title == "" {
			title = strings.TrimSpace(unesc(regexp.MustCompile(`(?s)<[^>]*>`).ReplaceAllString(m[2], "")))
		}
		if reAGeneric.MatchString(title) {
			title = ""
		}
		key := u.String()
		if i, ok := idx[key]; ok {
			if out[i].Title == "" {
				out[i].Title = title
			}
			continue
		}
		idx[key] = len(out)
		out = append(out, AnimeItem{URL: key, Episode: true, Title: title, Site: s.id})
	}
	for i := range out {
		out[i].N = i + 1
		if out[i].Title == "" {
			out[i].Title = "Episode " + strconv.Itoa(i+1)
		}
	}
	return out
}

func animeEpisodeLinks(s *animeSite, page, self string) []AnimeItem {
	baseU, _ := neturl.Parse(s.base)
	if w := animeWatchLinks(s, page, baseU); len(w) > 0 {
		return w
	}
	seen := map[string]bool{self: true}
	var out []AnimeItem
	add := func(it AnimeItem) {
		if seen[it.URL] || !animeCardOK(it.URL) || !it.Episode {
			return
		}
		seen[it.URL] = true
		out = append(out, it)
	}
	for _, m := range reAAnchor.FindAllStringSubmatch(page, -1) {
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		u, err := baseU.Parse(html.UnescapeString(strings.TrimSpace(hm[1])))
		if err != nil || !animeSameHost(s, u.String()) {
			continue
		}
		epq := ""
		for _, k := range []string{"ep", "episode", "e"} {
			if v := u.Query().Get(k); v != "" && len(v) < 6 {
				epq = k + "=" + v
			}
		}
		u.Fragment, u.RawQuery = "", epq
		segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		if len(segs) < 2 || !(strings.EqualFold(segs[0], "episodes") || strings.EqualFold(segs[0], "episode")) {
			continue
		}
		n := animeEpisodeNumber(u.String(), unesc(m[2]))
		if epq != "" {
			n, _ = strconv.Atoi(strings.SplitN(epq, "=", 2)[1])
		}
		add(AnimeItem{URL: u.String(), Episode: true, N: n})
	}
	withQuery := map[string]bool{} // a page that is listed as ?ep=1, ?ep=2… is not also one extra "episode"
	for _, e := range out {
		if i := strings.IndexByte(e.URL, '?'); i > 0 {
			withQuery[e.URL[:i]] = true
		}
	}
	for _, it := range animeLinks(s, page) {
		if withQuery[it.URL] {
			continue
		}
		add(it)
	}
	// number the unnumbered ones after the numbered, in page order; then name them all "Episode N"
	next := 0
	for _, e := range out {
		if e.N > next {
			next = e.N
		}
	}
	for i := range out {
		if out[i].N == 0 {
			next++
			out[i].N = next
		}
		out[i].Site = s.id
		out[i].Title = "Episode " + strconv.Itoa(out[i].N)
		out[i].Thumb = ""
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].N < out[j].N })
	if len(out) > 300 {
		out = out[:300]
	}
	return out
}

var rePosterBox = regexp.MustCompile(`(?is)class\s*=\s*"[^"]*poster[^"]*"[^>]*>\s*(<img\b[^>]*>)`)

func animeInfoParse(s *animeSite, page, self string) AnimeInfo {
	info := AnimeInfo{Site: s.id, Genres: []AnimeFilterOpt{}, Studios: []AnimeFilterOpt{}, Facts: []AnimeFact{}}
	clean := reAScript.ReplaceAllString(page, "")
	if m := reAH1.FindStringSubmatch(clean); m != nil {
		info.Title = unesc(m[1])
	}
	if info.Title == "" {
		t := animeMetaContent(clean, "og:title")
		for _, sep := range []string{" | ", " – ", " - ", " — "} {
			if i := strings.Index(t, sep); i > 2 {
				t = t[:i]
			}
		}
		info.Title = t
	}
	if info.Title == "" {
		info.Title = animeSlugTitle(self)
	}
	if p := animeMetaContent(clean, "og:image"); p != "" {
		full := heAbs(s.base, p)
		heRegisterHost(full, s.base+"/")
		info.Poster = heProxy(full)
	}
	if info.Poster == "" {
		// a picture inside an element whose class says "poster" (Aki-H: ak-detail__poster) is the cover
		if m := rePosterBox.FindStringSubmatch(clean); m != nil {
			if src, _ := heImgSrc(m[1]); src != "" {
				full := heAbs(s.base, src)
				heRegisterHost(full, s.base+"/")
				info.Poster = heProxy(full)
			}
		}
	}
	if info.Poster == "" {
		host := strings.TrimPrefix(strings.ToLower(strings.TrimPrefix(s.base, "https://")), "www.")
		if i := strings.IndexByte(host, '.'); i > 0 {
			host = host[:i]
		}
		for _, im := range reHEImg.FindAllString(clean, 40) {
			src, _ := heImgSrc(im)
			low := strings.ToLower(src)
			if src == "" || !strings.Contains(low, host) && strings.HasPrefix(low, "http") || strings.Contains(low, "logo") || strings.Contains(low, "icon") || strings.Contains(low, "avatar") || strings.Contains(low, "banner") || !strings.Contains(low, "upload") {
				continue
			}
			full := heAbs(s.base, src)
			heRegisterHost(full, s.base+"/")
			info.Poster = heProxy(full)
			break
		}
	}
	// synopsis: the longest of the meta description and the page's paragraphs
	best := animeMetaContent(clean, "og:description")
	if d := animeMetaContent(clean, "description"); len(d) > len(best) {
		best = d
	}
	for _, m := range reAPara.FindAllStringSubmatch(clean, -1) {
		t := unesc(m[1])
		if len(t) >= 80 && len(t) <= 3000 && len(t) > len(best) && !strings.Contains(strings.ToLower(t), "cookie") {
			best = t
		}
	}
	info.Synopsis = best
	// taxonomy links
	baseU, _ := neturl.Parse(s.base)
	gs, ss := map[string]bool{}, map[string]bool{}
	for _, m := range reAAnchor.FindAllStringSubmatch(clean, -1) {
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		u, err := baseU.Parse(html.UnescapeString(strings.TrimSpace(hm[1])))
		if err != nil || !animeSameHost(s, u.String()) {
			continue
		}
		segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		name := unesc(m[2])
		if len(segs) < 2 || name == "" || len(name) > 60 {
			continue
		}
		path := "/" + strings.Join(segs, "/") + "/"
		switch strings.ToLower(segs[0]) {
		case "genre", "genres", "tag", "tags":
			if !gs[path] && len(info.Genres) < 30 {
				gs[path] = true
				info.Genres = append(info.Genres, AnimeFilterOpt{path, name})
			}
		case "studio", "studios", "producer", "producers", "brand":
			if !ss[path] && len(info.Studios) < 10 {
				ss[path] = true
				info.Studios = append(info.Studios, AnimeFilterOpt{path, name})
			}
		case "year", "years", "release":
			if info.Year == "" {
				info.Year = name
			}
		}
	}
	// "Label: value" lines (value on the same or the next line)
	var lines []string
	for _, l := range strings.Split(reAnyTag.ReplaceAllString(strings.NewReplacer("</", "\n</", "<br", "\n<br").Replace(clean), "\n"), "\n") {
		if l = strings.TrimSpace(html.UnescapeString(l)); l != "" {
			lines = append(lines, strings.Join(strings.Fields(l), " "))
		}
	}
	have := map[string]bool{}
	for i, l := range lines {
		m := reAFactLine.FindStringSubmatch(l)
		if m == nil {
			if b := reAFactBare.FindStringSubmatch(l); b != nil && i+1 < len(lines) && len(lines[i+1]) <= 60 {
				m = []string{l, b[1], ""}
			} else {
				continue
			}
		}
		k, v := strings.Title(strings.ToLower(m[1])), strings.TrimSpace(m[2])
		if v == "" && i+1 < len(lines) {
			v = lines[i+1]
		}
		if v == "" || len(v) > 160 || have[strings.ToLower(k)] || reAFactLine.MatchString(v) {
			continue
		}
		have[strings.ToLower(k)] = true
		info.Facts = append(info.Facts, AnimeFact{k, v})
		if len(info.Facts) >= 16 {
			break
		}
	}
	info.Episodes = animeEpisodeLinks(s, clean, self)
	if info.Episodes == nil {
		info.Episodes = []AnimeItem{}
	}
	return info
}

var (
	animeInfoMu    sync.Mutex
	animeInfoCache = map[string]struct {
		at   time.Time
		info AnimeInfo
	}{}
)

func animeInfoFor(s *animeSite, pageURL string) (AnimeInfo, error) {
	if !animeSameHost(s, pageURL) {
		return AnimeInfo{}, errors.New("that page isn't on " + s.name)
	}
	animeInfoMu.Lock()
	if c, ok := animeInfoCache[pageURL]; ok && time.Since(c.at) < 10*time.Minute {
		animeInfoMu.Unlock()
		return c.info, nil
	}
	animeInfoMu.Unlock()
	b, err := animeGet(s, pageURL)
	if err != nil {
		return AnimeInfo{}, err
	}
	info := animeInfoParse(s, string(b), pageURL)
	animeInfoMu.Lock()
	animeInfoCache[pageURL] = struct {
		at   time.Time
		info AnimeInfo
	}{time.Now(), info}
	animeInfoMu.Unlock()
	return info, nil
}

func (a *App) animeInfoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/anime/info", func(w http.ResponseWriter, r *http.Request) {
		var info AnimeInfo
		var err error
		if r.URL.Query().Get("site") == hstreamID {
			info, err = hstreamInfo(r.URL.Query().Get("url"))
		} else {
			s := animeSiteByID(r.URL.Query().Get("site"))
			if s == nil {
				s = animeSites[0]
			}
			info, err = animeInfoFor(s, r.URL.Query().Get("url"))
		}
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, info)
	})
}
