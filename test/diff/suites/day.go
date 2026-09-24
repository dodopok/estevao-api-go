package suites

import (
	"fmt"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// dayDates covers feasts, octaves, Holy Week, Ember/Rogation days, year
// boundaries and ordinary weekdays in two liturgical years.
var dayDates = []string{
	"2025/11/30", "2025/12/1", "2025/12/7", "2025/12/17", "2025/12/24", "2025/12/25", "2025/12/26",
	"2025/12/28", "2025/12/31", "2026/1/1", "2026/1/4", "2026/1/6", "2026/1/11", "2026/1/13",
	"2026/1/25", "2026/2/2", "2026/2/15", "2026/2/18", "2026/2/19", "2026/2/25", "2026/3/1",
	"2026/3/19", "2026/3/25", "2026/3/29", "2026/3/30", "2026/4/1", "2026/4/2", "2026/4/3",
	"2026/4/4", "2026/4/5", "2026/4/6", "2026/4/9", "2026/4/12", "2026/5/10", "2026/5/11",
	"2026/5/14", "2026/5/17", "2026/5/24", "2026/5/27", "2026/5/31", "2026/6/4", "2026/6/7",
	"2026/6/24", "2026/6/29", "2026/7/22", "2026/8/6", "2026/8/15", "2026/9/16", "2026/9/29",
	"2026/10/31", "2026/11/1", "2026/11/2", "2026/11/22", "2026/11/26", "2026/11/29", "2026/12/21",
	"2027/1/10", "2027/2/10", "2027/3/28", "2025/2/29", "2026/13/1",
}

func init() {
	register("calendar_day", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		for _, b := range Books {
			for _, d := range dayDates {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/calendar/%s?%s", d, pref(b)), Headers: h})
			}
			out = append(out, diff.Request{Name: "today " + b, Path: "/api/v1/calendar/today?" + pref(b), Headers: h})
		}
		extras := []string{
			"preferences%5Bbible_version%5D=kjv", "preferences%5Bbible_version%5D=arc",
			"preferences%5Breading_type%5D=complementary", "preferences%5Breading_type%5D=semicontinuous",
			"preferences%5Bpsalm_cycle%5D=monthly", "preferences%5Bpsalm_cycle%5D=course",
			"preferences%5Bmorning_psalm_cycle%5D=monthly", "preferences%5Bpsalm_translation%5D=coverdale",
			"preferences%5Bdaily_office_rite%5D=1", "preferences%5Bdaily_office_rite%5D=2",
			"preferences%5Blectionary_variant%5D=revised_1871", "preferences%5Blectionary_variant%5D=revised_1922",
			"preferences%5Blectionary_variant%5D=original_1928", "preferences%5Blectionary_variant%5D=revised_1945",
			"preferences%5Bascension_readings%5D=alternative",
		}
		for _, b := range Books {
			for _, e := range extras {
				for _, d := range []string{"2025/12/24", "2026/3/30", "2026/5/19", "2026/9/16", "2026/11/1", "2026/1/13"} {
					out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/calendar/%s?%s&%s", d, pref(b), e), Headers: h})
				}
			}
		}
		return out
	})

	register("lectionary", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		for _, b := range Books {
			for _, d := range dayDates {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/lectionary/%s?%s", d, pref(b)), Headers: h})
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/lectionary/%s/all_services?%s", d, pref(b)), Headers: h})
			}
			for _, e := range []string{"preferences%5Breading_type%5D=complementary", "preferences%5Bbible_version%5D=kjv", "preferences%5Blectionary_variant%5D=revised_1922"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/lectionary/2026/6/7/all_services?%s&%s", pref(b), e), Headers: h})
			}
		}
		for _, y := range []string{"2025", "2026", "2027", "1899", "2201", "abc"} {
			out = append(out, diff.Request{Path: "/api/v1/lectionary/cycle/" + y + "?" + pref("loc_2015"), Headers: h})
		}
		out = append(out, diff.Request{Path: "/api/v1/lectionary/2026/2/30?" + pref("loc_2015"), Headers: h})
		out = append(out, diff.Request{Path: "/api/v1/lectionary/2026/2/3", Headers: h})
		return out
	})
}
