package main

// Hitomi.la as a CG source (Game CG type).
//
// How a search works, fastest first:
//  0. Word index: Hitomi's own search box uses a public B-tree of title words (galleriesindex/). ShioriBox reads the
//     same index with a few small range requests, so a title resolves across every gallery in about a second.
//  1. Series lookup: Hitomi keeps one small index file per series ("series/<name>-all.nozomi"). Game CG galleries are
//     tagged with the game's name, so the game title usually resolves to its galleries with a handful of requests.
//  2. Title index: every gallery title ShioriBox has seen is remembered on disk (cache/hitomi-index.json), so
//     repeat searches match instantly and never fetch the same gallery page twice.
//  3. Scan: whatever is still unknown is read newest-first in time-boxed batches (the page keeps asking until
//     something turns up), and everything read lands in the title index for next time.
// Images are fetched through /api/cg/img because Hitomi's image servers insist on a hitomi.la Referer.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hosts that serve Hitomi's index files, tried in order. The second element is the domain the image
// servers (w1., w2., aa., ba., tn. ...) live under for that index host.
var hitomiHosts = []struct{ ltn, domain string }{
	{"https://ltn.gold-usergeneratedcontent.net", "gold-usergeneratedcontent.net"},
	{"https://ltn.hitomi.la", "hitomi.la"},
}

// Index files live under "/n/" on current servers and at the root on older ones.
var hitomiPrefixes = []string{"/n", ""}

const (
	hitomiBrowseChunk = 40              // galleries per page when browsing without a query
	hitomiScanBudget  = 7 * time.Second // how long one search call may keep scanning
	hitomiScanBatch   = 24              // galleries read per round while scanning
	hitomiFetchPar    = 20              // parallel requests
	hitomiMaxHits     = 24              // stop a scan call once this many matches are found
)

type hitomiGG struct {
	def   int
	cases map[int]int
	b     string // path prefix, ends with "/"
	at    time.Time
	host  int // index into hitomiHosts that worked
}

var (
	reGGB     = regexp.MustCompile(`\bb:\s*'([^']*)'`)
	reGGDef   = regexp.MustCompile(`var\s+o\s*=\s*(\d+)\s*;`)
	reGGCases = regexp.MustCompile(`((?:case\s+\d+\s*:\s*)+)o\s*=\s*(\d+)\s*;`)
	reGGCase  = regexp.MustCompile(`case\s+(\d+)`)
)

func parseGG(js string) (*hitomiGG, error) {
	gg := &hitomiGG{cases: map[int]int{}}
	mb := reGGB.FindStringSubmatch(js)
	if mb == nil {
		return nil, errors.New("Hitomi: unexpected gg.js (no path prefix)")
	}
	gg.b = mb[1]
	if !strings.HasSuffix(gg.b, "/") {
		gg.b += "/"
	}
	if m := reGGDef.FindStringSubmatch(js); m != nil {
		gg.def, _ = strconv.Atoi(m[1])
	}
	for _, m := range reGGCases.FindAllStringSubmatch(js, -1) {
		val, _ := strconv.Atoi(m[2])
		for _, c := range reGGCase.FindAllStringSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(c[1])
			gg.cases[n] = val
		}
	}
	return gg, nil
}

// slot is gg.s(hash) as a number: the last three hex digits, rearranged, read as hex.
func hitomiSlot(hash string) (int, bool) {
	if len(hash) < 3 {
		return 0, false
	}
	t := hash[len(hash)-3:]
	n, err := strconv.ParseInt(t[2:]+t[:2], 16, 32)
	return int(n), err == nil
}

func (gg *hitomiGG) m(g int) int {
	if v, ok := gg.cases[g]; ok {
		return v
	}
	return gg.def
}

// hitomiImageURLs returns candidate URLs for one file, best first.
func hitomiImageURLs(gg *hitomiGG, domain, hash, name string, hasWebp bool) []string {
	slot, ok := hitomiSlot(hash)
	if !ok {
		return nil
	}
	m := gg.m(slot)
	path := gg.b + strconv.Itoa(slot) + "/" + hash
	var out []string
	if hasWebp {
		out = append(out, fmt.Sprintf("https://w%d.%s/%s.webp", 1+m, domain, path))
	}
	ext := "jpg"
	if i := strings.LastIndex(name, "."); i >= 0 && i < len(name)-1 {
		ext = strings.ToLower(name[i+1:])
	}
	out = append(out, fmt.Sprintf("https://%ca.%s/images/%s.%s", 'a'+m, domain, path, ext))
	return out
}

// hitomiThumbURL is the small preview of one image (what Hitomi's own gallery page shows).
func hitomiThumbURL(domain, hash, dir string) string {
	if len(hash) < 3 {
		return ""
	}
	return fmt.Sprintf("https://tn.%s/%s/%s/%s/%s.webp", domain, dir, hash[len(hash)-1:], hash[len(hash)-3:len(hash)-1], hash)
}

// ---------- state ----------

// hitomiEntry is what is remembered about one gallery: enough to show it as a search result.
type hitomiEntry struct {
	T string `json:"t"`           // title
	U string `json:"u,omitempty"` // cover thumbnail (upstream URL)
	P int    `json:"p,omitempty"` // page count
}

var (
	hitomiMu        sync.Mutex
	hitomiIDs       []int
	hitomiIDSet     map[int]struct{}
	hitomiIDsAt     time.Time
	hitomiPair      = [2]int{-1, -1} // host and prefix index that last worked
	hitomiGGc       *hitomiGG
	hitomiGals      = map[int]*hitomiGallery{}
	hitomiIdx       = map[int]hitomiEntry{}
	hitomiIdxLoaded bool
	hitomiIdxDirty  int
	hitomiCacheFile string
)

func (a *App) hitomiInit() {
	hitomiMu.Lock()
	hitomiCacheFile = filepath.Join(a.dataDir, "cache", "hitomi-index.json")
	hitomiMu.Unlock()
}

func hitomiLoadIdx() {
	hitomiMu.Lock()
	defer hitomiMu.Unlock()
	if hitomiIdxLoaded {
		return
	}
	hitomiIdxLoaded = true
	if hitomiCacheFile == "" {
		return
	}
	b, err := os.ReadFile(hitomiCacheFile)
	if err != nil {
		return
	}
	var m map[string]hitomiEntry
	if json.Unmarshal(b, &m) != nil {
		return
	}
	for k, v := range m {
		if id, err := strconv.Atoi(k); err == nil && v.T != "" {
			hitomiIdx[id] = v
		}
	}
}

func hitomiSaveIdx() {
	hitomiMu.Lock()
	if hitomiIdxDirty == 0 || hitomiCacheFile == "" {
		hitomiMu.Unlock()
		return
	}
	m := make(map[string]hitomiEntry, len(hitomiIdx))
	for id, e := range hitomiIdx {
		m[strconv.Itoa(id)] = e
	}
	hitomiIdxDirty = 0
	path := hitomiCacheFile
	hitomiMu.Unlock()
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, path)
	}
}

type hitomiFile struct {
	Hash    string `json:"hash"`
	Name    string `json:"name"`
	HasWebp int    `json:"haswebp"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

type hitomiGallery struct {
	Title string       `json:"title"`
	JP    string       `json:"japanese_title"`
	Files []hitomiFile `json:"files"`
}

// ---------- fetching ----------

func hitomiHeaders(extra map[string]string) map[string]string {
	h := map[string]string{"Referer": "https://hitomi.la/"}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// hitomiFetch asks the index servers for a path ("/gg.js", "/galleries/1.js", ...), trying each host in turn.
func hitomiFetch(path string, limit int64) ([]byte, int, error) {
	hitomiMu.Lock()
	first := hitomiPair[0]
	hitomiMu.Unlock()
	if first < 0 {
		first = 0
	}
	order := []int{first}
	for i := range hitomiHosts {
		if i != first {
			order = append(order, i)
		}
	}
	var lastErr error
	for _, i := range order {
		b, _, err := cgGet(hitomiHosts[i].ltn+path, hitomiHeaders(nil), limit)
		if err == nil {
			hitomiMu.Lock()
			if hitomiPair[0] != i {
				hitomiPair = [2]int{i, hitomiPair[1]}
			}
			hitomiMu.Unlock()
			return b, i, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("Hitomi: %w", lastErr)
}

// hitomiNozomi reads an index file such as "type/gamecg-all.nozomi". Once a host/prefix pair has worked only that
// pair is used, so asking for a series that does not exist costs one quick 404.
func hitomiNozomi(rel string, limit int64) ([]int, error) {
	hitomiMu.Lock()
	known := hitomiPair
	hitomiMu.Unlock()
	if known[0] >= 0 && known[1] >= 0 {
		b, _, err := cgGet(hitomiHosts[known[0]].ltn+hitomiPrefixes[known[1]]+"/"+rel, hitomiHeaders(nil), limit)
		if err != nil {
			return nil, err
		}
		return parseNozomi(b), nil
	}
	var lastErr error
	for i := range hitomiHosts {
		for j := range hitomiPrefixes {
			b, _, err := cgGet(hitomiHosts[i].ltn+hitomiPrefixes[j]+"/"+rel, hitomiHeaders(nil), limit)
			if err == nil {
				hitomiMu.Lock()
				hitomiPair = [2]int{i, j}
				hitomiMu.Unlock()
				return parseNozomi(b), nil
			}
			lastErr = err
		}
	}
	return nil, fmt.Errorf("Hitomi: %w", lastErr)
}

func parseNozomi(b []byte) []int {
	ids := make([]int, 0, len(b)/4)
	for i := 0; i+4 <= len(b); i += 4 {
		ids = append(ids, int(binary.BigEndian.Uint32(b[i:i+4])))
	}
	return ids
}

// hitomiList is the newest-first list of every Game CG gallery id.
func hitomiList() ([]int, error) {
	hitomiMu.Lock()
	if hitomiIDs != nil && time.Since(hitomiIDsAt) < cgListTTL {
		ids := hitomiIDs
		hitomiMu.Unlock()
		return ids, nil
	}
	hitomiMu.Unlock()
	ids, err := hitomiNozomi("type/gamecg-all.nozomi", 64<<20)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("Hitomi: the Game CG list was empty")
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	hitomiMu.Lock()
	hitomiIDs, hitomiIDSet, hitomiIDsAt = ids, set, time.Now()
	hitomiMu.Unlock()
	return ids, nil
}

func hitomiGGGet() (*hitomiGG, string, error) {
	hitomiMu.Lock()
	if hitomiGGc != nil && time.Since(hitomiGGc.at) < cgGGTTL {
		gg, d := hitomiGGc, hitomiHosts[hitomiGGc.host].domain
		hitomiMu.Unlock()
		return gg, d, nil
	}
	hitomiMu.Unlock()
	b, ix, err := hitomiFetch("/gg.js", 2<<20)
	if err != nil {
		return nil, "", err
	}
	gg, err := parseGG(string(b))
	if err != nil {
		return nil, "", err
	}
	gg.at, gg.host = time.Now(), ix
	hitomiMu.Lock()
	hitomiGGc = gg
	hitomiMu.Unlock()
	return gg, hitomiHosts[ix].domain, nil
}

// ---------- gallery blocks (title, cover, page count) ----------

var (
	reHBTitle = regexp.MustCompile(`(?is)<h1[^>]*>\s*<a[^>]*>(.*?)</a>`)
	reHBThumb = regexp.MustCompile(`(?is)<img[^>]*\b(?:data-src|src)="((?:https?:)?//[^"]+)"`)
	reHBPages = regexp.MustCompile(`(?i)(\d+)\s*pages`)
)

func parseHitomiBlock(page string) (hitomiEntry, bool) {
	m := reHBTitle.FindStringSubmatch(page)
	if m == nil {
		return hitomiEntry{}, false
	}
	e := hitomiEntry{T: unesc(m[1])}
	if t := reHBThumb.FindStringSubmatch(page); t != nil {
		u := html.UnescapeString(t[1])
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		e.U = u
	}
	if p := reHBPages.FindStringSubmatch(page); p != nil {
		e.P, _ = strconv.Atoi(p[1])
	}
	return e, e.T != ""
}

// hitomiEntryFor returns what is known about a gallery, reading its block from Hitomi when it is new.
func hitomiEntryFor(id int) (hitomiEntry, error) {
	hitomiMu.Lock()
	if e, ok := hitomiIdx[id]; ok {
		hitomiMu.Unlock()
		return e, nil
	}
	hitomiMu.Unlock()
	b, _, err := hitomiFetch("/galleryblock/"+strconv.Itoa(id)+".html", 1<<20)
	if err != nil {
		return hitomiEntry{}, err
	}
	e, ok := parseHitomiBlock(string(b))
	if !ok {
		return hitomiEntry{}, errors.New("Hitomi: unreadable gallery block")
	}
	hitomiMu.Lock()
	hitomiIdx[id] = e
	hitomiIdxDirty++
	hitomiMu.Unlock()
	return e, nil
}

// hitomiEntries resolves many galleries in parallel. Entries that could not be read are nil.
func hitomiEntries(ids []int) ([]*hitomiEntry, error) {
	out := make([]*hitomiEntry, len(ids))
	var wg sync.WaitGroup
	var emu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, hitomiFetchPar)
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(i, id int) {
			defer wg.Done()
			defer func() { <-sem }()
			e, err := hitomiEntryFor(id)
			if err != nil {
				emu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				emu.Unlock()
				return
			}
			out[i] = &e
		}(i, id)
	}
	wg.Wait()
	return out, firstErr
}

func (e hitomiEntry) item(id int) CGItem {
	it := CGItem{Ref: strconv.Itoa(id), Title: e.T, Pages: e.P}
	if e.U != "" {
		it.Thumb = "/api/cg/img?t=" + url.QueryEscape(e.U)
	}
	return it
}

// ---------- matching ----------

func normTitle(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 0x2fff {
			sb.WriteRune(r)
		} else {
			sb.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// titleMatches is true when every word of q appears in title (CJK text is matched as a substring).
func titleMatches(q, title string) bool {
	nq, nt := normTitle(q), normTitle(title)
	if nq == "" {
		return true
	}
	if strings.Contains(nt, nq) {
		return true
	}
	flat := strings.ReplaceAll(nt, " ", "")
	for _, w := range strings.Fields(nq) {
		if !strings.Contains(nt, w) && !strings.Contains(flat, w) {
			return false
		}
	}
	return true
}

var reSubtitleCut = regexp.MustCompile(`\s*(?::\s|\s[-–—~～〜]\s?|[~～〜]|\(|\[|（|【)`)

// hitomiSeriesNames lists the spellings a game's series tag might have on Hitomi: lower case, with punctuation
// kept, turned into spaces or dropped, and also without a trailing subtitle ("Clannad ~After Story~" -> "clannad").
func hitomiSeriesNames(titles ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
		if s != "" && !seen[s] && len(out) < 10 {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range titles {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		cuts := []string{t}
		if loc := reSubtitleCut.FindStringIndex(t); loc != nil && loc[0] > 0 {
			cuts = append(cuts, t[:loc[0]])
		}
		for _, c := range cuts {
			add(c)
			add(normTitle(c))
			var nopunct strings.Builder
			for _, r := range strings.ToLower(c) {
				if r == ' ' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 0x2fff {
					nopunct.WriteRune(r)
				}
			}
			add(nopunct.String())
		}
	}
	return out
}

// hitomiSeriesIDs returns the Game CG galleries filed under the game's series tag, newest first.
func hitomiSeriesIDs(titles ...string) []int {
	names := hitomiSeriesNames(titles...)
	hitomiMu.Lock()
	set := hitomiIDSet
	hitomiMu.Unlock()
	var mu sync.Mutex
	var wg sync.WaitGroup
	got := map[int]bool{}
	sem := make(chan struct{}, 4)
	for _, n := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(n string) {
			defer wg.Done()
			defer func() { <-sem }()
			ids, err := hitomiNozomi("series/"+url.PathEscape(n)+"-all.nozomi", 8<<20)
			if err != nil {
				return // no such series (404) or a hiccup: the scan below still covers it
			}
			mu.Lock()
			for _, id := range ids {
				if _, ok := set[id]; ok {
					got[id] = true
				}
			}
			mu.Unlock()
		}(n)
	}
	wg.Wait()
	ids := make([]int, 0, len(got))
	for id := range got {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	return ids
}

// ---------- word index (Hitomi's own title search) ----------

const (
	hitomiNodeSize   = 464 // bytes per B-tree node in galleries.<version>.index
	hitomiIndexFan   = 17  // child addresses stored per node
	hitomiMaxIDsData = 16 << 20
)

var (
	hitomiIdxVer   string
	hitomiIdxVerAt time.Time
	hitomiRangeOK  = true // false once a server ignored Range requests (then the index is skipped)
)

type hitomiNode struct {
	keys  [][]byte
	datas [][2]int64 // offset, length into galleries.<version>.data
	subs  [hitomiIndexFan]int64
}

func decodeHitomiNode(b []byte) (*hitomiNode, error) {
	pos := 0
	i32 := func() (int, bool) {
		if pos+4 > len(b) {
			return 0, false
		}
		v := int(int32(binary.BigEndian.Uint32(b[pos:])))
		pos += 4
		return v, true
	}
	bad := errors.New("Hitomi: unreadable search index")
	n := &hitomiNode{}
	nk, ok := i32()
	if !ok || nk < 0 || nk > 32 {
		return nil, bad
	}
	for i := 0; i < nk; i++ {
		sz, ok := i32()
		if !ok || sz <= 0 || sz > 32 || pos+sz > len(b) {
			return nil, bad
		}
		n.keys = append(n.keys, b[pos:pos+sz])
		pos += sz
	}
	nd, ok := i32()
	if !ok || nd < 0 || nd > 32 {
		return nil, bad
	}
	for i := 0; i < nd; i++ {
		if pos+8 > len(b) {
			return nil, bad
		}
		off := int64(binary.BigEndian.Uint64(b[pos:]))
		pos += 8
		ln, ok := i32()
		if !ok {
			return nil, bad
		}
		n.datas = append(n.datas, [2]int64{off, int64(ln)})
	}
	for i := 0; i < hitomiIndexFan; i++ {
		if pos+8 > len(b) {
			break // a short node: the remaining children are absent
		}
		n.subs[i] = int64(binary.BigEndian.Uint64(b[pos:]))
		pos += 8
	}
	if len(n.datas) < len(n.keys) {
		return nil, bad
	}
	return n, nil
}

// hitomiRangeGet reads bytes [from,to] of a file on the index server. It insists on a 206 answer: a server that
// ignores Range would send the whole (huge) file.
func hitomiRangeGet(path string, from, to int64) ([]byte, error) {
	hitomiMu.Lock()
	host := hitomiPair[0]
	ok := hitomiRangeOK
	hitomiMu.Unlock()
	if !ok {
		return nil, errors.New("range requests unavailable")
	}
	if host < 0 {
		host = 0
	}
	req, err := http.NewRequest("GET", hitomiHosts[host].ltn+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", cgUA)
	req.Header.Set("Referer", "https://hitomi.la/")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	resp, err := cgHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		hitomiMu.Lock()
		hitomiRangeOK = false
		hitomiMu.Unlock()
		return nil, errors.New("server ignored the Range header")
	}
	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("%s answered %d", req.URL.Host, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, hitomiMaxIDsData+1))
}

func hitomiGalleriesVersion() (string, error) {
	hitomiMu.Lock()
	if hitomiIdxVer != "" && time.Since(hitomiIdxVerAt) < cgListTTL {
		v := hitomiIdxVer
		hitomiMu.Unlock()
		return v, nil
	}
	hitomiMu.Unlock()
	b, _, err := hitomiFetch("/galleriesindex/version?_="+strconv.FormatInt(time.Now().UnixMilli(), 10), 256)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" || len(v) > 32 || strings.ContainsAny(v, "/\\ \n<>") {
		return "", errors.New("Hitomi: unexpected search index version")
	}
	hitomiMu.Lock()
	hitomiIdxVer, hitomiIdxVerAt = v, time.Now()
	hitomiMu.Unlock()
	return v, nil
}

// hitomiWordIDs returns every gallery whose title contains the word, or ok=false when the word is not in the index.
func hitomiWordIDs(ver, word string) ([]int, bool, error) {
	sum := sha256.Sum256([]byte(word))
	key := sum[:4]
	idxPath := "/galleriesindex/galleries." + ver + ".index"
	addr := int64(0)
	for depth := 0; depth < 10; depth++ {
		b, err := hitomiRangeGet(idxPath, addr, addr+hitomiNodeSize-1)
		if err != nil {
			return nil, false, err
		}
		node, err := decodeHitomiNode(b)
		if err != nil {
			return nil, false, err
		}
		if len(node.keys) == 0 {
			return nil, false, nil
		}
		at, found := len(node.keys), false
		for i, k := range node.keys {
			c := bytes.Compare(key, k)
			if c == 0 {
				at, found = i, true
				break
			}
			if c < 0 {
				at = i
				break
			}
		}
		if found {
			off, ln := node.datas[at][0], node.datas[at][1]
			if ln < 4 || ln > hitomiMaxIDsData {
				return nil, false, errors.New("Hitomi: unreadable search index entry")
			}
			d, err := hitomiRangeGet("/galleriesindex/galleries."+ver+".data", off, off+ln-1)
			if err != nil {
				return nil, false, err
			}
			if len(d) < 4 {
				return nil, false, errors.New("Hitomi: unreadable search index entry")
			}
			n := int(binary.BigEndian.Uint32(d))
			if n < 0 || 4+n*4 > len(d) {
				return nil, false, errors.New("Hitomi: unreadable search index entry")
			}
			ids := make([]int, n)
			for i := range ids {
				ids[i] = int(binary.BigEndian.Uint32(d[4+i*4:]))
			}
			return ids, true, nil
		}
		next := node.subs[at]
		if next == 0 { // leaf: the word is not there
			return nil, false, nil
		}
		addr = next
	}
	return nil, false, errors.New("Hitomi: search index too deep")
}

// hitomiIndexSearch finds Game CG galleries whose title holds the query's words. ok=false means the index could not be
// used (caller falls back to scanning). Words missing from the index are ignored, so a title that Hitomi tokenises
// differently still gets found; the caller double-checks every candidate against the real title.
func hitomiIndexSearch(titles ...string) (ids []int, ok bool) {
	ver, err := hitomiGalleriesVersion()
	if err != nil {
		return nil, false
	}
	hitomiMu.Lock()
	set := hitomiIDSet
	hitomiMu.Unlock()
	if set == nil {
		return nil, false
	}
	have := map[int]bool{}
	for _, t := range titles {
		words := strings.Fields(normTitle(t))
		if len(words) == 0 {
			continue
		}
		if len(words) > 6 {
			words = words[:6]
		}
		lists := make([][]int, len(words))
		oks := make([]bool, len(words))
		var wg sync.WaitGroup
		var failed bool
		var fmu sync.Mutex
		for i, w := range words {
			wg.Add(1)
			go func(i int, w string) {
				defer wg.Done()
				l, found, err := hitomiWordIDs(ver, w)
				if err != nil {
					fmu.Lock()
					failed = true
					fmu.Unlock()
					return
				}
				lists[i], oks[i] = l, found
			}(i, w)
		}
		wg.Wait()
		if failed {
			return nil, false
		}
		var cur map[int]bool
		for i := range words {
			if !oks[i] {
				continue
			}
			next := map[int]bool{}
			for _, id := range lists[i] {
				if _, isCG := set[id]; isCG && (cur == nil || cur[id]) {
					next[id] = true
				}
			}
			cur = next
		}
		for id := range cur {
			have[id] = true
		}
	}
	for id := range have {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	return ids, true
}

// ---------- search ----------

// hitomiSearch: an empty query lists the Game CG galleries newest first, a page (offset) at a time. With a query the
// first call (offset 0) adds the series hits and everything already in the title index, then every call scans
// onwards from offset for up to hitomiScanBudget. Next is where to continue, or -1 when the whole list is covered.
func hitomiSearch(q, alt string, offset int, deep bool) (*CGSearchResult, error) {
	hitomiLoadIdx()
	ids, err := hitomiList()
	if err != nil {
		return nil, err
	}
	if offset < 0 || offset > len(ids) {
		offset = 0
	}
	res := &CGSearchResult{Site: "hitomi", Total: len(ids), Next: -1}
	q, alt = strings.TrimSpace(q), strings.TrimSpace(alt)
	defer hitomiSaveIdx()

	if q == "" {
		end := offset + hitomiBrowseChunk
		if end > len(ids) {
			end = len(ids)
		}
		es, ferr := hitomiEntries(ids[offset:end])
		got := 0
		for i, e := range es {
			if e != nil {
				got++
				res.Items = append(res.Items, e.item(ids[offset+i]))
			}
		}
		if got == 0 && ferr != nil {
			return nil, ferr
		}
		res.Scanned = end
		if end < len(ids) {
			res.Next = end
		}
		return res, nil
	}

	matches := func(title string) bool { return titleMatches(q, title) || (alt != "" && titleMatches(alt, title)) }
	seen := map[int]bool{}
	add := func(id int, e hitomiEntry) {
		if !seen[id] {
			seen[id] = true
			res.Items = append(res.Items, e.item(id))
		}
	}
	if offset == 0 && !deep {
		if iids, ok := hitomiIndexSearch(q, alt); ok {
			// The word index covers every gallery, so this answer is complete: no scan needed.
			if len(iids) > 60 {
				iids = iids[:60]
			}
			es, _ := hitomiEntries(iids)
			for i, e := range es {
				if e != nil && matches(e.T) {
					add(iids[i], *e)
				}
			}
			for _, id := range hitomiSeriesIDs(q, alt) {
				if e, err := hitomiEntryFor(id); err == nil {
					add(id, e)
				}
			}
			res.Indexed, res.Scanned = true, len(ids)
			return res, nil
		}
	}
	if offset == 0 {
		if sids := hitomiSeriesIDs(q, alt); len(sids) > 0 {
			if es, _ := hitomiEntries(sids); es != nil {
				for i, e := range es {
					if e != nil {
						add(sids[i], *e)
					}
				}
			}
		}
		hitomiMu.Lock()
		for _, id := range ids { // everything already known: free
			if e, ok := hitomiIdx[id]; ok && matches(e.T) {
				add(id, e)
			}
		}
		hitomiMu.Unlock()
	}

	deadline := time.Now().Add(hitomiScanBudget)
	pos, read := offset, 0
	var firstErr error
	for pos < len(ids) && time.Now().Before(deadline) && len(res.Items) < hitomiMaxHits {
		end := pos + hitomiScanBatch
		if end > len(ids) {
			end = len(ids)
		}
		es, ferr := hitomiEntries(ids[pos:end])
		if ferr != nil && firstErr == nil {
			firstErr = ferr
		}
		for i, e := range es {
			if e != nil {
				read++
				if matches(e.T) {
					add(ids[pos+i], *e)
				}
			}
		}
		pos = end
	}
	if read == 0 && len(res.Items) == 0 && firstErr != nil {
		return nil, firstErr
	}
	res.Scanned = pos
	if pos < len(ids) {
		res.Next = pos
	}
	return res, nil
}

// ---------- gallery ----------

func hitomiGalleryInfo(id int) (*hitomiGallery, error) {
	hitomiMu.Lock()
	if g, ok := hitomiGals[id]; ok {
		hitomiMu.Unlock()
		return g, nil
	}
	hitomiMu.Unlock()
	b, _, err := hitomiFetch("/galleries/"+strconv.Itoa(id)+".js", 4<<20)
	if err != nil {
		return nil, err
	}
	i := bytes.IndexByte(b, '{')
	if i < 0 {
		return nil, errors.New("Hitomi: unreadable gallery data")
	}
	var g hitomiGallery
	if err := json.Unmarshal(bytes.TrimRight(b[i:], "; \r\n\t"), &g); err != nil {
		return nil, fmt.Errorf("Hitomi: unreadable gallery data: %w", err)
	}
	hitomiMu.Lock()
	if len(hitomiGals) > 300 {
		hitomiGals = map[int]*hitomiGallery{}
	}
	hitomiGals[id] = &g
	hitomiMu.Unlock()
	return &g, nil
}

// hitomiGallerySummary lists every image of a gallery in one go (Hitomi has no pages), each with a small preview.
func hitomiGallerySummary(id int) (*CGGallery, error) {
	g, err := hitomiGalleryInfo(id)
	if err != nil {
		return nil, err
	}
	title := g.Title
	if title == "" {
		title = g.JP
	}
	n := len(g.Files)
	out := &CGGallery{Site: "hitomi", Ref: strconv.Itoa(id), Title: title, Count: n, PerPage: n}
	for i := range g.Files {
		im := CGImage{N: i + 1, URL: fmt.Sprintf("/api/cg/img?site=hitomi&id=%d&i=%d", id, i)}
		im.Thumb = im.URL + "&th=1"
		out.Images = append(out.Images, im)
	}
	return out, nil
}

// hitomiImageRequest turns /api/cg/img?site=hitomi&id=..&i=.. into upstream URLs.
func hitomiImageRequest(idStr, ixStr string, thumb bool) ([]string, error) {
	id, e1 := strconv.Atoi(idStr)
	ix, e2 := strconv.Atoi(ixStr)
	if e1 != nil || e2 != nil || id <= 0 || ix < 0 {
		return nil, errors.New("bad image reference")
	}
	g, err := hitomiGalleryInfo(id)
	if err != nil {
		return nil, err
	}
	if ix >= len(g.Files) {
		return nil, errors.New("image number out of range")
	}
	gg, domain, err := hitomiGGGet()
	if err != nil {
		return nil, err
	}
	f := g.Files[ix]
	full := hitomiImageURLs(gg, domain, f.Hash, f.Name, f.HasWebp != 0)
	var th []string
	for _, dir := range []string{"webpsmalltn", "webpbigtn"} {
		if t := hitomiThumbURL(domain, f.Hash, dir); t != "" {
			th = append(th, t)
		}
	}
	// Previews: the small thumbnail exists for every image (the "big" one is made for covers only), and the full
	// picture is the last resort. The full picture is the other way round: the preview is the last resort, so the
	// viewer shows something even when Hitomi's image servers refuse the full file.
	if thumb {
		return append(th, full...), nil
	}
	return append(full, th...), nil
}
