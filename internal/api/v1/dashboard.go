package v1

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/dashboard"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// dashboardDate ports DashboardController#parse_date_param (strict
// Date.iso8601); ok is false once the 400 is rendered.
func dashboardDate(c *web.Context, name string, def civil.Date) (civil.Date, bool) {
	v := c.Param(name)
	if rb.Blank(v) {
		return def, true
	}
	d, err := rb.DateISO8601(rb.ToS(v))
	if err == nil {
		if cd, ok := d.Civil(); ok {
			return cd, true
		}
	}
	c.JSON(400, rb.M("error", "Invalid "+name+". Use ISO 8601 format (YYYY-MM-DD)", "code", "INVALID_DATE", "parameter", name))
	return 0, false
}

// DashboardIndex ports DashboardController#index.
func DashboardIndex(c *web.Context) {
	auth.AuthenticateAdmin(c)
	today := civil.FromTime(clock.Now().In(rb.AppZone))
	start, ok := dashboardDate(c, "start_date", today.Add(-30))
	if !ok {
		return
	}
	end, ok := dashboardDate(c, "end_date", today)
	if !ok {
		return
	}
	sections := dashboard.Sections
	if v := c.Param("sections"); !rb.Blank(v) {
		requested := []string{}
		for _, s := range strings.Split(rb.ToS(v), ",") {
			if s = rb.Strip(s); !rb.Blank(s) {
				requested = append(requested, s)
			}
		}
		var invalid []any
		for _, s := range requested {
			known := false
			for _, k := range dashboard.Sections {
				known = known || k == s
			}
			if !known {
				invalid = append(invalid, s)
			}
		}
		if len(invalid) > 0 {
			available := []any{}
			for _, k := range dashboard.Sections {
				available = append(available, k)
			}
			c.JSON(400, rb.M("error", "Unknown dashboard sections", "invalid_sections", invalid, "available_sections", available))
			return
		}
		sections = []string{}
		seen := map[string]bool{}
		for _, s := range requested {
			if !seen[s] {
				seen[s] = true
				sections = append(sections, s)
			}
		}
	}
	if start > end {
		c.JSON(422, rb.M("error", "start_date must be before end_date"))
		return
	}
	data := dashboard.Call(c.Ctx, start, end, sections)
	names := []any{}
	for _, s := range sections {
		names = append(names, s)
	}
	c.JSON(200, rb.M("period", rb.M("start_date", start.ISO(), "end_date", end.ISO()), "sections", names, "data", data))
}
