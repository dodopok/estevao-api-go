package suites

import (
	"net/http"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// notificationFixture gives users 990070-990079 registration tokens the fake
// FCM answers differently (test/oracle/fake_google.py): delivered, disabled
// by preference, unregistered, rejected as invalid, refused, raw error
// bodies, none, and a token idle for more than 60 days.
var notificationFixture = lifeRuleFixture + `
DELETE FROM notification_logs WHERE user_id BETWEEN 990001 AND 990099;
INSERT INTO users (id, provider_uid, email, name, preferences, timezone, created_at, updated_at) VALUES
 (990070, 'difftest-nt-ok', 'nt-ok@example.com', 'OK', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990071, 'difftest-nt-off', 'nt-off@example.com', 'Off', '{"notifications":false}', 'UTC', '2026-01-01', '2026-01-01'),
 (990072, 'difftest-nt-unreg', 'nt-unreg@example.com', 'Unreg', '{"notifications":true}', 'UTC', '2026-01-01', '2026-01-01'),
 (990073, 'difftest-nt-bad', 'nt-bad@example.com', 'Bad', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990074, 'difftest-nt-403', 'nt-403@example.com', 'Denied', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990075, 'difftest-nt-500', 'nt-500@example.com', 'Down', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990076, 'difftest-nt-none', 'nt-none@example.com', 'None', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990077, 'difftest-nt-old', 'nt-old@example.com', 'Old', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990078, 'difftest-nt-raw', 'nt-raw@example.com', 'Raw', '{}', 'UTC', '2026-01-01', '2026-01-01'),
 (990079, 'difftest-nt-unreg400', 'nt-unreg400@example.com', 'Unreg400', '{}', 'UTC', '2026-01-01', '2026-01-01');
INSERT INTO fcm_tokens (user_id, token, platform, created_at, updated_at) VALUES
 (990070, 'fcm-ok#a', 'android', now(), now()), (990070, 'fcm-ok#b', 'ios', now(), now()),
 (990071, 'fcm-ok#c', 'android', now(), now()),
 (990072, 'fcm-unregistered#1', 'android', now(), now()), (990072, 'fcm-ok#d', 'ios', now(), now()),
 (990073, 'fcm-badtoken#1', 'android', now(), now()),
 (990074, 'fcm-403#1', 'android', now(), now()),
 (990077, 'fcm-ok#old', 'android', '2025-01-01', '2025-01-01'),
 (990078, 'fcm-rawbody#1', 'android', now(), now()),
 (990079, 'fcm-unreg400#1', 'ios', now(), now());
`

func resetFakeFCM() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__fcm", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func init() {
	registerScenarios("notifications", func() []diff.Scenario {
		admin := userHeaders("difftest-admin", "admin@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__fcm"
		snapshots := []string{
			`SELECT user_id, notification_type, title, body, data::text, sent, error_message, idempotency_key IS NOT NULL,
			   delivery_status::text FROM notification_logs WHERE user_id BETWEEN 990001 AND 990099
			   ORDER BY user_id, notification_type, sent, error_message`,
			`SELECT user_id, token FROM fcm_tokens WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, token`,
		}
		tables := []string{"notification_logs", "fcm_tokens"}
		allIDs := `[990070,990071,990072,990073,990074,990075,990076,990077,990078,990079,999999,"abc",null]`
		return []diff.Scenario{
			{Name: "send", Setup: notificationFixture +
				`INSERT INTO fcm_tokens (user_id, token, platform, created_at, updated_at) VALUES (990075, 'fcm-500#1', 'android', now(), now()), (990075, 'fcm-ok#e', 'ios', now(), now());`,
				Prepare: resetFakeFCM, Tables: tables, Snapshot: snapshots, HTTPSnapshots: []string{fake},
				Steps: []diff.Request{
					jsonReq("POST", "/api/v1/notifications/send", plain, `{"user_ids":[990070],"title":"t","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[],"title":"t","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":"990070","title":"t","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[990070],"title":{"x":1},"body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[990070],"title":"  ","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[[990070]],"title":"t","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":`+allIDs+`,"title":"Olá <b>&</b>","body":"Corpo",
						"data":{"type":"announcement","url":"https://x.test/?a=1&b=2","n":5,"f":1.5,"t":true,"z":null,"nested":{"a":1,"b":["x"]},"list":[1,"b",[2],{"c":3}]}}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":["990070"],"title":"Sem dados","body":7}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[990070],"title":"Tipo vazio","body":"b","data":{"type":""}}`),
					jsonReq("POST", "/api/v1/notifications/send", admin, `{"user_ids":[990070],"title":"Click","body":"b","data":{"click_action":"X","type":7}}`),
					{Method: "POST", Path: "/api/v1/notifications/send?user_ids[]=990072&title=Query&body=b&data[k]=v", Headers: admin},
				}},
			{Name: "broadcast", Setup: notificationFixture, Prepare: resetFakeFCM, Tables: tables, Snapshot: snapshots,
				HTTPSnapshots: []string{fake}, Settle: settleJobs("BroadcastNotificationJob"),
				Steps: []diff.Request{
					jsonReq("POST", "/api/v1/notifications/broadcast", plain, `{"title":"t","body":"b"}`),
					jsonReq("POST", "/api/v1/notifications/broadcast", admin, `{"title":"t"}`),
					jsonReq("POST", "/api/v1/notifications/broadcast", admin, `{"title":"Aviso","body":"Para todos","data":{"type":"news","id":3}}`),
				}},
		}
	})
}
