package liturgical

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// LoadCelebrations returns a book's celebrations (empty when the book does
// not exist). Wired by the application to the database-backed cache.
var LoadCelebrations = func(code string) *BookCelebrations { return &BookCelebrations{} }

// Calendar ports LiturgicalCalendar for one book and civil year.
type Calendar struct {
	Year   int
	Code   string
	Easter *Easter
	Rules  *RuleSet
	Season *SeasonDeterminator

	mu       sync.Mutex
	book     *BookCelebrations
	resolver *dayResolver
	contexts map[Date]*DayContext
	siblings map[int]*Calendar
}

// NewCalendar builds the calendar of year for a prayer book code.
func NewCalendar(year int, code string) *Calendar {
	rules := RulesFor(code)
	easter := EasterFor(year, rules.Paschalion())
	return &Calendar{
		Year:     year,
		Code:     code,
		Easter:   easter,
		Rules:    rules,
		Season:   NewSeasonDeterminator(year, easter, code),
		contexts: map[Date]*DayContext{},
		siblings: map[int]*Calendar{},
	}
}

// ForDate returns the calendar owning date's civil year.
func (c *Calendar) ForDate(date Date) *Calendar {
	if date.Year() == c.Year {
		return c
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.siblings[date.Year()]; ok {
		return s
	}
	s := NewCalendar(date.Year(), c.Code)
	s.book = c.book
	c.siblings[date.Year()] = s
	return s
}

func (c *Calendar) dayResolver() *dayResolver {
	if c.resolver == nil {
		if c.book == nil {
			c.book = LoadCelebrations(c.Code)
		}
		c.resolver = newDayResolver(c.Year, c.Code, c.Easter, c.book)
	}
	return c.resolver
}

// ContextFor resolves the liturgical day.
func (c *Calendar) ContextFor(date Date) *DayContext {
	if date.Year() != c.Year {
		return c.ForDate(date).ContextFor(date)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx, ok := c.contexts[date]; ok {
		return ctx
	}
	ctx := c.dayResolver().resolve(date)
	c.contexts[date] = ctx
	return ctx
}

// CelebrationResolver exposes the resolver for callers needing candidates.
func (c *Calendar) CelebrationResolver() *CelebrationResolver {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dayResolver().celebs
}

func (c *Calendar) SeasonFor(date Date) string { return c.ContextFor(date).Season }

func (c *Calendar) SeasonRangeFor(date Date) SeasonRange {
	if date.Year() != c.Year {
		return c.ForDate(date).SeasonRangeFor(date)
	}
	return c.Season.RangeFor(date)
}

// CelebrationForDate returns the legacy hash of the primary celebration or nil.
func (c *Calendar) CelebrationForDate(date Date) *rb.Map {
	return c.ContextFor(date).Primary.LegacyH()
}

func (c *Calendar) ColorForDate(date Date) string {
	if date.Year() != c.Year {
		return c.ForDate(date).ColorForDate(date)
	}
	var attrs *CelebrationAttrs
	if p := c.ContextFor(date).Primary; p != nil {
		attrs = p.Attrs
	}
	return ColorFor(c.Season, c.Easter, c.Rules, date, attrs)
}

func (c *Calendar) WeekNumber(date Date) (int, bool) {
	ctx := c.ContextFor(date)
	return ctx.Week, ctx.HasWeek
}

// SundayName ports LiturgicalCalendar#sunday_name ("" == nil).
func (c *Calendar) SundayName(date Date) string {
	if date.Year() != c.Year {
		return c.ForDate(date).SundayName(date)
	}
	if !date.IsSunday() {
		return ""
	}
	season := c.SeasonFor(date)
	m := c.Easter.Dates
	if date == m["baptism_of_the_lord"] && c.Rules.ObservesBaptism() {
		return "Batismo de nosso Senhor Jesus Cristo"
	}
	if date == m["rogation_sunday"] && c.Rules.ObservesRogationSunday() {
		return "Domingo de Rogação"
	}
	switch date {
	case m["easter"]:
		return "Domingo da Páscoa"
	case m["pentecost"]:
		return "Pentecostes"
	case m["trinity_sunday"]:
		return "Santíssima Trindade"
	case m["sunday_after_ascension"]:
		return "Domingo após a Ascensão"
	case m["palm_sunday"]:
		return "Domingo de Ramos"
	case m["first_sunday_of_advent"]:
		return "1º Domingo do Advento"
	}
	if date == m["christ_the_king"] && c.Rules.ObservesChristTheKing() {
		return "Cristo Rei do Universo"
	}
	week, _ := c.WeekNumber(date)
	if c.Rules.TraditionalCalendar() {
		ash := m["ash_wednesday"]
		switch date {
		case ash.Add(-17):
			return "Septuagésima"
		case ash.Add(-10):
			return "Sexagésima"
		case ash.Add(-3):
			return "Quinquagésima"
		}
		if season == EasterSeason && date > m["easter"] {
			return fmt.Sprintf("%dº Domingo após a Páscoa", week-1)
		}
	}
	if season == Christmas {
		return fmt.Sprintf("%dº Domingo após Natal", week)
	}
	prep := "do"
	switch season {
	case OrdinaryTime:
		prep = "no"
	case Lent:
		prep = "na"
	case Epiphany, EasterSeason:
		prep = "da"
	}
	if c.Rules.TrinityCalendar() && season == OrdinaryTime && date > m["trinity_sunday"] {
		return fmt.Sprintf("%dº Domingo após a Trindade", week)
	}
	return fmt.Sprintf("%dº Domingo %s %s", week, prep, season)
}

// SundayAfterPentecost ports the delegated WeekCalculator method.
func (c *Calendar) SundayAfterPentecost(date Date) (int, bool) {
	if date.Year() != c.Year {
		return c.ForDate(date).SundayAfterPentecost(date)
	}
	return c.Season.SundayAfterPentecost(date)
}

// ProperNumber ports proper_number(date) (season: nil).
func (c *Calendar) ProperNumber(date Date) (int, bool) {
	ctx := c.ContextFor(date)
	return ctx.Proper, ctx.HasProper
}

// ProperNumberFor ports proper_number(date, season:).
func (c *Calendar) ProperNumberFor(date Date, season string) (int, bool) {
	if date.Year() != c.Year {
		return c.ForDate(date).ProperNumberFor(date, season)
	}
	return ProperNumber(c.Year, c.Easter, date, season)
}

func (c *Calendar) LiturgicalYearCycle(date Date) string { return c.ContextFor(date).Cycle }

// Description ports LiturgicalCalendar#description.
func (c *Calendar) Description(date Date) []string {
	if date.Year() != c.Year {
		return c.ForDate(date).Description(date)
	}
	descs := []string{}
	m := c.Easter.Dates
	if cel := c.ContextFor(date).Primary; cel != nil {
		t := cel.Attrs.Type
		if t == "principal_feast" || t == "major_holy_day" {
			name := cel.Name
			if cel.Attrs.Transferred {
				name += " (movido)"
			}
			return append(descs, name)
		}
	}
	if date >= m["palm_sunday"] && date <= m["holy_saturday"] {
		if n := holyWeekDayName(date, m); n != "" {
			descs = append(descs, n)
		}
		return descs
	}
	if special := c.specialMovableDayName(date, m); special != "" {
		descs = append(descs, special)
	}
	for _, d := range c.weekPeriodDescriptions(date) {
		if !contains(descs, d) {
			descs = append(descs, d)
		}
	}
	return descs
}

func holyWeekDayName(date Date, m Movable) string {
	p := m["palm_sunday"]
	switch date {
	case p:
		return "Domingo de Ramos"
	case p.Add(1):
		return "Segunda-feira Santa"
	case p.Add(2):
		return "Terça-feira Santa"
	case p.Add(3):
		return "Quarta-feira Santa"
	case m["maundy_thursday"]:
		return "Quinta-feira Santa"
	case m["good_friday"]:
		return "Sexta-feira da Paixão"
	case m["holy_saturday"]:
		return "Sábado Santo"
	}
	return ""
}

func (c *Calendar) specialMovableDayName(date Date, m Movable) string {
	switch date {
	case m["ash_wednesday"]:
		return "Quarta-feira de Cinzas"
	case m["easter"]:
		return "Domingo da Páscoa"
	case m["pentecost"]:
		return "Pentecostes"
	case m["trinity_sunday"]:
		return "Santíssima Trindade"
	case m["ascension"]:
		return "Ascensão"
	case m["christ_the_king"]:
		if c.Rules.ObservesChristTheKing() {
			return "Cristo Rei do Universo"
		}
		return ""
	case m["baptism_of_the_lord"]:
		if !c.Rules.ObservesBaptism() {
			return ""
		}
		return "Batismo de nosso Senhor Jesus Cristo"
	}
	return ""
}

func (c *Calendar) weekPeriodDescriptions(date Date) []string {
	var out []string
	season := c.SeasonFor(date)
	week, _ := c.WeekNumber(date)
	m := c.Easter.Dates
	switch season {
	case Advent:
		out = append(out, fmt.Sprintf("%dª Semana do Advento", week))
	case Christmas:
		if date.Month() == 12 && date.Day() >= 25 {
			out = append(out, "Oitava do Natal")
		} else if date.Month() == 1 {
			out = append(out, "Semana após o Natal")
		}
	case Epiphany:
		if n := c.preLentWeekName(date, m); n != "" {
			out = append(out, n)
		} else {
			out = append(out, fmt.Sprintf("%dª Semana após a Epifania", week))
		}
	case Lent:
		if date < m["palm_sunday"] {
			if date >= m["ash_wednesday"] && date < m["first_sunday_in_lent"] {
				out = append(out, "Semana após a Quarta-feira de Cinzas")
			} else {
				out = append(out, fmt.Sprintf("%dª Semana da Quaresma", week))
			}
		}
	case EasterSeason:
		if date > m["easter"] && date <= m["easter"].Add(7) {
			out = append(out, "Oitava da Páscoa")
		} else {
			out = append(out, fmt.Sprintf("%dª Semana da Páscoa", week))
		}
	case OrdinaryTime:
		if date >= m["pentecost"] {
			out = append(out, c.afterPentecostDescriptions(date)...)
		} else {
			out = append(out, fmt.Sprintf("%dª Semana do Tempo Comum", week))
		}
	}
	return out
}

func referenceSunday(date Date) Date {
	if date.IsSunday() {
		return date
	}
	return date.Add(-date.Weekday())
}

func (c *Calendar) afterPentecostDescriptions(date Date) []string {
	ref := referenceSunday(date)
	if ref == c.Easter.Dates["pentecost"] {
		return []string{"Oitava de Pentecostes"}
	}
	if c.Rules.TrinityCalendar() {
		if ref == c.Easter.Dates["trinity_sunday"] {
			return []string{"Semana após a Trindade"}
		}
		week, ok := c.WeekNumber(ref)
		if !ok || week <= 0 {
			return nil
		}
		return []string{fmt.Sprintf("%dª Semana após a Trindade", week)}
	}
	var out []string
	if proper, ok := c.ProperNumber(ref); ok && proper >= 3 {
		out = append(out, fmt.Sprintf("Próprio %d", proper))
		out = append(out, fmt.Sprintf("%dª Semana do Tempo Comum", proper+5))
	}
	if after, ok := c.SundayAfterPentecost(ref); ok && after > 0 {
		out = append(out, fmt.Sprintf("%dª Semana após Pentecostes", after))
	}
	return out
}

func (c *Calendar) preLentWeekName(date Date, m Movable) string {
	if !c.Rules.TraditionalCalendar() {
		return ""
	}
	ash := m["ash_wednesday"]
	switch referenceSunday(date) {
	case ash.Add(-17):
		return "Semana da Septuagésima"
	case ash.Add(-10):
		return "Semana da Sexagésima"
	case ash.Add(-3):
		return "Semana da Quinquagésima"
	}
	return ""
}

// DayInfo ports LiturgicalCalendar#day_info (DayPresenter#to_legacy_h).
func (c *Calendar) DayInfo(date Date) *rb.Map {
	if date.Year() != c.Year {
		return c.ForDate(date).DayInfo(date)
	}
	ctx := c.ContextFor(date)
	var sundayName any
	if n := c.SundayName(date); n != "" {
		sundayName = n
	}
	var bookSeason, bookSeasonSlug any
	if ctx.BookSeason != "" {
		bookSeason = ctx.BookSeason
		bookSeasonSlug = BookSeasonSlug(ctx.BookSeason)
	}
	var celebration any
	if ctx.Primary != nil {
		celebration = ctx.Primary.LegacyH()
	}
	celebrations := []any{}
	for _, o := range ctx.Celebrations() {
		celebrations = append(celebrations, o.LegacyH())
	}
	var week, proper, sap any
	if ctx.HasWeek {
		week = ctx.Week
	}
	if ctx.HasProper {
		proper = ctx.Proper
	}
	if v, ok := c.SundayAfterPentecost(date); ok {
		sap = v
	}
	var saint any
	if p := ctx.Primary; p != nil && (p.Attrs.Type == "lesser_feast" || p.Attrs.Type == "commemoration") {
		saint = rb.M("name", p.Name, "description", p.Attrs.Description)
	}
	desc := []any{}
	for _, d := range c.Description(date) {
		desc = append(desc, d)
	}
	var slug any
	if s, ok := SeasonSlugs[ctx.Season]; ok {
		slug = s
	}
	return rb.M(
		"date", date.DMY(),
		"sunday_name", sundayName,
		"description", desc,
		"day_of_week", DayNamesPT[date.Weekday()],
		"liturgical_season", ctx.Season,
		"season_post_slug", slug,
		"book_season", bookSeason,
		"book_season_post_slug", bookSeasonSlug,
		"color", ctx.Color,
		"celebration", celebration,
		"celebrations", celebrations,
		"is_sunday", ctx.Sunday,
		"is_holy_day", ctx.HolyDay,
		"fast_day", ctx.FastDay(),
		"fast_observance", FastObservanceMap(ctx.FastObservance),
		"week_of_season", week,
		"proper_week", proper,
		"sunday_after_pentecost", sap,
		"liturgical_year", ctx.Cycle,
		"saint", saint,
	)
}

// FastObservanceMap renders the observance hash (nil when none).
func FastObservanceMap(f *FastObservance) any {
	if f == nil {
		return nil
	}
	return rb.M("kind", f.Kind, "status", f.Status, "reason", f.Reason)
}

// MonthCalendar returns day_info for each day of month.
func (c *Calendar) MonthCalendar(month int) []*rb.Map {
	var out []*rb.Map
	days := civil.DaysInMonth(c.Year, month)
	for d := 1; d <= days; d++ {
		out = append(out, c.DayInfo(civil.MustNew(c.Year, month, d)))
	}
	return out
}

// SaintForDate ports saint_for_date.
func (c *Calendar) SaintForDate(date Date) *rb.Map {
	p := c.ContextFor(date).Primary
	if p == nil {
		return nil
	}
	if p.Attrs.Type == "lesser_feast" || p.Attrs.Type == "commemoration" || saintNamePattern(p.Name) {
		return rb.M("name", p.Name, "type", p.Attrs.Type, "color", p.Color)
	}
	return nil
}

func saintNamePattern(name string) bool {
	l := strings.ToLower(name)
	for _, s := range []string{"são", "santa", "santo", "bem-aventurado"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// NewCalendarWith builds a calendar over already loaded celebrations.
func NewCalendarWith(year int, code string, book *BookCelebrations) *Calendar {
	c := NewCalendar(year, code)
	c.book = book
	return c
}
