// Package store holds the SQL access to the Rails-managed schema. Queries
// keep the ORDER BY (or its absence) of the ActiveRecord calls they port,
// because ordering is observable in responses.
package store

import (
	"context"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// PrayerBook is a prayer_books row.
type PrayerBook struct {
	ID              int64
	Code            string
	Name            *string
	Description     *string
	Language        string
	Year            *int
	Jurisdiction    *string
	IsRecommended   *bool
	PremiumRequired bool
	ExternalOnly    bool
	Order           int
	Features        *rb.Map
	ImageURL        *string
	ThumbnailURL    *string
	PdfURL          *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const prayerBookColumns = `id, code, name, description, language, year, jurisdiction, is_recommended,
premium_required, external_only, "order", features, image_url, thumbnail_url, pdf_url, created_at, updated_at`

func scanPrayerBook(row interface{ Scan(...any) error }) (*PrayerBook, error) {
	var pb PrayerBook
	var features []byte
	if err := row.Scan(&pb.ID, &pb.Code, &pb.Name, &pb.Description, &pb.Language, &pb.Year, &pb.Jurisdiction,
		&pb.IsRecommended, &pb.PremiumRequired, &pb.ExternalOnly, &pb.Order, &features, &pb.ImageURL,
		&pb.ThumbnailURL, &pb.PdfURL, &pb.CreatedAt, &pb.UpdatedAt); err != nil {
		return nil, err
	}
	pb.Features = rb.NewMap()
	if len(features) > 0 {
		if v, err := rb.ParseJSON(features); err == nil {
			if m, ok := v.(*rb.Map); ok {
				pb.Features = m
			}
		}
	}
	return &pb, nil
}

// PrayerBookByCode ports PrayerBook.find_by_code (nil when absent). The
// default scope orders by "order", which does not change a lookup by code.
func PrayerBookByCode(ctx context.Context, code string) (*PrayerBook, error) {
	row := db.Q().QueryRow(ctx, `SELECT `+prayerBookColumns+` FROM prayer_books WHERE code = $1 ORDER BY "order" ASC LIMIT 1`, code)
	pb, err := scanPrayerBook(row)
	if db.NoRows(err) {
		return nil, nil
	}
	return pb, err
}

// PrayerBooksWhere loads prayer books with the default scope order.
func PrayerBooksWhere(ctx context.Context, where string, args ...any) ([]*PrayerBook, error) {
	q := `SELECT ` + prayerBookColumns + ` FROM prayer_books`
	if where != "" {
		q += " WHERE " + where
	}
	q += ` ORDER BY "order" ASC`
	rows, err := db.Q().Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PrayerBook
	for rows.Next() {
		pb, err := scanPrayerBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	return out, rows.Err()
}

// DefaultPrayerBook ports PrayerBook.default.first (is_recommended, "order").
func DefaultPrayerBook(ctx context.Context) (*PrayerBook, error) {
	list, err := PrayerBooksWhere(ctx, "is_recommended = TRUE")
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

// --- celebrations -------------------------------------------------------

type celebrationsEntry struct {
	version time.Time
	value   *liturgical.BookCelebrations
}

var (
	celebrationsMu    sync.Mutex
	celebrationsCache = map[int64]celebrationsEntry{}
)

const celebrationColumns = `id, name, latin_name, celebration_type, rank, liturgical_color, description,
description_year, fixed_month, fixed_day, movable, can_be_transferred, calculation_rule, post_slug,
person_type, gender, prayer_book_id`

// ScanCelebration reads a celebrations row selected with celebrationColumns.
func ScanCelebration(row interface{ Scan(...any) error }) (*liturgical.Celebration, error) {
	var c liturgical.Celebration
	var ctype, ptype, gender int
	var postSlug *string
	if err := row.Scan(&c.ID, &c.Name, &c.LatinName, &ctype, &c.Rank, &c.LiturgicalColor, &c.Description,
		&c.DescriptionYear, &c.FixedMonth, &c.FixedDay, &c.Movable, &c.CanBeTransferred, &c.CalculationRule,
		&postSlug, &ptype, &gender, &c.PrayerBookID); err != nil {
		return nil, err
	}
	c.CelebrationType = liturgical.CelebrationTypes[ctype]
	c.PersonType = liturgical.PersonTypes[ptype]
	c.Gender = liturgical.Genders[gender]
	if postSlug == nil {
		c.PostSlugNull = true
	} else {
		c.PostSlug = *postSlug
	}
	return &c, nil
}

// CelebrationsForBook returns a book's celebrations in table order, cached
// per Prayer Book version (a celebration change touches its book).
func CelebrationsForBook(ctx context.Context, pb *PrayerBook) (*liturgical.BookCelebrations, error) {
	if pb == nil {
		return &liturgical.BookCelebrations{}, nil
	}
	celebrationsMu.Lock()
	if e, ok := celebrationsCache[pb.ID]; ok && e.version.Equal(pb.UpdatedAt) {
		celebrationsMu.Unlock()
		return e.value, nil
	}
	celebrationsMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT `+celebrationColumns+` FROM celebrations WHERE prayer_book_id = $1`, pb.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bc := &liturgical.BookCelebrations{}
	for rows.Next() {
		c, err := ScanCelebration(rows)
		if err != nil {
			return nil, err
		}
		bc.All = append(bc.All, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	celebrationsMu.Lock()
	celebrationsCache[pb.ID] = celebrationsEntry{pb.UpdatedAt, bc}
	celebrationsMu.Unlock()
	return bc, nil
}
