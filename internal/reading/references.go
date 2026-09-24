package reading

import (
	"fmt"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

type Date = civil.Date

// --- SundayReferenceMapper -------------------------------------------------

var specialSundays = map[string]string{
	"Cristo Rei do Universo":               "christ_the_king",
	"Domingo da Páscoa":                    "easter_sunday",
	"Pentecostes":                          "pentecost",
	"Santíssima Trindade":                  "trinity_sunday",
	"Domingo de Ramos":                     "palm_sunday",
	"Batismo de nosso Senhor Jesus Cristo": "baptism_of_the_lord",
	"Septuagésima":                         "septuagesima",
	"Sexagésima":                           "sexagesima",
	"Quinquagésima":                        "quinquagesima",
	"Domingo de Rogação":                   "rogation_sunday",
	"Domingo após a Ascensão":              "sunday_after_ascension",
}

var christmasSundays = map[string]string{
	"1º Domingo após Natal": "1st_sunday_after_christmas",
	"2º Domingo após Natal": "2nd_sunday_after_christmas",
}

var seasonTranslations = map[string]string{
	"Advento": "advent", "Natal": "christmas", "Epifania": "epiphany", "Quaresma": "lent",
	"Páscoa": "easter", "a Páscoa": "easter", "Tempo Comum": "ordinary_time", "Trindade": "trinity",
	"a Trindade": "trinity",
}

var referenceAliases = map[string][]string{
	"pentecost":              {"pentecost_day", "pentecost", "whitsunday", "pentecost_sunday"},
	"easter_sunday":          {"easter_sunday", "easter_day", "easter"},
	"trinity_sunday":         {"trinity_sunday", "trinity"},
	"baptism_of_the_lord":    {"baptism_of_the_lord", "baptism_of_christ", "1st_sunday_after_epiphany"},
	"christ_the_king":        {"christ_the_king", "sunday_before_advent"},
	"7th_sunday_of_easter":   {"7th_sunday_of_easter", "sunday_after_ascension"},
	"palm_sunday":            {"palm_sunday_palms", "palm_sunday_word", "palm_sunday"},
	"sunday_after_ascension": {"sunday_after_ascension", "7th_sunday_of_easter"},
	"rogation_sunday":        {"rogation_sunday", "5th_sunday_after_easter"},
}

var sundayNameRe = rx.MustCompile(`(\d+)º Domingo (do|no|da|na|após) (.+)`)
var numberedEpiphanyRe = rx.MustCompile(`\A\d+(?:st|nd|rd|th)_sunday_after_epiphany\z`)

// MapSunday ports SundayReferenceMapper.map ("" == nil).
func MapSunday(date Date, cal *liturgical.Calendar) string {
	cal = cal.ForDate(date)
	movable := cal.Easter.Dates
	if cal.Rules.UsesSundayBeforeAdventReference() {
		if ctk, ok := movable["christ_the_king"]; ok && date == ctk {
			return "sunday_before_advent"
		}
	}
	if late := cal.Rules.LateTrinityReadingReference(date, movable); late != "" {
		return late
	}
	name := cal.SundayName(date)
	if name == "" {
		return ""
	}
	if v, ok := specialSundays[name]; ok {
		return v
	}
	if v, ok := christmasSundays[name]; ok {
		return v
	}
	if isLastSundayAfterEpiphany(date, cal) {
		return "last_sunday_after_epiphany"
	}
	return parseNumberedSunday(name)
}

// MapSundayWithAliases ports SundayReferenceMapper.map_with_aliases.
func MapSundayWithAliases(date Date, cal *liturgical.Calendar) []string {
	cal = cal.ForDate(date)
	primary := MapSunday(date, cal)
	if primary == "" {
		return []string{}
	}
	aliases := append([]string(nil), referenceAliases[primary]...)
	aliases = append(aliases, cal.Rules.WeeklyReferenceAliases(primary)...)
	aliases = append(aliases, cal.Rules.DatedSundayReferenceAliases(date, cal.Easter.Dates)...)
	if primary == "last_sunday_after_epiphany" {
		if n := numberedEpiphanyReference(cal, date); n != "" {
			aliases = append(aliases, n)
		}
	}
	return uniq(append([]string{primary}, aliases...))
}

func numberedEpiphanyReference(cal *liturgical.Calendar, date Date) string {
	name := cal.SundayName(date)
	if rb.BlankString(name) {
		return ""
	}
	ref := parseNumberedSunday(name)
	if numberedEpiphanyRe.MatchString(ref) {
		return ref
	}
	return ""
}

func isLastSundayAfterEpiphany(date Date, cal *liturgical.Calendar) bool {
	if date.Weekday() != 0 {
		return false
	}
	if cal.SeasonFor(date) != "Epifania" {
		return false
	}
	last, ok := cal.Easter.Dates["last_sunday_after_epiphany"]
	return ok && date == last
}

func parseNumberedSunday(name string) string {
	m := sundayNameRe.Find(name)
	if m == nil {
		return rb.ParameterizeSep(name, "_")
	}
	number := rb.StringToI(m.G(1))
	prep, season := m.G(2), m.G(3)
	ordinal := toOrdinal(number)
	translated, ok := seasonTranslations[season]
	if !ok {
		translated = rb.ParameterizeSep(season, "_")
	}
	if prep == "após" || season == "Epifania" {
		return fmt.Sprintf("%s_sunday_after_%s", ordinal, translated)
	}
	p := "of"
	if prep == "no" || prep == "na" {
		p = "in"
	}
	return fmt.Sprintf("%s_sunday_%s_%s", ordinal, p, translated)
}

func toOrdinal(n int) string {
	if r := n % 100; r >= 11 && r <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// compactUniq drops the "" (nil) entries then de-duplicates.
func compactUniq(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return uniq(out)
}

// --- Reading::ReferenceBuilder ---------------------------------------------

// ReferenceBuilder is the per-book seam of reference construction.
type ReferenceBuilder interface {
	FixedDateReferences() []string
	CelebrationReferences(c *liturgical.Celebration) []string
	WeeklyReferences() []string
	SundayReferences() []string
	DailyCourseReference() string
}

var weekdayNames = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

var specialDateReferences = map[[2]int][]string{
	{12, 25}: {"christmas_day", "christmas"},
	{12, 24}: {"christmas_eve"},
	{1, 1}:   {"holy_name"},
	{1, 6}:   {"epiphany"},
	{3, 25}:  {"annunciation"},
	{11, 1}:  {"all_saints"},
	{8, 6}:   {"transfiguration"},
	{2, 2}:   {"presentation_of_the_lord", "presentation"},
}

type baseBuilder struct {
	date    Date
	cal     *liturgical.Calendar
	variant string
	rules   *liturgical.RuleSet
}

func newBaseBuilder(date Date, cal *liturgical.Calendar, variant string, rules *liturgical.RuleSet) *baseBuilder {
	cal = cal.ForDate(date)
	if rules == nil {
		rules = liturgical.RulesFor(cal.Code)
	}
	return &baseBuilder{date: date, cal: cal, variant: variant, rules: rules}
}

func (b *baseBuilder) movable() liturgical.Movable { return b.cal.Easter.Dates }

func (b *baseBuilder) FixedDateReferences() []string {
	var refs []string
	m, d := b.date.Month(), b.date.Day()
	if b.rules.WeekdayTableLectionary() {
		refs = append(refs, b.rules.FixedDateReadingReferences(m, d)...)
	}
	if b.rules.TraditionalLectionary() {
		refs = append(refs, b.rules.FixedDateReadingReferences(m, d)...)
	}
	refs = append(refs, b.rules.CivilDateReference(b.date))
	refs = append(refs, b.fixedDateSpecialReferences(m, d)...)
	return compactUniq(refs)
}

func (b *baseBuilder) fixedDateSpecialReferences(m, d int) []string {
	if b.rules.TraditionalLectionary() {
		return b.rules.FixedDateReadingReferences(m, d)
	}
	return append([]string(nil), specialDateReferences[[2]int{m, d}]...)
}

func (b *baseBuilder) CelebrationReferences(c *liturgical.Celebration) []string {
	var refs []string
	if c.CalculationRule != nil && !rb.BlankString(*c.CalculationRule) {
		refs = append(refs, liturgical.CalculationRuleToDateReferences[*c.CalculationRule]...)
	}
	if c.FixedMonth != nil && c.FixedDay != nil {
		m, d := *c.FixedMonth, *c.FixedDay
		if b.rules.WeekdayTableLectionary() {
			refs = append(refs, b.rules.FixedDateReadingReferences(m, d)...)
		}
		if b.rules.TraditionalLectionary() {
			refs = append(refs, b.rules.FixedDateReadingReferences(m, d)...)
		}
		refs = append(refs, b.rules.CivilDateReferenceFor(m, d))
		refs = append(refs, b.fixedDateSpecialReferences(m, d)...)
	}
	return compactUniq(refs)
}

// WeeklyReferences may contain "" (nil) entries, like the Ruby array.
func (b *baseBuilder) WeeklyReferences() []string {
	if b.rules.WeekdayTableLectionary() {
		return b.cwWeeklyReferences()
	}
	weekday := weekdayNames[b.date.Weekday()]
	refs := []string{b.rules.CivilDateReference(b.date)}
	sunday := b.date.Add(-b.date.Weekday())
	if n, ok := b.cal.ProperNumberFor(sunday, b.cal.SeasonFor(b.date)); ok {
		refs = append(refs, fmt.Sprintf("proper_%d_%s", n, weekday))
	}
	if sref := MapSunday(sunday, b.cal); sref != "" {
		list := append([]string{sref}, b.rules.WeeklyReferenceAliases(sref)...)
		for _, r := range list {
			refs = append(refs, r+"_"+weekday)
		}
	}
	refs = append(append([]string(nil), b.rules.WeeklyReadingReferences(b.date, b.movable())...), refs...)
	return append(refs, b.specialWeekReferences(weekday)...)
}

func (b *baseBuilder) SundayReferences() []string { return MapSundayWithAliases(b.date, b.cal) }

func (b *baseBuilder) DailyCourseReference() string { return b.rules.CivilDateReference(b.date) }

func (b *baseBuilder) specialWeekReferences(weekday string) []string {
	var refs []string
	week, hasWeek := b.cal.WeekNumber(b.date)
	switch b.cal.SeasonFor(b.date) {
	case "Advento":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_advent_"+weekday)
		}
	case "Natal":
		refs = append(refs, "week_of_christmas_"+weekday, "first_sunday_after_christmas_"+weekday)
		if b.date.Month() == 1 {
			refs = append(refs, "baptism_of_christ_"+weekday, "week_of_epiphany_"+weekday)
		}
	case "Epifania":
		if hasWeek && week >= 1 {
			refs = append(refs, fmt.Sprintf("ordinary_time_%d_%s", week+1, weekday),
				fmt.Sprintf("ordinary_time_%d_%s", week, weekday),
				liturgical.Ordinalize(week)+"_sunday_after_epiphany_"+weekday)
		}
		refs = append(refs, "week_of_epiphany_"+weekday, "baptism_of_christ_"+weekday, "last_sunday_after_epiphany_"+weekday)
	case "Quaresma":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_lent_"+weekday)
		}
		refs = append(refs, "holy_week_"+weekday, "holy_"+weekday)
	case "Páscoa":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_easter_"+weekday)
		}
		refs = append(refs, "week_of_pentecost_"+weekday, "trinity_sunday_"+weekday)
		m := b.movable()
		if b.date > m["ascension"] && b.date < m["pentecost"] {
			refs = append(refs, "ascension_"+weekday)
		}
	case "Tempo Comum":
		pentecost := b.movable()["pentecost"]
		trinity := pentecost.Add(7)
		if b.date > pentecost && b.date < trinity {
			refs = append(refs, "week_of_pentecost_"+weekday, "trinity_sunday_"+weekday)
		}
	}
	return refs
}

var cwWeekdayCodes = map[int]string{1: "m", 2: "t", 3: "w", 4: "th", 5: "f", 6: "s"}

func (b *baseBuilder) cwWeeklyReferences() []string {
	if b.date.Weekday() == 0 {
		return []string{b.rules.CivilDateReference(b.date)}
	}
	weekday := cwWeekdayCodes[b.date.Weekday()]
	regular := b.cwWeekdayReference(weekday)
	alternative := b.cwAscensionAlternativeReference()
	refs := b.cwFixedOfficeReferences()
	if alternative != "" {
		refs = append(refs, alternative, b.rules.CivilDateReference(b.date))
	} else {
		refs = append(refs, regular, b.rules.CivilDateReference(b.date))
	}
	return compactUniq(refs)
}

func (b *baseBuilder) cwWeekdayReference(weekday string) string {
	d := b.date
	m := b.movable()
	year := d.Year()
	firstAdvent := m["first_sunday_of_advent"]
	if d > firstAdvent && d < civil.MustNew(year, 12, 25) {
		week := d.Sub(firstAdvent)/7 + 1
		if week >= 1 && week <= 4 {
			return fmt.Sprintf("advent_%d_%s", week, weekday)
		}
	}
	if d.Month() == 12 && d.Day() >= 29 && d.Day() <= 31 {
		return fmt.Sprintf("december_%d", d.Day())
	}
	if d.Month() == 1 && d.Day() >= 2 && d.Day() <= 5 {
		return fmt.Sprintf("january_%d", d.Day())
	}
	epiphany := civil.MustNew(year, 1, 6)
	satAfter := epiphany.Add(mod(6-epiphany.Weekday(), 7))
	jan12 := civil.MustNew(year, 1, 12)
	limit := satAfter
	if jan12 < limit {
		limit = jan12
	}
	if d >= epiphany.Add(1) && d <= limit {
		return fmt.Sprintf("january_%d", d.Day())
	}
	var baptism Date
	if epiphany.Weekday() == 0 {
		baptism = epiphany.Add(7)
	} else {
		baptism = epiphany.Add(7 - epiphany.Weekday())
	}
	epiphanyWeekMonday := baptism.Add(1)
	if d >= epiphanyWeekMonday && d < civil.MustNew(year, 2, 2) {
		week := d.Sub(epiphanyWeekMonday)/7 + 1
		if week >= 1 && week <= 4 {
			return fmt.Sprintf("epiphany_%d_%s", week, weekday)
		}
	}
	firstLent := m["first_sunday_in_lent"]
	beforeLentMonday := firstLent.Add(-34)
	if d >= beforeLentMonday && d < firstLent {
		week := 5 - d.Sub(beforeLentMonday)/7
		if week >= 1 && week <= 5 {
			return fmt.Sprintf("before_lent_%d_%s", week, weekday)
		}
	}
	lentMonday := firstLent.Add(1)
	if d >= lentMonday && d < m["palm_sunday"] {
		week := d.Sub(lentMonday)/7 + 1
		if week >= 1 && week <= 5 {
			return fmt.Sprintf("lent_%d_%s", week, weekday)
		}
	}
	easterMonday := m["easter"].Add(1)
	if d >= easterMonday && d < m["pentecost"] {
		week := d.Sub(easterMonday)/7 + 1
		if week >= 1 && week <= 7 {
			if week == 1 {
				return "easter_" + weekday
			}
			return fmt.Sprintf("easter_%d_%s", week, weekday)
		}
	}
	if d > m["pentecost"] && d <= m["pentecost"].Add(6) {
		return "pentecost_" + weekday
	}
	beforeAdvent4Monday := firstAdvent.Add(-27)
	if d >= beforeAdvent4Monday && d < firstAdvent {
		week := 4 - d.Sub(beforeAdvent4Monday)/7
		if week >= 1 && week <= 4 {
			return fmt.Sprintf("before_advent_%d_%s", week, weekday)
		}
	}
	trinityMonday := m["trinity_sunday"].Add(1)
	if d >= trinityMonday && d < beforeAdvent4Monday {
		week := d.Sub(trinityMonday) / 7
		if week >= 0 && week <= 22 {
			if week == 0 {
				return "trinity_" + weekday
			}
			return fmt.Sprintf("trinity_%d_%s", week, weekday)
		}
	}
	return ""
}

func (b *baseBuilder) cwAscensionAlternativeReference() string {
	if b.variant != "ascension_alternative" {
		return ""
	}
	m := b.movable()
	d := b.date
	if !(d > m["ascension"] && d < m["pentecost"] && d.Weekday() != 0) {
		return ""
	}
	offset := d.Sub(m["ascension"])
	index := offset
	if offset > 3 {
		index = offset - 1
	}
	if index >= 1 && index <= 8 {
		return fmt.Sprintf("ascension_alternative_%d", index)
	}
	return ""
}

func (b *baseBuilder) cwFixedOfficeReferences() []string {
	m := b.movable()
	var refs []string
	if b.date.Month() == 12 && b.date.Day() == 24 {
		refs = append(refs, "christmas_eve")
	}
	// Ruby builds a Hash keyed by date: a later key with the same date
	// overwrites the value but keeps the first position; the dates are
	// distinct, so a linear scan is equivalent.
	for _, kv := range []struct{ key, ref string }{
		{"holy_monday", "monday_of_holy_week"}, {"holy_tuesday", "tuesday_of_holy_week"},
		{"holy_wednesday", "wednesday_of_holy_week"}, {"maundy_thursday", "maundy_thursday"},
		{"good_friday", "good_friday"}, {"holy_saturday", "easter_eve"},
	} {
		if d, ok := m[kv.key]; ok && d == b.date {
			refs = append(refs, kv.ref)
			break
		}
	}
	return refs
}

func mod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// --- Reading::Loc1662EnReferenceBuilder ------------------------------------

type loc1662Builder struct{ *baseBuilder }

func revised1922Fixed(m, d int) []string {
	v := loc1662Revised1922Fixed.Get(fmt.Sprintf("%d,%d", m, d))
	return anyStrings(v)
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		out = append(out, rb.ToS(x))
	}
	return out
}

func (b *loc1662Builder) FixedDateReferences() []string {
	return uniq(append(b.baseBuilder.FixedDateReferences(), revised1922Fixed(b.date.Month(), b.date.Day())...))
}

func (b *loc1662Builder) CelebrationReferences(c *liturgical.Celebration) []string {
	refs := b.baseBuilder.CelebrationReferences(c)
	if c.FixedMonth != nil && c.FixedDay != nil {
		refs = append(refs, revised1922Fixed(*c.FixedMonth, *c.FixedDay)...)
	}
	return compactUniq(refs)
}

func (b *loc1662Builder) WeeklyReferences() []string {
	refs := b.baseBuilder.WeeklyReferences()
	if b.variant != "revised_1922" {
		return refs
	}
	return compactUniq(append(refs, b.revised1922WeekReferences()...))
}

func (b *loc1662Builder) SundayReferences() []string {
	refs := b.baseBuilder.SundayReferences()
	if b.variant != "revised_1922" {
		return refs
	}
	return compactUniq(append(b.revised1922SundayReferences(), refs...))
}

func (b *loc1662Builder) sundayBeforeAdvent() Date {
	return b.movable()["first_sunday_of_advent"].Add(-7)
}

func (b *loc1662Builder) revised1922SundayReferences() []string {
	var refs []string
	if MapSunday(b.date, b.cal) == "1st_sunday_of_advent" {
		refs = append(refs, "first_sunday_of_advent")
	}
	if b.date == b.sundayBeforeAdvent() {
		refs = append(refs, "sunday_before_advent")
	}
	return refs
}

func (b *loc1662Builder) revised1922WeekReferences() []string {
	weekday := weekdayNames[b.date.Weekday()]
	var refs []string
	m := b.movable()
	if b.date > m["pentecost"] && b.date < m["pentecost"].Add(7) {
		refs = append(refs, "whitsun_"+weekday)
	}
	epiphany := civil.MustNew(b.date.Year(), 1, 6)
	first := epiphany.Add(mod(7-epiphany.Weekday(), 7))
	if first == epiphany {
		first = first.Add(7)
	}
	if b.date > epiphany && b.date < first {
		refs = append(refs, "week_of_epiphany_"+weekday)
	}
	if b.date > b.sundayBeforeAdvent() && b.date < m["first_sunday_of_advent"] {
		refs = append(refs, "sunday_before_advent_"+weekday)
	}
	sunday := b.date.Add(-b.date.Weekday())
	if MapSunday(sunday, b.cal) == "1st_sunday_of_advent" {
		refs = append(refs, "first_sunday_of_advent_"+weekday)
	}
	return refs
}

// --- Reading::Loc1962EnReferenceBuilder ------------------------------------

type loc1962Builder struct{ *baseBuilder }

func (b *loc1962Builder) FixedDateReferences() []string {
	refs := b.baseBuilder.FixedDateReferences()
	refs = append(refs, liturgical.Loc1962FixedDateReferences(b.date.Month(), b.date.Day())...)
	return compactUniq(refs)
}

func (b *loc1962Builder) CelebrationReferences(c *liturgical.Celebration) []string {
	refs := b.baseBuilder.CelebrationReferences(c)
	if c.FixedMonth != nil && c.FixedDay != nil {
		refs = append(refs, liturgical.Loc1962FixedDateReferences(*c.FixedMonth, *c.FixedDay)...)
	}
	return compactUniq(refs)
}

func (b *loc1962Builder) SundayReferences() []string {
	src := b.sourceSundayReference(b.date)
	refs := append([]string{src}, b.baseBuilder.SundayReferences()...)
	refs = append(refs, sourceSundayAliases(src)...)
	return compactUniq(refs)
}

func (b *loc1962Builder) WeeklyReferences() []string {
	refs := b.baseBuilder.WeeklyReferences()
	return compactUniq(append(refs, b.sourceWeekReferences()...))
}

func sourceSundayAliases(src string) []string {
	switch src {
	case "pentecost_sunday":
		return []string{"pentecost", "sunday_after_ascension"}
	case "easter_sunday":
		return []string{"easter_sunday", "easter_day", "easter"}
	case "sunday_before_advent":
		return []string{"christ_the_king"}
	}
	return nil
}

func (b *loc1962Builder) sourceSundayReference(date Date) string {
	if date.Weekday() != 0 {
		return ""
	}
	m := b.movable()
	must := func(k string) Date {
		d, ok := m[k]
		if !ok {
			rb.RaiseKeyError(":" + k)
		}
		return d
	}
	switch date {
	case must("first_sunday_of_advent"):
		return "first_sunday_of_advent"
	case must("first_sunday_of_advent").Add(-7):
		return "sunday_before_advent"
	case must("easter"):
		return "easter_sunday"
	case must("palm_sunday"):
		return "palm_sunday"
	case must("sunday_after_ascension"):
		return "sunday_after_ascension"
	case must("pentecost"):
		return "pentecost_sunday"
	case must("trinity_sunday"):
		return "trinity_sunday"
	}
	// A Ruby Hash literal: a later duplicate key overwrites the value.
	special := map[Date]string{}
	for _, kv := range []struct{ k, v string }{{"septuagesima", "septuagesima"}, {"sexagesima", "sexagesima"},
		{"quinquagesima", "quinquagesima"}, {"first_sunday_in_lent", "1st_sunday_in_lent"}} {
		special[must(kv.k)] = kv.v
	}
	if s, ok := special[date]; ok {
		return s
	}
	firstLent := must("first_sunday_in_lent")
	if date >= firstLent && date <= firstLent.Add(28) {
		n := date.Sub(firstLent)/7 + 1
		if n >= 1 && n <= 5 {
			return liturgical.Ordinalize(n) + "_sunday_in_lent"
		}
	}
	if s := christmasSundayReference(date); s != "" {
		return s
	}
	if s := epiphanySundayReference(date); s != "" {
		return s
	}
	if s := b.additionalSundayAfterTrinity(date); s != "" {
		return s
	}
	easterSunday := date.Add(-date.Weekday())
	if easterSunday > must("easter") && easterSunday < must("sunday_after_ascension") {
		n := easterSunday.Sub(must("easter")) / 7
		if n >= 1 && n <= 5 {
			return liturgical.Ordinalize(n) + "_sunday_after_easter"
		}
	}
	firstTrinity := must("trinity_sunday").Add(7)
	if date >= firstTrinity && date < must("first_sunday_of_advent").Add(-7) {
		n := date.Sub(firstTrinity)/7 + 1
		if n >= 1 && n <= 26 {
			return liturgical.Ordinalize(n) + "_sunday_after_trinity"
		}
	}
	return ""
}

func (b *loc1962Builder) additionalSundayAfterTrinity(date Date) string {
	m := b.movable()
	firstTrinity := m["trinity_sunday"].Add(7)
	sundayBeforeAdvent := m["first_sunday_of_advent"].Add(-7)
	firstAdditional := firstTrinity.Add(24 * 7)
	if !(date >= firstAdditional && date < sundayBeforeAdvent) {
		return ""
	}
	count := sundayBeforeAdvent.Sub(firstAdditional) / 7
	number := date.Sub(firstAdditional)/7 + 1
	epiphanyNumber := 6
	if count != 1 {
		epiphanyNumber = 4 + number
		if epiphanyNumber > 6 {
			epiphanyNumber = 6
		}
	}
	return liturgical.Ordinalize(epiphanyNumber) + "_sunday_after_epiphany"
}

func firstSundayAfterStrict(d Date) Date {
	s := d.Add(mod(7-d.Weekday(), 7))
	if s == d {
		s = s.Add(7)
	}
	return s
}

func christmasSundayReference(date Date) string {
	year := date.Year()
	if date.Month() == 1 {
		year--
	}
	christmas := civil.MustNew(year, 12, 25)
	if date == christmas {
		return "christmas_day"
	}
	first := firstSundayAfterStrict(christmas)
	if date == first {
		return "1st_sunday_after_christmas"
	}
	if date == first.Add(7) {
		return "2nd_sunday_after_christmas"
	}
	return ""
}

func epiphanySundayReference(date Date) string {
	epiphany := civil.MustNew(date.Year(), 1, 6)
	first := firstSundayAfterStrict(epiphany)
	if !(date >= first && date <= first.Add(35)) {
		return ""
	}
	n := date.Sub(first)/7 + 1
	if n >= 1 && n <= 6 {
		return liturgical.Ordinalize(n) + "_sunday_after_epiphany"
	}
	return ""
}

func (b *loc1962Builder) sourceWeekReferences() []string {
	if b.date.Weekday() == 0 {
		return nil
	}
	m := b.movable()
	d := b.date
	weekday := weekdayNames[d.Weekday()]
	var refs []string
	checks := []struct {
		date Date
		ref  string
	}{
		{m["ash_wednesday"], "ash_wednesday"}, {m["ascension"], "ascension_day"},
		{m["holy_monday"], "monday_before_easter"}, {m["holy_tuesday"], "tuesday_before_easter"},
		{m["holy_wednesday"], "wednesday_before_easter"}, {m["maundy_thursday"], "maundy_thursday"},
		{m["good_friday"], "good_friday"}, {m["holy_saturday"], "holy_saturday"},
		{m["easter_monday"], "easter_monday"}, {m["easter_tuesday"], "easter_tuesday"},
		{m["easter"].Add(3), "easter_wednesday"}, {m["easter"].Add(4), "easter_thursday"},
		{m["easter"].Add(5), "easter_friday"}, {m["easter"].Add(6), "easter_saturday"},
	}
	for _, c := range checks {
		if d == c.date {
			refs = append(refs, c.ref)
		}
	}
	epiphany := civil.MustNew(d.Year(), 1, 6)
	if d > epiphany && d < firstSundayAfterStrict(epiphany) {
		refs = append(refs, "week_of_epiphany_"+weekday)
	}
	if s := b.sourceSundayReferenceForWeek(); s != "" {
		refs = append(refs, s+"_"+weekday)
	}
	return refs
}

func (b *loc1962Builder) sourceSundayReferenceForWeek() string {
	sunday := b.date.Add(-b.date.Weekday())
	m := b.movable()
	if sunday == m["pentecost"] {
		return "pentecost_sunday"
	}
	if sunday == m["trinity_sunday"] {
		return "trinity_sunday"
	}
	return b.sourceSundayReference(sunday)
}
