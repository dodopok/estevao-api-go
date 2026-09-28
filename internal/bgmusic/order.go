package bgmusic

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
)

const orderingTTL = 24 * time.Hour

// Entry ports TrackOrder::Entry.
type Entry struct {
	Track    *Track
	Facet    string
	Key      string
	Label    any
	OrderKey []int64
}

// Compare ports Array#<=> on two order keys of integers.
func Compare(a, b []int64) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// sortEntries ports sort_by(&:order_key) (every key ends in the track id,
// so the order is total).
func sortEntries(entries []*Entry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && Compare(entries[j].OrderKey, entries[j-1].OrderKey) < 0; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// Order ports TrackOrder: the request's parameters.
type Order struct {
	Date           civil.Date
	OfficeType     string
	PrayerBookCode string
	CatalogVersion string
	Language       string
}

type orderingItem struct {
	TrackID  int64   `json:"track_id"`
	Facet    string  `json:"facet"`
	Key      string  `json:"key"`
	Label    any     `json:"label"`
	OrderKey []int64 `json:"order_key"`
}

func (o *Order) language() string {
	if strings.TrimSpace(o.Language) == "" {
		return fallbackLocale
	}
	return o.Language
}

// Call ports TrackOrder#call on tracks (already loaded).
func (o *Order) Call(ctx context.Context, tracks []*Track) []*Entry {
	if len(tracks) == 0 {
		return nil
	}
	byID := map[int64]orderingItem{}
	for _, it := range o.cachedOrdering(ctx) {
		byID[it.TrackID] = it
	}
	var out []*Entry
	for _, t := range tracks {
		if it, ok := byID[t.ID]; ok {
			out = append(out, &Entry{Track: t, Facet: it.Facet, Key: it.Key, Label: it.Label, OrderKey: it.OrderKey})
		} else {
			out = append(out, o.unmatched(t))
		}
	}
	sortEntries(out)
	return out
}

func (o *Order) cachedOrdering(ctx context.Context) []orderingItem {
	key := strings.Join([]string{"background_music/ordering/v1", o.CatalogVersion, o.PrayerBookCode, o.language(),
		o.Date.ISO(), o.OfficeType}, "/")
	raw := rediscache.FetchJSON(ctx, key, orderingTTL, func() []byte {
		b, err := json.Marshal(o.buildOrdering(ctx))
		must(err)
		return b
	})
	var items []orderingItem
	must(json.Unmarshal(raw, &items))
	return items
}

type placement struct {
	ID       int64
	TrackID  int64
	Facet    string
	Key      string
	Code     *string
	Priority int64
}

func (o *Order) buildOrdering(ctx context.Context) []orderingItem {
	rows, err := db.Q().Query(ctx, `SELECT background_tracks.id, background_tracks.sort_order FROM background_tracks WHERE `+
		PublishedWhere, PublishedArgs()...)
	must(err)
	var catalog []*Track
	for rows.Next() {
		t := &Track{}
		must(rows.Scan(&t.ID, &t.SortOrder))
		catalog = append(catalog, t)
	}
	rows.Close()
	must(rows.Err())
	if len(catalog) == 0 {
		return []orderingItem{}
	}
	dayCtx := liturgical.NewCalendar(o.Date.Year(), o.PrayerBookCode).ContextFor(o.Date)
	byTrack := o.placementsFor(ctx, catalog, dayCtx)
	entries := make([]*Entry, 0, len(catalog))
	for _, t := range catalog {
		entries = append(entries, o.entryFor(t, byTrack[t.ID], dayCtx))
	}
	sortEntries(entries)
	items := make([]orderingItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, orderingItem{TrackID: e.Track.ID, Facet: e.Facet, Key: e.Key, Label: e.Label, OrderKey: e.OrderKey})
	}
	return items
}

// contextKeys ports TrackOrder#context_keys.
func (o *Order) contextKeys(c *liturgical.DayContext) map[string][]string {
	keys := map[string][]string{"office": {o.OfficeType}, "default": {"all"}}
	if c.Primary != nil {
		keys["celebration"] = []string{c.Primary.Key}
	}
	if c.BookSeason != "" {
		keys["book_season"] = []string{liturgical.BookSeasonSlug(c.BookSeason)}
	}
	if s, ok := liturgical.SeasonSlugs[c.Season]; ok && c.Season != "" {
		keys["season"] = []string{s}
	}
	return keys
}

func (o *Order) placementsFor(ctx context.Context, tracks []*Track, c *liturgical.DayContext) map[int64][]placement {
	desired := o.contextKeys(c)
	ids := make([]int64, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}
	rows, err := db.Q().Query(ctx, `SELECT id, track_id, facet, key, prayer_book_code, priority FROM background_track_placements
		WHERE track_id = ANY($1) AND facet IN ('celebration', 'book_season', 'season', 'office', 'default')
		AND (prayer_book_code IS NULL OR prayer_book_code = $2)`, ids, o.PrayerBookCode)
	must(err)
	defer rows.Close()
	out := map[int64][]placement{}
	for rows.Next() {
		var p placement
		must(rows.Scan(&p.ID, &p.TrackID, &p.Facet, &p.Key, &p.Code, &p.Priority))
		for _, k := range desired[p.Facet] {
			if k == p.Key {
				out[p.TrackID] = append(out[p.TrackID], p)
				break
			}
		}
	}
	must(rows.Err())
	return out
}

func (o *Order) scopeRank(p *placement) int64 {
	if p.Code != nil && *p.Code == o.PrayerBookCode {
		return 0
	}
	return 1
}

func (o *Order) best(ps []placement, keep func(*placement) bool, key func(*placement) []int64) *placement {
	var best *placement
	var bestKey []int64
	for i := range ps {
		p := &ps[i]
		if !keep(p) {
			continue
		}
		if k := key(p); best == nil || Compare(k, bestKey) < 0 {
			best, bestKey = p, k
		}
	}
	return best
}

func (o *Order) placementKey(p *placement) []int64 { return []int64{o.scopeRank(p), p.Priority, p.ID} }

func (o *Order) entryFor(t *Track, ps []placement, c *liturgical.DayContext) *Entry {
	facet := func(f string) func(*placement) bool { return func(p *placement) bool { return p.Facet == f } }
	celebration := o.best(ps, facet("celebration"), o.placementKey)
	season := o.best(ps, func(p *placement) bool { return p.Facet == "book_season" || p.Facet == "season" },
		func(p *placement) []int64 { return append([]int64{seasonSpecificity(p)}, o.placementKey(p)...) })
	office := o.best(ps, facet("office"), o.placementKey)
	def := o.best(ps, facet("default"), o.placementKey)
	switch {
	case celebration != nil:
		return o.build(t, celebration, c, 0, season, office)
	case season != nil && office != nil:
		return o.build(t, season, c, 1, season, office)
	case season != nil:
		return o.build(t, season, c, 2, season, office)
	case office != nil:
		return o.build(t, office, c, 3, season, office)
	case def != nil:
		return o.build(t, def, c, 4, season, office)
	}
	return o.unmatched(t)
}

func seasonSpecificity(p *placement) int64 {
	if p.Facet == "book_season" {
		return 0
	}
	return 1
}

func (o *Order) unmatched(t *Track) *Entry {
	return &Entry{Track: t, Facet: "default", Key: "all", Label: o.recommendedLabel(),
		OrderKey: []int64{5, 1, 100_000, t.SortOrder, t.ID}}
}

func (o *Order) build(t *Track, p *placement, c *liturgical.DayContext, rank int64, season, office *placement) *Entry {
	var key []int64
	switch rank {
	case 0, 4:
		key = []int64{rank, o.scopeRank(p), p.Priority, t.SortOrder, t.ID}
	case 1:
		key = []int64{rank, seasonSpecificity(season), o.scopeRank(season), season.Priority, o.scopeRank(office),
			office.Priority, t.SortOrder, t.ID}
	case 2:
		key = []int64{rank, seasonSpecificity(season), o.scopeRank(season), season.Priority, t.SortOrder, t.ID}
	case 3:
		key = []int64{rank, o.scopeRank(office), office.Priority, t.SortOrder, t.ID}
	}
	return &Entry{Track: t, Facet: p.Facet, Key: p.Key, Label: o.labelFor(p.Facet, p.Key, c), OrderKey: key}
}

func (o *Order) labelFor(facet, key string, c *liturgical.DayContext) any {
	switch facet {
	case "celebration":
		if c.Primary != nil {
			return c.Primary.Name
		}
		return key
	case "book_season":
		return o.seasonLabel(key, c.BookSeason)
	case "season":
		return o.seasonLabel(key, c.Season)
	case "office":
		return Translate("offices."+key, o.language(), key)
	}
	return o.recommendedLabel()
}

func (o *Order) recommendedLabel() any {
	return Translate("recommended", o.language(), "Música recomendada")
}

func (o *Order) seasonLabel(key, fallback string) any {
	slug := strings.TrimPrefix(key, "season-")
	if strings.TrimSpace(fallback) == "" {
		fallback = key
	}
	return Translate("seasons."+slug, o.language(), fallback)
}

// Recommendation ports BackgroundMusic::Recommend::Result.
type Recommendation struct {
	Track *Track
	Facet string
	Key   string
	Label any
}

type recommendPayload struct {
	TrackID *int64 `json:"track_id"`
	Facet   string `json:"facet"`
	Key     string `json:"key"`
	Label   any    `json:"label"`
}

// Recommend ports BackgroundMusic::Recommend.call (cached a day).
func (o *Order) Recommend(ctx context.Context) *Recommendation {
	key := strings.Join([]string{"background_music/recommendation/v1", o.CatalogVersion, o.PrayerBookCode, o.Language,
		o.Date.ISO(), o.OfficeType}, "/")
	raw := rediscache.FetchJSON(ctx, key, orderingTTL, func() []byte {
		p := recommendPayload{}
		for _, e := range o.Call(ctx, LoadTracks(ctx, "", "", "")) {
			if e.OrderKey[0] < 5 {
				id := e.Track.ID
				p = recommendPayload{TrackID: &id, Facet: e.Facet, Key: e.Key, Label: e.Label}
				break
			}
		}
		b, err := json.Marshal(p)
		must(err)
		return b
	})
	var p recommendPayload
	must(json.Unmarshal(raw, &p))
	if p.TrackID == nil {
		return nil
	}
	t := PublishedTrack(ctx, *p.TrackID)
	if t == nil {
		return nil
	}
	return &Recommendation{Track: t, Facet: p.Facet, Key: p.Key, Label: p.Label}
}
