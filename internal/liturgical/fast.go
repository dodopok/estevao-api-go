package liturgical

import "github.com/dodopok/estevao-api-go/internal/civil"

// FastObservance is the fasting classification of a day.
type FastObservance struct {
	Kind, Status, Reason string
}

type fastResolver struct {
	rules   []FastRule
	movable Movable
}

func (f *fastResolver) resolve(date Date, c, next *CelebrationAttrs) *FastObservance {
	for i := range f.rules {
		rule := &f.rules[i]
		if f.applies(rule, date, next) && !f.excluded(rule.Except, date, c) {
			reason := rule.Reason
			if reason == "" {
				reason = rule.On
			}
			return &FastObservance{Kind: rule.Kind, Status: rule.Status, Reason: reason}
		}
	}
	return nil
}

func (f *fastResolver) applies(rule *FastRule, date Date, next *CelebrationAttrs) bool {
	m := f.movable
	switch rule.On {
	case "principal_feast_vigil":
		if next == nil || date.IsSunday() {
			return false
		}
		allowed := rule.CelebrationTypes
		if allowed == nil {
			allowed = []string{"principal_feast"}
		}
		return contains(allowed, next.Type)
	case "listed_vigil":
		return containsDate(f.listedVigils(rule, date.Year()), date)
	case "ash_wednesday":
		return date == m["ash_wednesday"]
	case "good_friday":
		return date == m["good_friday"]
	case "lenten_weekday":
		return f.lentenWeekday(date)
	case "wednesday":
		return date.IsWednesday()
	case "friday":
		return date.IsFriday()
	case "ember_day":
		return containsDate(f.emberDays(date.Year(), rule.EmberDaySets), date)
	case "rogation_day":
		a := m["ascension"]
		return containsDate([]Date{a.Add(-3), a.Add(-2), a.Add(-1)}, date)
	case "holy_cross_day":
		return date == civil.MustNew(date.Year(), 9, 14)
	}
	return false
}

func (f *fastResolver) lentenWeekday(date Date) bool {
	return date.Between(f.movable["ash_wednesday"], f.movable["holy_saturday"]) && !date.IsSunday()
}

func (f *fastResolver) emberDays(year int, sets []EmberSet) []Date {
	if len(sets) == 0 {
		sets = []EmberSet{
			{Anchor: "first_sunday_in_lent", AnchorType: "sunday"},
			{Anchor: "pentecost", AnchorType: "sunday"},
			{Anchor: "holy_cross_day", AnchorType: "fixed", Month: 9, Day: 14},
			{Anchor: "winter", AnchorType: "fixed", Month: 12, Day: 13},
		}
	}
	var out []Date
	for _, s := range sets {
		out = append(out, emberDatesForSet(s, year, f.movable)...)
	}
	return uniqDates(out)
}

func (f *fastResolver) listedVigils(rule *FastRule, year int) []Date {
	var targets []Date
	for _, md := range rule.FixedDates {
		targets = append(targets, civil.MustNew(year, md[0], md[1]))
	}
	for _, k := range rule.MovableDates {
		if d, ok := f.movable[k]; ok {
			targets = append(targets, d)
		}
	}
	out := make([]Date, 0, len(targets))
	for _, t := range targets {
		if rule.MoveMondayToSaturday && t.IsMonday() {
			out = append(out, t.Add(-2))
		} else {
			out = append(out, t.Add(-1))
		}
	}
	return out
}

func (f *fastResolver) excluded(ex *FastExcept, date Date, c *CelebrationAttrs) bool {
	if ex == nil {
		return false
	}
	for _, r := range ex.DateRanges {
		if lo, hi, ok := f.exclusionRange(r, date); ok && date.Between(lo, hi) {
			return true
		}
	}
	ctype := ""
	if c != nil {
		ctype = c.Type
	}
	if contains(ex.CelebrationTypes, ctype) {
		return true
	}
	if contains(ex.CelebrationTypesOutsideLent, ctype) && !f.lentenWeekday(date) {
		return true
	}
	return contains(ex.CelebrationKeys, fastCelebrationKey(c))
}

func (f *fastResolver) exclusionRange(name string, date Date) (Date, Date, bool) {
	year := date.Year()
	if date.Month() == 1 {
		year--
	}
	christmas := civil.MustNew(year, 12, 25)
	easter := f.movable["easter"]
	switch name {
	case "christmas_day":
		return christmas, christmas, true
	case "epiphany":
		return christmas.Add(12), christmas.Add(12), true
	case "twelve_days_of_christmas":
		return christmas, christmas.Add(11), true
	case "christmas_to_epiphany":
		return christmas, christmas.Add(12), true
	case "week_after_christmas":
		return christmas, christmas.Add(7), true
	case "fifty_days_of_easter":
		return easter, f.movable["pentecost"], true
	case "week_after_easter":
		return easter, easter.Add(7), true
	case "week_after_ascension":
		a := f.movable["ascension"]
		return a, a.Add(7), true
	}
	return 0, 0, false
}

func fastCelebrationKey(c *CelebrationAttrs) string {
	if c == nil {
		return ""
	}
	if c.PostSlug != nil && !blank(*c.PostSlug) {
		return *c.PostSlug
	}
	if c.CalculationRule != nil && !blank(*c.CalculationRule) {
		return *c.CalculationRule
	}
	return ""
}
