package suites

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/dodopok/estevao-api-go/test/diff"
)

const stripeWebhookSecret = "whsec_test"

func stripeSignature(payload string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(stripeWebhookSecret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + payload))
	return "t=" + strconv.FormatInt(ts, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func resetFakeStripe() error {
	req, _ := http.NewRequest(http.MethodDelete, envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998")+"/__stripe", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

var billingCleanup = `DELETE FROM stripe_webhook_events WHERE event_id LIKE 'evt_difftest_%';`

func init() {
	registerScenarios("billing", func() []diff.Scenario {
		dev := func(sub, email string) map[string]string {
			return withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test",
				"Authorization": "Bearer " + DeveloperTokenFor(map[string]any{"sub": sub, "email": email})})
		}
		founder := dev("dev-founder", "founder@dev.example.com")
		parish := dev("dev-parish", "parish@dev.example.com")
		lapsed := dev("dev-lapsed", "lapsed@dev.example.com")
		none := dev("dev-none", "none@dev.example.com")
		ts := []string{"created_at", "updated_at", "expires_at"}
		post := func(path string, h map[string]string, body string) diff.Request {
			return jsonReq("POST", path, h, body, ts...)
		}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h, Volatile: ts}
		}
		fake := envOr("ORACLE_FAKE_GOOGLE_URL", "http://127.0.0.1:3998") + "/__stripe"
		tables := []string{"developers", "api_keys", "developer_subscriptions", "developer_checkout_sessions", "stripe_webhook_events"}
		snapshots := []string{
			`SELECT id, stripe_customer_id, max_keys FROM developers WHERE id BETWEEN 990001 AND 990099 ORDER BY id`,
			`SELECT developer_id, plan_code, interval, currency, trial_period_days, stripe_checkout_session_id, url,
			   idempotency_key ~ '^[0-9a-f-]{36}$', expires_at > now() FROM developer_checkout_sessions
			   WHERE developer_id BETWEEN 990001 AND 990099 ORDER BY developer_id`,
			`SELECT developer_id, stripe_subscription_id, stripe_customer_id, plan_code, status, interval, currency,
			   cancel_at_period_end, trial_ends_at, current_period_end, canceled_at, last_stripe_event_created_at
			   FROM developer_subscriptions WHERE developer_id BETWEEN 990001 AND 990099 ORDER BY developer_id`,
			`SELECT id, rate_limit_multiplier, billing_active FROM api_keys WHERE developer_id BETWEEN 990001 AND 990099 ORDER BY id`,
			`SELECT event_id, event_type, processed_at IS NOT NULL FROM stripe_webhook_events WHERE event_id LIKE 'evt_difftest_%' ORDER BY event_id`,
		}
		now := time.Now().Unix()
		hook := func(name, payload, signature, contentType string) diff.Request {
			return diff.Request{Name: name, Method: "POST", Path: "/api/v1/webhooks/stripe", Body: payload,
				Headers: map[string]string{"Content-Type": contentType, "Stripe-Signature": signature, "Host": "api.example.test"}}
		}
		signed := func(name, payload string) diff.Request {
			return hook(name, payload, stripeSignature(payload, now), "application/json")
		}
		event := func(id, typ, object string) string {
			return `{"id":"evt_difftest_` + id + `","type":"` + typ + `","created":` + strconv.FormatInt(now, 10) + `,"data":{"object":` + object + `}}`
		}
		return []diff.Scenario{
			{Name: "checkout", Setup: userFixture + developerFixture + billingCleanup, Prepare: resetFakeStripe,
				Tables: tables, Snapshot: snapshots, HTTPSnapshots: []string{fake},
				Steps: []diff.Request{
					get("/api/v1/developers/billing", founder),
					get("/api/v1/developers/billing", parish),
					get("/api/v1/developers/billing", lapsed),
					get("/api/v1/developers/billing", none),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"diocese","interval":"month","currency":"brl"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"founder"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"parish","interval":"week"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"parish","currency":"EUR"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"province","interval":"month","currency":"brl"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"parish","locale":"en"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"parish","interval":"month","currency":"BRL"}`),
					post("/api/v1/developers/billing/checkout", none, `{"plan_code":"diocese","interval":"year","currency":"usd"}`),
					post("/api/v1/developers/billing/checkout", parish, `{"plan_code":"parish"}`),
					post("/api/v1/developers/billing/checkout", lapsed, `{"plan_code":"diocese","interval":"year","currency":"usd","locale":"es"}`),
					post("/api/v1/developers/billing/checkout", founder, `{}`),
					post("/api/v1/developers/billing/portal", none, `{"locale":"en"}`),
					post("/api/v1/developers/billing/portal", founder, `{}`),
					post("/api/v1/developers/billing/portal", lapsed, `{"locale":"fr"}`),
					get("/api/v1/developers/billing", none),
				}},
			{Name: "webhook", Setup: userFixture + developerFixture + billingCleanup, Prepare: resetFakeStripe,
				Tables: tables, Snapshot: snapshots, HTTPSnapshots: []string{fake},
				Steps: []diff.Request{
					hook("unsigned", event("a", "customer.subscription.updated", `{}`), "", "application/json"),
					hook("bad signature", event("a", "customer.subscription.updated", `{}`), "t=1,v1=abc", "application/json"),
					hook("stale", event("a", "customer.subscription.updated", `{}`), stripeSignature(event("a", "customer.subscription.updated", `{}`), now-3600), "application/json"),
					hook("malformed", event("a", "x", `{}`), "v1=abc", "application/json"),
					hook("wrong secret", `{}`, "t="+strconv.FormatInt(now, 10)+",v1=0000", "application/json"),
					signed("not json", `{"id":`),
					hook("not json, text", `{"id":`, stripeSignature(`{"id":`, now), "text/plain"),
					signed("array", `[1,2]`),
					signed("string", `"type"`),
					signed("ignored", event("ignored", "invoice.paid", `{"customer":"cus_parish"}`)),
					signed("unmatched", event("unmatched", "customer.subscription.updated", `{"id":"sub_parish","customer":"cus_nobody"}`)),
					signed("checkout completed", event("checkout", "checkout.session.completed", `{"id":"cs_x","customer":"cus_990005","subscription":"sub_new_parish","metadata":{"developer_id":"990005"}}`)),
					signed("duplicate", event("checkout", "checkout.session.completed", `{"id":"cs_x","customer":"cus_990005","subscription":"sub_new_parish","metadata":{"developer_id":"990005"}}`)),
					signed("canceled", event("canceled", "customer.subscription.updated", `{"id":"sub_parish","customer":"cus_parish"}`)),
					signed("superseded", event("superseded", "customer.subscription.updated", `{"id":"sub_trial","customer":"cus_990005","metadata":{"developer_id":"990005"}}`)),
					signed("meta plan", event("meta", "customer.subscription.created", `{"id":"sub_meta_only","customer":"cus_lapsed"}`)),
					signed("unresolvable", event("unresolvable", "customer.subscription.created", `{"id":"sub_unresolvable","customer":"cus_lapsed"}`)),
					signed("stripe down", event("down", "customer.subscription.updated", `{"id":"sub_500","customer":"cus_lapsed"}`)),
					signed("no id", `{"type":"customer.subscription.updated","data":{"object":{"id":"sub_parish","customer":"cus_parish"}}}`),
					signed("no subscription", event("nosub", "checkout.session.completed", `{"customer":"cus_parish"}`)),
					get("/api/v1/developers/billing", dev("dev-parish", "parish@dev.example.com")),
					get("/api/v1/developers/billing", none),
				}},
		}
	})
}
