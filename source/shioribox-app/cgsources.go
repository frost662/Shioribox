package main

// CG sources: E-Hentai (Game CG category) here and Hitomi.la (Game CG type) in hitomi.go.
//
// The embedded browser never talks to those sites directly. E-Hentai's image URLs are only found by walking its pages and
// Hitomi's image servers want a hitomi.la Referer, so everything goes through the CG files: search -> gallery -> image, with /api/cg/img as a small allow-listed image proxy.

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const cgUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

var (
	// Many small requests go to the same few hosts, so keep plenty of connections warm.
	cgHTTP = &http.Client{Timeout: 120 * time.Second, Transport: cgTransport()}

	ehBase = "https://e-hentai.org"
)

const (
	ehGameCGCats  = "1007" // E-Hentai category filter is an "exclude" bitmask: 1023 - 16 (Game CG) = only Game CG
	maxCGImage    = 60 << 20
	cgListTTL     = time.Hour
	cgGGTTL       = 10 * time.Minute
	cgMaxPageBody = 4 << 20
)

// ---------- common types ----------

type CGItem struct {
	Ref   string `json:"ref"` // E-Hentai: "gid/token"
	Title string `json:"title"`
	Thumb string `json:"thumb"` // proxied URL, may be empty
	Pages int    `json:"pages"`
}

type CGSearchResult struct {
	Site    string   `json:"site"`
	Items   []CGItem `json:"items"`
	Next    int      `json:"next"`    // next page (E-Hentai); -1 when there is nothing more
	Total   int      `json:"total"`   // unused for E-Hentai
	Scanned int      `json:"scanned"` // unused for E-Hentai
	Indexed bool     `json:"indexed"` // Hitomi: answered from the word index, so the result is complete
}

type CGImage struct {
	N     int    `json:"n"`
	URL   string `json:"url"`
	Thumb string `json:"thumb,omitempty"` // small preview (Hitomi); the grid falls back to URL
}

type CGGallery struct {
	Site    string    `json:"site"`
	Ref     string    `json:"ref"`
	Title   string    `json:"title"`
	Count   int       `json:"count"`
	PerPage int       `json:"perPage"` // E-Hentai only: how many images each gallery page holds
	Page    int       `json:"page"`
	More    bool      `json:"more"`
	Images  []CGImage `json:"images"`
}

func cgGet(u string, hdr map[string]string, limit int64) ([]byte, http.Header, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", cgUA)
	req.Header.Set("Accept-Language", "en")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := cgHTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("%s answered %d", req.URL.Host, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return b, resp.Header, err
}

func unesc(s string) string {
	return strings.TrimSpace(html.UnescapeString(strings.Join(strings.Fields(reAnyTag.ReplaceAllString(s, "")), " ")))
}

var reAnyTag = regexp.MustCompile(`(?s)<[^>]*>`)

// ================= E-Hentai =================

var (
	reEHRow    = regexp.MustCompile(`(?is)<tr[^>]*>.*?</tr>`)
	reEHLink   = regexp.MustCompile(`/g/(\d+)/([0-9a-f]{10})/`)
	reEHGlink  = regexp.MustCompile(`(?is)class="glink"[^>]*>(.*?)</div>`)
	reEHTitle  = regexp.MustCompile(`(?is)<img[^>]*\btitle="([^"]+)"`)
	reEHThumb  = regexp.MustCompile(`(?is)<img[^>]*\b(?:data-src|src)="(https?://[^"]*ehgt\.org[^"]*)"`)
	reEHPages  = regexp.MustCompile(`(?i)(\d+)\s*pages`)
	reEHNext   = regexp.MustCompile(`(?i)id="dnext"[^>]*href="`)
	reEHGN     = regexp.MustCompile(`(?is)<h1 id="gn">(.*?)</h1>`)
	reEHLength = regexp.MustCompile(`(?is)Length:</td>\s*<td[^>]*>\s*(\d+)\s*pages`)
	reEHShot   = regexp.MustCompile(`/s/([0-9a-f]{10})/(\d+)-(\d+)`)
	reEHImg1   = regexp.MustCompile(`(?is)<img[^>]*\bid="img"[^>]*\bsrc="([^"]+)"`)
	reEHImg2   = regexp.MustCompile(`(?is)<img[^>]*\bsrc="([^"]+)"[^>]*\bid="img"`)
	reEHRef    = regexp.MustCompile(`^\d{1,9}/[0-9a-f]{10}$`)
	reEHShotID = regexp.MustCompile(`^[0-9a-f]{10}/\d{1,9}-\d{1,5}$`)
)

func ehHeaders() map[string]string {
	// nw=1 skips the "this gallery may contain offensive content" interstitial.
	return map[string]string{"Cookie": "nw=1", "Referer": ehBase + "/"}
}

// ehParseList extracts galleries from any E-Hentai list layout (compact, extended, thumbnail, minimal).
func ehParseList(page string) ([]CGItem, bool) {
	var out []CGItem
	seen := map[string]bool{}
	rows := reEHRow.FindAllString(page, -1)
	if len(rows) == 0 { // thumbnail layout uses <div class="gl1t"> blocks
		rows = strings.Split(page, `class="gl1t"`)
	}
	for _, row := range rows {
		m := reEHLink.FindStringSubmatch(row)
		if m == nil {
			continue
		}
		ref := m[1] + "/" + m[2]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		it := CGItem{Ref: ref}
		if t := reEHGlink.FindStringSubmatch(row); t != nil {
			it.Title = unesc(t[1])
		} else if t := reEHTitle.FindStringSubmatch(row); t != nil {
			it.Title = unesc(t[1])
		}
		if it.Title == "" {
			it.Title = "Gallery " + m[1]
		}
		if t := reEHThumb.FindStringSubmatch(row); t != nil {
			it.Thumb = "/api/cg/img?t=" + neturl.QueryEscape(html.UnescapeString(t[1]))
		}
		if p := reEHPages.FindStringSubmatch(row); p != nil {
			it.Pages, _ = strconv.Atoi(p[1])
		}
		out = append(out, it)
	}
	return out, reEHNext.MatchString(page)
}

func ehSearch(q string, page int) (*CGSearchResult, error) {
	v := neturl.Values{}
	v.Set("f_cats", ehGameCGCats)
	if q = strings.TrimSpace(q); q != "" {
		v.Set("f_search", q)
	}
	if page > 0 {
		v.Set("page", strconv.Itoa(page))
	}
	b, _, err := cgGet(ehBase+"/?"+v.Encode(), ehHeaders(), cgMaxPageBody)
	if err != nil {
		return nil, fmt.Errorf("E-Hentai: %w", err)
	}
	items, more := ehParseList(string(b))
	res := &CGSearchResult{Site: "ehentai", Items: items, Next: -1}
	if more {
		res.Next = page + 1
	}
	return res, nil
}

func ehParseGallery(page string) (title string, count int, shots []CGImage) {
	if m := reEHGN.FindStringSubmatch(page); m != nil {
		title = unesc(m[1])
	}
	if m := reEHLength.FindStringSubmatch(page); m != nil {
		count, _ = strconv.Atoi(m[1])
	}
	seen := map[string]bool{}
	for _, m := range reEHShot.FindAllStringSubmatch(page, -1) {
		id := m[1] + "/" + m[2] + "-" + m[3]
		if seen[id] {
			continue
		}
		seen[id] = true
		n, _ := strconv.Atoi(m[3])
		shots = append(shots, CGImage{N: n, URL: "/api/cg/img?s=" + id})
	}
	return
}

func ehGallery(ref string, p int) (*CGGallery, error) {
	if !reEHRef.MatchString(ref) {
		return nil, errors.New("bad gallery reference")
	}
	u := fmt.Sprintf("%s/g/%s/?p=%d", ehBase, ref, p)
	b, _, err := cgGet(u, ehHeaders(), cgMaxPageBody)
	if err != nil {
		return nil, fmt.Errorf("E-Hentai: %w", err)
	}
	title, count, shots := ehParseGallery(string(b))
	if len(shots) == 0 {
		if p == 0 {
			return nil, errors.New("E-Hentai returned no images for this gallery (it may be removed or need a login)")
		}
		return nil, fmt.Errorf("E-Hentai returned no images for gallery page %d; the page may be unavailable or the site may have changed its pagination", p+1)
	}
	g := &CGGallery{Site: "ehentai", Ref: ref, Title: title, Count: count, Page: p, Images: shots}
	g.PerPage = len(shots)
	if p > 0 {
		g.PerPage = 0 // the client keeps the value it learnt from page 0
	}
	g.More = len(shots) > 0 && (count == 0 || (shots[len(shots)-1].N < count))
	return g, nil
}

var (
	ehImgMu    sync.Mutex
	ehImgCache = map[string]string{}
)

// ehResolveImage turns "pagetoken/gid-n" into the direct image URL by reading the image's own page.
func ehResolveImage(id string) (string, error) {
	if !reEHShotID.MatchString(id) {
		return "", errors.New("bad image reference")
	}
	ehImgMu.Lock()
	if u, ok := ehImgCache[id]; ok {
		ehImgMu.Unlock()
		return u, nil
	}
	ehImgMu.Unlock()
	b, _, err := cgGet(ehBase+"/s/"+id, ehHeaders(), cgMaxPageBody)
	if err != nil {
		return "", fmt.Errorf("E-Hentai: %w", err)
	}
	m := reEHImg1.FindStringSubmatch(string(b))
	if m == nil {
		m = reEHImg2.FindStringSubmatch(string(b))
	}
	if m == nil {
		return "", errors.New("E-Hentai image page had no image (limit reached or page removed)")
	}
	u := html.UnescapeString(m[1])
	ehImgMu.Lock()
	if len(ehImgCache) > 4000 {
		ehImgCache = map[string]string{}
	}
	ehImgCache[id] = u
	ehImgMu.Unlock()
	return u, nil
}

// ================= image proxy =================

func cgAllowedThumbHost(host string) bool {
	host = strings.ToLower(host)
	if _, ok := heHostReferer(host); ok {
		return true
	}
	for _, d := range []string{"ehgt.org", "e-hentai.org", "gold-usergeneratedcontent.net", "hitomi.la"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func cgPublicURL(raw string) (*neturl.URL, error) {
	u, err := neturl.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, errors.New("bad image address")
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return nil, errors.New("blocked image address")
	}
	if ip := net.ParseIP(h); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return nil, errors.New("blocked image address")
	}
	return u, nil
}

// cgImageRequest works out which upstream URL(s) a /api/cg/img query refers to.
func cgImageRequest(q neturl.Values) (urls []string, referer string, err error) {
	switch {
	case q.Get("t") != "":
		u, e := cgPublicURL(q.Get("t"))
		if e != nil || !cgAllowedThumbHost(u.Hostname()) {
			return nil, "", errors.New("that image address isn't allowed")
		}
		ref := ehBase + "/"
		if r, ok := heHostReferer(u.Hostname()); ok {
			ref = r
		}
		if strings.Contains(u.Hostname(), "hitomi") || strings.Contains(u.Hostname(), "usergeneratedcontent") {
			ref = "https://hitomi.la/"
		}
		urls := []string{u.String()}
		// "e" lists other file extensions to try when the address was built by pattern.
		if e := q.Get("e"); e != "" {
			p := u.Path
			if dot := strings.LastIndex(p, "."); dot > strings.LastIndex(p, "/") {
				for _, x := range strings.Split(e, ",") {
					if len(x) < 1 || len(x) > 4 || strings.Trim(x, "abcdefghijklmnopqrstuvwxyz0123456789") != "" || len(urls) > 4 {
						continue
					}
					alt := *u
					alt.Path = p[:dot+1] + x
					if s := alt.String(); s != urls[0] {
						urls = append(urls, s)
					}
				}
			}
		}
		if v := strings.Split(q.Get("v"), ":"); len(v) == 3 && heSites[v[0]] != nil {
			if n, e := strconv.Atoi(v[2]); e == nil && n > 0 {
				if _, e := strconv.Atoi(v[1]); e == nil {
					urls = append(urls, "view:"+q.Get("v"))
				}
			}
		}
		if th := q.Get("th"); th != "" {
			if tu, e := cgPublicURL(th); e == nil && cgAllowedThumbHost(tu.Hostname()) {
				urls = append(urls, tu.String())
			}
		}
		return urls, ref, nil
	case q.Get("s") != "":
		img, e := ehResolveImage(q.Get("s"))
		if e != nil {
			return nil, "", e
		}
		u, e := cgPublicURL(img)
		if e != nil {
			return nil, "", e
		}
		return []string{u.String()}, ehBase + "/", nil
	case q.Get("site") == "hitomi":
		urls, e := hitomiImageRequest(q.Get("id"), q.Get("i"), q.Get("th") == "1")
		return urls, "https://hitomi.la/", e
	}
	return nil, "", errors.New("missing image reference")
}

func cgServeImage(w http.ResponseWriter, r *http.Request) {
	urls, referer, err := cgImageRequest(r.URL.Query())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var lastErr error
	for _, u := range urls {
		if strings.HasPrefix(u, "view:") { // "view:<site>:<gallery>:<n>": ask the site's viewer page where the picture is
			v := strings.Split(u, ":")
			n, _ := strconv.Atoi(v[3])
			real, e := heResolveView(heSites[v[1]], v[2], n)
			if e != nil {
				lastErr = e
				continue
			}
			pu, e := cgPublicURL(real)
			if e != nil {
				lastErr = e
				continue
			}
			u = pu.String()
		}
		var resp *http.Response
		var ct string
		// Busy image servers answer 429/5xx or drop a connection now and then: one more try usually works.
		for try := 0; try < 3; try++ {
			if try > 0 {
				select {
				case <-time.After(time.Duration(300*try) * time.Millisecond):
				case <-r.Context().Done():
					return
				}
			}
			req, _ := http.NewRequestWithContext(r.Context(), "GET", u, nil)
			req.Header.Set("User-Agent", cgUA)
			req.Header.Set("Referer", referer)
			req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")
			rs, err := cgHTTP.Do(req)
			if err != nil {
				lastErr = err
				continue
			}
			ct = rs.Header.Get("Content-Type")
			if rs.StatusCode/100 == 2 && !strings.HasPrefix(ct, "image/") {
				// Some image servers label pictures as application/octet-stream: trust the file extension then.
				if m := cgExtMime(req.URL.Path); m != "" && (ct == "" || strings.Contains(ct, "octet-stream") || strings.Contains(ct, "binary")) {
					ct = m
				}
			}
			if rs.StatusCode/100 == 2 && strings.HasPrefix(ct, "image/") {
				resp = rs
				break
			}
			rs.Body.Close()
			lastErr = fmt.Errorf("%s answered %d", req.URL.Host, rs.StatusCode)
			if rs.StatusCode != 429 && rs.StatusCode/100 != 5 {
				break // 404 and the like will not change
			}
		}
		if resp == nil {
			continue
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		io.Copy(w, io.LimitReader(resp.Body, maxCGImage))
		resp.Body.Close()
		return
	}
	msg := "image unavailable"
	if lastErr != nil {
		msg = lastErr.Error()
	}
	fail(w, 502, msg)
}

// ================= HTTP handlers =================

func (a *App) cgRoutes(mux *http.ServeMux) {
	a.hitomiInit()
	mux.HandleFunc("GET /api/cg/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		n, _ := strconv.Atoi(q.Get("page"))
		var (
			res *CGSearchResult
			err error
		)
		switch q.Get("site") {
		case "ehentai":
			res, err = ehSearch(q.Get("q"), n)
		case "hentaiera", "hentaienvy":
			res, err = heSearch(heSites[q.Get("site")], q.Get("q"), n)
		case "hitomi":
			res, err = hitomiSearch(q.Get("q"), q.Get("alt"), n, q.Get("deep") == "1")
		default:
			fail(w, 400, "unknown source")
			return
		}
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if res.Items == nil {
			res.Items = []CGItem{}
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("GET /api/cg/gallery", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var (
			g   *CGGallery
			err error
		)
		switch q.Get("site") {
		case "ehentai":
			p, _ := strconv.Atoi(q.Get("p"))
			g, err = ehGallery(q.Get("ref"), p)
		case "hentaiera", "hentaienvy":
			g, err = heGallery(heSites[q.Get("site")], q.Get("ref"))
		case "hitomi":
			id, e := strconv.Atoi(q.Get("ref"))
			if e != nil || id <= 0 {
				fail(w, 400, "bad gallery reference")
				return
			}
			g, err = hitomiGallerySummary(id)
		default:
			fail(w, 400, "unknown source")
			return
		}
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if g.Images == nil {
			g.Images = []CGImage{}
		}
		writeJSON(w, 200, g)
	})
	mux.HandleFunc("GET /api/cg/img", cgServeImage)
}

func cgExtMime(path string) string {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	switch strings.ToLower(path[i+1:]) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "avif":
		return "image/avif"
	}
	return ""
}

func cgTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = 32
	t.MaxConnsPerHost = 0
	return t
}
