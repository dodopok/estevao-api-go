// Package bgmusic ports the background music catalogue the app plays under
// the Daily Office: BackgroundTrack and its categories, assets and
// placements, BackgroundMusic::Catalog, TrackOrder, Recommend and
// CategoryNames.
package bgmusic

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/i18n"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
)

// Kinds ports BackgroundTrack::KINDS.
var Kinds = []string{"instrumental", "chant", "choral"}

const (
	// DefaultProfile ports BackgroundMusic::Storage::DEFAULT_PROFILE.
	DefaultProfile = "mp3_96"
	fallbackLocale = "pt-BR"
	// incompatibleLicense ports INCOMPATIBLE_LICENSE_SQL_PATTERN.
	incompatibleLicense = `(non[- ]?commercial|(^|[- ])nc(\s|$)|no[- ]?derivatives|(^|[- ])nd(\s|$))`
)

// PublishedWhere ports the BackgroundTrack.published scope; its one bind
// value is PublishedArgs.
const PublishedWhere = `background_tracks.status = 'ready' AND background_tracks.background_eligible = TRUE
 AND background_tracks.published_at IS NOT NULL AND background_tracks.license_type IS NOT NULL
 AND background_tracks.license_url IS NOT NULL AND background_tracks.source_url IS NOT NULL
 AND background_tracks.credit_text IS NOT NULL AND background_tracks.notes IS NOT NULL
 AND (background_tracks.license_type !~* $1)
 AND background_tracks.id IN (SELECT background_track_assets.track_id FROM background_track_assets
   WHERE background_track_assets.status = 'ready' AND background_track_assets.active = TRUE)`

// PublishedArgs are the bind values of PublishedWhere.
func PublishedArgs() []any { return []any{incompatibleLicense} }

// Category is a BackgroundCategory.
type Category struct {
	ID       int64
	Slug     string
	Name     string
	Position int
}

// Track is a BackgroundTrack with its categories.
type Track struct {
	ID             int64
	Slug           string
	Title          string
	Composer       *string
	Performer      *string
	Kind           string
	DurationMs     int64
	LoopApproved   bool
	SortOrder      int64
	CreditText     *string
	LicenseType    *string
	LicenseURL     *string
	LicenseVersion *string
	SourceURL      *string
	Notes          *string
	Categories     []Category
}

// Asset is a BackgroundTrackAsset.
type Asset struct {
	ID          int64
	Profile     string
	ObjectKey   string
	MimeType    string
	Bytes       int64
	DurationMs  int64
	Sha256      string
	LoopStartMs *int64
	LoopEndMs   *int64
	Status      string
	Active      bool
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

const trackColumns = `background_tracks.id, background_tracks.slug, background_tracks.title, background_tracks.composer,
 background_tracks.performer, background_tracks.kind, background_tracks.duration_ms, background_tracks.loop_approved,
 background_tracks.sort_order, background_tracks.credit_text, background_tracks.license_type, background_tracks.license_url,
 background_tracks.license_version, background_tracks.source_url, background_tracks.notes`

// LoadTracks runs `SELECT background_tracks.* FROM background_tracks <from>
// WHERE <published> AND <where> <suffix>`; args continue after the
// published scope's own bind value.
func LoadTracks(ctx context.Context, from, where, suffix string, args ...any) []*Track {
	sql := "SELECT " + trackColumns + " FROM background_tracks " + from + " WHERE " + PublishedWhere
	if where != "" {
		sql += " AND " + where
	}
	rows, err := db.Q().Query(ctx, sql+" "+suffix, append(PublishedArgs(), args...)...)
	must(err)
	defer rows.Close()
	var out []*Track
	for rows.Next() {
		t := &Track{}
		must(rows.Scan(&t.ID, &t.Slug, &t.Title, &t.Composer, &t.Performer, &t.Kind, &t.DurationMs, &t.LoopApproved,
			&t.SortOrder, &t.CreditText, &t.LicenseType, &t.LicenseURL, &t.LicenseVersion, &t.SourceURL, &t.Notes))
		out = append(out, t)
	}
	must(rows.Err())
	return out
}

// PublishedTrack ports BackgroundTrack.published.includes(:categories).find_by(id:).
func PublishedTrack(ctx context.Context, id int64) *Track {
	list := LoadTracks(ctx, "", "background_tracks.id = $2", "LIMIT 1", id)
	if len(list) == 0 {
		return nil
	}
	LoadCategories(ctx, list, nil)
	return list[0]
}

// LoadCategories preloads each track's categories; only, when not nil,
// keeps the one category an eager-loading join filtered on.
func LoadCategories(ctx context.Context, tracks []*Track, only *Category) {
	if len(tracks) == 0 {
		return
	}
	if only != nil {
		for _, t := range tracks {
			t.Categories = []Category{*only}
		}
		return
	}
	ids := make([]int64, len(tracks))
	byID := map[int64]*Track{}
	for i, t := range tracks {
		ids[i] = t.ID
		byID[t.ID] = t
		t.Categories = nil
	}
	rows, err := db.Q().Query(ctx, `SELECT tc.track_id, c.id, c.slug, c.name, c.position
		FROM background_track_categories tc JOIN background_categories c ON c.id = tc.category_id
		WHERE tc.track_id = ANY($1)`, ids)
	must(err)
	defer rows.Close()
	for rows.Next() {
		var tid int64
		var c Category
		must(rows.Scan(&tid, &c.ID, &c.Slug, &c.Name, &c.Position))
		byID[tid].Categories = append(byID[tid].Categories, c)
	}
	must(rows.Err())
}

// CategoryBySlug ports BackgroundCategory.find_by(slug:).
func CategoryBySlug(ctx context.Context, slug string) *Category {
	var c Category
	err := db.Q().QueryRow(ctx, `SELECT id, slug, name, position FROM background_categories WHERE slug = $1 LIMIT 1`, slug).
		Scan(&c.ID, &c.Slug, &c.Name, &c.Position)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return &c
}

// OrderedCategories ports BackgroundCategory.ordered.
func OrderedCategories(ctx context.Context) []Category {
	rows, err := db.Q().Query(ctx, `SELECT id, slug, name, position FROM background_categories ORDER BY position ASC, id ASC`)
	must(err)
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		must(rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Position))
		out = append(out, c)
	}
	must(rows.Err())
	return out
}

// PreferredAsset ports BackgroundTrack#preferred_asset(profile: mp3_96).
func PreferredAsset(ctx context.Context, trackID int64) *Asset {
	rows, err := db.Q().Query(ctx, `SELECT id, profile, object_key, mime_type, bytes, duration_ms, sha256, loop_start_ms,
		loop_end_ms, status, active FROM background_track_assets WHERE track_id = $1`, trackID)
	must(err)
	defer rows.Close()
	var streamable []*Asset
	for rows.Next() {
		a := &Asset{}
		must(rows.Scan(&a.ID, &a.Profile, &a.ObjectKey, &a.MimeType, &a.Bytes, &a.DurationMs, &a.Sha256, &a.LoopStartMs,
			&a.LoopEndMs, &a.Status, &a.Active))
		if a.Status == "ready" && a.Active {
			streamable = append(streamable, a)
		}
	}
	must(rows.Err())
	for _, a := range streamable {
		if a.Profile == DefaultProfile {
			return a
		}
	}
	var best *Asset
	for _, a := range streamable {
		if best == nil || a.Profile < best.Profile || (a.Profile == best.Profile && a.ID > best.ID) {
			best = a
		}
	}
	return best
}

// Version ports BackgroundTrackAsset#version (the first 16 characters of
// the digest).
func (a *Asset) Version() string {
	r := []rune(a.Sha256)
	if len(r) > 16 {
		r = r[:16]
	}
	return string(r)
}

// CatalogVersion ports BackgroundMusic::Catalog.version (cached a minute).
func CatalogVersion(ctx context.Context) string {
	raw := rediscache.FetchJSON(ctx, "background_music/catalog_version/v1", time.Minute, func() []byte {
		b, _ := json.Marshal(uncachedVersion(ctx))
		return b
	})
	var v string
	must(json.Unmarshal(raw, &v))
	return v
}

func uncachedVersion(ctx context.Context) string {
	var ts *time.Time
	must(db.Q().QueryRow(ctx, `SELECT GREATEST((SELECT MAX(updated_at) FROM background_tracks),
		(SELECT MAX(updated_at) FROM background_categories), (SELECT MAX(updated_at) FROM background_track_categories),
		(SELECT MAX(updated_at) FROM background_track_placements))`).Scan(&ts))
	var count int64
	must(db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM background_tracks WHERE `+PublishedWhere, PublishedArgs()...).Scan(&count))
	return timestampVersion(ts) + "-" + strconv.FormatInt(count, 10)
}

// timestampVersion ports Cacheable.timestamp_version.
func timestampVersion(t *time.Time) string {
	if t == nil {
		return "0"
	}
	u := t.UTC()
	s := u.Format("20060102150405") + strconv.Itoa(1_000_000 + u.Nanosecond()/1000)[1:]
	return strings.TrimLeft(s, "0")
}

// localeFor ports CategoryNames.locale_for.
func localeFor(language string) string {
	if l := langs.OfficeNamesLocaleFor(language); strings.TrimSpace(l) != "" {
		return l
	}
	return fallbackLocale
}

// Translate ports CategoryNames.translate: I18n.t("background_music.<key>")
// in the language's locale, with fallbacks, else fallback.
func Translate(key, language string, fallback any) any {
	locale := localeFor(language)
	if !i18n.Available(locale) {
		return fallback
	}
	if v, ok := i18n.Lookup(locale, "background_music."+key); ok {
		return v
	}
	return fallback
}

// CategoryName ports CategoryNames.for.
func CategoryName(slug, language, fallback string) any {
	return Translate("categories."+slug, language, fallback)
}
