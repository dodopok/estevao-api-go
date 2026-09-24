package v1

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// calendarFilters runs CalendarController's before_actions.
func calendarFilters(c *web.Context) *resolver {
	auth.AuthenticateUserOrAPIKey(c)
	r := newResolver(c)
	r.apiKeyCalls = auth.APIKey(c) != nil
	if !r.apiKeyCalls {
		auth.VerifyAppRequest(c)
	}
	r.validatePreferences()
	return r
}

// withCalendar wraps an action with the filters and the api-key after_action.
func withCalendar(action func(c *web.Context, r *resolver)) web.HandlerFunc {
	return func(c *web.Context) {
		r := calendarFilters(c)
		action(c, r)
		auth.RecordAPIKeyUsage(c, "calendar")
	}
}

// --- DateValidations ------------------------------------------------------

func invalidDate(msg string) { web.Raise("InvalidDate", msg, "") }

func validateYear(year int) {
	if year < 1900 || year > 2200 {
		invalidDate("Ano deve estar entre 1900 e 2200")
	}
}

func validateYearMonth(year, month int) {
	validateYear(year)
	if month < 1 || month > 12 {
		invalidDate("Mês deve estar entre 1 e 12")
	}
}

// parseDate ports DateValidations#parse_date.
func parseDate(c *web.Context) civil.Date {
	year, month, day := rb.ToI(c.Param("year")), rb.ToI(c.Param("month")), rb.ToI(c.Param("day"))
	validateYearMonth(year, month)
	if day < 1 || day > 31 {
		invalidDate("Dia inválido para o mês especificado")
	}
	d, ok := civil.New(year, month, day)
	if !ok {
		invalidDate("Invalid date: " + itoa(year) + "-" + itoa(month) + "-" + itoa(day))
	}
	return d
}

// --- grid ------------------------------------------------------------------

func celebrationsOf(c *web.Context, pb *store.PrayerBook) *liturgical.BookCelebrations {
	bc, err := store.CelebrationsForBook(c.Ctx, pb)
	must(err)
	return bc
}

func newCalendar(c *web.Context, year int, pb *store.PrayerBook) *liturgical.Calendar {
	return liturgical.NewCalendarWith(year, pb.Code, celebrationsOf(c, pb))
}

// compactDay ports Calendar::CompactDayPayload#call.
func compactDay(info *rb.Map, fast *liturgical.FastObservance, language string) *rb.Map {
	dmy := rb.ToS(info.Get("date"))
	date := dmy[6:10] + "-" + dmy[3:5] + "-" + dmy[0:2]
	var celebrationName any
	if cel, ok := info.Get("celebration").(*rb.Map); ok {
		celebrationName = cel.Get("name")
	}
	var weekName any
	if sn := info.Get("sunday_name"); sn != nil {
		s := rb.ToS(sn)
		weekName = *liturgical.TranslateSundayName(&s, language)
	} else {
		var descs []string
		for _, d := range info.Get("description").([]any) {
			descs = append(descs, rb.ToS(d))
		}
		week := ""
		for _, d := range descs {
			if strings.Contains(d, "Semana após") {
				week = d
				break
			}
		}
		if week == "" {
			for _, d := range descs {
				if strings.Contains(d, "Semana") || strings.Contains(d, "Oitava") {
					week = d
					break
				}
			}
		}
		if week != "" {
			weekName = liturgical.TranslateDescription(week, language)
		}
	}
	return rb.M(
		"date", date,
		"color", info.Get("color"),
		"season_post_slug", info.Get("season_post_slug"),
		"book_season_post_slug", info.Get("book_season_post_slug"),
		"fast_day", info.Get("fast_day"),
		"fast_observance", liturgical.PresentFastObservance(fast, language),
		"celebration_name", celebrationName,
		"week_name", weekName,
	)
}

func compactDays(c *web.Context, pb *store.PrayerBook, year int, months []int) []any {
	cal := newCalendar(c, year, pb)
	out := []any{}
	for _, m := range months {
		for d := 1; d <= civil.DaysInMonth(year, m); d++ {
			date := civil.MustNew(year, m, d)
			out = append(out, compactDay(cal.DayInfo(date), cal.ContextFor(date).FastObservance, pb.Language))
		}
	}
	return out
}

// CalendarMonth ports CalendarController#month.
var CalendarMonth = withCalendar(func(c *web.Context, r *resolver) {
	year, month := rb.ToI(c.Param("year")), rb.ToI(c.Param("month"))
	validateYearMonth(year, month)
	c.JSON(200, compactDays(c, r.book(), year, []int{month}))
})

// CalendarYear ports CalendarController#year.
var CalendarYear = withCalendar(func(c *web.Context, r *resolver) {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	c.JSON(200, compactDays(c, r.book(), year, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}))
})

func overview(c *web.Context, r *resolver) *liturgical.YearOverview {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	pb := r.book()
	return liturgical.NewYearOverview(year, pb.Code, celebrationsOf(c, pb), true)
}

// CalendarOverview ports #overview.
var CalendarOverview = withCalendar(func(c *web.Context, r *resolver) { c.JSON(200, overview(c, r).Call()) })

// CalendarYearSeasons ports #year_seasons.
var CalendarYearSeasons = withCalendar(func(c *web.Context, r *resolver) { c.JSON(200, overview(c, r).Seasons()) })

// CalendarYearKeyDates ports #year_key_dates.
var CalendarYearKeyDates = withCalendar(func(c *web.Context, r *resolver) { c.JSON(200, overview(c, r).KeyDates()) })

// CalendarYearCelebrations ports #year_celebrations.
var CalendarYearCelebrations = withCalendar(func(c *web.Context, r *resolver) {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	typ := ""
	if v := c.Param("type"); rb.Present(v) {
		typ = rb.ToS(v)
		valid := false
		for _, t := range liturgical.ValidCelebrationTypes {
			if t == typ {
				valid = true
			}
		}
		if !valid {
			web.Raise("InvalidParameter", "Tipo inválido. Valores aceitos: "+strings.Join(liturgical.ValidCelebrationTypes, ", "), "INVALID_CELEBRATION_TYPE")
		}
	}
	grouped := c.Param("grouped") == "true"
	pb := r.book()
	c.JSON(200, liturgical.NewYearOverview(year, pb.Code, celebrationsOf(c, pb), true).Celebrations(typ, grouped))
})
