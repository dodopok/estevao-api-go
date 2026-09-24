package suites

import (
	"fmt"

	"github.com/dodopok/estevao-api-go/test/diff"
)

func init() {
	register("explanations", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		dates := []string{"2025/11/30", "2025/12/25", "2026/1/6", "2026/2/18", "2026/3/19", "2026/3/30", "2026/4/3",
			"2026/4/5", "2026/5/14", "2026/5/19", "2026/5/24", "2026/6/7", "2026/8/6", "2026/9/16", "2026/11/1",
			"2026/11/3", "2026/11/22", "2027/1/13", "2027/2/10"}
		for _, b := range Books {
			for _, d := range dates {
				for _, q := range []string{"", "service_type=morning_prayer", "service_type=evening_prayer"} {
					out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/liturgical_explanation/%s?%s&%s", d, pref(b), q), Headers: h})
				}
			}
			for _, q := range []string{"locale=en", "locale=es", "locale=cy", "locale=pt-PT", "locale=xx", "service_type=bogus",
				"service_type=eucharist", "service_type=morning_prayer&preferences%5Bpsalm_cycle%5D=monthly",
				"service_type=evening_prayer&preferences%5Bpsalm_cycle%5D=course",
				"service_type=morning_prayer&preferences%5Blectionary_variant%5D=revised_1922",
				"preferences%5Bascension_readings%5D=alternative&service_type=evening_prayer",
				"preferences%5Breading_type%5D=complementary"} {
				for _, d := range []string{"2026/5/19", "2026/11/3", "2026/2/10"} {
					out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/liturgical_explanation/%s?%s&%s", d, pref(b), q), Headers: h})
				}
			}
		}
		return out
	})
}
