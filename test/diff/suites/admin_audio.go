package suites

import (
	"net/http"
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// adminAudioFixture: clips at fixed ids in each profile class (legacy,
// current-looking and stale fingerprints), with usages, candidates, a
// customization, generation sessions, operations and a dead worker job.
var adminAudioFixture = rosaryFixture + `
DELETE FROM audio_clip_candidates WHERE audio_clip_id BETWEEN 990001 AND 990099 OR id BETWEEN 990001 AND 990099
  OR audio_clip_id IN (SELECT id FROM audio_clips WHERE key LIKE 'dtaa-%');
DELETE FROM audio_clip_usages WHERE audio_clip_id BETWEEN 990001 AND 990099
  OR audio_clip_id IN (SELECT id FROM audio_clips WHERE key LIKE 'dtaa-%');
DELETE FROM user_audio_usages WHERE audio_clip_id BETWEEN 990001 AND 990099
  OR audio_clip_id IN (SELECT id FROM audio_clips WHERE key LIKE 'dtaa-%');
DELETE FROM audio_clips WHERE id BETWEEN 990001 AND 990099 OR key LIKE 'dtaa-%';
DELETE FROM audio_clip_customizations WHERE created_by = 'difftest-aa';
DELETE FROM audio_generation_sessions;
DELETE FROM solid_queue_jobs WHERE class_name = 'PrewarmOfficeAudioJob' OR id = 990001;
DELETE FROM audio_operations;
INSERT INTO audio_clips (id, key, filename, text, line_type, kind, provider, voice, model, speed, duration, character_count, language,
  instructions_sha256, configuration_fingerprint, custom_instructions_sha256, created_at, updated_at) VALUES
 (990001, 'dtaa-1', 'tts/openai/dtaa-1.mp3', 'Senhor, tende piedade de nós.', 'congregation', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 2.5, 29, 'pt-BR', 'x', NULL, NULL, '2026-01-01 10:00', '2026-01-01 10:00'),
 (990002, 'dtaa-2', 'tts/openai/dtaa-2.mp3', 'Glória ao Pai e ao Filho e ao Espírito Santo.', 'congregation', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, NULL, 45, 'pt-BR', 'x', 'stalefp', NULL, '2026-01-02 10:00', '2026-01-02 10:00'),
 (990003, 'dtaa-3', 'tts/google/dtaa-3.mp3', 'The Lord be with you.', 'leader', 'line', 'google', 'en-GB-Chirp3-HD-Charon', 'chirp', 1.25, 1.5, 21, 'en', NULL, NULL, 'cust', '2026-01-03 10:00', '2026-01-03 10:00'),
 (990004, 'dtaa-4', 'tts/openai/dtaa-4.mp3', '', 'silence', 'silence', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 0.5, 0, 'pt-BR', NULL, NULL, NULL, '2026-01-04 10:00', '2026-01-04 10:00'),
 (990005, 'dtaa-5', 'tts/openai/dtaa-5.mp3', '100% _certo_ \ sim', 'leader', 'line', 'openai', 'cedar', 'gpt-4o-mini-tts', 1.0, 3.0, 18, 'pt-BR', NULL, NULL, NULL, '2026-01-05 10:00', '2026-01-05 10:00');
INSERT INTO audio_clip_usages (audio_clip_id, prayer_book_code, source_name, source_key, created_at, updated_at) VALUES
 (990001, 'loc_2015', 'liturgical_texts', 'kyrie', '2026-01-01', '2026-01-01'),
 (990001, 'loc_2019', 'liturgical_texts', 'kyrie', '2026-01-01', '2026-01-01'),
 (990002, 'loc_2015', 'psalms', '23', '2026-01-01', '2026-01-01'),
 (990005, 'loc_2015', 'collects', 'x', '2026-01-01', '2026-01-01');
INSERT INTO audio_clip_candidates (id, audio_clip_id, key, filename, text, line_type, provider, model, voice, language, status, speed,
  duration, character_count, instructions_sha256, configuration_fingerprint, custom_instructions, custom_instructions_sha256, created_at, updated_at) VALUES
 (990001, 990001, 'dtaa-1b', 'tts/openai/candidates/dtaa-1b-aaaa.mp3', 'Senhor, tende piedade de nós.', 'congregation', 'openai', 'gpt-4o-mini-tts', 'cedar', 'pt-BR', 'pending', 1.0, 2.4, 29, 'y', 'fp-new', 'Pronuncie devagar', 'cust2', '2026-02-01 10:00', '2026-02-01 10:00'),
 (990002, 990001, 'dtaa-1c', 'tts/openai/candidates/dtaa-1c-bbbb.mp3', 'Senhor, tende piedade de nós.', 'congregation', 'openai', 'gpt-4o-mini-tts', 'cedar', 'pt-BR', 'pending', 1.0, 2.6, 29, 'y', 'fp-new', NULL, NULL, '2026-02-02 10:00', '2026-02-02 10:00'),
 (990003, 990002, 'dtaa-2', 'tts/openai/candidates/dtaa-2-cccc.mp3', 'Glória ao Pai e ao Filho e ao Espírito Santo.', 'congregation', 'openai', 'gpt-4o-mini-tts', 'cedar', 'pt-BR', 'pending', 1.0, 4.0, 45, 'y', 'fp-new', NULL, NULL, '2026-02-03 10:00', '2026-02-03 10:00'),
 (990004, 990005, 'dtaa-5b', 'tts/openai/candidates/dtaa-5b-dddd.mp3', '100% _certo_ \ sim', 'leader', 'openai', 'gpt-4o-mini-tts', 'cedar', 'pt-BR', 'rejected', 1.0, 1.0, 18, 'y', NULL, NULL, NULL, '2026-02-04 10:00', '2026-02-04 10:00');
INSERT INTO audio_clip_customizations (text_digest, text, instructions, instructions_sha256, provider, model, voice, language, status, created_by, created_at, updated_at) VALUES
 (encode(sha256('The Lord be with you.'::bytea), 'hex'), 'The Lord be with you.', 'Warmly', 'cust', 'google', 'chirp', 'en-GB-Chirp3-HD-Charon', 'en', 'active', 'difftest-aa', '2026-01-01', '2026-01-01');
INSERT INTO audio_generation_sessions (id, prayer_book_code, voice_keys, status, current_voice_key, current_text_id, total_texts, processed_count, failed_count, started_at, completed_at, error_log, created_at, updated_at) VALUES
 (990011, 'dtaa_old', '{male_1,female_1}', 'completed', NULL, NULL, 10, 10, 0, '2026-01-01 10:00', '2026-01-01 11:00', NULL, '2026-01-01 10:00', '2026-01-01 11:00'),
 (990012, 'dtaa_run', '{male_1}', 'running', 'male_1', 42, 3, 1, 1, '2026-01-02 10:00', NULL, 'oops', '2026-01-02 10:00', '2026-01-02 10:00'),
 (990013, 'dtaa_zero', '{}', 'failed', NULL, NULL, 0, 0, 0, NULL, '2026-01-03 10:00', NULL, '2026-01-03 10:00', '2026-01-03 10:00');
INSERT INTO audio_operations (id, kind, status, parameters, prayer_book_code, requested_by, active_job_id, total_items, processed_items, error_message, created_at, updated_at) VALUES
 (990001, 'generate_office', 'queued', '{"days": 1, "start_date": "2026-01-01", "prayer_book_code": "loc_2015"}', 'loc_2015', '990040', 'dead-job-1', 4, 1, NULL, '2026-01-01 10:00', '2026-01-01 10:00'),
 (990002, 'cleanup_clips', 'completed', '{"profile_status": "legacy"}', NULL, '990040', NULL, 3, 3, NULL, '2026-01-02 10:00', '2026-01-02 10:00'),
 (990003, 'index_catalog', 'failed', '{}', NULL, NULL, NULL, 0, 0, 'RuntimeError: boom', '2026-01-03 10:00', '2026-01-03 10:00');
INSERT INTO solid_queue_jobs (id, queue_name, class_name, arguments, priority, active_job_id, scheduled_at, created_at, updated_at) VALUES
 (990001, 'maintenance', 'PrewarmOfficeAudioJob', '{}', 0, 'dead-job-1', '2026-01-01 10:00', '2026-01-01 10:00', '2026-01-01 10:00');
`

func putFakeObjects(keys ...string) func() error {
	return func() error {
		endpoint := envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000")
		req, _ := http.NewRequest(http.MethodDelete, endpoint+"/__objects", nil)
		if resp, err := http.DefaultClient.Do(req); err != nil {
			return err
		} else {
			resp.Body.Close()
		}
		for _, k := range keys {
			req, _ := http.NewRequest(http.MethodPut, endpoint+"/test-avatars/"+k, strings.NewReader("ID3-"+k))
			req.Header.Set("Content-Type", "audio/mpeg")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			resp.Body.Close()
		}
		return nil
	}
}

func init() {
	registerScenarios("admin_audio", func() []diff.Scenario {
		admin := userHeaders("difftest-admin", "admin@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		vol := []string{"created_at", "updated_at", "started_at", "completed_at", "active_job_id", "generated_at", "scheduled_at"}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: vol}
		}
		post := func(path string, body string) diff.Request { return jsonReq("POST", path, admin, body, vol...) }
		base := "/api/v1/admin/audio"
		tables := []string{"audio_operations", "audio_clips", "audio_clip_candidates", "audio_clip_customizations", "audio_clip_usages",
			"audio_generation_sessions", "solid_queue_jobs"}
		snapshots := []string{
			`SELECT id, kind, status, parameters, prayer_book_code, requested_by, active_job_id IS NOT NULL, total_items, processed_items, error_message
			   FROM audio_operations ORDER BY id`,
			`SELECT id, key, filename, duration, character_count, configuration_fingerprint, custom_instructions_sha256
			   FROM audio_clips WHERE key LIKE 'dtaa-%' ORDER BY key`,
			`SELECT c.id, k.key, c.status FROM audio_clip_candidates c LEFT JOIN audio_clips k ON k.id = c.audio_clip_id
			   WHERE c.key LIKE 'dtaa-%' ORDER BY c.id`,
			`SELECT k.key, u.prayer_book_code, u.source_name, u.source_key FROM audio_clip_usages u JOIN audio_clips k ON k.id = u.audio_clip_id
			   WHERE k.key LIKE 'dtaa-%' ORDER BY k.key, u.prayer_book_code, u.source_name`,
			`SELECT text, instructions, instructions_sha256, provider, status FROM audio_clip_customizations
			   WHERE created_by = 'difftest-aa' OR text LIKE 'Senhor, tende%' ORDER BY id`,
			`SELECT count(*) FROM solid_queue_jobs WHERE active_job_id = 'dead-job-1'`,
		}
		objects := []string{"audio/tts/openai/candidates/dtaa-1b-aaaa.mp3", "audio/tts/openai/candidates/dtaa-1c-bbbb.mp3",
			"audio/tts/openai/candidates/dtaa-2-cccc.mp3", "audio/tts/openai/dtaa-1.mp3", "audio/tts/openai/dtaa-2.mp3"}
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: adminAudioFixture, Prepare: putFakeObjects(objects...), Tables: tables,
				Snapshot: snapshots, Steps: steps, Settle: settleJobs("NoSuchJob"),
				HTTPSnapshots: []string{envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000") + "/__objects"}}
		}
		return []diff.Scenario{
			sc("read",
				get(base+"/summary", map[string]string{}),
				get(base+"/summary", plain),
				get(base+"/generation_status", admin),
				get(base+"/clips?q=Senhor", admin),
				get(base+"/clips?kind=line&provider=openai&sort=character_count&direction=asc", admin),
				get(base+"/clips?q=100%25%20_&kind=bogus", admin),
				get(base+"/clips?max_characters=30&profile_status=legacy", admin),
				get(base+"/clips?profile_status=stale", admin),
				get(base+"/clips?profile_status=current&language=en", admin),
				get(base+"/clips?prayer_book_code=loc_2015&source_name=liturgical_texts", admin),
				get(base+"/clips?prayer_book_code=loc_2015&limit=2&offset=1&sort=duration", admin),
				get(base+"/clips?limit=abc&offset=-3&created_after=2026-01-02&created_before=2026-01-04%2012:00&speed=1.25", admin),
				get(base+"/clips?fingerprint=stalefp&voice=cedar&model=gpt-4o-mini-tts", admin),
				get(base+"/clips/990001", admin),
				get(base+"/clips/990003", admin),
				get(base+"/clips/990001abc", admin),
				get(base+"/clips/999999", admin),
				get(base+"/clips/990002/url", admin),
				get(base+"/operations", admin),
				get(base+"/operations?limit=1", admin),
				get(base+"/operations?limit=abc", admin),
				get(base+"/operations?limit=0x2", admin),
				get(base+"/operations/990003", admin),
				get(base+"/operations/5", admin),
				get(base+"/worker_queue", admin),
				post(base+"/cleanup/preview", `{"profile_status":"legacy"}`),
				post(base+"/cleanup/preview", `{"cleanup":{"profile_status":"stale","prayer_book_code":"loc_2015"}}`),
				post(base+"/cleanup/preview", `{"kind":"line","q":"Senhor"}`),
			),
			sc("enqueue",
				post(base+"/cleanup", `{"kind":"line"}`),
				post(base+"/cleanup", `{"cleanup":{"profile_status":"legacy","provider":"openai","sort":"duration","x":"y"}}`),
				post(base+"/catalog/reindex", `{}`),
				post(base+"/catalog/reindex", `{"prayer_book_code":"loc_2019"}`),
				post(base+"/catalog/generations", `{"catalog":{"prayer_book_code":"loc_2019","translations":["nvi","",3],"dry_run":"true"}}`),
				post(base+"/catalog/generations", `{"prayer_book_code":"nope"}`),
				post(base+"/catalog/generations", `{"dry_run":"false"}`),
				post(base+"/generations", `{"start_date":"2026-04-05","days":2,"offices":["morning"],"preferences":{"bible_version":"nvi","x":{"y":1}}}`),
				post(base+"/generations", `{"generation":{"start_date":"2026-04-05","days":"3","variants":[{"bible_version":"nvi"}],"preferences":{"a":{"b":2}}}}`),
				post(base+"/generations", `{"days":40}`),
				post(base+"/generations", `{"days":"abc"}`),
				post(base+"/generations", `{"start_date":"2026-02-30"}`),
				post(base+"/generations", `{"prayer_book_code":"nope"}`),
				post(base+"/generations", `{"offices":["morning","bogus","x"]}`),
				post(base+"/generations", `{"offices":"evening","prayer_book_code":"loc_2019","preferences":"x"}`),
				post(base+"/clips/990002/regenerate", `{"additional_instructions":"  Mais alegre  "}`),
				post(base+"/clips/990002/regenerate", `{}`),
				post(base+"/clips/990002/regenerate", `{"additional_instructions":"`+strings.Repeat("a", 1001)+`"}`),
				post(base+"/clips/999999/regenerate", `{}`),
				get(base+"/operations", admin),
				get(base+"/worker_queue", admin),
				post(base+"/worker_queue/purge", `{}`),
				post(base+"/worker_queue/purge", `{"scope":"all"}`),
				get(base+"/operations?limit=100", admin),
			),
			sc("candidates",
				post(base+"/clips/990001/candidates/990003/accept", `{}`),
				post(base+"/clips/990001/candidates/990001/accept", `{}`),
				get(base+"/clips/990001", admin),
				post(base+"/clips/990001/candidates/990001/accept", `{}`),
				diff.Request{Method: "DELETE", Path: base + "/clips/990001/candidates/990002", Headers: admin},
				post(base+"/clips/990002/candidates/990003/accept", `{}`),
				diff.Request{Method: "DELETE", Path: base + "/clips/990005/candidates/990004", Headers: admin},
				diff.Request{Method: "DELETE", Path: base + "/clips/990005/candidates/42", Headers: admin},
				get(base+"/clips?q=Gl%C3%B3ria", admin),
			),
		}
	})
}
