package main

// Anime page: a browsable grid (latest, search, the site's own genre/studio/year filters, a random pick) built from
// the listing pages of the anime site. Nothing here assumes one exact layout: filter fields are read from the
// site's own filter form, "next page" from the page's own next link, and cards from the same tolerant link reader the
// Anime tab uses.

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AnimeFilterOpt struct {
	V string `json:"v"`
	L string `json:"l"`
}
type AnimeFilter struct {
	Name    string           `json:"name"`
	Label   string           `json:"label"`
	Options []AnimeFilterOpt `json:"options"`
}

var (
	reAForm   = regexp.MustCompile(`(?is)<form\b([^>]*)>(.*?)</form>`)
	reASelect = regexp.MustCompile(`(?is)<select\b([^>]*)>(.*?)</select>`)
	reAOption = regexp.MustCompile(`(?is)<option\b([^>]*)>(.*?)</option>`)
	reAName   = regexp.MustCompile(`(?is)\bname\s*=\s*["']([^"']+)["']`)
	reAValue  = regexp.MustCompile(`(?is)\bvalue\s*=\s*["']([^"']*)["']`)
	reAAction = regexp.MustCompile(`(?is)\baction\s*=\s*["']([^"']*)["']`)
	reARel    = regexp.MustCompile(`(?is)\brel\s*=\s*["']([^"']*)["']`)
	reAClass  = regexp.MustCompile(`(?is)\bclass\s*=\s*["']([^"']*)["']`)
	reANextTx = regexp.MustCompile(`(?i)^(?:next(?: page)?|older|show more|load more|»|›|>>?)$`)
	reAFName  = regexp.MustCompile(`^[A-Za-z0-9_\-\[\]]{1,40}$`)
)

var animeTaxo = map[string]bool{"genre": true, "genres": true, "tag": true, "tags": true, "browse": true, "page": true, "studio": true,
	"studios": true, "year": true, "years": true, "category": true, "author": true, "random": true, "wp-content": true, "wp-json": true,
	"feed": true, "search": true, "az-list": true, "login": true, "register": true, "contact": true, "dmca": true, "privacy": true}

// animeCardOK keeps links that look like a series or an episode (/something/slug/) and drops menu links such as
// /hentai/, /browse/, /random/, /genre/x/ and /page/2/.
func animeCardOK(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(segs) < 2 || animeTaxo[strings.ToLower(segs[0])] {
		return false
	}
	for _, s := range segs {
		if strings.EqualFold(s, "page") {
			return false
		}
	}
	return true
}

func animeSameHost(s *animeSite, raw string) bool {
	u, err := neturl.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") == strings.TrimPrefix(strings.TrimPrefix(s.base, "https://"), "www.")
}

func animeNextLink(s *animeSite, page, cur string) string {
	baseU, _ := neturl.Parse(s.base)
	for _, m := range reAAnchor.FindAllStringSubmatch(page, -1) {
		hm := reAHref.FindStringSubmatch(m[1])
		if hm == nil {
			continue
		}
		rel, cls := "", ""
		if r := reARel.FindStringSubmatch(m[1]); r != nil {
			rel = strings.ToLower(r[1])
		}
		if c := reAClass.FindStringSubmatch(m[1]); c != nil {
			cls = strings.ToLower(c[1])
		}
		txt := unesc(m[2])
		if !(strings.Contains(rel, "next") || strings.Contains(cls, "next") || reANextTx.MatchString(txt)) {
			continue
		}
		u, err := baseU.Parse(html.UnescapeString(strings.TrimSpace(hm[1])))
		if err != nil || !animeSameHost(s, u.String()) {
			continue
		}
		u.Fragment = ""
		if u.String() != cur {
			return u.String()
		}
	}
	return ""
}

// animeFilters reads the selects of the site's filter form (genre, studio, year, sort…).
func animeFilters(s *animeSite) (action string, fields []AnimeFilter, err error) {
	b, err := animeGet(s, s.base+"/browse/")
	if err != nil {
		return "", nil, err
	}
	page := string(b)
	scan := page
	action = "/browse/"
	for _, f := range reAForm.FindAllStringSubmatch(page, -1) {
		if reASelect.MatchString(f[2]) {
			scan = f[2]
			if a := reAAction.FindStringSubmatch(f[1]); a != nil && strings.TrimSpace(a[1]) != "" {
				action = html.UnescapeString(strings.TrimSpace(a[1]))
			}
			break
		}
	}
	for _, sm := range reASelect.FindAllStringSubmatch(scan, -1) {
		nm := reAName.FindStringSubmatch(sm[1])
		if nm == nil || !reAFName.MatchString(nm[1]) {
			continue
		}
		fl := AnimeFilter{Name: nm[1], Label: nm[1]}
		for _, om := range reAOption.FindAllStringSubmatch(sm[2], -1) {
			label := unesc(om[2])
			v := ""
			if vm := reAValue.FindStringSubmatch(om[1]); vm != nil {
				v = html.UnescapeString(vm[1])
			} else {
				v = label
			}
			if v == "" {
				if label != "" {
					fl.Label = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(label, "All "), "Any "))
				}
				continue
			}
			if len(fl.Options) < 400 && len(v) <= 80 && label != "" {
				fl.Options = append(fl.Options, AnimeFilterOpt{v, label})
			}
		}
		if len(fl.Options) > 0 {
			fields = append(fields, fl)
		}
	}
	return action, fields, nil
}

var (
	animeOptMu sync.Mutex
	animeOptAt time.Time
	animeOptA  string
	animeOptF  []AnimeFilter
)

func animeOptions(s *animeSite) (string, []AnimeFilter, error) {
	animeOptMu.Lock()
	defer animeOptMu.Unlock()
	if animeOptF != nil && time.Since(animeOptAt) < time.Hour {
		return animeOptA, animeOptF, nil
	}
	a, f, err := animeFilters(s)
	if err != nil {
		return "", nil, err
	}
	if f == nil {
		f = []AnimeFilter{}
	}
	animeOptA, animeOptF, animeOptAt = a, f, time.Now()
	return a, f, nil
}

type animeListOut struct {
	Items []AnimeItem `json:"items"`
	Next  string      `json:"next"`
	URL   string      `json:"url"`
	Count int         `json:"links"`
}

func animeListPage(s *animeSite, target string) (animeListOut, error) {
	out := animeListOut{Items: []AnimeItem{}, URL: target}
	b, err := animeGet(s, target)
	if err != nil {
		return out, err
	}
	links := animeLinks(s, string(b))
	out.Count = len(links)
	seen := map[string]bool{}
	for _, it := range links {
		if seen[it.URL] || !s.cardOK(it.URL) {
			continue
		}
		seen[it.URL] = true
		it.Site = s.id
		out.Items = append(out.Items, it)
	}
	if len(out.Items) > 120 {
		out.Items = out.Items[:120]
	}
	out.Next = animeNextLink(s, string(b), target)
	if out.Next == "" && s.cfg != nil && s.cfg.pageQ != "" && len(out.Items) > 0 {
		if u, err := neturl.Parse(target); err == nil {
			q := u.Query()
			n, _ := strconv.Atoi(q.Get(s.cfg.pageQ))
			if n < 1 {
				n = 1
			}
			q.Set(s.cfg.pageQ, strconv.Itoa(n+1))
			u.RawQuery = q.Encode()
			out.Next = u.String()
		}
	}
	return out, nil
}

func animeRandom(s *animeSite) (AnimeItem, error) {
	if it, err := animeRandomRedirect(s); err == nil {
		return it, nil
	}
	// The site's own /random/ isn't reachable this way: pick one from a random page of the series list.
	pageOf := func(u string) (animeListOut, error) { return animeListPage(s, u) }
	lp := s.latestPaths()[0]
	first, err := pageOf(s.base + lp)
	if err != nil || len(first.Items) == 0 {
		return AnimeItem{}, errors.New("the site's random pick isn't available right now")
	}
	max := 1
	if b, err := animeGet(s, s.base+lp); err == nil {
		for _, m := range reAPageNum.FindAllStringSubmatch(string(b), -1) {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
	}
	items := first.Items
	if max > 1 {
		if max > 60 {
			max = 60
		}
		p, _ := crand.Int(crand.Reader, big.NewInt(int64(max)))
		if n := int(p.Int64()) + 1; n > 1 {
			pu := fmt.Sprintf("%s%spage/%d/", s.base, lp, n)
			if s.cfg != nil && s.cfg.pageQ != "" {
				pu = fmt.Sprintf("%s%s?%s=%d", s.base, lp, s.cfg.pageQ, n)
			}
			if pg, err := pageOf(pu); err == nil && len(pg.Items) > 0 {
				items = pg.Items
			}
		}
	}
	i, _ := crand.Int(crand.Reader, big.NewInt(int64(len(items))))
	return items[i.Int64()], nil
}

var reAPageNum = regexp.MustCompile(`/page/(\d+)/`)

func animeRandomRedirect(s *animeSite) (AnimeItem, error) {
	c := *cgHTTP
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequest("GET", s.base+s.randomPath(), nil)
	if err != nil {
		return AnimeItem{}, err
	}
	req.Header.Set("User-Agent", cgUA)
	req.Header.Set("Referer", s.base+"/")
	resp, err := c.Do(req)
	if err != nil {
		return AnimeItem{}, err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return AnimeItem{}, errors.New("the site's random pick isn't available right now")
	}
	baseU, _ := neturl.Parse(s.base)
	u, err := baseU.Parse(loc)
	if err != nil || !animeSameHost(s, u.String()) || !s.cardOK(u.String()) {
		return AnimeItem{}, errors.New("the site's random pick isn't available right now")
	}
	u.Fragment, u.RawQuery = "", ""
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	name := strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(segs[len(segs)-1])), " ")
	it := AnimeItem{Title: strings.Title(name), URL: u.String(), Site: s.id}
	if m := reAEpURL.FindStringSubmatch(it.URL); m != nil {
		fmt.Sscan(m[1], &it.N)
	}
	it.Episode = it.N > 0 || strings.Contains(strings.ToLower(it.URL), "episode")
	return it, nil
}

func (a *App) animeListRoutes(mux *http.ServeMux) {
	siteFor := func(r *http.Request) *animeSite {
		if s := animeSiteByID(r.URL.Query().Get("site")); s != nil {
			return s
		}
		return animeSites[0]
	}
	mux.HandleFunc("GET /api/anime/options", func(w http.ResponseWriter, r *http.Request) {
		act, f, err := animeOptions(animeSites[0])
		if err != nil {
			writeJSON(w, 200, map[string]any{"action": "", "fields": []AnimeFilter{}, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"action": act, "fields": f})
	})
	mux.HandleFunc("GET /api/anime/list", func(w http.ResponseWriter, r *http.Request) {
		s, q := siteFor(r), r.URL.Query()
		if q.Get("site") == hstreamID {
			page := 1
			if q.Get("kind") == "url" {
				page, _ = strconv.Atoi(q.Get("v"))
			}
			text, tag := "", strings.TrimSpace(q.Get("tags"))
			if q.Get("kind") == "search" {
				text = strings.TrimSpace(q.Get("q"))
			}
			items, next, err := hstreamList(text, tag, page)
			if err != nil {
				fail(w, 502, err.Error())
				return
			}
			writeJSON(w, 200, animeListOut{Items: items, Next: next, URL: hstreamListURL(text, tag, page), Count: len(items)})
			return
		}
		var targets []string
		switch q.Get("kind") {
		case "search":
			if strings.TrimSpace(q.Get("q")) == "" {
				fail(w, 400, "type something to search for")
				return
			}
			for _, p := range s.searchPaths(strings.TrimSpace(q.Get("q"))) {
				targets = append(targets, s.base+p)
			}
		case "tag":
			t := strings.TrimSpace(q.Get("tags"))
			if t == "" || len(t) > 60 {
				fail(w, 400, "missing tag")
				return
			}
			targets = []string{s.base + s.genrePath(t)}
		case "browse":
			act, fields, err := animeOptions(s)
			if err != nil {
				fail(w, 502, err.Error())
				return
			}
			known := map[string]bool{}
			for _, f := range fields {
				known[f.Name] = true
			}
			baseU, _ := neturl.Parse(s.base)
			u, err := baseU.Parse(act)
			if err != nil || !animeSameHost(s, u.String()) {
				fail(w, 502, "the site's filter address is not usable")
				return
			}
			vals := neturl.Values{}
			for _, f := range q["f"] {
				if i := strings.IndexByte(f, ':'); i > 0 && known[f[:i]] && len(f) < 120 {
					vals.Set(f[:i], f[i+1:])
				}
			}
			u.RawQuery = vals.Encode()
			targets = []string{u.String()}
		case "url":
			if !animeSameHost(s, q.Get("v")) {
				fail(w, 400, "bad page address")
				return
			}
			targets = []string{q.Get("v")}
		default:
			for _, p := range s.latestPaths() {
				targets = append(targets, s.base+p)
			}
		}
		var out animeListOut
		var err error
		for _, t := range targets {
			out, err = animeListPage(s, t)
			if err == nil && len(out.Items) > 0 {
				break
			}
		}
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, out)
	})
	// A plain-text report of what ShioriBox gets from a page (for fixing a site that doesn't read right).
	mux.HandleFunc("GET /api/anime/dump", func(w http.ResponseWriter, r *http.Request) {
		s := siteFor(r)
		if r.URL.Query().Get("site") == hstreamID {
			s = hstreamSite
		}
		raw := r.URL.Query().Get("url")
		if !animeSameHost(s, raw) {
			fail(w, 400, "that page isn't on "+s.name)
			return
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "ShioriBox %s page report\nsite: %s\nurl: %s\n\n", version, s.name, raw)
		b, err := animeGet(s, raw)
		if err != nil {
			fmt.Fprintf(&sb, "FETCH ERROR: %v\n", err)
		} else {
			fmt.Fprintf(&sb, "fetched %d bytes\n", len(b))
			if info := animeInfoParse(s, string(b), raw); true {
				fmt.Fprintf(&sb, "details: title=%q poster=%v synopsis=%d chars genres=%d studios=%d facts=%d episodes=%d\n", info.Title, info.Poster != "", len(info.Synopsis), len(info.Genres), len(info.Studios), len(info.Facts), len(info.Episodes))
				for _, e := range info.Episodes {
					fmt.Fprintf(&sb, "  episode %d: %s\n", e.N, e.URL)
				}
			}
			if s != hstreamSite {
				src, embeds, rerr := animeResolve(s, raw)
				fmt.Fprintf(&sb, "player: %d sources, %d embeds, err=%v\n", len(src), len(embeds), rerr)
				for _, x := range src {
					fmt.Fprintf(&sb, "  source: %s (%s)\n", x.URL, x.Type)
				}
				for _, x := range embeds {
					fmt.Fprintf(&sb, "  embed: %s\n", x)
				}
			}
			body := b
			if len(body) > 700<<10 {
				body = body[:700<<10]
			}
			sb.WriteString("\n===== page source =====\n")
			sb.Write(body)
			// the site's own scripts (not libraries) often hold the way the player gets its video
			if base, e := neturl.Parse(raw); e == nil {
				n := 0
				for _, m := range regexp.MustCompile(`(?is)<script\b[^>]*?\bsrc\s*=\s*"([^"]+)"`).FindAllStringSubmatch(string(b), -1) {
					su, e := base.Parse(html.UnescapeString(m[1]))
					if e != nil {
						continue
					}
					low := strings.ToLower(su.String())
					if n >= 3 || !animeSameHost(s, su.String()) || !strings.Contains(low, ".js") || strings.Contains(low, "jquery") || strings.HasSuffix(strings.SplitN(low, "?", 2)[0], "/ui.js") {
						continue
					}
					js, e := animeGet(s, su.String())
					n++
					if e != nil {
						fmt.Fprintf(&sb, "\n===== script %s: %v =====\n", su.String(), e)
						continue
					}
					if len(js) > 150<<10 {
						js = js[:150<<10]
					}
					fmt.Fprintf(&sb, "\n===== script %s =====\n", su.String())
					sb.Write(js)
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="shioribox-page-report.txt"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("GET /api/anime/random", func(w http.ResponseWriter, r *http.Request) {
		var it AnimeItem
		var err error
		if r.URL.Query().Get("site") == hstreamID {
			it, err = hstreamRandom()
		} else {
			it, err = animeRandom(siteFor(r))
		}
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"item": it})
	})
}
