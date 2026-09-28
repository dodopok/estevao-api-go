package suites

import (
	"net/http"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// rosaryCleanup removes the fixture prayers first: their blocks reference
// them, so the users fixture could not delete their owners.
const rosaryCleanup = `
DELETE FROM custom_rosary_steps WHERE custom_rosary_block_id IN (SELECT b.id FROM custom_rosary_blocks b
  JOIN custom_rosary_prayers p ON p.id = b.custom_rosary_prayer_id WHERE p.user_id BETWEEN 990001 AND 990099 OR p.id BETWEEN 990001 AND 990099);
DELETE FROM custom_rosary_blocks WHERE custom_rosary_prayer_id IN (SELECT id FROM custom_rosary_prayers
  WHERE user_id BETWEEN 990001 AND 990099 OR id BETWEEN 990001 AND 990099);
DELETE FROM custom_rosary_prayers WHERE user_id BETWEEN 990001 AND 990099 OR id BETWEEN 990001 AND 990099;
`

// rosaryFixture adds custom rosary prayers (ids 990001-990099) for the users
// fixture, in each publication state the user flows can move.
var rosaryFixture = userFixture + `
INSERT INTO users (id, provider_uid, email, name, preferences, timezone, created_at, updated_at) VALUES
 (990040, 'difftest-admin', 'admin@example.com', 'Admin', '{}', 'UTC', '2026-01-01', '2026-01-01');
INSERT INTO custom_rosary_prayers (id, user_id, client_id, title, description, locale, cycle_repeat, is_public, share_status,
  moderation_decision, publication_status, strapi_document_id, source_hash, source_revision, publication_category, strapi_slug,
  created_at, updated_at) VALUES
 (990001, 990002, 'c-1', 'Terço A', 'Descrição', 'pt-BR', 4, FALSE, 'private', NULL, 'pending', NULL, NULL, NULL, NULL, NULL,
  '2026-09-01 10:00', '2026-09-01 10:00'),
 (990002, 990002, 'c-2', 'Terço publicado', NULL, 'pt-BR', 1, TRUE, 'approved', 'approved', 'published', 'doc-990002', 'h', 'rev-2',
  '{"mode":"existing","slug":"marian"}', 'terco-publicado', '2026-09-02 10:00', '2026-09-02 10:00'),
 (990003, 990001, 'c-3', 'Alheio', NULL, 'en', 1, FALSE, 'private', NULL, 'pending', NULL, NULL, NULL, NULL, NULL,
  '2026-09-03 10:00', '2026-09-03 10:00'),
 (990004, 990002, 'c-4', 'Voltando', NULL, 'es', 2, FALSE, 'private', 'approved', 'unpublished', 'doc-990004', 'h', 'rev-4',
  '{"mode":"new","slug":"new-cat","name":"Nova","description":null,"icon":null}', NULL, '2026-09-04 10:00', '2026-09-04 10:00'),
 (990005, 990002, 'c-5', 'Some', NULL, 'pt-PT', 1, TRUE, 'approved', 'approved', 'published', 'doc-missing', 'h', 'rev-5',
  '{"mode":"existing","slug":"x"}', 'some', '2026-09-05 10:00', '2026-09-05 10:00'),
 (990006, 990002, 'c-6', 'Apagar', NULL, 'pt-BR', 1, TRUE, 'approved', 'approved', 'published', 'doc-990006', 'h', 'rev-6',
  '{"mode":"existing","slug":"x"}', 'apagar', '2026-09-06 10:00', '2026-09-06 10:00'),
 (990007, 990001, 'c-7', 'Fila 100%_x', 'Pedido', 'en', 3, TRUE, 'pending_review', NULL, 'pending', NULL, NULL, NULL, NULL, NULL,
  '2026-09-07 10:00', '2026-09-07 10:00'),
 (990008, 990001, 'c-8', 'STRAPI-422 falha', NULL, 'pt-BR', 1, TRUE, 'pending_review', NULL, 'pending', NULL, NULL, NULL, NULL, NULL,
  '2026-09-08 10:00', '2026-09-08 10:00'),
 (990009, 990002, 'c-9', 'Rejeitar publicado', NULL, 'pt-BR', 1, TRUE, 'approved', 'approved', 'published', 'doc-990009', 'h', 'rev-9',
  '{"mode":"existing","slug":"x"}', 'rej', '2026-09-09 10:00', '2026-09-09 10:00');
INSERT INTO custom_rosary_blocks (id, custom_rosary_prayer_id, client_id, position, name, repeat_count, in_cycle, created_at, updated_at) VALUES
 (990001, 990001, 'b-1', 1, 'Abertura', 1, FALSE, '2026-09-01 10:00', '2026-09-01 10:00'),
 (990002, 990001, 'b-2', 2, 'Semanas', 1, TRUE, '2026-09-01 10:00', '2026-09-01 10:00'),
 (990003, 990002, 'b-3', 1, NULL, 1, FALSE, '2026-09-02 10:00', '2026-09-02 10:00'),
 (990004, 990004, 'b-4', 1, 'Única', 2, TRUE, '2026-09-04 10:00', '2026-09-04 10:00'),
 (990005, 990003, 'b-5', 1, NULL, 1, FALSE, '2026-09-03 10:00', '2026-09-03 10:00'),
 (990006, 990007, 'b-6', 1, 'Voltas', 2, TRUE, '2026-09-07 10:00', '2026-09-07 10:00'),
 (990007, 990008, 'b-7', 1, NULL, 1, FALSE, '2026-09-08 10:00', '2026-09-08 10:00');
INSERT INTO custom_rosary_steps (id, custom_rosary_block_id, client_id, position, step_type, title, text, repeat_count, created_at, updated_at) VALUES
 (990001, 990001, 's-1', 1, 'cross', 'Cruz', 'Creio em Deus', 1, '2026-09-01 10:00', '2026-09-01 10:00'),
 (990002, 990002, 's-2', 1, 'cruciform', 'Cruciforme', 'Santo Deus', 1, '2026-09-01 10:00', '2026-09-01 10:00'),
 (990003, 990002, 's-3', 2, 'week', 'Conta de semana', 'Senhor Jesus', 7, '2026-09-01 10:00', '2026-09-01 10:00'),
 (990004, 990003, 's-4', 1, 'invitatory', 'Convite', 'Vinde', 1, '2026-09-02 10:00', '2026-09-02 10:00'),
 (990005, 990004, 's-5', 1, 'week', 'Conta', 'Ave', 3, '2026-09-04 10:00', '2026-09-04 10:00'),
 (990006, 990004, 's-6', 2, 'cruciform', 'Cruz', 'Glória', 1, '2026-09-04 10:00', '2026-09-04 10:00'),
 (990007, 990005, 's-7', 1, 'cross', 'Cruz', 'Texto', 1, '2026-09-03 10:00', '2026-09-03 10:00'),
 (990008, 990006, 's-8', 1, 'cruciform', 'Cruciform', 'Holy', 1, '2026-09-07 10:00', '2026-09-07 10:00'),
 (990009, 990006, 's-9', 2, 'week', 'Week bead', 'Lord', 7, '2026-09-07 10:00', '2026-09-07 10:00'),
 (990010, 990007, 's-10', 1, 'cross', 'Cruz', 'X', 1, '2026-09-08 10:00', '2026-09-08 10:00');
`

func resetFakeStrapi() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__strapi", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

const newRosaryTree = `"blocks_attributes":[
 {"client_id":"nb-1","position":1,"name":"Abertura","steps_attributes":[{"client_id":"ns-1","position":1,"step_type":"cross","title":"Cruz","text":"Creio"}]},
 {"client_id":"nb-2","position":2,"in_cycle":true,"repeat_count":1,"steps_attributes":[
   {"client_id":"ns-2","position":1,"step_type":"cruciform","title":"Cruciforme","text":"Santo"},
   {"client_id":"ns-3","position":2,"step_type":"week","title":"Semana","text":"Jesus","repeat_count":7}]}]`

func init() {
	registerScenarios("custom_rosary", func() []diff.Scenario {
		owner := userHeaders("difftest-us-onb", "us-onb@example.com")
		other := userHeaders("difftest-us-plain", "us-plain@example.com")
		admin := userHeaders("difftest-admin", "admin@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		ts := []string{"created_at", "updated_at"}
		get := func(path string, h map[string]string, volatile ...string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: volatile}
		}
		body := func(inner string) string { return `{"custom_rosary_prayer":{` + inner + `}}` }
		post := func(inner string, volatile ...string) diff.Request {
			return jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, body(inner), volatile...)
		}
		patch := func(id, inner string, h map[string]string, volatile ...string) diff.Request {
			return jsonReq("PATCH", "/api/v1/custom_rosary_prayers/"+id, h, body(inner), volatile...)
		}
		del := func(id string, h map[string]string) diff.Request {
			return diff.Request{Method: "DELETE", Path: "/api/v1/custom_rosary_prayers/" + id, Headers: h}
		}
		tables := []string{"custom_rosary_prayers", "custom_rosary_blocks", "custom_rosary_steps"}
		snapshots := []string{
			`SELECT id, user_id, client_id, title, description, locale, cycle_repeat, is_public, share_status, moderation_decision,
			   moderation_reentry_count, last_moderation_reentry_at IS NOT NULL, reviewed_at IS NOT NULL, publication_status,
			   publication_error, publication_retry_at IS NOT NULL, publication_started_at IS NOT NULL, strapi_document_id, strapi_slug,
			   source_revision, publication_attempts, published_at IS NOT NULL
			   FROM custom_rosary_prayers WHERE user_id BETWEEN 990001 AND 990099 ORDER BY id`,
			`SELECT count(*) FROM solid_queue_jobs WHERE finished_at IS NULL AND class_name LIKE 'CustomRosary%'`,
			`SELECT b.id, b.custom_rosary_prayer_id, b.client_id, b.position, b.name, b.repeat_count, b.in_cycle FROM custom_rosary_blocks b
			   JOIN custom_rosary_prayers p ON p.id = b.custom_rosary_prayer_id WHERE p.user_id BETWEEN 990001 AND 990099 ORDER BY b.id`,
			`SELECT s.id, s.custom_rosary_block_id, s.client_id, s.position, s.step_type, s.title, s.text, s.repeat_count FROM custom_rosary_steps s
			   JOIN custom_rosary_blocks b ON b.id = s.custom_rosary_block_id JOIN custom_rosary_prayers p ON p.id = b.custom_rosary_prayer_id
			   WHERE p.user_id BETWEEN 990001 AND 990099 ORDER BY s.id`,
		}
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__strapi"
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: rosaryFixture, Tables: tables, Steps: steps, Snapshot: snapshots,
				Prepare: resetFakeStrapi, HTTPSnapshots: []string{fake},
				Settle: settleJobs("CustomRosaryPrayers::PublishJob", "CustomRosaryPrayers::UnpublishJob")}
		}
		return []diff.Scenario{
			sc("index",
				get("/api/v1/custom_rosary_prayers", owner),
				get("/api/v1/custom_rosary_prayers?limit=2", owner),
				get("/api/v1/custom_rosary_prayers?limit=2&offset=2", owner),
				get("/api/v1/custom_rosary_prayers?offset=-5&limit=500", owner),
				get("/api/v1/custom_rosary_prayers?limit=abc", owner),
				get("/api/v1/custom_rosary_prayers?updated_after=2026-09-03T12:00:00-03:00", owner),
				get("/api/v1/custom_rosary_prayers?updated_after=garbage", owner),
				get("/api/v1/custom_rosary_prayers?updated_after=2026-09-04T10:00:00.5", owner),
				get("/api/v1/custom_rosary_prayers?limit[]=1", owner),
				get("/api/v1/custom_rosary_prayers", other),
				get("/api/v1/custom_rosary_prayers", anon),
			),
			sc("create",
				post(`"client_id":"new-1","title":"Novo terço","description":"D","locale":"pt-BR","cycle_repeat":4,`+newRosaryTree, ts...),
				post(`"client_id":"new-1","title":"Novo terço (reenvio)",`+newRosaryTree, ts...),
				post(`"client_id":"new-2","title":"Hash","blocks_attributes":{"0":{"position":1,"steps_attributes":{"0":{"position":1,"step_type":"week","title":"T","text":"X"}}},"1":{"position":2}}`, ts...),
				post(`"client_id":"new-3","title":"","locale":"xx","cycle_repeat":13,"blocks_attributes":[{"position":0,"steps_attributes":[{"position":1,"step_type":"x","title":"","text":""}]},{"position":0,"repeat_count":21,"in_cycle":null}]`),
				post(`"client_id":"new-4","title":"Posições","blocks_attributes":[{"position":1},{"position":1}]`),
				post(`"client_id":"new-5","title":"Grande","cycle_repeat":12,"blocks_attributes":[{"position":1,"in_cycle":true,"repeat_count":20,"steps_attributes":[{"position":1,"step_type":"week","title":"W","text":"T","repeat_count":20}]}]`),
				post(`"client_id":"new-6","title":"Id alheio","blocks_attributes":[{"id":990005,"position":1}]`),
				post(`"client_id":"c-1","title":"Sobrescreve c-1","blocks_attributes":[{"id":990001,"position":3},{"position":4,"_destroy":"1"}]`, ts...),
				post(`"client_id":"new-7","title":"Tipos","cycle_repeat":"2","is_public":"true","locale":5,"blocks_attributes":[{"position":"1","in_cycle":"false","steps_attributes":[{"position":"1.5","step_type":"cross","title":"T","text":"X"}]}]`),
				post(`"client_id":"new-8","title":"Público","is_public":true,"blocks_attributes":[]`, ts...),
				post(`"client_id":"new-9","title":"Sem blocos","blocks_attributes":{"position":1}`),
				post(`"client_id":"   ","title":"Branco"`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, `{}`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, `{"title":"Sem chave","client_id":"wrapped-1","blocks_attributes":[{"position":1}]}`, ts...),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, `{bad json`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", anon, `{bad json`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, `{"custom_rosary_prayer":"x"}`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", owner, `{"custom_rosary_prayer":{}}`),
				jsonReq("POST", "/api/v1/custom_rosary_prayers", anon, body(`"client_id":"z","title":"z"`)),
			),
			sc("update",
				patch("990001", `"title":"Terço A editado","blocks_attributes":[{"id":990001,"position":1,"name":"Abertura nova"},{"position":3,"steps_attributes":[{"position":1,"step_type":"dismissal","title":"Fim","text":"Amém"}]}]`, owner, ts...),
				patch("990001", `"description":"Só descrição"`, owner, ts...),
				patch("990001", `"title":"Terço A editado"`, owner, ts...),
				patch("990001", `"blocks_attributes":[{"id":990001,"steps_attributes":[{"id":990001,"_destroy":true},{"position":2,"step_type":"reflection","title":"R","text":"Reflexão"}]}]`, owner, ts...),
				patch("990001", `"blocks_attributes":[{"id":990001,"steps_attributes":[{"id":"990002"}]}]`, owner),
				patch("990001", `"blocks_attributes":[{"id":"990001x"}]`, owner),
				patch("990002", `"title":"Terço publicado (editado)"`, owner, ts...),
				patch("990004", `"is_public":true`, owner, ts...),
				patch("990005", `"is_public":false`, owner, ts...),
				patch("990003", `"title":"Roubado"`, owner),
				jsonReq("PATCH", "/api/v1/custom_rosary_prayers/990003", owner, `{}`),
				patch("999999", `"title":"x"`, owner),
				patch("abc", `"title":"x"`, owner),
				get("/api/v1/custom_rosary_prayers", owner, ts...),
			),
			sc("moderation",
				get("/api/v1/admin/custom_rosary_prayers", admin),
				get("/api/v1/admin/custom_rosary_prayers?share_status=pending_review&sort=author&direction=asc", admin),
				get("/api/v1/admin/custom_rosary_prayers?search=100%25_&limit=1", admin),
				get("/api/v1/admin/custom_rosary_prayers?search=us-plain&sort=reentries&offset=1", admin),
				get("/api/v1/admin/custom_rosary_prayers?share_status[]=approved&share_status[]=rejected&sort=nope", admin),
				get("/api/v1/admin/custom_rosary_prayers/990007", admin),
				get("/api/v1/admin/custom_rosary_prayers/999999", admin),
				get("/api/v1/admin/custom_rosary_prayers", owner),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990007/approve", admin, `{"category":{"mode":"bogus"}}`),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990007/approve", admin, `{"category":"x"}`),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990007/approve", admin, `{"category":{"mode":"new","slug":"a b","name":"N"}}`),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990007/approve", admin,
					`{"category":{"mode":"new","slug":"fila","name":" Fila ","description":"","icon":"star"},"strapi_slug":"fila-custom"}`, "reviewed_at", "updated_at"),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990008/approve", admin, `{"category":{"mode":"existing","slug":"x","documentId":"cat-1"}}`, "reviewed_at", "updated_at"),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990009/reject", admin, `{"reason":"Fora do tema"}`, "reviewed_at", "updated_at"),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990003/reject", admin, `{}`, "reviewed_at", "updated_at"),
				jsonReq("POST", "/api/v1/admin/custom_rosary_prayers/990002/approve", admin, `{"category":{"mode":"existing","slug":"marian"}}`, "reviewed_at", "updated_at"),
				get("/api/v1/admin/rosary_categories?locale=xx", admin),
			),
			sc("destroy",
				del("990003", owner),
				del("999999", owner),
				del("990006", owner),
				del("990001", owner),
				del("990001", owner),
				get("/api/v1/custom_rosary_prayers", owner),
			),
		}
	})
}
