package liturgical

import (
	"github.com/dodopok/estevao-api-go/internal/civil"
)

// CalculationRuleToMovable maps celebrations.calculation_rule to a movable key.
var CalculationRuleToMovable = map[string]string{
	"easter": "easter", "easter_minus_46_days": "ash_wednesday", "ash_wednesday": "ash_wednesday",
	"easter_minus_7_days": "palm_sunday", "palm_sunday": "palm_sunday",
	"easter_minus_6_days": "holy_monday", "holy_monday": "holy_monday",
	"easter_minus_5_days": "holy_tuesday", "holy_tuesday": "holy_tuesday",
	"easter_minus_4_days": "holy_wednesday", "holy_wednesday": "holy_wednesday",
	"easter_minus_3_days": "maundy_thursday", "maundy_thursday": "maundy_thursday",
	"easter_minus_2_days": "good_friday", "good_friday": "good_friday",
	"easter_minus_1_day": "holy_saturday", "easter_minus_1_days": "holy_saturday", "holy_saturday": "holy_saturday",
	"easter_plus_39_days": "ascension", "ascension": "ascension",
	"sunday_after_ascension": "sunday_after_ascension",
	"easter_plus_49_days":    "pentecost", "pentecost": "pentecost",
	"easter_plus_56_days": "trinity_sunday", "trinity_sunday": "trinity_sunday",
	"first_sunday_after_pentecost": "trinity_sunday",
	"first_sunday_after_epiphany":  "baptism_of_the_lord",
	"sunday_before_advent":         "christ_the_king",
	"first_sunday_of_advent":       "first_sunday_of_advent",
	"easter_monday":                "easter_monday", "easter_tuesday": "easter_tuesday",
	"whitsun_monday": "whitsun_monday", "whitsun_tuesday": "whitsun_tuesday",
	"corpus_christi": "corpus_christi", "patronage_of_saint_joseph": "patronage_of_saint_joseph",
	"last_sunday_of_october": "last_sunday_of_october", "sacred_heart": "sacred_heart",
	"immaculate_heart": "immaculate_heart", "septuagesima": "septuagesima", "sexagesima": "sexagesima",
	"quinquagesima": "quinquagesima", "rogation_sunday": "rogation_sunday", "thanksgiving": "thanksgiving",
	"thanksgiving_first_thursday": "thanksgiving_first_thursday", "thanksgiving_brazil": "thanksgiving",
	"fourth_thursday_of_november": "thanksgiving",
}

// CalculationRuleToDateReferences maps a calculation rule to lectionary
// date_reference names.
var CalculationRuleToDateReferences = map[string][]string{
	"easter_minus_46_days": {"ash_wednesday"}, "ash_wednesday": {"ash_wednesday"},
	"easter_minus_7_days": {"palm_sunday_palms", "palm_sunday_word", "palm_sunday"},
	"palm_sunday":         {"palm_sunday_palms", "palm_sunday_word", "palm_sunday"},
	"easter_minus_6_days": {"holy_monday", "monday_holy_week"}, "holy_monday": {"holy_monday", "monday_holy_week"},
	"easter_minus_5_days": {"holy_tuesday", "tuesday_holy_week"}, "holy_tuesday": {"holy_tuesday", "tuesday_holy_week"},
	"easter_minus_4_days": {"holy_wednesday", "wednesday_holy_week"}, "holy_wednesday": {"holy_wednesday", "wednesday_holy_week"},
	"easter_minus_3_days": {"maundy_thursday", "holy_thursday"}, "maundy_thursday": {"maundy_thursday", "holy_thursday"},
	"easter_minus_2_days": {"good_friday"}, "good_friday": {"good_friday"},
	"easter_minus_1_days": {"holy_saturday", "holy_saturday_vigil"}, "easter_minus_1_day": {"holy_saturday", "holy_saturday_vigil"},
	"holy_saturday":       {"holy_saturday", "holy_saturday_vigil"},
	"easter":              {"easter_principal_office", "easter_first_office", "easter_sunday", "easter_day", "easter"},
	"easter_plus_39_days": {"ascension", "ascension_day"}, "ascension": {"ascension", "ascension_day"},
	"sunday_after_ascension":       {"sunday_after_ascension", "7th_sunday_of_easter"},
	"corpus_christi":               {"corpus_christi"},
	"patronage_of_saint_joseph":    {"patronage_of_saint_joseph"},
	"last_sunday_of_october":       {"christ_the_king"},
	"sacred_heart":                 {"sacred_heart"},
	"immaculate_heart":             {"immaculate_heart"},
	"easter_plus_49_days":          {"day_of_pentecost", "pentecost_day", "pentecost", "whitsunday", "pentecost_sunday"},
	"pentecost":                    {"day_of_pentecost", "pentecost_day", "pentecost", "whitsunday", "pentecost_sunday"},
	"rogation_sunday":              {"rogation_sunday", "5th_sunday_after_easter"},
	"easter_plus_56_days":          {"trinity_sunday", "trinity"},
	"trinity_sunday":               {"trinity_sunday", "trinity"},
	"first_sunday_after_pentecost": {"trinity_sunday", "trinity"},
	"first_sunday_after_epiphany":  {"baptism_of_the_lord", "baptism_of_christ"},
	"sunday_before_advent":         {"christ_the_king"},
	"first_sunday_of_advent":       {"1st_sunday_of_advent", "advent_sunday"},
	"easter_monday":                {"monday_in_easter_week", "easter_monday"},
	"easter_tuesday":               {"tuesday_in_easter_week", "easter_tuesday"},
	"whitsun_monday":               {"monday_in_whitsun_week", "whitsun_monday"},
	"whitsun_tuesday":              {"tuesday_in_whitsun_week", "whitsun_tuesday"},
	"thanksgiving":                 {"thanksgiving_day", "thanksgiving"},
	"thanksgiving_first_thursday":  {"thanksgiving_day", "thanksgiving"},
	"thanksgiving_brazil":          {"thanksgiving_day", "thanksgiving"},
	"fourth_thursday_of_november":  {"thanksgiving_day", "thanksgiving"},
}

// BookCelebrations are one book's celebrations in database order.
type BookCelebrations struct {
	All []*Celebration
}

// Fixed returns movable=false celebrations in order.
func (b *BookCelebrations) Fixed() []*Celebration {
	var out []*Celebration
	for _, c := range b.All {
		if !c.Movable {
			out = append(out, c)
		}
	}
	return out
}

func (b *BookCelebrations) MovableList() []*Celebration {
	var out []*Celebration
	for _, c := range b.All {
		if c.Movable {
			out = append(out, c)
		}
	}
	return out
}

// CelebrationResolution is the result of resolving one date.
type CelebrationResolution struct {
	Primary    *Celebration
	Candidates []*Celebration
	Transfers  []Transfer
	Precedence string
}

// Transfer explains why a candidate is observed away from its nominal date.
type Transfer struct {
	CelebrationID int64
	From, To      Date
	Reason        string
}

type mdKey struct{ m, d int }

// CelebrationResolver resolves which celebrations fall on each date of a
// year. Port of Liturgical::CelebrationResolver.
type CelebrationResolver struct {
	year   int
	easter *Easter
	rules  *RuleSet
	season *SeasonDeterminator
	book   *BookCelebrations
	actual map[*Celebration]actualDate

	fixedByDate    map[mdKey][]*Celebration
	fixedKeysOrder []mdKey
	movable        []*Celebration
	movableByDate  map[Date][]*Celebration
	movableDates   []Date
	transferable   []*Celebration
	corpusObserved *bool
}

type actualDate struct {
	date Date
	ok   bool
}

func NewCelebrationResolver(year int, code string, easter *Easter, book *BookCelebrations) *CelebrationResolver {
	rules := RulesFor(code)
	if easter == nil {
		easter = EasterFor(year, rules.Paschalion())
	}
	r := &CelebrationResolver{
		year:   year,
		easter: easter,
		rules:  rules,
		season: NewSeasonDeterminator(year, easter, code),
		book:   book,
		actual: map[*Celebration]actualDate{},
	}
	r.index()
	return r
}

func (r *CelebrationResolver) index() {
	r.fixedByDate = map[mdKey][]*Celebration{}
	r.movableByDate = map[Date][]*Celebration{}
	if r.book == nil {
		return
	}
	for _, c := range r.book.All {
		if c.Movable {
			continue
		}
		var k mdKey
		if c.FixedMonth != nil {
			k.m = *c.FixedMonth
		} else {
			k.m = -1
		}
		if c.FixedDay != nil {
			k.d = *c.FixedDay
		} else {
			k.d = -1
		}
		if _, ok := r.fixedByDate[k]; !ok {
			r.fixedKeysOrder = append(r.fixedKeysOrder, k)
		}
		r.fixedByDate[k] = append(r.fixedByDate[k], c)
		if c.CanBeTransferred {
			r.transferable = append(r.transferable, c)
		}
	}
	for _, c := range r.book.All {
		if !c.Movable {
			continue
		}
		r.movable = append(r.movable, c)
		if d, ok := r.calculateMovableDate(c); ok {
			if _, seen := r.movableByDate[d]; !seen {
				r.movableDates = append(r.movableDates, d)
			}
			r.movableByDate[d] = append(r.movableByDate[d], c)
		}
	}
}

// ResolveForDate returns the observed celebration or nil.
func (r *CelebrationResolver) ResolveForDate(date Date) *Celebration {
	cands := r.collectCandidates(date)
	if len(cands) == 0 {
		return nil
	}
	if len(cands) == 1 {
		return cands[0]
	}
	return r.resolveByHierarchy(cands, date)
}

// ResolveAllForDate returns all candidates ordered by rank.
func (r *CelebrationResolver) ResolveAllForDate(date Date) []*Celebration {
	return sortByRank(r.collectCandidates(date))
}

// ResolveDay resolves candidates once and records the decision trace.
func (r *CelebrationResolver) ResolveDay(date Date) CelebrationResolution {
	cands := sortByRank(r.collectCandidates(date))
	var primary *Celebration
	if len(cands) > 0 {
		primary = r.resolveByHierarchy(cands, date)
	}
	return CelebrationResolution{
		Primary:    primary,
		Candidates: cands,
		Transfers:  r.transferExplanations(cands, date),
		Precedence: r.precedenceReason(primary, cands, date),
	}
}

// ActualDateFor returns where a celebration is observed this year.
func (r *CelebrationResolver) ActualDateFor(c *Celebration) (Date, bool) {
	if v, ok := r.actual[c]; ok {
		return v.date, v.ok
	}
	d, ok := r.calculateActualDate(c)
	r.actual[c] = actualDate{d, ok}
	return d, ok
}

func (r *CelebrationResolver) calculateActualDate(c *Celebration) (Date, bool) {
	if c.Movable {
		return r.calculateMovableDate(c)
	}
	if !c.HasFixedDate() {
		return 0, false
	}
	original, ok := civil.New(r.year, *c.FixedMonth, *c.FixedDay)
	if !ok {
		panic(invalidDatePanic{c})
	}
	if !c.CanBeTransferred {
		return original, true
	}
	return r.transferIfNeeded(c, original), true
}

type invalidDatePanic struct{ c *Celebration }

func (r *CelebrationResolver) transferExplanations(cands []*Celebration, observed Date) []Transfer {
	var out []Transfer
	for _, c := range cands {
		if c.Movable || !c.HasFixedDate() {
			continue
		}
		original, ok := civil.New(r.year, *c.FixedMonth, *c.FixedDay)
		if !ok || original == observed {
			continue
		}
		out = append(out, Transfer{CelebrationID: c.ID, From: original, To: observed, Reason: r.transferReason(c, original)})
	}
	return out
}

func (r *CelebrationResolver) transferReason(c *Celebration, original Date) string {
	if c.PostSlug == "celebration-annunciation" && r.season.InProtectedPeriod(original) {
		return "annunciation_protected_period"
	}
	if r.season.InProtectedPeriod(original) {
		return "protected_period"
	}
	if original.IsSunday() {
		return "sunday_conflict"
	}
	if c.PostSlug == "celebration-all-saints" {
		return "all_saints_observance"
	}
	return "calendar_conflict"
}

func (r *CelebrationResolver) precedenceReason(primary *Celebration, cands []*Celebration, date Date) string {
	switch {
	case primary == nil:
		return "no_celebration"
	case len(cands) == 1:
		return "only_candidate"
	case primary.IsPrincipalFeast():
		return "principal_feast"
	case primary.IsMajorHolyDay():
		return "major_holy_day"
	case date.IsSunday() && r.season.InMajorSeason(date):
		return "sunday_in_major_season"
	}
	return "lowest_rank"
}

func (r *CelebrationResolver) collectCandidates(date Date) []*Celebration {
	var cands []*Celebration
	for _, c := range r.fixedByDate[mdKey{date.Month(), date.Day()}] {
		if !r.explicitlyDisplaced(c, date) {
			cands = append(cands, c)
		}
	}
	cands = append(cands, r.movableByDate[date]...)
	for _, c := range r.transferable {
		if c.Movable {
			continue
		}
		td, ok := r.ActualDateFor(c)
		if !ok {
			continue
		}
		nominal, _ := civil.New(r.year, *c.FixedMonth, *c.FixedDay)
		if td == date && td != nominal {
			cands = append(cands, c)
		}
	}
	return cands
}

func (r *CelebrationResolver) resolveByHierarchy(cs []*Celebration, date Date) *Celebration {
	sorted := sortByRank(cs)
	for _, c := range sorted {
		if c.IsPrincipalFeast() {
			return c
		}
	}
	for _, c := range sorted {
		if c.IsMajorHolyDay() {
			return c
		}
	}
	return sorted[0]
}

func (r *CelebrationResolver) calculateMovableDate(c *Celebration) (Date, bool) {
	if !c.Movable {
		return 0, false
	}
	key, ok := CalculationRuleToMovable[c.Rule()]
	if !ok {
		return 0, false
	}
	d, ok := r.easter.Dates[key]
	return d, ok
}

func (r *CelebrationResolver) transferIfNeeded(c *Celebration, original Date) Date {
	return transferIfNeeded(r.year, r.easter, r.rules, c, original, r.corpusChristiObserved(), r.occupiedDatesFor(c))
}

func (r *CelebrationResolver) occupiedDatesFor(c *Celebration) []Date {
	var dates []Date
	for _, k := range r.fixedKeysOrder {
		cs := r.fixedByDate[k]
		all := true
		for _, x := range cs {
			if x.ID != c.ID {
				all = false
				break
			}
		}
		if all {
			continue
		}
		if d, ok := civil.New(r.year, k.m, k.d); ok {
			dates = append(dates, d)
		} else {
			panic(invalidDatePanic{cs[0]})
		}
	}
	dates = append(dates, r.movableDates...)
	return uniqDates(dates)
}

func (r *CelebrationResolver) explicitlyDisplaced(c *Celebration, date Date) bool {
	actual, ok := r.ActualDateFor(c)
	if !ok || actual == date {
		return false
	}
	return c.PostSlug == "celebration-annunciation" || r.corpusChristiConflict(c, date) || r.rules.DisplaceTransferredNominalDate()
}

func (r *CelebrationResolver) corpusChristiConflict(c *Celebration, date Date) bool {
	return r.corpusChristiObserved() && c.IsFestival() && date == r.easter.Dates["corpus_christi"]
}

func (r *CelebrationResolver) corpusChristiObserved() bool {
	if r.corpusObserved != nil {
		return *r.corpusObserved
	}
	v := false
	for _, c := range r.movable {
		if c.Rule() == "corpus_christi" {
			v = true
			break
		}
	}
	// Ruby memoizes with ||=, so a false result is recomputed each time;
	// the value cannot change, so caching it is equivalent.
	r.corpusObserved = &v
	return v
}

// transferIfNeeded ports Liturgical::TransferRules#transfer_if_needed.
func transferIfNeeded(year int, easter *Easter, rules *RuleSet, c *Celebration, original Date, corpus bool, occupied []Date) Date {
	m := easter.Dates
	if d, ok := rules.TransferDateFor(c, original, m, occupied); ok {
		return d
	}
	if c.PostSlug == "celebration-annunciation" {
		return transferAnnunciation(m, original)
	}
	if corpus && c.IsFestival() && original == m["corpus_christi"] {
		return original.Add(1)
	}
	if c.PostSlug == "celebration-joseph-of-nazareth" || c.PostSlug == "celebration-joseph" || c.PostSlug == "celebration-mark-the-evangelist" {
		return transferIfHolyWeek(year, m, original)
	}
	allSaints := c.PostSlug == "celebration-all-saints"
	if allSaints && rules.AllSaintsTransferMode() == "none" {
		return original
	}
	if (c.IsFestival() || c.IsMajorHolyDay()) && original.IsSunday() {
		return original.Add(1)
	}
	if allSaints {
		if original.IsSunday() {
			return original
		}
		start := civil.MustNew(year, 10, 30)
		end := civil.MustNew(year, 11, 5)
		for d := start; d <= end; d++ {
			if d.IsSunday() {
				return d
			}
		}
		return original
	}
	return original
}

func transferAnnunciation(m Movable, original Date) Date {
	palm, second := m["palm_sunday"], m["second_sunday_of_easter"]
	if original >= palm && original <= second {
		return second.Add(1)
	}
	if original.IsSunday() {
		return original.Add(1)
	}
	return original
}

func transferIfHolyWeek(year int, m Movable, original Date) Date {
	palm, second := m["palm_sunday"], m["second_sunday_of_easter"]
	if !(original >= palm && original <= second) {
		return original
	}
	monday := second.Add(1)
	if transferAnnunciation(m, civil.MustNew(year, 3, 25)) == monday {
		return second.Add(2)
	}
	return monday
}
