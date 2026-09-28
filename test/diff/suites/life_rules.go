package suites

import "github.com/dodopok/estevao-api-go/test/diff"

// lifeRuleCleanup removes the fixture users' rules (their steps and the
// adoptions pointing at them would block deleting the users).
const lifeRuleCleanup = `
UPDATE life_rules SET original_life_rule_id = NULL WHERE original_life_rule_id IN (SELECT id FROM life_rules WHERE user_id BETWEEN 990001 AND 990099);
DELETE FROM life_rule_exam_items WHERE life_rule_exam_id IN (SELECT id FROM life_rule_exams WHERE user_id BETWEEN 990001 AND 990099);
DELETE FROM life_rule_exams WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM life_rule_steps WHERE life_rule_id IN (SELECT id FROM life_rules WHERE user_id BETWEEN 990001 AND 990099);
DELETE FROM life_rules WHERE user_id BETWEEN 990001 AND 990099;
UPDATE life_rules SET adoption_count = 0 WHERE translation_key IN ('saint_bernard', 'grace', 'triple_duty', 'acop_canada', 'simple', 'gloucester');
`

// lifeRuleHeapOrder rewrites the rule tables in id order once the fixture
// rows are in. The index and admin listings read them without a unique
// ORDER BY, so both sides must start from the same physical row order for
// PostgreSQL to hand them the rows in the same sequence.
const lifeRuleHeapOrder = `
CLUSTER life_rules USING life_rules_pkey;
CLUSTER life_rule_steps USING life_rule_steps_pkey;
`

var lifeRuleFixture = userFixture + `
INSERT INTO users (id, provider_uid, email, name, preferences, timezone, created_at, updated_at) VALUES
 (990040, 'difftest-admin', 'admin@example.com', 'Admin', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990050, 'difftest-lr-en', 'lr-en@example.com', 'EN', '{"language":"en"}', 'UTC', '2026-01-01', '2026-01-01'),
 (990051, 'difftest-lr-es', 'lr-es@example.com', 'ES', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990052, 'difftest-lr-pub', 'lr-pub@example.com', 'PUB', '{}', 'UTC', '2026-01-01', '2026-01-01');
INSERT INTO life_rules (id, user_id, translation_key, locale, icon, title, description, is_public, approved, adoption_count, created_at, updated_at) VALUES
 (990001, 990050, 'saint_bernard', 'en', '⛪', 'Saint Bernard Anglican Rule of Life', 'EN desc', TRUE, TRUE, 0, '2026-02-01', '2026-02-01'),
 (990002, 990051, 'saint_bernard', 'es', '⛪', 'Regla de Vida de San Bernardo', NULL, TRUE, TRUE, 3, '2026-02-01', '2026-02-01'),
 (990003, 990002, 'rule:difftest-own', 'pt-BR', '🙏', 'Minha regra', 'Pessoal', FALSE, FALSE, 0, '2026-03-01', '2026-03-01'),
 (990004, 990052, 'rule:difftest-pub', 'pt-BR', '✝️', 'Regra pública pendente', NULL, TRUE, FALSE, 7, '2026-03-02', '2026-03-02');
INSERT INTO life_rule_steps (id, life_rule_id, "order", title, description, created_at, updated_at) VALUES
 (990001, 990001, 1, 'Daily Prayer', 'Pray', '2026-02-01', '2026-02-01'),
 (990002, 990001, 2, 'Fasting', NULL, '2026-02-01', '2026-02-01'),
 (990003, 990003, 0, 'Orar', 'Manhã', '2026-03-01', '2026-03-01'),
 (990004, 990003, 1, 'Jejuar', NULL, '2026-03-01', '2026-03-01'),
 (990005, 990003, 2, 'Ler', NULL, '2026-03-01', '2026-03-01');
`

func init() {
	registerScenarios("life_rules", func() []diff.Scenario {
		owner := userHeaders("difftest-us-onb", "us-onb@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		admin := userHeaders("difftest-admin", "admin@example.com")
		en := userHeaders("difftest-lr-en", "lr-en@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		ts := []string{"created_at"}
		get := func(path string, h map[string]string, volatile ...string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: volatile}
		}
		body := func(inner string) string { return `{"life_rule":{` + inner + `}}` }
		tables := []string{"life_rules", "life_rule_steps"}
		snapshots := []string{
			`SELECT id, user_id, translation_key LIKE 'rule:%' OR translation_key IN ('saint_bernard'), locale, icon, title, description, is_public,
			   approved, adoption_count, original_life_rule_id FROM life_rules
			   WHERE user_id BETWEEN 990001 AND 990099 OR translation_key IN ('saint_bernard', 'grace') ORDER BY id`,
			`SELECT s.id, s.life_rule_id, s."order", s.title, s.description FROM life_rule_steps s JOIN life_rules r ON r.id = s.life_rule_id
			   WHERE r.user_id BETWEEN 990001 AND 990099 ORDER BY s.id`,
		}
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: lifeRuleFixture + lifeRuleHeapOrder, Tables: tables, Steps: steps, Snapshot: snapshots}
		}
		return []diff.Scenario{
			sc("read",
				get("/api/v1/life_rules", anon),
				get("/api/v1/life_rules?locale=en", anon),
				get("/api/v1/life_rules?language=es_ES", anon),
				get("/api/v1/life_rules?locale=pt_PT&sort=popular", anon),
				get("/api/v1/life_rules", owner),
				get("/api/v1/life_rules", en),
				get("/api/v1/life_rules?search=REGRA%20DE%20VIDA&limit=2&offset=1", owner),
				get("/api/v1/life_rules?offset=100", owner),
				get("/api/v1/life_rules?sort=popularity&limit=0", owner),
				get("/api/v1/life_rules/1?locale=en", anon),
				get("/api/v1/life_rules/990002", anon),
				get("/api/v1/life_rules/990003", owner),
				get("/api/v1/life_rules/990003", plain),
				get("/api/v1/life_rules/990004", anon),
				get("/api/v1/life_rules/999999", anon),
				get("/api/v1/life_rules/my", owner),
				get("/api/v1/life_rules/my?locale=en", en),
				get("/api/v1/life_rules/my", plain),
				get("/api/v1/life_rules/my", anon),
				get("/api/v1/admin/life_rules", admin),
				get("/api/v1/admin/life_rules?status=pending", admin),
				get("/api/v1/admin/life_rules?status=approved&search=_%25&limit=2&offset=1", admin),
				get("/api/v1/admin/life_rules?status=bogus", admin),
				get("/api/v1/admin/life_rules?status[]=all", admin),
				get("/api/v1/admin/life_rules", owner),
			),
			sc("write",
				jsonReq("POST", "/api/v1/life_rules", plain, body(`"icon":"🕯","title":"Nova","locale":"en_US","life_rule_steps_attributes":[{"order":1,"title":"A"},{"order":1,"title":"B","description":"dup order ok"}]`), ts...),
				jsonReq("POST", "/api/v1/life_rules", plain, body(`"icon":"","title":"","is_public":null,"life_rule_steps_attributes":[{"order":-1,"title":""},{"title":"x"},{"order":"1.5","title":"y"}]`)),
				jsonReq("POST", "/api/v1/life_rules", plain, body(`"icon":"i","title":"T","life_rule_steps_attributes":[{"id":990003,"title":"stolen"}]`)),
				jsonReq("POST", "/api/v1/life_rules", plain, `{}`),
				jsonReq("POST", "/api/v1/life_rules", plain, `{"icon":"🌿","title":"Envolvida pelo wrapper"}`, ts...),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, body(`"title":"Minha regra editada","life_rule_steps_attributes":[{"id":990003,"title":"Orar mais"},{"id":990004,"_destroy":"1"},{"order":9,"title":"Novo"}]`), ts...),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, body(`"life_rule_steps_attributes":[{"id":990005,"order":0}]`)),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, body(`"life_rule_steps_attributes":[{"id":990001,"title":"x"}]`)),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, body(`"life_rule_steps_attributes":{"0":{"order":10,"title":"Hash"}}`), ts...),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, body(`"is_public":"yes","locale":"es-AR"`), ts...),
				jsonReq("PATCH", "/api/v1/life_rules/990001", owner, body(`"title":"x"`)),
				jsonReq("PATCH", "/api/v1/life_rules/999999", owner, body(`"title":"x"`)),
				jsonReq("PATCH", "/api/v1/life_rules/990003", owner, `{}`),
				jsonReq("POST", "/api/v1/life_rules/1/adopt?locale=en", plain, `{}`, ts...),
				jsonReq("POST", "/api/v1/life_rules/2/adopt", plain, `{}`, ts...),
				jsonReq("POST", "/api/v1/life_rules/990004/adopt", plain, `{}`),
				jsonReq("POST", "/api/v1/life_rules/990002/adopt", en, `{}`, ts...),
				jsonReq("POST", "/api/v1/life_rules/990004/approve", owner, `{}`),
				jsonReq("POST", "/api/v1/life_rules/990003/approve", admin, `{}`),
				jsonReq("POST", "/api/v1/life_rules/990004/approve", admin, `{}`, ts...),
				jsonReq("POST", "/api/v1/life_rules", owner, body(`"icon":"🔥","title":"Substitui","is_public":true`), ts...),
				diff.Request{Method: "DELETE", Path: "/api/v1/life_rules/990004", Headers: owner},
				diff.Request{Method: "DELETE", Path: "/api/v1/life_rules/990001", Headers: en},
				get("/api/v1/life_rules?locale=en", plain, "created_at", "updated_at"),
			),
		}
	})
}
