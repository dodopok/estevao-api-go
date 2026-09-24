package suites

import (
	"fmt"

	"github.com/dodopok/estevao-api-go/test/diff"
)

func init() {
	register("calendar_grid", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		for _, b := range Books {
			for _, y := range []int{2024, 2025, 2026, 2027} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/calendar/%d?%s", y, pref(b)), Headers: h})
				for _, sub := range []string{"overview", "seasons", "key_dates", "celebrations", "celebrations?grouped=true", "celebrations?type=sunday", "celebrations?type=festival"} {
					sep := "?"
					if len(sub) > 12 && sub[12] == '?' {
						sep = "&"
					}
					out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/calendar/%d/%s%s%s", y, sub, sep, pref(b)), Headers: h})
				}
			}
		}
		// Error and edge cases.
		for _, p := range []string{
			"/api/v1/calendar/2025/12", "/api/v1/calendar/2025/13?" + pref("loc_2015"), "/api/v1/calendar/1899?" + pref("loc_2015"),
			"/api/v1/calendar/2025/12?" + pref("nope"), "/api/v1/calendar/2025/12?" + pref("loc_2027"),
			"/api/v1/calendar/2025/celebrations?type=bogus&" + pref("loc_2015"),
			"/api/v1/calendar/abcd/overview?" + pref("loc_2015"), "/api/v1/calendar/2025/12.json?" + pref("loc_2015"),
			"/api/v1/calendar/2025/12?preferences=%7B%22prayer_book_code%22%3A%22loc_2019%22%7D",
			"/api/v1/calendar/2025/12?preferences=notjson",
			"/api/v1/calendar/2025/12?" + pref("loc_2015") + "&preferences%5Bbible_version%5D=zzz",
		} {
			out = append(out, diff.Request{Path: p, Headers: h})
		}
		out = append(out, diff.Request{Name: "no app id", Path: "/api/v1/calendar/2025/12?" + pref("loc_2015"), Headers: map[string]string{"X-Trusted-Server-Key": "trusted-server-test-key"}})
		return out
	})
}
