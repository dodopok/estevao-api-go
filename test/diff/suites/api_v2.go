package suites

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// The keys test/diff/fixtures/api_v2.sql installs.
const (
	v2Key         = "estevao_v2fixture00000000000000000000000000000000000000001"
	v2InactiveKey = "estevao_v2fixture00000000000000000000000000000000000000002"
	v2ExpiredKey  = "estevao_v2fixture00000000000000000000000000000000000000003"
)

func v2Headers(key string) map[string]string {
	h := map[string]string{"Accept": "application/json", "X-Trusted-Server-Key": "trusted-server-test-key"}
	if key != "" {
		h["X-API-Key"] = key
	}
	return h
}

// v2Cursor builds the cursor CollectionCursor.encode would for a book and
// a filter set serialized as Active Support JSON.
func v2Cursor(book, filtersJSON, keyJSON string) string {
	sum := sha256.Sum256([]byte(`["` + book + `",` + filtersJSON + `]`))
	scope := hex.EncodeToString(sum[:])[:20]
	payload := `{"version":1,"scope":"` + scope + `","key":` + keyJSON + `}`
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// v2CelebrationIDs picks a few celebrations per book (the first by id, a
// movable one and one with collects), read from the shared database.
func v2CelebrationIDs(book string) []int64 {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		(SELECT c.id FROM celebrations c JOIN prayer_books pb ON pb.id = c.prayer_book_id WHERE pb.code = $1 ORDER BY c.id LIMIT 1)
		UNION (SELECT c.id FROM celebrations c JOIN prayer_books pb ON pb.id = c.prayer_book_id WHERE pb.code = $1 AND c.movable ORDER BY c.id LIMIT 1)
		UNION (SELECT c.id FROM celebrations c JOIN prayer_books pb ON pb.id = c.prayer_book_id
		       WHERE pb.code = $1 AND EXISTS (SELECT 1 FROM collects k WHERE k.celebration_id = c.id)
		         AND EXISTS (SELECT 1 FROM lectionary_readings l WHERE l.celebration_id = c.id AND l.service_type = 'eucharist')
		       ORDER BY c.id LIMIT 1)
		ORDER BY 1`, book)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func init() {
	register("api_v2", func() []diff.Request {
		h := v2Headers(v2Key)
		var out []diff.Request
		get := func(path string) {
			out = append(out, diff.Request{Path: path, Headers: h, Volatile: []string{"generated_at"}, KeyedETag: true})
		}
		dates := []string{"2026-01-06", "2026-02-18", "2026-04-03", "2026-04-05", "2026-05-24", "2026-08-15", "2026-11-29", "2026-12-25", "2025-06-29"}
		for _, b := range Books {
			q := "?book=" + url.QueryEscape(b)
			for _, d := range dates {
				get("/api/v2/days/" + d + q)
				get("/api/v2/days/" + d + q + "&include=readings,collect,celebrations")
			}
			get("/api/v2/days/2026-04-05" + q + "&include=readings.text,readings.morning,readings.evening,readings.alternatives,collect.text")
			get("/api/v2/days/2026-12-25" + q + "&include=explanation&service=eucharist")
			get("/api/v2/days" + q + "&from=2026-03-01&to=2026-03-07&include=readings,collect")
			get("/api/v2/days" + q + "&year=2025")
			for _, d := range []string{"2026-04-05", "2026-08-15"} {
				get("/api/v2/days/" + d + "/readings" + q + "&service=all&include=text,alternatives")
			}
			get("/api/v2/days/2026-02-18/readings" + q + "&service=morning")
			get("/api/v2/days/2026-02-18/readings" + q + "&service=evening&include=text&text_format=plain")
			get("/api/v2/collects/2026-11-29" + q + "&include=text")
			get("/api/v2/collects/2026-06-29" + q)
			get("/api/v2/liturgical-explanations/2026-04-05" + q)
			get("/api/v2/liturgical-explanations/2026-02-18" + q + "&service=morning")
			for _, sub := range []string{"", "/seasons", "/key-dates", "/lectionary-cycle"} {
				get("/api/v2/years/2026" + sub + q)
			}
			get("/api/v2/years/2027" + q)
			get("/api/v2/celebrations" + q)
			get("/api/v2/celebrations" + q + "&limit=7&type=festival")
			get("/api/v2/celebrations" + q + "&year=2026&limit=15&month=4")
			get("/api/v2/celebrations" + q + "&year=2026&type=sunday&limit=5")
			get("/api/v2/celebrations" + q + "&limit=3&cursor=" + v2Cursor(b, "{}", "[3,0]"))
			get("/api/v2/celebrations" + q + "&year=2026&limit=4&cursor=" + v2Cursor(b, `{"year":2026}`, `["2026-06-01","",""]`))
			for _, id := range v2CelebrationIDs(b) {
				get(fmt.Sprintf("/api/v2/celebrations/%d%s", id, q))
			}
			get("/api/v2/prayer-books/" + b)
			get("/api/v2/prayer-books/" + b + "/preferences")
		}

		lq := "?book=loc_2015"
		for _, p := range []string{
			// Include and fields.
			"/api/v2/days/2026-04-05" + lq + "&include=bogus,readings",
			"/api/v2/days/2026-04-05" + lq + "&include=readings.text.extra",
			"/api/v2/days/2026-04-05" + lq + "&include=%20readings%20,,%20,collect",
			"/api/v2/days/2026-04-05" + lq + "&include%5B%5D=readings",
			"/api/v2/days/2026-04-05" + lq + "&include=explanation&service=bogus",
			"/api/v2/days/2026-04-05" + lq + "&include=explanation&service=morning",
			"/api/v2/days/2026-04-05" + lq + "&fields%5Bday%5D=date,color,readings&include=readings",
			"/api/v2/days/2026-04-05" + lq + "&fields%5Bday%5D=date,bogus,nope",
			"/api/v2/days/2026-04-05" + lq + "&fields%5Bbogus%5D=x",
			"/api/v2/days/2026-04-05" + lq + "&fields=date",
			"/api/v2/days/2026-04-05" + lq + "&fields%5Bday%5D=",
			"/api/v2/days/2026-04-05" + lq + "&fields%5Breading%5D=reference,slot&fields%5Bcollect%5D=title&include=readings.morning,collect",
			"/api/v2/days/2026-04-05" + lq + "&include=readings.text&text_format=markdown",
			"/api/v2/days/2026-04-05" + lq + "&include=readings.text&text_format=bogus",
			// Context parameters.
			"/api/v2/days/2026-04-05",
			"/api/v2/days/2026-04-05?book=",
			"/api/v2/days/2026-04-05?book=nope",
			"/api/v2/days/2026-04-05?book%5B%5D=loc_2015",
			"/api/v2/days/2026-04-05" + lq + "&bible=kjv&include=readings.text",
			"/api/v2/days/2026-04-05" + lq + "&bible=zzz&include=readings",
			"/api/v2/days/2026-04-05" + lq + "&lang=en",
			"/api/v2/days/2026-04-05" + lq + "&reading_type=continuous&include=readings",
			"/api/v2/days/2026-04-05" + lq + "&lectionary=bogus",
			"/api/v2/days/2026-04-05" + lq + "&psalter=bogus&include=readings.morning",
			"/api/v2/days/2026-04-05" + lq + "&preferences=%7B%22bible_version%22%3A%22kjv%22%7D&include=readings.text",
			"/api/v2/days/2026-04-05" + lq + "&preferences=notjson",
			"/api/v2/days/2026-04-05" + lq + "&preferences=%5B1%5D",
			"/api/v2/days/2026-04-05" + lq + "&preferences%5Bbible_version%5D=kjv&include=readings.text",
			"/api/v2/days/2026-04-05?book=loc_1662_en&lectionary=original_1662&include=readings",
			"/api/v2/days/2026-04-05?book=loc_1928_en&lectionary=revised_1945&include=readings",
			// Dates.
			"/api/v2/days/2026-02-30" + lq, "/api/v2/days/abc" + lq, "/api/v2/days/2026-2-3" + lq, "/api/v2/days/today" + lq,
			"/api/v2/days/2026-04-05.json" + lq,
			"/api/v2/days" + lq, "/api/v2/days" + lq + "&from=2026-03-10&to=2026-03-01",
			"/api/v2/days" + lq + "&from=2026-03-10", "/api/v2/days" + lq + "&from=2026-01-01&to=2027-01-02",
			"/api/v2/days" + lq + "&from=2026-01-01&to=2026-02-01&include=readings.text",
			"/api/v2/days" + lq + "&from=2026-01-01&to=2026-01-31&include=explanation",
			"/api/v2/days" + lq + "&from=2026-01-01&to=2026-02-01&include=explanation",
			"/api/v2/days" + lq + "&year=1899", "/api/v2/days" + lq + "&year=20x6",
			"/api/v2/days?from=2026-01-01&to=2026-01-05",
			"/api/v2/days" + lq + "&from=bogus&to=2026-01-05&include=bogus",
			"/api/v2/days" + lq + "&from=2026-12-28&to=2027-01-03&fields%5Bday%5D=date,season",
			// Readings.
			"/api/v2/days/2026-04-05/readings" + lq + "&service=bogus",
			"/api/v2/days/2026-04-05/readings" + lq + "&service%5B%5D=all",
			"/api/v2/days/2026-04-05/readings" + lq + "&service=eucharist&include=alternatives&fields%5Breading%5D=reference",
			"/api/v2/days/2026-04-05/readings" + lq + "&service=all&fields%5Breading%5D=slot",
			"/api/v2/days/2026-04-05/readings" + lq + "&include=readings.text",
			"/api/v2/days/2026-04-05/readings?include=bogus",
			// Explanations and collects.
			"/api/v2/liturgical-explanations/2026-04-05" + lq + "&service=bogus",
			"/api/v2/liturgical-explanations/2026-04-05" + lq + "&include=readings",
			"/api/v2/liturgical-explanations/2026-04-05" + lq + "&fields%5Bday%5D=date",
			"/api/v2/liturgical-explanations/2026-04-05" + lq + "&lang=cy",
			"/api/v2/collects/2026-04-05" + lq + "&include=bogus",
			"/api/v2/collects/2026-04-05" + lq + "&include=text&fields%5Bcollect%5D=text,title",
			// Passages.
			"/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=nvi",
			"/api/v2/passages?ref=John%203:16-18&bible=KJV&text_format=markdown",
			"/api/v2/passages?ref=Sl%2023&bible=nvi&text_format=plain",
			"/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=nvi&text_format=bogus",
			"/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=nvi&include=text",
			"/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=nvi&fields%5Bx%5D=y",
			"/api/v2/passages?ref=Xyz%2099:99&bible=nvi", "/api/v2/passages?ref=&bible=nvi", "/api/v2/passages?bible=nvi",
			"/api/v2/passages?ref=Jo%C3%A3o%203:16", "/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=zzz",
			"/api/v2/passages?ref=Tobias%201:1&bible=nvi", "/api/v2/passages?ref=Jo%C3%A3o%2099:1&bible=nvi",
			// Catalogues.
			"/api/v2/prayer-books", "/api/v2/prayer-books?lang=pt-BR", "/api/v2/prayer-books?lang=en", "/api/v2/prayer-books?lang=zz",
			"/api/v2/prayer-books?fields%5Bprayer_book%5D=code,name", "/api/v2/prayer-books?include=x",
			"/api/v2/prayer-books?fields%5Bprayer_book%5D=bogus", "/api/v2/prayer-books/nope", "/api/v2/prayer-books/nope/preferences",
			"/api/v2/prayer-books/loc_2015?fields%5Bprayer_book%5D=capabilities",
			"/api/v2/bible-versions", "/api/v2/bible-versions?lang=pt", "/api/v2/bible-versions?lang=en", "/api/v2/bible-versions?lang=zz",
			"/api/v2/bible-versions?fields%5Bbible_version%5D=code,language",
			// Celebrations.
			"/api/v2/celebrations", "/api/v2/celebrations/types", "/api/v2/celebrations/types?lang=pt-BR", "/api/v2/celebrations/types?lang=PT",
			"/api/v2/celebrations/types?fields%5Bcelebration_type%5D=value", "/api/v2/celebrations/types?include=x",
			"/api/v2/celebrations" + lq + "&type=bogus", "/api/v2/celebrations" + lq + "&type=sunday",
			"/api/v2/celebrations" + lq + "&movable=true&limit=100", "/api/v2/celebrations" + lq + "&movable=0",
			"/api/v2/celebrations" + lq + "&movable=maybe", "/api/v2/celebrations" + lq + "&month=12&day=25",
			"/api/v2/celebrations" + lq + "&day=25", "/api/v2/celebrations" + lq + "&month=2&day=30",
			"/api/v2/celebrations" + lq + "&year=2026&month=2&day=30", "/api/v2/celebrations" + lq + "&year=2028&month=2&day=29",
			"/api/v2/celebrations" + lq + "&month=13", "/api/v2/celebrations" + lq + "&month=1x",
			"/api/v2/celebrations" + lq + "&q=MARIA", "/api/v2/celebrations" + lq + "&q=%20%20",
			"/api/v2/celebrations" + lq + "&q=" + strings.Repeat("a", 201), "/api/v2/celebrations" + lq + "&q=s%C3%A3o&year=2026",
			"/api/v2/celebrations" + lq + "&year=2026&movable=true", "/api/v2/celebrations" + lq + "&year=2026&type=festival&q=a",
			"/api/v2/celebrations" + lq + "&limit=0", "/api/v2/celebrations" + lq + "&limit=101", "/api/v2/celebrations" + lq + "&limit=abc",
			"/api/v2/celebrations" + lq + "&limit=5&fields%5Bcelebration%5D=id,name&x=1&z%5B%5D=a&z%5B%5D=b",
			"/api/v2/celebrations" + lq + "&cursor=garbage", "/api/v2/celebrations" + lq + "&cursor=eyJ2ZXJzaW9uIjoxfQ",
			"/api/v2/celebrations" + lq + "&cursor=" + v2Cursor("loc_2019", "{}", "[3,0]"),
			"/api/v2/celebrations" + lq + "&cursor=" + v2Cursor("loc_2015", "{}", `["a",0]`),
			"/api/v2/celebrations" + lq + "&cursor=" + v2Cursor("loc_2015", "{}", "[]"),
			"/api/v2/celebrations" + lq + "&limit=2&cursor=" + v2Cursor("loc_2015", "{}", "[5]"),
			"/api/v2/celebrations" + lq + "&limit=2&cursor=" + base64.RawURLEncoding.EncodeToString([]byte("[1,2]")),
			"/api/v2/celebrations" + lq + "&limit=2&cursor=" + base64.RawURLEncoding.EncodeToString([]byte(`{"version":1.0,"scope":"x","key":[]}`)),
			"/api/v2/celebrations?book=loc_2015&cursor=eyJ2ZXJzaW9uIjoxLCJzY29wZSI6ImYxMzY3ODVhYWM0Yzk4Y2E2MGU2Iiwia2V5IjpbIjIwMjYtMDEtMTgiLCIywrogRG9taW5nbyBkYSBFcGlmYW5pYSIsInN1bmRheSJdfQ&limit=2&type=sunday&year=2026",
			"/api/v2/celebrations/0" + lq, "/api/v2/celebrations/abc" + lq, "/api/v2/celebrations/999999999" + lq,
			"/api/v2/celebrations/99999999999999999999" + lq, "/api/v2/celebrations/1", "/api/v2/celebrations/-1" + lq,
			// Years.
			"/api/v2/years/1899" + lq, "/api/v2/years/abcd" + lq, "/api/v2/years/2026",
			"/api/v2/years/2026" + lq + "&fields%5Bannual_overview%5D=year,seasons&fields%5Bseason%5D=name",
			"/api/v2/years/2026" + lq + "&fields%5Bannual_overview%5D=key_dates&fields%5Bkey_date%5D=date",
			"/api/v2/years/2026/seasons" + lq + "&fields%5Bseason%5D=slug&lang=en",
			"/api/v2/years/2026/key-dates" + lq + "&fields%5Bkey_date%5D=name",
			"/api/v2/years/2026/lectionary-cycle" + lq + "&fields%5Blectionary_cycle%5D=sunday",
			"/api/v2/years/2025/lectionary-cycle" + lq, "/api/v2/years/2200/lectionary-cycle" + lq,
		} {
			get(p)
		}
		// Authentication.
		for _, k := range []string{"", "nope", v2InactiveKey, v2ExpiredKey} {
			for _, p := range []string{"/api/v2/days/2026-04-05" + lq, "/api/v2/prayer-books", "/api/v2/passages"} {
				out = append(out, diff.Request{Path: p, Headers: v2Headers(k), Volatile: []string{"generated_at"}, KeyedETag: true})
			}
		}
		// Conditional GET.
		for _, p := range []string{"/api/v2/days/2026-04-05" + lq, "/api/v2/prayer-books", "/api/v2/passages?ref=Jo%C3%A3o%203:16&bible=nvi",
			"/api/v2/celebrations" + lq, "/api/v2/days/2026-04-05" + lq + "&fields%5Bbogus%5D=x"} {
			out = append(out, diff.Request{Path: p, Headers: withHeaders(h, map[string]string{"If-None-Match": "*"}),
				Volatile: []string{"generated_at"}, KeyedETag: true})
		}
		return out
	})
}
