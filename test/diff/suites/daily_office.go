package suites

import (
	"fmt"
	"net/url"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// officeDates spans Advent, Christmas, Lent, the Triduum, Easter,
// Pentecost/Trinity and ordinary time, plus invalid dates.
var officeDates = []string{"2025/11/30", "2025/12/25", "2026/2/18", "2026/4/2", "2026/4/5", "2026/5/31", "2026/9/16", "2026/12/24"}

var officeTypes = []string{"morning", "midday", "evening", "compline", "prime", "terce", "none", "late_evening", "bogus"}

func init() {
	register("daily_office", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		as := func(uid, email string) map[string]string {
			return withHeaders(h, map[string]string{"Authorization": "Bearer " + TokenFor(uid, email)})
		}
		free := as("difftest-do-free", "do-free@example.com")
		premium := as("difftest-do-premium", "do-premium@example.com")
		expired := as("difftest-do-expired", "do-expired@example.com")
		noonboard := as("difftest-do-noonboard", "do-noonboard@example.com")
		english := as("difftest-do-english", "do-english@example.com")

		for _, b := range Books {
			for _, d := range officeDates {
				for _, o := range officeTypes {
					out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/%s/%s?%s", d, o, pref(b)), Headers: h})
				}
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/%s/morning/family?%s", d, pref(b)), Headers: h})
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/%s/compline/family?%s", d, pref(b)), Headers: h})
			}
			for _, o := range []string{"morning", "evening", "bogus"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/today/%s?%s", o, pref(b)), Headers: h})
			}
			for _, ot := range []string{"family", "traditional", "standard", "alternative"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/2026/4/5/morning?%s&preferences%%5Boffice_type%%5D=%s", pref(b), ot), Headers: h})
			}
			out = append(out, diff.Request{Path: "/api/v1/daily_office/preferences?" + pref(b), Headers: h})
			out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/daily_office/2026/9/16/morning?%s&preferences%%5Bseed%%5D=12345", pref(b)), Headers: h})
		}

		// Preference encodings and validation.
		js := url.QueryEscape(`{"prayer_book_code":"loc_2015","seed":77,"lords_prayer_version":"traditional"}`)
		for _, q := range []string{
			"", "preferences=" + js, "preferences=%7Bnot-json", pref("nope"), pref("loc_2027"),
			pref("loc_2015") + "&preferences%5Bbible_version%5D=kjv", pref("loc_2015") + "&preferences%5Bbible_version%5D=zzz",
			pref("loc_2015") + "&preferences%5Bfamily_rite%5D=true", pref("loc_2015") + "&preferences%5Bfamily_rite%5D=1",
		} {
			out = append(out, diff.Request{Name: "q " + q, Path: "/api/v1/daily_office/2026/4/5/morning?" + q, Headers: h})
			out = append(out, diff.Request{Name: "q " + q, Path: "/api/v1/daily_office/preferences?" + q, Headers: h})
		}
		for _, d := range []string{"2026/2/30", "1899/1/1", "2201/1/1", "2026/13/1", "2026/0/5", "2026/1/32", "abc/1/1"} {
			out = append(out, diff.Request{Path: "/api/v1/daily_office/" + d + "/morning?" + pref("loc_2015"), Headers: h})
			out = append(out, diff.Request{Path: "/api/v1/daily_office/" + d + "/morning/family?" + pref("loc_2015"), Headers: h})
		}

		// Shared offices.
		for _, code := range []string{"DTDOACTIVE", "DTDOENGLISH", "DTDOFAMILY", "DTDOEXPIRED", "NOPE"} {
			for _, p := range []string{"/api/v1/daily_office/today/morning", "/api/v1/daily_office/2026/9/16/midday", "/api/v1/daily_office/preferences"} {
				out = append(out, diff.Request{Name: code, Path: p + "?shared_office_code=" + code, Headers: h})
				out = append(out, diff.Request{Name: code + " free", Path: p + "?shared_office_code=" + code, Headers: free})
			}
		}

		// Authenticated readers: stored preferences, overrides, premium gates,
		// completions and feature flags.
		for name, hh := range map[string]map[string]string{"free": free, "premium": premium, "expired": expired, "noonboard": noonboard, "english": english} {
			for _, p := range []string{
				"/api/v1/daily_office/2026/4/5/morning", "/api/v1/daily_office/2026/4/5/evening",
				"/api/v1/daily_office/2026/9/16/morning", "/api/v1/daily_office/2026/9/16/compline",
				"/api/v1/daily_office/2026/4/5/morning/family", "/api/v1/daily_office/today/morning",
				"/api/v1/daily_office/preferences", "/api/v1/daily_office/2026/4/5/morning?" + pref("loc_2019"),
				"/api/v1/daily_office/2026/4/5/morning?" + pref("locb_2008") + "&preferences%5Bmorning_prayer_rite%5D=3",
				"/api/v1/daily_office/2026/4/5/morning?" + pref("loc_1549"),
			} {
				out = append(out, diff.Request{Name: name, Path: p, Headers: hh})
			}
		}

		// Missing app identification.
		out = append(out, diff.Request{Name: "no app id", Path: "/api/v1/daily_office/2026/4/5/morning?" + pref("loc_2015"),
			Headers: map[string]string{"Accept": "application/json", "X-Trusted-Server-Key": "trusted-server-test-key"}})
		return out
	})
}
