package suites

import (
	"net/http"
	"regexp"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// audioJobsFixture adds to the admin audio fixture: a small prayer book in
// a language without Bible versions (its sources are only its own texts and
// collects), a four-verse corpus, clips whose text makes the fake provider
// fail, and an OpenAI customization.
var audioJobsFixture = adminAudioFixture + `
DELETE FROM audio_clip_candidates WHERE audio_clip_id > 990005;
DELETE FROM audio_clip_usages WHERE audio_clip_id > 990005 OR prayer_book_code IN ('dtaj_fr', 'loc_2021');
DELETE FROM user_audio_usages WHERE audio_clip_id > 990005;
DELETE FROM audio_clips WHERE id > 990005;
DELETE FROM audio_clip_customizations WHERE created_by = 'difftest-aj';
DELETE FROM liturgical_texts WHERE prayer_book_id = 990501;
DELETE FROM collects WHERE prayer_book_id = 990501;
DELETE FROM prayer_books WHERE id = 990501;
DELETE FROM bible_texts WHERE translation = 'dtaj_tiny';
INSERT INTO prayer_books (id, code, name, language, "order", features, created_at, updated_at) VALUES
 (990501, 'dtaj_fr', 'Difftest', 'fr', 9999, '{}', '2026-01-01', '2026-01-01');
INSERT INTO liturgical_texts (id, prayer_book_id, slug, category, content, created_at, updated_at) VALUES
 (990501, 990501, 'dtaj_opening', 'opening', 'Abri os meus lábios, Senhor.', '2026-01-01', '2026-01-01'),
 (990502, 990501, 'dtaj_rubric', 'rubric', 'O oficiante diz:', '2026-01-01', '2026-01-01'),
 (990503, 990501, 'dtaj_long', 'prayer', repeat('Senhor, ouvi a nossa oração e atendei-nos na vossa bondade. ', 40), '2026-01-01', '2026-01-01'),
 (990504, 990501, 'dtaj_amen', 'congregation', 'Amém.', '2026-01-01', '2026-01-01'),
 (990505, 990501, 'dtaj_ref', 'text', '**Bendito** seja Deus. __(Sl 113.4)__', '2026-01-01', '2026-01-01');
INSERT INTO collects (id, prayer_book_id, text, created_at, updated_at) VALUES
 (990501, 990501, 'Ó Deus, que nos deste este dia, guarda-nos em tua paz; por Jesus Cristo. Amém.', '2026-01-01', '2026-01-01'),
 (990502, 990501, 'Ó Deus, que nos deste este dia, guarda-nos em tua paz; por Jesus Cristo. Amém.', '2026-01-01', '2026-01-01'),
 (990503, 990501, NULL, '2026-01-01', '2026-01-01');
INSERT INTO bible_texts (id, translation, book, book_number, chapter, verse, text, created_at, updated_at) VALUES
 (99000004, 'dtaj_tiny', 'Salmos', 19, 23, 2, 'Em verdes pastagens me faz repousar.', '2026-01-01', '2026-01-01'),
 (99000001, 'dtaj_tiny', 'Gênesis', 1, 1, 1, 'No princípio <S>7225</S> criou Deus os céus e a terra.', '2026-01-01', '2026-01-01'),
 (99000002, 'dtaj_tiny', 'Gênesis', 1, 1, 2, 'A terra era sem forma e vazia.<sup>a</sup>', '2026-01-01', '2026-01-01'),
 (99000003, 'dtaj_tiny', 'Salmos', 19, 23, 1, 'O Senhor é o meu pastor; nada me faltará.', '2026-01-01', '2026-01-01');
INSERT INTO audio_clips (id, key, filename, text, line_type, kind, provider, voice, model, speed, duration, character_count, language,
  instructions_sha256, configuration_fingerprint, custom_instructions_sha256, created_at, updated_at) VALUES
 (990006, 'dtaj-6', 'tts/openai/dtaj-6.mp3', 'Falha TTS-500 aqui.', 'leader', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 1.0, 19, 'pt-BR', NULL, NULL, NULL, '2026-01-06 10:00', '2026-01-06 10:00'),
 (990007, 'dtaj-7', 'tts/openai/dtaj-7.mp3', 'Limite TTS-429 aqui.', 'leader', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 1.0, 20, 'pt-BR', NULL, NULL, NULL, '2026-01-07 10:00', '2026-01-07 10:00'),
 (990008, 'dtaj-8', 'tts/openai/dtaj-8.mp3', 'Recusado TTS-400 aqui.', 'rubric', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 1.0, 22, 'pt-BR', NULL, NULL, NULL, '2026-01-08 10:00', '2026-01-08 10:00');
INSERT INTO audio_clip_customizations (text_digest, text, instructions, instructions_sha256, provider, model, voice, language, status, created_by, created_at, updated_at) VALUES
 (encode(sha256('Glória ao Pai e ao Filho e ao Espírito Santo.'::bytea), 'hex'), 'Glória ao Pai e ao Filho e ao Espírito Santo.', 'Com alegria', encode(sha256('Com alegria'::bytea), 'hex'), 'openai', 'gpt-4o-mini-tts', 'cedar', 'pt-BR', 'active', 'difftest-aj', '2026-01-01', '2026-01-01');
`

const audioJobsTeardown = `
DELETE FROM audio_clip_candidates WHERE audio_clip_id > 990005;
DELETE FROM audio_clip_usages WHERE audio_clip_id > 990005 OR prayer_book_code IN ('dtaj_fr', 'loc_2021');
DELETE FROM user_audio_usages WHERE audio_clip_id > 990005;
DELETE FROM audio_clips WHERE id > 990005;
DELETE FROM liturgical_texts WHERE prayer_book_id = 990501;
DELETE FROM collects WHERE prayer_book_id = 990501;
DELETE FROM prayer_books WHERE id = 990501;
DELETE FROM bible_texts WHERE translation = 'dtaj_tiny';`

// enqueueJobSQL inserts a ready Solid Queue job with the given serialized
// arguments, as perform_later would (the admin endpoints never pass these).
func enqueueJobSQL(id, class, arguments string) string {
	return `DELETE FROM solid_queue_jobs WHERE id = ` + id + `;
INSERT INTO solid_queue_jobs (id, queue_name, class_name, arguments, priority, active_job_id, scheduled_at, created_at, updated_at) VALUES
 (` + id + `, 'maintenance', '` + class + `', '{"job_class":"` + class + `","job_id":"00000000-0000-4000-8000-` + id + `00000","provider_job_id":null,"queue_name":"maintenance","priority":null,"arguments":` + arguments + `,"executions":0,"exception_executions":{},"locale":"en","timezone":"America/Sao_Paulo","enqueued_at":"2026-01-01T10:00:00.000000000Z","scheduled_at":"2026-01-01T10:00:00.000000000Z"}', 0, '00000000-0000-4000-8000-` + id + `00000', '2026-01-01 10:00', '2026-01-01 10:00', '2026-01-01 10:00');
INSERT INTO solid_queue_ready_executions (job_id, queue_name, priority, created_at) VALUES (` + id + `, 'maintenance', 0, '2026-01-01 10:00');`
}

func clearTTS() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__tts", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func init() {
	registerScenarios("audio_jobs", func() []diff.Scenario {
		admin := userHeaders("difftest-admin", "admin@example.com")
		vol := []string{"created_at", "updated_at", "started_at", "completed_at", "active_job_id", "generated_at", "scheduled_at"}
		post := func(path string, body string) diff.Request { return jsonReq("POST", path, admin, body, vol...) }
		base := "/api/v1/admin/audio"
		tables := []string{"audio_operations", "audio_clips", "audio_clip_candidates", "audio_clip_customizations", "audio_clip_usages",
			"solid_queue_jobs"}
		snapshots := []string{
			`SELECT id, kind, status, parameters, prayer_book_code, total_items, processed_items, generated_clips, skipped_clips,
			   failed_items, generated_characters, error_message, result, started_at IS NOT NULL, completed_at IS NOT NULL
			   FROM audio_operations ORDER BY id`,
			`SELECT id, key, filename, text, line_type, kind, provider, voice, model, speed, duration, character_count, language,
			   instructions_sha256, configuration_fingerprint, custom_instructions_sha256
			   FROM audio_clips WHERE id > 990005 OR key LIKE 'dtaa-%' ORDER BY id`,
			`SELECT c.id, c.audio_clip_id, c.audio_operation_id, c.key, c.filename, c.text, c.line_type, c.provider, c.voice, c.model, c.speed,
			   c.duration, c.character_count, c.language, c.instructions_sha256, c.configuration_fingerprint, c.custom_instructions,
			   c.custom_instructions_sha256, c.status FROM audio_clip_candidates c ORDER BY c.id`,
			`SELECT k.key, u.prayer_book_code, u.source_name, u.source_key FROM audio_clip_usages u JOIN audio_clips k ON k.id = u.audio_clip_id
			   ORDER BY u.prayer_book_code, u.source_name, u.source_key, k.key`,
			`SELECT count(*) FROM user_audio_usages`,
		}
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")
		objects := []string{"audio/tts/openai/candidates/dtaa-1b-aaaa.mp3", "audio/tts/openai/candidates/dtaa-1c-bbbb.mp3",
			"audio/tts/openai/candidates/dtaa-2-cccc.mp3", "audio/tts/openai/dtaa-1.mp3", "audio/tts/openai/dtaa-2.mp3"}
		prepare := func() error {
			if err := clearTTS(); err != nil {
				return err
			}
			return putFakeObjects(objects...)()
		}
		scrub := []diff.Scrub{{Pattern: regexp.MustCompile(`(/candidates/[A-Za-z0-9-]+)-[0-9a-f]{16}\.mp3`), Replace: "$1-<hex>.mp3"}}
		all := []string{"CleanupAudioClipsJob", "IndexAudioCatalogJob", "RegenerateAudioClipJob", "GenerateBookAudioJob", "PrewarmOfficeAudioJob"}
		sc := func(name, extraSetup string, settle func(diff.Side) error, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: audioJobsFixture + extraSetup, Prepare: prepare, Tables: tables,
				Snapshot: snapshots, Steps: steps, Settle: settle, Teardown: audioJobsTeardown, Scrub: scrub,
				HTTPSnapshots: []string{envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000") + "/__objects", fake + "/__tts"}}
		}
		est := func(body string) diff.Request { return post(base+"/generations/estimate", body) }
		return []diff.Scenario{
			sc("estimate", "", nil,
				est(`{"generation": {"prayer_book_code": "loc_2021", "start_date": "2026-12-25"}}`),
				est(`{"generation": {"prayer_book_code": "loc_2021", "start_date": "2026-04-05", "days": 2, "offices": ["evening"]}}`),
				est(`{"prayer_book_code": "loc_2021", "start_date": "2026-04-05", "variants": [{"bible_version": "nvt"}, {"bible_version": "nvt"}, {}]}`),
				est(`{"generation": {"prayer_book_code": "loc_2015", "start_date": "2026-06-07", "offices": "morning"}}`),
				est(`{"generation": {"prayer_book_code": "loc_2015", "start_date": "2026-06-07", "offices": ["late_evening"], "preferences": {"family_rite": true}}}`),
				est(`{"generation": {"prayer_book_code": "loc_2021", "offices": ["compline"]}}`),
				est(`{"generation": {"prayer_book_code": "loc_2021", "days": 40}}`),
				est(`{"generation": {"prayer_book_code": "nope"}}`),
				est(`{"generation": {"prayer_book_code": "loc_2021", "start_date": "2026-02-30"}}`),
			),
			sc("cleanup_legacy", "", settleJobs(all...),
				post(base+"/cleanup", `{"cleanup": {"profile_status": "legacy", "created_before": "2026-02-01"}}`)),
			sc("cleanup_scoped", "", settleJobs(all...),
				post(base+"/cleanup", `{"cleanup": {"profile_status": "legacy", "created_before": "2026-02-01", "prayer_book_code": "loc_2015", "source_name": "liturgical_texts"}}`),
				post(base+"/cleanup", `{"cleanup": {"profile_status": "stale", "created_before": "2026-02-01", "prayer_book_code": "loc_2015"}}`)),
			sc("regenerate_openai", "", settleJobs(all...),
				post(base+"/clips/990001/regenerate", `{"additional_instructions": "  Mais devagar  "}`),
				post(base+"/clips/990002/regenerate", `{}`),
				post(base+"/clips/990005/regenerate", `{"additional_instructions": ""}`)),
			sc("regenerate_failures", "", settleJobs(all...),
				post(base+"/clips/990006/regenerate", `{}`),
				post(base+"/clips/990007/regenerate", `{}`),
				post(base+"/clips/990008/regenerate", `{}`),
				post(base+"/clips/990004/regenerate", `{}`)),
			sc("regenerate_google", "", settleJobsEnv([]string{"AUDIO_TTS_PROVIDER=google"}, all...),
				post(base+"/clips/990003/regenerate", `{"additional_instructions": "Warmly"}`)),
			sc("regenerate_elevenlabs", "", settleJobsEnv([]string{"AUDIO_TTS_PROVIDER=elevenlabs"}, all...),
				post(base+"/clips/990001/regenerate", `{}`)),
			sc("index_catalog", "", settleJobs(all...),
				post(base+"/catalog/reindex", `{"prayer_book_code": "dtaj_fr"}`),
				post(base+"/catalog/reindex", `{"prayer_book_code": "missing_book"}`)),
			sc("generate_catalog", "", settleJobs(all...),
				post(base+"/catalog/generations", `{"catalog": {"prayer_book_code": "dtaj_fr", "translations": ["dtaj_tiny"]}}`)),
			sc("generate_catalog_dry_run", "", settleJobs(all...),
				post(base+"/catalog/generations", `{"catalog": {"prayer_book_code": "dtaj_fr", "translations": ["dtaj_tiny", "dtaj_tiny"], "dry_run": true}}`),
				post(base+"/catalog/generations", `{"catalog": {"prayer_book_code": "dtaj_fr"}}`)),
			sc("prewarm", "", settleJobs(all...),
				post(base+"/generations", `{"generation": {"prayer_book_code": "loc_2021", "start_date": "2026-12-25", "days": 1}}`)),
			sc("jobs_without_operation", enqueueJobSQL("990601", "PrewarmOfficeAudioJob",
				`[{"prayer_book_code":"loc_2021","days":1,"start_date":"2026-12-24","character_budget":400,"_aj_ruby2_keywords":["prayer_book_code","days","start_date","character_budget"]}]`)+
				enqueueJobSQL("990602", "PrewarmOfficeAudioJob",
					`[{"prayer_book_code":"loc_2021","days":1,"start_date":"2026-12-24","generate":false,"include_fixed_sources":false,"_aj_ruby2_keywords":["prayer_book_code","days","start_date","generate","include_fixed_sources"]}]`)+
				enqueueJobSQL("990603", "GenerateBookAudioJob",
					`[{"prayer_book_code":"unknown_book","_aj_ruby2_keywords":["prayer_book_code"]}]`)+
				enqueueJobSQL("990604", "GenerateBookAudioJob",
					`[{"prayer_book_code":"dtaj_fr","translations":["dtaj_tiny"],"character_budget":60,"_aj_ruby2_keywords":["prayer_book_code","translations","character_budget"]}]`)+
				enqueueJobSQL("990605", "CleanupAudioClipsJob",
					`[{"profile_status":"legacy","q":"TTS-4","created_before":"2026-02-01","_aj_symbol_keys":[]}]`)+
				enqueueJobSQL("990606", "RegenerateAudioClipJob", `[123456789]`),
				settleJobs(all...)),
		}
	})
}
