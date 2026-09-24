// Package v1 ports the /api/v1 controllers.
package v1

import (
	"context"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// resolver ports Api::V1::Concerns::PreferencesResolver for one request.
type resolver struct {
	c           *web.Context
	apiKeyCalls bool
	books       map[string]*store.PrayerBook
	resolved    *prefs.Resolved
	param       *rb.Map
	shared      *sharedOffice
}

func newResolver(c *web.Context) *resolver {
	if r, ok := c.Get("prefs_resolver").(*resolver); ok {
		return r
	}
	r := &resolver{c: c, books: map[string]*store.PrayerBook{}}
	c.Set("prefs_resolver", r)
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// prayerBook ports prayer_book_for_request (bang raises RecordNotFound).
func (r *resolver) prayerBook(code string, bang bool) *store.PrayerBook {
	if pb, ok := r.books[code]; ok {
		return pb
	}
	pb, err := store.PrayerBookByCode(r.c.Ctx, code)
	must(err)
	if pb == nil {
		if bang {
			web.RecordNotFound("Couldn't find PrayerBook with code=" + code)
		}
		return nil
	}
	r.books[code] = pb
	return pb
}

// preferencesParam ports parse_preferences_param.
func (r *resolver) preferencesParam() *rb.Map {
	if r.param != nil {
		return r.param
	}
	switch v := r.c.Param("preferences").(type) {
	case string:
		parsed, err := rb.ParseJSON([]byte(v))
		if m, ok := parsed.(*rb.Map); err == nil && ok {
			r.param = m
		} else if err == nil {
			// JSON.parse of a non-object then with_indifferent_access raises.
			web.Fail("NoMethodError", "undefined method `with_indifferent_access'")
		} else {
			r.param = rb.NewMap()
		}
	case *rb.Map:
		r.param = v
	default:
		r.param = rb.NewMap()
	}
	return r.param
}

func (r *resolver) usingSharedOffice() bool { return r.c.ParamPresent("shared_office_code") }

// resolvedPreferences ports resolved_preferences.
func (r *resolver) resolvedPreferences() *prefs.Resolved {
	if r.resolved != nil {
		return r.resolved
	}
	var res *prefs.Resolved
	var err error
	ctx := r.c.Ctx
	switch {
	case r.usingSharedOffice():
		office := r.sharedOffice()
		pb := r.prayerBook(office.PrayerBookCode, true)
		overrides := office.Preferences.Dup()
		overrides.Set("prayer_book_code", office.PrayerBookCode)
		overrides.Set("seed", office.Seed)
		res, err = prefs.Resolve(ctx, pb, nil, overrides)
	case auth.CurrentUser(r.c) != nil:
		u := auth.CurrentUser(r.c)
		stored, e := u.EffectivePreferences(ctx, definitionsFor)
		must(e)
		if stored.Len() == 0 {
			stored = u.Preferences
		}
		code := ""
		if v := r.preferencesParam().Get("prayer_book_code"); rb.Present(v) {
			code = rb.ToS(v)
		} else if v := stored.Get("prayer_book_code"); rb.Truthy(v) {
			code = rb.ToS(v)
		} else {
			code = books.DefaultCode
		}
		pb := r.prayerBook(code, true)
		res, err = prefs.Resolve(ctx, pb, stored, r.preferencesParam())
	default:
		code := rb.ToS(r.preferencesParam().Get("prayer_book_code"))
		pb := r.prayerBook(code, true)
		res, err = prefs.Resolve(ctx, pb, nil, r.preferencesParam())
	}
	must(err)
	r.resolved = res
	return res
}

func definitionsFor(pb *store.PrayerBook) ([]string, *rb.Map, error) {
	set, err := prefs.For(context.Background(), pb)
	if err != nil {
		return nil, nil, err
	}
	return set.Keys(), set.Defaults(), nil
}

func (r *resolver) code() string            { return r.resolvedPreferences().String("prayer_book_code") }
func (r *resolver) book() *store.PrayerBook { return r.prayerBook(r.code(), true) }
func (r *resolver) language() string        { return r.book().Language }

func (r *resolver) readingType() string {
	if v := r.resolvedPreferences().Get("reading_type"); rb.Truthy(v) {
		return rb.ToS(v)
	}
	return "semicontinuous"
}

// lectionaryVariant ports resolved_lectionary_variant ("" == nil).
func (r *resolver) lectionaryVariant() string {
	pb := r.book()
	return books.For(pb.Code, pb.Features).LectionaryServiceVariant(r.resolvedPreferences().Values)
}

// validatePreferences ports validate_preferences! (renders and halts).
func (r *resolver) validatePreferences() {
	if r.usingSharedOffice() {
		return
	}
	code, message, status := r.rejection()
	if code == "" {
		return
	}
	r.c.RenderJSON(status, rb.M("success", false, "error", rb.M("code", code, "message", message)))
}

// rejection ports Preferences::RequestPolicy#rejection.
func (r *resolver) rejection() (string, string, int) {
	requested := r.preferencesParam()
	if u := auth.CurrentUser(r.c); u != nil {
		if !u.OnboardingCompleted() {
			return "ONBOARDING_REQUIRED", "Você precisa completar o onboarding antes de usar este recurso", 428
		}
		code := ""
		if v := requested.Get("prayer_book_code"); rb.Present(v) {
			code = rb.ToS(v)
		} else if v := u.Preferences.Get("prayer_book_code"); rb.Present(v) {
			code = rb.ToS(v)
		} else {
			code = books.DefaultCode
		}
		return r.visibility(code)
	}
	code := requested.Get("prayer_book_code")
	if rb.Blank(code) {
		return "PRAYER_BOOK_REQUIRED", "O parâmetro preferences[prayer_book_code] é obrigatório", 400
	}
	if c, m, s := r.visibility(rb.ToS(code)); c != "" {
		return c, m, s
	}
	version := requested.Get("bible_version")
	if rb.Blank(version) {
		return "", "", 0
	}
	ok, err := store.ActiveBibleVersionExists(r.c.Ctx, rb.ToS(version))
	must(err)
	if ok {
		return "", "", 0
	}
	return "INVALID_BIBLE_VERSION", "Versão da Bíblia '" + rb.ToS(version) + "' não encontrada", 400
}

func (r *resolver) visibility(code string) (string, string, int) {
	pb, err := store.PrayerBookByCode(r.c.Ctx, code)
	must(err)
	if pb != nil && (!pb.ExternalOnly || r.apiKeyCalls) {
		return "", "", 0
	}
	return "INVALID_PRAYER_BOOK", "Prayer Book '" + code + "' não encontrado", 400
}

// sharedOffice is the subset of shared_offices the resolver needs.
type sharedOffice struct {
	ID             int64
	ShortCode      string
	PrayerBookCode string
	OfficeType     string
	Seed           int
	Preferences    *rb.Map
	UserID         *int64
}

// sharedOffice ports the lookup that raises SharedOfficeNotFoundError or
// SharedOfficeExpiredError.
func (r *resolver) sharedOffice() *sharedOffice {
	if r.shared != nil {
		return r.shared
	}
	code := r.c.ParamS("shared_office_code")
	so, err := store.ActiveSharedOffice(r.c.Ctx, code)
	must(err)
	if so == nil {
		expired, err := store.ExpiredSharedOfficeExists(r.c.Ctx, code)
		must(err)
		if expired {
			panic(&web.StandardError{Class: "Api::V1::Concerns::PreferencesResolver::SharedOfficeExpiredError", Message: "Este link expirou"})
		}
		panic(&web.StandardError{Class: "Api::V1::Concerns::PreferencesResolver::SharedOfficeNotFoundError", Message: "Link não encontrado"})
	}
	r.shared = &sharedOffice{ID: so.ID, ShortCode: so.ShortCode, PrayerBookCode: so.PrayerBookCode,
		OfficeType: so.OfficeType, Seed: so.Seed, Preferences: so.Preferences, UserID: so.UserID}
	return r.shared
}

var _ = strings.TrimSpace
var _ = users.Now
