package main

// In-app player support. For an episode page ShioriBox looks for the video the page plays (direct mp4/webm files, HLS
// playlists, or the same inside the page's embedded players) and hands the browser a short-lived token per file. The
// browser only ever talks to /api/anime/stream?k=<token>; this side adds the Referer the video host wants, passes
// Range requests through so seeking works, and rewrites HLS playlists so every segment goes the same way.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type streamEntry struct {
	url, referer string
	at           time.Time
}

var (
	streamMu   sync.Mutex
	streamToks = map[string]streamEntry{}
	streamTTL  = 4 * time.Hour
	streamHTTP = &http.Client{Transport: streamTransport()}
)

func streamTransport() *http.Transport {
	t := cgTransport()
	t.ResponseHeaderTimeout = 30 * time.Second
	return t
}

func streamToken(u, referer string) string {
	var b [12]byte
	rand.Read(b[:])
	k := hex.EncodeToString(b[:])
	streamMu.Lock()
	defer streamMu.Unlock()
	if len(streamToks) > 6000 {
		for key, e := range streamToks {
			if time.Since(e.at) > streamTTL {
				delete(streamToks, key)
			}
		}
		if len(streamToks) > 6000 { // still full: start over rather than grow without limit
			streamToks = map[string]streamEntry{}
		}
	}
	streamToks[k] = streamEntry{u, referer, time.Now()}
	return k
}

func streamLookup(k string) (streamEntry, bool) {
	streamMu.Lock()
	defer streamMu.Unlock()
	e, ok := streamToks[k]
	if !ok || time.Since(e.at) > streamTTL {
		return streamEntry{}, false
	}
	return e, true
}

type AnimeSource struct {
	Type  string `json:"type"` // "mp4" or "hls"
	URL   string `json:"url"`  // /api/anime/stream?k=...
	Label string `json:"label"`
}

var (
	reSMedia  = regexp.MustCompile(`(?i)(?:https?:)?//[^\s"'<>\\()]+?\.(?:m3u8|mp4|webm)(?:\?[^\s"'<>\\()]*)?`)
	reSAttr   = regexp.MustCompile(`(?is)<(?:source|video)\b[^>]*?\bsrc\s*=\s*["']([^"']+)["']`)
	reSFile   = regexp.MustCompile(`(?i)["']?(?:file|src|source|url|video_url|hls)["']?\s*[:=]\s*["']([^"']+\.(?:m3u8|mp4|webm)(?:\?[^"']*)?)["']`)
	reSIframe = regexp.MustCompile(`(?is)<iframe\b[^>]*?\b(?:data-lazy-src|data-src|src)\s*=\s*["']([^"']+)["']`)
)

func cleanPageText(s string) string {
	s = strings.NewReplacer(`\/`, `/`, `&`, `&`, `/`, `/`, `=`, `=`, `&amp;`, `&`).Replace(s)
	return s
}

// findMedia returns the video files and embedded players mentioned in a page, as absolute URLs.
func findMedia(pageURL, page string) (media, iframes []string) {
	base, err := neturl.Parse(pageURL)
	if err != nil {
		return
	}
	page = cleanPageText(page)
	seen := map[string]bool{}
	add := func(list *[]string, raw string) {
		raw = html.UnescapeString(strings.TrimSpace(raw))
		if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "blob:") || strings.HasPrefix(raw, "about:") {
			return
		}
		u, err := base.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return
		}
		s := u.String()
		if !seen[s] {
			seen[s] = true
			*list = append(*list, s)
		}
	}
	for _, m := range reSAttr.FindAllStringSubmatch(page, -1) {
		add(&media, m[1])
	}
	for _, m := range reSFile.FindAllStringSubmatch(page, -1) {
		add(&media, m[1])
	}
	for _, m := range reSMedia.FindAllString(page, -1) {
		add(&media, m)
	}
	for _, m := range reSIframe.FindAllStringSubmatch(page, -1) {
		add(&iframes, m[1])
	}
	return
}

func mediaType(u string) string {
	p := strings.ToLower(u)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if strings.HasSuffix(p, ".m3u8") {
		return "hls"
	}
	return "mp4"
}

// animeResolve finds playable sources for an episode page: the page itself, then the players embedded in it (two levels).
func animeResolve(s *animeSite, pageURL string) (sources []AnimeSource, embeds []string, err error) {
	pu, e := neturl.Parse(pageURL)
	if e != nil || pu.Scheme != "https" || strings.TrimPrefix(strings.ToLower(pu.Hostname()), "www.") != strings.TrimPrefix(strings.TrimPrefix(s.base, "https://"), "www.") {
		return nil, nil, fmt.Errorf("that page isn't on %s", s.name)
	}
	b, e := animeGet(s, pageURL)
	if e != nil {
		return nil, nil, fmt.Errorf("%s: %w", s.name, e)
	}
	seen := map[string]bool{}
	type cand struct{ url, referer string }
	var cands []cand
	var cmu sync.Mutex
	addSrc := func(u, referer string) {
		cmu.Lock()
		defer cmu.Unlock()
		if seen[u] || looksLikeAd(u) {
			return
		}
		if pu, e := cgPublicURL(u); e != nil || pu == nil {
			return
		}
		seen[u] = true
		cands = append(cands, cand{u, referer})
	}
	media, frames := findMedia(pageURL, string(b))
	for _, m := range media {
		addSrc(m, pageURL)
	}
	type job struct{ url, referer string }
	var queue []job
	for _, f := range frames {
		queue = append(queue, job{f, pageURL})
	}
	visited := map[string]bool{}
	for depth := 0; depth < 2 && len(queue) > 0; depth++ {
		var next []job
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, j := range queue {
			if visited[j.url] || len(visited) >= 8 {
				continue
			}
			visited[j.url] = true
			if _, e := cgPublicURL(j.url); e != nil {
				continue
			}
			mu.Lock()
			embeds = append(embeds, j.url)
			mu.Unlock()
			wg.Add(1)
			go func(j job) {
				defer wg.Done()
				body, _, e := cgGet(j.url, map[string]string{"Referer": j.referer, "Accept": "text/html,*/*"}, cgMaxPageBody)
				if e != nil {
					return
				}
				m2, f2 := findMedia(j.url, string(body))
				mu.Lock()
				defer mu.Unlock()
				for _, m := range m2 {
					addSrc(m, j.url)
				}
				for _, f := range f2 {
					next = append(next, job{f, j.url})
				}
			}(j)
		}
		wg.Wait()
		queue = next
	}
	// Pages often carry a short advert video next to the episode: measure every file and keep the real episode first.
	probes := make([]srcProbe, len(cands))
	var pw sync.WaitGroup
	for i, c := range cands {
		pw.Add(1)
		go func(i int, c cand) {
			defer pw.Done()
			probes[i] = probeSource(c.url, c.referer)
		}(i, c)
	}
	pw.Wait()
	order := rankSources(probes)
	for _, i := range order {
		c := cands[i]
		host := ""
		if x, e := neturl.Parse(c.url); e == nil {
			host = x.Hostname()
		}
		label := host
		if d := probes[i].describe(); d != "" {
			label = d + " · " + host
		}
		sources = append(sources, AnimeSource{Type: mediaType(c.url), URL: "/api/anime/stream?k=" + streamToken(c.url, c.referer), Label: label})
	}
	return sources, embeds, nil
}

var reAdURL = regexp.MustCompile(`(?i)(?:vast|adserver|adserv|doubleclick|googlesyndication|googleadservices|/ads?/|[/._-]ads?[/._-]|preroll|pre-roll|advert|adsterra|exoclick|juicyads|trafficjunky|popads|popcash|propeller|banner|promo|sponsor)`)

func looksLikeAd(u string) bool { return reAdURL.MatchString(u) }

type srcProbe struct {
	dur  float64 // seconds, 0 = unknown
	size int64   // bytes, 0 = unknown
}

const (
	adMaxSeconds = 150.0
	adMaxBytes   = 15 << 20
)

func (p srcProbe) isShort() bool {
	return (p.dur > 0 && p.dur < adMaxSeconds) || (p.dur == 0 && p.size > 0 && p.size < adMaxBytes)
}

func (p srcProbe) describe() string {
	switch {
	case p.dur >= 60:
		return fmt.Sprintf("%d min", int(p.dur/60+0.5))
	case p.dur > 0:
		return fmt.Sprintf("%d s", int(p.dur+0.5))
	case p.size > 0:
		return fmt.Sprintf("%d MB", p.size>>20)
	}
	return ""
}

// rankSources puts long videos first, unknown ones next, and drops short clips (adverts) when anything else exists.
func rankSources(p []srcProbe) []int {
	var long, unknown, short []int
	for i := range p {
		switch {
		case p[i].isShort():
			short = append(short, i)
		case p[i].dur == 0 && p[i].size == 0:
			unknown = append(unknown, i)
		default:
			long = append(long, i)
		}
	}
	weight := func(x srcProbe) float64 { return x.dur*1e6 + float64(x.size) }
	sort.SliceStable(long, func(a, b int) bool { return weight(p[long[a]]) > weight(p[long[b]]) })
	out := append(long, unknown...)
	if len(out) == 0 {
		out = short
	}
	return out
}

var reExtInf = regexp.MustCompile(`#EXTINF:([0-9.]+)`)

// probeSource learns how long (HLS) or how big (file) a video is without downloading it.
func probeSource(u, referer string) srcProbe {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	get := func(target string, rng string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", cgUA)
		req.Header.Set("Referer", referer)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		return streamHTTP.Do(req)
	}
	if mediaType(u) == "hls" {
		for hop := 0; hop < 3; hop++ {
			resp, err := get(u, "")
			if err != nil || resp.StatusCode/100 != 2 {
				if resp != nil {
					resp.Body.Close()
				}
				return srcProbe{}
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			text := string(b)
			if strings.Contains(text, "#EXT-X-STREAM-INF") { // master playlist: look into its first variant
				for _, ln := range strings.Split(text, "\n") {
					ln = strings.TrimSpace(ln)
					if ln != "" && !strings.HasPrefix(ln, "#") {
						if nu, e := resp.Request.URL.Parse(ln); e == nil {
							u = nu.String()
						}
						break
					}
				}
				continue
			}
			var total float64
			for _, m := range reExtInf.FindAllStringSubmatch(text, -1) {
				d, _ := strconv.ParseFloat(m[1], 64)
				total += d
			}
			return srcProbe{dur: total}
		}
		return srcProbe{}
	}
	resp, err := get(u, "bytes=0-0")
	if err != nil {
		return srcProbe{}
	}
	defer resp.Body.Close()
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			if n, e := strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64); e == nil {
				return srcProbe{size: n}
			}
		}
	}
	if resp.StatusCode == 200 && resp.ContentLength > 0 {
		return srcProbe{size: resp.ContentLength}
	}
	return srcProbe{}
}

var reM3U8URI = regexp.MustCompile(`URI="([^"]+)"`)

// rewriteM3U8 points every playlist/segment/key line at the stream proxy.
func rewriteM3U8(body string, base *neturl.URL, referer string) string {
	conv := func(ref string) string {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return ref
		}
		if _, err := cgPublicURL(u.String()); err != nil {
			return ref
		}
		return "/api/anime/stream?k=" + streamToken(u.String(), referer)
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case t == "":
		case strings.HasPrefix(t, "#"):
			lines[i] = reM3U8URI.ReplaceAllStringFunc(ln, func(m string) string {
				return `URI="` + conv(m[5:len(m)-1]) + `"`
			})
		default:
			lines[i] = conv(t)
		}
	}
	return strings.Join(lines, "\n")
}

func animeStream(w http.ResponseWriter, r *http.Request) {
	e, ok := streamLookup(r.URL.Query().Get("k"))
	if !ok {
		http.Error(w, "this video link expired, press play again", 410)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", e.url, nil)
	if err != nil {
		http.Error(w, "bad link", 400)
		return
	}
	req.Header.Set("User-Agent", cgUA)
	req.Header.Set("Accept", "*/*")
	if e.referer != "" {
		req.Header.Set("Referer", e.referer)
		if ru, err := neturl.Parse(e.referer); err == nil {
			req.Header.Set("Origin", ru.Scheme+"://"+ru.Host)
		}
	}
	for _, h := range []string{"Range", "If-Range"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := streamHTTP.Do(req)
	if err != nil {
		http.Error(w, "video host unreachable: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("video host answered %d", resp.StatusCode), 502)
		return
	}
	ct := resp.Header.Get("Content-Type")
	low := strings.ToLower(ct)
	isPlaylist := strings.Contains(low, "mpegurl") || strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".m3u8")
	if isPlaylist {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			http.Error(w, "playlist unreadable", 502)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, rewriteM3U8(string(b), resp.Request.URL, e.referer))
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (a *App) streamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/anime/source", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("site") == hstreamID {
			emb, err := hstreamEmbed(q.Get("url"))
			if err != nil {
				fail(w, 400, err.Error())
				return
			}
			// The site refuses to be shown inside other apps and signs its video requests itself: hand back the page to open.
			writeJSON(w, 200, map[string]any{"sources": []AnimeSource{}, "embeds": []string{}, "external": emb[0]})
			return
		}
		s := animeSiteByID(q.Get("site"))
		if s == nil {
			fail(w, 400, "unknown site")
			return
		}
		src, embeds, err := animeResolve(s, q.Get("url"))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if src == nil {
			src = []AnimeSource{}
		}
		if embeds == nil {
			embeds = []string{}
		}
		if s.id == "akih" && len(src) == 0 && len(embeds) == 0 {
			// the player is built by the site's own script: until that is read, show the page in the app's browser window
			writeJSON(w, 200, map[string]any{"sources": src, "embeds": embeds, "external": q.Get("url")})
			return
		}
		writeJSON(w, 200, map[string]any{"sources": src, "embeds": embeds})
	})
	mux.HandleFunc("GET /api/anime/stream", animeStream)
}
