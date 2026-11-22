package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Recommendations: find what the games you liked have in common (their VNDB tags), then ask VNDB
// for well-voted games that share those tags and are not in your library yet.

type Recommendation struct {
	Results []VNDBResult `json:"results"`
	Tags    []string     `json:"tags"`  // the tags the suggestions are based on
	Games   []string     `json:"games"` // the games you liked most
}

type likedGame struct {
	id, title string
	w         float64
}

// likedGames picks library games with a VNDB id that the user seems to like, strongest first.
func likedGames(games []*Game) []likedGame {
	var out []likedGame
	for _, g := range games {
		if !reVNID.MatchString(g.VNDBID) {
			continue
		}
		w := 0.0
		switch {
		case g.Rating >= 7:
			w = g.Rating - 5
		case g.Rating == 0 && g.Status == "completed":
			w = 3
		}
		if g.Favorite {
			w += 2
		}
		if w > 0 {
			out = append(out, likedGame{strings.ToLower(g.VNDBID), g.Title, w})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].w > out[j].w })
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

type tagScore struct {
	id, name string
	score    float64
	n        int
}

// topTags weighs each tag by how strongly VNDB applies it and how much you liked the game.
func topTags(raw []byte, liked []likedGame) ([]tagScore, error) {
	var p struct {
		Results []struct {
			ID   string `json:"id"`
			Tags []struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				Rating   float64 `json:"rating"`
				Spoiler  int     `json:"spoiler"`
				Category string  `json:"category"`
			} `json:"tags"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	weight := map[string]float64{}
	for _, l := range liked {
		weight[l.id] = l.w
	}
	m := map[string]*tagScore{}
	for _, r := range p.Results {
		for _, t := range r.Tags {
			if t.Spoiler != 0 || t.Category == "tech" || t.Rating < 1.5 || !reTagID.MatchString(t.ID) {
				continue
			}
			ts := m[t.ID]
			if ts == nil {
				ts = &tagScore{id: t.ID, name: t.Name}
				m[t.ID] = ts
			}
			ts.score += weight[r.ID] * t.Rating / 3
			ts.n++
		}
	}
	var out []tagScore
	for _, t := range m {
		if len(liked) < 3 || t.n >= 2 { // with a few games, ignore one-off tags
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].id < out[j].id
	})
	return out, nil
}

func recommend(games []*Game) (*Recommendation, error) {
	liked := likedGames(games)
	if len(liked) < 2 {
		return nil, errors.New("not enough to go on yet — rate at least two games 7 or higher (or mark them completed or favorite) that have a VNDB link, and recommendations will appear")
	}
	var idf []any
	for _, l := range liked {
		idf = append(idf, []any{"id", "=", l.id})
	}
	var filter any = idf[0]
	if len(idf) > 1 {
		filter = append([]any{"or"}, idf...)
	}
	raw, err := vndbPost("/vn", map[string]any{
		"filters": filter, "fields": "id,tags.id,tags.name,tags.rating,tags.spoiler,tags.category", "results": 25,
	})
	if err != nil {
		return nil, err
	}
	tags, err := topTags(raw, liked)
	if err != nil {
		return nil, err
	}
	if len(tags) < 2 {
		return nil, errors.New("the games you liked don't share enough VNDB tags yet")
	}
	if len(tags) > 8 {
		tags = tags[:8]
	}
	// a few queries over pairs of your strongest tags: specific enough to be relevant
	pairs := [][2]int{{0, 1}, {0, 2}, {1, 2}, {0, 3}}
	own := map[string]bool{}
	for _, g := range games {
		own[strings.ToLower(g.VNDBID)] = true
	}
	top := map[string]bool{}
	for _, t := range tags {
		top[t.name] = true
	}
	seen := map[string]bool{}
	var cands []VNDBResult
	for _, pr := range pairs {
		if pr[1] >= len(tags) {
			continue
		}
		f := []any{"and", []any{"tag", "=", tags[pr[0]].id}, []any{"tag", "=", tags[pr[1]].id}, []any{"votecount", ">=", 100}}
		res, _, err := vndbVN(f, "rating", true, 1, 20)
		if err != nil {
			return nil, err
		}
		for _, r := range res {
			if !own[r.ID] && !seen[r.ID] {
				seen[r.ID] = true
				cands = append(cands, r)
			}
		}
	}
	match := func(r VNDBResult) int {
		n := 0
		for _, t := range r.Tags {
			if top[t] {
				n++
			}
		}
		return n
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if mi, mj := match(cands[i]), match(cands[j]); mi != mj {
			return mi > mj
		}
		return cands[i].Rating > cands[j].Rating
	})
	if len(cands) > 24 {
		cands = cands[:24]
	}
	rec := &Recommendation{Results: cands, Tags: []string{}, Games: []string{}}
	for i, t := range tags {
		if i < 5 {
			rec.Tags = append(rec.Tags, t.name)
		}
	}
	for i, l := range liked {
		if i < 5 {
			rec.Games = append(rec.Games, l.title)
		}
	}
	return rec, nil
}
