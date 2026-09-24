package v1

import (
	"context"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/dailyoffice"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// withDailyOffice runs DailyOfficeController's before_actions and its
// rescue_from handlers for the shared-office lookups.
func withDailyOffice(validate bool, action func(c *web.Context, r *resolver)) web.HandlerFunc {
	return func(c *web.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				if se, ok := rec.(*web.StandardError); ok {
					switch se.Class {
					case "Api::V1::Concerns::PreferencesResolver::SharedOfficeNotFoundError":
						c.JSON(404, rb.M("error", se.Message))
						return
					case "Api::V1::Concerns::PreferencesResolver::SharedOfficeExpiredError":
						c.JSON(410, rb.M("error", se.Message))
						return
					}
				}
				panic(rec)
			}
		}()
		auth.AuthenticateUserOrAPIKey(c)
		r := newResolver(c)
		r.apiKeyCalls = auth.APIKey(c) != nil
		if !r.apiKeyCalls {
			auth.VerifyAppRequest(c)
		}
		if validate {
			r.validatePreferences()
			if !r.usingSharedOffice() {
				authorizePrayerBookAccess(c, r)
			}
		}
		action(c, r)
	}
}

// authorizePrayerBookAccess ports authorize_prayer_book_access!.
func authorizePrayerBookAccess(c *web.Context, r *resolver) {
	pb := r.book()
	if !pb.PremiumRequired {
		return
	}
	if u := auth.CurrentUser(c); u != nil && u.Premium() {
		return
	}
	c.RenderJSON(403, rb.M("success", false, "error", rb.M(
		"code", "PREMIUM_REQUIRED",
		"message", "O Prayer Book '"+rb.ToS(rb.Deref(pb.Name))+"' requer assinatura premium",
		"prayer_book_code", pb.Code,
		"premium_required", true)))
}

// rescueOfficeArgumentError ports "rescue ArgumentError => e" in the actions.
func rescueOfficeArgumentError(c *web.Context) {
	if rec := recover(); rec != nil {
		if re, ok := rec.(*rb.RubyError); ok && re.Class == "ArgumentError" {
			c.JSON(400, rb.M("error", "Erro ao buscar ofício: "+re.Message))
			return
		}
		panic(rec)
	}
}

// DailyOfficeToday ports DailyOfficeController#today.
var DailyOfficeToday = withDailyOffice(true, func(c *web.Context, r *resolver) {
	defer rescueOfficeArgumentError(c)
	date := today()
	if r.usingSharedOffice() {
		date = r.sharedOffice().Date
	}
	renderOffice(c, r, date, officeTypeFromSharedOrParams(c, r), nil)
})

// DailyOfficeShow ports DailyOfficeController#show.
var DailyOfficeShow = withDailyOffice(true, func(c *web.Context, r *resolver) {
	defer rescueOfficeArgumentError(c)
	var date civil.Date
	if r.usingSharedOffice() {
		date = r.sharedOffice().Date
	} else {
		date = parseDate(c)
	}
	renderOffice(c, r, date, officeTypeFromSharedOrParams(c, r), nil)
})

// DailyOfficeFamilyRite ports DailyOfficeController#family_rite.
var DailyOfficeFamilyRite = withDailyOffice(true, func(c *web.Context, r *resolver) {
	defer rescueOfficeArgumentError(c)
	date := parseDate(c)
	officeType := parseOfficeType(c, r)
	pb := r.book()
	if !books.For(pb.Code, pb.Features).SupportsFamilyRite() {
		c.JSON(422, rb.M("error", "O Prayer Book '"+r.code()+"' não suporta rito familiar"))
		return
	}
	prefs := r.resolvedPreferences().Values.Dup()
	prefs.Set("family_rite", true)
	renderOffice(c, r, date, officeType, prefs)
})

func renderOffice(c *web.Context, r *resolver, date civil.Date, officeType string, prefs *rb.Map) {
	response := fetchOffice(c, r, date, officeType, prefs)
	user := auth.CurrentUser(c)
	premium := user != nil && user.Premium()
	now := time.Now()
	if features.EnabledFor(c.Ctx, "daily_office_audio", user, premium, now) {
		response = addAudioTrack(c, r, response, user)
	}
	response.Set("features", features.ForUser(c.Ctx, user, premium, now))
	if user != nil {
		response.Set("user", userOfficeData(c.Ctx, user, date, officeType))
	}
	c.JSON(200, response)
}

// officeTypeFromSharedOrParams ports resolve_office_type_from_shared_or_params.
func officeTypeFromSharedOrParams(c *web.Context, r *resolver) string {
	if !r.usingSharedOffice() {
		return parseOfficeType(c, r)
	}
	officeType := r.sharedOffice().OfficeType
	requireAvailableOffice(r, officeType)
	return officeType
}

// parseOfficeType ports parse_office_type (defaults to morning).
func parseOfficeType(c *web.Context, r *resolver) string {
	officeType := "morning"
	if v := c.Param("office_type"); v != nil {
		officeType = rb.ToS(v)
	}
	requireAvailableOffice(r, officeType)
	return officeType
}

func requireAvailableOffice(r *resolver, officeType string) {
	pb := r.book()
	caps := books.For(pb.Code, pb.Features)
	for _, o := range append(caps.AvailableOffices(true), caps.AvailableOffices(false)...) {
		if o == officeType {
			return
		}
	}
	web.Raise("UnsupportedOfficeType", "Erro ao buscar ofício: O ofício '"+officeType+"' não está disponível "+
		"para o Livro de Oração "+pb.Code, "")
}

// fetchOffice ports fetch_office: the resolved preferences with the
// per-request office_type override, through DailyOfficeService.
func fetchOffice(c *web.Context, r *resolver, date civil.Date, officeType string, prefs *rb.Map) *rb.Map {
	if prefs == nil {
		prefs = r.resolvedPreferences().Values.Dup()
	}
	switch rb.ToS(r.preferencesParam().Get("office_type")) {
	case "family":
		pb := r.book()
		if books.For(pb.Code, pb.Features).SupportsFamilyRite() {
			prefs = prefs.Dup()
			prefs.Set("office_type", "family")
		} else {
			prefs = prefs.Dup()
			prefs.Delete("office_type")
		}
	case "traditional", "standard":
		prefs = prefs.Dup()
		prefs.Set("office_type", "traditional")
	}
	svc := dailyoffice.NewService(c.Ctx, date, officeType, prefs)
	base := svc.Base(c.Ctx)
	user := auth.CurrentUser(c)
	premium := user != nil && user.Premium()
	if !features.EnabledFor(c.Ctx, "daily_office_audio", user, premium, time.Now()) {
		return dailyoffice.RemoveAudioData(base).(*rb.Map)
	}
	return addAudioURLs(c.Ctx, base, svc, user, officeType)
}

// addAudioURLs ports add_audio_urls_to_response.
func addAudioURLs(ctx context.Context, response *rb.Map, svc *dailyoffice.Service, user *users.User, officeType string) *rb.Map {
	code := rb.ToS(svc.Prefs.Get("prayer_book_code"))
	pb, err := store.PrayerBookByCode(ctx, code)
	must(err)
	texts := store.LiturgicalTextsFor(ctx, pb)
	if len(texts) == 0 {
		return response
	}
	voice := user.PreferredAudioVoice()
	var used []*store.LiturgicalText
	seen := map[int64]bool{}
	var walk func(node any)
	walk = func(node any) {
		switch x := node.(type) {
		case *rb.Map:
			if slug := rb.ToS(x.Get("slug")); !rb.BlankString(slug) {
				if t := texts[slug]; t != nil {
					if url := AudioURLForVoice(t, voice); url != "" {
						if !seen[t.ID] {
							seen[t.ID] = true
							used = append(used, t)
						}
						x.Set("audio_url", url)
					}
				}
			}
			x.Each(func(_ string, v any) { walk(v) })
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(response)
	if len(used) > 0 {
		usages := make([]*rb.Map, len(used))
		for i, t := range used {
			usages[i] = rb.M("audio_type", "liturgical_text", "asset_key", itoa64(t.ID)+":"+voice,
				"liturgical_text_id", t.ID, "prayer_book_code", code, "office_type", officeType, "voice", voice)
		}
		RecordAudioUsage(ctx, user.ID, usages)
	}
	return response
}

// AudioURLForVoice ports LiturgicalText#audio_url_for_voice.
func AudioURLForVoice(t *store.LiturgicalText, voice string) string {
	if t.AudioURLs == nil {
		return ""
	}
	relative := rb.ToS(t.AudioURLs.Get(voice))
	if rb.BlankString(relative) {
		return ""
	}
	switch {
	case features.TestEnv:
		return relative
	case config.Present("AUDIO_CDN_HOST"):
		return config.Get("AUDIO_CDN_HOST") + relative
	case config.RailsEnv() == "production":
		return config.Fetch("APP_HOST", "https://api.estevao.app") + relative
	}
	return relative
}

// userOfficeData ports add_user_data (completion status is read fresh; Rails
// caches it for five minutes and expires it on create/destroy).
func userOfficeData(ctx context.Context, u *users.User, date civil.Date, officeType string) *rb.Map {
	var createdAt *time.Time
	err := db.Q().QueryRow(ctx, `SELECT "completions"."created_at" FROM "completions"
WHERE "completions"."user_id" = $1 AND "completions"."date_reference" = $2 AND "completions"."office_type" = $3 LIMIT 1`,
		u.ID, date.Time(), officeType).Scan(&createdAt)
	if err != nil && !db.NoRows(err) {
		panic(err)
	}
	var completedAt any
	if createdAt != nil {
		completedAt = rb.FormatTime(*createdAt)
	}
	return rb.M("current_streak", rb.Deref(u.CurrentStreak), "longest_streak", rb.Deref(u.LongestStreak),
		"completed", createdAt != nil, "completed_at", completedAt)
}

// DailyOfficePreferences ports DailyOfficeController#preferences.
var DailyOfficePreferences = withDailyOffice(false, func(c *web.Context, r *resolver) {
	pb := resolvedBookOrNil(r)
	var officeTypes []any
	if pb != nil {
		caps := books.For(pb.Code, pb.Features)
		seen := map[string]bool{}
		for _, o := range append(caps.AvailableOffices(true), caps.AvailableOffices(false)...) {
			if !seen[o] {
				seen[o] = true
				officeTypes = append(officeTypes, o)
			}
		}
	} else {
		officeTypes = []any{"morning", "midday", "evening", "compline"}
	}
	versions := stringColumn(c.Ctx, `SELECT "prayer_books"."code" FROM "prayer_books" WHERE "prayer_books"."external_only" = FALSE ORDER BY "prayer_books"."order" ASC`)
	languages := stringColumn(c.Ctx, `SELECT DISTINCT "prayer_books"."language" FROM "prayer_books" WHERE "prayer_books"."external_only" = FALSE ORDER BY "prayer_books"."language" ASC`)
	bibles := stringColumn(c.Ctx, `SELECT "bible_versions"."code" FROM "bible_versions" WHERE "bible_versions"."is_active" = TRUE ORDER BY "bible_versions"."id" ASC`)
	c.JSON(200, rb.M(
		"versions", versions,
		"languages", languages,
		"bible_versions", bibles,
		"lords_prayer_versions", []any{"traditional", "contemporary"},
		"creed_types", []any{"apostles", "nicene"},
		"confession_types", []any{"long", "short"},
		"office_types", officeTypes,
	))
})

// resolvedBookOrNil ports "resolved_prayer_book rescue nil".
func resolvedBookOrNil(r *resolver) (pb *store.PrayerBook) {
	defer func() {
		if rec := recover(); rec != nil {
			switch rec.(type) {
			case *web.StandardError, *web.DomainError, *rb.RubyError:
				pb = nil
			default:
				panic(rec)
			}
		}
	}()
	return r.book()
}

func stringColumn(ctx context.Context, sql string, args ...any) []any {
	rows, err := db.Q().Query(ctx, sql, args...)
	must(err)
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var s string
		must(rows.Scan(&s))
		out = append(out, s)
	}
	must(rows.Err())
	return out
}

func itoa64(n int64) string { return itoa(int(n)) }
