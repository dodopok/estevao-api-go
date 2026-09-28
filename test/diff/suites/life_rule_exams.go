package suites

import (
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// examFixture gives the onboarded user (990002, rule 990003) two completed
// monthly exams and the English user (990050, rule 990001) an open draft.
var examFixture = lifeRuleFixture + `
INSERT INTO life_rule_exams (id, user_id, client_id, life_rule_id, life_rule_title, period, period_start, period_end, status,
  score, band, reflection, intention, focus_step_id, completed_at, created_at, updated_at) VALUES
 (990001, 990002, 'exam-jul', 990003, 'Minha regra', 'monthly', '2026-07-01', '2026-07-31', 'completed', 50, 'growing',
  'Julho', 'Mais oração', 990003, '2026-07-31 21:00:00', '2026-07-30 10:00:00', '2026-07-31 21:00:00'),
 (990002, 990002, 'exam-aug', 990003, 'Minha regra', 'monthly', '2026-08-01', '2026-08-31', 'completed', 58, 'growing',
  '', '', NULL, '2026-08-31 21:00:00', '2026-08-30 10:00:00', '2026-08-31 21:00:00'),
 (990003, 990050, 'exam-en', 990001, 'Saint Bernard Anglican Rule of Life', 'weekly', '2026-09-21', '2026-09-27', 'draft',
  NULL, NULL, 'draft', '', NULL, NULL, '2026-09-22 10:00:00', '2026-09-22 10:00:00');
INSERT INTO life_rule_exam_items (id, life_rule_exam_id, client_id, life_rule_step_id, step_title, step_description, step_order,
  rating, note, created_at, updated_at) VALUES
 (990001, 990001, 'step-990003', 990003, 'Orar', 'Manhã', 0, 'faithful', 'bem', '2026-07-30 10:00:00', '2026-07-31 21:00:00'),
 (990002, 990001, 'step-990004', 990004, 'Jejuar', '', 1, 'missed', '', '2026-07-30 10:00:00', '2026-07-31 21:00:00'),
 (990003, 990001, 'step-990005', 990005, 'Ler', '', 2, 'not_applicable', '', '2026-07-30 10:00:00', '2026-07-31 21:00:00'),
 (990004, 990002, 'step-990003', 990003, 'Orar', 'Manhã', 0, 'mostly', '', '2026-08-30 10:00:00', '2026-08-31 21:00:00'),
 (990005, 990002, 'step-990004', 990004, 'Jejuar', '', 1, 'rarely', 'difícil', '2026-08-30 10:00:00', '2026-08-31 21:00:00'),
 (990006, 990002, 'step-990005', 990005, 'Ler', '', 2, 'partial', '', '2026-08-30 10:00:00', '2026-08-31 21:00:00'),
 (990007, 990003, 'step-990001', 990001, 'Daily Prayer', 'Pray', 1, 'faithful', '', '2026-09-22 10:00:00', '2026-09-22 10:00:00'),
 (990008, 990003, 'step-990002', 990002, 'Fasting', '', 2, NULL, '', '2026-09-22 10:00:00', '2026-09-22 10:00:00');
`

func init() {
	registerScenarios("life_rule_exams", func() []diff.Scenario {
		owner := userHeaders("difftest-us-onb", "us-onb@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		en := userHeaders("difftest-lr-en", "lr-en@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		ts := []string{"created_at", "updated_at", "completed_at"}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: ts}
		}
		body := func(inner string) string { return `{"life_rule_exam":{` + inner + `}}` }
		tables := []string{"life_rule_exams", "life_rule_exam_items"}
		snapshots := []string{
			`SELECT id, user_id, client_id, life_rule_id, life_rule_title, period, period_start, period_end, season_slug, season_name,
			   status, score, band, reflection, intention, focus_step_id, completed_at IS NOT NULL FROM life_rule_exams
			   WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`,
			`SELECT i.id, i.life_rule_exam_id, i.client_id, i.life_rule_step_id, i.step_title, i.step_description, i.step_order, i.rating, i.note
			   FROM life_rule_exam_items i JOIN life_rule_exams e ON e.id = i.life_rule_exam_id
			   WHERE e.user_id BETWEEN 990001 AND 990099 ORDER BY i.id`,
		}
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: examFixture + lifeRuleHeapOrder, Tables: tables, Steps: steps, Snapshot: snapshots}
		}
		del := func(path string, h map[string]string) diff.Request {
			return diff.Request{Method: "DELETE", Path: path, Headers: h}
		}
		post := func(path string, h map[string]string, body string) diff.Request {
			return jsonReq("POST", path, h, body, ts...)
		}
		patch := func(path string, h map[string]string, body string) diff.Request {
			return jsonReq("PATCH", path, h, body, ts...)
		}
		return []diff.Scenario{
			sc("read",
				get("/api/v1/life_rule_exams", owner),
				get("/api/v1/life_rule_exams?life_rule_id=990003&limit=1&offset=1", owner),
				get("/api/v1/life_rule_exams?life_rule_id=abc", owner),
				get("/api/v1/life_rule_exams?life_rule_id[]=990003&life_rule_id[]=", owner),
				get("/api/v1/life_rule_exams?limit=0&offset=-5", owner),
				get("/api/v1/life_rule_exams", en),
				get("/api/v1/life_rule_exams", plain),
				get("/api/v1/life_rule_exams", anon),
				get("/api/v1/life_rule_exams/990001", owner),
				get("/api/v1/life_rule_exams/990002", owner),
				get("/api/v1/life_rule_exams/990003", owner),
				get("/api/v1/life_rule_exams/990003", en),
				get("/api/v1/life_rule_exams/abc", owner),
				get("/api/v1/life_rule_exams/current", owner),
				get("/api/v1/life_rule_exams/current", en),
				get("/api/v1/life_rule_exams/current", anon),
				get("/api/v1/life_rule_exams/stats", owner),
				get("/api/v1/life_rule_exams/stats", en),
				get("/api/v1/life_rule_exams/stats", plain),
			),
			sc("lifecycle",
				post("/api/v1/life_rule_exams", plain, body(`"client_id":"p1"`)),
				post("/api/v1/life_rule_exams", owner, body(`"period":"weekly"`)),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"exam-jul"`)),
				post("/api/v1/life_rule_exams", owner, `{"client_id":"c1","period":"weekly","reflection":"ignored"}`),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"c2","period":"monthly"`)),
				get("/api/v1/life_rule_exams/current", owner),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"reflection":"Semana boa","intention":"Jejuar","focus_step_id":990004,
					"period":"custom","items_attributes":[{"client_id":"step-990003","rating":"faithful","note":"diário"},{"id":990010,"rating":"rarely"},
					{"id":999999,"rating":"missed"},{"rating":"missed"}]`)),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"items_attributes":[{"id":990011,"rating":"bogus"}]`)),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"items_attributes":[{"id":990011,"note":"`+strings.Repeat("n", 2001)+`"}]`)),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"reflection":"`+strings.Repeat("r", 5001)+`"`)),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"items_attributes":{"0":{"id":990011,"rating":"partial"}}`)),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"items_attributes":"x"`)),
				patch("/api/v1/life_rule_exams/990004", owner, `{"life_rule_exam":"x"}`),
				patch("/api/v1/life_rule_exams/990004", owner, `{"life_rule_exam":null}`),
				patch("/api/v1/life_rule_exams/990003", owner, body(`"reflection":"x"`)),
				patch("/api/v1/life_rule_exams/990001", owner, body(`"reflection":"x"`)),
				post("/api/v1/life_rule_exams/990004/complete", owner, body(`"intention":"Ler mais","items_attributes":[{"client_id":"step-990005","rating":"mostly"}]`)),
				post("/api/v1/life_rule_exams/990004/complete", owner, `{}`),
				patch("/api/v1/life_rule_exams/990004", owner, body(`"reflection":"late"`)),
				get("/api/v1/life_rule_exams/stats", owner),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"s1","period":"seasonal"`)),
				post("/api/v1/life_rule_exams/990005/complete", owner, `{}`),
				del("/api/v1/life_rule_exams/990005", owner),
				del("/api/v1/life_rule_exams/990005", owner),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"x1","period":"custom","period_start":"2026/03/05","period_end":"1 mar 2026"`)),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"x2","period":"yearly","period_start":"garbage","period_end":20261231`)),
				patch("/api/v1/life_rule_exams/990006", owner, body(`"reflection":null`)),
				del("/api/v1/life_rule_exams/990006", owner),
				post("/api/v1/life_rule_exams", owner, body(`"client_id":"`+strings.Repeat("c", 121)+`"`)),
				post("/api/v1/life_rule_exams", en, body(`"client_id":"en-2"`)),
				post("/api/v1/life_rule_exams/990003/complete", en, `{}`),
				del("/api/v1/life_rule_exams/990001", owner),
				get("/api/v1/life_rule_exams", owner),
				post("/api/v1/life_rule_exams/990002/complete", anon, `{}`),
			),
		}
	})
}
