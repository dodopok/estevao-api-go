package suites

import (
	"github.com/dodopok/estevao-api-go/test/diff"
)

// The audio suite reads offices as premium readers with the
// daily_office_audio flag (test/diff/fixtures/audio.sql), whose track is
// assembled from a partial clip catalogue: present, missing and customized
// clips, segment gaps, and the presigned storage URLs.
func init() {
	register("audio", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		as := func(uid, email string) map[string]string {
			return withHeaders(h, map[string]string{"Authorization": "Bearer " + TokenFor(uid, email)})
		}
		readers := map[string]map[string]string{
			"pt":   as("difftest-au-pt", "au-pt@example.com"),
			"en":   as("difftest-au-en", "au-en@example.com"),
			"free": as("difftest-au-free", "au-free@example.com"),
		}
		paths := []string{
			"/api/v1/daily_office/2026/4/5/morning", "/api/v1/daily_office/2026/4/5/evening",
			"/api/v1/daily_office/2026/9/16/morning", "/api/v1/daily_office/2026/9/16/evening",
			"/api/v1/daily_office/2026/12/24/compline", "/api/v1/daily_office/2026/9/16/compline",
			"/api/v1/daily_office/2026/5/24/compline/family", "/api/v1/daily_office/2026/4/5/morning/family",
			"/api/v1/daily_office/2026/4/5/morning?" + pref("loc_2019"),
			"/api/v1/daily_office/2026/9/16/morning?" + pref("loc_1662_en") + "&preferences%5Bbible_version%5D=kjv",
			"/api/v1/daily_office/2026/9/16/morning?" + pref("loc_1979_en") + "&preferences%5Bbible_version%5D=kjv",
			"/api/v1/daily_office/2026/4/5/midday", "/api/v1/daily_office/today/morning",
			"/api/v1/daily_office/2026/4/5/morning?shared_office_code=DTDOACTIVE",
			"/api/v1/daily_office/2026/4/5/bogus",
		}
		for name, hh := range readers {
			for _, p := range paths {
				out = append(out, diff.Request{Name: name, Path: p, Headers: hh})
			}
		}
		return out
	})
}
