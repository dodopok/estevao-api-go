// Package calgrid ports Calendar::GridCache and Calendar::CacheVersion: the
// cache behind the calendar month, year and overview endpoints, shared by
// the requests and CalendarWarmerJob so the warmer fills exactly the keys
// requests read.
//
// The payloads come from celebrations and code, never from texts, collects
// or readings, so they are versioned by a digest of the book's celebrations
// instead of by PrayerBook#updated_at. As every Go cache entry, they are JSON
// under "go/" (rediscache.FetchJSON), beside the Rails entries of the same
// name. A change to the grid code must bump the scope versions below.
package calgrid

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// TTLs of Cacheable::TTL (ActiveSupport's 1.year and 30.days).
const (
	CalendarYearTTL = 31556952 * time.Second
	StaticDataTTL   = 30 * 24 * time.Hour
)

var (
	monthScope = []any{"month", "v3"}
	yearScope  = []any{"year", "v3"}
)

// Grid is Calendar::GridCache for one Prayer Book.
type Grid struct {
	ctx     context.Context
	book    *store.PrayerBook
	version string
}

// New ports GridCache.new(prayer_book).
func New(ctx context.Context, book *store.PrayerBook) *Grid {
	return &Grid{ctx: ctx, book: book}
}

// Book is the Prayer Book the grid belongs to.
func (g *Grid) Book() *store.PrayerBook { return g.book }

// Key ports GridCache#key.
func (g *Grid) Key(scope ...any) string {
	parts := []string{"calendar"}
	for _, s := range scope {
		parts = append(parts, fmt.Sprint(s))
	}
	parts = append(parts, g.book.Code, g.book.Language, "cal_"+g.Version())
	return strings.Join(parts, "/")
}

// Fetch ports GridCache#fetch with the default CALENDAR_YEAR expiry: the
// JSON of compute(), cached under the scope.
func (g *Grid) Fetch(compute func() any, scope ...any) []byte {
	return rediscache.FetchJSON(g.ctx, g.Key(scope...), CalendarYearTTL, func() []byte { return rb.JSON(compute()) })
}

// Month ports GridCache#month; cal may be nil.
func (g *Grid) Month(year, month int, cal *liturgical.Calendar) []byte {
	return g.Fetch(func() any { return g.compactDays(year, []int{month}, cal) }, append(monthScope, year, month)...)
}

// MonthCached ports GridCache#month_cached?.
func (g *Grid) MonthCached(year, month int) bool {
	return rediscache.ExistsJSON(g.ctx, g.Key(append(monthScope, year, month)...))
}

// Year ports GridCache#year.
func (g *Grid) Year(year int) []byte {
	return g.Fetch(func() any { return g.compactDays(year, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, nil) }, append(yearScope, year)...)
}

// Version ports CacheVersion.for(prayer_book): the digest of the book's
// celebrations, itself cached under the book's updated_at (a saved
// celebration touches its book), so a touch that only moved texts
// recomputes the same digest and keeps every grid key.
func (g *Grid) Version() string {
	if g.version == "" {
		key := "calendar_version/" + g.book.Code + "/pb_" + rediscache.TimestampVersion(&g.book.UpdatedAt)
		g.version = string(rediscache.FetchJSON(g.ctx, key, StaticDataTTL, func() []byte { return []byte(g.digest()) }))
	}
	return g.version
}

// digest hashes every column of the book's celebrations but the timestamps,
// in id order. The Rails digest is computed over Ruby's JSON of the same
// rows; the two never meet, since each side keys only its own entries.
func (g *Grid) digest() string {
	var sum []byte
	err := db.Q().QueryRow(g.ctx, `
		SELECT sha256(convert_to(coalesce(string_agg((to_jsonb(c) - 'created_at' - 'updated_at')::text, E'\n' ORDER BY c.id), ''), 'UTF8'))
		FROM celebrations c WHERE c.prayer_book_id = $1`, g.book.ID).Scan(&sum)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(sum)[:16]
}

func (g *Grid) compactDays(year int, months []int, cal *liturgical.Calendar) []any {
	if cal == nil {
		bc, err := store.CelebrationsForBook(g.ctx, g.book)
		if err != nil {
			panic(err)
		}
		cal = liturgical.NewCalendarWith(year, g.book.Code, bc)
	}
	out := []any{}
	for _, m := range months {
		for d := 1; d <= civil.DaysInMonth(year, m); d++ {
			date := civil.MustNew(year, m, d)
			out = append(out, CompactDay(cal.DayInfo(date), cal.ContextFor(date).FastObservance, g.book.Language))
		}
	}
	return out
}

// CompactDay ports Calendar::CompactDayPayload#call.
func CompactDay(info *rb.Map, fast *liturgical.FastObservance, language string) *rb.Map {
	dmy := rb.ToS(info.Get("date"))
	date := dmy[6:10] + "-" + dmy[3:5] + "-" + dmy[0:2]
	var celebrationName any
	if cel, ok := info.Get("celebration").(*rb.Map); ok {
		celebrationName = cel.Get("name")
	}
	var weekName any
	if sn := info.Get("sunday_name"); sn != nil {
		s := rb.ToS(sn)
		weekName = *liturgical.TranslateSundayName(&s, language)
	} else {
		var descs []string
		for _, d := range info.Get("description").([]any) {
			descs = append(descs, rb.ToS(d))
		}
		week := ""
		for _, d := range descs {
			if strings.Contains(d, "Semana após") {
				week = d
				break
			}
		}
		if week == "" {
			for _, d := range descs {
				if strings.Contains(d, "Semana") || strings.Contains(d, "Oitava") {
					week = d
					break
				}
			}
		}
		if week != "" {
			weekName = liturgical.TranslateDescription(week, language)
		}
	}
	return rb.M(
		"date", date,
		"color", info.Get("color"),
		"season_post_slug", info.Get("season_post_slug"),
		"book_season_post_slug", info.Get("book_season_post_slug"),
		"fast_day", info.Get("fast_day"),
		"fast_observance", liturgical.PresentFastObservance(fast, language),
		"celebration_name", celebrationName,
		"week_name", weekName,
	)
}
