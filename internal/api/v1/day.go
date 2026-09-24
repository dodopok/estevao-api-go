package v1

import (
	"time"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// today ports Date.current in the application time zone.
func today() civil.Date {
	return civil.FromTime(time.Now().In(rb.AppZone))
}

// CalendarToday ports CalendarController#today.
var CalendarToday = withCalendar(func(c *web.Context, r *resolver) {
	c.JSON(200, dayPayload(c, r, today()))
})

// CalendarDay ports CalendarController#day.
var CalendarDay = withCalendar(func(c *web.Context, r *resolver) {
	c.JSON(200, dayPayload(c, r, parseDate(c)))
})

func dayPayload(c *web.Context, r *resolver, date civil.Date) *rb.Map {
	pb := r.book()
	cal := newCalendar(c, date.Year(), pb)
	values := r.resolvedPreferences().Values
	style := liturgical.RulesFor(r.code()).CalendarCollectLanguageStyle(rb.ToS(values.Get("daily_office_rite")))
	return (&DayPayload{
		Ctx: c, Date: date, Preferences: values, Language: pb.Language, CollectLanguageStyle: style,
		Calendar: cal, DayContext: cal.ContextFor(date), LectionaryVariant: r.lectionaryVariant(), variantSet: true,
	}).Call()
}

// DayPayload ports Calendar::DayPayload.
type DayPayload struct {
	Ctx                  *web.Context
	Date                 civil.Date
	Preferences          *rb.Map
	Language             string
	CollectLanguageStyle string
	Calendar             *liturgical.Calendar
	DayContext           *liturgical.DayContext
	LectionaryVariant    string
	variantSet           bool

	selection     *reading.Selection
	selectionDone bool
}

func (p *DayPayload) code() string { return rb.ToS(p.Preferences.Get("prayer_book_code")) }

// Call ports #call.
func (p *DayPayload) Call() *rb.Map {
	if p.Calendar == nil {
		p.Calendar = liturgical.NewCalendar(p.Date.Year(), p.code())
	}
	if p.DayContext == nil {
		p.DayContext = p.Calendar.ContextFor(p.Date)
	}
	info := p.Calendar.DayInfo(p.Date)
	lang := p.Language
	celebrations := info.Get("celebrations")
	if celebrations == nil {
		celebrations = []any{}
	}
	out := rb.M(
		"season_post_slug", info.Get("season_post_slug"),
		"book_season_post_slug", info.Get("book_season_post_slug"),
		"date", info.Get("date"),
		"liturgical_year", info.Get("liturgical_year"),
		"is_sunday", info.Get("is_sunday"),
		"is_holy_day", info.Get("is_holy_day"),
		"fast_day", info.Get("fast_day"),
		"fast_observance", liturgical.PresentFastObservance(p.DayContext.FastObservance, lang),
		"week_of_season", info.Get("week_of_season"),
		"proper_week", info.Get("proper_week"),
		"sunday_after_pentecost", info.Get("sunday_after_pentecost"),
		"celebration", info.Get("celebration"),
		"celebrations", celebrations,
		"saint", info.Get("saint"),
	)
	out.Set("day_of_week", liturgical.DayName(p.Date, lang))
	out.Set("liturgical_season", liturgical.TranslateSeason(rb.ToS(info.Get("liturgical_season")), lang))
	var bookSeason any
	if v := liturgical.TranslateBookSeason(rb.ToS(info.Get("book_season")), lang); v != "" {
		bookSeason = v
	}
	out.Set("book_season", bookSeason)
	color := rb.ToS(info.Get("color"))
	if liturgical.TranslatedLanguage(lang) {
		color = liturgical.TranslateColor(color, lang)
	}
	out.Set("liturgical_color", color)
	var sundayName any
	if v := info.Get("sunday_name"); v != nil {
		s := rb.ToS(v)
		if t := liturgical.TranslateSundayName(&s, lang); t != nil {
			sundayName = *t
		}
	}
	out.Set("sunday_name", sundayName)
	var descs []string
	for _, d := range info.Get("description").([]any) {
		descs = append(descs, rb.ToS(d))
	}
	translated := []any{}
	for _, d := range liturgical.TranslateDescriptions(descs, lang) {
		translated = append(translated, d)
	}
	out.Set("description", translated)
	out.Set("collect", p.collects())
	out.Set("readings", reading.SelectionToH(p.readingSelection()))
	out.Set("morning_readings", reading.SelectionToH(p.officeReadings("morning")))
	out.Set("evening_readings", reading.SelectionToH(p.officeReadings("evening")))
	return out
}

func (p *DayPayload) collects() any {
	return collects.New(p.Ctx.Ctx, p.Date, collects.Options{
		PrayerBookCode: p.code(), LanguageStyle: p.CollectLanguageStyle, IncludeFixedOfficeCollect: false,
	}).FindCollectsValue()
}

func prefString(m *rb.Map, key string) string {
	v := m.Get(key)
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

func (p *DayPayload) readingOptions() reading.Options {
	readingType := "semicontinuous"
	if v := p.Preferences.Get("reading_type"); v != nil && v != false {
		readingType = rb.ToS(v)
	}
	return reading.Options{
		PrayerBookCode: p.code(), Calendar: p.Calendar, DayContext: p.DayContext,
		Translation: prefString(p.Preferences, "bible_version"), ReadingType: readingType,
		PsalmTranslation: prefString(p.Preferences, "psalm_translation"), LoadContent: true,
	}
}

func (p *DayPayload) readingSelection() *reading.Selection {
	if !p.selectionDone {
		p.selectionDone = true
		p.selection = reading.For(p.Ctx.Ctx, p.Date, p.readingOptions()).Selection()
	}
	return p.selection
}

func (p *DayPayload) officeReadings(office string) *reading.Selection {
	o := p.readingOptions()
	o.ServiceVariant = p.lectionaryServiceVariant()
	o.ServiceType = office + "_prayer"
	o.PsalmTable = prefs.PsalmCycle(p.Preferences, office)
	return reading.For(p.Ctx.Ctx, p.Date, o).Selection()
}

func (p *DayPayload) lectionaryServiceVariant() string {
	if p.variantSet && p.LectionaryVariant != "" {
		return p.LectionaryVariant
	}
	pb, err := store.PrayerBookByCode(p.Ctx.Ctx, p.code())
	must(err)
	if pb == nil {
		v := p.Preferences.Get("lectionary_variant")
		if rb.Present(v) {
			return rb.ToS(v)
		}
		return ""
	}
	return books.For(pb.Code, pb.Features).LectionaryServiceVariant(p.Preferences)
}
