package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func always(*req) bool { return true }

// Endpoints lists the v2 actions.
func Endpoints() map[string]web.HandlerFunc {
	return map[string]web.HandlerFunc{
		"api/v2/days#show":                    days.action(daysShow),
		"api/v2/days#index":                   days.action(daysIndex),
		"api/v2/readings#show":                readings.action(readingsShow),
		"api/v2/liturgical_explanations#show": explanations.action(explanationsShow),
		"api/v2/collects#show":                collectsCtrl.action(collectsShow),
		"api/v2/passages#show":                passages.action(passagesShow),
		"api/v2/prayer_books#index":           prayerBooks.action(prayerBooksIndex),
		"api/v2/prayer_books#show":            prayerBooks.action(prayerBooksShow),
		"api/v2/prayer_books#preferences":     prayerBooks.action(prayerBooksPreferences),
		"api/v2/bible_versions#index":         bibleVersions.action(bibleVersionsIndex),
		"api/v2/celebrations#index":           celebrations.action(celebrationsIndex),
		"api/v2/celebrations#show":            celebrations.action(celebrationsShow),
		"api/v2/celebrations#types":           celebrations.action(celebrationsTypes),
		"api/v2/years#show":                   years.action(yearsShow),
		"api/v2/years#seasons":                years.action(yearsSeasons),
		"api/v2/years#key_dates":              years.action(yearsKeyDates),
		"api/v2/years#lectionary_cycle":       years.action(yearsLectionaryCycle),
	}
}

// --- DaysController -----------------------------------------------------------

var days = &controller{
	name: "days",
	includes: []string{"readings", "readings.text", "readings.morning", "readings.evening", "readings.alternatives",
		"collect", "collect.text", "celebrations", "explanation"},
	fields:      spec("day", dayAttributes, "reading", readingAttributes, "collect", collectAttributes),
	contextMeta: always,
}

func (r *req) explanationService() string {
	if !r.includes().Has("explanation") {
		return ""
	}
	return explanationServiceName(r.c.Param("service"))
}

func dims(service string) []string {
	if service == "" {
		return nil
	}
	return []string{service}
}

var dayReadingKeys = []string{"readings", "morning_readings", "evening_readings"}

// trimDay ports trim_nested plus fields.apply(:day, ...).
func (r *req) trimDay(v any) any {
	day, _ := v.(*rb.Map)
	f := r.fields()
	if f.Any() {
		day = day.Dup()
		for _, k := range dayReadingKeys {
			if list, ok := day.Lookup(k); ok {
				day.Set(k, applyList(f, "reading", list))
			}
		}
		if list, ok := day.Lookup("collect"); ok {
			day.Set("collect", applyList(f, "collect", list))
		}
	}
	return f.Apply("day", day)
}

func applyList(f *FieldSet, typ string, v any) []any {
	list, _ := v.([]any)
	out := make([]any, 0, len(list))
	for _, e := range list {
		m, _ := e.(*rb.Map)
		out = append(out, f.Apply(typ, m))
	}
	return out
}

func daysShow(r *req) {
	date := ParseDate(r.c.Param("date"))
	service := r.explanationService()
	key := r.payloadKey(append([]string{"day", date.ISO()}, dims(service)...)...)
	if !r.staleResource(-1, key) {
		return
	}
	day := r.cached(key, ttlReadings, func() any { return r.day(date, service) })
	r.renderResource(r.trimDay(day), nil, nil, -1, maxAgeFor(date))
}

func daysIndex(r *req) {
	first, last := DateRange(r.c, RangeLimit(r.includes()))
	service := r.explanationService()
	key := r.payloadKey(append([]string{"days", first.ISO(), last.ISO()}, dims(service)...)...)
	count := last.Sub(first) + 1
	if !r.staleResource(RequestCost(r.includes(), count), key) {
		return
	}
	list, _ := r.cached(key, ttlReadings, func() any {
		out := []any{}
		for d := first; d <= last; d++ {
			out = append(out, r.day(d, service))
		}
		return out
	}).([]any)
	data := make([]any, len(list))
	for i, d := range list {
		data[i] = r.trimDay(d)
	}
	r.renderResource(data, rb.M("from", first.ISO(), "to", last.ISO(), "count", len(list)), nil,
		RequestCost(r.includes(), len(list)), datedMaxAge)
}

// --- ReadingsController ----------------------------------------------------------

var readings = &controller{
	name: "readings", includes: []string{"text", "alternatives"},
	fields: spec("reading", readingAttributes), contextMeta: always,
}

func readingsShow(r *req) {
	date := ParseDate(r.c.Param("date"))
	var service any = "daily"
	if v := presence(r.c.Param("service")); v != nil {
		service = v
	}
	key := r.payloadKey("readings", date.ISO(), toS(service))
	if !r.staleResource(-1, key) {
		return
	}
	payload := r.cached(key, ttlReadings, func() any {
		var names []string
		for _, n := range r.includes().Names() {
			names = append(names, "readings."+n)
		}
		rr := &readingsResource{r: r, includes: NewIncludeSet(names)}
		return rr.call(date, service)
	}).(*rb.Map)
	f := r.fields()
	if f.Any() {
		payload = payload.Dup()
		if list, ok := payload.Lookup("readings"); ok {
			payload.Set("readings", applyList(f, "reading", list))
		}
		if services, ok := payload.Get("services").(*rb.Map); ok {
			trimmed := rb.NewMap()
			services.Each(func(k string, v any) { trimmed.Set(k, applyList(f, "reading", v)) })
			payload.Set("services", trimmed)
		}
	}
	r.renderResource(payload, nil, nil, -1, maxAgeFor(date))
}

// --- LiturgicalExplanationsController ----------------------------------------------

var explanations = &controller{name: "liturgical_explanations", fields: spec(), contextMeta: always}

var explanationCost = RequestCost(NewIncludeSet([]string{"explanation"}), 1)

func explanationsShow(r *req) {
	date := ParseDate(r.c.Param("date"))
	service := explanationServiceName(r.c.Param("service"))
	key := r.payloadKey("liturgical_explanations", date.ISO(), service)
	if !r.staleResource(explanationCost, key) {
		return
	}
	payload := r.cached(key, ttlReadings, func() any {
		explanationServiceName(service)
		return r.explanation(date, service)
	})
	r.renderResource(payload, nil, nil, explanationCost, maxAgeFor(date))
}

// --- CollectsController -------------------------------------------------------

var collectsCtrl = &controller{
	name: "collects", includes: []string{"text"}, fields: spec("collect", collectAttributes), contextMeta: always,
}

func collectsShow(r *req) {
	date := ParseDate(r.c.Param("date"))
	key := r.payloadKey("collects", date.ISO())
	if !r.staleResource(-1, key) {
		return
	}
	payload := r.cached(key, ttlReadings, func() any {
		return rb.M("date", date.ISO(), "collects", r.collects(date, r.includes().Has("text")))
	}).(*rb.Map)
	if f := r.fields(); f.Any() {
		payload = payload.Merge(rb.M("collects", applyList(f, "collect", payload.Get("collects"))))
	}
	r.renderResource(payload, nil, nil, -1, maxAgeFor(date))
}

// --- PassagesController -------------------------------------------------------

var passages = &controller{name: "passages", fields: spec()}

func required(v any, code, message string) string {
	if rb.Present(v) {
		return toS(v)
	}
	domainError("InvalidParameter", message, code, nil)
	return ""
}

func passagesShow(r *req) {
	reference := required(r.c.Param("ref"), "MISSING_REFERENCE", "Informe a referência em 'ref'.")
	code := required(r.c.Param("bible"), "MISSING_BIBLE_VERSION", "Informe a versão bíblica em 'bible'.")
	version, err := store.BibleVersionByCode(r.c.Ctx, code)
	must(err)
	if version == nil {
		domainError("InvalidPreference", "Versão bíblica desconhecida: '"+code+"'.", "UNKNOWN_BIBLE_VERSION", rb.M("bible", code))
	}
	format := textVerses
	if v := presence(r.c.Param("text_format")); v != nil {
		format = toS(v)
	}
	valid := false
	for _, f := range textFormats {
		if f == format {
			valid = true
		}
	}
	if !valid {
		domainError("InvalidParameter", "text_format inválido: '"+format+"'.", "INVALID_TEXT_FORMAT", rb.M("allowed", strs(textFormats)))
	}
	key := cacheKey("api", "v2", "passage", version.Code, reference, format)
	r.setCost(RequestCost(NewIncludeSet(nil), 1))
	if !r.c.Stale(key) {
		return
	}
	content, _ := r.cached(key, ttlReadings, func() any {
		s, err := bible.NewTextService(version.Code, nil).FetchPassageStructured(r.c.Ctx, reference)
		must(err)
		if s == nil {
			return nil
		}
		return s
	}).(*rb.Map)
	if content == nil || content.Len() == 0 {
		domainError("ReadingNotFound", "Passagem não encontrada: '"+reference+"' em "+version.Code+".", "PASSAGE_NOT_FOUND",
			rb.M("reference", reference, "bible", version.Code))
	}
	ref := rb.Squish(rb.ToS(content.Get("reference")))
	if ref == "" {
		web.Raise("InvalidBibleReference", "Bible reference cannot be blank", "INVALID_BIBLE_REFERENCE")
	}
	data := rb.M("reference", content.Get("reference"), "bible", content.Get("translation"),
		"text", readingText(content, format))
	r.renderResource(data, rb.M("bible", version.Code), nil, -1, datedMaxAge)
}

// --- PrayerBooksController -------------------------------------------------------

var prayerBookAttributes = []string{"code", "name", "description", "language", "jurisdiction", "year", "is_recommended",
	"image_url", "thumbnail_url", "pdf_url", "capabilities", "preferences_url", "updated_at"}

var prayerBooks = &controller{name: "prayer_books", fields: spec("prayer_book", prayerBookAttributes)}

func catalogVersion(r *req, table string) string {
	var count, maxID int64
	var maxUpdated *time.Time
	must(db.Q().QueryRow(r.c.Ctx, `SELECT COUNT(*), MAX(updated_at), COALESCE(MAX(id), 0) FROM `+table).Scan(&count, &maxUpdated, &maxID))
	version := "0"
	if maxUpdated != nil {
		version = timestampVersion(*maxUpdated)
	}
	return strconv.FormatInt(count, 10) + "-" + version + "-" + strconv.FormatInt(maxID, 10)
}

func iso8601(t time.Time) string { return t.In(rb.AppZone).Format("2006-01-02T15:04:05Z07:00") }

func setIf(m *rb.Map, k string, v any) {
	if v = rb.Deref(v); v != nil {
		m.Set(k, v)
	}
}

// serializePrayerBook ports PrayerBookSerializer.call.
func serializePrayerBook(pb *store.PrayerBook) *rb.Map {
	caps := books.For(pb.Code, pb.Features)
	m := rb.M("code", pb.Code)
	setIf(m, "name", pb.Name)
	setIf(m, "description", pb.Description)
	m.Set("language", pb.Language)
	setIf(m, "jurisdiction", pb.Jurisdiction)
	setIf(m, "year", pb.Year)
	m.Set("is_recommended", pb.IsRecommended != nil && *pb.IsRecommended)
	setIf(m, "image_url", pb.ImageURL)
	setIf(m, "thumbnail_url", pb.ThumbnailURL)
	setIf(m, "pdf_url", pb.PdfURL)
	family := caps.SupportsFamilyRite()
	readingTypes := strs(caps.AvailableReadingTypes())
	defaultType := caps.NormalizeReadingType(nil)
	if len(readingTypes) == 0 {
		readingTypes = []any{defaultType}
	}
	var defaultVariant any
	if v := caps.DefaultLectionaryVariant(); v != "" {
		defaultVariant = v
	}
	familyOffices := []any{}
	if family {
		familyOffices = caps.OfficeNames(pb.Language, true)
	}
	m.Set("capabilities", rb.M(
		"lectionary", rb.M(
			"reading_types", readingTypes,
			"default_reading_type", defaultType,
			"variants", strs(caps.AvailableLectionaryVariants()),
			"default_variant", defaultVariant,
			"supports_vigil", caps.SupportsVigil(),
		),
		"daily_office", rb.M(
			"offices", caps.OfficeNames(pb.Language, false),
			"family_rite", rb.M("supported", family, "offices", familyOffices),
		),
	))
	m.Set("preferences_url", "/api/v2/prayer-books/"+pb.Code+"/preferences")
	m.Set("updated_at", iso8601(pb.UpdatedAt))
	return m
}

func applyMaps(f *FieldSet, typ string, list []any) []any {
	out := make([]any, len(list))
	for i, e := range list {
		m, _ := e.(*rb.Map)
		out[i] = f.Apply(typ, m)
	}
	return out
}

func prayerBooksIndex(r *req) {
	language := presence(r.c.Param("lang"))
	langKey := "all"
	if language != nil {
		langKey = toS(language)
	}
	key := cacheKey("api", "v2", "prayer_books", langKey, "v_"+catalogVersion(r, "prayer_books"))
	if !r.staleResource(-1, key) {
		return
	}
	list, _ := r.cached(key, ttlPrayerBook, func() any {
		where := ""
		var args []any
		if language != nil {
			where = "language = $1"
			args = append(args, toS(language))
		}
		pbs, err := store.PrayerBooksOrdered(r.c.Ctx, where, "is_recommended DESC, year DESC, code ASC", args...)
		must(err)
		out := []any{}
		for _, pb := range pbs {
			out = append(out, serializePrayerBook(pb))
		}
		return out
	}).([]any)
	meta := rb.M("count", len(list))
	if language != nil {
		meta.Set("language", language)
	}
	r.renderResource(applyMaps(r.fields(), "prayer_book", list), meta, nil, -1, datedMaxAge)
}

func (r *req) prayerBookParam() *store.PrayerBook {
	code := r.c.Param("code")
	pb, err := store.PrayerBookByCode(r.c.Ctx, toS(code))
	must(err)
	if pb == nil {
		domainError("UnsupportedPrayerBook", "Livro de oração desconhecido: '"+toS(code)+"'.", "UNKNOWN_PRAYER_BOOK",
			rb.M("book", toS(code)))
	}
	return pb
}

func prayerBooksShow(r *req) {
	pb := r.prayerBookParam()
	key := cacheKey("api", "v2", "prayer_book", pb.Code, "pb_"+timestampVersion(pb.UpdatedAt))
	if !r.staleResource(-1, key) {
		return
	}
	book := r.cached(key, ttlPrayerBook, func() any { return serializePrayerBook(pb) }).(*rb.Map)
	r.renderResource(r.fields().Apply("prayer_book", book), rb.M("prayer_book", pb.Code), nil, -1, datedMaxAge)
}

func prayerBooksPreferences(r *req) {
	pb := r.prayerBookParam()
	key := cacheKey("api", "v2", "prayer_book_preferences", pb.Code, "pb_"+timestampVersion(pb.UpdatedAt))
	if !r.staleResource(-1, key) {
		return
	}
	payload := r.cached(key, ttlPrayerBook, func() any {
		cats, count, last, err := prefs.CategoriesForAPI(r.c.Ctx, pb)
		must(err)
		m := rb.M("prayer_book", pb.Code, "categories", cats, "preference_count", count)
		if last != nil {
			m.Set("preferences_updated_at", iso8601(*last))
		}
		return m
	}).(*rb.Map)
	r.renderResource(payload, rb.M("prayer_book", pb.Code, "count", payload.Get("preference_count")), nil, -1, datedMaxAge)
}

// --- BibleVersionsController -----------------------------------------------------

var bibleVersionAttributes = []string{"code", "name", "full_name", "language", "description", "publisher", "year",
	"is_recommended", "versification_system", "license"}

var bibleVersions = &controller{name: "bible_versions", fields: spec("bible_version", bibleVersionAttributes)}

func bibleVersionsIndex(r *req) {
	language := presence(r.c.Param("lang"))
	langKey := "all"
	if language != nil {
		langKey = toS(language)
	}
	key := cacheKey("api", "v2", "bible_versions", langKey, "v_"+catalogVersion(r, "bible_versions"))
	if !r.staleResource(-1, key) {
		return
	}
	list, _ := r.cached(key, ttlPrayerBook, func() any {
		var versions []*store.BibleVersion
		var err error
		order := ` ORDER BY is_recommended DESC, name ASC, code ASC`
		if language != nil {
			candidates := langs.BibleLanguageCandidatesFor(toS(language))
			if len(candidates) > 0 {
				var ph []string
				var args []any
				for i, cnd := range candidates {
					args = append(args, cnd)
					ph = append(ph, "$"+strconv.Itoa(i+1))
				}
				versions, err = store.BibleVersionsWhere(r.c.Ctx, `WHERE is_active = TRUE AND language IN (`+strings.Join(ph, ", ")+`)`+order, args...)
			}
		} else {
			versions, err = store.BibleVersionsWhere(r.c.Ctx, `WHERE is_active = TRUE`+order)
		}
		must(err)
		out := []any{}
		for _, bv := range versions {
			m := rb.M("code", bv.Code, "name", bv.Name)
			setIf(m, "full_name", bv.FullName)
			setIf(m, "language", bv.Language)
			setIf(m, "description", bv.Description)
			setIf(m, "publisher", bv.Publisher)
			setIf(m, "year", bv.Year)
			setIf(m, "is_recommended", bv.IsRecommended)
			m.Set("versification_system", bv.VersificationSystem)
			setIf(m, "license", bv.License)
			out = append(out, m)
		}
		return out
	}).([]any)
	meta := rb.M("count", len(list))
	if language != nil {
		meta.Set("language", language)
	}
	var latest *time.Time
	must(db.Q().QueryRow(r.c.Ctx, `SELECT MAX(updated_at) FROM bible_versions`).Scan(&latest))
	if latest != nil {
		meta.Set("catalog_updated_at", iso8601(*latest))
	}
	r.renderResource(applyMaps(r.fields(), "bible_version", list), meta, nil, -1, datedMaxAge)
}

// --- CelebrationsController -------------------------------------------------------

var celebrationCatalogAttributes = []string{"id", "slug", "name", "latin_name", "type", "rank", "color", "description",
	"movable", "fixed_date", "can_be_transferred", "calculation_rule", "date", "transferred", "transfer_rules",
	"collects", "readings"}

var celebrations = &controller{
	name:        "celebrations",
	fields:      spec("celebration", celebrationCatalogAttributes, "celebration_type", []string{"value", "name"}),
	contextMeta: func(r *req) bool { return r.c.Action != "types" },
}

// serializeCatalogCelebration ports CelebrationCatalogSerializer.call.
func serializeCatalogCelebration(c *liturgical.Celebration) *rb.Map {
	m := rb.M("id", c.ID)
	setIf(m, "slug", c.PostSlugPtr())
	m.Set("name", c.Name)
	setIf(m, "latin_name", c.LatinName)
	m.Set("type", c.CelebrationType)
	m.Set("rank", c.Rank)
	setIf(m, "color", c.LiturgicalColor)
	setIf(m, "description", c.Description)
	m.Set("movable", c.Movable)
	if !c.Movable {
		fd := rb.NewMap()
		setIf(fd, "month", c.FixedMonth)
		setIf(fd, "day", c.FixedDay)
		if fd.Len() > 0 {
			m.Set("fixed_date", fd)
		}
	}
	m.Set("can_be_transferred", c.CanBeTransferred)
	setIf(m, "calculation_rule", c.CalculationRule)
	return m
}

// celebrationQuery ports CelebrationCatalogQuery.
type celebrationQuery struct {
	r       *req
	filters *rb.Map
}

func (q *celebrationQuery) intFilter(k string) (int, bool) {
	v, ok := q.filters.Lookup(k)
	if !ok {
		return 0, false
	}
	return v.(int), true
}

func numericFilter(name string, value any, lo, hi int) int {
	raw := toS(value)
	n := rb.StringToI(raw)
	digits := raw != ""
	for _, c := range raw {
		if c < '0' || c > '9' {
			digits = false
		}
	}
	if digits && n >= lo && n <= hi {
		return n
	}
	domainError("InvalidDate", name+" deve estar entre "+strconv.Itoa(lo)+" e "+strconv.Itoa(hi)+".", "", rb.M(name, raw))
	return 0
}

var celebrationTypeFilters = append(append([]string{}, liturgical.CelebrationTypeOrder...), "sunday")

func newCelebrationQuery(r *req) *celebrationQuery {
	r.context()
	c := r.c
	f := rb.NewMap()
	if c.ParamPresent("year") {
		first, _ := YearBounds(c.Param("year"))
		f.Set("year", first.Year())
	}
	if c.ParamPresent("month") {
		f.Set("month", numericFilter("month", c.Param("month"), 1, 12))
	}
	if c.ParamPresent("day") {
		f.Set("day", numericFilter("day", c.Param("day"), 1, 31))
	}
	_, hasYear := f.Lookup("year")
	month, hasMonth := f.Lookup("month")
	day, hasDay := f.Lookup("day")
	if hasDay && !hasMonth && !hasYear {
		domainError("InvalidParameter", "day exige month ou year.", "INVALID_CELEBRATION_DATE", nil)
	}
	if hasMonth && hasDay {
		y := 2000
		if v, ok := f.Lookup("year"); ok {
			y = v.(int)
		}
		if _, ok := civil.New(y, month.(int), day.(int)); !ok {
			var year any
			if v, ok := f.Lookup("year"); ok {
				year = v
			}
			domainError("InvalidDate", "Data inválida para os filtros month/day.", "", rb.M("month", month, "day", day, "year", year))
		}
	}
	if c.ParamPresent("type") {
		t := toS(c.Param("type"))
		ok := false
		for _, v := range celebrationTypeFilters {
			if v == t {
				ok = true
			}
		}
		if !ok {
			domainError("InvalidPreference", "Tipo de celebração desconhecido: '"+t+"'.", "INVALID_CELEBRATION_TYPE",
				rb.M("type", t, "allowed", strs(celebrationTypeFilters)))
		}
		f.Set("type", t)
	}
	if c.ParamPresent("movable") {
		switch strings.ToLower(toS(c.Param("movable"))) {
		case "true", "1":
			f.Set("movable", true)
		case "false", "0":
			f.Set("movable", false)
		default:
			domainError("InvalidParameter", "movable deve ser true ou false.", "INVALID_BOOLEAN", rb.M("movable", toS(c.Param("movable"))))
		}
	}
	if c.ParamPresent("q") {
		value := rb.Strip(toS(c.Param("q")))
		if rb.BlankString(value) || len([]rune(value)) > 200 {
			domainError("InvalidParameter", "q deve conter entre 1 e 200 caracteres.", "INVALID_SEARCH_QUERY", rb.M("max_length", 200))
		}
		f.Set("q", rb.Downcase(value))
	}
	if f.Get("type") == "sunday" && !hasYear {
		domainError("InvalidParameter", "type=sunday exige year porque os domingos dependem do calendário anual.",
			"INVALID_CELEBRATION_TYPE", rb.M("type", "sunday", "required", "year"))
	}
	return &celebrationQuery{r: r, filters: f}
}

func (q *celebrationQuery) cost() int {
	if _, ok := q.filters.Lookup("year"); ok {
		return 3
	}
	return 1
}

func (q *celebrationQuery) cursorScope() string {
	sum := sha256.Sum256(rb.ToJSON([]any{q.r.context().code(), q.filters}))
	return hex.EncodeToString(sum[:])[:20]
}

func (q *celebrationQuery) cursorKey(c *rb.Map) []any {
	if _, ok := q.filters.Lookup("year"); ok {
		return []any{c.Get("date"), c.Get("name"), c.Get("type")}
	}
	return []any{rb.ToI(c.Get("rank")), rb.ToI(c.Get("id"))}
}

// filtersInspect ports Hash#to_s on the filters (symbol keys).
func (q *celebrationQuery) filtersInspect() string {
	var parts []string
	q.filters.Each(func(k string, v any) { parts = append(parts, ":"+k+"=>"+rb.Inspect(v)) })
	return "{" + strings.Join(parts, ", ") + "}"
}

func (q *celebrationQuery) call() []any {
	pb := q.r.context().pb
	bc, err := store.CelebrationsForBook(q.r.c.Ctx, pb)
	must(err)
	if year, ok := q.intFilter("year"); ok {
		return q.annual(year, bc)
	}
	out := []any{}
	for _, c := range bc.All {
		if t, ok := q.filters.Lookup("type"); ok && c.CelebrationType != t {
			continue
		}
		if m, ok := q.filters.Lookup("movable"); ok && c.Movable != m {
			continue
		}
		if m, ok := q.intFilter("month"); ok && (c.FixedMonth == nil || *c.FixedMonth != m) {
			continue
		}
		if d, ok := q.intFilter("day"); ok && (c.FixedDay == nil || *c.FixedDay != d) {
			continue
		}
		if s, ok := q.filters.Lookup("q"); ok && !strings.Contains(rb.Downcase(c.Name), s.(string)) {
			continue
		}
		out = append(out, serializeCatalogCelebration(c))
	}
	return out
}

func (q *celebrationQuery) annual(year int, bc *liturgical.BookCelebrations) []any {
	typ := ""
	if t, ok := q.filters.Lookup("type"); ok {
		typ = t.(string)
	}
	entries, _ := liturgical.NewYearOverview(year, q.r.context().code(), bc, true).Celebrations(typ, false).([]any)
	byNameType := map[string]*liturgical.Celebration{}
	for _, c := range bc.All {
		byNameType[c.Name+"\x00"+c.CelebrationType] = c
	}
	language := q.r.context().language()
	out := []any{}
	for _, e := range entries {
		entry := e.(*rb.Map)
		date, _ := civil.ParseISO(rb.ToS(entry.Get("date")))
		if m, ok := q.intFilter("month"); ok && date.Month() != m {
			continue
		}
		if d, ok := q.intFilter("day"); ok && date.Day() != d {
			continue
		}
		name := rb.ToS(entry.Get("name"))
		if s, ok := q.filters.Lookup("q"); ok && !strings.Contains(rb.Downcase(name), s.(string)) {
			continue
		}
		cel := byNameType[name+"\x00"+rb.ToS(entry.Get("type"))]
		movable := cel != nil && cel.Movable
		if m, ok := q.filters.Lookup("movable"); ok && movable != m {
			continue
		}
		s := rb.NewMap()
		if cel != nil {
			s.Set("id", cel.ID)
		}
		setIf(s, "slug", entry.Get("post_slug"))
		setIf(s, "name", entry.Get("name"))
		setIf(s, "type", entry.Get("type"))
		setIf(s, "color", entry.Get("color"))
		setIf(s, "date", entry.Get("date"))
		setIf(s, "transferred", entry.Get("transferred"))
		if col := s.Get("color"); rb.Present(col) {
			s.Set("color", liturgical.TranslateColor(rb.ToS(col), language))
		}
		out = append(out, s)
	}
	return out
}

func celebrationsIndex(r *req) {
	q := newCelebrationQuery(r)
	limit := CursorLimit(r.c.Param("limit"))
	cursor := DecodeCursor(r.c.Param("cursor"), q.cursorScope())
	hasCursor := len(cursor) > 0
	key := r.payloadKey("celebrations", q.filtersInspect())
	if !r.staleResource(q.cost(), key, retrieveCacheKey(cursorValue(cursor, hasCursor)), strconv.Itoa(limit)) {
		return
	}
	raw, _ := r.cached(key, ttlStaticData, func() any { return q.call() }).([]any)
	results := make([]*rb.Map, len(raw))
	for i, e := range raw {
		results[i] = e.(*rb.Map)
	}
	page, next := CursorPage(results, limit, cursor, hasCursor, q.cursorScope(), q.cursorKey)
	data := make([]any, len(page))
	for i, p := range page {
		data[i] = r.fields().Apply("celebration", p)
	}
	r.renderResource(data, rb.M("count", len(data), "total", len(results)), r.collectionLinks(next), q.cost(), datedMaxAge)
}

func cursorValue(cursor []any, has bool) any {
	if !has {
		return nil
	}
	return cursor
}

// retrieveCacheKey ports ActiveSupport::Cache.retrieve_cache_key.
func retrieveCacheKey(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = retrieveCacheKey(e)
		}
		return strings.Join(parts, "/")
	case *rb.Map:
		var parts []string
		x.Each(func(k string, e any) { parts = append(parts, k+"/"+retrieveCacheKey(e)) })
		return strings.Join(parts, "/")
	}
	return rb.ToS(v)
}

// collectionLinks ports collection_links.
func (r *req) collectionLinks(next string) *rb.Map {
	path := r.c.R.URL.EscapedPath()
	if r.c.R.URL.RawPath == "" {
		path = r.c.R.URL.Path
	}
	query := r.c.QueryParams()
	self := path
	if rb.Present(query) {
		self += "?" + rb.ToQuery(query, "")
	}
	if next == "" {
		return rb.M("self", self)
	}
	nq := query.Dup()
	nq.Set("cursor", next)
	return rb.M("self", self, "next", path+"?"+rb.ToQuery(nq, ""))
}

func celebrationsShow(r *req) {
	id := toS(r.c.Param("id"))
	notFound := func() {
		domainError("ReadingNotFound", "Celebração não encontrada: '"+id+"'.", "CELEBRATION_NOT_FOUND",
			rb.M("id", id, "book", r.context().code()))
	}
	digits := id != ""
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			digits = false
		}
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if !digits || (err == nil && n <= 0) {
		notFound()
	}
	if err != nil {
		// A positive id beyond bigint: find_by answers nothing.
		notFound()
	}
	cel, transferRules, err := store.CelebrationWithTransferRules(r.c.Ctx, n, r.context().pb.ID)
	must(err)
	if cel == nil {
		notFound()
	}
	key := r.payloadKey("celebration", strconv.FormatInt(cel.ID, 10))
	if !r.staleResource(-1, key) {
		return
	}
	detail := r.cached(key, ttlStaticData, func() any { return celebrationDetail(r, cel, transferRules) }).(*rb.Map)
	r.renderResource(r.fields().Apply("celebration", detail), nil, nil, -1, datedMaxAge)
}

// celebrationDetail ports CelebrationCatalogSerializer.detail.
func celebrationDetail(r *req, cel *liturgical.Celebration, transferRules []byte) *rb.Map {
	m := serializeCatalogCelebration(cel)
	var rules any
	if transferRules != nil {
		v, err := rb.ParseJSON(transferRules)
		must(err)
		rules = v
	}
	m.Set("transfer_rules", rules)
	rows, err := db.Q().Query(r.c.Ctx, `SELECT text, preface FROM collects WHERE celebration_id = $1 AND prayer_book_id = $2 ORDER BY id ASC`,
		cel.ID, cel.PrayerBookID)
	must(err)
	cols := []any{}
	for rows.Next() {
		var text, preface *string
		must(rows.Scan(&text, &preface))
		c := rb.NewMap()
		setIf(c, "text", text)
		setIf(c, "preface", preface)
		cols = append(cols, c)
	}
	rows.Close()
	must(rows.Err())
	m.Set("collects", cols)
	rows, err = db.Q().Query(r.c.Ctx, `SELECT cycle, reading_type, first_reading, psalm, second_reading, gospel FROM lectionary_readings
		WHERE celebration_id = $1 AND prayer_book_id = $2 AND service_type = 'eucharist' ORDER BY cycle ASC, reading_type ASC`,
		cel.ID, cel.PrayerBookID)
	must(err)
	reads := []any{}
	for rows.Next() {
		var v [6]*string
		must(rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5]))
		rm := rb.NewMap()
		for i, k := range []string{"cycle", "reading_type", "first_reading", "psalm", "second_reading", "gospel"} {
			setIf(rm, k, v[i])
		}
		reads = append(reads, rm)
	}
	rows.Close()
	must(rows.Err())
	m.Set("readings", reads)
	return m
}

var celebrationTypeLabels = map[string][2]string{
	"principal_feast": {"Principal Feast", "Festa Principal"}, "major_holy_day": {"Major Holy Day", "Dia Santo Principal"},
	"festival": {"Festival", "Festival"}, "lesser_feast": {"Lesser Feast", "Festa Menor"},
	"commemoration": {"Commemoration", "Comemoração"}, "sunday": {"Sunday", "Domingo"},
}

func celebrationsTypes(r *req) {
	language := presence(r.c.Param("lang"))
	langKey := "en"
	if language != nil {
		langKey = toS(language)
	}
	key := cacheKey("api", "v2", "celebration_types", langKey)
	if !r.staleResource(-1, key) {
		return
	}
	pt := strings.HasPrefix(rb.Downcase(toS(language)), "pt")
	data := []any{}
	for _, t := range celebrationTypeFilters {
		name := celebrationTypeLabels[t][0]
		if pt {
			name = celebrationTypeLabels[t][1]
		}
		data = append(data, r.fields().Apply("celebration_type", rb.M("value", t, "name", name)))
	}
	r.renderResource(data, rb.M("count", len(data)), nil, -1, datedMaxAge)
}

// --- YearsController ------------------------------------------------------------

var years = &controller{
	name: "years",
	fields: spec(
		"annual_overview", []string{"year", "liturgical_year", "lectionary_cycle", "seasons", "key_dates"},
		"lectionary_cycle", []string{"year", "sunday", "weekday"},
		"season", []string{"name", "slug", "start_date", "end_date"},
		"key_date", []string{"date", "name", "post_slug"},
	),
	contextMeta: always,
}

const yearCost = 3

func yearParameter(r *req) int {
	first, _ := YearBounds(r.c.Param("year"))
	return first.Year()
}

func cycleFor(year int) *rb.Map {
	sunday := map[int]string{0: "C", 1: "A", 2: "B"}[year%3]
	weekday := "I"
	if year%2 == 0 {
		weekday = "II"
	}
	return rb.M("sunday", sunday, "weekday", weekday)
}

func (r *req) overview(year int) *liturgical.YearOverview {
	pb := r.context().pb
	bc, err := store.CelebrationsForBook(r.c.Ctx, pb)
	must(err)
	return liturgical.NewYearOverview(year, pb.Code, bc, true)
}

func (r *req) serializeSeasons(seasons []any) []any {
	out := make([]any, len(seasons))
	for i, s := range seasons {
		m := s.(*rb.Map).Dup()
		m.Set("name", liturgical.TranslateSeason(rb.ToS(m.Get("name")), r.context().language()))
		out[i] = m
	}
	return out
}

func (r *req) trimSeasons(v any) []any { return applyList(r.fields(), "season", v) }

func (r *req) trimKeyDates(v any) *rb.Map {
	out := rb.NewMap()
	if m, ok := v.(*rb.Map); ok {
		m.Each(func(k string, d any) {
			dm, _ := d.(*rb.Map)
			out.Set(k, r.fields().Apply("key_date", dm))
		})
	}
	return out
}

func yearsShow(r *req) {
	year := yearParameter(r)
	key := r.payloadKey("year_overview", strconv.Itoa(year))
	if !r.staleResource(yearCost, key) {
		return
	}
	overview := r.cached(key, ttlCalendarYear, func() any {
		o := r.overview(year)
		return rb.M("year", year, "liturgical_year", o.LiturgicalYear(), "lectionary_cycle", cycleFor(year),
			"seasons", r.serializeSeasons(o.Seasons()), "key_dates", o.KeyDates())
	}).(*rb.Map)
	trimmed := r.fields().Apply("annual_overview", overview)
	if trimmed.Has("seasons") {
		trimmed = trimmed.Dup()
		trimmed.Set("seasons", r.trimSeasons(overview.Get("seasons")))
	}
	if trimmed.Has("key_dates") {
		trimmed = trimmed.Dup()
		trimmed.Set("key_dates", r.trimKeyDates(overview.Get("key_dates")))
	}
	r.renderResource(trimmed, nil, nil, yearCost, datedMaxAge)
}

func yearsSeasons(r *req) {
	year := yearParameter(r)
	key := r.payloadKey("year_seasons", strconv.Itoa(year))
	if !r.staleResource(-1, key) {
		return
	}
	payload := r.cached(key, ttlCalendarYear, func() any {
		return rb.M("year", year, "seasons", r.serializeSeasons(r.overview(year).Seasons()))
	}).(*rb.Map)
	payload = payload.Merge(rb.M("seasons", r.trimSeasons(payload.Get("seasons"))))
	r.renderResource(payload, rb.M("year", year), nil, -1, datedMaxAge)
}

func yearsKeyDates(r *req) {
	year := yearParameter(r)
	key := r.payloadKey("year_key_dates", strconv.Itoa(year))
	if !r.staleResource(-1, key) {
		return
	}
	payload := r.cached(key, ttlCalendarYear, func() any {
		return rb.M("year", year, "key_dates", r.overview(year).KeyDates())
	}).(*rb.Map)
	payload = payload.Merge(rb.M("key_dates", r.trimKeyDates(payload.Get("key_dates"))))
	r.renderResource(payload, rb.M("year", year), nil, -1, datedMaxAge)
}

func yearsLectionaryCycle(r *req) {
	year := yearParameter(r)
	key := r.payloadKey("year_lectionary_cycle", strconv.Itoa(year))
	if !r.staleResource(-1, key) {
		return
	}
	data := rb.M("year", year).Merge(cycleFor(year))
	r.renderResource(r.fields().Apply("lectionary_cycle", data), rb.M("year", year), nil, -1, datedMaxAge)
}
