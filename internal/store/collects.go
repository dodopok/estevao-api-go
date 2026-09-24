package store

import (
	"context"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Collect is a cached collects row (Collect::CachedCollect).
type Collect struct {
	ID            int64
	Text          *string
	Preface       *string
	CelebrationID *int64
	SeasonID      *int64
	SundayRef     *string
	PrayerBookID  int64
	LanguageStyle *string
}

// CollectIndex ports Collect.collects_cache_for: groups keep pluck order.
type CollectIndex struct {
	ByCelebration map[int64][]*Collect
	BySunday      map[string][]*Collect
}

type collectsEntry struct {
	version time.Time
	value   *CollectIndex
}

var (
	collectsMu    sync.Mutex
	collectsCache = map[int64]collectsEntry{}
)

// CollectsFor returns the book's collect index (empty for a nil book).
func CollectsFor(ctx context.Context, pb *PrayerBook) *CollectIndex {
	if pb == nil {
		return &CollectIndex{ByCelebration: map[int64][]*Collect{}, BySunday: map[string][]*Collect{}}
	}
	collectsMu.Lock()
	if e, ok := collectsCache[pb.ID]; ok && e.version.Equal(pb.UpdatedAt) {
		collectsMu.Unlock()
		return e.value
	}
	collectsMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT "collects"."id", "collects"."text", "collects"."preface", "collects"."celebration_id",
"collects"."season_id", "collects"."sunday_reference", "collects"."prayer_book_id", "collects"."language_style"
FROM "collects" WHERE "collects"."prayer_book_id" = $1`, pb.ID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	idx := &CollectIndex{ByCelebration: map[int64][]*Collect{}, BySunday: map[string][]*Collect{}}
	for rows.Next() {
		var c Collect
		if err := rows.Scan(&c.ID, &c.Text, &c.Preface, &c.CelebrationID, &c.SeasonID, &c.SundayRef, &c.PrayerBookID, &c.LanguageStyle); err != nil {
			panic(err)
		}
		if c.CelebrationID != nil {
			idx.ByCelebration[*c.CelebrationID] = append(idx.ByCelebration[*c.CelebrationID], &c)
		}
		if c.SundayRef != nil && !rb.BlankString(*c.SundayRef) {
			idx.BySunday[*c.SundayRef] = append(idx.BySunday[*c.SundayRef], &c)
		}
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	collectsMu.Lock()
	collectsCache[pb.ID] = collectsEntry{pb.UpdatedAt, idx}
	collectsMu.Unlock()
	return idx
}

// LiturgicalText is LiturgicalText::CachedText.
type LiturgicalText struct {
	ID           int64
	Slug         string
	Content      string
	Title        *string
	Reference    *string
	Category     string
	AudioURLs    *rb.Map
	PrayerBookID int64
}

const liturgicalTextColumns = `"liturgical_texts"."id", "liturgical_texts"."slug", "liturgical_texts"."content",
"liturgical_texts"."title", "liturgical_texts"."reference", "liturgical_texts"."category",
"liturgical_texts"."audio_urls", "liturgical_texts"."prayer_book_id"`

func scanLiturgicalText(row interface{ Scan(...any) error }) (*LiturgicalText, error) {
	var t LiturgicalText
	var audio []byte
	if err := row.Scan(&t.ID, &t.Slug, &t.Content, &t.Title, &t.Reference, &t.Category, &audio, &t.PrayerBookID); err != nil {
		return nil, err
	}
	if v, err := rb.ParseJSON(audio); err == nil {
		t.AudioURLs, _ = v.(*rb.Map)
	}
	return &t, nil
}

// FindLiturgicalText ports LiturgicalText.find_text (where(...).first).
func FindLiturgicalText(ctx context.Context, pb *PrayerBook, slug string) *LiturgicalText {
	if pb == nil {
		return nil
	}
	row := db.Q().QueryRow(ctx, `SELECT `+liturgicalTextColumns+` FROM "liturgical_texts"
WHERE "liturgical_texts"."prayer_book_id" = $1 AND "liturgical_texts"."slug" = $2
ORDER BY "liturgical_texts"."id" ASC LIMIT 1`, pb.ID, slug)
	t, err := scanLiturgicalText(row)
	if db.NoRows(err) {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return t
}

type textsEntry struct {
	version time.Time
	value   map[string]*LiturgicalText
}

var (
	textsMu    sync.Mutex
	textsCache = map[int64]textsEntry{}
)

// LiturgicalTextsFor ports LiturgicalText.texts_cache_for (slug index; a
// later duplicate slug wins, as in each_with_object).
func LiturgicalTextsFor(ctx context.Context, pb *PrayerBook) map[string]*LiturgicalText {
	if pb == nil {
		return map[string]*LiturgicalText{}
	}
	textsMu.Lock()
	if e, ok := textsCache[pb.ID]; ok && e.version.Equal(pb.UpdatedAt) {
		textsMu.Unlock()
		return e.value
	}
	textsMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT `+liturgicalTextColumns+` FROM "liturgical_texts" WHERE "liturgical_texts"."prayer_book_id" = $1`, pb.ID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := map[string]*LiturgicalText{}
	for rows.Next() {
		t, err := scanLiturgicalText(rows)
		if err != nil {
			panic(err)
		}
		out[t.Slug] = t
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	textsMu.Lock()
	textsCache[pb.ID] = textsEntry{pb.UpdatedAt, out}
	textsMu.Unlock()
	return out
}

// CelebrationNameByID ports Celebration.where(id:).pick(:name).
func CelebrationNameByID(ctx context.Context, id int64) *string {
	var name *string
	err := db.Q().QueryRow(ctx, `SELECT "celebrations"."name" FROM "celebrations" WHERE "celebrations"."id" = $1 LIMIT 1`, id).Scan(&name)
	if db.NoRows(err) {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return name
}

// BookCelebrationWhere ports prayer_book.celebrations.find_by(...) for a
// fixed date (month > 0) or a calculation rule.
func BookCelebrationWhere(ctx context.Context, pb *PrayerBook, month, day int, calculationRule string) *liturgical.Celebration {
	if pb == nil {
		return nil
	}
	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
		Err() error
	}
	var err error
	if month > 0 {
		rows, err = db.Q().Query(ctx, `SELECT `+celebrationColumns+` FROM celebrations WHERE celebrations.prayer_book_id = $1 AND celebrations.fixed_month = $2 AND celebrations.fixed_day = $3 LIMIT 1`, pb.ID, month, day)
	} else {
		rows, err = db.Q().Query(ctx, `SELECT `+celebrationColumns+` FROM celebrations WHERE celebrations.prayer_book_id = $1 AND celebrations.calculation_rule = $2 LIMIT 1`, pb.ID, calculationRule)
	}
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil
	}
	c, err := ScanCelebration(rows)
	if err != nil {
		panic(err)
	}
	return c
}
