package v2

import (
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/explain"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
)

// --- serializers ----------------------------------------------------------

var dayAttributes = []string{"date", "day_of_week", "liturgical_year", "season", "week_of_season", "proper_week",
	"sunday_after_pentecost", "sunday_name", "color", "is_sunday", "is_holy_day", "fast", "description", "celebration",
	"celebrations", "collect", "readings", "morning_readings", "evening_readings", "explanation"}

var readingAttributes = []string{"slot", "reference", "title", "alternatives", "text"}

var collectAttributes = []string{"title", "subtitle", "kind", "sunday_reference", "celebration_id", "text", "preface"}

var celebrationAttributes = []string{"id", "slug", "name", "type", "rank", "color", "description",
	"movable", "transferred", "original_date", "calculation_rule"}

// serializeCelebration ports CelebrationSerializer.call: the published
// attributes of a celebration hash, compacted, nil when none remain.
func serializeCelebration(v any) *rb.Map {
	m, ok := v.(*rb.Map)
	if !ok || m == nil || m.Len() == 0 {
		return nil
	}
	out := rb.NewMap()
	for _, k := range celebrationAttributes {
		if val, ok := m.Lookup(k); ok {
			if val = rb.Deref(val); val != nil {
				out.Set(k, val)
			}
		}
	}
	if out.Len() == 0 {
		return nil
	}
	return out
}

func serializeCelebrations(v any) []any {
	out := []any{}
	list, _ := v.([]any)
	for _, c := range list {
		if s := serializeCelebration(c); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// serializeDay ports DaySerializer#to_h.
func serializeDay(info *rb.Map, date civil.Date, language string, fast *liturgical.FastObservance) *rb.Map {
	out := rb.M(
		"date", date.ISO(),
		"liturgical_year", info.Get("liturgical_year"),
		"week_of_season", info.Get("week_of_season"),
		"proper_week", info.Get("proper_week"),
		"sunday_after_pentecost", info.Get("sunday_after_pentecost"),
		"is_sunday", info.Get("is_sunday"),
		"is_holy_day", info.Get("is_holy_day"),
		"day_of_week", liturgical.DayName(date, language),
	)
	var season any
	if name := rb.ToS(info.Get("liturgical_season")); !rb.BlankString(name) {
		s := rb.NewMap()
		if slug := info.Get("season_post_slug"); slug != nil {
			s.Set("slug", slug)
		}
		s.Set("name", liturgical.TranslateSeason(name, language))
		season = s
	}
	out.Set("season", season)
	var sundayName any
	if v := info.Get("sunday_name"); v != nil {
		s := rb.ToS(v)
		if t := liturgical.TranslateSundayName(&s, language); t != nil {
			sundayName = *t
		}
	}
	out.Set("sunday_name", sundayName)
	color := info.Get("color")
	if liturgical.InTranslatedLanguages(language) {
		color = liturgical.TranslateColor(rb.ToS(color), language)
	}
	out.Set("color", color)
	var descs []string
	for _, d := range info.Get("description").([]any) {
		descs = append(descs, rb.ToS(d))
	}
	translated := []any{}
	for _, d := range liturgical.TranslateDescriptions(descs, language) {
		translated = append(translated, d)
	}
	out.Set("description", translated)
	var fastValue any
	if rb.Truthy(info.Get("fast_day")) {
		fastValue = rb.M("observance", liturgical.PresentFastObservance(fast, language))
	}
	out.Set("fast", fastValue)
	var celebration any
	if s := serializeCelebration(info.Get("celebration")); s != nil {
		celebration = s
	}
	out.Set("celebration", celebration)
	return out
}

// serializeCollects ports CollectSerializer.list.
func serializeCollects(list []*rb.Map, withText bool) []any {
	out := []any{}
	for _, c := range list {
		m := rb.NewMap()
		for _, kv := range [][2]string{{"title", "title"}, {"subtitle", "subtitle"}, {"kind", "module_title"},
			{"sunday_reference", "sunday_reference"}, {"celebration_id", "celebration_id"}} {
			if v := rb.Deref(c.Get(kv[1])); v != nil {
				m.Set(kv[0], v)
			}
		}
		if withText {
			for _, k := range []string{"text", "preface"} {
				if v := rb.Deref(c.Get(k)); v != nil {
					m.Set(k, v)
				}
			}
		}
		out = append(out, m)
	}
	return out
}

type readingOptions struct {
	withText, withAlternatives bool
	textFormat                 string
}

var readingSlots = []string{"first_reading", "psalm", "psalm_alternative", "second_reading", "gospel"}

// serializeReadings ports ReadingSerializer.list.
func serializeReadings(sel *reading.Selection, o readingOptions) []any {
	out := []any{}
	if sel == nil {
		return out
	}
	for _, slot := range readingSlots {
		if p := sel.Slot(slot); p != nil {
			out = append(out, serializeReading(p, slot, o))
		}
	}
	return out
}

func serializeReading(p *reading.Passage, slot string, o readingOptions) *rb.Map {
	m := rb.M("slot", slot, "reference", p.Reference)
	if p.Title != nil && !rb.BlankString(*p.Title) {
		m.Set("title", *p.Title)
	}
	if o.withAlternatives {
		alts := p.Alternatives
		if len(alts) == 0 && p.Alternative != nil {
			alts = []*reading.Passage{p.Alternative}
		}
		refs := []any{}
		for _, a := range alts {
			refs = append(refs, a.Reference)
		}
		m.Set("alternatives", refs)
	}
	if o.withText {
		m.Set("text", readingText(p.Content, o.textFormat))
	}
	return m
}

// readingText ports ReadingSerializer#text (nil when nothing was loaded).
func readingText(content *rb.Map, format string) any {
	if content == nil || content.Len() == 0 {
		return nil
	}
	verses, _ := content.Get("verses").([]any)
	if len(verses) == 0 {
		return nil
	}
	out := rb.M("format", format, "bible", content.Get("translation"))
	switch format {
	case textPlain, textMarkdown:
		parts := make([]string, len(verses))
		for i, v := range verses {
			vm, _ := v.(*rb.Map)
			if format == textPlain {
				parts[i] = rb.ToS(vm.Get("text"))
			} else {
				parts[i] = "**" + rb.ToS(vm.Get("number")) + "** " + rb.ToS(vm.Get("text"))
			}
		}
		out.Set("content", strings.Join(parts, " "))
	default:
		list := make([]any, len(verses))
		for i, v := range verses {
			vm, _ := v.(*rb.Map)
			list[i] = rb.M("number", vm.Get("number"), "text", vm.Get("text"))
		}
		out.Set("verses", list)
	}
	return out
}

// --- ReadingsResource -------------------------------------------------------

var readingServices = []string{"daily", "eucharist", "morning", "evening"}

var readingServiceTypes = map[string]string{"daily": "", "eucharist": "eucharist", "morning": "morning_prayer", "evening": "evening_prayer"}

type readingsResource struct {
	r        *req
	includes *IncludeSet
}

func (rr *readingsResource) options() readingOptions {
	return readingOptions{
		withText:         rr.includes.Has("readings.text"),
		withAlternatives: rr.includes.Has("readings.alternatives"),
		textFormat:       rr.r.context().textFormat,
	}
}

func (rr *readingsResource) resolve(date civil.Date, name string) ([]any, any) {
	x := rr.r.context()
	cal := rr.r.calendarFor(date)
	psalmTable := ""
	if name == "morning" || name == "evening" {
		psalmTable = prefs.PsalmCycle(x.values, name)
	}
	res := reading.For(rr.r.c.Ctx, date, reading.Options{
		PrayerBookCode: x.code(), Calendar: cal, DayContext: cal.ContextFor(date),
		Translation: x.pref("bible_version"), ReadingType: x.pref("reading_type"),
		PsalmTranslation: x.pref("psalm_translation"), ServiceType: readingServiceTypes[name],
		ServiceVariant: x.lectionaryVariant(), PsalmTable: psalmTable, LoadContent: rr.includes.Has("readings.text"),
	})
	var cycle any
	if res.Cycle != "" {
		cycle = res.Cycle
	}
	return serializeReadings(res.Selection(), rr.options()), cycle
}

func (rr *readingsResource) forService(date civil.Date, name string) []any {
	list, _ := rr.resolve(date, name)
	return list
}

// call ports ReadingsResource#call.
func (rr *readingsResource) call(date civil.Date, service any) *rb.Map {
	if toS(service) == "all" {
		var cycle any
		services := rb.NewMap()
		for _, name := range readingServices {
			list, c := rr.resolve(date, name)
			if cycle == nil {
				cycle = c
			}
			services.Set(name, list)
		}
		return rb.M("date", date.ISO(), "service", "all", "cycle", cycle, "services", services)
	}
	name := "daily"
	if rb.Present(service) {
		name = toS(service)
	}
	if _, ok := readingServiceTypes[name]; !ok {
		domainError("InvalidParameter", "Serviço inválido: '"+name+"'.", "INVALID_SERVICE",
			rb.M("allowed", strs(append(append([]string{}, readingServices...), "all"))))
	}
	list, cycle := rr.resolve(date, name)
	return rb.M("date", date.ISO(), "service", name, "cycle", cycle, "readings", list)
}

// --- LiturgicalExplanationResource ---------------------------------------------

// explanationServiceName ports LiturgicalExplanationResource.service_name.
func explanationServiceName(value any) string {
	name := "daily"
	if rb.Present(value) {
		name = toS(value)
	}
	if _, ok := readingServiceTypes[name]; ok {
		return name
	}
	domainError("InvalidParameter", "Serviço inválido: '"+name+"'.", "INVALID_SERVICE", rb.M("allowed", strs(readingServices)))
	return ""
}

func (r *req) explanation(date civil.Date, service string) any {
	x := r.context()
	psalmTable := ""
	if service == "morning" || service == "evening" {
		psalmTable = prefs.PsalmCycle(x.values, service, "weekday_psalm_table")
	}
	svc := explain.New(date, x.pb, x.pref("bible_version"), x.pref("reading_type"), x.language(),
		readingServiceTypes[service], x.lectionaryVariant(), psalmTable)
	return explain.Data(svc.Call(r.c.Ctx))
}

// --- DayResource ------------------------------------------------------------

func (r *req) collectLanguageStyle() string {
	x := r.context()
	return liturgical.RulesFor(x.code()).CalendarCollectLanguageStyle(x.pref("daily_office_rite"))
}

func (r *req) collects(date civil.Date, withText bool) []any {
	cal := r.calendarFor(date)
	list := collects.New(r.c.Ctx, date, collects.Options{
		PrayerBookCode: r.context().code(), Calendar: cal, LanguageStyle: r.collectLanguageStyle(),
		IncludeFixedOfficeCollect: false,
	}).FindCollects()
	return serializeCollects(list, withText)
}

// day ports DayResource#call.
func (r *req) day(date civil.Date, explanationService string) *rb.Map {
	cal := r.calendarFor(date)
	info := cal.DayInfo(date)
	incl := r.includes()
	out := serializeDay(info, date, r.context().language(), cal.ContextFor(date).FastObservance)
	if incl.Has("celebrations") {
		out.Set("celebrations", serializeCelebrations(info.Get("celebrations")))
	}
	if incl.Has("explanation") {
		out.Set("explanation", r.explanation(date, explanationService))
	}
	if incl.Has("collect") {
		out.Set("collect", r.collects(date, incl.Has("collect.text")))
	}
	rr := &readingsResource{r: r, includes: incl}
	for _, kv := range [][2]string{{"readings", "daily"}, {"readings.morning", "morning"}, {"readings.evening", "evening"}} {
		if incl.Has(kv[0]) {
			key := map[string]string{"daily": "readings", "morning": "morning_readings", "evening": "evening_readings"}[kv[1]]
			out.Set(key, rr.forService(date, kv[1]))
		}
	}
	return out
}

var _ = strconv.Itoa
