package v1

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// cacheKey ports Cacheable.key.
func cacheKey(parts ...any) string {
	out := []string{"v8"}
	for _, p := range parts {
		out = append(out, rb.ToS(p))
	}
	return strings.Join(out, "/")
}

// timestampVersion ports Cacheable.timestamp_version.
func timestampVersion(t *time.Time) string {
	if t == nil {
		return "0"
	}
	u := t.UTC()
	return strings.TrimLeft(u.Format("20060102150405")+fmt.Sprintf("%06d", u.Nanosecond()/1000), "0")
}

// iso8601 ports TimeWithZone#iso8601 in the application zone.
func iso8601(t time.Time) string { return t.In(rb.AppZone).Format("2006-01-02T15:04:05Z07:00") }

// appFilters runs authenticate_user_or_api_key and verify_app_request!.
func appFilters(c *web.Context) {
	auth.AuthenticateUserOrAPIKey(c)
	if auth.APIKey(c) == nil {
		auth.VerifyAppRequest(c)
	}
}

func withAppFilters(controller string, action func(c *web.Context)) web.HandlerFunc {
	return func(c *web.Context) {
		appFilters(c)
		action(c)
		auth.RecordAPIKeyUsage(c, controller)
	}
}

func catalogVersion(c *web.Context, table string) string {
	var count, maxID int64
	var maxUpdated *time.Time
	must(db.Q().QueryRow(c.Ctx, `SELECT COUNT(*), MAX(updated_at), COALESCE(MAX(id), 0) FROM `+table).Scan(&count, &maxUpdated, &maxID))
	return fmt.Sprintf("%d-%s-%d", count, timestampVersion(maxUpdated), maxID)
}

// --- PrayerBooksController ---------------------------------------------------------

func premiumScope(c *web.Context) string {
	if u := auth.CurrentUser(c); u != nil && u.Premium() {
		return "premium"
	}
	return "free"
}

func accessible(c *web.Context, pb *store.PrayerBook) bool {
	if !pb.PremiumRequired {
		return true
	}
	u := auth.CurrentUser(c)
	return u != nil && u.Premium()
}

func serializePrayerBook(c *web.Context, pb *store.PrayerBook) *rb.Map {
	caps := books.For(pb.Code, pb.Features)
	isRecommended := false
	if pb.IsRecommended != nil {
		isRecommended = *pb.IsRecommended
	}
	family := []any{}
	if caps.SupportsFamilyRite() {
		family = caps.OfficeNames(pb.Language, true)
	}
	offices := []any{}
	for _, o := range caps.AvailableOffices(false) {
		offices = append(offices, o)
	}
	return rb.M(
		"id", pb.Code, "code", pb.Code, "name", pb.Name, "full_name", pb.Name, "description", pb.Description,
		"language", pb.Language, "jurisdiction", pb.Jurisdiction, "year", pb.Year, "is_recommended", isRecommended,
		"premium_required", pb.PremiumRequired, "is_accessible", accessible(c, pb), "image_url", pb.ImageURL,
		"thumbnail_url", pb.ThumbnailURL, "pdf_url", pb.PdfURL, "available_offices", offices,
		"supports_family_rite", caps.SupportsFamilyRite(),
		"office_definitions", rb.M("standard", caps.OfficeNames(pb.Language, false), "family", family),
		"created_at", iso8601(pb.CreatedAt), "updated_at", iso8601(pb.UpdatedAt),
	)
}

// PrayerBooksIndex ports PrayerBooksController#index.
var PrayerBooksIndex = withAppFilters("prayer_books", func(c *web.Context) {
	language := "all"
	if c.ParamPresent("language") {
		language = c.ParamS("language")
	}
	scope := "app"
	if auth.APIKey(c) != nil {
		scope = "external"
	}
	version := catalogVersion(c, "prayer_books")
	etag := cacheKey("prayer_books", "etag", scope, language, premiumScope(c), "v_"+version)
	c.ExpiresIn(3600, false)
	if !c.Stale(etag) {
		return
	}
	where := "TRUE"
	var args []any
	if scope == "app" {
		where = "external_only = FALSE"
	}
	if language != "all" {
		args = append(args, language)
		where += fmt.Sprintf(" AND language = $%d", len(args))
	}
	list, err := store.PrayerBooksOrdered(c.Ctx, where, "is_recommended DESC, year DESC", args...)
	must(err)
	data := []any{}
	for _, pb := range list {
		data = append(data, serializePrayerBook(c, pb))
	}
	c.JSON(200, rb.M("data", data))
})

// PrayerBooksShow ports PrayerBooksController#show.
var PrayerBooksShow = withAppFilters("prayer_books", func(c *web.Context) {
	code := c.ParamS("code")
	pb, err := store.PrayerBookByCode(c.Ctx, code)
	must(err)
	if pb == nil || (pb.ExternalOnly && auth.APIKey(c) == nil) {
		c.JSON(404, rb.M("error", rb.M("code", "PRAYER_BOOK_NOT_FOUND", "message", "Prayer Book with code '"+code+"' not found")))
		return
	}
	scope := "app"
	if auth.APIKey(c) != nil {
		scope = "external"
	}
	updated := pb.UpdatedAt
	etag := cacheKey("prayer_books", "etag", scope, pb.Code, premiumScope(c), "pb_"+timestampVersion(&updated))
	c.ExpiresIn(3600, false)
	if !c.Stale(etag) {
		return
	}
	c.JSON(200, rb.M("data", serializePrayerBook(c, pb)))
})

// --- BibleVersionsController -------------------------------------------------------

// BibleVersionsIndex ports BibleVersionsController#index.
var BibleVersionsIndex = withAppFilters("bible_versions", func(c *web.Context) {
	var language any
	if c.ParamPresent("language") {
		language = c.ParamS("language")
	}
	version := catalogVersion(c, "bible_versions")
	lang := "all"
	if language != nil {
		lang = language.(string)
	}
	c.ExpiresIn(3600, true)
	if !c.Stale(cacheKey("bible_versions", "etag", lang, "v_"+version)) {
		return
	}
	var list []*store.BibleVersion
	var err error
	if language != nil {
		candidates := langs.BibleLanguageCandidatesFor(lang)
		if len(candidates) > 0 {
			var ph []string
			var args []any
			for i, cnd := range candidates {
				args = append(args, cnd)
				ph = append(ph, fmt.Sprintf("$%d", i+1))
			}
			list, err = store.BibleVersionsWhere(c.Ctx, `WHERE is_active = TRUE AND language IN (`+strings.Join(ph, ", ")+`) ORDER BY is_recommended DESC, name ASC`, args...)
		}
	} else {
		list, err = store.BibleVersionsWhere(c.Ctx, `WHERE is_active = TRUE ORDER BY is_recommended DESC, name ASC`)
	}
	must(err)
	data := []any{}
	for _, bv := range list {
		data = append(data, rb.M(
			"id", bv.Code, "code", strings.ToUpper(bv.Code), "name", bv.Name, "full_name", bv.FullName,
			"language", bv.Language, "description", bv.Description, "publisher", bv.Publisher, "year", bv.Year,
			"is_recommended", bv.IsRecommended, "license", bv.License, "created_at", iso8601(bv.CreatedAt),
		))
	}
	var latest *time.Time
	must(db.Q().QueryRow(c.Ctx, `SELECT MAX(updated_at) FROM bible_versions`).Scan(&latest))
	var last any
	if latest != nil {
		last = iso8601(*latest)
	}
	c.JSON(200, rb.M("data", data, "metadata", rb.M("total", len(data), "last_updated", last, "language", language)))
})

// --- CelebrationsController --------------------------------------------------------

var celebrationTypeNames = map[string]string{
	"principal_feast": "Festa Principal", "major_holy_day": "Dia Santo Principal", "festival": "Festival",
	"lesser_feast": "Festa Menor", "commemoration": "Comemoração",
}

var celebrationTypeDescriptions = map[string]string{
	"principal_feast": "Festas Principais (CAIXA ALTA, negrito, vermelho) - precedem domingos",
	"major_holy_day":  "Dias Santos Principais - Quarta-feira de Cinzas, Quinta-feira Santa, Sexta-feira da Paixão",
	"festival":        "Festivais (vermelho) - dias de apóstolos, evangelistas e santos importantes",
	"lesser_feast":    "Festas Menores (texto simples) - outros santos e dias comemorativos",
	"commemoration":   "Comemorações - menções nas intercessões apenas",
}

func typeName(t string) string {
	if v, ok := celebrationTypeNames[t]; ok {
		return v
	}
	return t
}

func formatCelebration(cel *liturgical.Celebration) *rb.Map {
	var fixed any
	if !cel.Movable {
		fixed = rb.M("mes", cel.FixedMonth, "dia", cel.FixedDay)
	}
	return rb.M(
		"id", cel.ID, "nome", cel.Name, "nome_latino", cel.LatinName, "tipo", cel.CelebrationType,
		"tipo_nome", typeName(cel.CelebrationType), "rank", cel.Rank, "data_fixa", fixed,
		"movel", cel.Movable, "cor_liturgica", cel.LiturgicalColor,
	)
}

func withCelebrations(validate bool, action func(c *web.Context, r *resolver)) web.HandlerFunc {
	return func(c *web.Context) {
		appFilters(c)
		r := newResolver(c)
		if validate {
			r.validatePreferences()
		}
		action(c, r)
		auth.RecordAPIKeyUsage(c, "celebrations")
	}
}

func bookCelebrations(c *web.Context, r *resolver) []*liturgical.Celebration {
	return celebrationsOf(c, r.book()).All
}

func byRank(list []*liturgical.Celebration) []*liturgical.Celebration {
	out := append([]*liturgical.Celebration(nil), list...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

func celebrationList(list []*liturgical.Celebration) []any {
	out := []any{}
	for _, cel := range list {
		out = append(out, formatCelebration(cel))
	}
	return out
}

// CelebrationsIndex ports CelebrationsController#index.
var CelebrationsIndex = withCelebrations(true, func(c *web.Context, r *resolver) {
	typ, movable := "all", "all"
	if c.ParamPresent("type") {
		typ = c.ParamS("type")
	}
	if c.ParamPresent("movable") {
		movable = c.ParamS("movable")
	}
	var list []*liturgical.Celebration
	for _, cel := range bookCelebrations(c, r) {
		if typ != "all" && cel.CelebrationType != typ {
			continue
		}
		if movable != "all" {
			if v, _ := rb.BoolCast(movable).(bool); cel.Movable != v {
				continue
			}
		}
		list = append(list, cel)
	}
	list = byRank(list)
	c.JSON(200, rb.M("total", len(list), "celebracoes", celebrationList(list)))
})

// railsIntegerID mirrors ActiveModel::Type::Integer casting of an id param.
func railsIntegerID(v any) (int64, bool) {
	s, ok := v.(string)
	if !ok || rb.BlankString(s) {
		return 0, false
	}
	t := strings.TrimLeft(s, " \t\n\v\f\r")
	if t == "" {
		return 0, false
	}
	i := 0
	if t[0] == '+' || t[0] == '-' {
		i = 1
	}
	if i >= len(t) || t[i] < '0' || t[i] > '9' {
		return 0, false
	}
	return int64(rb.StringToI(s)), true
}

// CelebrationsShow ports CelebrationsController#show.
var CelebrationsShow = withCelebrations(true, func(c *web.Context, r *resolver) {
	pb := r.book()
	id, ok := railsIntegerID(c.Param("id"))
	var cel *liturgical.Celebration
	if ok {
		for _, x := range celebrationsOf(c, pb).All {
			if x.ID == id {
				cel = x
			}
		}
	}
	if cel == nil {
		c.JSON(404, rb.M("error", "Celebração não encontrada"))
		return
	}
	var rules []byte
	must(db.Q().QueryRow(c.Ctx, `SELECT transfer_rules FROM celebrations WHERE id = $1`, cel.ID).Scan(&rules))
	var transfer any
	if rules != nil {
		v, err := rb.ParseJSON(rules)
		must(err)
		transfer = v
	}
	collects := []any{}
	rows, err := db.Q().Query(c.Ctx, `SELECT "collects"."text" FROM "collects" INNER JOIN "prayer_books" ON "prayer_books"."id" = "collects"."prayer_book_id" WHERE "collects"."celebration_id" = $1 AND "prayer_books"."language" = $2`, cel.ID, pb.Language)
	must(err)
	for rows.Next() {
		var text *string
		must(rows.Scan(&text))
		collects = append(collects, rb.M("texto", text))
	}
	rows.Close()
	must(rows.Err())
	readings := []any{}
	rrows, err := db.Q().Query(c.Ctx, `SELECT cycle, first_reading, psalm, second_reading, gospel FROM "lectionary_readings" WHERE "lectionary_readings"."celebration_id" = $1 AND "lectionary_readings"."service_type" = 'eucharist'`, cel.ID)
	must(err)
	for rrows.Next() {
		var cycle, first, psalm, second, gospel *string
		must(rrows.Scan(&cycle, &first, &psalm, &second, &gospel))
		readings = append(readings, rb.M("ciclo", cycle, "primeira_leitura", first, "salmo", psalm, "segunda_leitura", second, "evangelho", gospel))
	}
	rrows.Close()
	must(rrows.Err())
	detailed := formatCelebration(cel)
	detailed.Set("descricao", cel.Description)
	detailed.Set("pode_ser_transferida", cel.CanBeTransferred)
	detailed.Set("regras_transferencia", transfer)
	detailed.Set("regra_calculo", cel.CalculationRule)
	detailed.Set("coletas", collects)
	detailed.Set("leituras", readings)
	c.JSON(200, rb.M("celebracao", detailed))
})

// CelebrationsSearch ports CelebrationsController#search.
var CelebrationsSearch = withCelebrations(true, func(c *web.Context, r *resolver) {
	q := c.Param("q")
	if rb.Blank(q) {
		c.JSON(400, rb.M("error", "Parâmetro 'q' é obrigatório"))
		return
	}
	query, ok := q.(string)
	if !ok {
		web.Fail("NoMethodError", "undefined method `downcase' for %s", rb.Inspect(q))
	}
	needle := strings.ToLower(query)
	var list []*liturgical.Celebration
	for _, cel := range bookCelebrations(c, r) {
		if strings.Contains(strings.ToLower(cel.Name), needle) {
			list = append(list, cel)
		}
	}
	list = byRank(list)
	c.JSON(200, rb.M("total", len(list), "celebracoes", celebrationList(list)))
})

// CelebrationsByDate ports CelebrationsController#by_date.
var CelebrationsByDate = withCelebrations(true, func(c *web.Context, r *resolver) {
	month, day := rb.ToI(c.Param("month")), rb.ToI(c.Param("day"))
	if month < 1 || month > 12 || day < 1 || day > 31 {
		c.JSON(400, rb.M("error", "Data inválida"))
		return
	}
	var list []*liturgical.Celebration
	for _, cel := range bookCelebrations(c, r) {
		if cel.FixedMonth != nil && cel.FixedDay != nil && *cel.FixedMonth == month && *cel.FixedDay == day {
			list = append(list, cel)
		}
	}
	list = byRank(list)
	c.JSON(200, rb.M("mes", month, "dia", day, "total", len(list), "celebracoes", celebrationList(list)))
})

// CelebrationsTypes ports CelebrationsController#types.
var CelebrationsTypes = withCelebrations(false, func(c *web.Context, r *resolver) {
	out := []any{}
	for _, t := range liturgical.CelebrationTypeOrder {
		out = append(out, rb.M("valor", t, "nome", typeName(t), "descricao", celebrationTypeDescriptions[t]))
	}
	c.JSON(200, rb.M("tipos", out))
})
