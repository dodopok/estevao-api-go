// Package v2 ports the /api/v2 controllers: the read-only, API-key surface
// whose responses the caller shapes with include and fields.
package v2

import (
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

const (
	datedMaxAge   = 3600
	currentMaxAge = 300

	ttlStaticData   = 30 * 24 * time.Hour
	ttlCalendarYear = 365*24*time.Hour + 6*time.Hour // ActiveSupport's 1.year
	ttlReadings     = 24 * time.Hour
	ttlPrayerBook   = 24 * time.Hour

	documentationURL   = "https://api.caminhoanglicano.com.br/api-docs/v2/swagger.yaml"
	problemContentType = "application/problem+json; charset=utf-8"

	textVerses   = "verses"
	textPlain    = "plain"
	textMarkdown = "markdown"
)

var textFormats = []string{textVerses, textPlain, textMarkdown}

func today() civil.Date { return civil.FromTime(time.Now().In(rb.AppZone)) }

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// controller is one v2 controller's ALLOWED_INCLUDES/ALLOWED_FIELDS and
// whether its resource_meta is the liturgical context.
type controller struct {
	name        string
	includes    []string
	fields      fieldSpec
	contextMeta func(r *req) bool
}

// req is one v2 request: the controller instance state.
type req struct {
	c    *web.Context
	ctrl *controller

	ctx      *reqContext
	incl     *IncludeSet
	flds     *FieldSet
	calendar map[int]*liturgical.Calendar
}

// action wraps a v2 action with BaseController's require_api_key! and the
// ApiKeyAuthenticatable after_action.
func (ctrl *controller) action(fn func(r *req)) web.HandlerFunc {
	return func(c *web.Context) {
		c.RenderError = renderError
		if !auth.AuthenticateAPIKey(c) {
			renderProblem(c, "MISSING_API_KEY", "Envie sua chave no cabeçalho X-API-Key.", 401, nil)
			return
		}
		fn(&req{c: c, ctrl: ctrl, calendar: map[int]*liturgical.Calendar{}})
		auth.RecordAPIKeyUsage(c, ctrl.name)
	}
}

func renderError(c *web.Context, _ string, code, message string, context *rb.Map, status int) {
	renderProblem(c, code, message, status, context)
}

// renderProblem ports render_problem with ProblemSerializer.
func renderProblem(c *web.Context, code, detail string, status int, extras *rb.Map) {
	if rb.BlankString(code) {
		code = "ERROR"
	}
	title := strings.ReplaceAll(code, "_", " ")
	if title != "" {
		title = strings.ToUpper(title[:1]) + strings.ToLower(title[1:])
	}
	body := rb.M(
		"type", documentationURL+"#"+strings.ReplaceAll(strings.ToLower(code), "_", "-"),
		"title", title,
		"status", status,
		"code", code,
		"detail", detail,
		"documentation_url", documentationURL,
	)
	if c.RequestID != "" {
		body.Set("request_id", c.RequestID)
	}
	if extras != nil {
		body = body.Merge(extras)
	}
	c.Raw(status, problemContentType, rb.JSON(body))
}

func (r *req) includes() *IncludeSet {
	if r.incl == nil {
		r.incl = ParseIncludes(r.c.Param("include"), r.ctrl.includes)
	}
	return r.incl
}

func (r *req) fields() *FieldSet {
	if r.flds == nil {
		r.flds = ParseFields(r.c.Param("fields"), r.ctrl.fields)
	}
	return r.flds
}

func (r *req) context() *reqContext {
	if r.ctx == nil {
		r.ctx = buildContext(r.c)
	}
	return r.ctx
}

// calendarFor ports Calendars#for: one calendar per year for the request.
func (r *req) calendarFor(date civil.Date) *liturgical.Calendar {
	y := date.Year()
	if cal, ok := r.calendar[y]; ok {
		return cal
	}
	pb := r.context().pb
	bc, err := store.CelebrationsForBook(r.c.Ctx, pb)
	must(err)
	cal := liturgical.NewCalendarWith(y, pb.Code, bc)
	r.calendar[y] = cal
	return cal
}

// cacheKey ports Cacheable.key.
func cacheKey(parts ...string) string { return "v8/" + strings.Join(parts, "/") }

// payloadKey ports payload_key.
func (r *req) payloadKey(parts ...string) string {
	all := append([]string{"api", "v2"}, parts...)
	all = append(all, r.context().cacheKeyParts()...)
	return cacheKey(append(all, r.includes().CacheKey())...)
}

func (r *req) setCost(cost int) { r.c.Header.Set("X-Request-Cost", strconv.Itoa(cost)) }

// staleResource ports stale_resource?(key, cost:): cost < 0 uses the
// includes' cost. The validator may be several segments (an array key).
func (r *req) staleResource(cost int, validator ...string) bool {
	if cost < 0 {
		cost = RequestCost(r.includes(), 1)
	}
	r.setCost(cost)
	return r.c.Stale(strings.Join(append(validator, r.fields().CacheKey()), "/"))
}

// cached ports Rails.cache.fetch for a payload (stored as JSON under the
// Go namespace, beside Rails' own entry).
func (r *req) cached(key string, ttl time.Duration, compute func() any) any {
	raw := rediscache.FetchJSON(r.c.Ctx, "api_v2_payload/1/"+key, ttl, func() []byte { return rb.JSON(compute()) })
	v, err := rb.ParseJSON(raw)
	must(err)
	return v
}

func maxAgeFor(date civil.Date) int {
	if date == today() {
		return currentMaxAge
	}
	return datedMaxAge
}

// renderResource ports render_resource. cost < 0 uses the includes' cost.
func (r *req) renderResource(data any, meta *rb.Map, links *rb.Map, cost, maxAge int) {
	if cost < 0 {
		cost = RequestCost(r.includes(), 1)
	}
	r.setCost(cost)
	r.c.ExpiresIn(maxAge, false)
	m := rb.NewMap()
	if r.ctrl.contextMeta != nil && r.ctrl.contextMeta(r) {
		m = r.context().meta()
	}
	m.Set("generated_at", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if meta != nil {
		m = m.Merge(meta)
	}
	body := rb.M("data", data, "meta", m)
	if links != nil && links.Len() > 0 {
		body.Set("links", links)
	}
	r.c.JSON(200, body)
}

// --- RequestContext -------------------------------------------------------

// reqContext ports Api::V2::RequestContext.
type reqContext struct {
	pb         *store.PrayerBook
	resolved   *prefs.Resolved
	values     *rb.Map
	textFormat string
	variant    *string
}

var parameterPreferences = [][2]string{
	{"bible", "bible_version"}, {"psalter", "psalm_translation"}, {"lectionary", "lectionary_variant"},
	{"reading_type", "reading_type"}, {"lang", "language"},
}

func buildContext(c *web.Context) *reqContext {
	code := c.Param("book")
	if rb.Blank(code) {
		domainError("InvalidParameter", "O parâmetro 'book' é obrigatório na v2. Consulte GET /api/v2/prayer-books.",
			"MISSING_PRAYER_BOOK", nil)
	}
	pb, err := store.PrayerBookByCode(c.Ctx, toS(code))
	must(err)
	if pb == nil {
		domainError("InvalidPreference", "Livro de oração desconhecido: '"+toS(code)+"'.", "UNKNOWN_PRAYER_BOOK",
			rb.M("book", toS(code)))
	}
	overrides := preferencesBlob(c.Param("preferences"))
	for _, pp := range parameterPreferences {
		if v := c.Param(pp[0]); rb.Present(v) {
			overrides.Set(pp[1], v)
		}
	}
	resolved, err := prefs.Resolve(c.Ctx, pb, nil, overrides)
	must(err)
	ctx := &reqContext{pb: pb, resolved: resolved, values: resolved.Values}
	ctx.textFormat = validatedTextFormat(c.Param("text_format"))
	return ctx
}

// preferencesBlob ports preferences_blob: a JSON string or a nested hash;
// an unreadable string is ignored, JSON that is not an object raises.
func preferencesBlob(raw any) *rb.Map {
	switch x := raw.(type) {
	case string:
		parsed, err := rb.ParseJSON([]byte(x))
		if err != nil {
			return rb.NewMap()
		}
		m, ok := parsed.(*rb.Map)
		if !ok {
			web.Fail("NoMethodError", "%s", rb.NoMethodErrorMessage("symbolize_keys", parsed))
		}
		return m.Dup()
	case *rb.Map:
		return x.Dup()
	}
	return rb.NewMap()
}

func validatedTextFormat(value any) string {
	if rb.Blank(value) {
		return textVerses
	}
	s := toS(value)
	for _, f := range textFormats {
		if f == s {
			return s
		}
	}
	domainError("InvalidParameter", "text_format inválido: '"+s+"'.", "INVALID_TEXT_FORMAT", rb.M("allowed", strs(textFormats)))
	return ""
}

func (x *reqContext) code() string { return x.pb.Code }

func (x *reqContext) pref(key string) string {
	v := x.values.Get(key)
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

func (x *reqContext) bible() any { return x.values.Get("bible_version") }

func (x *reqContext) language() string {
	if v := x.values.Get("language"); rb.Present(v) {
		return rb.ToS(v)
	}
	return x.pb.Language
}

// lectionaryVariant ports lectionary_variant ("" is nil).
func (x *reqContext) lectionaryVariant() string {
	if x.variant == nil {
		v := books.For(x.pb.Code, x.pb.Features).LectionaryServiceVariant(x.values)
		x.variant = &v
	}
	return *x.variant
}

func (x *reqContext) meta() *rb.Map {
	m := rb.M("prayer_book", x.code(), "language", x.language())
	if b := x.bible(); b != nil {
		m.Set("bible", b)
	}
	if v := x.lectionaryVariant(); v != "" {
		m.Set("lectionary_variant", v)
	}
	return m
}

func timestampVersion(t time.Time) string {
	u := t.UTC()
	return strings.TrimLeft(u.Format("20060102150405")+strconv.Itoa(1000000 + u.Nanosecond()/1000)[1:], "0")
}

func (x *reqContext) cacheKeyParts() []string {
	return []string{x.code(), x.resolved.CacheKey(), x.textFormat, "pb_" + timestampVersion(x.pb.UpdatedAt)}
}
