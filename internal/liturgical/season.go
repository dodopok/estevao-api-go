package liturgical

import "github.com/dodopok/estevao-api-go/internal/civil"

// Season names, stored and published in Portuguese.
const (
	Advent       = "Advento"
	Christmas    = "Natal"
	Epiphany     = "Epifania"
	Lent         = "Quaresma"
	EasterSeason = "Páscoa"
	OrdinaryTime = "Tempo Comum"
)

var AllSeasons = []string{Advent, Christmas, Epiphany, Lent, EasterSeason, OrdinaryTime}

var SeasonSlugs = map[string]string{
	Advent: "season-advent", Christmas: "season-christmas", Epiphany: "season-epiphany",
	Lent: "season-lent", EasterSeason: "season-easter", OrdinaryTime: "season-ordinary-time",
}

// CelebrationPrecedenceSeasons are seasons where a Sunday outranks a minor
// festival for the observed celebration.
var CelebrationPrecedenceSeasons = []string{Advent, Christmas, Lent, EasterSeason}

// ReadingPrecedenceSeasons includes Epiphany (see Liturgical::Season).
var ReadingPrecedenceSeasons = []string{Advent, Christmas, Epiphany, Lent, EasterSeason}

// SeasonDeterminator ports Liturgical::SeasonDeterminator.
type SeasonDeterminator struct {
	Year   int
	Easter *Easter
	Code   string
	Rules  *RuleSet
}

func NewSeasonDeterminator(year int, easter *Easter, code string) *SeasonDeterminator {
	rules := RulesFor(code)
	if easter == nil {
		easter = EasterFor(year, rules.Paschalion())
	}
	return &SeasonDeterminator{Year: year, Easter: easter, Code: code, Rules: rules}
}

func (s *SeasonDeterminator) SeasonFor(date Date) string {
	m := s.Easter.Dates
	epiStart := s.Rules.EpiphanyStart(m)
	if date.Month() == 1 && date < epiStart {
		return Christmas
	}
	if date >= m["first_sunday_of_advent"] && date <= civil.MustNew(s.Year, 12, 24) {
		return Advent
	}
	if date >= civil.MustNew(s.Year, 12, 25) {
		return Christmas
	}
	if date >= epiStart && date < m["ash_wednesday"] {
		return Epiphany
	}
	if date >= m["ash_wednesday"] && date <= m["holy_saturday"] {
		return Lent
	}
	if date >= m["easter"] && date <= m["pentecost"] {
		return EasterSeason
	}
	return OrdinaryTime
}

// SeasonRange is the inclusive span of a season.
type SeasonRange struct {
	Name       string
	Start, End Date
}

func (s *SeasonDeterminator) RangeFor(date Date) SeasonRange {
	m := s.Easter.Dates
	season := s.SeasonFor(date)
	var start, end Date
	switch season {
	case Christmas:
		if date.Month() == 1 {
			start = civil.MustNew(s.Year-1, 12, 25)
			end = s.Rules.EpiphanyStart(m).Add(-1)
		} else {
			next := EasterFor(s.Year+1, s.Easter.Paschalion).Dates
			start = civil.MustNew(s.Year, 12, 25)
			end = s.Rules.EpiphanyStart(next).Add(-1)
		}
	case Epiphany:
		start, end = s.Rules.EpiphanyStart(m), m["ash_wednesday"].Add(-1)
	case Lent:
		start, end = m["ash_wednesday"], m["holy_saturday"]
	case EasterSeason:
		start, end = m["easter"], m["pentecost"]
	case Advent:
		start, end = m["first_sunday_of_advent"], civil.MustNew(s.Year, 12, 24)
	default:
		start, end = m["pentecost"].Add(1), m["first_sunday_of_advent"].Add(-1)
	}
	return SeasonRange{Name: season, Start: start, End: end}
}

func (s *SeasonDeterminator) InMajorSeason(date Date) bool {
	return contains(CelebrationPrecedenceSeasons, s.SeasonFor(date))
}

func (s *SeasonDeterminator) InProtectedPeriod(date Date) bool {
	m := s.Easter.Dates
	return date >= m["palm_sunday"] && date <= m["second_sunday_of_easter"]
}

func (s *SeasonDeterminator) SeasonSlugFor(date Date) string {
	return SeasonSlugs[s.SeasonFor(date)]
}

// WeekNumber ports Liturgical::WeekCalculator#week_number (0,false == nil).
func (s *SeasonDeterminator) WeekNumber(date Date) (int, bool) {
	m := s.Easter.Dates
	weeksSince := func(start Date) int { return floorDiv(date.Sub(start), 7) + 1 }
	switch s.SeasonFor(date) {
	case Advent:
		return weeksSince(m["first_sunday_of_advent"]), true
	case Christmas:
		christmas := civil.MustNew(date.Year()-1, 12, 25)
		if date.Month() == 12 {
			christmas = civil.MustNew(date.Year(), 12, 25)
		}
		return weeksSince(christmas), true
	case Epiphany:
		return weeksSince(s.Rules.EpiphanyWeekStart(m)), true
	case Lent:
		return weeksSince(m["first_sunday_in_lent"]), true
	case EasterSeason:
		return weeksSince(m["easter"]), true
	case OrdinaryTime:
		if date < m["ash_wednesday"] {
			b := m["baptism_of_the_lord"]
			first := b
			if !b.IsSunday() {
				first = b.Add(7 - b.Weekday())
			}
			return weeksSince(first), true
		}
		return weeksSince(m["trinity_sunday"].Add(7)), true
	}
	return 0, false
}

// SundayAfterPentecost ports WeekCalculator#sunday_after_pentecost.
func (s *SeasonDeterminator) SundayAfterPentecost(date Date) (int, bool) {
	if !date.IsSunday() || s.SeasonFor(date) != OrdinaryTime {
		return 0, false
	}
	p := s.Easter.Dates["pentecost"]
	if date < p {
		return 0, false
	}
	w := floorDiv(date.Sub(p), 7)
	if w >= 0 {
		return w, true
	}
	return 0, false
}

var properTargetDates = []struct {
	n, m, d int
}{
	{29, 11, 23}, {28, 11, 16}, {27, 11, 9}, {26, 11, 2}, {25, 10, 26}, {24, 10, 19}, {23, 10, 12},
	{22, 10, 5}, {21, 9, 28}, {20, 9, 21}, {19, 9, 14}, {18, 9, 7}, {17, 8, 31}, {16, 8, 24},
	{15, 8, 17}, {14, 8, 10}, {13, 8, 3}, {12, 7, 27}, {11, 7, 20}, {10, 7, 13}, {9, 7, 6},
	{8, 6, 29}, {7, 6, 22}, {6, 6, 15}, {5, 6, 8}, {4, 6, 1}, {3, 5, 25}, {2, 5, 18}, {1, 5, 11},
}

// ProperNumber ports ProperCalculator#calculate (0,false == nil). year is
// the calculator's year; season is required.
func ProperNumber(year int, easter *Easter, date Date, season string) (int, bool) {
	if season != OrdinaryTime {
		return 0, false
	}
	m := easter.Dates
	target := date
	if !date.IsSunday() {
		target = date.Add(-date.Weekday())
	}
	if date < m["ash_wednesday"] {
		b := m["baptism_of_the_lord"]
		first := b
		if !b.IsSunday() {
			first = b.Add(7 - b.Weekday())
		}
		if target >= first {
			return floorDiv(target.Sub(first), 7) + 1, true
		}
		return 0, false
	}
	for _, p := range properTargetDates {
		ref := civil.MustNew(year, p.m, p.d)
		w := ref.Weekday()
		var sunday Date
		switch {
		case w == 0:
			sunday = ref
		case w <= 3:
			sunday = ref.Add(-w)
		default:
			sunday = ref.Add(7 - w)
		}
		if target == sunday {
			return p.n, true
		}
	}
	return 0, false
}
