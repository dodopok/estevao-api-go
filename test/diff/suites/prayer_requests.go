package suites

import (
	"net/http"
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// Weeks (all Sundays) that hold one request whose title switches the fake
// Perplexity into an error.
var perplexityErrorWeeks = [][2]string{
	{"2026-06-07", "PX-500"}, {"2026-06-14", "PX-429"}, {"2026-06-21", "PX-400"}, {"2026-06-28", "PX-badjson"},
	{"2026-07-12", "PX-nochoices"}, {"2026-07-19", "PX-empty"}, {"2026-07-26", "PX-url"}, {"2026-08-02", "PX-extra"},
	{"2026-08-09", "PX-long"}, {"2026-08-16", "PX-nobullet"},
}

var prayerRequestFixture = func() string {
	var b strings.Builder
	b.WriteString(userFixture)
	b.WriteString(`
DELETE FROM weekly_prayers WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM prayer_requests WHERE user_id BETWEEN 990001 AND 990099;
INSERT INTO users (id, provider_uid, email, preferences, premium_expires_at, timezone, created_at, updated_at) VALUES
 (990031, 'difftest-pr-en', 'pr-en@example.com', '{"prayer_book_code":"loc_1979_en"}', '2099-01-01', 'America/New_York', '2026-01-01', '2026-01-01'),
 (990032, 'difftest-pr-tz', 'pr-tz@example.com', '{}', '2099-01-01', 'Mars/Olympus', '2026-01-01', '2026-01-01'),
 (990033, 'difftest-pr-es', 'pr-es@example.com', '{"prayer_book_code":5}', '2099-01-01', 'UTC', '2026-01-01', '2026-01-01');
INSERT INTO prayer_requests (id, user_id, week_start, title, content, position, created_at, updated_at) VALUES
 (990001, 990002, '2026-09-13', 'Saúde da mãe', 'Recuperação da cirurgia', 0, '2026-09-13 10:00', '2026-09-13 10:00'),
 (990002, 990002, '2026-09-13', 'Trabalho', NULL, 1, '2026-09-13 11:00', '2026-09-13 11:00'),
 (990003, 990002, '2026-09-13', 'Paz', '', 1, '2026-09-13 09:00', '2026-09-13 09:00'),
 (990004, 990002, '2026-09-20', 'Esta semana', 'x', 0, '2026-09-20 10:00', '2026-09-20 10:00'),
 (990005, 990031, '2026-09-13', 'Alheio', NULL, 0, '2026-09-13 10:00', '2026-09-13 10:00'),
 (990006, 990002, '2026-08-23', 'Fonte', NULL, 0, '2026-08-23 10:00', '2026-08-23 10:00'),
 (990007, 990002, '2026-07-05', 'Old', 'c', 0, '2026-07-05 10:00', '2026-07-05 10:00'),
 (990008, 990002, '2026-05-31', 'Tia <Maria> & "João" \ / é', E'linha1\nlinha2\ttab\u0001 fim', 0, '2026-05-31 10:00', '2026-05-31 10:00'),
 (990009, 990031, '2026-09-13', 'Peace', 'For the world', 1, '2026-09-13 10:00', '2026-09-13 10:00');
INSERT INTO prayer_requests (id, user_id, week_start, title, content, position, created_at, updated_at)
SELECT 990100 + g, 990002, '2026-08-30', 'Cheia ' || g, NULL, g, '2026-08-30 10:00', '2026-08-30 10:00' FROM generate_series(0, 19) g;
`)
	for i, w := range perplexityErrorWeeks {
		b.WriteString("INSERT INTO prayer_requests (id, user_id, week_start, title, content, position, created_at, updated_at) VALUES (" +
			itoa(990200+i) + ", 990002, '" + w[0] + "', '" + w[1] + "', NULL, 0, '" + w[0] + " 10:00', '" + w[0] + " 10:00');\n")
	}
	b.WriteString(`
INSERT INTO weekly_prayers (id, user_id, week_start, prayer_book_code, generated_prayer, requests_digest, language, generated_at, created_at, updated_at) VALUES
 (990001, 990002, '2026-07-05', 'loc_2015', '• Cached old', md5('[["Old","c"]]'), 'pt-BR', '2026-07-05 12:00', '2026-07-05 12:00', '2026-07-05 12:00'),
 (990002, 990002, '2026-07-05', 'loc_2019_es', 'not a bullet', md5('[["Old","c"]]'), 'es', '2026-07-05 12:00', '2026-07-05 12:00', '2026-07-05 12:00'),
 (990003, 990002, '2026-09-13', 'loc_2015', '• Stale', 'stale-digest', 'pt-BR', '2026-09-13 12:00', '2026-09-13 12:00', '2026-09-13 12:00');
`)
	return b.String()
}()

func resetFakePerplexity() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__perplexity", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func init() {
	registerScenarios("prayer_requests", func() []diff.Scenario {
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		pr := userHeaders("difftest-us-onb", "us-onb@example.com")
		en := userHeaders("difftest-pr-en", "pr-en@example.com")
		tz := userHeaders("difftest-pr-tz", "pr-tz@example.com")
		es := userHeaders("difftest-pr-es", "pr-es@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		ts := []string{"created_at", "updated_at"}
		get := func(path string, h map[string]string, volatile ...string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: volatile}
		}
		del := func(path string, h map[string]string) diff.Request {
			return diff.Request{Method: "DELETE", Path: path, Headers: h}
		}
		tables := []string{"prayer_requests", "weekly_prayers"}
		snapshots := []string{
			`SELECT id, user_id, week_start, title, content, position, updated_at > '2026-09-24' FROM prayer_requests
			   WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`,
			`SELECT id, user_id, week_start, prayer_book_code, generated_prayer, requests_digest, language, generated_at > '2026-09-24'
			   FROM weekly_prayers WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`,
		}
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__perplexity"
		crud := []diff.Request{
			get("/api/v1/prayer_requests?week_start=2026-09-16", pr),
			get("/api/v1/prayer_requests?week_start=2026-09-13", en),
			get("/api/v1/prayer_requests", pr),
			get("/api/v1/prayer_requests", en),
			get("/api/v1/prayer_requests", tz),
			get("/api/v1/prayer_requests?week_start=16/09/2026", pr),
			get("/api/v1/prayer_requests?week_start=not-a-date", pr),
			get("/api/v1/prayer_requests?week_start=2026-02-30", pr),
			get("/api/v1/prayer_requests?week_start="+strings.Repeat("9", 130), pr),
			get("/api/v1/prayer_requests?week_start[a]=1", pr),
			get("/api/v1/prayer_requests?week_start[]=1", pr),
			get("/api/v1/prayer_requests?week_start=", pr),
			get("/api/v1/prayer_requests", plain),
			get("/api/v1/prayer_requests", anon),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Nova","content":"Detalhe","week_start":"2026-09-17"}}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"title":"Topo","position":"7","week_start":"2026-09-17"}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Sem semana"}}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos abc","position":"abc","week_start":"2026-09-10"}}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos 1.5","position":"1.5","week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos float","position":2.0,"week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos neg","position":-3,"week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos sp","position":" 5 ","week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos null","position":null,"week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos big","position":"99999999999","week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Pos bool","position":true,"week_start":"2026-09-10"}}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"","content":"`+strings.Repeat("x", 1001)+`","week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"`+strings.Repeat("á", 256)+`","week_start":"2026-09-10"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Cheia","week_start":"2026-08-31"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Data ruim","week_start":"xx"}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":"Data vazia","week_start":""},"week_start":"2026-09-10"}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":"texto"}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":false,"title":"falso"}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{},"title":"Vazio","week_start":"2026-09-10"}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":5,"content":true,"week_start":20260910}}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":[1],"week_start":"2026-09-10"}`),
			jsonReq("POST", "/api/v1/prayer_requests", pr, `{"prayer_request":{"title":{"a":1},"week_start":"2026-09-10"}}`),
			jsonReq("PATCH", "/api/v1/prayer_requests/990001", pr, `{"prayer_request":{"title":"Saúde da mãe","position":"0"}}`),
			jsonReq("PATCH", "/api/v1/prayer_requests/990001", pr, `{"prayer_request":{"content":"Nova descrição"}}`, "updated_at"),
			jsonReq("PUT", "/api/v1/prayer_requests/990002", pr, `{"position":"4"}`, "updated_at"),
			jsonReq("PATCH", "/api/v1/prayer_requests/990003", pr, `{"prayer_request":{"position":null}}`),
			jsonReq("PATCH", "/api/v1/prayer_requests/990003", pr, `{"prayer_request":{"position":"x","title":" "}}`),
			jsonReq("PATCH", "/api/v1/prayer_requests/990005", pr, `{"prayer_request":{"title":"Roubo"}}`),
			jsonReq("PATCH", "/api/v1/prayer_requests/990003abc", pr, `{"prayer_request":{"title":"Id sujo"}}`, "updated_at"),
			jsonReq("PATCH", "/api/v1/prayer_requests/abc", pr, `{"prayer_request":{"title":"x"}}`),
			del("/api/v1/prayer_requests/990004", pr),
			del("/api/v1/prayer_requests/990004", pr),
			del("/api/v1/prayer_requests/990005", pr),
			get("/api/v1/prayer_requests?week_start=2026-09-13", pr, ts...),
		}
		copySteps := []diff.Request{
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week?week_start=2026-09-20", pr, `{}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week?week_start=2026-09-20", pr, `{}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week?week_start=2026-08-30", pr, `{}`),
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week?week_start=2026-01-04", pr, `{}`),
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week", en, `{}`, ts...),
			jsonReq("POST", "/api/v1/prayer_requests/copy_previous_week?week_start=bad", pr, `{}`),
			get("/api/v1/prayer_requests?week_start=2026-09-20", pr, ts...),
		}
		weekly := []diff.Request{
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13", pr, "generated_at"),
			jsonReq("PATCH", "/api/v1/prayer_requests/990002", pr, `{"prayer_request":{"content":"Novo emprego"}}`, "updated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code=loc_1979_en", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code=loc_2019_es", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code=loc_1928_en", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code=nope", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code=", pr),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13&prayer_book_code="+strings.Repeat("x", 60), pr),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13", en, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-09-13", es),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-07-05", pr),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-07-05&prayer_book_code=loc_2019_es", pr),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2026-05-31", pr, "generated_at"),
			get("/api/v1/prayer_requests/weekly_prayer?week_start=2025-01-05", pr),
			get("/api/v1/prayer_requests/weekly_prayer", tz),
		}
		for _, w := range perplexityErrorWeeks {
			weekly = append(weekly, get("/api/v1/prayer_requests/weekly_prayer?week_start="+w[0], pr))
		}
		sc := func(name string, steps []diff.Request, extra ...string) diff.Scenario {
			return diff.Scenario{Name: name, Setup: prayerRequestFixture, Tables: tables, Steps: steps,
				Snapshot: snapshots, Prepare: resetFakePerplexity, HTTPSnapshots: extra}
		}
		return []diff.Scenario{
			sc("crud", crud),
			sc("copy previous week", copySteps),
			sc("weekly prayer", weekly, fake),
		}
	})
}
