package main

// HentaiEra and HentaiEnvy: two sites built on the same gallery engine. Search is /search/?key=..., a gallery lives at
// /gallery/<id>/ and lists small previews ("1t.jpg"); the full picture has the same address without the "t".
// Everything is read with tolerant patterns, so a small layout change on the sites does not break it.

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type heSite struct{ id, name, base string }

var heSites = map[string]*heSite{
	"hentaiera":  {"hentaiera", "HentaiEra", "https://hentaiera.com"},
	"hentaienvy": {"hentaienvy", "HentaiEnvy", "https://hentaienvy.com"},
}

var (
	heMu    sync.Mutex
	heHosts = map[string]string{} // image host -> Referer to send
)

// heRegisterHost remembers an image host seen on one of these sites so the image proxy may fetch from it.
func heRegisterHost(rawURL, referer string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return
	}
	heMu.Lock()
	heHosts[strings.ToLower(u.Hostname())] = referer
	heMu.Unlock()
}

// heHostReferer reports whether the host is one of these sites (or an image host found on them).
func heHostReferer(host string) (string, bool) {
	host = strings.ToLower(host)
	heMu.Lock()
	defer heMu.Unlock()
	if r, ok := heHosts[host]; ok {
		return r, true
	}
	for _, s := range heSites {
		d := strings.TrimPrefix(s.base, "https://")
		if host == d || strings.HasSuffix(host, "."+d) {
			return s.base + "/", true
		}
	}
	return "", false
}

var (
	reHEGal   = regexp.MustCompile(`(?is)<a\b[^>]*\bhref="(?:https?://[^"/]+)?/gallery/(\d+)/?"[^>]*>`)
	reHEImg   = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	reHEAttr  = regexp.MustCompile(`(?is)\b(data-src|data-original|src|alt|title)\s*=\s*"([^"]*)"`)
	reHECap   = regexp.MustCompile(`(?is)<(?:h\d|div|span|p)[^>]*class="[^"]*(?:caption|title|name)[^"]*"[^>]*>(.*?)</(?:h\d|div|span|p)>`)
	reHEPages = regexp.MustCompile(`(?i)(\d+)\s*pages`)
	reHEH1    = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	reHEThumb = regexp.MustCompile(`(?i)/(\d+)t\.(jpe?g|png|webp|gif|avif)(?:[?#].*)?$`)
)

func heAbs(base, ref string) string {
	ref = html.UnescapeString(strings.TrimSpace(ref))
	switch {
	case strings.HasPrefix(ref, "//"):
		return "https:" + ref
	case strings.HasPrefix(ref, "/"):
		return base + ref
	}
	return ref
}

func heImgSrc(tag string) (src, alt string) {
	for _, m := range reHEAttr.FindAllStringSubmatch(tag, -1) {
		v := m[2]
		switch strings.ToLower(m[1]) {
		case "alt", "title":
			if alt == "" {
				alt = v
			}
		default:
			if v != "" && !strings.HasPrefix(v, "data:") && (src == "" || strings.ToLower(m[1]) != "src") {
				src = v
			}
		}
	}
	return
}

func heProxy(u string) string { return "/api/cg/img?t=" + url.QueryEscape(u) }

// heFull is the address of a full picture. When the guessed address fails the proxy asks the site's own viewer page
// for the real one ("v"), and as a last resort shows the preview ("th") so the viewer is never empty.
func heFull(s *heSite, ref string, n int, full, thumb string, guess bool) string {
	u := heProxy(full)
	if guess {
		u += "&e=jpg,png,webp,gif"
	}
	return u + fmt.Sprintf("&v=%s:%s:%d&th=%s", s.id, ref, n, url.QueryEscape(thumb))
}

var reHEMain = regexp.MustCompile(`(?is)<img\b[^>]*\bid="(?:gimg|main-image|image)"[^>]*>`)

// heResolveView reads /view/<id>/<n>/ and returns the picture address shown there.
func heResolveView(s *heSite, ref string, n int) (string, error) {
	b, _, err := cgGet(fmt.Sprintf("%s/view/%s/%d/", s.base, ref, n), map[string]string{"Referer": s.base + "/gallery/" + ref + "/"}, cgMaxPageBody)
	if err != nil {
		return "", err
	}
	page := string(b)
	var cands []string
	if tag := reHEMain.FindString(page); tag != "" {
		if src, _ := heImgSrc(tag); src != "" {
			cands = append(cands, src)
		}
	}
	for _, tag := range reHEImg.FindAllString(page, -1) {
		if src, _ := heImgSrc(tag); src != "" && regexp.MustCompile(`(?i)/\d+\.(jpe?g|png|webp|gif|avif)(?:[?#].*)?$`).MatchString(src) {
			cands = append(cands, src)
		}
	}
	if len(cands) == 0 {
		return "", errors.New("no picture on the viewer page")
	}
	u := heAbs(s.base, cands[0])
	heRegisterHost(u, s.base+"/")
	return u, nil
}

// heProxyGuess is heProxy for an address built by pattern: the proxy also tries the other usual file extensions.
func heProxyGuess(u string) string { return heProxy(u) + "&e=jpg,png,webp,gif" }

func heParseList(s *heSite, page string, cur int) (items []CGItem, more bool) {
	locs := reHEGal.FindAllStringSubmatchIndex(page, -1)
	seen := map[string]bool{}
	for i, l := range locs {
		id := page[l[2]:l[3]]
		if seen[id] {
			continue
		}
		seen[id] = true
		end := len(page)
		for _, n := range locs[i+1:] {
			if page[n[2]:n[3]] != id {
				end = n[0]
				break
			}
		}
		seg := page[l[0]:end]
		it := CGItem{Ref: id}
		var alt string
		if im := reHEImg.FindString(seg); im != "" {
			src, a := heImgSrc(im)
			alt = a
			if src != "" {
				full := heAbs(s.base, src)
				heRegisterHost(full, s.base+"/")
				it.Thumb = heProxy(full)
			}
		}
		if m := reHECap.FindStringSubmatch(seg); m != nil {
			it.Title = unesc(m[1])
		}
		if it.Title == "" {
			if t := regexp.MustCompile(`(?is)\btitle="([^"]+)"`).FindStringSubmatch(page[l[0]:l[1]]); t != nil {
				it.Title = unesc(t[1])
			}
		}
		if it.Title == "" {
			it.Title = unesc(alt)
		}
		if it.Title == "" {
			it.Title = "Gallery " + id
		}
		if p := reHEPages.FindStringSubmatch(unesc(seg)); p != nil {
			it.Pages, _ = strconv.Atoi(p[1])
		}
		items = append(items, it)
	}
	more = regexp.MustCompile(`[?&]page=` + strconv.Itoa(cur+2) + `\b`).MatchString(page)
	return
}

// heSearch: page is 0-based here, the sites count from 1.
func heSearch(s *heSite, q string, page int) (*CGSearchResult, error) {
	q = strings.TrimSpace(q)
	var items []CGItem
	var more bool
	var lastErr error
	for _, key := range []string{"key", "q"} {
		v := url.Values{}
		v.Set(key, q)
		if page > 0 {
			v.Set("page", strconv.Itoa(page+1))
		}
		b, _, err := cgGet(s.base+"/search/?"+v.Encode(), map[string]string{"Referer": s.base + "/"}, cgMaxPageBody)
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		if items, more = heParseList(s, string(b), page); len(items) > 0 {
			break
		}
	}
	if lastErr != nil && len(items) == 0 {
		return nil, fmt.Errorf("%s: %w", s.name, lastErr)
	}
	res := &CGSearchResult{Site: s.id, Items: items, Next: -1}
	if more {
		res.Next = page + 1
	}
	return res, nil
}

func heParseGallery(s *heSite, ref, page string) *CGGallery {
	g := &CGGallery{Site: s.id, Ref: ref}
	if m := reHEH1.FindStringSubmatch(page); m != nil {
		g.Title = unesc(m[1])
	}
	byN := map[int]CGImage{}
	maxN := 0
	for _, tag := range reHEImg.FindAllString(page, -1) {
		src, _ := heImgSrc(tag)
		if src == "" {
			continue
		}
		m := reHEThumb.FindStringSubmatch(src)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n <= 0 || n > 20000 {
			continue
		}
		if _, dup := byN[n]; dup {
			continue
		}
		thumb := heAbs(s.base, src)
		full := reHEThumb.ReplaceAllString(thumb, "/"+m[1]+"."+m[2])
		// the query string, if any, is dropped from the full address
		if i := strings.IndexAny(full, "?#"); i >= 0 {
			full = full[:i]
		}
		heRegisterHost(thumb, s.base+"/")
		byN[n] = CGImage{N: n, URL: heFull(s, ref, n, full, thumb, false), Thumb: heProxy(thumb)}
		if n > maxN {
			maxN = n
		}
	}
	for n := 1; n <= maxN; n++ {
		if im, ok := byN[n]; ok {
			g.Images = append(g.Images, im)
		}
	}
	g.Count = len(g.Images)
	defer func() {
		// The page lists only the first few previews, but every picture sits next to them under a number:
		// build the rest from the same folder so the whole gallery is there.
		if g.Count <= len(g.Images) || len(g.Images) == 0 {
			return
		}
		first := byN[g.Images[0].N]
		tu, err := url.QueryUnescape(strings.TrimPrefix(first.Thumb, "/api/cg/img?t="))
		if err != nil {
			return
		}
		m := reHEThumb.FindStringSubmatch(tu)
		if m == nil {
			return
		}
		dir := tu[:strings.LastIndex(tu, "/")+1]
		ext := m[2]
		all := make([]CGImage, 0, g.Count)
		for n := 1; n <= g.Count; n++ {
			if im, ok := byN[n]; ok {
				all = append(all, im)
				continue
			}
			all = append(all, CGImage{N: n,
				URL:   heFull(s, ref, n, fmt.Sprintf("%s%d.%s", dir, n, ext), fmt.Sprintf("%s%dt.%s", dir, n, ext), true),
				Thumb: heProxyGuess(fmt.Sprintf("%s%dt.%s", dir, n, ext))})
		}
		g.Images, g.PerPage = all, len(all)
	}()
	if m := regexp.MustCompile(`(?i)pages?\s*:?\s*(?:</[^>]+>\s*)*(?:<[^>]+>\s*)*(\d+)`).FindStringSubmatch(page); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > g.Count && n <= 20000 {
			g.Count = n
		}
	}
	g.PerPage = len(g.Images)
	return g
}

func heGallery(s *heSite, ref string) (*CGGallery, error) {
	if _, err := strconv.Atoi(ref); err != nil || ref == "" {
		return nil, errors.New("bad gallery reference")
	}
	b, _, err := cgGet(s.base+"/gallery/"+ref+"/", map[string]string{"Referer": s.base + "/"}, cgMaxPageBody)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.name, err)
	}
	g := heParseGallery(s, ref, string(b))
	if len(g.Images) == 0 {
		return nil, fmt.Errorf("%s: no pictures found on that gallery page (the site layout may have changed)", s.name)
	}
	return g, nil
}
