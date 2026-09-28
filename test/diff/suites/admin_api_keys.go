package suites

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/dodopok/estevao-api-go/test/diff"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var adminAPIKeyFixture = lifeRuleFixture + adminAPIKeyFixtureTail

// adminAPIKeyFixtureTail is the API keys and their usage logs.
const adminAPIKeyFixtureTail = `
DELETE FROM api_key_usage_logs WHERE api_key_id BETWEEN 990001 AND 990099 OR api_key_id IN (SELECT id FROM api_keys WHERE contact_email LIKE '%@ak.example.com');
DELETE FROM api_keys WHERE id BETWEEN 990001 AND 990099 OR contact_email LIKE '%@ak.example.com';
INSERT INTO api_keys (id, name, key, contact_email, active, billing_active, rate_limit_multiplier, requests_count,
  last_used_at, expires_at, created_at, updated_at) VALUES
 (990001, 'Parish site', 'estevao_akfixture0000000000000000000000000000000000000001', 'p@ak.example.com', TRUE, TRUE, 1, 42,
  '2026-09-01 10:00:00', NULL, '2026-05-01 10:00:00', '2026-05-01 10:00:00'),
 (990002, 'Old app', 'estevao_akfixture0000000000000000000000000000000000000002', 'o@ak.example.com', TRUE, TRUE, 3, 7,
  NULL, '2026-01-01 00:00:00', '2026-05-02 10:00:00', '2026-05-02 10:00:00'),
 (990003, 'Suspended', 'estevao_akfixture0000000000000000000000000000000000000003', 's@ak.example.com', FALSE, FALSE, 1, 0,
  NULL, '2099-01-01 00:00:00', '2026-05-03 10:00:00', '2026-05-03 10:00:00');
INSERT INTO api_key_usage_logs (api_key_id, endpoint, date, requests_count, created_at, updated_at) VALUES
 (990001, 'calendar#day', current_date - 1, 10, now(), now()),
 (990001, 'calendar#month', current_date - 1, 3, now(), now()),
 (990001, 'calendar#day', current_date - 5, 20, now(), now()),
 (990001, 'calendar#year', current_date - 40, 9, now(), now()),
 (990001, 'calendar#month', '2026-03-10', 2, now(), now()),
 (990002, 'calendar#day', current_date, 7, now(), now());
`

func init() {
	registerScenarios("admin_api_keys", func() []diff.Scenario {
		admin := userHeaders("difftest-admin", "admin@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		ts := []string{"created_at", "updated_at"}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: ts}
		}
		body := func(inner string) string { return `{"api_key":{` + inner + `}}` }
		return []diff.Scenario{{
			Name: "crud", Setup: adminAPIKeyFixture, Tables: []string{"api_keys", "api_key_usage_logs"},
			Snapshot: []string{
				`SELECT id, name, contact_email, active, billing_active, rate_limit_multiplier, requests_count, last_used_at,
				   expires_at, key LIKE 'estevao\_%' AND length(key) = 56 FROM api_keys
				   WHERE id BETWEEN 990001 AND 990099 OR contact_email LIKE '%@ak.example.com' ORDER BY id`,
				`SELECT api_key_id, endpoint, date, requests_count FROM api_key_usage_logs
				   WHERE api_key_id BETWEEN 990001 AND 990099 ORDER BY api_key_id, endpoint, date`,
			},
			Steps: []diff.Request{
				get("/api/v1/admin/api_keys", plain),
				get("/api/v1/admin/api_keys/990001", admin),
				get("/api/v1/admin/api_keys/990002", admin),
				get("/api/v1/admin/api_keys/999999", admin),
				get("/api/v1/admin/api_keys/abc", admin),
				get("/api/v1/admin/api_keys/990001/usage", admin),
				get("/api/v1/admin/api_keys/990001/usage?start_date=2026-01-01&end_date=2026-12-31", admin),
				get("/api/v1/admin/api_keys/990001/usage?start_date=bogus&end_date=", admin),
				get("/api/v1/admin/api_keys/990001/usage?start_date=March%2010&end_date=10/3/2026", admin),
				get("/api/v1/admin/api_keys/990001/usage?start_date[]=x", admin),
				get("/api/v1/admin/api_keys/990003/usage", admin),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"name":"New","contact_email":"n@ak.example.com","expires_at":"2027-01-01 10:00"`),
					"created_at", "updated_at", "full_key", "key_preview"),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"name":"Tz","contact_email":"tz@ak.example.com","expires_at":"2027-01-01T10:00:00Z","active":false`),
					"created_at", "updated_at", "full_key", "key_preview"),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"name":"Bad date","contact_email":"bd@ak.example.com","expires_at":"garbage"`),
					"created_at", "updated_at", "full_key", "key_preview"),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"name":"","contact_email":"not-an-email"`)),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"contact_email":""`)),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, body(`"name":"x","contact_email":["a@ak.example.com"]`)),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, `{"name":"Wrapped","contact_email":"w@ak.example.com"}`,
					"created_at", "updated_at", "full_key", "key_preview"),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, `{"api_key":"x"}`),
				jsonReq("POST", "/api/v1/admin/api_keys", admin, `{}`),
				jsonReq("PATCH", "/api/v1/admin/api_keys/990001", admin, body(`"name":"Parish site v2","active":"0","expires_at":"2030-06-01"`), ts...),
				jsonReq("PATCH", "/api/v1/admin/api_keys/990001", admin, body(`"name":"Parish site v2"`), ts...),
				jsonReq("PATCH", "/api/v1/admin/api_keys/990001", admin, body(`"contact_email":"bad"`)),
				jsonReq("PATCH", "/api/v1/admin/api_keys/990001", admin, body(`"active":""`)),
				jsonReq("PATCH", "/api/v1/admin/api_keys/990001", admin, body(`"key":"estevao_stolen","requests_count":0`), ts...),
				jsonReq("PATCH", "/api/v1/admin/api_keys/999999", admin, body(`"name":"x"`)),
				{Method: "DELETE", Path: "/api/v1/admin/api_keys/990002", Headers: admin},
				{Method: "DELETE", Path: "/api/v1/admin/api_keys/990002", Headers: admin},
				get("/api/v1/admin/api_keys/990001/usage?start_date=2026-03-10&end_date=2026-03-10", admin),
			},
		}}
	})
}
