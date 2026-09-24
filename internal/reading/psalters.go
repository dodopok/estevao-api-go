package reading

import (
	"fmt"
	"math"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

func init() {
	liturgical.DwdoPsalterReference = DwdoPsalterReference
	liturgical.ProperPsalmsReference = func(table string, date Date, serviceType string, movable liturgical.Movable) string {
		switch table {
		case "bcp_1662":
			return Loc1662ProperPsalm(date, serviceType, movable)
		case "bcp_1962_canada":
			return Loc1962ProperPsalm(date, serviceType, movable)
		}
		return ""
	}
}

func fetchDate(m liturgical.Movable, key string) Date {
	d, ok := m[key]
	if !ok {
		rb.RaiseKeyError(":" + key)
	}
	return d
}

func mapString(m *rb.Map, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m.Lookup(key)
	if !ok || v == nil {
		return ""
	}
	return rb.ToS(v)
}

func mapMap(m *rb.Map, key string) *rb.Map {
	if m == nil {
		return nil
	}
	v, _ := m.Get(key).(*rb.Map)
	return v
}

// --- Reading::DwdoPsalter --------------------------------------------------

// DwdoPsalterReference ports DwdoPsalter.reference_for ("" == nil).
func DwdoPsalterReference(date Date, serviceType string) string {
	day := date.Day()
	if day > 30 {
		day = 30
	}
	return mapString(dwdoCycle, fmt.Sprintf("%d,%s", day, serviceType))
}

// --- Reading::Loc1662EnProperPsalms ----------------------------------------

// Loc1662ProperPsalm ports Loc1662EnProperPsalms.reference_for.
func Loc1662ProperPsalm(date Date, serviceType string, movable liturgical.Movable) string {
	var key string
	if date.Month() == 12 && date.Day() == 25 {
		key = "christmas_day"
	} else {
		for _, k := range loc1662ProperAppointments.Keys() {
			if k == "christmas_day" {
				continue
			}
			if d, ok := movable[k]; ok && d == date {
				key = k
				break
			}
		}
	}
	if key == "" {
		return ""
	}
	return mapString(mapMap(loc1662ProperAppointments, key), serviceType)
}

// --- Reading::Loc1962EnProperPsalms ----------------------------------------

// Loc1962ProperPsalm ports Loc1962EnProperPsalms.reference_for.
func Loc1962ProperPsalm(date Date, serviceType string, movable liturgical.Movable) string {
	if serviceType != "morning_prayer" && serviceType != "evening_prayer" {
		return ""
	}
	if serviceType == "evening_prayer" {
		if v := mapString(loc1962FirstEvensong, fmt.Sprintf("%d,%d", date.Month(), date.Day())); v != "" {
			return v
		}
		for _, feast := range loc1962MovableFirstEvensong.Keys() {
			if date == fetchDate(movable, feast).Add(-1) {
				if v := mapString(loc1962MovableFirstEvensong, feast); v != "" {
					return v
				}
			}
		}
	}
	key := loc1962DayKey(date, movable)
	if key == "" {
		return ""
	}
	return mapString(mapMap(loc1962ProperAppointments, key), serviceType)
}

func loc1962DayKey(date Date, m liturgical.Movable) string {
	if k := mapString(loc1962FixedAppointments, fmt.Sprintf("%d,%d", date.Month(), date.Day())); k != "" {
		return k
	}
	advent := fetchDate(m, "first_sunday_of_advent")
	switch date {
	case advent:
		return "advent_1"
	case advent.Add(7):
		return "advent_2"
	case advent.Add(14):
		return "advent_3"
	case advent.Add(21):
		return "advent_4"
	}
	epiphany := civil.MustNew(date.Year(), 1, 6)
	if date == epiphany {
		return "epiphany"
	}
	epiphanySunday := firstSundayAfterStrict(epiphany)
	if date.Weekday() == 0 && date >= epiphanySunday && date <= epiphanySunday.Add(35) {
		n := date.Sub(epiphanySunday)/7 + 1
		if n >= 1 && n <= 6 {
			return fmt.Sprintf("epiphany_%d", n)
		}
	}
	if k := loc1962MovableDayKey(date, m); k != "" {
		return k
	}
	if loc1962ChristmasSunday(date) {
		return "christmas_sunday"
	}
	if date == advent.Add(-7) {
		return "sunday_before_advent"
	}
	if date == fetchDate(m, "trinity_sunday") {
		return "trinity_sunday"
	}
	if date.Weekday() == 0 {
		firstTrinity := fetchDate(m, "trinity_sunday").Add(7)
		last := advent.Add(-7)
		sundayBeforeAdvent := advent.Add(-7)
		firstAdditional := firstTrinity.Add(24 * 7)
		if date >= firstAdditional && date < sundayBeforeAdvent {
			count := sundayBeforeAdvent.Sub(firstAdditional) / 7
			number := date.Sub(firstAdditional)/7 + 1
			n := 6
			if count != 1 {
				n = 4 + number
				if n > 6 {
					n = 6
				}
			}
			return fmt.Sprintf("epiphany_%d", n)
		}
		if date >= firstTrinity && date <= last {
			n := date.Sub(firstTrinity)/7 + 1
			if n >= 1 && n <= 24 {
				return fmt.Sprintf("trinity_%d", n)
			}
		}
	}
	easter := fetchDate(m, "easter")
	table := map[Date]string{}
	for _, kv := range []struct {
		d Date
		k string
	}{
		{fetchDate(m, "septuagesima"), "septuagesima"}, {fetchDate(m, "sexagesima"), "sexagesima"},
		{fetchDate(m, "quinquagesima"), "quinquagesima"}, {fetchDate(m, "first_sunday_in_lent"), "lent_1"},
		{easter.Add(-14), "passion_sunday"}, {fetchDate(m, "palm_sunday"), "palm_sunday"},
		{easter, "easter_day"}, {fetchDate(m, "rogation_sunday"), "rogation_sunday"},
		{fetchDate(m, "ascension"), "ascension_day"}, {fetchDate(m, "sunday_after_ascension"), "ascension_1"},
		{fetchDate(m, "pentecost"), "pentecost"},
	} {
		table[kv.d] = kv.k
	}
	return table[date]
}

func loc1962MovableDayKey(date Date, m liturgical.Movable) string {
	for _, kv := range []struct{ dateKey, key string }{
		{"ash_wednesday", "ash_wednesday"}, {"holy_monday", "monday_holy_week"},
		{"holy_tuesday", "tuesday_holy_week"}, {"holy_wednesday", "wednesday_holy_week"},
		{"maundy_thursday", "maundy_thursday"}, {"good_friday", "good_friday"},
		{"holy_saturday", "holy_saturday"}, {"easter_monday", "easter_monday"},
		{"easter_tuesday", "easter_tuesday"}, {"whitsun_monday", "whitsun_monday"},
		{"whitsun_tuesday", "whitsun_tuesday"},
	} {
		if date == fetchDate(m, kv.dateKey) {
			return kv.key
		}
	}
	easter := fetchDate(m, "easter")
	if date >= easter.Add(3) && date <= easter.Add(6) {
		return []string{"easter_wednesday", "easter_thursday", "easter_friday", "easter_saturday"}[date.Sub(easter)-3]
	}
	rogation := fetchDate(m, "rogation_sunday")
	if date >= rogation.Add(-3) && date <= rogation.Add(-1) {
		return []string{"rogation_monday", "rogation_tuesday", "rogation_wednesday"}[date.Sub(rogation.Add(-3))]
	}
	pentecost := fetchDate(m, "pentecost")
	switch date {
	case pentecost.Add(3):
		return "ember_wednesday"
	case pentecost.Add(4):
		return "ember_thursday"
	case pentecost.Add(5):
		return "ember_friday"
	case pentecost.Add(6):
		return "ember_saturday"
	}
	if k := emberKeyFor(date, fetchDate(m, "first_sunday_in_lent")); k != "" {
		return k
	}
	if k := emberKeyFor(date, fetchDate(m, "first_sunday_of_advent").Add(14)); k != "" {
		return k
	}
	return fixedEmberKeyFor(date, civil.MustNew(date.Year(), 9, 14))
}

var emberKeys = []string{"ember_wednesday", "ember_friday", "ember_saturday"}

func emberKeyFor(date, sunday Date) string {
	for i, d := range []Date{sunday.Add(3), sunday.Add(5), sunday.Add(6)} {
		if d == date {
			return emberKeys[i]
		}
	}
	return ""
}

func fixedEmberKeyFor(date, anchor Date) string {
	wednesday := anchor.Add(mod(3-anchor.Weekday(), 7))
	if wednesday <= anchor {
		wednesday = wednesday.Add(7)
	}
	for i, d := range []Date{wednesday, wednesday.Add(2), wednesday.Add(3)} {
		if d == date {
			return emberKeys[i]
		}
	}
	return ""
}

func loc1962ChristmasSunday(date Date) bool {
	year := date.Year()
	if date.Month() == 1 {
		year--
	}
	first := firstSundayAfterStrict(civil.MustNew(year, 12, 25))
	return date == first || date == first.Add(7)
}

// --- Reading::Loc1984WalesPsalter ------------------------------------------

var walesWeekdays = map[int]string{1: "monday", 2: "tuesday", 3: "wednesday", 4: "thursday", 5: "friday", 6: "saturday"}

func walesFirstMonday(year int) Date {
	jan1 := civil.MustNew(year, 1, 1)
	return jan1.Add(mod(1-jan1.Weekday(), 7))
}

func walesCycleAnchor(date Date) Date {
	current := walesFirstMonday(date.Year())
	if date >= current {
		return current
	}
	return walesFirstMonday(date.Year() - 1)
}

// WalesPsalmWeek ports Loc1984WalesPsalter.week_for.
func WalesPsalmWeek(date Date) int {
	return mod(floorDiv(date.Sub(walesCycleAnchor(date)), 7), 10) + 1
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// WalesPsalterReference ports Loc1984WalesPsalter.reference_for.
func WalesPsalterReference(date Date, serviceType string) string {
	weekday, ok := walesWeekdays[date.Weekday()]
	if !ok {
		return ""
	}
	week := WalesPsalmWeek(date)
	idx := 0
	if serviceType == "evening_prayer" {
		idx = 1
	}
	pair, _ := mapMap(walesPsalter, fmt.Sprint(week)).Get(weekday).([]any)
	return "Psalm " + rb.ToS(pair[idx])
}

// --- Reading::Loc1984WalesWeekdayLectionary --------------------------------

// WalesProvision is the table row picked for a date (string values).
type WalesProvision struct {
	Values *rb.Map // first_reading, second_reading, psalm, table, index
}

type walesLectionary struct {
	date        Date
	serviceType string
	cycle       string
	movable     liturgical.Movable
}

// WalesProvisionFor ports Loc1984WalesWeekdayLectionary.provision_for.
func WalesProvisionFor(date Date, serviceType, cycle string, cal *liturgical.Calendar) *rb.Map {
	l := &walesLectionary{date: date, serviceType: serviceType, cycle: cycle, movable: cal.Easter.Dates}
	return l.call()
}

type dateRange struct{ begin, end Date }

func (r dateRange) covers(d Date) bool { return d >= r.begin && d <= r.end }

func (l *walesLectionary) cycleKey() string {
	if l.cycle == "B" {
		return "B"
	}
	return "A"
}

func (l *walesLectionary) call() *rb.Map {
	if l.date.Weekday() == 0 {
		return nil
	}
	if d := l.directProvision(); d != nil {
		return d
	}
	name, rng, rows, ok := l.course()
	if !ok || !rng.covers(l.date) {
		return nil
	}
	days := l.eligibleDays(rng)
	pos := indexOfDate(days, l.date)
	if pos < 0 {
		return nil
	}
	row := distributedRow(rows, pos, len(days)).(*rb.Map)
	refs, ok := row.Lookup(l.cycleKey())
	if !ok {
		rb.RaiseKeyError(rb.Inspect(l.cycleKey()))
	}
	index, ok := row.Lookup("index")
	if !ok {
		rb.RaiseKeyError(`"index"`)
	}
	out := refs.(*rb.Map).Dup()
	out.Set("table", name)
	out.Set("index", index)
	return out
}

func indexOfDate(list []Date, d Date) int {
	for i, x := range list {
		if x == d {
			return i
		}
	}
	return -1
}

func (l *walesLectionary) directProvision() *rb.Map {
	if p := l.holyWeek(); p != nil {
		return p
	}
	if p := l.easterWeek(); p != nil {
		return p
	}
	return l.preChristmas()
}

func fetchKey(m *rb.Map, key string) any {
	v, ok := m.Lookup(key)
	if !ok {
		rb.RaiseKeyError(rb.Inspect(key))
	}
	return v
}

func (l *walesLectionary) holyWeek() *rb.Map {
	start := fetchDate(l.movable, "easter").Add(-6)
	if !(dateRange{start, start.Add(5)}).covers(l.date) {
		return nil
	}
	n := l.date.Sub(start) + 1
	row, ok := walesHolyWeek.Lookup(fmt.Sprint(n))
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(n))
	}
	refs := fetchKey(row.(*rb.Map), l.serviceType).([]any)
	return pairProvision(refs, "holy_week")
}

func (l *walesLectionary) easterWeek() *rb.Map {
	monday := fetchDate(l.movable, "easter").Add(1)
	if !(dateRange{monday, monday.Add(5)}).covers(l.date) {
		return nil
	}
	row := walesEasterWeek[l.date.Sub(monday)].(*rb.Map)
	list := fetchKey(row, l.cycleKey()).([]any)
	get := func(i int) any {
		if i < len(list) {
			return list[i]
		}
		return nil
	}
	return rb.M("psalm", get(0), "first_reading", get(1), "second_reading", get(2), "table", "easter_week")
}

func (l *walesLectionary) preChristmas() *rb.Map {
	rng := dateRange{civil.MustNew(l.date.Year(), 12, 19), civil.MustNew(l.date.Year(), 12, 24)}
	if !rng.covers(l.date) {
		return nil
	}
	days := l.eligibleDays(rng)
	pos := indexOfDate(days, l.date)
	if pos < 0 {
		return nil
	}
	row := distributedRow(walesPreChristmas, pos, len(days)).(*rb.Map)
	refs := fetchKey(row, l.serviceType).([]any)
	return pairProvision(refs, "pre_christmas")
}

func pairProvision(refs []any, table string) *rb.Map {
	m := rb.NewMap()
	if len(refs) > 0 && refs[0] != nil {
		m.Set("first_reading", refs[0])
	}
	if len(refs) > 1 && refs[1] != nil {
		m.Set("second_reading", refs[1])
	}
	m.Set("table", table)
	return m
}

func (l *walesLectionary) data(key string) []any {
	v, ok := walesWeekdayData.Lookup(key)
	if !ok {
		rb.RaiseKeyError(rb.Inspect(key))
	}
	return v.([]any)
}

func (l *walesLectionary) course() (string, dateRange, []any, bool) {
	builders := []func() (string, dateRange, []any, bool){
		l.adventCourse, l.postChristmasCourse, l.epiphanyCourse, l.preLentCourse, l.eastertideCourse,
		l.pentecostWeekCourse, l.afterTrinityCourse, l.lastThreeWeeksCourse, l.extraEpiphanyCourse,
	}
	for _, b := range builders {
		if name, r, rows, ok := b(); ok {
			return name, r, rows, true
		}
	}
	return "", dateRange{}, nil, false
}

func (l *walesLectionary) result(name string, r dateRange, rows func() []any) (string, dateRange, []any, bool) {
	if r.covers(l.date) {
		return name, r, rows(), true
	}
	return "", dateRange{}, nil, false
}

func (l *walesLectionary) firstAdvent() Date { return fetchDate(l.movable, "first_sunday_of_advent") }

func (l *walesLectionary) adventCourse() (string, dateRange, []any, bool) {
	fa := l.firstAdvent()
	end := fa.Add(20)
	if dec18 := civil.MustNew(l.date.Year(), 12, 18); dec18 < end {
		end = dec18
	}
	return l.result("advent", dateRange{fa.Add(1), end}, func() []any { return l.data("advent") })
}

func (l *walesLectionary) postChristmasCourse() (string, dateRange, []any, bool) {
	year := l.date.Year()
	if l.date.Month() == 1 {
		year--
	}
	r := dateRange{civil.MustNew(year, 12, 29), civil.MustNew(year+1, 1, 5)}
	return l.result("post_christmas", r, func() []any { return l.data("post_christmas") })
}

func (l *walesLectionary) epiphanyRange() dateRange {
	epiphany := civil.MustNew(l.date.Year(), 1, 6)
	var first Date
	if epiphany.Weekday() == 0 {
		first = epiphany.Add(7)
	} else {
		first = epiphany.Add(7 - epiphany.Weekday())
	}
	two := first.Add(7)
	return dateRange{epiphany.Add(1), two.Add(6)}
}

func (l *walesLectionary) epiphanyCourse() (string, dateRange, []any, bool) {
	return l.result("epiphany", l.epiphanyRange(), func() []any { return l.data("epiphany") })
}

func (l *walesLectionary) preLentCourse() (string, dateRange, []any, bool) {
	r := dateRange{fetchDate(l.movable, "septuagesima").Add(1), fetchDate(l.movable, "easter").Add(-8)}
	return l.result("pre_lent_and_lent", r, func() []any { return l.data("pre_lent_and_lent") })
}

func (l *walesLectionary) eastertideCourse() (string, dateRange, []any, bool) {
	r := dateRange{fetchDate(l.movable, "easter").Add(8), fetchDate(l.movable, "pentecost").Add(-1)}
	return l.result("eastertide", r, func() []any { return l.data("eastertide") })
}

func (l *walesLectionary) pentecostWeekCourse() (string, dateRange, []any, bool) {
	r := dateRange{fetchDate(l.movable, "pentecost").Add(1), fetchDate(l.movable, "trinity_sunday").Add(-1)}
	return l.result("after_pentecost", r, func() []any { return l.data("after_pentecost") })
}

func (l *walesLectionary) lastThreeMonday() Date { return l.firstAdvent().Add(-20) }

func (l *walesLectionary) afterTrinityCourse() (string, dateRange, []any, bool) {
	r := dateRange{fetchDate(l.movable, "trinity_sunday").Add(1), l.lastThreeMonday().Add(-1)}
	if !r.covers(l.date) {
		return "", dateRange{}, nil, false
	}
	all := l.data("after_trinity")
	base := all
	if len(base) > 117 {
		base = base[:117]
	}
	needed := len(l.eligibleDays(r)) - len(base)
	if needed < 0 {
		needed = 0
	}
	extras := l.extraWeekRows()
	if needed < len(extras) {
		extras = extras[:needed]
	}
	rows := append(append([]any(nil), base...), extras...)
	return "after_trinity", r, rows, true
}

func (l *walesLectionary) lastThreeWeeksCourse() (string, dateRange, []any, bool) {
	r := dateRange{l.lastThreeMonday(), l.firstAdvent().Add(-1)}
	return l.result("last_three_weeks", r, func() []any {
		all := l.data("after_trinity")
		if len(all) > 18 {
			return all[len(all)-18:]
		}
		return all
	})
}

func (l *walesLectionary) extraEpiphanyCourse() (string, dateRange, []any, bool) {
	r := dateRange{l.epiphanyRange().end.Add(2), fetchDate(l.movable, "septuagesima").Add(-1)}
	if r.begin > r.end {
		return "", dateRange{}, nil, false
	}
	return l.result("extra_weeks", r, l.extraWeekRows)
}

func (l *walesLectionary) extraWeekRows() []any {
	var out []any
	for n := 1; n <= 6; n++ {
		out = append(out, l.data(fmt.Sprintf("extra_week_%d", n))...)
	}
	return out
}

func (l *walesLectionary) eligibleDays(r dateRange) []Date {
	var out []Date
	ascension, hasAscension := l.movable["ascension"]
	for d := r.begin; d <= r.end; d = d.Add(1) {
		if d.Weekday() == 0 || walesFixedProperDate(d) {
			continue
		}
		if hasAscension && d == ascension {
			continue
		}
		out = append(out, d)
	}
	return out
}

func walesFixedProperDate(d Date) bool {
	for _, v := range walesFixedProperDates {
		pair := v.([]any)
		if rb.ToI(pair[0]) == d.Month() && rb.ToI(pair[1]) == d.Day() {
			return true
		}
	}
	return false
}

func distributedRow(rows []any, position, dayCount int) any {
	if dayCount <= 1 {
		if len(rows) == 0 {
			return nil
		}
		return rows[0]
	}
	index := int(math.Round(float64(position) * float64(len(rows)-1) / float64(dayCount-1)))
	if index < 0 || index >= len(rows) {
		panic(&rb.RubyError{Class: "IndexError", Message: fmt.Sprintf("index %d outside of array bounds: %d...%d", index, -len(rows), len(rows))})
	}
	return rows[index]
}

// --- Reading::Cw2005EnPsalmTable -------------------------------------------

var cwWeekdayKeys = map[int]string{1: "m", 2: "t", 3: "w", 4: "th", 5: "f", 6: "s"}

type cwPsalmTable struct {
	date        Date
	serviceType string
	table       string
	mov         liturgical.Movable
}

// CwProvision is the provision a psalm table picks for a date.
type CwProvision struct {
	Reference string
	Table     string
	Period    string
}

func newCwPsalmTable(date Date, serviceType, table string) *cwPsalmTable {
	return &cwPsalmTable{date: date, serviceType: serviceType, table: table}
}

// CwPsalmReference ports Cw2005EnPsalmTable.reference_for ("" == nil).
func CwPsalmReference(date Date, serviceType, table string) string {
	p := newCwPsalmTable(date, serviceType, table).provision()
	if p == nil {
		return ""
	}
	return p.Reference
}

// CwPsalmProvision ports Cw2005EnPsalmTable.provision_for.
func CwPsalmProvision(date Date, serviceType, table string) *CwProvision {
	return newCwPsalmTable(date, serviceType, table).provision()
}

func (c *cwPsalmTable) movable() liturgical.Movable {
	if c.mov == nil {
		c.mov = liturgical.EasterFor(c.date.Year(), liturgical.Gregorian).Dates
	}
	return c.mov
}

func (c *cwPsalmTable) provision() *CwProvision {
	wd := c.date.Weekday()
	if wd < 1 || wd > 6 {
		return nil
	}
	if c.table == "course" {
		if ref := c.table5Reference(); ref != "" {
			return &CwProvision{Reference: ref, Table: "table_5", Period: "course"}
		}
		return nil
	}
	if p := c.seasonalProvision(); p != nil {
		return p
	}
	ref := c.ordinaryTimeReference()
	if ref == "" {
		return nil
	}
	return &CwProvision{Reference: ref, Table: "table_4", Period: "ordinary_time"}
}

func (c *cwPsalmTable) seasonalProvision() *CwProvision {
	for _, p := range []struct {
		period string
		f      func() string
	}{
		{"advent", c.adventReference}, {"christmas", c.christmasReference}, {"epiphany", c.epiphanyReference},
		{"presentation", c.presentationReference}, {"lent", c.lentReference}, {"easter", c.easterReference},
		{"before_advent", c.preAdventReference},
	} {
		if ref := p.f(); ref != "" {
			return &CwProvision{Reference: ref, Table: "table_3", Period: p.period}
		}
	}
	return nil
}

func (c *cwPsalmTable) adventReference() string {
	fa := c.movable()["first_sunday_of_advent"]
	if !(c.date > fa && c.date < civil.MustNew(c.date.Year(), 12, 25)) {
		return ""
	}
	if c.date.Month() == 12 && c.date.Day() >= 19 && c.date.Day() <= 24 {
		return c.formatPsalm(cwFixedAdvent(c.date.Day()))
	}
	week := c.date.Sub(fa)/7 + 1
	if week < 1 || week > 3 {
		return ""
	}
	return c.table3Reference("advent", week)
}

func cwFixedAdvent(day int) *rb.Map {
	table := map[int][2]string{
		19: {"144, 146", "10, 57"}, 20: {"46, 95", "4, 9"}, 21: {"121, 122, 123", "80, 84"},
		22: {"124, 125, 126, 127", "24, 48"}, 23: {"128, 129, 130, 131", "89.1-37"}, 24: {"45, 113", "85"},
	}
	v, ok := table[day]
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(day))
	}
	return rb.M("m", v[0], "e", v[1])
}

func (c *cwPsalmTable) christmasReference() string {
	return c.formatPsalm(mapMap(cwChristmasPsalms, fmt.Sprintf("%d,%d", c.date.Month(), c.date.Day())))
}

func (c *cwPsalmTable) epiphanyReference() string {
	year := c.date.Year()
	epiphany := civil.MustNew(year, 1, 6)
	lastFixed := epiphany.Add(mod(6-epiphany.Weekday(), 7))
	if c.date >= epiphany && c.date <= lastFixed {
		return c.formatPsalm(mapMap(cwEpiphanyFixedPsalms, fmt.Sprint(c.date.Day())))
	}
	if epiphany.Weekday() == 0 && c.date >= epiphany.Add(1) && c.date <= civil.MustNew(year, 1, 12) {
		return c.formatPsalm(mapMap(cwEpiphanyFixedPsalms, fmt.Sprint(c.date.Day())))
	}
	baptism, ok := c.movable()["baptism_of_the_lord"]
	if !ok {
		rb.RaiseNoMethodOnNil("+")
	}
	start := baptism.Add(1)
	if !(c.date >= start && c.date < civil.MustNew(year, 2, 2)) {
		return ""
	}
	week := c.date.Sub(start)/7 + 1
	if week < 1 || week > 4 {
		return ""
	}
	return c.table3Reference("epiphany", week)
}

func (c *cwPsalmTable) presentationReference() string {
	if c.date != civil.MustNew(c.date.Year(), 2, 2) {
		return ""
	}
	return c.formatPsalm(rb.M("m", "48, 146", "e", "122, 132"))
}

func (c *cwPsalmTable) lentReference() string {
	m := c.movable()
	ash := m["ash_wednesday"]
	if c.date == ash {
		return c.formatPsalm(rb.M("m", "38", "e", "51 or 102"))
	}
	firstLent := m["first_sunday_in_lent"]
	if c.date > ash && c.date < firstLent {
		return c.formatPsalm(mapMap(cwLentPreLentPsalms, cwWeekdayKeys[c.date.Weekday()]))
	}
	monday := firstLent.Add(1)
	if !(c.date >= monday && c.date < m["palm_sunday"]) {
		return ""
	}
	week := c.date.Sub(monday)/7 + 1
	if week < 1 || week > 5 {
		return ""
	}
	return c.table3Reference("lent", week)
}

func (c *cwPsalmTable) easterReference() string {
	m := c.movable()
	easter := m["easter"]
	if !(c.date > easter && c.date < m["pentecost"]) {
		return ""
	}
	if c.date == m["ascension"] {
		return ""
	}
	if c.date >= easter.Add(36) && c.date <= easter.Add(42) {
		v, _ := cwEasterSixPsalms.Lookup(cwWeekdayKeys[c.date.Weekday()])
		if v == nil {
			return ""
		}
		return c.formatPsalmString(rb.ToS(v))
	}
	monday := easter.Add(1)
	week := c.date.Sub(monday)/7 + 1
	if week < 1 || week > 7 {
		return ""
	}
	return c.table3Reference("easter", week)
}

func (c *cwPsalmTable) preAdventReference() string {
	fa := c.movable()["first_sunday_of_advent"]
	fourth := fa.Add(-28)
	if !(c.date >= fourth.Add(1) && c.date < fa) {
		return ""
	}
	week := 4 - c.date.Sub(fourth.Add(1))/7
	if week < 1 || week > 4 {
		return ""
	}
	return c.table3Reference("ordinary_before_advent", week)
}

func (c *cwPsalmTable) ordinaryTimeReference() string {
	m := c.movable()
	ash := m["ash_wednesday"]
	presentationEnd := civil.MustNew(c.date.Year(), 2, 2)
	fourth := m["first_sunday_of_advent"].Add(-28)
	preLent := c.date > presentationEnd && c.date < ash
	postPentecost := c.date > m["pentecost"] && c.date < fourth
	if !preLent && !postPentecost {
		return ""
	}
	var week int
	if preLent {
		jan2 := civil.MustNew(c.date.Year(), 1, 2)
		anchor := jan2.Add(mod(1-jan2.Weekday(), 7))
		week = mod(floorDiv(c.date.Sub(anchor), 7)+3, 7) + 1
	} else {
		anchor := m["second_sunday_of_easter"].Add(1)
		week = mod(floorDiv(c.date.Sub(anchor), 7), 7) + 1
	}
	v, ok := cwTable4.Lookup(fmt.Sprint(week))
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(week))
	}
	return c.formatPsalm(v.(*rb.Map))
}

func (c *cwPsalmTable) table3Reference(season string, week int) string {
	s, ok := cwTable3.Lookup(season)
	if !ok {
		rb.RaiseKeyError(":" + season)
	}
	w, ok := s.(*rb.Map).Lookup(fmt.Sprint(week))
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(week))
	}
	return c.formatPsalm(w.(*rb.Map))
}

func (c *cwPsalmTable) table5Reference() string {
	day := c.date.Day()
	if day == 31 {
		day = 30
	}
	if day < 1 || day > 30 {
		return ""
	}
	row, ok := cwTable5.Lookup(fmt.Sprint(day))
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(day))
	}
	key := "e"
	if c.serviceType == "morning_prayer" {
		key = "m"
	}
	v, ok := row.(*rb.Map).Lookup(key)
	if !ok {
		rb.RaiseKeyError(":" + key)
	}
	return formatCoursePsalm(rb.ToS(v))
}

func (c *cwPsalmTable) psalmKey() string {
	prefix := "evening_"
	if c.serviceType == "morning_prayer" {
		prefix = ""
	}
	k, ok := cwWeekdayKeys[c.date.Weekday()]
	if !ok {
		rb.RaiseKeyError(fmt.Sprint(c.date.Weekday()))
	}
	return prefix + k
}

var orSplitRe = rx.MustCompile(`\s+or\s+`, "i")

func (c *cwPsalmTable) formatPsalm(value *rb.Map) string {
	if value == nil {
		return ""
	}
	var key string
	if value.Has("e") {
		key = "e"
		if c.serviceType == "morning_prayer" {
			key = "m"
		}
	} else {
		key = c.psalmKey()
	}
	raw, _ := value.Lookup(key)
	if raw == nil {
		return ""
	}
	return c.formatPsalmString(rb.ToS(raw))
}

func (c *cwPsalmTable) formatPsalmString(raw string) string {
	parts := rb.StripAll(orSplitRe.Split(raw, 0))
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = rb.Strip("Psalm " + strings.ReplaceAll(p, "*", ""))
	}
	return strings.Join(out, " or ")
}

func formatCoursePsalm(value string) string {
	if !strings.Contains(value, "-") {
		return "Psalm " + value
	}
	if strings.HasPrefix(value, "119:") {
		return "Psalm " + strings.ReplaceAll(value, ":", ".")
	}
	parts := strings.SplitN(value, "-", 2)
	start, end := rb.StringToI(parts[0]), rb.StringToI(parts[1])
	var nums []string
	for n := start; n <= end; n++ {
		nums = append(nums, fmt.Sprint(n))
	}
	return "Psalm " + strings.Join(nums, ", ")
}
