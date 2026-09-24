package reading

import (
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// --- Loc2015Service / Loc2027Service ------------------------------------------

func (r *Resolver) preparationDay() bool {
	wd := r.Date.Weekday()
	return wd == 4 || wd == 5 || wd == 6
}

func (r *Resolver) referenceSunday() Date {
	if r.preparationDay() {
		days := mod(7-r.Date.Weekday(), 7)
		if days == 0 {
			days = 7
		}
		return r.Date.Add(days)
	}
	return r.Date.Add(-r.Date.Weekday())
}

func (r *Resolver) monthDayName() string {
	return strings.ToLower(civil.MonthNamesEN[r.Date.Month()]) + "_" + fmt.Sprint(r.Date.Day())
}

func (r *Resolver) loc2015WeeklyReferences() []string {
	weekday := weekdayNames[r.Date.Weekday()]
	refs := []string{r.monthDayName()}
	sunday := r.referenceSunday()
	if n, ok := r.cal.ProperNumber(sunday); ok {
		refs = append(refs, fmt.Sprintf("proper_%d_%s", n, weekday))
	}
	if s := MapSunday(sunday, liturgical.NewCalendar(sunday.Year(), r.Code)); s != "" {
		refs = append(refs, s+"_"+weekday)
	}
	return append(refs, r.specialWeekReferencesFor(sunday, weekday, true)...)
}

func (r *Resolver) loc2027WeeklyReferences() []string {
	weekday := weekdayNames[r.Date.Weekday()]
	refs := []string{r.monthDayName()}
	sunday := r.referenceSunday()
	if s := MapSunday(sunday, liturgical.NewCalendar(sunday.Year(), r.Code)); s != "" {
		refs = append(refs, s+"_"+weekday)
	}
	if n, ok := r.cal.ProperNumber(sunday); ok {
		refs = append(refs, fmt.Sprintf("proper_%d_%s", n, weekday))
	}
	return append(refs, r.specialWeekReferencesFor(sunday, weekday, false)...)
}

// specialWeekReferencesFor ports build_special_week_references of the two
// preparation/reflection books; they differ only in the Epiphany block's
// guards, which are equivalent, so one port serves both.
func (r *Resolver) specialWeekReferencesFor(sunday Date, weekday string, _ bool) []string {
	var refs []string
	cal := liturgical.NewCalendar(sunday.Year(), r.Code)
	week, hasWeek := cal.WeekNumber(sunday)
	switch cal.SeasonFor(sunday) {
	case "Advento":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_advent_"+weekday)
		}
	case "Natal":
		refs = append(refs, "week_of_christmas_"+weekday, "first_sunday_after_christmas_"+weekday)
		if sunday.Month() == 1 {
			refs = append(refs, "baptism_of_christ_"+weekday, "week_of_epiphany_"+weekday)
		}
	case "Epifania":
		if hasWeek && week >= 1 {
			refs = append(refs, fmt.Sprintf("ordinary_time_%d_%s", week+1, weekday), fmt.Sprintf("ordinary_time_%d_%s", week, weekday))
		}
		refs = append(refs, "week_of_epiphany_"+weekday, "baptism_of_christ_"+weekday, "last_sunday_after_epiphany_"+weekday)
	case "Quaresma":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_lent_"+weekday)
		}
		refs = append(refs, "holy_week_"+weekday)
	case "Páscoa":
		if hasWeek {
			refs = append(refs, liturgical.Ordinalize(week)+"_sunday_of_easter_"+weekday)
		}
		refs = append(refs, "week_of_pentecost_"+weekday, "trinity_sunday_"+weekday)
	case "Tempo Comum":
		pentecost := liturgical.EasterFor(sunday.Year(), liturgical.Gregorian).Dates["pentecost"]
		trinity := pentecost.Add(7)
		if sunday > pentecost && sunday <= trinity {
			refs = append(refs, "week_of_pentecost_"+weekday, "trinity_sunday_"+weekday)
		}
		if sunday.Month() == 11 && sunday.Day() >= 20 && sunday.Day() <= 26 {
			refs = append(refs, "christ_the_king_"+weekday)
		}
	}
	return refs
}

func (r *Resolver) weeklyQuery2027() *Query {
	sunday := r.referenceSunday()
	r.referenceCycle = liturgical.NewCalendar(sunday.Year(), r.Code).LiturgicalYearCycle(sunday)
	if r.referenceCycle == r.Cycle {
		return r.Query()
	}
	return &Query{PrayerBookID: r.prayerBookID(), Cycle: r.referenceCycle, Date: r.Date, ReadingType: r.ReadingType, ReadingTypeRaw: r.readingTypeRaw, ServiceType: r.ServiceType}
}

// --- Loc1984WalesService ----------------------------------------------------------

func (r *Resolver) walesContentLoader() *ContentLoader {
	psalm := ""
	if r.Code == "loc_1984_en" {
		psalm = r.PsalmTranslation
	}
	return &ContentLoader{Translation: r.Translation, PsalmTranslation: psalm, Cache: r.cache}
}

func (r *Resolver) walesFindReadings() *Selection {
	sel := r.baseFindReadings()
	if sel == nil || (sel.FirstReading == nil && sel.SecondReading == nil) {
		if provision := WalesProvisionFor(r.Date, r.ServiceType, r.Cycle, r.cal); provision != nil {
			sel = r.walesSelectionFrom(provision)
			r.outcome = &Outcome{Rule: "daily_course", Source: "daily_cycle", Attempts: []Attempt{{Rule: "daily_course", Status: "selected"}}}
		}
	}
	if sel == nil {
		sel = &Selection{}
	}
	return r.walesSupplementPsalter(sel)
}

func (r *Resolver) walesSelectionFrom(p *rb.Map) *Selection {
	first, second, psalm := p.Get("first_reading"), p.Get("second_reading"), p.Get("psalm")
	contents := map[string]*rb.Map{}
	if r.LoadContent {
		var refs []string
		for _, v := range []any{psalm, first, second} {
			if v != nil {
				refs = append(refs, rb.ToS(v))
			}
		}
		contents = r.walesContentLoader().Load(r.ctx, refs)
	}
	notes := rb.M("source", "Church in Wales, Book of Common Prayer 1984, Volume I")
	if v := p.Get("table"); v != nil {
		notes.Set("table", v)
	}
	if v := p.Get("index"); v != nil {
		notes.Set("index", v)
	}
	notes.Set("cycle", r.Cycle)
	return &Selection{
		Psalm:         r.walesPassage(psalm, contents),
		FirstReading:  r.walesPassage(first, contents),
		SecondReading: r.walesPassage(second, contents),
		Notes:         notes,
	}
}

func (r *Resolver) walesPassage(v any, contents map[string]*rb.Map) *Passage {
	if v == nil || rb.Blank(v) {
		return nil
	}
	ref := rb.ToS(v)
	return r.SelectionBuilder().Presenter().Passage(ref, contents[ref])
}

func (r *Resolver) walesSupplementPsalter(sel *Selection) *Selection {
	if sel.Psalm != nil {
		return sel
	}
	ref := WalesPsalterReference(r.Date, r.ServiceType)
	if ref == "" {
		return sel
	}
	var content *rb.Map
	if r.LoadContent {
		content = r.walesContentLoader().Load(r.ctx, []string{ref})[ref]
	}
	out := *sel
	out.Psalm = r.SelectionBuilder().Presenter().Passage(ref, content)
	return &out
}
