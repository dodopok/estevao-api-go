package suites

import (
	"io"
	"net/http"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// accountFixture builds users with every kind of dependent row, for the
// account deletion cascade, and a yearly history for Wrapped.
const accountFixture = `
DELETE FROM life_rule_exam_items WHERE life_rule_exam_id IN (SELECT id FROM life_rule_exams WHERE user_id BETWEEN 990101 AND 990199);
DELETE FROM life_rule_exams WHERE user_id BETWEEN 990101 AND 990199;
UPDATE life_rules SET original_life_rule_id = NULL WHERE original_life_rule_id IN (SELECT id FROM life_rules WHERE user_id BETWEEN 990101 AND 990199);
DELETE FROM life_rule_steps WHERE life_rule_id IN (SELECT id FROM life_rules WHERE user_id BETWEEN 990101 AND 990199);
DELETE FROM life_rules WHERE user_id BETWEEN 990101 AND 990199;
DELETE FROM custom_rosary_blocks WHERE custom_rosary_prayer_id IN (SELECT id FROM custom_rosary_prayers WHERE user_id BETWEEN 990101 AND 990199);
DELETE FROM custom_rosary_prayers WHERE user_id BETWEEN 990101 AND 990199;
DELETE FROM active_storage_attachments WHERE record_type = 'User' AND record_id BETWEEN 990101 AND 990199;
DELETE FROM active_storage_blobs WHERE id BETWEEN 990101 AND 990199;
DO $$ DECLARE t text; BEGIN
  FOREACH t IN ARRAY ARRAY['completions','fcm_tokens','notification_logs','journals','shared_offices','user_onboardings',
    'prayer_requests','weekly_prayers','user_favorites','premium_subscription_events','user_audio_usages',
    'user_background_track_favorites','feature_flags'] LOOP
    EXECUTE format('DELETE FROM %I WHERE user_id BETWEEN 990101 AND 990199', t);
  END LOOP;
END $$;
DELETE FROM users WHERE id BETWEEN 990101 AND 990199;
INSERT INTO users (id, provider_uid, email, name, preferences, timezone, created_at, updated_at) VALUES
 (990101, 'difftest-ac-delete', 'ac-delete@example.com', 'Delete Me', '{"prayer_book_code":"loc_2015"}', 'America/Sao_Paulo', '2025-01-01', '2025-01-01'),
 (990102, 'difftest-ac-flagged', 'ac-flagged@example.com', 'Flagged', '{"prayer_book_code":"loc_2015"}', 'UTC', '2025-01-01', '2025-01-01'),
 (990103, 'difftest-reject-ac', 'ac-reject@example.com', 'Rejected', '{"prayer_book_code":"loc_2015"}', 'UTC', '2025-01-01', '2025-01-01'),
 (990104, 'difftest-ac-adopter', 'ac-adopter@example.com', 'Adopter', '{"prayer_book_code":"loc_2015"}', 'UTC', '2025-01-01', '2025-01-01'),
 (990105, 'difftest-ac-wrapped', 'ac-wrapped@example.com', 'Wrapped Reader', '{"prayer_book_code":"loc_2019","language":"es"}', 'America/Sao_Paulo', '2025-01-01', '2025-01-01'),
 (990106, 'difftest-ac-noflag', 'ac-noflag@example.com', NULL, '{}', 'Mars/Nowhere', '2025-01-01', '2025-01-01'),
 (990107, 'difftest-ac-empty', 'ac-empty@example.com', 'Empty', '{}', 'Europe/London', '2025-01-01', '2025-01-01');
INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at) VALUES
 ('yearly_wrapped', 'user', 990102, TRUE, now(), now()),
 ('yearly_wrapped', 'user', 990105, TRUE, now(), now()),
 ('yearly_wrapped', 'user', 990107, TRUE, now(), now());
INSERT INTO completions (id, user_id, date_reference, office_type, duration_seconds, prayer_book_id, created_at, updated_at)
SELECT 990100 + g, 990101, '2026-03-01'::date + g, 'morning', 60, (SELECT id FROM prayer_books WHERE code = 'loc_2015'), '2026-03-01 10:00'::timestamp + g * interval '1 day', now()
FROM generate_series(1, 5) g;
INSERT INTO completions (id, user_id, date_reference, office_type, duration_seconds, prayer_book_id, created_at, updated_at)
SELECT 990200 + g, 990105, ('2025-01-01'::date + (g * 7) % 360), (ARRAY['morning','evening','compline','midday','prime','bogus'])[1 + g % 6], 60,
  (SELECT id FROM prayer_books WHERE code = (ARRAY['loc_2019','loc_2019','loc_2015','loc_1979_en'])[1 + g % 4]),
  ('2025-01-01'::timestamp + ((g * 7) % 360) * interval '1 day' + ((g * 5) % 24) * interval '1 hour'), now()
FROM generate_series(1, 120) g;
INSERT INTO completions (id, user_id, date_reference, office_type, prayer_book_id, created_at, updated_at) VALUES
 (990400, 990105, '2025-04-18', 'evening', NULL, '2025-04-18 23:30', now()),
 (990401, 990105, '2025-04-19', 'morning', (SELECT id FROM prayer_books WHERE code = 'loc_2019'), '2025-04-19 09:00', now()),
 (990402, 990105, '2025-04-20', 'morning', (SELECT id FROM prayer_books WHERE code = 'loc_2019'), '2025-04-20 09:00', now()),
 (990403, 990105, '2025-12-31', 'compline', (SELECT id FROM prayer_books WHERE code = 'loc_2019'), '2026-01-01 01:30', now());
INSERT INTO fcm_tokens (user_id, token, platform, created_at, updated_at) VALUES (990101, 'ac-tok', 'ios', now(), now());
INSERT INTO notification_logs (user_id, delivery_status, created_at, updated_at) VALUES (990101, '{}', now(), now());
INSERT INTO journals (user_id, content, date_reference, entry_type, created_at, updated_at) VALUES (990101, 'x', '2026-03-02', 'reflection', now(), now());
INSERT INTO shared_offices (user_id, short_code, prayer_book_code, office_type, date, seed, preferences, expires_at, created_at, updated_at)
 VALUES (990101, 'DTACDEL', 'loc_2015', 'morning', '2026-03-02', 1, '{}', '2099-01-01', now(), now());
INSERT INTO user_onboardings (user_id, prayer_book_id, bible_version_id, mode, onboarding_completed, preferences, created_at, updated_at)
 SELECT 990101, pb.id, bv.id, 'basic', TRUE, '{}', now(), now() FROM prayer_books pb, bible_versions bv WHERE pb.code = 'loc_2015' AND bv.code = 'nvi';
INSERT INTO user_onboardings (user_id, prayer_book_id, bible_version_id, mode, onboarding_completed, preferences, created_at, updated_at)
 SELECT 990105, pb.id, bv.id, 'basic', TRUE, '{}', now(), now() FROM prayer_books pb, bible_versions bv WHERE pb.code = 'loc_2019' AND bv.code = 'nvi';
INSERT INTO life_rules (id, user_id, title, icon, translation_key, created_at, updated_at) VALUES
 (990101, 990101, 'Regra', 'x', 'difftest.rule', now(), now());
INSERT INTO life_rules (id, user_id, title, icon, translation_key, original_life_rule_id, created_at, updated_at) VALUES
 (990104, 990104, 'Adopted', 'x', 'difftest.adopted', 990101, now(), now()),
 (990105, 990105, 'Wrapped rule', 'x', 'difftest.wrapped', NULL, now(), now());
INSERT INTO life_rule_steps (id, life_rule_id, "order", title, created_at, updated_at) VALUES
 (990101, 990101, 1, 'Step', now(), now()), (990105, 990105, 1, 'W1', now(), now()), (990106, 990105, 2, 'W2', now(), now());
INSERT INTO life_rule_exams (id, user_id, life_rule_id, life_rule_title, client_id, status, period, period_start, period_end, completed_at, created_at, updated_at) VALUES
 (990101, 990101, 990101, 'Regra', 'c1', 'completed', 'monthly', '2026-02-01', '2026-02-28', '2026-03-01', now(), now()),
 (990105, 990105, 990105, 'Wrapped rule', 'w1', 'completed', 'weekly', '2025-01-06', '2025-01-12', '2025-01-12 12:00', now(), now()),
 (990106, 990105, 990105, 'Wrapped rule', 'w2', 'completed', 'weekly', '2025-01-13', '2025-01-19', '2025-01-19 12:00', now(), now()),
 (990107, 990105, 990105, 'Wrapped rule', 'w3', 'completed', 'weekly', '2025-03-03', '2025-03-09', '2025-03-09 12:00', now(), now()),
 (990108, 990105, 990105, 'Wrapped rule', 'w4', 'draft', 'weekly', '2025-04-07', '2025-04-13', NULL, now(), now());
INSERT INTO life_rule_exam_items (life_rule_exam_id, client_id, step_title, life_rule_step_id, rating, created_at, updated_at) VALUES
 (990101, 'i1', 'Step', 990101, 'good', now(), now()),
 (990105, 'i2', 'W1', 990105, 'good', now(), now()), (990105, 'i3', 'W2', 990106, 'not_applicable', now(), now()),
 (990106, 'i4', 'W1', 990105, 'poor', now(), now()), (990107, 'i5', 'W2', 990106, 'good', now(), now());
INSERT INTO prayer_requests (user_id, title, week_start, created_at, updated_at) VALUES
 (990101, 'r', '2026-03-02', now(), now()),
 (990105, 'r1', '2025-01-06', '2025-01-07', now()), (990105, 'r2', '2025-01-06', '2025-01-08', now()), (990105, 'r3', '2025-02-03', '2025-02-04', now());
INSERT INTO weekly_prayers (user_id, generated_at, generated_prayer, prayer_book_code, requests_digest, week_start, created_at, updated_at) VALUES
 (990101, now(), 'p', 'loc_2015', 'd', '2026-03-02', now(), now()),
 (990105, now(), 'p', 'loc_2019', 'd1', '2025-01-06', now(), now()), (990105, now(), 'p', 'loc_2019', 'd2', '2025-03-03', now(), now());
INSERT INTO user_favorites (user_id, kind, post_slug, created_at, updated_at) VALUES (990101, 'post', 'x', now(), now());
INSERT INTO custom_rosary_prayers (id, user_id, client_id, title, created_at, updated_at) VALUES
 (990101, 990101, 'cr1', 'Rosary', now(), now()), (990105, 990105, 'cr2', 'Rosary', '2025-05-01', now());
INSERT INTO custom_rosary_blocks (custom_rosary_prayer_id, position, created_at, updated_at) VALUES (990101, 0, now(), now());
INSERT INTO premium_subscription_events (user_id, event_key, event_type, occurred_at, created_at, updated_at) VALUES (990101, 'e1', 'INITIAL_PURCHASE', now(), now(), now());
INSERT INTO active_storage_blobs (id, key, filename, content_type, metadata, service_name, byte_size, checksum, created_at) VALUES
 (990101, 'difftestdeleteavatar00000001', 'a.png', 'image/png', '{"identified":true}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', now());
INSERT INTO active_storage_attachments (name, record_type, record_id, blob_id, created_at) VALUES ('avatar', 'User', 990101, 990101, now());
`

var accountSnapshots = []string{
	`SELECT id, email FROM users WHERE id BETWEEN 990101 AND 990199 ORDER BY id`,
	`SELECT 'completions', count(*) FROM completions WHERE user_id = 990101 UNION ALL
	 SELECT 'journals', count(*) FROM journals WHERE user_id = 990101 UNION ALL
	 SELECT 'life_rules', count(*) FROM life_rules WHERE user_id = 990101 UNION ALL
	 SELECT 'steps', count(*) FROM life_rule_steps WHERE id = 990101 UNION ALL
	 SELECT 'items', count(*) FROM life_rule_exam_items WHERE life_rule_exam_id = 990101 UNION ALL
	 SELECT 'blocks', count(*) FROM custom_rosary_blocks WHERE custom_rosary_prayer_id = 990101 UNION ALL
	 SELECT 'attachments', count(*) FROM active_storage_attachments WHERE record_type = 'User' AND record_id = 990101 UNION ALL
	 SELECT 'blob', count(*) FROM active_storage_blobs WHERE id = 990101`,
	`SELECT id, original_life_rule_id FROM life_rules WHERE id = 990104`,
}

func fakeGoogleDeleted() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__deleted", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resetFakeS3()
}

func init() {
	registerScenarios("users_account", func() []diff.Scenario {
		del := userHeaders("difftest-ac-delete", "ac-delete@example.com")
		flagged := userHeaders("difftest-ac-flagged", "ac-flagged@example.com")
		reject := userHeaders("difftest-reject-ac", "ac-reject@example.com")
		reader := userHeaders("difftest-ac-wrapped", "ac-wrapped@example.com")
		noflag := userHeaders("difftest-ac-noflag", "ac-noflag@example.com")
		empty := userHeaders("difftest-ac-empty", "ac-empty@example.com")
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: []string{"generated_at"}}
		}
		wrappedSteps := []diff.Request{}
		for _, q := range []string{"year=2025", "year=2025&locale=pt", "year=2025&locale=en_us", "year=2025&locale=pt-pt",
			"year=2025&locale=fr", "year=2026", "year=2024", "year=2099", "year=25", "year=0000", "year=abcd", ""} {
			wrappedSteps = append(wrappedSteps, get("/api/v1/users/wrapped?"+q, reader))
		}
		wrappedSteps = append(wrappedSteps, get("/api/v1/users/wrapped?year=2025", noflag),
			get("/api/v1/users/wrapped?year=2025", empty), get("/api/v1/users/wrapped?year=2025", map[string]string{"Accept": "application/json"}))

		return []diff.Scenario{
			{
				Name: "wrapped", Setup: accountFixture, Steps: wrappedSteps, Snapshot: []string{`SELECT count(*) FROM users WHERE id BETWEEN 990101 AND 990199`},
			},
			{
				Name: "destroy", Setup: accountFixture, Prepare: fakeGoogleDeleted,
				Tables: []string{"fcm_tokens", "notification_logs", "journals", "shared_offices", "user_onboardings", "prayer_requests",
					"weekly_prayers", "user_favorites", "custom_rosary_blocks", "premium_subscription_events", "life_rule_exam_items", "active_storage_attachments"},
				Steps: []diff.Request{
					{Method: "DELETE", Path: "/api/v1/users/me", Headers: reject},
					{Method: "DELETE", Path: "/api/v1/users/me", Headers: flagged},
					{Method: "DELETE", Path: "/api/v1/users/me", Headers: del},
					{Method: "DELETE", Path: "/api/v1/users/me", Headers: map[string]string{"Accept": "application/json"}},
				},
				Settle:        settleJobs("ActiveStorage::PurgeJob"),
				Snapshot:      accountSnapshots,
				HTTPSnapshots: []string{envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__deleted", envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000") + "/__objects?summary=1"},
			},
		}
	})
}
