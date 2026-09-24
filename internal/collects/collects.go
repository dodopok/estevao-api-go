// Package collects ports CollectService: the collect(s) of a liturgical day.
package collects

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rx"
	"github.com/dodopok/estevao-api-go/internal/store"
)

type Date = liturgical.Date

// Options mirrors CollectService.new keyword arguments.
type Options struct {
	PrayerBookCode            string
	Calendar                  *liturgical.Calendar
	DayContext                *liturgical.DayContext
	OfficeType                string // default "morning"
	LanguageStyle             string // "" == nil
	IncludeFixedOfficeCollect bool
}

// Service ports CollectService.
type Service struct {
	ctx           context.Context
	date          Date
	code          string
	rules         *liturgical.RuleSet
	cal           *liturgical.Calendar
	dayCtx        *liturgical.DayContext
	officeType    string
	languageStyle string
	includeFixed  bool

	pbLoaded   bool
	pb         *store.PrayerBook
	resolvers  map[int]*liturgical.CelebrationResolver
	names      map[int64]*string
	namesKnown map[int64]bool
}

// New builds the service.
func New(ctx context.Context, date Date, o Options) *Service {
	code := o.PrayerBookCode
	if code == "" {
		code = books.DefaultCode
	}
	s := &Service{ctx: ctx, date: date, code: code, rules: liturgical.RulesFor(code), includeFixed: o.IncludeFixedOfficeCollect}
	if o.Calendar != nil {
		s.cal = o.Calendar.ForDate(date)
	} else {
		s.cal = liturgical.NewCalendar(date.Year(), code)
	}
	s.dayCtx = o.DayContext
	if s.dayCtx == nil {
		s.dayCtx = s.cal.ContextFor(date)
	}
	s.officeType = o.OfficeType
	if s.officeType == "" {
		s.officeType = "morning"
	}
	if !rb.BlankString(o.LanguageStyle) {
		s.languageStyle = o.LanguageStyle
	}
	return s
}

// Collect is one formatted collect hash.
type Collect = *rb.Map

// FindCollects ports #find_collects: nil when there is none.
func (s *Service) FindCollects() []*rb.Map {
	out := s.findCollectsUncached()
	if len(out) == 0 {
		return nil
	}
	return out
}

// FindCollectsValue is FindCollects as a JSON value (nil or an array).
func (s *Service) FindCollectsValue() any {
	list := s.FindCollects()
	if list == nil {
		return nil
	}
	out := make([]any, len(list))
	for i, m := range list {
		out[i] = m
	}
	return out
}

func (s *Service) prayerBook() *store.PrayerBook {
	if !s.pbLoaded {
		s.pbLoaded = true
		pb, err := store.PrayerBookByCode(s.ctx, s.code)
		if err != nil {
			panic(err)
		}
		s.pb = pb
	}
	return s.pb
}

func (s *Service) translatorLanguage() string {
	lang := ""
	if pb := s.prayerBook(); pb != nil {
		lang = pb.Language
	}
	if v := langs.TranslatorLanguageFor(lang); v != "" {
		return v
	}
	return "pt-BR"
}

func (s *Service) celebrations() []*rb.Map {
	var out []*rb.Map
	if s.dayCtx.Primary != nil {
		out = append(out, s.dayCtx.Primary.LegacyH())
	}
	for _, c := range s.dayCtx.Commemorations {
		if c != nil {
			out = append(out, c.LegacyH())
		}
	}
	return out
}

func typeOf(c *rb.Map) string { return rb.ToS(c.Get("type")) }

func (s *Service) findCollectsUncached() []*rb.Map {
	var all []*rb.Map
	cels := s.celebrations()
	language := s.translatorLanguage()
	label := map[string]string{"en": "Collect of the Day", "es": "Colecta del día", "cy": "Colect y Dydd"}[language]
	if label == "" {
		label = "Coleta do Dia"
	}
	for _, c := range cels {
		switch typeOf(c) {
		case "principal_feast", "major_holy_day", "festival":
			if s.rules.SuppressAscensionOn(s.date) && rb.ToS(c.Get("calculation_rule")) == "ascension" {
				continue
			}
			all = append(all, s.collectsForCelebrationInfo(c, label, false)...)
		}
	}
	if s.date.Weekday() == 0 {
		all = append(all, s.collectsForSundayWithProper(s.date, s.cal, label, optionalString(s.cal.SundayName(s.date)))...)
	} else {
		seasonal := s.findSpecialWeekCollect(label)
		if len(seasonal) == 0 && !s.rules.SuppressSeasonalCollectFallback(s.date, s.cal.Easter.Dates) {
			seasonal = s.findBySeason(label)
		}
		all = append(all, seasonal...)
	}
	all = append(all, s.repeatedCollectsFromBookRules(label)...)
	for _, c := range cels {
		switch typeOf(c) {
		case "lesser_feast", "commemoration":
			all = append(all, s.collectsForCelebrationInfo(c, label, true)...)
		}
	}
	if s.includeFixed {
		all = append(all, s.findFixedOfficeCollect()...)
	}
	// uniq { |c| c[:text] }
	seen := map[string]bool{}
	nilSeen := false
	var out []*rb.Map
	for _, c := range all {
		t, ok := c.Get("text").(string)
		if !ok {
			if nilSeen {
				continue
			}
			nilSeen = true
			out = append(out, c)
			continue
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, c)
	}
	return out
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) resolverForYear(year int) *liturgical.CelebrationResolver {
	if s.resolvers == nil {
		s.resolvers = map[int]*liturgical.CelebrationResolver{}
	}
	if r, ok := s.resolvers[year]; ok {
		return r
	}
	r := liturgical.NewCalendar(year, s.code).CelebrationResolver()
	s.resolvers[year] = r
	return r
}

func (s *Service) collectsForCelebrationInfo(info *rb.Map, moduleTitle string, joinSubtitle bool) []*rb.Map {
	name := info.Get("name")
	var descParts []string
	for _, k := range []string{"description", "description_year"} {
		if v := info.Get(k); v != nil && rb.Present(v) {
			descParts = append(descParts, rb.ToS(v))
		}
	}
	desc := strings.Join(descParts, ", ")
	title := name
	var subtitle any = desc
	if joinSubtitle && !rb.BlankString(desc) {
		title = fmt.Sprintf("%s, %s", rb.ToS(name), desc)
		subtitle = nil
	}
	id, hasID := celebrationID(info)
	var celebration *liturgical.Celebration
	if hasID {
		celebration = store.CelebrationByID(s.ctx, s.prayerBook(), id)
	}
	var records []*store.Collect
	if hasID {
		records = s.collectsForCelebrationID(id)
	}
	if len(records) > 0 {
		return s.format(records, moduleTitle, title, subtitle, celebration)
	}
	return s.findCommonCollectFor(info, celebration, moduleTitle, title, subtitle)
}

func celebrationID(info *rb.Map) (int64, bool) {
	switch v := info.Get("id").(type) {
	case int64:
		return v, v != 0
	case int:
		return int64(v), v != 0
	}
	return 0, false
}

func (s *Service) collectsForSundayWithProper(target Date, cal *liturgical.Calendar, moduleTitle string, title *string) []*rb.Map {
	language := s.translatorLanguage()
	if liturgical.TranslatedLanguage(language) && title != nil && !rb.BlankString(*title) {
		title = liturgical.TranslateSundayName(title, language)
	}
	var titleVal any
	if title != nil {
		titleVal = *title
	}
	for _, ref := range reading.MapSundayWithAliases(target, cal) {
		if records := s.collectsForSunday(ref); len(records) > 0 {
			return s.format(records, moduleTitle, titleVal, nil, nil)
		}
	}
	if n, ok := cal.ProperNumber(target); ok {
		if records := s.collectsForSunday(fmt.Sprintf("proper_%d", n)); len(records) > 0 {
			return s.format(records, moduleTitle, titleVal, nil, nil)
		}
	}
	var records []*store.Collect
	if c := s.resolverForYear(target.Year()).ResolveForDate(target); c != nil {
		records = s.collectsForCelebrationID(c.ID)
	}
	return s.format(records, moduleTitle, titleVal, nil, nil)
}

func (s *Service) collectsForCelebrationID(id int64) []*store.Collect {
	return s.styleFiltered(store.CollectsFor(s.ctx, s.prayerBook()).ByCelebration[id])
}

func (s *Service) collectsForSunday(ref string) []*store.Collect {
	if ref == "" {
		return nil
	}
	return s.styleFiltered(store.CollectsFor(s.ctx, s.prayerBook()).BySunday[ref])
}

func (s *Service) styleFiltered(records []*store.Collect) []*store.Collect {
	if s.languageStyle == "" {
		return records
	}
	var out []*store.Collect
	for _, r := range records {
		if r.LanguageStyle == nil || rb.BlankString(*r.LanguageStyle) || *r.LanguageStyle == s.languageStyle {
			out = append(out, r)
		}
	}
	return out
}

func (s *Service) findCommonCollectFor(info *rb.Map, celebration *liturgical.Celebration, moduleTitle string, title, subtitle any) []*rb.Map {
	t := typeOf(info)
	lesserFallback := s.rules.CommonLesserFeastCollect() && t == "lesser_feast"
	if !rb.Present(info.Get("description")) && !lesserFallback {
		return nil
	}
	if t == "commemoration" && !s.rules.CommonCollectForCommemoration() {
		return nil
	}
	if celebration != nil && celebration.PersonType == "event" {
		return nil
	}
	var parts []string
	for _, k := range []string{"description", "name", "description_year"} {
		if v := info.Get(k); v != nil {
			parts = append(parts, rb.ToS(v))
		}
	}
	desc := strings.ToLower(strings.Join(parts, " "))
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(desc, w) {
				return true
			}
		}
		return false
	}
	primary, secondary := "common_saints", ""
	switch {
	case has("martyr", "mártir"):
		primary = "common_martyrs"
	case has("missionary", "missionário"):
		primary = "common_missionaries"
	case has("pastor", "bispo", "bishop"):
		primary = "common_pastors"
	case has("teacher", "professor", "doutor", "doctor"):
		primary = "common_theologians"
	case has("monk", "nun", "monge", "monja", "religious", "religioso", "religiosa"):
		primary = "common_religious"
	case has("ecumenist", "ecumenista"):
		primary = "common_ecumenists"
	case has("reformer", "reformador"):
		primary, secondary = "common_reformers", "common_theologians"
	case has("renewer", "renovador"):
		primary = "common_renewers"
	}
	records := s.collectsForSunday(primary)
	if len(records) == 0 && secondary != "" {
		records = s.collectsForSunday(secondary)
	}
	if len(records) == 0 && primary != "common_saints" {
		records = s.collectsForSunday("common_saints")
	}
	return s.format(records, moduleTitle, title, subtitle, celebration)
}

var weekdayKeys = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

func (s *Service) findSpecialWeekCollect(moduleTitle string) []*rb.Map {
	candidates, title := s.specialWeekCandidates()
	if len(candidates) == 0 {
		return nil
	}
	var records []*store.Collect
	for _, ref := range candidates {
		if r := s.collectsForSunday(ref); len(r) > 0 {
			records = r
			break
		}
	}
	if len(records) == 0 {
		return nil
	}
	return s.format(records, moduleTitle, title, nil, nil)
}

func (s *Service) specialWeekCandidates() ([]string, string) {
	easter := s.cal.Easter.EasterDate
	weekday := weekdayKeys[s.date.Weekday()]
	if refs := s.rules.SpecialWeekCollectReferences(s.date, easter); refs != nil {
		return refs, s.specialWeekTitle("holy_week")
	}
	switch {
	case s.date > easter && s.date <= easter.Add(6):
		return []string{"easter_" + weekday, weekday + "_easter_week"}, s.specialWeekTitle("easter")
	case s.date == easter.Add(-6) || s.date == easter.Add(-5) || s.date == easter.Add(-4):
		return []string{weekday + "_holy_week", weekday + "_of_holy_week"}, s.specialWeekTitle("holy_week")
	}
	return nil, ""
}

func (s *Service) dayLabel(language string) string {
	switch language {
	case "en":
		return liturgical.DayNamesEN[s.date.Weekday()]
	case "es":
		return liturgical.DayNamesES[s.date.Weekday()]
	case "cy":
		return liturgical.DayNamesCY[s.date.Weekday()]
	}
	return liturgical.DayNamesPT[s.date.Weekday()]
}

func (s *Service) specialWeekTitle(kind string) string {
	language := s.translatorLanguage()
	label := s.dayLabel(language)
	if kind == "easter" {
		switch language {
		case "en":
			return label + " of Easter Week"
		case "es":
			return label + " de la Semana de Pascua"
		case "cy":
			return label + " yn Wythnos y Pasg"
		}
		return label + " da Semana da Páscoa"
	}
	switch language {
	case "en":
		return label + " in Holy Week"
	case "es":
		return label + " de la Semana Santa"
	case "cy":
		return label + " yn yr Wythnos Fawr"
	}
	return label + " da Semana Santa"
}

func (s *Service) repeatedCollectsFromBookRules(moduleTitle string) []*rb.Map {
	movable := s.cal.Easter.Dates
	refs := s.rules.RepeatedCollectReferences(s.date, movable)
	if len(refs) == 0 {
		return nil
	}
	var records []*store.Collect
	for _, ref := range refs {
		if r := s.collectsForSunday(ref); len(r) > 0 {
			records = append(records, r...)
			continue
		}
		if c := s.repeatedCollectCelebration(ref); c != nil {
			records = append(records, s.collectsForCelebrationID(c.ID)...)
		}
	}
	return s.format(records, moduleTitle, nil, nil, nil)
}

func (s *Service) repeatedCollectCelebration(reference string) *liturgical.Celebration {
	q, ok := s.rules.RepeatedCollectCelebrationQuery(reference, s.date.Year())
	if !ok {
		return nil
	}
	if q.FixedDate != nil {
		return store.BookCelebrationWhere(s.ctx, s.prayerBook(), q.FixedDate[0], q.FixedDate[1], "")
	}
	if q.CalculationRule != "" {
		return store.BookCelebrationWhere(s.ctx, s.prayerBook(), 0, 0, q.CalculationRule)
	}
	return nil
}

func (s *Service) findBySeason(moduleTitle string) []*rb.Map {
	if s.date.Weekday() == 0 {
		return nil
	}
	lastSunday := s.date.Add(-s.date.Weekday())
	cal := s.cal
	if lastSunday.Year() != s.date.Year() {
		cal = liturgical.NewCalendar(lastSunday.Year(), s.code)
	}
	sundayName := optionalString(cal.SundayName(lastSunday))
	language := s.translatorLanguage()
	if liturgical.TranslatedLanguage(language) {
		sundayName = liturgical.TranslateSundayName(sundayName, language)
	}
	name := ""
	if sundayName != nil {
		name = *sundayName
	}
	var title string
	switch language {
	case "en":
		title = s.dayLabel("en") + " after the " + name
	case "es":
		title = s.dayLabel("es") + " después de " + name
	case "cy":
		title = s.dayLabel("cy") + " ar ôl " + name
	default:
		title = liturgical.DayNamesPT[s.date.Weekday()] + " após o " + name
	}
	if language == "en" {
		title = strings.ReplaceAll(title, "1st Sunday after Christmas", "First Sunday of Christmas")
		title = strings.ReplaceAll(title, "2nd Sunday after Christmas", "Second Sunday of Christmas")
	}
	return s.collectsForSundayWithProper(lastSunday, cal, moduleTitle, &title)
}

func (s *Service) findFixedOfficeCollect() []*rb.Map {
	weekday := strings.ToLower(liturgical.DayNamesEN[s.date.Weekday()])
	slug := fmt.Sprintf("%s_collect_%s", s.officeType, weekday)
	if s.languageStyle == "traditional" && s.rules.TraditionalCollectsInLiturgicalTexts() {
		slug = "trad_" + slug
	}
	text := store.FindLiturgicalText(s.ctx, s.prayerBook(), slug)
	if text == nil {
		return nil
	}
	var moduleTitle any
	if text.Title != nil {
		moduleTitle = *text.Title
	} else {
		moduleTitle = rb.Humanize(text.Slug)
	}
	return []*rb.Map{rb.M("text", text.Content, "module_title", moduleTitle, "title", s.dayLabel(s.translatorLanguage()), "slug", text.Slug)}
}

// TraditionalCollectSlug ports CollectService.traditional_collect_slug ("" == nil).
func TraditionalCollectSlug(sundayReference, celebrationName string) string {
	if !rb.BlankString(sundayReference) {
		return "trad_collect_" + sundayReference
	}
	if rb.BlankString(celebrationName) {
		return ""
	}
	return "trad_collect_cel_" + rb.ParameterizeSep(celebrationName, "_")
}

func (s *Service) celebrationName(id *int64) *string {
	if id == nil {
		return nil
	}
	if s.namesKnown == nil {
		s.namesKnown = map[int64]bool{}
		s.names = map[int64]*string{}
	}
	if !s.namesKnown[*id] {
		s.namesKnown[*id] = true
		s.names[*id] = store.CelebrationNameByID(s.ctx, *id)
	}
	return s.names[*id]
}

func (s *Service) collectText(c *store.Collect) *string {
	text := c.Text
	if s.languageStyle == "traditional" && s.rules.TraditionalCollectsInLiturgicalTexts() {
		ref, name := "", ""
		if c.SundayRef != nil {
			ref = *c.SundayRef
		}
		if n := s.celebrationName(c.CelebrationID); n != nil {
			name = *n
		}
		if slug := TraditionalCollectSlug(ref, name); slug != "" {
			if t, ok := store.LiturgicalTextsFor(s.ctx, s.prayerBook())[slug]; ok {
				content := t.Content
				text = &content
			}
		}
	}
	return text
}

func (s *Service) format(records []*store.Collect, moduleTitle any, title, subtitle any, celebration *liturgical.Celebration) []*rb.Map {
	if len(records) == 0 {
		return nil
	}
	out := make([]*rb.Map, 0, len(records))
	for _, c := range records {
		textPtr := s.collectText(c)
		var text any
		if textPtr != nil {
			text = *textPtr
		}
		if celebration != nil && celebration.PersonType != "event" {
			if textPtr == nil {
				rb.RaiseNoMethodOnNil("match?")
			}
			if saintPlaceholder.MatchString(*textPtr) {
				text = SubstituteGrammaticalPatterns(*textPtr, celebration, s.translatorLanguage())
			}
		}
		m := rb.NewMap()
		setNonNil(m, "text", text)
		setNonNil(m, "preface", strOrNil(c.Preface))
		setNonNil(m, "module_title", moduleTitle)
		setNonNil(m, "title", title)
		setNonNil(m, "subtitle", subtitle)
		setNonNil(m, "sunday_reference", strOrNil(c.SundayRef))
		if c.CelebrationID != nil {
			m.Set("celebration_id", *c.CelebrationID)
		}
		setNonNil(m, "celebration_name", strOrNil(s.celebrationName(c.CelebrationID)))
		out = append(out, m)
	}
	return out
}

func strOrNil(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func setNonNil(m *rb.Map, k string, v any) {
	if v != nil {
		m.Set(k, v)
	}
}

// --- grammatical substitution ---------------------------------------------------

const saintPattern = `(?:\(\s*N\.?\s*\)|\[\s*N\.?\s*\]|\bN\.(?!\w)|\bN\b)`

var (
	saintPlaceholder = rx.MustCompile(saintPattern, "i")
	embedded         = `(?i:` + saintPattern + `)`
)

func possessivePronoun(c *liturgical.Celebration) string {
	switch {
	case c.PersonType == "singular" && c.Gender == "masculine":
		return "teu"
	case c.PersonType == "singular" && c.Gender == "feminine":
		return "tua"
	case c.PersonType == "plural" && (c.Gender == "masculine" || c.Gender == "mixed"):
		return "teus"
	case c.PersonType == "plural" && c.Gender == "feminine":
		return "tuas"
	}
	return "teu"
}

func servantForm(c *liturgical.Celebration) string {
	switch {
	case c.PersonType == "singular" && c.Gender == "masculine":
		return "servo"
	case c.PersonType == "singular" && c.Gender == "feminine":
		return "serva"
	case c.PersonType == "plural" && (c.Gender == "masculine" || c.Gender == "mixed"):
		return "servos"
	case c.PersonType == "plural" && c.Gender == "feminine":
		return "servas"
	}
	return "servo"
}

func servantPhrase(c *liturgical.Celebration, name string) string {
	if c.PersonType == "event" {
		return ""
	}
	return possessivePronoun(c) + " " + servantForm(c) + " " + name
}

func servantPhraseEN(c *liturgical.Celebration, name string) string {
	if c.PersonType == "event" {
		return ""
	}
	form := "servant"
	if c.PersonType == "plural" {
		form = "servants"
	}
	return "your " + form + " " + name
}

// SubstituteGrammaticalPatterns ports substitute_grammatical_patterns.
func SubstituteGrammaticalPatterns(text string, c *liturgical.Celebration, language string) string {
	if v := langs.TranslatorLanguageFor(language); v != "" {
		language = v
	}
	name := c.Name
	constant := func(v string) func(*rx.Match) string { return func(*rx.Match) string { return v } }
	switch language {
	case "en":
		text = rx.MustCompile(`your servant\s+`+embedded, "i").GsubFunc(text, constant(servantPhraseEN(c, name)))
		text = rx.MustCompile(`your servants\s+`+embedded, "i").GsubFunc(text, constant(servantPhraseEN(c, name)))
	case "es":
		text = saintPlaceholder.GsubFunc(text, constant(name))
	default:
		text = rx.MustCompile(`teu\(tua\) fiel servo\(a\)\s+`+embedded).GsubFunc(text, constant("fiel "+servantPhrase(c, name)))
		text = rx.MustCompile(`teu\(tua\) servo\(a\)\s+`+embedded).GsubFunc(text, constant(servantPhrase(c, name)))
		for _, p := range []string{`teu servo\s+`, `tua serva\s+`, `teus servos\s+`, `tuas servas\s+`} {
			text = rx.MustCompile(p+embedded).GsubFunc(text, constant(servantPhrase(c, name)))
		}
	}
	text = saintPlaceholder.GsubFunc(text, constant(name))
	text = resolveOptionalBishopRole(text, c, language)
	if strings.HasPrefix(language, "pt") {
		text = resolvePortugueseRoleForms(text, c)
		text = resolvePortugueseInclusiveForms(text, c)
	} else if language == "en" {
		text = resolveEnglishInclusiveForms(text, c)
	}
	return text
}

var upperStart = rx.MustCompile(`\A[[:upper:]]`)

// capitalize mirrors String#capitalize.
func capitalize(s string) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return s
	}
	out := string(unicode.ToUpper(rs[0]))
	return out + strings.ToLower(string(rs[1:]))
}

func preserveFormCase(original, replacement string) string {
	if upperStart.MatchString(original) {
		return capitalize(replacement)
	}
	return replacement
}

func resolveEnglishInclusiveForms(text string, c *liturgical.Celebration) string {
	possessive, object, subject := "their", "them", "they"
	switch {
	case c.PersonType == "plural":
	case c.Gender == "feminine":
		possessive, object, subject = "her", "her", "she"
	case c.Gender == "masculine":
		possessive, object, subject = "his", "him", "he"
	}
	for _, p := range []struct{ re, rep string }{
		{`\b(?:his|her)\/(?:her|his)\b`, possessive},
		{`\b(?:him|her)\/(?:her|him)\b`, object},
		{`\b(?:he|she)\/(?:she|he)\b`, subject},
	} {
		rep := p.rep
		text = rx.MustCompile(p.re, "i").GsubFunc(text, func(m *rx.Match) string { return preserveFormCase(m.String(), rep) })
	}
	return text
}

var bishopRe = rx.MustCompile(`\b(?:arce)?bispos?\b|\b(?:arch)?bishops?\b|\b(?:arce)?obispo?s?\b`, "i")

func celebrationDescription(c *liturgical.Celebration) string {
	parts := []string{c.Name}
	if c.Description != nil {
		parts = append(parts, *c.Description)
	}
	if c.DescriptionYear != nil {
		parts = append(parts, *c.DescriptionYear)
	}
	return strings.Join(parts, " ")
}

func resolveOptionalBishopRole(text string, c *liturgical.Celebration, language string) string {
	include := bishopRe.MatchString(celebrationDescription(c))
	if language == "en" {
		phrase := ""
		if include {
			phrase = "Bishop and "
		}
		return rx.MustCompile(`\[\s*Bishop and\s*\]\s*`, "i").GsubFunc(text, func(*rx.Match) string { return phrase })
	}
	if include {
		return rx.MustCompile(`(?:\[\s*Bispo e\s*\]|\(\s*Bispo e\s*\))\s*`, "i").GsubFunc(text, func(*rx.Match) string { return "Bispo e " })
	}
	return rx.MustCompile(`(?:\[\s*Bispo e\s*\]|\(\s*Bispo e\s*\)|\bBispo\(a\)\s+e\s*)`, "i").GsubFunc(text, func(*rx.Match) string { return "" })
}

var roleForms = map[string][4]string{ // singular m, singular f, plural m, plural f
	"bispo":      {"bispo", "bispa", "bispos", "bispas"},
	"pastor":     {"pastor", "pastora", "pastores", "pastoras"},
	"presbítero": {"presbítero", "presbítera", "presbíteros", "presbíteras"},
	"diácono":    {"diácono", "diácona", "diáconos", "diáconas"},
}

func formIndex(c *liturgical.Celebration, feminine bool) int {
	i := 0
	if c.PersonType == "plural" {
		i = 2
	}
	if feminine {
		i++
	}
	return i
}

func resolvePortugueseRoleForms(text string, c *liturgical.Celebration) string {
	re := rx.MustCompile(`\b(bispo|pastor|presb[ií]tero|di[aá]cono)\(a\)`, "i")
	return re.GsubFunc(text, func(m *rx.Match) string {
		match := m.String()
		base := strings.ToLower(strings.TrimSuffix(match, "(a)"))
		switch base {
		case "presbitero":
			base = "presbítero"
		case "diacono":
			base = "diácono"
		}
		forms, ok := roleForms[base]
		if !ok {
			rb.RaiseKeyError(rb.Inspect(base))
		}
		form := forms[formIndex(c, c.Gender == "feminine")]
		return preserveFormCase(match, form)
	})
}

var inclusiveForms = []struct {
	re    string
	forms [4]string
}{
	{`\bteu\(tua\)`, [4]string{"teu", "tua", "teus", "tuas"}},
	{`\bservo\(a\)`, [4]string{"servo", "serva", "servos", "servas"}},
	{`\bbispo\(a\)`, [4]string{"bispo", "bispa", "bispos", "bispas"}},
	{`\bpastor\(a\)`, [4]string{"pastor", "pastora", "pastores", "pastoras"}},
	{`\bsanto\(a\)`, [4]string{"santo", "santa", "santos", "santas"}},
	{`\bo\(a\)`, [4]string{"o", "a", "os", "as"}},
	{`\bpelo\(a\)`, [4]string{"pelo", "pela", "pelos", "pelas"}},
	{`\bao\(à\)|\bà\(ao\)`, [4]string{"ao", "à", "aos", "às"}},
	{`\bpelo\(s\)`, [4]string{"pelo", "pela", "pelos", "pelas"}},
	{`\bteu\(s\)`, [4]string{"teu", "tua", "teus", "tuas"}},
	{`\bservo\(s\)`, [4]string{"servo", "serva", "servos", "servas"}},
	{`\bseja\(m\)`, [4]string{"seja", "seja", "sejam", "sejam"}},
	{`\btenha\(m\)`, [4]string{"tenha", "tenha", "tenham", "tenham"}},
	{`\brestaurado\(s\)`, [4]string{"restaurado", "restaurada", "restaurados", "restauradas"}},
	{`\bqual\s+\(is\)`, [4]string{"qual", "qual", "quais", "quais"}},
}

func resolvePortugueseInclusiveForms(text string, c *liturgical.Celebration) string {
	feminine := c.Gender == "feminine"
	for _, f := range inclusiveForms {
		form := f.forms[formIndex(c, feminine)]
		text = rx.MustCompile(f.re, "i").GsubFunc(text, func(m *rx.Match) string { return preserveFormCase(m.String(), form) })
	}
	return text
}

// FindCollectsUncached returns the uncached list (possibly empty), as the
// Rails private method does; used by the golden tests.
func (s *Service) FindCollectsUncached() []*rb.Map { return s.findCollectsUncached() }
