package v1

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/calgrid"
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

// CalendarMonth ports CalendarController#month.
var CalendarMonth = withCalendar(func(c *web.Context, r *resolver) {
	year, month := rb.ToI(c.Param("year")), rb.ToI(c.Param("month"))
	validateYearMonth(year, month)
	c.Raw(200, "application/json; charset=utf-8", r.grid(c).Month(year, month, nil))
})

// CalendarYear ports CalendarController#year.
var CalendarYear = withCalendar(func(c *web.Context, r *resolver) {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	c.Raw(200, "application/json; charset=utf-8", r.grid(c).Year(year))
})

// grid ports CalendarController#grid_cache.
func (r *resolver) grid(c *web.Context) *calgrid.Grid {
	if r.gridCache == nil {
		r.gridCache = calgrid.New(c.Ctx, r.book())
	}
	return r.gridCache
}

// yearOverview ports #year_overview: validates the year, then caches and
// renders one key of the overview.
func yearOverview(c *web.Context, r *resolver, scope string, key func(*liturgical.YearOverview) any) {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	g := r.grid(c)
	c.Raw(200, "application/json; charset=utf-8", g.Fetch(func() any { return key(overviewService(c, g.Book(), year)) }, scope, year))
}

func overviewService(c *web.Context, pb *store.PrayerBook, year int) *liturgical.YearOverview {
	return liturgical.NewYearOverview(year, pb.Code, celebrationsOf(c, pb), true)
}

// CalendarOverview ports #overview.
var CalendarOverview = withCalendar(func(c *web.Context, r *resolver) {
	yearOverview(c, r, "overview", func(o *liturgical.YearOverview) any { return o.Call() })
})

// CalendarYearSeasons ports #year_seasons.
var CalendarYearSeasons = withCalendar(func(c *web.Context, r *resolver) {
	yearOverview(c, r, "seasons", func(o *liturgical.YearOverview) any { return o.Seasons() })
})

// CalendarYearKeyDates ports #year_key_dates.
var CalendarYearKeyDates = withCalendar(func(c *web.Context, r *resolver) {
	yearOverview(c, r, "key_dates", func(o *liturgical.YearOverview) any { return o.KeyDates() })
})

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
	filter := typ
	if filter == "" {
		filter = "all"
	}
	g := r.grid(c)
	c.Raw(200, "application/json; charset=utf-8", g.Fetch(func() any {
		return overviewService(c, g.Book(), year).Celebrations(typ, grouped)
	}, "celebrations", year, filter, grouped))
})
