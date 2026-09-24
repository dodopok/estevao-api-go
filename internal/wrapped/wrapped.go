// Package wrapped ports WrappedSnapshot: the user-private yearly summary
// behind GET /api/v1/users/wrapped.
package wrapped

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/i18n"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
)

// OfficeTypes is WrappedSnapshot::OFFICE_TYPES.
var OfficeTypes = []string{"morning", "prime", "terce", "midday", "evening", "compline", "late_evening"}

// ErrNotAvailable is WrappedSnapshot::NotAvailable.
var ErrNotAvailable = errors.New("wrapped not available")

// Period ports WrappedSnapshot::Period.
type Period struct {
	Year                         int
	Timezone                     string
	zone                         *time.Location
	PersonalStart, PersonalEnd   time.Time
	CommunityStart, CommunityEnd time.Time
}

func newPeriod(u *users.User, year int, generatedAt time.Time) *Period {
	p := &Period{Year: year}
	if loc := rb.RailsZone(u.Timezone); loc != nil {
		p.zone, p.Timezone = loc, u.Timezone
	} else {
		p.zone, p.Timezone = time.UTC, "UTC"
	}
	p.PersonalStart = time.Date(year, 1, 1, 0, 0, 0, 0, p.zone)
	next := time.Date(year+1, 1, 1, 0, 0, 0, 0, p.zone)
	p.PersonalEnd = next
	if generatedAt.Before(next) {
		p.PersonalEnd = generatedAt
	}
	p.CommunityStart = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	p.CommunityEnd = time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	if generatedAt.Before(p.CommunityEnd) {
		p.CommunityEnd = generatedAt
	}
	return p
}

type event struct {
	office    string
	localDate civil.Date
	hour      int
	bookCode  string
}

func loadEvents(ctx context.Context, u *users.User, p *Period) ([]event, error) {
	rows, err := db.Q().Query(ctx, `SELECT completions.office_type, completions.created_at, prayer_books.code
		FROM completions LEFT OUTER JOIN prayer_books ON prayer_books.id = completions.prayer_book_id
		WHERE completions.user_id = $1 AND completions.created_at >= $2 AND completions.created_at < $3
		AND completions.office_type = ANY($4)`, u.ID, p.PersonalStart.UTC(), p.PersonalEnd.UTC(), OfficeTypes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		var office string
		var created time.Time
		var code *string
		if err := rows.Scan(&office, &created, &code); err != nil {
			return nil, err
		}
		local := created.In(p.zone)
		e := event{office: office, localDate: civil.FromTime(local), hour: local.Hour()}
		if code != nil && strings.TrimSpace(*code) != "" {
			e.bookCode = *code
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Snapshot ports WrappedSnapshot#call.
func Snapshot(ctx context.Context, u *users.User, year int, locale any, generatedAt time.Time) (*rb.Map, error) {
	generatedAt = generatedAt.UTC()
	p := newPeriod(u, year, generatedAt)
	events, err := loadEvents(ctx, u, p)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrNotAvailable
	}
	pb, err := currentPrayerBook(ctx, u)
	if err != nil {
		return nil, err
	}
	personal := personalMetrics(events, p)
	cal := liturgical.NewCalendar(year, pb.Code)
	seasons, err := seasonMetrics(events, cal)
	if err != nil {
		return nil, err
	}
	bookMetrics, err := prayerBookMetrics(ctx, events, pb)
	if err != nil {
		return nil, err
	}
	lifeRule, err := lifeRuleMetrics(ctx, u, p)
	if err != nil {
		return nil, err
	}
	intentions, err := intentionsMetrics(ctx, u, p)
	if err != nil {
		return nil, err
	}
	community, err := communityMetrics(ctx, p)
	if err != nil {
		return nil, err
	}
	name := ""
	if u.Name != nil {
		name = *u.Name
	}
	out := rb.M("schema_version", 1, "generated_at", generatedAt.Format("2006-01-02T15:04:05Z"), "timezone", p.Timezone,
		"year", year, "user", rb.M("name", name))
	personal.Each(func(k string, v any) { out.Set(k, v) })
	out.Set("prayer_book", bookMetrics)
	out.Set("seasons", seasons)
	out.Set("life_rule", lifeRule)
	out.Set("intentions", intentions)
	out.Set("community", community)
	out.Set("copy", copyBuilder(events, p, cal, personal, resolveLocale(locale, u)))
	return out, nil
}

func currentPrayerBook(ctx context.Context, u *users.User) (*store.PrayerBook, error) {
	var stored *rb.Map
	if u.Onboarding != nil {
		eff, err := u.EffectivePreferences(ctx, func(pb *store.PrayerBook) ([]string, *rb.Map, error) {
			set, err := prefs.For(ctx, pb)
			if err != nil {
				return nil, nil, err
			}
			return set.Keys(), set.Defaults(), nil
		})
		if err != nil {
			return nil, err
		}
		if eff.Len() > 0 {
			stored = eff
		}
	}
	if stored == nil {
		stored = u.Preferences
	}
	if stored == nil {
		stored = rb.NewMap()
	}
	code := stored.Get("prayer_book_code")
	if code == nil {
		code = stored.Get("version")
	}
	if code == nil {
		code = "loc_2015"
	}
	var pb *store.PrayerBook
	var err error
	if !rb.Blank(code) {
		if pb, err = store.PrayerBookByCode(ctx, rb.ToS(code)); err != nil {
			return nil, err
		}
	}
	if pb == nil {
		if pb, err = store.DefaultPrayerBook(ctx); err != nil {
			return nil, err
		}
	}
	if pb == nil {
		return nil, errors.New("No Prayer Book is available for Wrapped")
	}
	return pb, nil
}

func percentage(count, total int) int {
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(count) * 100.0 / float64(total)))
}

func personalMetrics(events []event, p *Period) *rb.Map {
	hourly := make([]int, 24)
	dateCounts := map[civil.Date]int{}
	for _, e := range events {
		hourly[e.hour]++
		dateCounts[e.localDate]++
	}
	// most_frequent_office: max count, ties by key.
	bestKey, bestCount := "", -1
	for _, k := range OfficeTypes {
		n := 0
		for _, e := range events {
			if e.office == k {
				n++
			}
		}
		if n > bestCount || (n == bestCount && k < bestKey) {
			bestKey, bestCount = k, n
		}
	}
	// period_distribution
	sum := func(hours ...int) int {
		t := 0
		for _, h := range hours {
			t += hourly[h]
		}
		return t
	}
	raw := []int{sum(6, 7, 8, 9, 10, 11), sum(12, 13, 14, 15, 16, 17), sum(0, 1, 2, 3, 4, 5, 18, 19, 20, 21, 22, 23)}
	pct := make([]int, 3)
	total := 0
	for i, r := range raw {
		pct[i] = percentage(r, len(events))
		total += pct[i]
	}
	if raw[0] != 0 || raw[1] != 0 || raw[2] != 0 {
		largest := 0
		for i := range raw {
			if raw[i] > raw[largest] {
				largest = i
			}
		}
		pct[largest] += 100 - total
	}
	periods := []any{}
	for i, k := range []string{"morning", "afternoon", "night"} {
		periods = append(periods, rb.M("key", k, "percent", pct[i]))
	}
	// common time window
	startHour, windowCount := 0, -1
	for s := 0; s < 24; s++ {
		n := hourly[s] + hourly[(s+1)%24] + hourly[(s+2)%24]
		if n > windowCount {
			startHour, windowCount = s, n
		}
	}
	// best month
	var best *rb.Map
	bestDays, bestOffices, bestMonth := -1, -1, 0
	for m := 1; m <= 12; m++ {
		first := civil.MustNew(p.Year, m, 1)
		days := civil.DaysInMonth(p.Year, m)
		cal := []any{}
		prayerDays, offices := 0, 0
		for d := 0; d < days; d++ {
			n := dateCounts[first.Add(d)]
			if n > 0 {
				prayerDays++
			}
			offices += n
			cal = append(cal, min(n, 3))
		}
		if prayerDays > bestDays || (prayerDays == bestDays && offices > bestOffices) ||
			(prayerDays == bestDays && offices == bestOffices && -m > -bestMonth) {
			bestDays, bestOffices, bestMonth = prayerDays, offices, m
			best = rb.M("month", m, "prayer_days", prayerDays, "completed_offices", offices,
				"weekday_offset", first.Weekday(), "calendar", cal)
		}
	}
	// best streak: longest run of consecutive dates, the latest on ties.
	dates := make([]civil.Date, 0, len(dateCounts))
	for d := range dateCounts {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i] < dates[j] })
	var runs [][]civil.Date
	var cur []civil.Date
	for _, d := range dates {
		if len(cur) == 0 || d == cur[len(cur)-1].Add(1) {
			cur = append(cur, d)
		} else {
			runs = append(runs, cur)
			cur = []civil.Date{d}
		}
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
	}
	win := runs[0]
	for _, r := range runs[1:] {
		if len(r) > len(win) || (len(r) == len(win) && r[len(r)-1] > win[len(win)-1]) {
			win = r
		}
	}
	daily := []any{}
	for _, d := range win {
		daily = append(daily, dateCounts[d])
	}
	// prayer calendar
	calendar := []any{}
	for d := civil.MustNew(p.Year, 1, 1); d.Year() == p.Year; d = d.Add(1) {
		_, ok := dateCounts[d]
		calendar = append(calendar, ok)
	}
	hourlyAny := make([]any, 24)
	for i, h := range hourly {
		hourlyAny[i] = h
	}
	return rb.M("prayer_calendar", calendar, "prayer_days", len(dateCounts), "completed_offices", len(events),
		"most_frequent_office", rb.M("key", bestKey, "count", bestCount), "hourly_distribution", hourlyAny,
		"period_distribution", periods, "common_time_start_hour", startHour, "common_time_end_hour", (startHour+2)%24,
		"common_time_count", windowCount, "best_month", best, "best_streak", len(win), "streak_daily_offices", daily,
		"streak_start", win[0].ISO(), "streak_end", win[len(win)-1].ISO())
}

var seasonOrder = []string{"advent", "christmas", "epiphany", "lent", "easter", "ordinary_time"}

var seasonColors = map[string]string{"advent": "#B39DDB", "christmas": "#E8C968", "epiphany": "#66BB6A",
	"lent": "#8F78BF", "easter": "#F3DFA0", "ordinary_time": "#4F9A53"}

var seasonKeys = map[string]string{liturgical.Advent: "advent", liturgical.Christmas: "christmas",
	liturgical.Epiphany: "epiphany", liturgical.Lent: "lent", liturgical.EasterSeason: "easter", liturgical.OrdinaryTime: "ordinary_time"}

func seasonMetrics(events []event, cal *liturgical.Calendar) ([]any, error) {
	counts := map[string]int{}
	seen := map[civil.Date]bool{}
	for _, e := range events {
		if seen[e.localDate] {
			continue
		}
		seen[e.localDate] = true
		season := cal.SeasonFor(e.localDate)
		key, ok := seasonKeys[season]
		if !ok {
			return nil, fmt.Errorf("Unknown liturgical season %q for %s", season, e.localDate.ISO())
		}
		counts[key]++
	}
	out := []any{}
	for _, k := range seasonOrder {
		out = append(out, rb.M("key", k, "days", counts[k], "color", seasonColors[k]))
	}
	return out, nil
}

func prayerBookMetrics(ctx context.Context, events []event, pb *store.PrayerBook) (*rb.Map, error) {
	counts := map[string]int{}
	for _, e := range events {
		if e.bookCode != "" {
			counts[e.bookCode]++
		}
	}
	current := counts[pb.Code]
	percent := percentage(current, len(events))
	type kv struct {
		code  string
		count int
	}
	var others []kv
	for code, n := range counts {
		if code != pb.Code && n > 0 {
			others = append(others, kv{code, n})
		}
	}
	sort.Slice(others, func(i, j int) bool {
		if others[i].count != others[j].count {
			return others[i].count > others[j].count
		}
		return others[i].code < others[j].code
	})
	list := []any{}
	for i, o := range others {
		if i == 2 {
			break
		}
		book, err := store.PrayerBookByCode(ctx, o.code)
		if err != nil {
			return nil, err
		}
		name := ""
		if book != nil && book.Name != nil {
			name = *book.Name
		}
		list = append(list, rb.M("key", o.code, "name", name, "completed_offices", o.count, "percent", percentage(o.count, len(events))))
	}
	return rb.M("key", pb.Code, "percent", percent, "completed_offices", current, "other_percent", 100-percent,
		"other_prayer_books", list, "thumbnail_url", rb.Deref(pb.ThumbnailURL)), nil
}

func lifeRuleMetrics(ctx context.Context, u *users.User, p *Period) (*rb.Map, error) {
	start, end := p.PersonalStart.UTC(), p.PersonalEnd.UTC()
	var exams int
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM life_rule_exams WHERE user_id = $1 AND status = 'completed'
		AND completed_at >= $2 AND completed_at < $3`, u.ID, start, end).Scan(&exams); err != nil {
		return nil, err
	}
	var weekly int
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT DISTINCT life_rule_exams.life_rule_id, life_rule_exam_items.life_rule_step_id
		FROM life_rule_exam_items INNER JOIN life_rule_exams ON life_rule_exams.id = life_rule_exam_items.life_rule_exam_id
		WHERE life_rule_exam_items.life_rule_exam_id IN (SELECT id FROM life_rule_exams WHERE user_id = $1 AND status = 'completed'
		  AND completed_at >= $2 AND completed_at < $3 AND period = 'weekly')
		AND life_rule_exam_items.life_rule_step_id IS NOT NULL AND life_rule_exam_items.rating IS NOT NULL
		AND life_rule_exam_items.rating != 'not_applicable') t`, u.ID, start, end).Scan(&weekly); err != nil {
		return nil, err
	}
	rows, err := db.Q().Query(ctx, `SELECT life_rule_id, completed_at, period_start, period_end FROM life_rule_exams
		WHERE user_id = $1 AND status = 'completed' AND completed_at IS NOT NULL AND completed_at < $2
		ORDER BY period_start, id`, u.ID, end)
	if err != nil {
		return nil, err
	}
	type exam struct {
		completed         time.Time
		periodStart, pEnd time.Time
	}
	groups := map[int64][]exam{}
	var order []int64
	for rows.Next() {
		var id int64
		var e exam
		if err := rows.Scan(&id, &e.completed, &e.periodStart, &e.pEnd); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	returns := 0
	for _, id := range order {
		list := groups[id]
		for i, e := range list {
			if e.completed.Before(start) {
				continue
			}
			// rule_exams[index - 1]: the first exam is compared with the last.
			prev := list[(i-1+len(list))%len(list)]
			if e.periodStart.After(prev.pEnd.AddDate(0, 0, 1)) {
				returns++
			}
		}
	}
	return rb.M("exams", exams, "weekly_commitments", weekly, "pause_returns", returns), nil
}

func intentionsMetrics(ctx context.Context, u *users.User, p *Period) (*rb.Map, error) {
	start, end := p.PersonalStart.UTC(), p.PersonalEnd.UTC()
	var requests, weekly, rosaries int
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM prayer_requests WHERE user_id = $1 AND created_at >= $2 AND created_at < $3`,
		u.ID, start, end).Scan(&requests); err != nil {
		return nil, err
	}
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(DISTINCT week_start) FROM prayer_requests WHERE user_id = $1 AND week_start IN
		(SELECT DISTINCT week_start FROM weekly_prayers WHERE user_id = $1 AND week_start >= $2 AND week_start < $3)`,
		u.ID, fmt.Sprintf("%04d-01-01", p.Year), fmt.Sprintf("%04d-01-01", p.Year+1)).Scan(&weekly); err != nil {
		return nil, err
	}
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM custom_rosary_prayers WHERE user_id = $1 AND created_at >= $2 AND created_at < $3`,
		u.ID, start, end).Scan(&rosaries); err != nil {
		return nil, err
	}
	return rb.M("prayer_requests", requests, "weekly_prayers", weekly, "custom_rosaries", rosaries), nil
}

func communityMetrics(ctx context.Context, p *Period) (*rb.Map, error) {
	start, end := p.CommunityStart, p.CommunityEnd
	var total, people, books int
	where := `completions.created_at >= $1 AND completions.created_at < $2 AND completions.office_type = ANY($3)`
	args := []any{start, end, OfficeTypes}
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(*), COUNT(DISTINCT user_id) FROM completions WHERE `+where, args...).Scan(&total, &people); err != nil {
		return nil, err
	}
	rows, err := db.Q().Query(ctx, `SELECT DISTINCT users.timezone FROM completions INNER JOIN users ON users.id = completions.user_id WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	countries := map[string]bool{}
	for rows.Next() {
		var tz string
		if err := rows.Scan(&tz); err != nil {
			rows.Close()
			return nil, err
		}
		if c := CountryFromTimezone(tz); c != "" {
			countries[c] = true
		}
	}
	rows.Close()
	if err := db.Q().QueryRow(ctx, `SELECT COUNT(DISTINCT prayer_books.code) FROM completions INNER JOIN prayer_books ON prayer_books.id = completions.prayer_book_id
		WHERE `+where+` AND NOT (prayer_books.code IS NULL OR prayer_books.code = '')`, args...).Scan(&books); err != nil {
		return nil, err
	}
	most := "morning"
	crow, err := db.Q().Query(ctx, `SELECT office_type, COUNT(*) FROM completions WHERE `+where+` GROUP BY office_type`, args...)
	if err != nil {
		return nil, err
	}
	bestCount := -1
	for crow.Next() {
		var k string
		var n int
		if err := crow.Scan(&k, &n); err != nil {
			crow.Close()
			return nil, err
		}
		if n > bestCount || (n == bestCount && k < most) {
			most, bestCount = k, n
		}
	}
	crow.Close()
	return rb.M("completed_offices", total, "people", people, "countries", len(countries), "prayer_books", books,
		"most_prayed_office", most), nil
}

var movableNames = []struct {
	key   string
	names map[string]string
}{
	{"ash_wednesday", map[string]string{"pt-BR": "Quarta-feira de Cinzas", "pt-PT": "Quarta-feira de Cinzas", "es": "Miércoles de Ceniza", "en": "Ash Wednesday"}},
	{"palm_sunday", map[string]string{"pt-BR": "Domingo de Ramos", "pt-PT": "Domingo de Ramos", "es": "Domingo de Ramos", "en": "Palm Sunday"}},
	{"maundy_thursday", map[string]string{"pt-BR": "Quinta-feira Santa", "pt-PT": "Quinta-feira Santa", "es": "Jueves Santo", "en": "Maundy Thursday"}},
	{"good_friday", map[string]string{"pt-BR": "Sexta-feira da Paixão", "pt-PT": "Sexta-feira Santa", "es": "Viernes Santo", "en": "Good Friday"}},
	{"easter", map[string]string{"pt-BR": "Páscoa", "pt-PT": "Páscoa", "es": "Pascua", "en": "Easter"}},
	{"ascension", map[string]string{"pt-BR": "Ascensão", "pt-PT": "Ascensão", "es": "Ascensión", "en": "Ascension"}},
	{"pentecost", map[string]string{"pt-BR": "Pentecostes", "pt-PT": "Pentecostes", "es": "Pentecostés", "en": "Pentecost"}},
	{"first_sunday_of_advent", map[string]string{"pt-BR": "1º Domingo do Advento", "pt-PT": "1.º Domingo do Advento", "es": "Primer Domingo de Adviento", "en": "First Sunday of Advent"}},
}

var englishMonths = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

// translate ports I18n.t("wrapped.#{key}", locale:, default: nil, **values).
func translate(locale, key string, values i18n.Opts) (out any) {
	defer func() {
		if rec := recover(); rec != nil {
			if re, ok := rec.(*rb.RubyError); ok && re.Class == "I18n::MissingTranslationData" {
				out = nil
				return
			}
			panic(rec)
		}
	}()
	return i18n.TBang(locale, "wrapped."+key, values)
}

func copyBuilder(events []event, p *Period, cal *liturgical.Calendar, personal *rb.Map, locale string) *rb.Map {
	hourLabel := func(h int) any { return translate(locale, "hour", i18n.Opts{"hour": h}) }
	key := personal.Get("most_frequent_office").(*rb.Map).String("key")
	frequent := translate(locale, "frequent_office."+key, nil)
	if frequent == nil {
		frequent = translate(locale, "frequent_office.generic", nil)
	}
	hourly := personal.Get("hourly_distribution").([]any)
	busiest, busiestCount := 0, -1
	for h, v := range hourly {
		if n := v.(int); n > busiestCount {
			busiest, busiestCount = h, n
		}
	}
	highlight := translate(locale, "common_time_highlight", i18n.Opts{
		"start": hourLabel(personal.Get("common_time_start_hour").(int)), "finish": hourLabel(personal.Get("common_time_end_hour").(int))})
	summary := translate(locale, "common_time_summary", i18n.Opts{"highlight": highlight, "count": personal.Get("common_time_count")})
	observance := func(iso string) any {
		d, _ := civil.ParseISO(iso)
		for _, m := range movableNames {
			if md, ok := cal.Easter.Dates.Get(m.key); ok && md == d {
				return m.names[locale]
			}
		}
		if c := cal.CelebrationForDate(d); c != nil {
			if name := c.Get("name"); !rb.Blank(name) {
				return name
			}
		}
		if desc := cal.Description(d); len(desc) > 0 && !rb.BlankString(desc[0]) {
			return desc[0]
		}
		if locale != "en" {
			return fmt.Sprintf("%02d/%02d/%04d", d.Day(), d.Month(), d.Year())
		}
		return fmt.Sprintf("%s %d, %d", englishMonths[d.Month()-1], d.Day(), d.Year())
	}
	streak := translate(locale, "streak_intro", i18n.Opts{"start": observance(personal.String("streak_start")),
		"finish": observance(personal.String("streak_end"))})
	return rb.M("locale", locale, "frequent_office_headline", frequent,
		"prayer_time_headline", translate(locale, "prayer_time_headline", i18n.Opts{"hour": hourLabel(busiest)}),
		"common_time_summary", summary, "common_time_highlight", highlight, "streak_intro", streak)
}

var availableLocales = []string{"pt-BR", "pt-PT", "es", "en"}

func availableLocale(l string) bool {
	for _, a := range availableLocales {
		if a == l {
			return true
		}
	}
	return false
}

// resolveLocale ports resolved_locale and WrappedSnapshot::Locale.resolve.
func resolveLocale(param any, u *users.User) string {
	requested := param
	if rb.Blank(requested) {
		requested = nil
		if u.Preferences != nil {
			requested = u.Preferences.Get("language")
		}
		if requested == nil {
			requested = "en"
		}
	}
	var parts []string
	for _, part := range strings.Split(strings.ReplaceAll(rb.Strip(rb.ToS(requested)), "_", "-"), "-") {
		if !rb.BlankString(part) {
			parts = append(parts, part)
		}
	}
	normalized := ""
	if len(parts) > 0 {
		out := []string{strings.ToLower(parts[0])}
		for _, part := range parts[1:] {
			if len([]rune(part)) == 2 {
				part = strings.ToUpper(part)
			}
			out = append(out, part)
		}
		normalized = strings.Join(out, "-")
	}
	if availableLocale(normalized) {
		return normalized
	}
	language := strings.Split(normalized, "-")[0]
	if language == "pt" {
		return "pt-BR"
	}
	if availableLocale(language) {
		return language
	}
	return "en"
}
