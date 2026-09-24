package suites

import (
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// contentFixture extends the users fixture with journals, favorites and
// shared offices at fixed ids.
var contentFixture = strings.Replace(userFixture, "DELETE FROM users WHERE id BETWEEN 990001 AND 990099;", `
DELETE FROM journals WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM user_favorites WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM shared_offices WHERE user_id BETWEEN 990001 AND 990099 OR short_code LIKE 'DTUC%';
DELETE FROM users WHERE id BETWEEN 990001 AND 990099;`, 1) + `
INSERT INTO journals (id, user_id, date_reference, entry_type, office_type, content, created_at, updated_at) VALUES
 (990001, 990001, '2026-03-02', 'daily_office', 'morning', 'Primeira', '2026-03-02 10:00', '2026-03-02 10:00'),
 (990002, 990001, '2026-03-02', 'life_rule', NULL, 'Regra', '2026-03-02 11:00', '2026-03-02 11:00'),
 (990003, 990001, '2026-03-15', 'daily_office', 'evening', 'Outra', '2026-03-15 20:00', '2026-03-15 20:00'),
 (990004, 990002, '2026-03-02', 'daily_office', 'morning', 'Alheia', '2026-03-02 10:00', '2026-03-02 10:00');
INSERT INTO user_favorites (id, user_id, post_slug, kind, name, created_at, updated_at) VALUES
 (990001, 990001, 'advent', 'season', 'Advento', '2026-01-01', '2026-01-01'),
 (990002, 990001, 'st-mark', 'commemoration', NULL, '2026-02-01', '2026-02-01');
INSERT INTO shared_offices (id, user_id, short_code, prayer_book_code, office_type, date, seed, preferences, expires_at, created_at, updated_at) VALUES
 (990001, 990001, 'DTUCACT', 'loc_2015', 'morning', '2026-03-02', 11, '{"creed_type":"apostles"}', '2099-01-01', now(), now()),
 (990002, NULL, 'DTUCOLD', 'loc_2015', 'evening', '2026-03-02', 12, '{}', '2020-01-01', now(), now());
`

func init() {
	registerScenarios("user_content", func() []diff.Scenario {
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		onb := userHeaders("difftest-us-onb", "us-onb@example.com")
		bad := userHeaders("difftest-us-badvoice", "us-badvoice@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		tables := []string{"completions", "journals", "user_favorites", "shared_offices", "fcm_tokens", "user_onboardings",
			"active_storage_blobs", "active_storage_attachments"}
		sc := func(name string, snapshots []string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: contentFixture, Tables: tables, Steps: steps, Snapshot: snapshots}
		}
		get := func(path string, h map[string]string) diff.Request { return diff.Request{Path: path, Headers: h} }
		created := []string{"created_at", "updated_at"}
		getV := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: created}
		}
		return []diff.Scenario{
			sc("completions", []string{
				`SELECT user_id, date_reference, office_type, duration_seconds, prayer_book_id FROM completions WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`,
				`SELECT id, current_streak, longest_streak, last_completed_office_at IS NOT NULL FROM users WHERE id BETWEEN 990001 AND 990099 ORDER BY id`},
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"morning","date":"2026-09-20","duration_seconds":"120"}`, created...),
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"morning","date":"2026-09-20"}`),
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"vespers","date":"20/09/2026"}`),
				jsonReq("POST", "/api/v1/completions", plain, `{"date":"2026-09-21"}`),
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"evening","date":"not a date"}`),
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"evening","date":"Sep 21 2026","duration_seconds":"abc"}`, created...),
				jsonReq("POST", "/api/v1/completions", plain, `{"office_type":"compline","date":"2026-02-30"}`),
				jsonReq("POST", "/api/v1/completions", onb, `{"office_type":"compline"}`, created...),
				jsonReq("POST", "/api/v1/completions", bad, `{"office_type":"compline","date":"2026-09-01"}`, created...),
				getV("/api/v1/completions/2026/9/20", plain),
				get("/api/v1/completions/2026/3/2", plain),
				get("/api/v1/completions/2026/2/30", plain),
				get("/api/v1/completions/1800/1/1", plain),
				get("/api/v1/completions/2026/13/1", plain),
				getV("/api/v1/completions/2026/9/20/morning", plain),
				get("/api/v1/completions/2026/9/20/evening", plain),
				get("/api/v1/completions/2026/9/20/vespers", plain),
				diff.Request{Method: "DELETE", Path: "/api/v1/completions/990001", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/completions/990001", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/completions/abc", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/completions/990002", Headers: onb},
				getV("/api/v1/users/completions?limit=3", plain),
			),
			sc("journals", []string{
				`SELECT id, user_id, date_reference, entry_type, office_type, content FROM journals WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`},
				jsonReq("POST", "/api/v1/journals", plain, `{"journal":{"date_reference":"2026-09-20","entry_type":"daily_office","office_type":"morning","content":"Hoje"}}`, created...),
				jsonReq("POST", "/api/v1/journals", plain, `{"date_reference":"20/09/2026","entry_type":"life_rule","content":"Top level"}`, created...),
				jsonReq("POST", "/api/v1/journals", plain, `{"journal":{"date_reference":"2026-02-30","entry_type":"daily_office","office_type":"vespers","content":""}}`),
				jsonReq("POST", "/api/v1/journals", plain, `{"journal":{"entry_type":"other"}}`),
				jsonReq("POST", "/api/v1/journals", plain, `{}`),
				jsonReq("PATCH", "/api/v1/journals/990001", plain, `{"journal":{"content":"Editada"}}`, "updated_at"),
				jsonReq("PUT", "/api/v1/journals/990002", plain, `{"journal":{"content":"Regra"}}`),
				jsonReq("PATCH", "/api/v1/journals/990001", plain, `{"journal":{"entry_type":"daily_office","office_type":null}}`),
				jsonReq("PATCH", "/api/v1/journals/990004", plain, `{"journal":{"content":"x"}}`),
				jsonReq("PATCH", "/api/v1/journals/abc", plain, `{"journal":{"content":"x"}}`),
				getV("/api/v1/journals/2026/3/2", plain),
				get("/api/v1/journals/2026/3", plain),
				getV("/api/v1/journals/2026/9", plain),
				get("/api/v1/journals/2026/2/30", plain),
				get("/api/v1/journals/2026/13", plain),
				get("/api/v1/journals/1800/1", plain),
				diff.Request{Method: "DELETE", Path: "/api/v1/journals/990003", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/journals/990003", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/journals/990004", Headers: plain},
				get("/api/v1/journals/2026/3", plain),
			),
			sc("favorites", []string{
				`SELECT user_id, post_slug, kind, name, updated_at > '2026-06-01' FROM user_favorites WHERE user_id BETWEEN 990001 AND 990099 ORDER BY post_slug`},
				get("/api/v1/favorites", plain),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"lent","kind":"season","name":"Quaresma"}`, created...),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"advent","kind":"season","name":"Advento"}`),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"st-mark","kind":"commemoration","name":"São Marcos"}`),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"x","kind":"other"}`),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"","kind":""}`),
				jsonReq("POST", "/api/v1/favorites", plain, `{"post_slug":"advent","kind":"nope"}`),
				diff.Request{Method: "DELETE", Path: "/api/v1/favorites/lent", Headers: plain},
				diff.Request{Method: "DELETE", Path: "/api/v1/favorites/lent", Headers: plain},
				get("/api/v1/favorites", plain),
				get("/api/v1/favorites", onb),
			),
			sc("shared offices", []string{
				`SELECT user_id, prayer_book_code, office_type, date, seed, preferences::text, length(short_code), expires_at > now() FROM shared_offices
				   WHERE user_id BETWEEN 990001 AND 990099 OR short_code LIKE 'DTUC%' OR (user_id IS NULL AND seed IN (4242, 4343)) ORDER BY id`},
				get("/api/v1/shared_offices/DTUCACT", anon),
				get("/api/v1/shared_offices/DTUCOLD", anon),
				get("/api/v1/shared_offices/NOPE", anon),
				get("/api/v1/shared_offices/DTUCACT", map[string]string{"Accept": "application/json"}),
				withVolatile(jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-09-20","office_type":"morning","seed":4242,"prayer_book_code":"loc_2015","preferences":{"creed_type":"nicene","seed":9}}`)),
				withVolatile(jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-09-20","office_type":"morning","seed":"4242","prayer_book_code":"loc_2015","preferences":{"creed_type":"nicene"}}`)),
				withVolatile(jsonReq("POST", "/api/v1/shared_offices", plain, `{"date":"2026-03-02","office_type":"morning","seed":"11","preferences":{"creed_type":"apostles"}}`)),
				withVolatile(jsonReq("POST", "/api/v1/shared_offices", onb, `{"date":"2026-09-21","office_type":"evening","seed":"4343"}`)),
				jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-09-20","office_type":"morning","seed":4242}`),
				jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-09-20","office_type":"vespers","seed":1,"prayer_book_code":"loc_2015"}`),
				jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-09-20","office_type":"morning","seed":"01","prayer_book_code":"loc_2015"}`),
				jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"20/09/2026","office_type":"morning","seed":1,"prayer_book_code":"loc_2015"}`),
				jsonReq("POST", "/api/v1/shared_offices", anon, `{"date":"2026-02-30","office_type":"morning","seed":1,"prayer_book_code":"loc_2015"}`),
				jsonReq("POST", "/api/v1/shared_offices", map[string]string{"Accept": "application/json"}, `{"date":"2026-09-20"}`),
			),
			sc("prayer book preferences", nil,
				get("/api/v1/prayer_books/loc_2015/preferences", anon),
				get("/api/v1/prayer_books/loc_1979_en/preferences", anon),
				get("/api/v1/prayer_books/nope/preferences", anon),
				diff.Request{Path: "/api/v1/prayer_books/loc_2015/preferences", Headers: withHeaders(anon, map[string]string{"If-None-Match": "*"})},
			),
		}
	})
}

// withVolatile marks the fields a new shared office gets from the clock and
// the random short code.
func withVolatile(r diff.Request) diff.Request {
	r.Volatile = append(r.Volatile, "short_code", "share_path", "expires_at")
	return r
}
