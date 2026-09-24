package suites

import (
	"fmt"

	"github.com/dodopok/estevao-api-go/test/diff"
)

func init() {
	register("catalog", func() []diff.Request {
		var out []diff.Request
		h := AppHeaders()
		user := withHeaders(h, map[string]string{"Authorization": "Bearer " + TokenFor("difftest-catalog-user", "catalog@example.com")})
		for _, hh := range []map[string]string{h, user} {
			for _, p := range []string{
				"/api/v1/prayer_books", "/api/v1/prayer_books?language=en", "/api/v1/prayer_books?language=pt-BR",
				"/api/v1/prayer_books?language=xx", "/api/v1/bible_versions", "/api/v1/bible_versions?language=en",
				"/api/v1/bible_versions?language=pt-BR", "/api/v1/bible_versions?language=cy", "/api/v1/bible_versions?language=zz",
				"/api/v1/prayer_books/nope", "/api/v1/celebrations/types",
			} {
				out = append(out, diff.Request{Path: p, Headers: hh})
			}
		}
		for _, inm := range []string{`*`, `W/"nope"`} {
			out = append(out, diff.Request{Name: "inm " + inm, Path: "/api/v1/prayer_books", Headers: withHeaders(h, map[string]string{"If-None-Match": inm})})
			out = append(out, diff.Request{Name: "inm " + inm, Path: "/api/v1/bible_versions", Headers: withHeaders(h, map[string]string{"If-None-Match": inm})})
			out = append(out, diff.Request{Name: "inm " + inm, Path: "/api/v1/prayer_books/loc_2015", Headers: withHeaders(h, map[string]string{"If-None-Match": inm})})
		}
		for _, b := range Books {
			out = append(out, diff.Request{Path: "/api/v1/prayer_books/" + b, Headers: h})
			for _, q := range []string{"", "type=festival", "type=lesser_feast", "movable=true", "movable=false", "movable=0", "type=bogus"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/celebrations?%s&%s", pref(b), q), Headers: h})
			}
			for _, d := range []string{"12/25", "1/6", "11/1", "6/29", "13/1", "2/30", "0/5"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/celebrations/date/%s?%s", d, pref(b)), Headers: h})
			}
			out = append(out, diff.Request{Path: "/api/v1/celebrations/search?q=santo&" + pref(b), Headers: h})
		}
		for _, id := range []string{"1", "100", "2500", "3881", "3012", "99999999", "abc", "12abc", "search"} {
			for _, b := range []string{"loc_2015", "loc_1979_es", "awrv_2025_en"} {
				out = append(out, diff.Request{Path: fmt.Sprintf("/api/v1/celebrations/%s?%s", id, pref(b)), Headers: h})
			}
		}
		return out
	})
}
