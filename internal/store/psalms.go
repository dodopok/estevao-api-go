package store

import (
	"context"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Psalm is Psalm::CachedPsalm (the attributes the offices read).
type Psalm struct {
	Number int
	Verses any // parsed jsonb (nil when NULL)
}

// FormattedVerses ports #formatted_verses: {number, text, pointer} per verse,
// or nothing when verses is not an Array.
func (p *Psalm) FormattedVerses() []*rb.Map {
	list, ok := p.Verses.([]any)
	if !ok {
		return nil
	}
	out := make([]*rb.Map, len(list))
	for i, v := range list {
		m, _ := v.(*rb.Map)
		get := func(k string) any {
			if m == nil {
				return nil
			}
			return m.Get(k)
		}
		out[i] = rb.M("number", get("number"), "text", get("text"), "pointer", get("hebrew_pointer"))
	}
	return out
}

type psalmsEntry struct {
	version time.Time
	value   map[int]*Psalm
}

var (
	psalmsMu    sync.Mutex
	psalmsCache = map[int64]psalmsEntry{}
)

// PsalmsFor ports Psalm.psalms_cache_for: the book's psalms by number (a
// later duplicate number wins).
func PsalmsFor(ctx context.Context, pb *PrayerBook) map[int]*Psalm {
	if pb == nil {
		return map[int]*Psalm{}
	}
	psalmsMu.Lock()
	if e, ok := psalmsCache[pb.ID]; ok && e.version.Equal(pb.UpdatedAt) {
		psalmsMu.Unlock()
		return e.value
	}
	psalmsMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT "psalms"."number", "psalms"."verses"::text FROM "psalms" WHERE "psalms"."prayer_book_id" = $1`, pb.ID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := map[int]*Psalm{}
	for rows.Next() {
		var number int
		var verses *string
		if err := rows.Scan(&number, &verses); err != nil {
			panic(err)
		}
		p := &Psalm{Number: number}
		if verses != nil {
			v, err := rb.ParseJSON([]byte(*verses))
			if err != nil {
				panic(err)
			}
			p.Verses = v
		}
		out[number] = p
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	psalmsMu.Lock()
	psalmsCache[pb.ID] = psalmsEntry{pb.UpdatedAt, out}
	psalmsMu.Unlock()
	return out
}
