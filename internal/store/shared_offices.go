package store

import (
	"context"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// SharedOffice is a shared_offices row.
type SharedOffice struct {
	ID             int64
	ShortCode      string
	PrayerBookCode string
	OfficeType     string
	Date           civil.Date
	Seed           int
	Preferences    *rb.Map
	UserID         *int64
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

const sharedOfficeColumns = `id, short_code, prayer_book_code, office_type, date, seed, preferences, user_id, expires_at, created_at, updated_at`

func scanSharedOffice(row interface{ Scan(...any) error }) (*SharedOffice, error) {
	var s SharedOffice
	var prefs []byte
	var d time.Time
	if err := row.Scan(&s.ID, &s.ShortCode, &s.PrayerBookCode, &s.OfficeType, &d, &s.Seed, &prefs, &s.UserID,
		&s.ExpiresAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Date = civil.FromTime(d.UTC())
	s.Preferences = rb.NewMap()
	if len(prefs) > 0 {
		if v, err := rb.ParseJSON(prefs); err == nil {
			if m, ok := v.(*rb.Map); ok {
				s.Preferences = m
			}
		}
	}
	return &s, nil
}

// ActiveSharedOffice ports SharedOffice.find_active.
func ActiveSharedOffice(ctx context.Context, code string) (*SharedOffice, error) {
	row := db.Q().QueryRow(ctx, `SELECT `+sharedOfficeColumns+` FROM shared_offices WHERE expires_at > $1 AND short_code = $2 LIMIT 1`, time.Now().UTC(), code)
	s, err := scanSharedOffice(row)
	if db.NoRows(err) {
		return nil, nil
	}
	return s, err
}

// ExpiredSharedOfficeExists ports SharedOffice.expired.find_by(short_code:).
func ExpiredSharedOfficeExists(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shared_offices WHERE expires_at <= $1 AND short_code = $2)`, time.Now().UTC(), code).Scan(&ok)
	return ok, err
}
