package suites

import "github.com/dodopok/estevao-api-go/test/diff"

// maintenanceFixture: rows on both sides of each retention cut-off, and
// custom rosary prayers stuck in each state the reconciler recovers.
var maintenanceFixture = rosaryFixture + `
DELETE FROM notification_logs WHERE id BETWEEN 990001 AND 990099;
DELETE FROM shared_offices WHERE short_code LIKE 'DTMT%';
DELETE FROM api_key_usage_flushes WHERE id::text LIKE '99000000-%';
DELETE FROM api_key_usage_logs WHERE api_key_id = 990301;
UPDATE api_keys SET requests_count = 0, last_used_at = NULL WHERE id = 990301;
INSERT INTO notification_logs (id, user_id, title, body, notification_type, sent, created_at, updated_at) VALUES
 (990001, 990001, 't', 'b', 'x', TRUE, now() - interval '91 days', now()),
 (990002, 990001, 't', 'b', 'x', TRUE, now() - interval '89 days', now()),
 (990003, 990002, 't', 'b', 'x', FALSE, now(), now());
INSERT INTO shared_offices (short_code, date, office_type, prayer_book_code, seed, expires_at, created_at, updated_at) VALUES
 ('DTMT0001', '2026-01-01', 'morning', 'loc_2015', 1, now() - interval '1 minute', now(), now()),
 ('DTMT0002', '2026-01-01', 'evening', 'loc_2015', 2, now() + interval '1 day', now(), now());
INSERT INTO api_key_usage_flushes (id, created_at) VALUES
 ('99000000-0000-4000-8000-000000000001', now() - interval '31 days'),
 ('99000000-0000-4000-8000-000000000002', now() - interval '1 day');
UPDATE custom_rosary_prayers SET share_status = 'approved', moderation_decision = 'approved', publication_status = 'publishing',
  publication_started_at = now() - interval '1 hour' WHERE id = 990004;
UPDATE custom_rosary_prayers SET publication_status = 'unpublishing', publication_started_at = now() - interval '1 hour' WHERE id = 990005;
UPDATE custom_rosary_prayers SET publication_status = 'failed', publication_retry_at = now() - interval '1 minute',
  publication_attempts = 1 WHERE id = 990006;
UPDATE custom_rosary_prayers SET publication_status = 'pending', publication_attempts = 10 WHERE id = 990009;
`

func init() {
	registerScenarios("maintenance", func() []diff.Scenario {
		h := v2Headers(v2Key)
		return []diff.Scenario{{
			Name: "recurring", Setup: maintenanceFixture,
			Steps: []diff.Request{
				{Path: "/api/v2/days/2026-04-05?book=loc_2015", Headers: h, Volatile: []string{"generated_at"}, KeyedETag: true},
				{Path: "/api/v2/days/2026-04-05?book=loc_2015", Headers: h, Volatile: []string{"generated_at"}, KeyedETag: true},
				{Path: "/api/v2/prayer-books", Headers: h, Volatile: []string{"generated_at"}, KeyedETag: true},
			},
			Settle: enqueueAndSettle("FlushApiKeyUsageJob", "DatabaseCleanupJob", "CleanupExpiredSharedOfficesJob",
				"CustomRosaryPrayers::ReconcilePublicationJobsJob"),
			Snapshot: []string{
				`SELECT id FROM notification_logs WHERE id BETWEEN 990001 AND 990099 ORDER BY id`,
				`SELECT short_code FROM shared_offices WHERE short_code LIKE 'DTMT%' ORDER BY short_code`,
				`SELECT id FROM api_key_usage_flushes WHERE id::text LIKE '99000000-%' ORDER BY id`,
				`SELECT endpoint, date = current_date, requests_count FROM api_key_usage_logs WHERE api_key_id = 990301 ORDER BY endpoint`,
				`SELECT requests_count, last_used_at IS NOT NULL FROM api_keys WHERE id = 990301`,
				`SELECT id, publication_status, publication_started_at IS NULL, publication_retry_at IS NULL, publication_error
				   FROM custom_rosary_prayers WHERE id BETWEEN 990001 AND 990099 ORDER BY id`,
				`SELECT class_name, queue_name, regexp_replace(regexp_replace(arguments, '"job_id":"[0-9a-f-]{36}"', '"job_id":"<uuid>"'),
				   '"(enqueued_at|scheduled_at)":"[0-9T:.Z-]+"', '"\1":"<time>"', 'g')
				   FROM solid_queue_jobs WHERE finished_at IS NULL ORDER BY id`,
			},
		}}
	})
}
