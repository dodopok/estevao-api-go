package v1

import (
	"github.com/dodopok/estevao-api-go/internal/explain"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func paramPresence(c *web.Context, key string) string {
	v := c.Param(key)
	if rb.Blank(v) {
		return ""
	}
	return rb.ToS(v)
}

// LiturgicalExplanationsShow ports LiturgicalExplanationsController#show.
var LiturgicalExplanationsShow = withFilters("liturgical_explanations", func(c *web.Context, r *resolver) {
	date := parseDate(c)
	pb := r.book()
	values := r.resolvedPreferences().Values
	rules := liturgical.RulesFor(pb.Code)
	variant := rules.ServiceVariantFromPreferences(rb.ToS(values.Get("ascension_readings")))
	if !rules.WeekdayTableLectionary() {
		variant = r.lectionaryVariant()
	}
	serviceType := paramPresence(c, "service_type")
	psalmTable := ""
	if st := rb.ToS(c.Param("service_type")); st == "morning_prayer" || st == "evening_prayer" {
		psalmTable = prefs.PsalmCycle(values, st, "weekday_psalm_table")
	}
	svc := explain.New(date, pb, prefString(values, "bible_version"), r.readingType(), paramPresence(c, "locale"), serviceType, variant, psalmTable)
	data := explain.Data(svc.Call(c.Ctx))
	c.JSON(200, rb.M("data", data, "meta", rb.M("request_id", c.RequestID, "contract_version", 1)))
})
