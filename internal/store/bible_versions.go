package store

import (
	"context"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/langs"
)

// BibleVersion is a bible_versions row.
type BibleVersion struct {
	ID                  int64
	Code                string
	CreatedAt           time.Time
	Description         *string
	FullName            *string
	IsActive            *bool
	IsRecommended       *bool
	Language            *string
	License             *string
	Name                string
	Publisher           *string
	UpdatedAt           time.Time
	VersificationSystem string
	Year                *int
}

const bibleVersionColumns = `id, code, created_at, description, full_name, is_active, is_recommended, language,
license, name, publisher, updated_at, versification_system, year`

func scanBibleVersion(row interface{ Scan(...any) error }) (*BibleVersion, error) {
	var b BibleVersion
	err := row.Scan(&b.ID, &b.Code, &b.CreatedAt, &b.Description, &b.FullName, &b.IsActive, &b.IsRecommended,
		&b.Language, &b.License, &b.Name, &b.Publisher, &b.UpdatedAt, &b.VersificationSystem, &b.Year)
	return &b, err
}

// LanguageS returns language or "".
func (b *BibleVersion) LanguageS() string {
	if b.Language == nil {
		return ""
	}
	return *b.Language
}

// BibleVersionByCode ports BibleVersion.find_by_code (case-insensitive code).
func BibleVersionByCode(ctx context.Context, code string) (*BibleVersion, error) {
	row := db.Q().QueryRow(ctx, `SELECT `+bibleVersionColumns+` FROM bible_versions WHERE code = $1 LIMIT 1`, strings.ToLower(code))
	b, err := scanBibleVersion(row)
	if db.NoRows(err) {
		return nil, nil
	}
	return b, err
}

// BibleVersionsWhere runs an arbitrary filtered/ordered query.
func BibleVersionsWhere(ctx context.Context, tail string, args ...any) ([]*BibleVersion, error) {
	rows, err := db.Q().Query(ctx, `SELECT `+bibleVersionColumns+` FROM bible_versions `+tail, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BibleVersion
	for rows.Next() {
		b, err := scanBibleVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DefaultBibleVersionFor ports BibleVersion.default_for_language.
func DefaultBibleVersionFor(ctx context.Context, language string) (*BibleVersion, error) {
	for _, cand := range langs.BibleLanguageCandidatesFor(langs.Normalize(language)) {
		list, err := BibleVersionsWhere(ctx, `WHERE is_active = TRUE AND language = $1 ORDER BY is_recommended DESC, name ASC LIMIT 1`, cand)
		if err != nil {
			return nil, err
		}
		if len(list) > 0 {
			return list[0], nil
		}
	}
	return nil, nil
}

// ActiveBibleVersionExists ports BibleVersion.active_exists_by_code?.
func ActiveBibleVersionExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bible_versions WHERE is_active = TRUE AND code = $1)`, strings.ToLower(code)).Scan(&exists)
	return exists, err
}
