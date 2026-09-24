package suites

import (
	"net/http"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// subscriptionFixture adds subscription users (990010-990029), a
// liturgical text with one generated voice, and audio feature overrides.
var subscriptionFixture = userFixture + `
INSERT INTO users (id, provider_uid, email, preferences, premium_expires_at, revenue_cat_user_id, timezone, created_at, updated_at) VALUES
 (990010, 'difftest-sub-a', 'sub-a@example.com', '{}', NULL, NULL, 'UTC', '2026-01-01', '2026-01-01'),
 (990011, 'difftest-sub-b', 'sub-b@example.com', '{}', '2030-01-01 00:00:00', 'rc_old', 'UTC', '2026-01-01', '2026-01-01'),
 (990012, 'difftest-sub-c', 'sub-c@example.com', '{}', '2099-01-01 00:00:00', NULL, 'UTC', '2026-01-01', '2026-01-01'),
 (990013, 'difftest-sub-bad', 'sub-bad@example.com', '{"preferred_audio_voice":"robot"}', NULL, NULL, 'UTC', '2026-01-01', '2026-01-01'),
 (990014, 'difftest-sub-exp', 'sub-exp@example.com', '{}', '2025-06-01 12:00:00.123456', 'rc_expired', 'UTC', '2026-01-01', '2026-01-01'),
 (990015, 'difftest-sub-bad2', 'sub-bad2@example.com', '{"preferred_audio_voice":"robot"}', NULL, 'rc_active', 'UTC', '2026-01-01', '2026-01-01');
` + errorUsersSQL + `
DELETE FROM liturgical_texts WHERE id = 990001;
INSERT INTO liturgical_texts (id, prayer_book_id, slug, category, content, title, audio_urls, created_at, updated_at)
SELECT 990001, id, 'difftest-audio', 'prayer', 'Texto de teste', 'Oração de teste',
  '{"male_1":"/audio/loc_2015/male_1/difftest-audio.mp3","female_1":""}', '2026-01-01', '2026-01-01'
FROM prayer_books WHERE code = 'loc_2015';
INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at) VALUES
 ('daily_office_audio', 'user', 990002, TRUE, now(), now()),
 ('daily_office_audio', 'user', 990011, FALSE, now(), now());
`

// Subscribers of the fake whose responses make RevenueCatService raise.
var errorSubscribers = []string{"rc_str_ent_key", "rc_bad_date", "rc_int_date", "rc_arr_sub", "rc_str_sub", "rc_arr_ents",
	"rc_null", "rc_badjson", "rc_500", "rc_429"}

// errorUsersSQL gives each of them its own user (990020...).
var errorUsersSQL = func() string {
	sql := "INSERT INTO users (id, provider_uid, email, preferences, timezone, created_at, updated_at) VALUES\n"
	for i := range errorSubscribers {
		if i > 0 {
			sql += ",\n"
		}
		id := 990020 + i
		sql += " (" + itoa(id) + ", 'difftest-sub-e" + itoa(i) + "', 'sub-e" + itoa(i) + "@example.com', '{}', 'UTC', '2026-01-01', '2026-01-01')"
	}
	return sql + ";\n"
}()

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

func resetFakeRevenueCat() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__revenuecat", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func init() {
	registerScenarios("subscriptions", func() []diff.Scenario {
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		premium := userHeaders("difftest-us-onb", "us-onb@example.com")
		sub := func(uid, email string) map[string]string { return userHeaders(uid, email) }
		a := sub("difftest-sub-a", "sub-a@example.com")
		b := sub("difftest-sub-b", "sub-b@example.com")
		cc := sub("difftest-sub-c", "sub-c@example.com")
		bad := sub("difftest-sub-bad", "sub-bad@example.com")
		exp := sub("difftest-sub-exp", "sub-exp@example.com")
		bad2 := sub("difftest-sub-bad2", "sub-bad2@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		get := func(path string, h map[string]string, volatile ...string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: volatile}
		}
		verify := func(h map[string]string, id string, volatile ...string) diff.Request {
			return jsonReq("POST", "/api/v1/subscription/verify", h, `{"revenue_cat_user_id":"`+id+`"}`, volatile...)
		}
		tables := []string{"premium_subscription_events", "user_audio_usages", "feature_flags"}
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__revenuecat"
		steps := []diff.Request{
			verify(a, "rc_active"),
			verify(a, "rc_active"),
			verify(a, "rc_later"),
			verify(a, "rc_earlier"),
			verify(a, "rc_active"),
			verify(a, "rc_expired"),
			verify(a, "rc_404"),
			verify(b, "rc_multi"),
			verify(cc, "rc_active"),
			verify(cc, "rc_lifetime", "expires_at"),
			verify(exp, "rc_expired"),
			verify(exp, "rc_none"),
			verify(bad, "rc_active"),
			verify(bad2, "rc_active"),
			verify(bad2, "rc_zoneless"),
			verify(plain, "rc_nosub"),
			verify(plain, "rc_nullents"),
			verify(premium, "rc_str_ent", "expires_at"),
			get("/api/v1/subscription/premium_status", premium, "expires_at"),
			jsonReq("POST", "/api/v1/subscription/verify", plain, `{}`),
			jsonReq("POST", "/api/v1/subscription/verify", plain, `{"revenue_cat_user_id":"  "}`),
			jsonReq("POST", "/api/v1/subscription/verify", plain, `{"revenue_cat_user_id":12345}`),
			jsonReq("POST", "/api/v1/subscription/verify", anon, `{"revenue_cat_user_id":"rc_active"}`),
			get("/api/v1/subscription/premium_status", plain),
			get("/api/v1/subscription/premium_status", b),
			get("/api/v1/subscription/premium_status", anon),
		}
		for i, id := range errorSubscribers {
			steps = append(steps, verify(sub("difftest-sub-e"+itoa(i), "sub-e"+itoa(i)+"@example.com"), id))
		}
		audioSteps := []diff.Request{
			get("/api/v1/audio/voice_samples", plain),
			get("/api/v1/audio/voice_samples", withHeaders(plain, map[string]string{"X-Forwarded-Host": "a.example, b.example:8443"})),
			get("/api/v1/audio/voice_samples", withHeaders(plain, map[string]string{"X-Forwarded-Host": "cdn.example:443"})),
			get("/api/v1/audio/voice_samples", withHeaders(plain, map[string]string{"Forwarded": `for=1.2.3.4;host="fwd.example";proto=http`})),
			get("/api/v1/audio/voice_samples", withHeaders(plain, map[string]string{"Host": "api.example.test:443"})),
			get("/api/v1/audio/voice_samples", anon),
			get("/api/v1/audio/url/loc_2015/male_1/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/male_1/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/female_1/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/male_2/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/robot/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/male_1/nope", premium),
			get("/api/v1/audio/url/nope/male_1/difftest-audio", premium),
			get("/api/v1/audio/url/loc_2015/male_1/difftest-audio", plain),
			get("/api/v1/audio/url/loc_2015/male_1/difftest-audio", b),
			get("/api/v1/audio/url/loc_2015/male_1/difftest-audio", anon),
		}
		return []diff.Scenario{
			{Name: "plans", Steps: []diff.Request{
				{Path: "/api/v1/plans"},
				get("/api/v1/plans", anon),
				get("/api/v1/plans.json", anon),
			}},
			{Name: "verify", Setup: subscriptionFixture, Tables: tables, Prepare: resetFakeRevenueCat, Steps: steps,
				Snapshot: []string{
					`SELECT id, revenue_cat_user_id, CASE WHEN premium_expires_at > '2100-01-01' THEN 'far' ELSE premium_expires_at::text END
					   FROM users WHERE id BETWEEN 990001 AND 990099 ORDER BY id`,
					`SELECT user_id, event_type, CASE WHEN expires_at > '2100-01-01' THEN 'far' ELSE event_key END, product_identifier,
					   CASE WHEN expires_at > '2100-01-01' THEN NULL ELSE expires_at END, will_renew, occurred_at > now() - interval '1 hour'
					   FROM premium_subscription_events WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, id`,
				},
				HTTPSnapshots: []string{fake},
			},
			{Name: "audio", Setup: subscriptionFixture, Tables: tables, Steps: audioSteps,
				Settle: settleJobs("Audio::RecordUserUsageJob"),
				Snapshot: []string{
					`SELECT user_id, audio_type, asset_key, liturgical_text_id, office_type, prayer_book_code, voice, access_count
					   FROM user_audio_usages WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, asset_key`,
				},
			},
		}
	})
}
