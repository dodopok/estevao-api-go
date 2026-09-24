package v1

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// --- SharedOfficesController -----------------------------------------------

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func shortCode() string {
	b := make([]byte, 7)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(62))
		b[i] = base62[n.Int64()]
	}
	return string(b)
}

var positiveInt = regexp.MustCompile(`^[1-9]\d*$`)
var isoDateShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// SharedOfficesCreate ports SharedOfficesController#create (SharedOfficeService#find_or_create).
func SharedOfficesCreate(c *web.Context) {
	auth.AuthenticateOptional(c)
	auth.VerifyAppRequest(c)
	u := auth.CurrentUser(c)
	bad := func(msg string) { c.JSON(400, rb.M("error", msg)) }
	p := c.Params()
	required := []string{"date", "office_type", "seed"}
	if u == nil {
		required = append(required, "prayer_book_code")
	}
	var missing []string
	for _, k := range required {
		if rb.Blank(p.Get(k)) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		bad("Parâmetros obrigatórios faltando: " + strings.Join(missing, ", "))
		return
	}
	if !positiveInt.MatchString(rb.ToS(p.Get("seed"))) || strings.Contains(rb.ToS(p.Get("seed")), "\n") {
		bad("Seed deve ser um número inteiro positivo")
		return
	}
	if !validOfficeType(p.Get("office_type")) {
		bad("Tipo de ofício inválido. Use: " + strings.Join(OfficeTypesAll, ", "))
		return
	}
	dateStr := rb.ToS(p.Get("date"))
	if !isoDateShape.MatchString(dateStr) || strings.Contains(dateStr, "\n") {
		bad("Formato de data inválido. Use: YYYY-MM-DD")
		return
	}
	date, err := rb.DateParse(dateStr, true)
	if err != nil {
		bad("Formato de data inválido. Use: YYYY-MM-DD")
		return
	}
	// Preferences: the user's (effective, else stored) merged with the request's.
	base := rb.NewMap()
	if u != nil {
		eff, err := u.EffectivePreferences(c.Ctx, definitionsFor)
		must(err)
		if eff.Len() > 0 {
			base = eff
		} else if u.Preferences != nil {
			base = u.Preferences.Dup()
		}
	}
	merged := base.Dup()
	if req := p.Get("preferences"); req != nil {
		m, ok := req.(*rb.Map)
		if !ok {
			panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_unsafe_h", req)})
		}
		merged = merged.Merge(m)
	}
	code := p.Get("prayer_book_code")
	if code == nil {
		code = base.Get("prayer_book_code")
	}
	seed, _ := castIntegerParam(p.Get("seed"))
	// sanitized_preferences
	sanitized := merged.Dup()
	pb, err := findBook(c.Ctx, code)
	must(err)
	if pb != nil {
		defs, err := prefs.For(c.Ctx, pb)
		must(err)
		sanitized = defs.CanonicalizeLegacyKeys(sanitized)
	}
	sanitized.Delete("seed")
	codeS := castString(code)
	if codeS == nil || rb.BlankString(*codeS) {
		c.JSON(422, rb.M("error", "Validation failed: Prayer book code can't be blank"))
		return
	}
	prefsJSON := string(rb.JSON(sanitized))
	office := rb.ToS(p.Get("office_type"))
	var userID any
	userCond := ""
	args := []any{date.ISO(), office, *codeS, seed, prefsJSON, time.Now().UTC()}
	if u != nil {
		userID = u.ID
		userCond = " AND user_id = $7"
		args = append(args, u.ID)
	}
	var existing *store.SharedOffice
	row := db.Q().QueryRow(c.Ctx, `SELECT short_code FROM shared_offices WHERE expires_at > $6 AND date = $1 AND office_type = $2
		AND prayer_book_code = $3 AND seed = $4`+userCond+` AND preferences = $5::jsonb LIMIT 1`, args...)
	var sc string
	if err := row.Scan(&sc); err == nil {
		existing, err = store.ActiveSharedOffice(c.Ctx, sc)
		must(err)
	} else if !db.NoRows(err) {
		pgMust(err)
	}
	if existing == nil {
		now := users.Now()
		for {
			sc = shortCode()
			var taken bool
			must(db.Q().QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM shared_offices WHERE short_code = $1)`, sc).Scan(&taken))
			if !taken {
				break
			}
		}
		_, err := db.Q().Exec(c.Ctx, `INSERT INTO shared_offices (user_id, date, office_type, prayer_book_code, seed, preferences, short_code, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $9)`, userID, date.ISO(), office, *codeS, seed, prefsJSON, sc, now.Add(30*24*time.Hour), now)
		pgMust(err)
		existing, err = store.ActiveSharedOffice(c.Ctx, sc)
		must(err)
	}
	c.JSON(201, rb.M("short_code", existing.ShortCode, "share_path", "/o/"+existing.ShortCode,
		"expires_at", iso8601(existing.ExpiresAt), "date", existing.Date.ISO(), "office_type", existing.OfficeType,
		"prayer_book_code", existing.PrayerBookCode))
}

// SharedOfficesShow ports SharedOfficesController#show.
func SharedOfficesShow(c *web.Context) {
	auth.AuthenticateOptional(c)
	auth.VerifyAppRequest(c)
	code := rb.ToS(c.Param("code"))
	s, err := store.ActiveSharedOffice(c.Ctx, code)
	must(err)
	if s == nil {
		expired, err := store.ExpiredSharedOfficeExists(c.Ctx, code)
		must(err)
		if expired {
			c.JSON(410, rb.M("error", "Este link expirou"))
		} else {
			c.JSON(404, rb.M("error", "Link não encontrado"))
		}
		return
	}
	var createdBy any
	if s.UserID != nil {
		owner, err := users.ByID(c.Ctx, *s.UserID)
		must(err)
		if owner != nil {
			createdBy = rb.Deref(owner.Name)
		}
	}
	c.JSON(200, rb.M("short_code", s.ShortCode, "date", s.Date.ISO(), "office_type", s.OfficeType,
		"prayer_book_code", s.PrayerBookCode, "seed", s.Seed, "preferences", s.Preferences,
		"expires_at", iso8601(s.ExpiresAt), "created_by", createdBy))
}

// --- PreferencesController ------------------------------------------------

// PreferencesShow ports PreferencesController#show.
func PreferencesShow(c *web.Context) {
	code := c.Param("prayer_book_code")
	pb, err := findBook(c.Ctx, code)
	must(err)
	if pb == nil {
		c.JSON(404, rb.M("error", rb.M("code", "PRAYER_BOOK_NOT_FOUND",
			"message", "Prayer Book with code '"+rb.ToS(code)+"' not found")))
		return
	}
	updated := pb.UpdatedAt
	c.ExpiresIn(3600, true)
	if !c.Stale(cacheKey("preferences", "etag", pb.Code, "pb_"+timestampVersion(&updated))) {
		return
	}
	rows, err := db.Q().Query(c.Ctx, `SELECT id, key, name, description, icon, position FROM preference_categories
		WHERE prayer_book_id = $1 ORDER BY position ASC, position ASC`, pb.ID)
	must(err)
	type category struct {
		id                int64
		key, name         string
		description, icon *string
		position          int
	}
	var cats []category
	for rows.Next() {
		var ct category
		must(rows.Scan(&ct.id, &ct.key, &ct.name, &ct.description, &ct.icon, &ct.position))
		cats = append(cats, ct)
	}
	rows.Close()
	ids := make([]int64, len(cats))
	for i, ct := range cats {
		ids[i] = ct.id
	}
	defs, err := prefs.ForCategories(c.Ctx, ids)
	must(err)
	list := []any{}
	total := 0
	var last *time.Time
	for _, ct := range cats {
		items := []any{}
		for _, d := range defs {
			if d.PreferenceCategoryID != ct.id {
				continue
			}
			items = append(items, d.AsJSONForAPI())
			total++
			if last == nil || d.UpdatedAt.After(*last) {
				t := d.UpdatedAt
				last = &t
			}
		}
		list = append(list, rb.M("id", ct.key, "key", ct.key, "name", ct.name, "description", rb.Deref(ct.description),
			"icon", rb.Deref(ct.icon), "order", ct.position, "preferences", items))
	}
	var lastUpdated any
	if last != nil {
		lastUpdated = iso8601(*last)
	}
	c.JSON(200, rb.M("prayer_book_id", pb.Code, "prayer_book_name", rb.Deref(pb.Name), "categories", list,
		"metadata", rb.M("total_categories", len(cats), "total_preferences", total, "last_updated", lastUpdated, "version", "1.0")))
}

// --- FavoritesController --------------------------------------------------

var favoriteKinds = []string{"season", "commemoration", "rosary"}

type favorite struct {
	id                   int64
	postSlug, kind       string
	name                 *string
	createdAt, updatedAt time.Time
}

func favoriteJSON(f *favorite) *rb.Map {
	return rb.M("post_slug", f.postSlug, "kind", f.kind, "name", rb.Deref(f.name), "created_at", rb.FormatTime(f.createdAt))
}

// FavoritesIndex ports FavoritesController#index.
func FavoritesIndex(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	rows, err := db.Q().Query(c.Ctx, `SELECT id, post_slug, kind, name, created_at, updated_at FROM user_favorites WHERE user_id = $1 ORDER BY created_at DESC`, u.ID)
	must(err)
	list := []any{}
	for rows.Next() {
		var f favorite
		must(rows.Scan(&f.id, &f.postSlug, &f.kind, &f.name, &f.createdAt, &f.updatedAt))
		list = append(list, favoriteJSON(&f))
	}
	rows.Close()
	c.JSON(200, rb.M("count", len(list), "favorites", list))
}

// favoriteErrors ports the UserFavorite validations as full messages.
func favoriteErrors(slug, kind, name *string) []string {
	var out []string
	if slug == nil || rb.BlankString(*slug) {
		out = append(out, "Post slug can't be blank")
	}
	if slug != nil && utf8.RuneCountInString(*slug) > 255 {
		out = append(out, "Post slug is too long (maximum is 255 characters)")
	}
	if kind == nil || rb.BlankString(*kind) {
		out = append(out, "Kind can't be blank")
	}
	valid := false
	for _, k := range favoriteKinds {
		valid = valid || (kind != nil && *kind == k)
	}
	if !valid {
		out = append(out, "Kind "+rb.ToS(rb.Deref(kind))+" is not a valid kind")
	}
	if name != nil && utf8.RuneCountInString(*name) > 255 {
		out = append(out, "Name is too long (maximum is 255 characters)")
	}
	return out
}

// FavoritesCreate ports FavoritesController#create (create_or_find_by).
func FavoritesCreate(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	slug := castString(scalarParam(c, "post_slug"))
	kind := castString(scalarParam(c, "kind"))
	name := castString(scalarParam(c, "name"))
	if msgs := favoriteErrors(slug, kind, name); len(msgs) > 0 {
		c.JSON(422, rb.M("error", strings.Join(msgs, ", ")))
		return
	}
	now := users.Now()
	var f favorite
	err := db.Q().QueryRow(c.Ctx, `INSERT INTO user_favorites (user_id, post_slug, kind, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5) RETURNING id, post_slug, kind, name, created_at, updated_at`, u.ID, *slug, *kind, name, now).
		Scan(&f.id, &f.postSlug, &f.kind, &f.name, &f.createdAt, &f.updatedAt)
	if err == nil {
		c.JSON(201, rb.M("message", "Favorite created successfully", "favorite", favoriteJSON(&f)))
		return
	}
	if !db.UniqueViolation(err) {
		pgMust(err)
	}
	must(db.Q().QueryRow(c.Ctx, `SELECT id, post_slug, kind, name, created_at, updated_at FROM user_favorites WHERE user_id = $1 AND post_slug = $2 LIMIT 1`,
		u.ID, *slug).Scan(&f.id, &f.postSlug, &f.kind, &f.name, &f.createdAt, &f.updatedAt))
	if f.kind != *kind || !strPtrEq(f.name, name) {
		_, err := db.Q().Exec(c.Ctx, `UPDATE user_favorites SET kind = $1, name = $2, updated_at = $3 WHERE id = $4`, *kind, name, users.Now(), f.id)
		pgMust(err)
		f.kind, f.name = *kind, name
	}
	c.JSON(200, rb.M("message", "Favorite already exists", "favorite", favoriteJSON(&f)))
}

func strPtrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func scalarParam(c *web.Context, k string) any {
	v, ok := c.Params().Lookup(k)
	if !ok || !permittedScalar(v) {
		return nil
	}
	return v
}

// FavoritesDestroy ports FavoritesController#destroy.
func FavoritesDestroy(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	tag, err := db.Q().Exec(c.Ctx, `DELETE FROM user_favorites WHERE id = (SELECT id FROM user_favorites WHERE user_id = $1 AND post_slug = $2 LIMIT 1)`,
		u.ID, rb.ToS(c.Param("post_slug")))
	must(err)
	if tag.RowsAffected() == 0 {
		c.JSON(404, rb.M("error", "Favorite not found"))
		return
	}
	c.JSON(200, rb.M("message", "Favorite deleted successfully"))
}
