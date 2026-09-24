// Package explain ports Liturgical::ExplanationService and
// LiturgicalExplanationSerializer (the decision trail of a liturgical day).
package explain

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/i18n"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Service ports Liturgical::ExplanationService.
type Service struct {
	Date           civil.Date
	Book           *store.PrayerBook
	Translation    string
	ReadingType    string
	Locale         string
	ServiceType    string
	ServiceVariant string
	PsalmTable     string
	rules          *liturgical.RuleSet
}

func invalid(msg, code string) { web.Raise("InvalidPreference", msg, code) }

// New validates and normalizes the inputs (raising InvalidPreference).
func New(date civil.Date, pb *store.PrayerBook, translation, readingType, locale, serviceType, serviceVariant, psalmTable string) *Service {
	s := &Service{Date: date, Book: pb, Translation: translation, rules: liturgical.RulesFor(pb.Code)}
	s.ReadingType = s.rules.NormalizeReadingType(readingType)
	if locale == "" {
		locale = pb.Language
	}
	s.Locale = normalizeLocale(locale)
	switch {
	case rb.BlankString(serviceType) || serviceType == "eucharist":
		s.ServiceType = ""
	case serviceType == "morning_prayer" || serviceType == "evening_prayer":
		s.ServiceType = serviceType
	default:
		invalid("Tipo de serviço '"+serviceType+"' não é suportado.", "UNSUPPORTED_EXPLANATION_SERVICE_TYPE")
	}
	s.ServiceVariant = s.normalizeServiceVariant(serviceVariant)
	s.PsalmTable = s.normalizePsalmTable(psalmTable)
	return s
}

func normalizeLocale(locale string) string {
	value := langs.Normalize(locale)
	el := langs.ExplanationLocaleFor(value)
	for _, l := range langs.ExplanationLocales {
		if l == el && el != "" {
			return el
		}
	}
	invalid("Locale '"+locale+"' não é suportado para a explicação litúrgica.", "UNSUPPORTED_EXPLANATION_LOCALE")
	return ""
}

func (s *Service) caps() *books.Capabilities { return books.For(s.Book.Code, s.Book.Features) }

func (s *Service) normalizeServiceVariant(v string) string {
	if s.rules.WeekdayTableLectionary() {
		if rb.BlankString(v) || v == "standard" {
			return ""
		}
		if v == "ascension_alternative" {
			return v
		}
		invalid("Variante de leituras '"+v+"' não é suportada pelo Common Worship.", "INVALID_COMMON_WORSHIP_READING_VARIANT")
	}
	if len(s.caps().AvailableLectionaryVariants()) == 0 {
		return ""
	}
	return s.caps().NormalizeLectionaryServiceVariant(v)
}

func (s *Service) normalizePsalmTable(v string) string {
	if s.rules.WeekdayTableLectionary() {
		if rb.BlankString(v) || v == "appointed" {
			return "appointed"
		}
		if v == "course" {
			return v
		}
		invalid("Tabela de salmos '"+v+"' não é suportada pelo Common Worship.", "INVALID_COMMON_WORSHIP_PSALM_TABLE")
	}
	if rb.BlankString(v) {
		return ""
	}
	if v == "appointed" || v == "monthly" {
		return v
	}
	invalid("Ciclo de salmos '"+v+"' não é suportado por este Livro de Oração.", "INVALID_PSALM_CYCLE")
	return ""
}

// Result ports ExplanationService::Result.
type Result struct {
	Context     *liturgical.DayContext
	Resolution  *reading.Resolution
	SeasonRange liturgical.SeasonRange
	Locale      string
	SundayName  string
}

// Call ports #call.
func (s *Service) Call(ctx context.Context) *Result {
	cal := liturgical.NewCalendar(s.Date.Year(), s.Book.Code)
	dc := cal.ContextFor(s.Date)
	res := reading.For(ctx, s.Date, reading.Options{
		PrayerBookCode: s.Book.Code, Calendar: cal, DayContext: dc, Translation: s.Translation,
		ReadingType: s.ReadingType, ServiceType: s.ServiceType, ServiceVariant: s.ServiceVariant,
		PsalmTable: s.PsalmTable, LoadContent: true,
	}).Resolve()
	var rng liturgical.SeasonRange
	if dc.BookSeason == "" {
		rng = cal.SeasonRangeFor(s.Date)
	} else {
		rng = liturgical.NewBookSeasonResolver(dc.Movable).RangeFor(s.Date)
	}
	return &Result{Context: dc, Resolution: res, SeasonRange: rng, Locale: s.Locale, SundayName: cal.SundayName(s.Date)}
}

// --- serializer ------------------------------------------------------------------

type serializer struct {
	r     *Result
	c     *liturgical.DayContext
	p     *reading.Provenance
	sel   *reading.Selection
	rules *liturgical.RuleSet
}

// Data ports LiturgicalExplanationSerializer#as_json[:data].
func Data(r *Result) *rb.Map {
	sel := r.Resolution.Selection
	if sel == nil {
		sel = &reading.Selection{}
	}
	s := &serializer{r: r, c: r.Context, p: r.Resolution.Provenance, sel: sel, rules: liturgical.RulesFor(r.Context.PrayerBookCode)}
	readings := s.readings()
	return rb.M(
		"date", s.c.Date.ISO(),
		"prayer_book_code", s.c.PrayerBookCode,
		"locale", r.Locale,
		"presentation", s.dayPresentation(),
		"calendar", s.calendar(),
		"celebration", s.celebration(),
		"color", s.color(),
		"transfers", s.transfers(),
		"reading_guide", s.readingGuide(),
		"readings", readings,
		"partial", rb.M("partial", len(readings) == 0, "missing", missing(readings)),
	)
}

func missing(readings []any) []any {
	if len(readings) == 0 {
		return []any{"readings"}
	}
	return []any{}
}

func (s *serializer) t(key string, opts i18n.Opts) string {
	return i18n.TBang(s.locale(), key, opts)
}

func (s *serializer) locale() string {
	if l := langs.ExplanationLocaleFor(s.r.Locale); l != "" {
		return l
	}
	return "pt-BR"
}

func (s *serializer) presentation(key string, opts i18n.Opts) *rb.Map {
	return rb.M("title", s.t("liturgical.presentation."+key+".title", opts), "summary", s.t("liturgical.presentation."+key+".summary", opts))
}

func (s *serializer) readingPresentation(key string) *rb.Map {
	return rb.M("title", s.t("reading.presentation."+key+".title", nil), "summary", s.t("reading.presentation."+key+".summary", nil))
}

func (s *serializer) enumLabel(key, value string) any {
	if rb.BlankString(value) {
		return nil
	}
	k := "liturgical.enums." + key + "." + value
	if !i18n.Exists(s.locale(), k) {
		return nil
	}
	return s.t(k, nil)
}

func (s *serializer) liturgicalReason(code string) *rb.Map {
	if rb.BlankString(code) {
		return nil
	}
	return rb.M("code", code, "message", s.t("liturgical.explanations."+code, nil))
}

func (s *serializer) readingReason(code string) *rb.Map {
	return rb.M("code", code, "message", s.t("reading.explanations.rules."+code, nil))
}

func (s *serializer) seasonLanguage() string {
	if v := langs.TranslatorLanguageFor(s.r.Locale); v != "" {
		return v
	}
	return "pt-BR"
}

func isBookSeason(v string) bool {
	for _, x := range liturgical.BookSeasonsAll {
		if x == v {
			return true
		}
	}
	return false
}

func (s *serializer) localizedSeason(v string) string {
	lang := s.seasonLanguage()
	if isBookSeason(v) {
		return liturgical.TranslateBookSeason(v, lang)
	}
	if !liturgical.TranslatedLanguage(lang) {
		return v
	}
	return liturgical.TranslateSeason(v, lang)
}

func (s *serializer) daySeason() string {
	if s.c.BookSeason != "" {
		return s.c.BookSeason
	}
	return s.c.Season
}

func (s *serializer) localizedDate(d civil.Date) string {
	t := time.Date(d.Year(), time.Month(d.Month()), d.Day(), 0, 0, 0, 0, time.UTC)
	return i18n.LocalizeDate(s.locale(), t, "liturgical")
}

func (s *serializer) calendar() *rb.Map {
	m := rb.M("season", s.localizedSeason(s.daySeason()))
	if s.c.BookSeason != "" {
		k := "liturgical.season_meanings." + s.c.BookSeason
		if i18n.Exists(s.locale(), k) {
			m.Set("season_meaning", s.t(k, nil))
		}
	}
	rng := s.r.SeasonRange
	days := rng.End.Sub(s.c.Date)
	m.Set("season_range", rb.M(
		"start_date", rng.Start.ISO(), "end_date", rng.End.ISO(), "days_remaining", days,
		"presentation", s.presentation("season_range", i18n.Opts{
			"season": s.localizedSeason(rng.Name), "start_date": s.localizedDate(rng.Start),
			"end_date": s.localizedDate(rng.End), "count": days,
		}),
	))
	m.Set("cycle", s.c.Cycle)
	if s.c.HasWeek {
		m.Set("week", s.c.Week)
	}
	if s.c.HasProper {
		m.Set("proper", s.c.Proper)
	}
	return m
}

func (s *serializer) dayPresentation() *rb.Map {
	var name any
	if s.r.SundayName != "" {
		name = s.r.SundayName
		if liturgical.TranslatedLanguage(s.seasonLanguage()) {
			sn := s.r.SundayName
			if t := liturgical.TranslateSundayName(&sn, s.seasonLanguage()); t != nil {
				name = *t
			}
		}
	} else if s.c.Primary != nil {
		name = s.c.Primary.Name
	} else {
		name = s.localizedSeason(s.daySeason())
	}
	return s.presentation("day", i18n.Opts{"celebration": name, "season": s.localizedSeason(s.daySeason()), "cycle": s.c.Cycle})
}

func (s *serializer) occurrence(o *liturgical.Occurrence) *rb.Map {
	if o == nil {
		return nil
	}
	m := rb.M("key", o.Key, "name", o.Name)
	if o.Attrs != nil && o.Attrs.Type != "" {
		m.Set("type", o.Attrs.Type)
		if l := s.enumLabel("celebration_type", o.Attrs.Type); l != nil {
			m.Set("type_label", l)
		}
	}
	m.Set("date", o.Date.ISO())
	return m
}

func (s *serializer) celebration() *rb.Map {
	code := s.c.Precedence
	if code == "" {
		code = "no_celebration"
	}
	reason := s.liturgicalReason(code)
	candidates := []any{}
	for _, o := range s.c.Celebrations() {
		m := s.occurrence(o)
		selected := s.c.Primary != nil && o.Key == s.c.Primary.Key
		m.Set("selected", selected)
		if !selected && reason != nil {
			m.Set("discarded_reason", reason)
		}
		candidates = append(candidates, m)
	}
	key := "no_celebration"
	var celName any
	if s.c.Primary != nil {
		key = "celebration"
		celName = s.c.Primary.Name
	}
	var message any
	if reason != nil {
		message = reason.Get("message")
	}
	var selected any
	if s.c.Primary != nil {
		selected = s.occurrence(s.c.Primary)
	}
	return rb.M(
		"selected", selected, "candidates", candidates, "reason", reason,
		"presentation", s.presentation(key, i18n.Opts{"celebration": celName, "season": s.localizedSeason(s.c.Season), "reason": message}),
	)
}

func (s *serializer) colorLanguage() string { return s.seasonLanguage() }

func (s *serializer) color() *rb.Map {
	value := liturgical.TranslateColor(s.c.Color, s.colorLanguage())
	withAlt := liturgical.ColorWithAlternative(s.c.Color, s.colorLanguage())
	reason := s.liturgicalReason(s.c.ColorReason)
	meaning := s.colorMeaning()
	var parts []string
	if reason != nil {
		parts = append(parts, rb.ToS(reason.Get("message")))
	}
	if meaning != nil {
		parts = append(parts, rb.ToS(meaning.Get("message")))
	}
	m := rb.M("value", value)
	if code := liturgical.ColorCode(s.c.Color); code != "" {
		m.Set("code", code)
	}
	if s.c.ColorSource != "" {
		m.Set("source", s.c.ColorSource)
		if l := s.enumLabel("color_source", s.c.ColorSource); l != nil {
			m.Set("source_label", l)
		}
	}
	if reason != nil {
		m.Set("reason", reason)
	}
	if meaning != nil {
		m.Set("meaning", meaning)
	}
	m.Set("presentation", s.presentation("color", i18n.Opts{"color": withAlt, "reason": strings.Join(parts, " ")}))
	return m
}

func (s *serializer) colorMeaning() *rb.Map {
	code := s.c.ColorMeaning
	if code == "" && !rb.BlankString(s.c.Color) {
		source := s.c.ColorSource
		if source == "" {
			source = "season"
		}
		var attrs *liturgical.CelebrationAttrs
		if s.c.Primary != nil {
			attrs = s.c.Primary.Attrs
		}
		code = liturgical.ExplainColor(s.c.Date, s.c.Season, s.c.Color, source, attrs, s.c.Movable)
	}
	if rb.BlankString(code) {
		return nil
	}
	return rb.M("code", code, "message", s.t("liturgical.color_meanings."+code, nil))
}

func (s *serializer) transfers() []any {
	out := []any{}
	for _, tr := range s.c.Transfers {
		reason := s.liturgicalReason(tr.Reason)
		if reason == nil {
			rb.RaiseNoMethodOnNil("[]")
		}
		m := rb.M("from", tr.From.ISO(), "to", tr.To.ISO(), "reason", reason,
			"presentation", s.presentation("transfer", i18n.Opts{"from": s.localizedDate(tr.From), "to": s.localizedDate(tr.To), "reason": reason.Get("message")}))
		out = append(out, m)
	}
	return out
}

func (s *serializer) track() string {
	if s.p.Track == nil {
		return ""
	}
	return *s.p.Track
}

func (s *serializer) serviceType() string {
	if s.p.ServiceType == nil {
		return ""
	}
	return *s.p.ServiceType
}

func (s *serializer) selectedVariantLabel() any {
	v := ""
	if s.p.ServiceVariant != nil {
		v = strings.SplitN(*s.p.ServiceVariant, ":", 2)[0]
	}
	return s.enumLabel("lectionary_variant", v)
}

func (s *serializer) selectedVariant() any {
	if s.p.ServiceVariant == nil {
		return nil
	}
	code := strings.SplitN(*s.p.ServiceVariant, ":", 2)[0]
	if rb.BlankString(code) || s.selectedVariantLabel() == nil {
		return nil
	}
	return code
}

var slots = []string{"first_reading", "psalm", "psalm_alternative", "second_reading", "gospel"}

func alternativeReferences(p *reading.Passage) any {
	var refs []any
	if p.Alternative != nil {
		refs = append(refs, p.Alternative.Reference)
	}
	for _, a := range p.Alternatives {
		refs = append(refs, a.Reference)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func setOpt(m *rb.Map, k string, v any) {
	if v != nil {
		m.Set(k, v)
	}
}

func (s *serializer) readings() []any {
	out := []any{}
	st := s.serviceType()
	if st == "" {
		st = "eucharist"
	}
	for _, slot := range slots {
		p := s.sel.Slot(slot)
		if p == nil {
			continue
		}
		sp := s.p.ForSlot(slot)
		reason := s.readingReason(sp.Rule)
		m := rb.M("slot", slot, "reference", p.Reference)
		setOpt(m, "alternatives", alternativeReferences(p))
		m.Set("source", sp.Source)
		setOpt(m, "source_label", s.enumLabel("reading_source", sp.Source))
		if s.p.Track != nil {
			m.Set("track", *s.p.Track)
		}
		setOpt(m, "track_label", s.enumLabel("reading_track", s.track()))
		m.Set("cycle", s.p.Cycle)
		setOpt(m, "cycle_label", s.enumLabel("reading_cycle", s.p.Cycle))
		m.Set("service_type", st)
		setOpt(m, "service_type_label", s.enumLabel("service_type", st))
		setOpt(m, "lectionary_variant", s.selectedVariant())
		setOpt(m, "lectionary_variant_label", s.selectedVariantLabel())
		m.Set("reason", reason)
		if sp.Fallback {
			m.Set("fallback", rb.M("applied", true, "rule", sp.Rule))
			setOpt(m, "fallback_label", s.enumLabel("reading_fallback", sp.Rule))
		}
		m.Set("presentation", s.presentation("reading", i18n.Opts{
			"slot": s.t("liturgical.presentation.reading_slot."+slot, nil), "reference": p.Reference, "reason": reason.Get("message"),
		}))
		out = append(out, m)
	}
	return out
}

func (s *serializer) readingGuide() *rb.Map {
	m := rb.M("cycle", s.cycleGuide())
	setOpt(m, "track", s.trackGuide())
	m.Set("method", s.methodGuide())
	setOpt(m, "lectionary", s.lectionaryGuide())
	setOpt(m, "psalter", s.psalterGuide())
	setOpt(m, "common_worship", s.commonWorshipGuide())
	return m
}

var cwCycleRe = regexp.MustCompile(`\A[ABC]-[12]\z`)

func (s *serializer) cycleGuide() *rb.Map {
	code := s.p.Cycle
	key := "other"
	switch code {
	case "A", "B", "C", "odd", "even":
		key = strings.ToLower(code)
	}
	system := "weekday_biennial"
	if s.rules.WeekdayCycleScheme() == "paired_weekday_year" && cwCycleRe.MatchString(code) {
		system = "common_worship_weekday_course"
	} else if code == "A" || code == "B" || code == "C" {
		system = "sunday_triennial"
	}
	m := rb.M("code", code)
	m.Set("code_label", s.enumLabel("reading_cycle", code))
	m.Set("system", system)
	m.Set("presentation", s.readingPresentation("cycle."+key))
	return m
}

func (s *serializer) trackGuide() any {
	if rb.BlankString(s.track()) {
		return nil
	}
	return rb.M("code", s.track(), "code_label", s.enumLabel("reading_track", s.track()), "presentation", s.readingPresentation("track."+s.track()))
}

func (s *serializer) methodGuide() *rb.Map {
	return rb.M("code", s.p.Rule, "source", s.p.Source, "source_label", s.enumLabel("reading_source", s.p.Source),
		"presentation", s.readingPresentation("method."+s.p.Rule))
}

func (s *serializer) lectionaryGuide() any {
	code, label := s.selectedVariant(), s.selectedVariantLabel()
	if code == nil || label == nil {
		return nil
	}
	return rb.M("code", code, "label", label, "presentation", rb.M(
		"title", s.t("liturgical.presentation.lectionary.title", nil),
		"summary", s.t("liturgical.presentation.lectionary.summary", i18n.Opts{"lectionary": label}),
	))
}

func (s *serializer) psalterGuide() any {
	st := s.serviceType()
	if st != "morning_prayer" && st != "evening_prayer" {
		return nil
	}
	if s.sel.Psalm == nil || !s.rules.MonthlyPsalter() {
		return nil
	}
	pref := ""
	if s.p.PsalmTable != nil {
		pref = *s.p.PsalmTable
	}
	if pref != "appointed" && pref != "monthly" {
		return nil
	}
	sp := s.p.ForSlot("psalm")
	m := rb.M("preference", pref)
	setOpt(m, "preference_label", s.enumLabel("psalm_cycle", pref))
	m.Set("source", sp.Source)
	setOpt(m, "source_label", s.enumLabel("reading_source", sp.Source))
	if sp.Fallback {
		m.Set("fallback", rb.M("applied", true, "rule", sp.Rule))
		setOpt(m, "fallback_label", s.enumLabel("reading_fallback", sp.Rule))
	}
	m.Set("presentation", rb.M(
		"title", s.t("reading.presentation.psalter."+pref+".title", nil),
		"summary", s.t("reading.presentation.psalter."+pref+".summary", nil),
	))
	return m
}

func (s *serializer) commonWorshipGuide() any {
	if s.rules.ExplanationProfile() != "common_worship" {
		return nil
	}
	facts := s.cwFacts()
	if facts.Len() == 0 {
		return nil
	}
	m := rb.NewMap()
	if w, ok := facts.Get("weekday_readings").(*rb.Map); ok {
		wm := rb.NewMap()
		for _, k := range []struct{ key, enum string }{{"table", "common_worship_reading_table"}, {"cycle", "reading_cycle"}, {"weekday_year", "common_worship_weekday_year"}, {"boundary", "common_worship_reading_table"}} {
			if v := w.Get(k.key); v != nil {
				wm.Set(k.key, v)
				setOpt(wm, k.key+"_label", s.enumLabel(k.enum, rb.ToS(v)))
			}
		}
		m.Set("weekday_readings", wm)
	}
	if p, ok := facts.Get("psalms").(*rb.Map); ok {
		pm := rb.NewMap()
		for _, k := range []struct{ key, enum string }{{"table", "common_worship_psalm_table"}, {"period", "common_worship_psalm_period"}, {"preference", "common_worship_psalm_preference"}} {
			if v := p.Get(k.key); v != nil {
				pm.Set(k.key, v)
				setOpt(pm, k.key+"_label", s.enumLabel(k.enum, rb.ToS(v)))
			}
		}
		setOpt(pm, "reference", p.Get("reference"))
		m.Set("psalms", pm)
	}
	if rules, ok := facts.Get("rules").([]any); ok {
		out := []any{}
		for _, r := range rules {
			code := rb.ToS(r)
			rm := rb.M("code", code)
			setOpt(rm, "label", s.enumLabel("common_worship_rule", code))
			out = append(out, rm)
		}
		m.Set("rules", out)
	}
	if m.Len() == 0 {
		return nil
	}
	return m
}

var weekdayYearRe = regexp.MustCompile(`\A[ABC]-(1|2)\z`)

// cwFacts ports Reading::Cw2005EnExplanation#call.
func (s *serializer) cwFacts() *rb.Map {
	out := rb.NewMap()
	date := s.c.Date
	boundary := ""
	if wd := date.Weekday(); wd >= 1 && wd <= 6 {
		m := liturgical.EasterFor(date.Year(), liturgical.Gregorian).Dates
		firstLent, firstAdvent := m["first_sunday_in_lent"], m["first_sunday_of_advent"]
		if (date >= firstLent.Add(-34) && date < firstLent) || (date >= firstAdvent.Add(-27) && date < firstAdvent) {
			boundary = "seasonal_boundary"
		}
	}
	if s.p.Source == "common_worship_table_2" {
		w := rb.M("table", "table_2", "cycle", s.p.Cycle)
		if m := weekdayYearRe.FindStringSubmatch(s.p.Cycle); m != nil {
			w.Set("weekday_year", "year_"+m[1])
		}
		if boundary != "" {
			w.Set("boundary", boundary)
		}
		out.Set("weekday_readings", w)
	}
	if s.p.ForSlot("psalm").Source == "common_worship_psalm_table" {
		pref := "appointed"
		if s.p.PsalmTable != nil {
			pref = *s.p.PsalmTable
		}
		if prov := reading.CwPsalmProvision(date, s.serviceType(), pref); prov != nil {
			pm := rb.M("table", prov.Table, "period", prov.Period, "preference", pref)
			if s.sel.Psalm != nil {
				pm.Set("reference", s.sel.Psalm.Reference)
			}
			out.Set("psalms", pm)
		}
	}
	var codes []any
	if s.p.Rule == "common_worship_christmas_eve" {
		codes = append(codes, "christmas_eve_evening")
	}
	if s.p.ServiceVariant != nil && *s.p.ServiceVariant == "ascension_alternative" {
		codes = append(codes, "ascension_alternative")
	}
	if boundary != "" {
		codes = append(codes, "table_2_seasonal_boundary")
	}
	if len(codes) > 0 {
		out.Set("rules", codes)
	}
	return out
}
