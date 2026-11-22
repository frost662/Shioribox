package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Full details of one visual novel for the Discover page's detail window.

type VNDetail struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	AltTitle      string      `json:"altTitle"`
	Titles        []DTitle    `json:"titles"`
	Aliases       []string    `json:"aliases"`
	DevStatus     int         `json:"devStatus"` // 0 finished, 1 in development, 2 cancelled
	Released      string      `json:"released"`
	OLang         string      `json:"olang"`
	Languages     []string    `json:"languages"`
	Platforms     []string    `json:"platforms"`
	Image         string      `json:"image"`
	Sexual        float64     `json:"sexual"`
	Description   string      `json:"description"` // VNDB markup
	Length        int         `json:"length"`      // 1 very short .. 5 very long
	LengthMinutes int         `json:"lengthMinutes"`
	LengthVotes   int         `json:"lengthVotes"`
	Rating        float64     `json:"rating"`
	Votes         int         `json:"votes"`
	Developers    []string    `json:"developers"`
	Producers     []DProducer `json:"producers"` // developers and publishers, from the releases
	Tags          []DTag      `json:"tags"`
	Screenshots   []DShot     `json:"screenshots"`
	Relations     []DRelation `json:"relations"`
	Links         []DLink     `json:"links"`
	Staff         []DStaff    `json:"staff"`
	Characters    []DChar     `json:"characters"`
}

type DProducer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Developer bool   `json:"developer"`
	Publisher bool   `json:"publisher"`
}
type DTitle struct {
	Lang     string `json:"lang"`
	Title    string `json:"title"`
	Official bool   `json:"official"`
}
type DTag struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Rating   float64 `json:"rating"`
	Spoiler  int     `json:"spoiler"`
	Category string  `json:"category"`
}
type DShot struct {
	URL    string  `json:"url"`
	Thumb  string  `json:"thumb"`
	Sexual float64 `json:"sexual"`
}
type DRelation struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Relation string `json:"relation"`
	Official bool   `json:"official"`
}
type DLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}
type DStaff struct {
	Name string `json:"name"`
	Role string `json:"role"`
	Note string `json:"note"`
}
type DChar struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Original    string  `json:"original"`
	Image       string  `json:"image"`
	Sexual      float64 `json:"sexual"`
	Role        string  `json:"role"`
	Description string  `json:"description"`
}

const detailFields = "id,title,alttitle,titles.lang,titles.title,titles.official,aliases,devstatus,released,olang,languages,platforms," +
	"image.url,image.sexual,description,length,length_minutes,length_votes,rating,votecount,developers.name," +
	"tags.id,tags.name,tags.rating,tags.spoiler,tags.category,screenshots.url,screenshots.thumbnail,screenshots.sexual," +
	"relations.id,relations.title,relations.relation,relations.relation_official,extlinks.label,extlinks.url,staff.name,staff.role,staff.note"

func vndbDetail(id string) (*VNDetail, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !reVNID.MatchString(id) {
		return nil, fmt.Errorf("bad VNDB id")
	}
	raw, err := vndbPost("/vn", map[string]any{"filters": []any{"id", "=", id}, "fields": detailFields, "results": 1})
	if err != nil {
		return nil, err
	}
	var p struct {
		Results []struct {
			ID            string   `json:"id"`
			Title         string   `json:"title"`
			AltTitle      string   `json:"alttitle"`
			Titles        []DTitle `json:"titles"`
			Aliases       []string `json:"aliases"`
			DevStatus     int      `json:"devstatus"`
			Released      string   `json:"released"`
			OLang         string   `json:"olang"`
			Languages     []string `json:"languages"`
			Platforms     []string `json:"platforms"`
			Description   string   `json:"description"`
			Length        *int     `json:"length"`
			LengthMinutes *int     `json:"length_minutes"`
			LengthVotes   int      `json:"length_votes"`
			Rating        *float64 `json:"rating"`
			Votes         int      `json:"votecount"`
			Image         *struct {
				URL    string   `json:"url"`
				Sexual *float64 `json:"sexual"`
			} `json:"image"`
			Developers []struct {
				Name string `json:"name"`
			} `json:"developers"`
			Tags        []DTag `json:"tags"`
			Screenshots []struct {
				URL    string   `json:"url"`
				Thumb  string   `json:"thumbnail"`
				Sexual *float64 `json:"sexual"`
			} `json:"screenshots"`
			Relations []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Relation string `json:"relation"`
				Official bool   `json:"relation_official"`
			} `json:"relations"`
			Links []DLink  `json:"extlinks"`
			Staff []DStaff `json:"staff"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if len(p.Results) == 0 {
		return nil, fmt.Errorf("VNDB has no visual novel %s", id)
	}
	r := p.Results[0]
	d := &VNDetail{
		ID: r.ID, Title: r.Title, AltTitle: r.AltTitle, Titles: r.Titles, Aliases: r.Aliases, DevStatus: r.DevStatus,
		Released: r.Released, OLang: r.OLang, Languages: r.Languages, Platforms: r.Platforms, Description: r.Description,
		LengthVotes: r.LengthVotes, Votes: r.Votes, Tags: r.Tags, Links: r.Links, Staff: r.Staff,
		Developers: []string{}, Screenshots: []DShot{}, Relations: []DRelation{}, Characters: []DChar{},
	}
	if d.Titles == nil {
		d.Titles = []DTitle{}
	}
	if d.Aliases == nil {
		d.Aliases = []string{}
	}
	if d.Languages == nil {
		d.Languages = []string{}
	}
	if d.Platforms == nil {
		d.Platforms = []string{}
	}
	if d.Tags == nil {
		d.Tags = []DTag{}
	}
	if d.Links == nil {
		d.Links = []DLink{}
	}
	if d.Staff == nil {
		d.Staff = []DStaff{}
	}
	if r.Length != nil {
		d.Length = *r.Length
	}
	if r.LengthMinutes != nil {
		d.LengthMinutes = *r.LengthMinutes
	}
	if r.Rating != nil {
		d.Rating = *r.Rating
	}
	if r.Image != nil {
		d.Image = r.Image.URL
		if r.Image.Sexual != nil {
			d.Sexual = *r.Image.Sexual
		}
	}
	for _, x := range r.Developers {
		d.Developers = append(d.Developers, x.Name)
	}
	for _, s := range r.Screenshots {
		ds := DShot{URL: s.URL, Thumb: s.Thumb}
		if s.Sexual != nil {
			ds.Sexual = *s.Sexual
		}
		d.Screenshots = append(d.Screenshots, ds)
	}
	for _, x := range r.Relations {
		d.Relations = append(d.Relations, DRelation{x.ID, x.Title, x.Relation, x.Official})
	}
	d.Producers = vndbProducers(id)
	d.Characters = vndbCharacters(id) // best effort: the rest of the page is still useful without it
	return d, nil
}

// vndbProducers lists everyone who developed or published a release of the VN (best effort).
func vndbProducers(vn string) []DProducer {
	out := []DProducer{}
	raw, err := vndbPost("/release", map[string]any{
		"filters": []any{"vn", "=", []any{"id", "=", vn}},
		"fields":  "producers.id,producers.name,producers.developer,producers.publisher",
		"results": 100,
	})
	if err != nil {
		return out
	}
	var p struct {
		Results []struct {
			Producers []DProducer `json:"producers"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return out
	}
	idx := map[string]int{}
	for _, r := range p.Results {
		for _, x := range r.Producers {
			if x.ID == "" {
				continue
			}
			if i, ok := idx[x.ID]; ok {
				out[i].Developer = out[i].Developer || x.Developer
				out[i].Publisher = out[i].Publisher || x.Publisher
				continue
			}
			idx[x.ID] = len(out)
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Developer && !out[j].Developer })
	return out
}

func vndbCharacters(vn string) []DChar {
	raw, err := vndbPost("/character", map[string]any{
		"filters": []any{"vn", "=", []any{"id", "=", vn}},
		"fields":  "id,name,original,image.url,image.sexual,vns.id,vns.role,description",
		"results": 40,
	})
	if err != nil {
		return []DChar{}
	}
	var p struct {
		Results []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Original    string `json:"original"`
			Description string `json:"description"`
			Image       *struct {
				URL    string   `json:"url"`
				Sexual *float64 `json:"sexual"`
			} `json:"image"`
			VNs []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"vns"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return []DChar{}
	}
	rank := map[string]int{"main": 0, "primary": 1, "side": 2, "appears": 3}
	out := []DChar{}
	for _, c := range p.Results {
		dc := DChar{ID: c.ID, Name: c.Name, Original: c.Original, Description: c.Description}
		if c.Image != nil {
			dc.Image = c.Image.URL
			if c.Image.Sexual != nil {
				dc.Sexual = *c.Image.Sexual
			}
		}
		for _, v := range c.VNs {
			if v.ID == vn {
				dc.Role = v.Role
			}
		}
		out = append(out, dc)
	}
	// main characters first, stable otherwise
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rank[out[j].Role] < rank[out[j-1].Role]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Full details of one character, for the character window.

type CharDetail struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Original    string       `json:"original"`
	Aliases     []string     `json:"aliases"`
	Description string       `json:"description"`
	Image       string       `json:"image"`
	Sexual      float64      `json:"sexual"`
	Birthday    string       `json:"birthday"`
	Age         int          `json:"age"`
	Sex         string       `json:"sex"`
	BloodType   string       `json:"bloodType"`
	Height      int          `json:"height"`
	Weight      int          `json:"weight"`
	Bust        int          `json:"bust"`
	Waist       int          `json:"waist"`
	Hips        int          `json:"hips"`
	Cup         string       `json:"cup"`
	Traits      []CharTrait  `json:"traits"`
	VNs         []CharVNRole `json:"vns"`
}
type CharTrait struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	Spoiler int    `json:"spoiler"`
}
type CharVNRole struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Role    string `json:"role"`
	Spoiler int    `json:"spoiler"`
}

var reCharID = regexp.MustCompile(`(?i)^c\d{1,8}$`)

func vndbCharDetail(id string) (*CharDetail, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !reCharID.MatchString(id) {
		return nil, fmt.Errorf("bad VNDB character id")
	}
	raw, err := vndbPost("/character", map[string]any{
		"filters": []any{"id", "=", id}, "results": 1,
		"fields": "id,name,original,aliases,description,image.url,image.sexual,blood_type,height,weight,bust,waist,hips,cup,age,birthday,sex," +
			"traits.id,traits.name,traits.group_name,traits.spoiler,vns.id,vns.title,vns.role,vns.spoiler",
	})
	if err != nil {
		return nil, err
	}
	var p struct {
		Results []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Original    string   `json:"original"`
			Aliases     []string `json:"aliases"`
			Description string   `json:"description"`
			Image       *struct {
				URL    string   `json:"url"`
				Sexual *float64 `json:"sexual"`
			} `json:"image"`
			BloodType string   `json:"blood_type"`
			Height    *int     `json:"height"`
			Weight    *int     `json:"weight"`
			Bust      *int     `json:"bust"`
			Waist     *int     `json:"waist"`
			Hips      *int     `json:"hips"`
			Cup       string   `json:"cup"`
			Age       *int     `json:"age"`
			Birthday  []int    `json:"birthday"`
			Sex       []string `json:"sex"`
			Traits    []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Group   string `json:"group_name"`
				Spoiler int    `json:"spoiler"`
			} `json:"traits"`
			VNs []CharVNRole `json:"vns"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if len(p.Results) == 0 {
		return nil, fmt.Errorf("VNDB has no character %s", id)
	}
	r := p.Results[0]
	d := &CharDetail{ID: r.ID, Name: r.Name, Original: r.Original, Aliases: r.Aliases, Description: r.Description,
		BloodType: r.BloodType, Cup: r.Cup, Traits: []CharTrait{}, VNs: r.VNs}
	if d.Aliases == nil {
		d.Aliases = []string{}
	}
	if d.VNs == nil {
		d.VNs = []CharVNRole{}
	}
	for _, t := range r.Traits {
		d.Traits = append(d.Traits, CharTrait{t.ID, t.Name, t.Group, t.Spoiler})
	}
	iv := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	d.Height, d.Weight, d.Bust, d.Waist, d.Hips, d.Age = iv(r.Height), iv(r.Weight), iv(r.Bust), iv(r.Waist), iv(r.Hips), iv(r.Age)
	if len(r.Birthday) == 2 && r.Birthday[0] > 0 {
		d.Birthday = fmt.Sprintf("%d/%d", r.Birthday[0], r.Birthday[1])
		if r.Birthday[1] == 0 {
			d.Birthday = fmt.Sprint(r.Birthday[0])
		}
	}
	if len(r.Sex) > 0 {
		d.Sex = r.Sex[0]
	}
	if r.Image != nil {
		d.Image = r.Image.URL
		if r.Image.Sexual != nil {
			d.Sexual = *r.Image.Sexual
		}
	}
	return d, nil
}
