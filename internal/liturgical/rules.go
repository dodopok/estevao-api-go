package liturgical

import (
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

// RuleSet answers every calendar question that varies by prayer book.
// Port of Liturgical::PrayerBookRules::RuleSet.
type RuleSet struct {
	Code string
	cfg  RuleConfig
}

// Hooks into book-specific psalter tables that live in the reading package.
var (
	DwdoPsalterReference  func(date Date, serviceType string) string
	ProperPsalmsReference func(table string, date Date, serviceType string, movable Movable) string
)

func newRuleSet(code string, cfg RuleConfig) *RuleSet { return &RuleSet{Code: code, cfg: cfg} }

// ForLectionaryVariant returns the rule set with a lectionary variant's
// overrides merged in (unknown variants return an identical copy).
func (r *RuleSet) ForLectionaryVariant(variant string) *RuleSet {
	cfg := r.cfg
	if f, ok := r.cfg.LectionaryVariants[variant]; ok {
		f(&cfg)
	}
	return &RuleSet{Code: r.Code, cfg: cfg}
}

func (r *RuleSet) TraditionalDefaults() bool { return r.cfg.TraditionalDefaults }

func (r *RuleSet) Paschalion() string {
	if r.cfg.Paschalion == "" {
		return Gregorian
	}
	return r.cfg.Paschalion
}

func (r *RuleSet) BookSeasons() bool { return r.cfg.BookSeasons }

func (r *RuleSet) BookSeasonColor(season string) string { return r.cfg.BookSeasonColors[season] }

func (r *RuleSet) EmberDayColor() string { return r.cfg.EmberDayColor }

func (r *RuleSet) EmberDayColorFor(date Date, movable Movable) string {
	if len(r.cfg.EmberDayColors) == 0 {
		return r.EmberDayColor()
	}
	for _, set := range r.cfg.EmberDaySets {
		if containsDate(emberDatesForSet(set, date.Year(), movable), date) {
			if c, ok := r.cfg.EmberDayColors[set.Anchor]; ok {
				return c
			}
			return r.EmberDayColor()
		}
	}
	return r.EmberDayColor()
}

func (r *RuleSet) LocalizedCelebrationColors() bool { return r.cfg.LocalizedCelebrationColors }

func (r *RuleSet) TraditionalCalendar() bool {
	if r.cfg.TraditionalCalendar != nil {
		return *r.cfg.TraditionalCalendar
	}
	return r.TraditionalDefaults()
}

func (r *RuleSet) TrinityCalendar() bool {
	if r.cfg.TrinityCalendar != nil {
		return *r.cfg.TrinityCalendar
	}
	return r.TraditionalCalendar()
}

func (r *RuleSet) TraditionalLectionary() bool {
	if r.cfg.TraditionalLectionary != nil {
		return *r.cfg.TraditionalLectionary
	}
	return r.TraditionalDefaults()
}

func (r *RuleSet) OfficeOnly() bool { return r.cfg.OfficeOnly }

func (r *RuleSet) OfficeLessonsWhenUnappointed() bool {
	return r.OfficeOnly() || r.cfg.OfficeLessonsWhenUnappointed
}

func (r *RuleSet) MonthlyPsalter() bool { return r.cfg.MonthlyPsalter }

// MonthlyPsalterWeekNumber returns 1/2 by month parity for books with two
// columns, or 0 (nil) otherwise.
func (r *RuleSet) MonthlyPsalterWeekNumber(date Date) int {
	if !r.cfg.MonthlyPsalterParity {
		return 0
	}
	if date.Month()%2 == 1 {
		return 1
	}
	return 2
}

func (r *RuleSet) PsalterRepeatsThirtieth() bool  { return r.cfg.PsalterRepeatsThirtieth }
func (r *RuleSet) CumulativePsalmLists() bool     { return r.cfg.CumulativePsalmLists }
func (r *RuleSet) CommonLesserFeastCollect() bool { return r.cfg.CommonLesserFeastCollect }
func (r *RuleSet) FastRules() []FastRule          { return r.cfg.FastObservances }
func (r *RuleSet) TraditionalCollectsInLiturgicalTexts() bool {
	return r.cfg.TraditionalCollectsInLiturgicalTexts
}
func (r *RuleSet) DailyOfficeCourse() bool { return r.cfg.DailyOfficeCourse }
func (r *RuleSet) VigilReadings() bool     { return r.cfg.VigilReadings }
func (r *RuleSet) ReadingCycle() string    { return r.cfg.ReadingCycle }

// FastObservance resolves the day's fasting classification (nil when none).
func (r *RuleSet) FastObservance(date Date, movable Movable, celebration, next *CelebrationAttrs) *FastObservance {
	return (&fastResolver{rules: r.cfg.FastObservances, movable: movable}).resolve(date, celebration, next)
}

func (r *RuleSet) FastObservanceRequiresNextDay() bool {
	for _, rule := range r.cfg.FastObservances {
		if rule.On == "principal_feast_vigil" {
			return true
		}
	}
	return false
}

// CalendarCollectLanguageStyle follows the daily office rite for LOC 1979.
func (r *RuleSet) CalendarCollectLanguageStyle(dailyOfficeRite string) string {
	if !r.cfg.CollectLanguageFollowsRite {
		return ""
	}
	switch dailyOfficeRite {
	case "1":
		return "traditional"
	case "2":
		return "contemporary"
	}
	return ""
}

func (r *RuleSet) BiennialCycleFor(serviceType string, date *Date) bool {
	if r.cfg.CycleScheme == "biennial" {
		return true
	}
	if date != nil && date.IsSunday() {
		return false
	}
	return r.cfg.OfficeCycleScheme == "biennial" && (serviceType == "morning_prayer" || serviceType == "evening_prayer")
}

func (r *RuleSet) CycleScheme() string       { return r.cfg.CycleScheme }
func (r *RuleSet) OfficeCycleScheme() string { return r.cfg.OfficeCycleScheme }

func (r *RuleSet) WeekdayCycleScheme() string {
	if r.cfg.WeekdayCycleScheme == "" {
		return "liturgical_year"
	}
	return r.cfg.WeekdayCycleScheme
}

func (r *RuleSet) WeekdayTableLectionary() bool { return r.cfg.WeekdayTableLectionary }

// NormalizeReadingType returns "" (nil) for weekday-table books.
func (r *RuleSet) NormalizeReadingType(v string) string {
	if r.WeekdayTableLectionary() {
		return ""
	}
	return v
}

func (r *RuleSet) DefaultServiceVariant(serviceType string) string {
	if !r.WeekdayTableLectionary() {
		return ""
	}
	switch serviceType {
	case "morning_prayer":
		return "third_service"
	case "evening_prayer":
		return "second_service"
	}
	return "principal_service"
}

func (r *RuleSet) FixedOfficeEvening(date Date, serviceType string) bool {
	return r.WeekdayTableLectionary() && date.Month() == 12 && date.Day() == 24 && serviceType == "evening_prayer"
}

func (r *RuleSet) WeeklyRuleCode() string {
	if r.WeekdayTableLectionary() {
		return "common_worship_weekday_course"
	}
	return "daily_course"
}

func (r *RuleSet) WeeklySourceCode() string {
	if r.WeekdayTableLectionary() {
		return "common_worship_table_2"
	}
	return "daily_cycle"
}

func (r *RuleSet) ExplanationProfile() string { return r.cfg.ExplanationProfile }
func (r *RuleSet) ExplanationCacheServiceDimensions() bool {
	return r.cfg.ExplanationCacheServiceDimensions
}

func (r *RuleSet) ServiceVariantFromPreferences(ascensionReadings string) string {
	if !r.WeekdayTableLectionary() {
		return ""
	}
	if ascensionReadings == "alternative" {
		return "ascension_alternative"
	}
	return ""
}

func (r *RuleSet) CommonCollectForCommemoration() bool {
	if r.cfg.CommonCollectForCommemoration != nil {
		return *r.cfg.CommonCollectForCommemoration
	}
	return true
}

func (r *RuleSet) FixedDateReadingReferences(month, day int) []string {
	return r.cfg.FixedDateReadingReferences[MD{month, day}]
}

func (r *RuleSet) DatedChristmastideCourse(date Date) bool {
	if !r.cfg.DatedChristmastideCourse {
		return false
	}
	return (date.Month() == 12 && date.Day() >= 26) || (date.Month() == 1 && date.Day() <= 5)
}

func (r *RuleSet) CivilDateReference(date Date) string {
	return r.CivilDateReferenceFor(date.Month(), r.civilCourseDay(date))
}

var ptMonthNames = map[int]string{1: "janeiro", 2: "fevereiro", 3: "marco", 4: "abril", 5: "maio", 6: "junho",
	7: "julho", 8: "agosto", 9: "setembro", 10: "outubro", 11: "novembro", 12: "dezembro"}

func (r *RuleSet) CivilDateReferenceFor(month, day int) string {
	switch r.cfg.CivilDateReferenceStyle {
	case "portuguese_month":
		if name, ok := ptMonthNames[month]; ok {
			return fmt.Sprintf("%s_%d", name, day)
		}
		return ""
	case "numeric", "historic_leap_numeric":
		return fmt.Sprintf("%d-%d", month, day)
	default:
		if month >= 1 && month <= 12 {
			return fmt.Sprintf("%s_%d", strings.ToLower(civil.MonthNamesEN[month]), day)
		}
		return ""
	}
}

func (r *RuleSet) FixedPsalterReference(date Date, serviceType string) string {
	if r.cfg.FixedPsalter != "dwdo" || DwdoPsalterReference == nil {
		return ""
	}
	return DwdoPsalterReference(date, serviceType)
}

func (r *RuleSet) ProperPsalmReference(date Date, serviceType string, movable Movable) string {
	if r.cfg.ProperPsalms == "" || ProperPsalmsReference == nil {
		return ""
	}
	return ProperPsalmsReference(r.cfg.ProperPsalms, date, serviceType, movable)
}

func (r *RuleSet) AlternatingOfficeSeriesAnchorYear() int {
	return r.cfg.AlternatingOfficeSeriesAnchorYear
}

func (r *RuleSet) ObservesBaptism() bool {
	return r.cfg.ObservesBaptism == nil || *r.cfg.ObservesBaptism
}

func (r *RuleSet) ObservesRogationSunday() bool { return r.cfg.ObservesRogationSunday }

func (r *RuleSet) ObservesChristTheKing() bool {
	return r.cfg.ObservesChristTheKing == nil || *r.cfg.ObservesChristTheKing
}

func (r *RuleSet) UsesSundayBeforeAdventReference() bool { return r.cfg.SundayBeforeAdventReference }

func (r *RuleSet) EpiphanyStart(movable Movable) Date {
	key := r.cfg.EpiphanyStart
	if key == "" {
		key = "baptism_of_the_lord"
	}
	if key == "epiphany" {
		return civil.MustNew(movable["easter"].Year(), 1, 6)
	}
	return movable[key]
}

func (r *RuleSet) EpiphanyWeekStart(movable Movable) Date {
	start := r.EpiphanyStart(movable)
	if !r.cfg.EpiphanyWeeksFromNextSunday {
		return start
	}
	return start.Add(7 - start.Weekday())
}

func (r *RuleSet) AllSaintsDate(year int) Date {
	md := MD{11, 1}
	if r.cfg.AllSaintsDate != nil {
		md = *r.cfg.AllSaintsDate
	}
	return civil.MustNew(year, md[0], md[1])
}

func (r *RuleSet) AllSaintsTransferMode() string {
	if r.cfg.AllSaintsTransfer == "" {
		return "nearest_sunday"
	}
	return r.cfg.AllSaintsTransfer
}

func (r *RuleSet) AllSaintsOctave() bool { return r.cfg.AllSaintsOctave }

func (r *RuleSet) inAllSaintsOctave(date Date) bool {
	as := r.AllSaintsDate(date.Year())
	return date.Between(as, as.Add(7))
}

// OctaveColorFor returns the octave colour of a weekday, or "".
func (r *RuleSet) OctaveColorFor(date Date) string {
	if date.IsSunday() || !r.AllSaintsOctave() || !r.inAllSaintsOctave(date) {
		return ""
	}
	return r.cfg.AllSaintsOctaveColor
}

// TransferDateFor applies a book's own transfer rubric. ok=false means the
// book has no source-specific rule and the shared fallback applies.
func (r *RuleSet) TransferDateFor(c *Celebration, original Date, movable Movable, occupied []Date) (Date, bool) {
	if r.cfg.TransferCalendar != "bcp_1962_canada" {
		return 0, false
	}
	if c != nil && c.Movable {
		return original, true
	}
	if c == nil || !(c.IsMajorHolyDay() || c.IsPrincipalFeast()) {
		return original, true
	}
	if canadianFixedHolyDaySpecialSunday(c, original, movable) {
		return canadianFollowingFreeDate(nextTuesday(original), occupied), true
	}
	if original.Between(movable["palm_sunday"], movable["easter"]) {
		return canadianFollowingFreeDate(movable["easter"].Add(2), occupied), true
	}
	if original.Between(movable["pentecost"], movable["trinity_sunday"]) {
		return canadianFollowingFreeDate(movable["trinity_sunday"].Add(2), occupied), true
	}
	if original.IsMonday() && !canadianChristmasToEpiphany(original) {
		return canadianFollowingFreeDate(original.Add(1), occupied), true
	}
	return original, true
}

func (r *RuleSet) SuppressAscensionOn(date Date) bool {
	for _, md := range r.cfg.SuppressAscensionOn {
		if md[0] == date.Month() && md[1] == date.Day() {
			return true
		}
	}
	return false
}

func (r *RuleSet) DisplaceTransferredNominalDate() bool { return r.cfg.DisplaceTransferredNominalDate }

// FixedHolyDaySecondReading returns the replacement second lesson, or "".
func (r *RuleSet) FixedHolyDaySecondReading(dateReference, serviceType string, observed Date, movable Movable, alternative string) (string, bool) {
	for _, rule := range r.cfg.FixedHolyDaySubstitutions {
		if !contains(rule.DateReferences, dateReference) || !contains(rule.ServiceTypes, serviceType) {
			continue
		}
		if !substitutionConditionMet(rule.Condition, observed, movable) {
			continue
		}
		if rule.Alternative {
			return alternative, true
		}
		return rule.SecondReading, true
	}
	return "", false
}

func (r *RuleSet) WeeklyReferenceAliases(reference string) []string {
	return r.cfg.WeeklyReferenceAliases[reference]
}

func (r *RuleSet) DatedSundayReferenceAliases(date Date, movable Movable) []string {
	if !r.cfg.DatedSundayReferences || !date.IsSunday() {
		return nil
	}
	if refs, ok := preLentSundayReferences(date, movable); ok {
		return refs
	}
	if refs, ok := preAdventSundayReferences(date, movable); ok {
		return refs
	}
	return nil
}

func (r *RuleSet) VigilReadingReferences(date Date, movable Movable) []string {
	refs := append([]string{}, r.cfg.VigilReadingReferences[MD{date.Month(), date.Day()}]...)
	if r.cfg.EasterVigil && movable != nil && date == movable["holy_saturday"] {
		refs = append(refs, "easter_vigil")
	}
	refs = append(refs, r.movableVigilReadingReferences(date, movable)...)
	return uniqStrings(refs)
}

func (r *RuleSet) movableVigilReadingReferences(date Date, movable Movable) []string {
	if len(movable) == 0 {
		return nil
	}
	var out []string
	for _, v := range r.cfg.MovableVigilReadingReferences {
		anchor, ok := movable[v.Anchor]
		if ok && date == anchor.Add(v.Offset) {
			out = append(out, v.Reference)
		}
	}
	return out
}

func (r *RuleSet) WeeklyReadingReferences(date Date, m Movable) []string {
	if !r.TraditionalLectionary() {
		return nil
	}
	var refs []string
	switch date {
	case m["ash_wednesday"].Add(1):
		refs = append(refs, "ash_wednesday_thursday")
	case m["ash_wednesday"].Add(2):
		refs = append(refs, "ash_wednesday_friday")
	case m["ash_wednesday"].Add(3):
		refs = append(refs, "ash_wednesday_saturday")
	case m["holy_monday"]:
		refs = append(refs, "monday_before_easter")
	case m["holy_tuesday"]:
		refs = append(refs, "tuesday_before_easter")
	case m["holy_wednesday"]:
		refs = append(refs, "wednesday_before_easter")
	case m["maundy_thursday"]:
		refs = append(refs, "maundy_thursday")
	case m["good_friday"]:
		refs = append(refs, "good_friday")
	case m["holy_saturday"]:
		refs = append(refs, "holy_saturday")
	}
	add := func(cond bool, ref string) {
		if cond {
			refs = append(refs, ref)
		}
	}
	e := m["easter"]
	add(date == m["easter_monday"], "easter_monday")
	add(date == m["easter_tuesday"], "easter_tuesday")
	add(date == e.Add(3), "easter_wednesday")
	add(date == e.Add(4), "easter_thursday")
	add(date == e.Add(5), "easter_friday")
	add(date == e.Add(6), "easter_saturday")
	add(date == m["ascension_eve"], "ascension_eve")
	add(date == m["ascension_friday"], "ascension_friday")
	add(date == m["ascension_saturday"], "ascension_saturday")
	add(date == m["pentecost_eve"], "pentecost_eve")
	add(date == m["week_of_pentecost_wednesday"], "week_of_pentecost_wednesday")
	add(date == m["week_of_pentecost_thursday"], "week_of_pentecost_thursday")
	add(date == m["week_of_pentecost_friday"], "week_of_pentecost_friday")
	add(date == m["week_of_pentecost_saturday"], "week_of_pentecost_saturday")
	add(date == m["trinity_sunday"].Add(-1), "trinity_eve")
	add(date == m["rogation_sunday"], "rogation_sunday")
	add(r.thanksgivingDateEnabled() && date == m["thanksgiving_first_thursday"], "thanksgiving_day")
	if ember := r.EmberReadingReference(date, m); ember != "" {
		refs = append(refs, ember)
	}
	for i, d := range r.RogationDays(m) {
		if d == date {
			refs = append(refs, "rogation_"+[]string{"monday", "tuesday", "wednesday"}[i])
			break
		}
	}
	return uniqStrings(refs)
}

// SpecialWeekCollectReferences returns nil when not configured or not a
// Holy Week weekday.
func (r *RuleSet) SpecialWeekCollectReferences(date, easter Date) []string {
	if !r.cfg.SpecialWeekCollects {
		return nil
	}
	switch date {
	case easter.Add(-6):
		return []string{"monday_before_easter"}
	case easter.Add(-5):
		return []string{"tuesday_before_easter"}
	case easter.Add(-4):
		return []string{"wednesday_before_easter"}
	case easter.Add(-3):
		return []string{"thursday_before_easter"}
	}
	return nil
}

func (r *RuleSet) EmberDays(year int, movable Movable) []Date {
	if !r.cfg.EmberDays {
		return nil
	}
	return r.configuredEmberDays(year, movable)
}

func (r *RuleSet) repeatedCollectsEnabled() bool {
	if r.cfg.RepeatedCollects != nil {
		return *r.cfg.RepeatedCollects
	}
	return r.TraditionalDefaults()
}

func (r *RuleSet) RepeatedCollectReferences(date Date, m Movable) []string {
	if !r.repeatedCollectsEnabled() {
		return nil
	}
	var refs []string
	christmas := civil.MustNew(date.Year(), 12, 25)
	if date.Month() == 1 {
		christmas = civil.MustNew(date.Year()-1, 12, 25)
	}
	epiphany := civil.MustNew(date.Year(), 1, 6)
	if date >= m["first_sunday_of_advent"] && date < christmas {
		refs = append(refs, "1st_sunday_of_advent")
	}
	if date.Between(christmas, christmas.Add(7)) {
		refs = append(refs, "christmas_day")
	}
	if r.cfg.EpiphanyCollectOctave && date.Between(epiphany, epiphany.Add(7)) {
		refs = append(refs, "epiphany")
	}
	if date >= m["ash_wednesday"] && date < m["palm_sunday"] {
		refs = append(refs, "ash_wednesday")
	}
	if date >= m["palm_sunday"] && date <= m["good_friday"] && !r.repeatedCollectExcluded("palm_sunday", date, m) {
		refs = append(refs, "palm_sunday")
	}
	if date >= m["easter"] && date <= m["easter"].Add(6) {
		refs = append(refs, "easter_day")
	}
	if date.Between(m["ascension"], m["ascension"].Add(7)) && !r.SuppressAscensionOn(date) {
		refs = append(refs, "ascension_day")
	}
	if date >= m["pentecost"] && date <= m["pentecost"].Add(6) {
		refs = append(refs, "whitsunday")
	}
	if r.AllSaintsOctave() && r.inAllSaintsOctave(date) {
		refs = append(refs, "all_saints")
	}
	if containsDate(r.EmberDays(date.Year(), m), date) {
		refs = append(refs, "ember_days")
	}
	if containsDate(r.RogationDays(m), date) {
		refs = append(refs, "rogation_days")
	}
	if r.thanksgivingDateEnabled() && date == firstThursdayOfNovember(date.Year()) {
		refs = append(refs, "thanksgiving_day")
	}
	return refs
}

func (r *RuleSet) repeatedCollectExcluded(reference string, date Date, m Movable) bool {
	for _, key := range r.cfg.RepeatedCollectExclusions[reference] {
		if d, ok := m[key]; ok && d == date {
			return true
		}
	}
	return false
}

func (r *RuleSet) SuppressSeasonalCollectFallback(date Date, m Movable) bool {
	for _, key := range r.cfg.SuppressSeasonalCollectFallbackDates {
		if d, ok := m[key]; ok && d == date {
			return true
		}
	}
	if !contains(r.cfg.SuppressSeasonalCollectFallbackPeriods, "all_saints_octave") {
		return false
	}
	if date.IsSunday() {
		return false
	}
	return r.AllSaintsOctave() && r.inAllSaintsOctave(date)
}

func (r *RuleSet) EmberReadingReference(date Date, m Movable) string {
	if !r.cfg.EmberReadingReferences {
		return ""
	}
	if len(r.cfg.EmberDaySets) > 0 {
		for _, set := range r.cfg.EmberDaySets {
			if set.ReadingPrefix == "" {
				continue
			}
			if ref := referenceForDays(date, emberDatesForSet(set, date.Year(), m), set.ReadingPrefix); ref != "" {
				return ref
			}
		}
		return ""
	}
	if len(r.cfg.EmberReadingPrefixes) > 0 {
		sets := [][]Date{
			daysAfterSunday(m["first_sunday_in_lent"]),
			daysAfterSunday(m["pentecost"]),
			daysAfterFixedDate(civil.MustNew(date.Year(), 9, 14)),
			daysAfterFixedDate(civil.MustNew(date.Year(), 12, 13)),
		}
		for i, prefix := range r.cfg.EmberReadingPrefixes {
			if prefix == "" || i >= len(sets) {
				continue
			}
			if ref := referenceForDays(date, sets[i], prefix); ref != "" {
				return ref
			}
		}
		return ""
	}
	if ref := referenceForDays(date, daysAfterSunday(m["first_sunday_in_lent"]), "1st_sunday_in_lent"); ref != "" {
		return ref
	}
	if ref := referenceForDays(date, daysAfterSunday(m["pentecost"]), "week_of_pentecost"); ref != "" {
		return ref
	}
	if ref := referenceForDays(date, daysAfterFixedDate(civil.MustNew(date.Year(), 9, 14)), "autumn_ember"); ref != "" {
		return ref
	}
	return referenceForDays(date, daysAfterFixedDate(civil.MustNew(date.Year(), 12, 13)), "3rd_sunday_of_advent")
}

// RepeatedCollectCelebrationQuery returns the query for reference, resolving
// the book's All Saints date.
func (r *RuleSet) RepeatedCollectCelebrationQuery(reference string, year int) (CollectQuery, bool) {
	q, ok := r.cfg.RepeatedCollectCelebrationQueries[reference]
	if !ok {
		return q, false
	}
	if q.FixedAllSaints {
		d := r.AllSaintsDate(year)
		q.FixedDate = &MD{d.Month(), d.Day()}
		q.FixedAllSaints = false
	}
	return q, true
}

func (r *RuleSet) LateTrinityReadingReference(date Date, m Movable) string {
	if !r.TraditionalCalendar() || !date.IsSunday() {
		return ""
	}
	first := m["trinity_sunday"].Add(7)
	last := m["first_sunday_of_advent"].Add(-7)
	if !(date >= first && date < last) {
		return ""
	}
	total := last.Sub(first)/7 + 1
	number := date.Sub(first)/7 + 1
	switch total {
	case 26:
		if number == 25 {
			return "6th_sunday_after_epiphany"
		}
	case 27:
		switch number {
		case 25:
			return "5th_sunday_after_epiphany"
		case 26:
			return "6th_sunday_after_epiphany"
		}
	}
	return ""
}

func (r *RuleSet) civilCourseDay(date Date) int {
	if r.cfg.CivilDateReferenceStyle != "historic_leap_numeric" {
		return date.Day()
	}
	if !(civil.IsLeap(date.Year()) && date.Month() == 2 && date.Day() >= 26) {
		return date.Day()
	}
	return date.Day() - 1
}

func (r *RuleSet) configuredEmberDays(year int, m Movable) []Date {
	if r.cfg.EmberDaySets == nil {
		var out []Date
		out = append(out, daysAfterSunday(m["first_sunday_in_lent"])...)
		out = append(out, daysAfterSunday(m["pentecost"])...)
		out = append(out, daysAfterFixedDate(civil.MustNew(year, 9, 14))...)
		out = append(out, daysAfterFixedDate(civil.MustNew(year, 12, 13))...)
		return out
	}
	var out []Date
	for _, set := range r.cfg.EmberDaySets {
		out = append(out, emberDatesForSet(set, year, m)...)
	}
	return uniqDates(out)
}

// RogationDays are the three days before the Ascension.
func (r *RuleSet) RogationDays(m Movable) []Date {
	a := m["ascension"]
	return []Date{a.Add(-3), a.Add(-2), a.Add(-1)}
}

func (r *RuleSet) thanksgivingDateEnabled() bool {
	return r.cfg.ThanksgivingFirstThursday == nil || *r.cfg.ThanksgivingFirstThursday
}

var preLentPropers = []struct {
	number      int
	first, last int
}{{1, 4, 10}, {2, 11, 17}, {3, 18, 24}}

func preLentSundayReferences(date Date, m Movable) ([]string, bool) {
	ash, ok1 := m["ash_wednesday"]
	baptism, ok2 := m["baptism_of_the_lord"]
	if !ok1 || !ok2 {
		return nil, false
	}
	nextBeforeLent := ash.Add(-3)
	secondBeforeLent := ash.Add(-10)
	if date == nextBeforeLent {
		return []string{"sunday_next_before_lent"}, true
	}
	if date == secondBeforeLent {
		return []string{"2nd_sunday_before_lent"}, true
	}
	if !(date > baptism && date < secondBeforeLent) {
		return nil, false
	}
	epiphanyNumber := date.Sub(baptism)/7 + 1
	epiphanyRef := ""
	if epiphanyNumber >= 2 && epiphanyNumber <= 4 {
		epiphanyRef = Ordinalize(epiphanyNumber) + "_sunday_of_epiphany"
	}
	beforeLentNumber := nextBeforeLent.Sub(date)/7 + 1
	titleRef := ""
	if beforeLentNumber >= 3 && beforeLentNumber <= 5 {
		titleRef = Ordinalize(beforeLentNumber) + "_sunday_before_lent"
	}
	properRef := ""
	for _, p := range preLentPropers {
		if date.Month() == 2 && date.Day() >= p.first && date.Day() <= p.last {
			properRef = fmt.Sprintf("proper_%d", p.number)
			break
		}
	}
	var titles []string
	if date > civil.MustNew(date.Year(), 2, 2) {
		titles = []string{titleRef, epiphanyRef}
	} else {
		titles = []string{epiphanyRef, titleRef}
	}
	return compactStrings(append([]string{properRef}, titles...)), true
}

func preAdventSundayReferences(date Date, m Movable) ([]string, bool) {
	advent, ok := m["first_sunday_of_advent"]
	if !ok {
		return nil, false
	}
	if !(date < advent && date >= advent.Add(-28)) {
		return nil, false
	}
	number := advent.Sub(date) / 7
	if number < 2 || number > 4 {
		return nil, false
	}
	return []string{Ordinalize(number) + "_sunday_before_advent"}, true
}

func substitutionConditionMet(c *SubstitutionCondition, observed Date, m Movable) bool {
	if c == nil {
		return true
	}
	anchor := m[c.Anchor]
	boundary := anchor.Add(c.Offset)
	switch c.Operator {
	case "on_or_after":
		return observed >= boundary
	case "after":
		return observed > boundary
	case "before":
		return observed < boundary
	case "between_inclusive":
		return observed.Between(boundary, anchor.Add(c.Finish))
	}
	return false
}

func daysAfterSunday(sunday Date) []Date {
	return []Date{sunday.Add(3), sunday.Add(5), sunday.Add(6)}
}

func daysAfterFixedDate(date Date) []Date {
	wed := date.Add(mod(3-date.Weekday(), 7))
	if wed <= date {
		wed = wed.Add(7)
	}
	return []Date{wed, wed.Add(2), wed.Add(3)}
}

func emberDatesForSet(set EmberSet, year int, m Movable) []Date {
	switch set.AnchorType {
	case "sunday":
		var sunday Date
		if set.Anchor == "third_sunday_of_advent" {
			sunday = m["first_sunday_of_advent"].Add(14)
		} else {
			sunday = m[set.Anchor]
		}
		return daysAfterSunday(sunday)
	case "fixed":
		return daysAfterFixedDate(civil.MustNew(year, set.Month, set.Day))
	}
	return nil
}

func firstThursdayOfNovember(year int) Date {
	d := civil.MustNew(year, 11, 1)
	return d.Add(mod(4-d.Weekday(), 7))
}

func canadianFixedHolyDaySpecialSunday(c *Celebration, date Date, m Movable) bool {
	preLent := date == m["septuagesima"] || date == m["sexagesima"] || date == m["quinquagesima"]
	if c.PostSlug == "celebration-presentation" && preLent {
		return false
	}
	if (c.PostSlug == "celebration-conversion-of-st-paul" || c.PostSlug == "celebration-matthias-the-apostle") && preLent {
		return true
	}
	if date == m["ash_wednesday"] || date == m["ascension"] {
		return true
	}
	if !date.IsSunday() {
		return false
	}
	advent := m["first_sunday_of_advent"]
	palm := m["palm_sunday"]
	inAdventOrLent := date.Between(advent, civil.MustNew(date.Year(), 12, 24)) ||
		date.Between(m["first_sunday_in_lent"], palm.Add(-7))
	if inAdventOrLent {
		return true
	}
	return false
}

func canadianChristmasToEpiphany(date Date) bool {
	christmas := civil.MustNew(date.Year(), 12, 25)
	if date.Month() <= 1 {
		christmas = civil.MustNew(date.Year()-1, 12, 25)
	}
	return date.Between(christmas, christmas.Add(12))
}

func nextTuesday(date Date) Date { return date.Add(mod(2-date.Weekday(), 7)) }

func canadianFollowingFreeDate(date Date, occupied []Date) Date {
	c := date
	for containsDate(occupied, c) {
		c = c.Add(1)
	}
	return c
}

func referenceForDays(date Date, dates []Date, prefix string) string {
	for i, d := range dates {
		if d == date {
			return prefix + "_" + []string{"wednesday", "friday", "saturday"}[i]
		}
	}
	return ""
}

// Ordinalize mirrors ActiveSupport's Integer#ordinalize.
func Ordinalize(n int) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	if r := abs % 100; r >= 11 && r <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch abs % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsDate(list []Date, d Date) bool {
	for _, v := range list {
		if v == d {
			return true
		}
	}
	return false
}

func uniqStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func uniqDates(in []Date) []Date {
	seen := make(map[Date]bool, len(in))
	out := make([]Date, 0, len(in))
	for _, d := range in {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
