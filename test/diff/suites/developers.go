package suites

import (
	"github.com/dodopok/estevao-api-go/test/diff"
)

// developerFixture: a founder (legacy_free) developer with two keys and
// usage, a subscriber on the parish plan with three keys (its limit), one
// whose subscription lapsed, one pending approval, and one with no plan.
var developerFixture = `
DELETE FROM api_key_usage_logs WHERE api_key_id IN (SELECT id FROM api_keys WHERE developer_id BETWEEN 990001 AND 990099 OR contact_email LIKE '%@dev.example.com');
DELETE FROM api_keys WHERE developer_id BETWEEN 990001 AND 990099 OR contact_email LIKE '%@dev.example.com';
DELETE FROM developer_checkout_sessions WHERE developer_id IN (SELECT id FROM developers WHERE id BETWEEN 990001 AND 990099 OR email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal');
DELETE FROM developer_subscriptions WHERE developer_id IN (SELECT id FROM developers WHERE id BETWEEN 990001 AND 990099 OR email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal');
DELETE FROM api_key_usage_logs WHERE api_key_id IN (SELECT id FROM api_keys WHERE developer_id IN (SELECT id FROM developers WHERE email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal'));
DELETE FROM api_keys WHERE developer_id IN (SELECT id FROM developers WHERE email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal');
DELETE FROM developers WHERE id BETWEEN 990001 AND 990099 OR email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal';
INSERT INTO developers (id, provider_uid, email, name, company_name, approved, legacy_free, max_keys, stripe_customer_id, created_at, updated_at) VALUES
 (990001, 'dev-founder', 'founder@dev.example.com', 'Founder', 'Paróquia', TRUE, TRUE, 3, NULL, '2026-01-01', '2026-01-01'),
 (990002, 'dev-parish', 'parish@dev.example.com', NULL, 'Diocese SP', TRUE, FALSE, 3, 'cus_parish', '2026-01-02', '2026-01-02'),
 (990003, 'dev-lapsed', 'lapsed@dev.example.com', 'Lapsed', NULL, TRUE, FALSE, 10, 'cus_lapsed', '2026-01-03', '2026-01-03'),
 (990004, 'dev-pending', 'pending@dev.example.com', 'Pending', NULL, FALSE, FALSE, 3, NULL, '2026-01-04', '2026-01-04'),
 (990005, 'dev-none', 'none@dev.example.com', 'None', NULL, TRUE, FALSE, 3, NULL, '2026-01-05', '2026-01-05'),
 (990006, 'dev-old-uid', 'adopt@dev.example.com', NULL, NULL, TRUE, FALSE, 3, NULL, '2026-01-06', '2026-01-06');
INSERT INTO developer_subscriptions (developer_id, stripe_subscription_id, stripe_customer_id, plan_code, status, interval, currency,
  cancel_at_period_end, trial_ends_at, current_period_end, created_at, updated_at) VALUES
 (990002, 'sub_parish', 'cus_parish', 'parish', 'trialing', 'month', 'brl', FALSE, '2026-10-05 12:00:00', '2026-11-05 12:00:00', '2026-01-02', '2026-01-02'),
 (990003, 'sub_lapsed', 'cus_lapsed', 'diocese', 'canceled', 'year', 'usd', TRUE, NULL, '2026-08-01 00:00:00', '2026-01-03', '2026-01-03');
INSERT INTO api_keys (id, developer_id, name, key, contact_email, active, billing_active, rate_limit_multiplier, requests_count, created_at, updated_at) VALUES
 (990011, 990001, 'Site', 'estevao_devfixture000000000000000000000000000000000000011', 'founder@dev.example.com', TRUE, TRUE, 5, 3, '2026-02-01', '2026-02-01'),
 (990012, 990001, 'App', 'estevao_devfixture000000000000000000000000000000000000012', 'founder@dev.example.com', TRUE, TRUE, 5, 0, '2026-02-02', '2026-02-02'),
 (990021, 990002, 'P1', 'estevao_devfixture000000000000000000000000000000000000021', 'parish@dev.example.com', TRUE, TRUE, 5, 0, '2026-02-01', '2026-02-01'),
 (990022, 990002, 'P2', 'estevao_devfixture000000000000000000000000000000000000022', 'parish@dev.example.com', TRUE, TRUE, 5, 0, '2026-02-02', '2026-02-02'),
 (990023, 990002, 'P3', 'estevao_devfixture000000000000000000000000000000000000023', 'parish@dev.example.com', TRUE, TRUE, 5, 0, '2026-02-03', '2026-02-03'),
 (990031, 990003, 'L1', 'estevao_devfixture000000000000000000000000000000000000031', 'lapsed@dev.example.com', TRUE, FALSE, 1, 0, '2026-02-01', '2026-02-01');
INSERT INTO api_key_usage_logs (api_key_id, endpoint, date, requests_count, created_at, updated_at) VALUES
 (990011, 'calendar#day', current_date - 1, 3, now(), now()), (990011, 'calendar#month', '2026-03-01', 1, now(), now());
`

func init() {
	registerScenarios("developers", func() []diff.Scenario {
		dev := func(claims map[string]any) map[string]string {
			return withHeaders(AppHeaders(), map[string]string{"Authorization": "Bearer " + DeveloperTokenFor(claims), "Host": "api.example.test"})
		}
		founder := dev(map[string]any{"sub": "dev-founder", "email": "founder@dev.example.com", "email_verified": true})
		parish := dev(map[string]any{"sub": "dev-parish", "email": "parish@dev.example.com", "name": "Parish Admin"})
		lapsed := dev(map[string]any{"sub": "dev-lapsed", "email": "lapsed@dev.example.com"})
		pending := dev(map[string]any{"sub": "dev-pending", "email": "pending@dev.example.com"})
		none := dev(map[string]any{"sub": "dev-none", "email": "none@dev.example.com"})
		adopter := dev(map[string]any{"sub": "dev-new-uid", "email": "adopt@dev.example.com", "email_verified": true, "name": "Adopted"})
		unverified := dev(map[string]any{"sub": "dev-unverified", "email": "founder@dev.example.com"})
		newcomer := dev(map[string]any{"sub": "dev-new", "email": "new@dev.example.com", "name": "New Dev"})
		noEmail := dev(map[string]any{"sub": "dev-noemail"})
		appUser := userHeaders("difftest-us-plain", "us-plain@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		ts := []string{"created_at", "updated_at"}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: ts}
		}
		post := func(path string, h map[string]string, body string, extra ...string) diff.Request {
			return jsonReq("POST", path, h, body, append(ts, extra...)...)
		}
		keyVolatile := []string{"full_key", "key_preview", "new_key_preview"}
		tables := []string{"developers", "api_keys", "developer_subscriptions"}
		snapshots := []string{
			`SELECT id, provider_uid, email, name, company_name, website, use_case_description, approved, legacy_free, max_keys
			   FROM developers WHERE id BETWEEN 990001 AND 990099 OR email LIKE '%@dev.example.com' OR email LIKE '%@developer.portal'
			   ORDER BY provider_uid`,
			`SELECT developer_id, name, contact_email, active, billing_active, rate_limit_multiplier, requests_count, key LIKE 'estevao\_%'
			   FROM api_keys WHERE developer_id IN (SELECT id FROM developers WHERE email LIKE '%@dev.example.com') ORDER BY developer_id, name`,
		}
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: userFixture + developerFixture, Tables: tables, Snapshot: snapshots, Steps: steps}
		}
		return []diff.Scenario{
			sc("profile",
				get("/api/v1/developers/me", anon),
				get("/api/v1/developers/me", appUser),
				get("/api/v1/developers/me", withHeaders(anon, map[string]string{"Authorization": "Bearer not.a.token"})),
				get("/api/v1/developers/me", withHeaders(anon, map[string]string{"Authorization": "Basic abc"})),
				get("/api/v1/developers/me", founder),
				get("/api/v1/developers/me", parish),
				get("/api/v1/developers/me", lapsed),
				get("/api/v1/developers/me", pending),
				get("/api/v1/developers/me", adopter),
				get("/api/v1/developers/me", newcomer),
				get("/api/v1/developers/me", noEmail),
				get("/api/v1/developers/me", unverified),
				jsonReq("PATCH", "/api/v1/developers/me", founder, `{"developer":{"company_name":"Catedral","website":"https://x.test","use_case_description":"App","approved":false,"legacy_free":false}}`, ts...),
				jsonReq("PATCH", "/api/v1/developers/me", parish, `{"company_name":"Wrapped Co"}`, ts...),
				jsonReq("PATCH", "/api/v1/developers/me", parish, `{"developer":"x"}`),
				jsonReq("PATCH", "/api/v1/developers/me", noEmail, `{"developer":{"website":"w"}}`, ts...),
				get("/api/v1/developers/me", noEmail),
			),
			sc("keys",
				get("/api/v1/developers/api_keys", pending),
				get("/api/v1/developers/api_keys", founder),
				get("/api/v1/developers/api_keys", parish),
				get("/api/v1/developers/api_keys/990011", founder),
				get("/api/v1/developers/api_keys/990021", founder),
				get("/api/v1/developers/api_keys/abc", founder),
				get("/api/v1/developers/api_keys/990011/usage", founder),
				get("/api/v1/developers/api_keys/990011/usage?start_date=2026-01-01&end_date=2026-12-31", founder),
				post("/api/v1/developers/api_keys", none, `{"api_key":{"name":"x"}}`),
				post("/api/v1/developers/api_keys", lapsed, `{"api_key":{"name":"x"}}`),
				post("/api/v1/developers/api_keys", parish, `{"api_key":{"name":"x"}}`),
				post("/api/v1/developers/api_keys", founder, `{"api_key":{"name":""}}`),
				post("/api/v1/developers/api_keys", founder, `{}`),
				post("/api/v1/developers/api_keys", founder, `{"name":"Wrapped key"}`, keyVolatile...),
				post("/api/v1/developers/api_keys", founder, `{"api_key":{"name":"Fourth"}}`),
				post("/api/v1/developers/api_keys/990011/rotate", founder, `{}`, keyVolatile...),
				post("/api/v1/developers/api_keys/990021/rotate", founder, `{}`),
				diff.Request{Method: "DELETE", Path: "/api/v1/developers/api_keys/990012", Headers: founder},
				diff.Request{Method: "DELETE", Path: "/api/v1/developers/api_keys/990012", Headers: founder},
				post("/api/v1/developers/api_keys", founder, `{"api_key":{"name":"After delete"}}`, keyVolatile...),
				get("/api/v1/developers/me", founder),
				post("/api/v1/developers/playground/proxy", founder, `{"key_id":990031,"endpoint":"calendar/2026/12/25"}`),
				post("/api/v1/developers/playground/proxy", parish, `{"key_id":990021,"endpoint":"/calendar/2026/12/25","params":{"prayer_book_code":"loc_2015"}}`),
				post("/api/v1/developers/playground/proxy", parish, `{"key_id":990021,"endpoint":"/api/v1/calendar/2026/12?x=1+2&y","params":{"prayer_book_code":"loc_1662_en","n":3}}`),
				post("/api/v1/developers/playground/proxy", parish, `{"key_id":990021,"endpoint":"calendar/bogus"}`),
				post("/api/v1/developers/playground/proxy", parish, `{"key_id":990021,"endpoint":"/up"}`),
				post("/api/v1/developers/playground/proxy", parish, `{"key_id":990021}`),
			),
		}
	})
}
