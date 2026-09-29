package v1

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// withFilters runs the before_actions shared by CalendarController and
// LectionaryController, then the ApiKeyAuthenticatable after_action.
func withFilters(controller string, action func(c *web.Context, r *resolver)) web.HandlerFunc {
	return func(c *web.Context) {
		r := calendarFilters(c)
		action(c, r)
		auth.RecordAPIKeyUsage(c, controller)
	}
}

func (r *resolver) bibleVersionOrNVI() string {
	if v := r.resolvedPreferences().Get("bible_version"); v != nil && v != false {
		return rb.ToS(v)
	}
	return "nvi"
}

func (r *resolver) prayerBookCodeOrDefault() string {
	if v := r.resolvedPreferences().Get("prayer_book_code"); v != nil && v != false {
		return rb.ToS(v)
	}
	return "loc_2015"
}

func (r *resolver) lectionaryResolver(c *web.Context, date civil.Date, cal *liturgical.Calendar, serviceType string) *reading.Resolver {
	return reading.For(c.Ctx, date, reading.Options{
		PrayerBookCode: r.prayerBookCodeOrDefault(), Calendar: cal, DayContext: cal.ContextFor(date),
		Translation: r.bibleVersionOrNVI(), ReadingType: r.readingType(), ServiceType: serviceType,
		ServiceVariant: r.lectionaryVariant(), LoadContent: true,
	})
}

func serializeReading(p *reading.Passage) any {
	if p == nil {
		return nil
	}
	m := rb.M("reference", p.Reference)
	if p.Alternative != nil {
		m.Set("alternative_reference", p.Alternative.Reference)
	}
	if len(p.Alternatives) > 0 {
		refs := make([]any, len(p.Alternatives))
		for i, a := range p.Alternatives {
			refs[i] = a.Reference
		}
		m.Set("alternative_references", refs)
	}
	return m
}

func serializeServiceReadings(s *reading.Selection) any {
	if s == nil || s.FirstReading == nil {
		return nil
	}
	return rb.M(
		"primeira_leitura", serializeReading(s.FirstReading),
		"salmo", serializeReading(s.Psalm),
		"segunda_leitura", serializeReading(s.SecondReading),
		"evangelho", serializeReading(s.Gospel),
	)
}

// LectionaryDay ports LectionaryController#day.
var LectionaryDay = withFilters("lectionary", func(c *web.Context, r *resolver) {
	date := parseDate(c)
	cachedLectionary(c, r, "day", date, func() (int, any) { return lectionaryDayResponse(c, r, date) })
})

func lectionaryDayResponse(c *web.Context, r *resolver, date civil.Date) (int, any) {
	cal := liturgical.NewCalendar(date.Year(), r.prayerBookCodeOrDefault())
	res := r.lectionaryResolver(c, date, cal, "")
	sel := res.Selection()
	if sel != nil && sel.FirstReading != nil {
		return 200, rb.M(
			"data", date.ISO(),
			"dia_da_semana", liturgical.DayNamesPT[date.Weekday()],
			"ciclo", res.Cycle,
			"leituras", rb.M(
				"primeira_leitura", serializeReading(sel.FirstReading),
				"salmo", serializeReading(sel.Psalm),
				"segunda_leitura", serializeReading(sel.SecondReading),
				"evangelho", serializeReading(sel.Gospel),
			),
		)
	}
	return 404, rb.M(
		"data", date.ISO(),
		"ciclo", res.Cycle,
		"mensagem", "Leituras não encontradas para esta data. Por favor, adicione-as ao banco de dados.",
	)
}

// LectionaryAllServices ports LectionaryController#all_services.
var LectionaryAllServices = withFilters("lectionary", func(c *web.Context, r *resolver) {
	date := parseDate(c)
	cachedLectionary(c, r, "all_services", date, func() (int, any) { return 200, lectionaryAllServicesResponse(c, r, date) })
})

func lectionaryAllServicesResponse(c *web.Context, r *resolver, date civil.Date) any {
	cal := liturgical.NewCalendar(date.Year(), r.prayerBookCodeOrDefault())
	cycle := ""
	out := map[string]any{}
	for _, st := range []struct{ key, service string }{
		{"santa_eucaristia", "eucharist"}, {"oracao_matutina", "morning_prayer"}, {"oracao_vespertina", "evening_prayer"},
	} {
		res := r.lectionaryResolver(c, date, cal, st.service)
		if cycle == "" {
			cycle = res.Cycle
		}
		out[st.key] = serializeServiceReadings(res.Selection())
	}
	var cycleVal any
	if cycle != "" {
		cycleVal = cycle
	}
	return rb.M(
		"data", date.ISO(),
		"dia_da_semana", liturgical.DayNamesPT[date.Weekday()],
		"ciclo", cycleVal,
		"santa_eucaristia", out["santa_eucaristia"],
		"oracao_matutina", out["oracao_matutina"],
		"oracao_vespertina", out["oracao_vespertina"],
	)
}

// cachedLectionary renders the response build computes, cached for a day
// under LectionaryController#lectionary_readings_cache_key. The status is
// cached with the body, as Rails caches {status:, body:} for #day.
func cachedLectionary(c *web.Context, r *resolver, endpoint string, date civil.Date, build func() (int, any)) {
	code := r.prayerBookCodeOrDefault()
	variant := r.lectionaryVariant()
	if variant == "" {
		variant = "default"
	}
	key := strings.Join([]string{"lectionary", endpoint, "v1", date.ISO(), code, r.bibleVersionOrNVI(), r.readingType(), variant,
		"pb_" + rediscache.TimestampVersion(&r.prayerBook(code, true).UpdatedAt)}, "/")
	entry := rediscache.FetchJSON(c.Ctx, key, 24*time.Hour, func() []byte {
		status, body := build()
		return append([]byte(strconv.Itoa(status)+"\n"), rb.JSON(body)...)
	})
	head, body, _ := bytes.Cut(entry, []byte("\n"))
	status, _ := strconv.Atoi(string(head))
	c.Raw(status, "application/json; charset=utf-8", body)
}

// LectionaryCycleInfo ports LectionaryController#cycle_info.
var LectionaryCycleInfo = withFilters("lectionary", func(c *web.Context, r *resolver) {
	year := rb.ToI(c.Param("year"))
	validateYear(year)
	sunday := map[int]string{0: "C", 1: "A", 2: "B"}[((year%3)+3)%3]
	weekday := "odd"
	if year%2 == 0 {
		weekday = "even"
	}
	c.JSON(200, rb.M(
		"ano", year,
		"ciclo_dominical", sunday,
		"ciclo_semanal", weekday,
		"descricao", rb.M(
			"dominical", fmt.Sprintf("Leituras dos domingos seguem o ciclo %s (rotação trienal A, B, C)", sunday),
			"semanal", fmt.Sprintf("Leituras dos dias de semana seguem o ano %s (rotação bienal par/ímpar)", weekday),
		),
	))
})
