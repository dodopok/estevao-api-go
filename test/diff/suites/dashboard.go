package suites

import (
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// dashboardFixture adds activity for the fixture users across the tables the
// dashboard sections read, both inside the last 30 days (relative to now)
// and at fixed 2026 dates.
var dashboardFixture = notificationFixture + adminAPIKeyFixtureTail + `
DELETE FROM journals WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM user_favorites WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM premium_subscription_events WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM weekly_prayers WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM prayer_requests WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM audio_generation_sessions WHERE prayer_book_code = 'difftest';
UPDATE users SET created_at = now() - interval '3 days', premium_expires_at = now() + interval '10 days', country_code = 'pt'
  WHERE id = 990070;
UPDATE users SET created_at = now() - interval '9 days', timezone = 'Europe/Lisbon', preferences = '{"language":"en"}'
  WHERE id = 990071;
UPDATE users SET created_at = now() - interval '20 days', timezone = 'Asia/Tokyo', premium_expires_at = now() - interval '2 days'
  WHERE id = 990072;
INSERT INTO completions (user_id, date_reference, office_type, duration_seconds, prayer_book_id, created_at, updated_at)
SELECT u, (now() - (d || ' days')::interval)::date, o, 60 * d, (SELECT id FROM prayer_books WHERE code = 'loc_2015'),
  now() - (d || ' days')::interval, now() - (d || ' days')::interval
FROM unnest(ARRAY[990070, 990071, 990072]) u, generate_series(0, 12, 3) d, unnest(ARRAY['morning', 'evening']) o;
INSERT INTO journals (user_id, content, date_reference, entry_type, office_type, created_at, updated_at) VALUES
 (990070, 'a', current_date - 1, 'reflection', 'morning', now(), now()),
 (990070, 'b', current_date - 2, 'gratitude', NULL, now(), now()),
 (990071, 'c', current_date - 2, 'reflection', 'evening', now(), now());
INSERT INTO user_favorites (user_id, kind, post_slug, created_at, updated_at) VALUES
 (990070, 'post', 'advent-hope', now() - interval '1 day', now()), (990071, 'post', 'advent-hope', now() - interval '2 days', now()),
 (990071, 'quote', 'lent-dust', now() - interval '2 days', now());
INSERT INTO premium_subscription_events (user_id, event_key, event_type, occurred_at, created_at, updated_at) VALUES
 (990070, 'dash-1', 'started', now() - interval '3 days', now(), now()),
 (990072, 'dash-2', 'expired', now() - interval '2 days', now(), now()),
 (990071, 'dash-3', 'renewed', now() - interval '1 day', now(), now());
INSERT INTO prayer_requests (user_id, title, week_start, position, created_at, updated_at) VALUES
 (990070, 'Saúde', current_date - extract(isodow FROM current_date)::int + 1, 0, now(), now());
INSERT INTO weekly_prayers (user_id, generated_prayer, generated_at, language, prayer_book_code, requests_digest, week_start, created_at, updated_at) VALUES
 (990070, 'Senhor...', now(), 'pt-BR', 'loc_2015', 'digest', current_date - extract(isodow FROM current_date)::int + 1, now() - interval '1 day', now());
INSERT INTO audio_generation_sessions (id, prayer_book_code, status, started_at, created_at, updated_at) VALUES
 (990001, 'difftest', 'running', '2026-01-01 10:00:00', '2026-01-01 10:00:00', now()),
 (990002, 'difftest', 'failed', NULL, now(), now());
INSERT INTO notification_logs (user_id, notification_type, title, body, data, sent, error_message, delivery_status, created_at, updated_at) VALUES
 (990070, 'announcement', 'T', 'B', '{}', TRUE, NULL, '{"` + sha256Hex("fcm-ok#a") + `": "sent", "` + sha256Hex("fcm-ok#b") + `": "failed"}', now(), now()),
 (990072, 'streak_reminder', 'T', 'B', '{}', FALSE, 'No active tokens', '{"deadbeef": "invalid"}', now() - interval '2 hours', now()),
 (990073, 'broadcast', 'T', 'B', '{}', FALSE, 'x', '{}', now() - interval '40 days', now());
`

func init() {
	registerScenarios("dashboard", func() []diff.Scenario {
		admin := userHeaders("difftest-admin", "admin@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		get := func(path string, h map[string]string) diff.Request {
			// Dashboard values that move with the clock: the ages of the oldest
			// pending items and the audio operation timestamps.
			return diff.Request{Path: path, Headers: h, Volatile: []string{"oldest_pending_age_seconds"}, AnyKeyOrder: true}
		}
		sections := []string{"overview", "users", "completions", "prayer_books", "journals", "audio", "notifications",
			"life_rules", "shared_offices", "engagement", "moderation", "health", "retention", "onboarding", "premium",
			"geography", "custom_rosaries", "weekly_prayers", "favorites", "api", "developers"}
		var perSection []diff.Request
		for _, s := range sections {
			perSection = append(perSection, get("/api/v1/dashboard?sections="+s, admin))
			perSection = append(perSection, get("/api/v1/dashboard?sections="+s+"&start_date=2026-01-01&end_date=2026-12-31", admin))
		}
		sc := func(name, setup string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: setup, Steps: steps}
		}
		return []diff.Scenario{
			sc("errors", dashboardFixture,
				get("/api/v1/dashboard", plain),
				get("/api/v1/dashboard?start_date=2026-13-01", admin),
				get("/api/v1/dashboard?end_date=yesterday", admin),
				get("/api/v1/dashboard?start_date=2026-09-10&end_date=2026-09-01", admin),
				get("/api/v1/dashboard?sections=users,bogus,Users,bogus", admin),
				get("/api/v1/dashboard?sections=,", admin),
				get("/api/v1/dashboard?sections=users,%20users%20", admin),
				get("/api/v1/dashboard?start_date=20260901&end_date=2026-W40-1&sections=overview", admin),
				// A Rails-only zone name ("Brasilia") breaks the completions
				// section's AT TIME ZONE, and so the default dashboard.
				get("/api/v1/dashboard", admin),
			),
			sc("sections", dashboardFixture+`UPDATE users SET timezone = 'America/Sao_Paulo' WHERE timezone = 'Brasilia';`,
				append([]diff.Request{get("/api/v1/dashboard", admin),
					get("/api/v1/dashboard?sections="+strings.Join(sections, ",")+"&start_date=2025-01-01", admin)}, perSection...)...),
		}
	})
}
