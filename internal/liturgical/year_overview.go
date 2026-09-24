package liturgical

import (
	"sort"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// YearOverview ports Liturgical::YearOverviewService.
type YearOverview struct {
	Year   int
	Code   string
	Rules  *RuleSet
	Easter *Easter
	book   *BookCelebrations
	exists bool
	res    *CelebrationResolver
	cal    *Calendar
	db     []*rb.Map
	all    []*rb.Map
}

// ValidCelebrationTypes are the accepted ?type= filters.
var ValidCelebrationTypes = append(append([]string{}, CelebrationTypeOrder...), "sunday")

// NewYearOverview builds the overview. bookExists reports whether the
// prayer book exists (db_celebrations is [] otherwise).
func NewYearOverview(year int, code string, book *BookCelebrations, bookExists bool) *YearOverview {
	rules := RulesFor(code)
	easter := EasterFor(year, rules.Paschalion())
	return &YearOverview{Year: year, Code: code, Rules: rules, Easter: easter, book: book, exists: bookExists,
		res: NewCelebrationResolver(year, code, easter, book)}
}

func (y *YearOverview) calendar() *Calendar {
	if y.cal == nil {
		y.cal = NewCalendar(y.Year, y.Code)
		y.cal.book = y.book
	}
	return y.cal
}

// Call ports #call.
func (y *YearOverview) Call() *rb.Map {
	return rb.M(
		"year", y.Year,
		"prayer_book", y.Code,
		"liturgical_year", y.LiturgicalYear(),
		"seasons", y.Seasons(),
		"key_dates", y.KeyDates(),
		"celebrations", y.Celebrations("", false),
		"celebrations_by_type", y.Celebrations("", true),
	)
}

func (y *YearOverview) LiturgicalYear() string {
	return y.calendar().LiturgicalYearCycle(civil.MustNew(y.Year, 7, 1))
}

func (y *YearOverview) KeyDates() *rb.Map {
	keys := []string{"baptism_of_the_lord", "ash_wednesday", "palm_sunday", "maundy_thursday", "good_friday",
		"holy_saturday", "easter", "ascension", "pentecost", "trinity_sunday", "christ_the_king", "first_sunday_of_advent"}
	if !y.Rules.ObservesBaptism() {
		keys = keys[1:]
	}
	byDate := y.celebrationsByDate()
	out := rb.NewMap()
	for _, k := range keys {
		d := y.Easter.Dates[k].ISO()
		entry := rb.M("date", d)
		if cel, ok := byDate[d]; ok {
			entry.Set("name", cel.Get("name"))
			entry.Set("post_slug", cel.Get("post_slug"))
		}
		out.Set(k, entry)
	}
	return out
}

func (y *YearOverview) celebrationsByDate() map[string]*rb.Map {
	out := map[string]*rb.Map{}
	for _, c := range y.dbCelebrations() {
		d := rb.ToS(c.Get("date"))
		existing, ok := out[d]
		if !ok || CelebrationTypeValues[rb.ToS(c.Get("type"))] < CelebrationTypeValues[rb.ToS(existing.Get("type"))] {
			out[d] = c
		}
	}
	return out
}

func buildSeason(name string, start, end Date) *rb.Map {
	return rb.M("name", name, "slug", SeasonSlugs[name], "start_date", start.ISO(), "end_date", end.ISO())
}

func (y *YearOverview) Seasons() []any {
	m := y.Easter.Dates
	epi := y.Rules.EpiphanyStart(m)
	return []any{
		buildSeason(Christmas, civil.MustNew(y.Year-1, 12, 25), epi.Add(-1)),
		buildSeason(Epiphany, epi, m["ash_wednesday"].Add(-1)),
		buildSeason(Lent, m["ash_wednesday"], m["holy_saturday"]),
		buildSeason(EasterSeason, m["easter"], m["pentecost"]),
		buildSeason(OrdinaryTime, m["pentecost"].Add(1), m["first_sunday_of_advent"].Add(-1)),
		buildSeason(Advent, m["first_sunday_of_advent"], civil.MustNew(y.Year, 12, 24)),
	}
}

// Celebrations ports #celebrations(type:, grouped:). grouped returns *rb.Map.
func (y *YearOverview) Celebrations(typ string, grouped bool) any {
	var result []*rb.Map
	for _, c := range y.allCelebrations() {
		if typ != "" && rb.ToS(c.Get("type")) != typ {
			continue
		}
		result = append(result, c)
	}
	if !grouped {
		out := make([]any, len(result))
		for i, c := range result {
			out[i] = c
		}
		return out
	}
	g := rb.NewMap()
	for _, c := range result {
		t := rb.ToS(c.Get("type"))
		list, _ := g.Get(t).([]any)
		g.Set(t, append(list, c))
	}
	return g
}

func (y *YearOverview) allCelebrations() []*rb.Map {
	if y.all != nil {
		return y.all
	}
	all := append(append([]*rb.Map{}, y.dbCelebrations()...), y.sundayEntries()...)
	sort.SliceStable(all, func(i, j int) bool { return rb.ToS(all[i].Get("date")) < rb.ToS(all[j].Get("date")) })
	y.all = all
	return all
}

func (y *YearOverview) dbCelebrations() []*rb.Map {
	if y.db != nil {
		return y.db
	}
	y.db = []*rb.Map{}
	if !y.exists {
		return y.db
	}
	for _, c := range y.book.All {
		date, ok := y.res.ActualDateFor(c)
		if !ok || date.Year() != y.Year {
			continue
		}
		if date.IsSunday() && !c.IsPrincipalFeast() && !c.IsMajorHolyDay() {
			date = date.Add(1)
			if date.Year() != y.Year {
				continue
			}
		}
		transferred := false
		if !c.Movable && c.HasFixedDate() {
			orig, ok := civil.New(y.Year, *c.FixedMonth, *c.FixedDay)
			if !ok {
				panic(invalidDatePanic{c})
			}
			transferred = date != orig
		}
		y.db = append(y.db, rb.M(
			"date", date.ISO(), "name", c.Name, "type", c.CelebrationType, "color", c.LiturgicalColor,
			"post_slug", c.PostSlugPtr(), "transferred", transferred,
		))
	}
	sort.SliceStable(y.db, func(i, j int) bool { return rb.ToS(y.db[i].Get("date")) < rb.ToS(y.db[j].Get("date")) })
	return y.db
}

func (y *YearOverview) sundayEntries() []*rb.Map {
	covered := map[string]bool{}
	for _, c := range y.dbCelebrations() {
		t := rb.ToS(c.Get("type"))
		if t == "principal_feast" || t == "major_holy_day" {
			covered[rb.ToS(c.Get("date"))] = true
		}
	}
	var out []*rb.Map
	sd := NewSeasonDeterminator(y.Year, y.Easter, y.Code)
	for d := civil.MustNew(y.Year, 1, 1); d <= civil.MustNew(y.Year, 12, 31); d++ {
		if !d.IsSunday() || covered[d.ISO()] {
			continue
		}
		name := y.calendar().SundayName(d)
		if name == "" {
			continue
		}
		out = append(out, rb.M("date", d.ISO(), "name", name, "type", "sunday",
			"color", ColorFor(sd, y.Easter, y.Rules, d, nil), "post_slug", nil, "transferred", false))
	}
	return out
}
