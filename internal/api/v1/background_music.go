package v1

import (
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/activestorage"
	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/bgmusic"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// bgOfficeTypes ports DailyOffice::OfficeTypes::ALL.
var bgOfficeTypes = []string{"morning", "prime", "terce", "midday", "none", "evening", "compline", "late_evening"}

// bgMusic holds the request's memoized values, as the controller's
// instance variables do.
type bgMusic struct {
	c        *web.Context
	u        *users.User
	book     *store.PrayerBook
	language *string
	version  string
	names    map[string]any
	order    *bgmusic.Order
}

// withBackgroundMusic ports the controller's before_actions:
// set_private_cache_control, authenticate_user!, require_premium! and
// require_background_music!.
func withBackgroundMusic(action func(m *bgMusic)) web.HandlerFunc {
	return func(c *web.Context) {
		c.Header.Set("Cache-Control", "private, no-store")
		auth.AuthenticateRequired(c)
		auth.RequirePremium(c)
		u := auth.CurrentUser(c)
		if !features.EnabledFor(c.Ctx, "background_music", u, u.Premium(), time.Now()) {
			c.RenderJSON(404, rb.M("error", "Background music is not available", "code", "FEATURE_DISABLED"))
		}
		action(&bgMusic{c: c, u: u, names: map[string]any{}})
	}
}

func (m *bgMusic) preferredBookCode() any {
	if v := m.c.Param("prayer_book_code"); rb.Present(v) {
		return v
	}
	if m.u.Preferences != nil {
		if v := m.u.Preferences.Get("prayer_book_code"); v != nil {
			return v
		}
	}
	return books.DefaultCode
}

// selectedBook ports selected_prayer_book.
func (m *bgMusic) selectedBook() *store.PrayerBook {
	if m.book != nil {
		return m.book
	}
	code := rb.ToS(m.preferredBookCode())
	pb, err := store.PrayerBookByCode(m.c.Ctx, code)
	must(err)
	if pb == nil || pb.ExternalOnly || (pb.PremiumRequired && !m.u.Premium()) {
		web.Raise("UnsupportedPrayerBook", "unknown prayer book: "+code, "")
	}
	m.book = pb
	return pb
}

// officeType ports requested_office_type.
func (m *bgMusic) officeType(pb *store.PrayerBook) string {
	officeType := "morning"
	if v := m.c.Param("office_type"); rb.Present(v) {
		officeType = rb.ToS(v)
	}
	valid := false
	for _, o := range bgOfficeTypes {
		if o == officeType {
			valid = true
		}
	}
	available := false
	for _, o := range books.For(pb.Code, pb.Features).AvailableOffices(false) {
		if o == officeType {
			available = true
		}
	}
	if !valid || !available {
		web.Raise("UnsupportedOfficeType", "unsupported office type: "+officeType, "")
	}
	return officeType
}

// date ports requested_date.
func (m *bgMusic) date() civil.Date {
	var value any = m.c.Param("date")
	if !rb.Present(value) {
		loc := rb.RailsZone(m.u.Timezone)
		if loc == nil {
			panic(&web.StandardError{Class: "ArgumentError", Message: "Invalid Timezone: " + m.u.Timezone})
		}
		value = civil.FromTime(time.Now().In(loc)).ISO()
	}
	d, err := rb.DateISO8601(rb.ToS(value))
	if err != nil {
		if err != rb.ErrInvalidDate {
			panic(&web.StandardError{Class: "ArgumentError", Message: err.Error()})
		}
		invalidDate("Data inválida: " + rb.ToS(value))
	}
	validateYear(int(d.Y))
	cd, ok := d.Civil()
	if !ok {
		invalidDate("Data inválida: " + rb.ToS(value))
	}
	return cd
}

// language ports background_music_language.
func (m *bgMusic) lang() string {
	if m.language == nil {
		l := "pt-BR"
		pb, err := store.PrayerBookByCode(m.c.Ctx, rb.ToS(m.preferredBookCode()))
		must(err)
		if pb != nil {
			l = pb.Language
		}
		m.language = &l
	}
	return *m.language
}

func (m *bgMusic) catalogVersion() string {
	if m.version == "" {
		m.version = bgmusic.CatalogVersion(m.c.Ctx)
	}
	return m.version
}

func (m *bgMusic) categoryName(slug, fallback string) any {
	if v, ok := m.names[slug]; ok && v != nil {
		return v
	}
	v := bgmusic.CategoryName(slug, m.lang(), fallback)
	m.names[slug] = v
	return v
}

// trackOrder builds the TrackOrder of the request (selected book, date,
// office type), evaluated in the order the controller evaluates them.
func (m *bgMusic) trackOrder(pb *store.PrayerBook, date civil.Date, officeType string) *bgmusic.Order {
	return &bgmusic.Order{Date: date, OfficeType: officeType, PrayerBookCode: pb.Code, CatalogVersion: m.catalogVersion(),
		Language: pb.Language}
}

// trackPayload ports track_payload.
func (m *bgMusic) trackPayload(t *bgmusic.Track, favorite bool) *rb.Map {
	cats := append([]bgmusic.Category{}, t.Categories...)
	for i := 1; i < len(cats); i++ {
		for j := i; j > 0 && (cats[j].Position < cats[j-1].Position ||
			cats[j].Position == cats[j-1].Position && cats[j].ID < cats[j-1].ID); j-- {
			cats[j], cats[j-1] = cats[j-1], cats[j]
		}
	}
	categories := []any{}
	for _, c := range cats {
		categories = append(categories, rb.M("slug", c.Slug, "name", m.categoryName(c.Slug, c.Name)))
	}
	return rb.M("id", t.ID, "slug", t.Slug, "title", t.Title, "composer", rb.Deref(t.Composer), "performer", rb.Deref(t.Performer),
		"kind", t.Kind, "duration_ms", t.DurationMs, "loop_approved", t.LoopApproved, "categories", categories,
		"credit", rb.M("text", rb.Deref(t.CreditText), "license_type", rb.Deref(t.LicenseType),
			"license_url", rb.Deref(t.LicenseURL), "license_version", rb.Deref(t.LicenseVersion),
			"source_url", rb.Deref(t.SourceURL), "notes", rb.Deref(t.Notes)),
		"favorite", favorite)
}

// favoriteIDs ports favorite_ids_for.
func (m *bgMusic) favoriteIDs(tracks []*bgmusic.Track) map[int64]bool {
	out := map[int64]bool{}
	if len(tracks) == 0 {
		return out
	}
	ids := make([]int64, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}
	rows, err := db.Q().Query(m.c.Ctx, `SELECT track_id FROM user_background_track_favorites WHERE user_id = $1 AND track_id = ANY($2)`,
		m.u.ID, ids)
	must(err)
	defer rows.Close()
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		out[id] = true
	}
	must(rows.Err())
	return out
}

// favoriteTracks ports favorite_tracks(limit:).
func (m *bgMusic) favoriteTracks(limit int) []*bgmusic.Track {
	tracks := bgmusic.LoadTracks(m.c.Ctx,
		"INNER JOIN user_background_track_favorites ON user_background_track_favorites.track_id = background_tracks.id",
		"user_background_track_favorites.user_id = $2",
		"ORDER BY user_background_track_favorites.created_at DESC LIMIT $3", m.u.ID, limit)
	bgmusic.LoadCategories(m.c.Ctx, tracks, nil)
	return tracks
}

func (m *bgMusic) recommendationPayload() any {
	pb := m.selectedBook()
	officeType := m.officeType(pb)
	o := m.trackOrder(pb, m.date(), officeType)
	r := o.Recommend(m.c.Ctx)
	if r == nil {
		return nil
	}
	return rb.M("track", m.trackPayload(r.Track, m.favoriteIDs([]*bgmusic.Track{r.Track})[r.Track.ID]),
		"reason", rb.M("facet", r.Facet, "key", r.Key, "label", r.Label))
}

// categoryPayload ports category_payload (cached an hour).
func (m *bgMusic) categoryPayload() []any {
	key := strings.Join([]string{"background_music/categories/v1", m.catalogVersion(), m.lang()}, "/")
	raw := rediscache.FetchJSON(m.c.Ctx, key, time.Hour, func() []byte {
		ctx := m.c.Ctx
		counts := map[string]int64{}
		rows, err := db.Q().Query(ctx, `SELECT background_categories.slug, COUNT(*) FROM background_tracks
			INNER JOIN background_track_categories ON background_track_categories.track_id = background_tracks.id
			INNER JOIN background_categories ON background_categories.id = background_track_categories.category_id
			WHERE `+bgmusic.PublishedWhere+` GROUP BY background_categories.slug`, bgmusic.PublishedArgs()...)
		must(err)
		for rows.Next() {
			var slug string
			var n int64
			must(rows.Scan(&slug, &n))
			counts[slug] = n
		}
		rows.Close()
		must(rows.Err())
		var total int64
		must(db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM background_tracks WHERE `+bgmusic.PublishedWhere,
			bgmusic.PublishedArgs()...).Scan(&total))
		list := []any{rb.M("slug", "all", "name", m.categoryName("all", "Todas"), "count", total)}
		for _, c := range bgmusic.OrderedCategories(ctx) {
			list = append(list, rb.M("slug", c.Slug, "name", m.categoryName(c.Slug, c.Name), "count", counts[c.Slug]))
		}
		return rb.JSON(list)
	})
	v, err := rb.ParseJSON(raw)
	must(err)
	return v.([]any)
}

// BackgroundMusicHome ports #home.
var BackgroundMusicHome = withBackgroundMusic(func(m *bgMusic) {
	version := m.catalogVersion()
	recommendation := m.recommendationPayload()
	categories := m.categoryPayload()
	favorites := []any{}
	for _, t := range m.favoriteTracks(3) {
		favorites = append(favorites, m.trackPayload(t, true))
	}
	m.c.JSON(200, rb.M("recommendation", recommendation, "categories", categories, "favorites", favorites,
		"catalog_version", version))
})

func invalidParameter(msg string) { web.Raise("InvalidParameter", msg, "") }

// filteredTracks ports filtered_tracks. A category filter joins the
// categories the includes(:categories) eager-loads through, so each track
// then carries only that category.
func (m *bgMusic) filteredTracks() []*bgmusic.Track {
	var from, where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)+1) }
	var only *bgmusic.Category
	if v := m.c.Param("category"); rb.Present(v) && rb.ToS(v) != "all" {
		only = bgmusic.CategoryBySlug(m.c.Ctx, rb.ToS(v))
		if only == nil {
			invalidParameter("Invalid background music category")
		}
		from = append(from, `INNER JOIN background_track_categories ON background_track_categories.track_id = background_tracks.id
			INNER JOIN background_categories ON background_categories.id = background_track_categories.category_id`)
		where = append(where, "background_categories.id = "+arg(only.ID))
	}
	if v := m.c.Param("kind"); rb.Present(v) {
		kind := rb.ToS(v)
		ok := false
		for _, k := range bgmusic.Kinds {
			ok = ok || k == kind
		}
		if !ok {
			invalidParameter("Invalid background music kind")
		}
		where = append(where, "background_tracks.kind = "+arg(kind))
	}
	if rb.ToS(m.c.Param("favorites")) == "true" {
		from = append(from, `INNER JOIN user_background_track_favorites ON user_background_track_favorites.track_id = background_tracks.id`)
		where = append(where, "user_background_track_favorites.user_id = "+arg(m.u.ID))
	}
	if v := m.c.Param("q"); rb.Present(v) {
		query := rb.Strip(rb.ToS(v))
		if len([]rune(query)) > 80 {
			invalidParameter("Background music search is too long")
		}
		p := arg("%" + sanitizeSQLLike(query) + "%")
		where = append(where, "(background_tracks.title ILIKE "+p+" OR background_tracks.composer ILIKE "+p+
			" OR background_tracks.performer ILIKE "+p+")")
	}
	tracks := bgmusic.LoadTracks(m.c.Ctx, strings.Join(from, " "), strings.Join(where, " AND "),
		"ORDER BY background_tracks.sort_order ASC, background_tracks.id ASC", args...)
	bgmusic.LoadCategories(m.c.Ctx, tracks, only)
	return tracks
}

// pageLimit ports page_limit.
func (m *bgMusic) pageLimit() int {
	var v any = 20
	if raw, ok := m.c.Params().Lookup("limit"); ok {
		v = raw
	}
	n, cls := rb.KernelInteger(v)
	if cls == "FloatDomainError" {
		panic(&web.StandardError{Class: "FloatDomainError", Message: rb.ToS(v)})
	}
	if cls != "" || n < 1 || n > 50 {
		invalidParameter("limit must be between 1 and 50")
	}
	return int(n)
}

type bgCursor struct {
	orderKey  []int64
	legacyKey []int64
}

// decodeCursor ports decode_cursor.
func decodeCursor(value string) bgCursor {
	bad := func() { invalidParameter("Invalid background music cursor") }
	s := value
	if !strings.HasSuffix(s, "=") && len(s)%4 != 0 {
		s += strings.Repeat("=", (4-len(s)%4)%4)
	}
	s = strings.NewReplacer("-", "+", "_", "/").Replace(s)
	if strings.ContainsAny(s, "\r\n") {
		bad()
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil {
		bad()
	}
	parsed, err := rb.ParseJSON(decoded)
	if err != nil {
		bad()
	}
	payload, ok := parsed.(*rb.Map)
	if !ok {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("key?", parsed)})
	}
	integer := func(v any) int64 {
		n, cls := rb.KernelInteger(v)
		switch cls {
		case "":
			return n
		case "FloatDomainError":
			panic(&web.StandardError{Class: "FloatDomainError", Message: rb.ToS(v)})
		}
		bad()
		return 0
	}
	if v, ok := payload.Lookup("order_key"); ok {
		var key []int64
		for _, el := range arrayWrapParam(v) {
			key = append(key, integer(el))
		}
		if len(key) == 0 {
			bad()
		}
		return bgCursor{orderKey: key}
	}
	sortOrder, ok := payload.Lookup("sort_order")
	if !ok {
		bad()
	}
	so := integer(sortOrder)
	id, ok := payload.Lookup("id")
	if !ok {
		bad()
	}
	return bgCursor{legacyKey: []int64{so, integer(id)}}
}

// arrayWrapParam ports Array(value) on a parsed JSON value.
func arrayWrapParam(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	case *rb.Map:
		var out []any
		x.Each(func(k string, v any) { out = append(out, []any{k, v}) })
		return out
	}
	return []any{v}
}

func encodeCursor(e *bgmusic.Entry) string {
	key := make([]any, len(e.OrderKey))
	for i, n := range e.OrderKey {
		key[i] = n
	}
	return base64.RawURLEncoding.EncodeToString([]byte(rb.JSON(rb.M("order_key", key))))
}

// BackgroundMusicTracks ports #tracks.
var BackgroundMusicTracks = withBackgroundMusic(func(m *bgMusic) {
	version := m.catalogVersion()
	pb := m.selectedBook()
	tracks := m.filteredTracks()
	date := m.date()
	officeType := m.officeType(pb)
	ordered := m.trackOrder(pb, date, officeType).Call(m.c.Ctx, tracks)
	var cursor *bgCursor
	if v := m.c.Param("cursor"); rb.Present(v) {
		cur := decodeCursor(rb.ToS(v))
		cursor = &cur
	}
	var page []*bgmusic.Entry
	for _, e := range ordered {
		if cursor != nil {
			key := cursor.orderKey
			entryKey := e.OrderKey
			if key == nil {
				key, entryKey = cursor.legacyKey, []int64{e.Track.SortOrder, e.Track.ID}
			}
			if bgmusic.Compare(entryKey, key) != 1 {
				continue
			}
		}
		page = append(page, e)
	}
	limit := m.pageLimit()
	hasMore := len(page) > limit
	if hasMore {
		page = page[:limit]
	}
	pageTracks := make([]*bgmusic.Track, len(page))
	for i, e := range page {
		pageTracks[i] = e.Track
	}
	favs := m.favoriteIDs(pageTracks)
	out := []any{}
	for _, t := range pageTracks {
		out = append(out, m.trackPayload(t, favs[t.ID]))
	}
	var next any
	if hasMore {
		next = encodeCursor(page[len(page)-1])
	}
	m.c.JSON(200, rb.M("tracks", out, "next_cursor", next, "catalog_version", version))
})

// bgStorageService ports Storage#service_name.
func bgStorageService() string {
	if s := config.PresenceOr("BACKGROUND_MUSIC_STORAGE_SERVICE", ""); s != "" {
		return s
	}
	if config.RailsEnv() == "production" {
		return "railway_avatars"
	}
	return "local"
}

// bgURLExpiresIn ports Storage#url_expires_in.
func bgURLExpiresIn() int64 {
	raw, set := os.LookupEnv("BACKGROUND_MUSIC_URL_EXPIRES_IN")
	if !set {
		return 3600
	}
	n, cls := rb.KernelInteger(raw)
	if cls != "" {
		return 3600
	}
	return min(max(n, 300), 86_400)
}

func renderTrackNotFound(c *web.Context) {
	c.JSON(404, rb.M("error", "Background music track not found"))
}

// BackgroundMusicPlayback ports #playback.
var BackgroundMusicPlayback = withBackgroundMusic(func(m *bgMusic) {
	c := m.c
	var track *bgmusic.Track
	if id, ok := findID(c.Param("id")); ok {
		track = bgmusic.PublishedTrack(c.Ctx, id)
	}
	if track == nil {
		renderTrackNotFound(c)
		return
	}
	asset := bgmusic.PreferredAsset(c.Ctx, track.ID)
	if asset == nil {
		c.JSON(503, rb.M("error", "Background music asset unavailable"))
		return
	}
	var prefCode any
	if m.u.Preferences != nil {
		prefCode = m.u.Preferences.Get("prayer_book_code")
	}
	var officeType any
	if v := c.Param("office_type"); rb.Present(v) {
		officeType = v
	}
	audio.RecordUserUsageLater(m.c.Ctx, m.u.ID, []*rb.Map{rb.M("audio_type", "background_track", "asset_key", strconv.FormatInt(track.ID, 10),
		"background_track_id", track.ID, "prayer_book_code", prefCode, "office_type", officeType)})
	payload := m.trackPayload(track, m.favoriteIDs([]*bgmusic.Track{track})[track.ID])
	if !strings.HasPrefix(asset.ObjectKey, "background_music/") || strings.Contains(asset.ObjectKey, "..") {
		panic(&web.StandardError{Class: "ArgumentError", Message: "invalid background music object key"})
	}
	now := time.Now()
	expiresIn := bgURLExpiresIn()
	base := asset.ObjectKey[strings.LastIndex(asset.ObjectKey, "/")+1:]
	url, err := activestorage.ServiceObjectURL(bgStorageService(), asset.ObjectKey, int(expiresIn), base, "inline",
		asset.MimeType, c.BaseURL(), now)
	if err != nil {
		c.JSON(503, rb.M("error", "Background music storage unavailable"))
		return
	}
	c.JSON(200, rb.M("track", payload,
		"url", url, "expires_at", now.Add(time.Duration(expiresIn)*time.Second).In(rb.AppZone).Format(time.RFC3339),
		"mime_type", asset.MimeType, "bytes", asset.Bytes, "duration_ms", asset.DurationMs, "asset_version", asset.Version(),
		"loop_start_ms", rb.Deref(asset.LoopStartMs), "loop_end_ms", rb.Deref(asset.LoopEndMs)))
})

// BackgroundMusicFavorites ports #favorites.
var BackgroundMusicFavorites = withBackgroundMusic(func(m *bgMusic) {
	out := []any{}
	for _, t := range m.favoriteTracks(100) {
		out = append(out, m.trackPayload(t, true))
	}
	m.c.JSON(200, rb.M("favorites", out))
})

// BackgroundMusicCreateFavorite ports #create_favorite
// (create_or_find_by!, whose uniqueness validation raises first).
var BackgroundMusicCreateFavorite = withBackgroundMusic(func(m *bgMusic) {
	c := m.c
	var track *bgmusic.Track
	if id, ok := findID(c.Param("track_id")); ok {
		track = bgmusic.PublishedTrack(c.Ctx, id)
	}
	if track == nil {
		renderTrackNotFound(c)
		return
	}
	var taken bool
	must(db.Q().QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM user_background_track_favorites WHERE user_id = $1 AND track_id = $2)`,
		m.u.ID, track.ID).Scan(&taken))
	if taken {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: User has already been taken"})
	}
	now := users.Now()
	_, err := db.Q().Exec(c.Ctx, `INSERT INTO user_background_track_favorites (user_id, track_id, created_at, updated_at)
		VALUES ($1, $2, $3, $3)`, m.u.ID, track.ID, now)
	if err != nil && !db.UniqueViolation(err) {
		pgMust(err)
	}
	c.JSON(200, rb.M("favorite", m.trackPayload(track, true)))
})

// BackgroundMusicDestroyFavorite ports #destroy_favorite.
var BackgroundMusicDestroyFavorite = withBackgroundMusic(func(m *bgMusic) {
	if id, ok := findID(m.c.Param("track_id")); ok {
		_, err := db.Q().Exec(m.c.Ctx, `DELETE FROM user_background_track_favorites WHERE id =
			(SELECT id FROM user_background_track_favorites WHERE user_id = $1 AND track_id = $2 LIMIT 1)`, m.u.ID, id)
		pgMust(err)
	}
	m.c.HeadStatus(204)
})
