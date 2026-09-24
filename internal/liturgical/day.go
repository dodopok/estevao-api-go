package liturgical

import (
	"strconv"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Occurrence ports Liturgical::CelebrationOccurrence.
type Occurrence struct {
	Key   string
	Name  string
	Date  Date
	Rank  int
	Color *string
	Attrs *CelebrationAttrs
}

// LegacyH ports CelebrationOccurrence#to_legacy_h.
func (o *Occurrence) LegacyH() *rb.Map {
	if o == nil {
		return nil
	}
	a := o.Attrs
	return rb.M(
		"name", a.Name,
		"rank", a.Rank,
		"color", a.Color,
		"id", a.ID,
		"type", a.Type,
		"description", a.Description,
		"description_year", a.DescriptionYear,
		"transferred", a.Transferred,
		"post_slug", a.PostSlug,
		"person_type", a.PersonType,
		"gender", a.Gender,
		"calculation_rule", a.CalculationRule,
	)
}

// H ports CelebrationOccurrence#to_h (compact).
func (o *Occurrence) H() *rb.Map {
	a := o.Attrs
	m := rb.M("key", o.Key, "name", o.Name, "date", o.Date.ISO(), "rank", o.Rank)
	if o.Color != nil {
		m.Set("color", *o.Color)
	}
	m.Set("id", a.ID)
	m.Set("type", a.Type)
	for _, kv := range []struct {
		k string
		v *string
	}{{"description", a.Description}, {"description_year", a.DescriptionYear}} {
		if kv.v != nil {
			m.Set(kv.k, *kv.v)
		}
	}
	m.Set("transferred", a.Transferred)
	if a.PostSlug != nil {
		m.Set("post_slug", *a.PostSlug)
	}
	m.Set("person_type", a.PersonType)
	m.Set("gender", a.Gender)
	if a.CalculationRule != nil {
		m.Set("calculation_rule", *a.CalculationRule)
	}
	return m
}

// DayContext ports Liturgical::DayContext.
type DayContext struct {
	Date           Date
	PrayerBookCode string
	LiturgicalYear int
	Cycle          string
	Season         string
	BookSeason     string // "" == nil
	Week           int
	HasWeek        bool
	Movable        Movable
	Primary        *Occurrence
	Commemorations []*Occurrence
	Transfers      []Transfer
	Color          string
	Proper         int
	HasProper      bool
	Sunday         bool
	HolyDay        bool
	FastObservance *FastObservance
	// explanation
	CandidateIDs []int64
	SelectedID   *int64
	Precedence   string
	ColorSource  string
	ColorReason  string
	ColorMeaning string
}

// Celebrations returns [primary, *commemorations] without nils.
func (c *DayContext) Celebrations() []*Occurrence {
	var out []*Occurrence
	if c.Primary != nil {
		out = append(out, c.Primary)
	}
	return append(out, c.Commemorations...)
}

func (c *DayContext) FastDay() bool { return c.FastObservance != nil }

// CycleForYear ports LectionaryReading.cycle_for_year.
func CycleForYear(year int) string {
	switch mod(year, 3) {
	case 0:
		return "C"
	case 1:
		return "A"
	}
	return "B"
}

// dayResolver ports Liturgical::DayResolver.
type dayResolver struct {
	year   int
	code   string
	easter *Easter
	rules  *RuleSet
	season *SeasonDeterminator
	celebs *CelebrationResolver
	bookSR *BookSeasonResolver
}

func newDayResolver(year int, code string, easter *Easter, book *BookCelebrations) *dayResolver {
	rules := RulesFor(code)
	return &dayResolver{
		year:   year,
		code:   code,
		easter: easter,
		rules:  rules,
		season: NewSeasonDeterminator(year, easter, code),
		celebs: NewCelebrationResolver(year, code, easter, book),
	}
}

func (r *dayResolver) attributes(c *Celebration, date Date) *CelebrationAttrs {
	if c == nil {
		return nil
	}
	return &CelebrationAttrs{
		ID:              c.ID,
		Name:            c.Name,
		Type:            c.CelebrationType,
		Rank:            c.Rank,
		Color:           c.LiturgicalColor,
		Description:     c.Description,
		DescriptionYear: c.DescriptionYear,
		Transferred:     r.transferred(c, date),
		PostSlug:        c.PostSlugPtr(),
		PersonType:      c.PersonType,
		Gender:          c.Gender,
		CalculationRule: c.CalculationRule,
	}
}

func (r *dayResolver) transferred(c *Celebration, date Date) bool {
	if c.Movable {
		return false
	}
	nominal, ok := c.FixedDateIn(r.year)
	if !ok {
		panic(invalidDatePanic{c})
	}
	return nominal != date
}

func (r *dayResolver) occurrence(a *CelebrationAttrs, date Date) *Occurrence {
	if a == nil {
		return nil
	}
	return &Occurrence{Key: celebrationKey(a), Name: a.Name, Date: date, Rank: a.Rank, Color: a.Color, Attrs: a}
}

func celebrationKey(a *CelebrationAttrs) string {
	if a.PostSlug != nil && !blank(*a.PostSlug) {
		return *a.PostSlug
	}
	if a.CalculationRule != nil && !blank(*a.CalculationRule) {
		return *a.CalculationRule
	}
	return strconv.FormatInt(a.ID, 10)
}

func (r *dayResolver) resolve(date Date) *DayContext {
	res := r.celebs.ResolveDay(date)
	cands := make([]*CelebrationAttrs, len(res.Candidates))
	for i, c := range res.Candidates {
		cands[i] = r.attributes(c, date)
	}
	primary := r.attributes(res.Primary, date)
	var next *CelebrationAttrs
	if r.rules.FastObservanceRequiresNextDay() {
		nd := date.Add(1)
		next = r.attributes(r.celebs.ResolveDay(nd).Primary, nd)
	}
	fast := r.rules.FastObservance(date, r.easter.Dates, primary, next)
	season := r.season.SeasonFor(date)
	color := ColorFor(r.season, r.easter, r.rules, date, primary)
	source := "season"
	if principalCelebration(primary) || (!date.IsSunday() && primary.ColorPresent()) {
		source = "celebration"
	}
	reason := "season_color"
	switch {
	case principalCelebration(primary):
		reason = "principal_celebration_overrides_season"
	case date.IsSunday():
		reason = "sunday_uses_season_color"
	case primary.ColorPresent():
		reason = "weekday_celebration_color"
	}
	meaning := ExplainColor(date, season, color, source, primary, r.easter.Dates)
	proper, hasProper := ProperNumber(r.year, r.easter, date, season)
	week, hasWeek := r.season.WeekNumber(date)

	liturgicalYear := date.Year()
	if date >= r.easter.Dates["first_sunday_of_advent"] {
		liturgicalYear++
	}
	bookSeason := ""
	if r.rules.BookSeasons() {
		if r.bookSR == nil {
			r.bookSR = NewBookSeasonResolver(r.easter.Dates)
		}
		bookSeason = r.bookSR.Resolve(date)
	}
	var comms []*Occurrence
	for _, c := range cands {
		if primary != nil && c.ID == primary.ID {
			continue
		}
		comms = append(comms, r.occurrence(c, date))
	}
	ids := make([]int64, len(res.Candidates))
	for i, c := range res.Candidates {
		ids[i] = c.ID
	}
	var selected *int64
	if res.Primary != nil {
		id := res.Primary.ID
		selected = &id
	}
	holy := primary != nil && (primary.Type == "principal_feast" || primary.Type == "major_holy_day")
	return &DayContext{
		Date:           date,
		PrayerBookCode: r.code,
		LiturgicalYear: liturgicalYear,
		Cycle:          CycleForYear(liturgicalYear),
		Season:         season,
		BookSeason:     bookSeason,
		Week:           week,
		HasWeek:        hasWeek,
		Movable:        r.easter.Dates,
		Primary:        r.occurrence(primary, date),
		Commemorations: comms,
		Transfers:      res.Transfers,
		Color:          color,
		Proper:         proper,
		HasProper:      hasProper,
		Sunday:         date.IsSunday(),
		HolyDay:        holy,
		FastObservance: fast,
		CandidateIDs:   ids,
		SelectedID:     selected,
		Precedence:     res.Precedence,
		ColorSource:    source,
		ColorReason:    reason,
		ColorMeaning:   meaning,
	}
}
