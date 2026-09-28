package v1

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/billing"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/developers"
	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func init() { developers.SetCacheEviction(auth.ClearRateLimitMultiplierCache) }

func timeAttr(v any) any { return timeValue(v) }

// serializeDeveloper ports DevelopersController#serialize_developer.
func serializeDeveloper(ctx context.Context, d *developers.Developer) *rb.Map {
	return rb.M("id", d.ID, "email", d.Get("email"), "name", d.Get("name"), "company_name", d.Get("company_name"),
		"website", d.Get("website"), "use_case_description", d.Get("use_case_description"), "approved", d.Get("approved"),
		"legacy_free", d.Get("legacy_free"), "entitlement", developers.EntitlementFor(ctx, d).AsJSON(),
		"max_keys", d.Get("max_keys"), "keys_count", developers.KeysCount(ctx, d),
		"keys_remaining", developers.KeysRemaining(ctx, d), "can_create_key", developers.CanCreateKey(ctx, d),
		"created_at", timeAttr(d.Get("created_at")), "updated_at", timeAttr(d.Get("updated_at")))
}

// DevelopersMe ports DevelopersController#me.
func DevelopersMe(c *web.Context) {
	d := developers.Authenticate(c)
	c.JSON(200, serializeDeveloper(c.Ctx, d))
}

// fetchPermit ports params.fetch(key, {}).permit(fields...).
func fetchPermit(c *web.Context, key string, fields ...string) *rb.Map {
	raw, ok := c.Params().Lookup(key)
	if !ok {
		return rb.NewMap()
	}
	m, isMap := raw.(*rb.Map)
	if !isMap {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("permit", raw)})
	}
	out := rb.NewMap()
	for _, f := range fields {
		if v, ok := m.Lookup(f); ok && web.PermittedScalar(v) {
			out.Set(f, v)
		}
	}
	return out
}

// DevelopersUpdate ports DevelopersController#update.
func DevelopersUpdate(c *web.Context) {
	d := developers.Authenticate(c)
	attrs := fetchPermit(c, "developer", "company_name", "website", "use_case_description")
	attrs.Each(func(k string, v any) { d.Assign(k, v) })
	errs, err := d.Save(c.Ctx)
	pgMust(err)
	if errs != nil {
		c.JSON(422, rb.M("errors", errs.FullMessages()))
		return
	}
	c.JSON(200, serializeDeveloper(c.Ctx, d))
}

// approvedDeveloper ports authenticate_developer! + require_developer_approved!.
func approvedDeveloper(c *web.Context) *developers.Developer {
	d := developers.Authenticate(c)
	if d.Get("approved") != true {
		c.RenderJSON(403, rb.M("error", "Developer account pending approval"))
	}
	return d
}

// setDeveloperAPIKey ports set_api_key (DeveloperApiKeyPolicy.scope).
func setDeveloperAPIKey(c *web.Context, d *developers.Developer, param string) *ar.Record {
	if id, ok := findID(c.Param(param)); ok {
		if found := loadAPIKeys(c.Ctx, "api_keys.developer_id = $1 AND api_keys.id = $2", "LIMIT 1", d.ID, id); len(found) > 0 {
			return found[0]
		}
	}
	return nil
}

func requireDeveloperAPIKey(c *web.Context, d *developers.Developer) *ar.Record {
	k := setDeveloperAPIKey(c, d, "id")
	if k == nil {
		c.RenderJSON(404, rb.M("error", "API key not found"))
	}
	return k
}

// DeveloperAPIKeysIndex ports Developers::ApiKeysController#index.
func DeveloperAPIKeysIndex(c *web.Context) {
	d := approvedDeveloper(c)
	out := []any{}
	for _, k := range loadAPIKeys(c.Ctx, "api_keys.developer_id = $1", "ORDER BY api_keys.created_at DESC", d.ID) {
		out = append(out, serializeAPIKey(k))
	}
	c.JSON(200, out)
}

// DeveloperAPIKeysShow ports #show.
func DeveloperAPIKeysShow(c *web.Context) {
	d := approvedDeveloper(c)
	c.JSON(200, serializeAPIKey(requireDeveloperAPIKey(c, d)))
}

func newAPIKeyValue() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return "estevao_" + hex.EncodeToString(b[:])
}

// DeveloperAPIKeysCreate ports #create.
func DeveloperAPIKeysCreate(c *web.Context) {
	d := approvedDeveloper(c)
	if !developers.EntitlementFor(c.Ctx, d).Entitled() {
		c.JSON(403, rb.M("error", "An active plan is required to create API keys.", "code", "PLAN_REQUIRED"))
		return
	}
	if !developers.CanCreateKey(c.Ctx, d) {
		c.JSON(403, rb.M("error", "Key limit reached (max: "+rb.ToS(d.Get("max_keys"))+")", "code", "KEY_LIMIT_REACHED"))
		return
	}
	attrs := requireAPIKeyParams(c, "name")
	k := ar.NewRecord(apiKeySchema)
	k.Set("developer_id", d.ID)
	attrs.Each(func(f string, v any) { k.Assign(f, v) })
	k.Assign("contact_email", d.Get("email"))
	if errs := validateAPIKey(c.Ctx, k); errs.Any() {
		c.JSON(422, rb.M("errors", errs.FullMessages()))
		return
	}
	k.Set("key", newAPIKeyValue())
	pgMust(k.Insert(c.Ctx, db.Q(), users.Now()))
	auth.ClearRateLimitMultiplierCache()
	developers.ApplyEntitlement(c.Ctx, d)
	fresh := loadAPIKeys(c.Ctx, "api_keys.id = $1", "LIMIT 1", k.ID)[0]
	c.JSON(201, serializeAPIKey(fresh).Merge(rb.M("full_key", fresh.Get("key"))))
}

// DeveloperAPIKeysDestroy ports #destroy.
func DeveloperAPIKeysDestroy(c *web.Context) {
	d := approvedDeveloper(c)
	k := requireDeveloperAPIKey(c, d)
	pgMust(db.Transaction(c.Ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM api_key_usage_logs WHERE api_key_id = $1`, k.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, k.ID); err != nil {
			return err
		}
		developers.ApplyEntitlement(ctx, d)
		return nil
	}))
	auth.ClearRateLimitMultiplierCache()
	c.JSON(200, rb.M("message", "API key deleted"))
}

// DeveloperAPIKeysUsage ports #usage.
func DeveloperAPIKeysUsage(c *web.Context) {
	d := approvedDeveloper(c)
	renderAPIKeyUsage(c, requireDeveloperAPIKey(c, d))
}

// DeveloperAPIKeysRotate ports #rotate (ApiKeys::Rotate).
func DeveloperAPIKeysRotate(c *web.Context) {
	d := approvedDeveloper(c)
	k := requireDeveloperAPIKey(c, d)
	oldPreview := apiKeyPreview(k.Get("key"))
	k.Set("key", newAPIKeyValue())
	if errs := validateAPIKey(c.Ctx, k); errs.Any() {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + strings.Join(errs.FullMessages(), ", ")})
	}
	_, err := k.Update(c.Ctx, db.Q(), users.Now())
	pgMust(err)
	auth.ClearRateLimitMultiplierCache()
	c.JSON(200, rb.M("message", "API key rotated successfully", "old_key_preview", oldPreview,
		"new_key_preview", apiKeyPreview(k.Get("key")), "full_key", k.Get("key")))
}

// addressableEncode ports Addressable::URI.encode_component with the
// unreserved character class.
func addressableEncode(v any) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case bool, int, int64, float64:
		s = rb.ToS(x)
	default:
		panic(&web.StandardError{Class: "TypeError", Message: "Can't convert " + rb.ClassName(v) + " into String."})
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || strings.IndexByte("-._~", ch) >= 0 {
			b.WriteByte(ch)
		} else {
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{ch})))
		}
	}
	return b.String()
}

// proxyQuery ports http.rb's make_request_uri: the URL's own query pairs
// (Addressable query_values(Array)) followed by params, re-encoded.
func proxyQuery(rawQuery string, params any) string {
	type pair struct {
		k string
		v any
	}
	var pairs []pair
	for _, part := range strings.Split(rawQuery, "&") {
		if part == "" {
			continue
		}
		k, v, hasV := strings.Cut(part, "=")
		key, _ := url.PathUnescape(k)
		if !hasV {
			pairs = append(pairs, pair{key, nil})
			continue
		}
		val, _ := url.PathUnescape(strings.ReplaceAll(v, "+", " "))
		pairs = append(pairs, pair{key, val})
	}
	switch p := params.(type) {
	case *rb.Map:
		p.Each(func(k string, v any) { pairs = append(pairs, pair{k, v}) })
	case []any:
		for _, el := range p {
			if kv, ok := el.([]any); ok && len(kv) == 2 {
				pairs = append(pairs, pair{rb.ToS(kv[0]), kv[1]})
			}
		}
	}
	var parts []string
	for _, p := range pairs {
		key := addressableEncode(p.k)
		switch v := p.v.(type) {
		case nil:
			parts = append(parts, key)
		case []any:
			for _, sub := range v {
				parts = append(parts, key+"="+addressableEncode(sub))
			}
		default:
			parts = append(parts, key+"="+addressableEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

// DeveloperPlaygroundProxy ports #proxy: a GET to this API with the key,
// so the full key never reaches the browser.
func DeveloperPlaygroundProxy(c *web.Context) {
	d := approvedDeveloper(c)
	k := setDeveloperAPIKey(c, d, "key_id")
	if k == nil {
		c.JSON(404, rb.M("error", "API key not found"))
		return
	}
	base := "http://localhost:3000"
	if v, ok := os.LookupEnv("APP_HOST"); ok {
		base = v
	}
	// Test-only: lets a side-by-side run point each stack's playground at
	// itself while APP_HOST (which also builds audio and avatar URLs) stays
	// the same for both. Unset in every deployed environment.
	if v, ok := os.LookupEnv("PLAYGROUND_PROXY_BASE_URL"); ok {
		base = v
	}
	raw := c.Param("endpoint")
	endpoint, ok := raw.(string)
	if !ok {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("start_with?", raw)})
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	if !strings.HasPrefix(endpoint, "/api/v1") && !strings.HasPrefix(endpoint, "/api/v2") {
		endpoint = "/api/v1" + endpoint
	}
	var params any = c.Param("params")
	if params == nil || params == false {
		params = rb.NewMap()
	}
	target, err := url.Parse(base + endpoint)
	if err != nil {
		c.JSON(502, rb.M("error", "Proxy request failed", "request_id", c.RequestID))
		return
	}
	target.RawQuery = proxyQuery(target.RawQuery, params)
	req, err := http.NewRequestWithContext(c.Ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		c.JSON(502, rb.M("error", "Proxy request failed", "request_id", c.RequestID))
		return
	}
	req.Header.Set("X-API-Key", rb.ToS(k.Get("key")))
	req.Header.Set("User-Agent", "http.rb/5.3.1")
	resp, err := integrations.Client("DEVELOPER_PROXY_HTTP_TIMEOUT", 30).Do(req)
	if err != nil {
		c.JSON(502, rb.M("error", "Proxy request failed", "request_id", c.RequestID))
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(502, rb.M("error", "Proxy request failed", "request_id", c.RequestID))
		return
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mt != "application/json" {
		c.JSON(502, rb.M("error", "Proxy request failed", "request_id", c.RequestID))
		return
	}
	parsed, err := rb.ParseJSON(body)
	if err != nil {
		c.JSON(500, rb.M("error", "Invalid response from target API"))
		return
	}
	c.JSON(resp.StatusCode, parsed)
}

var _ = errors.New

// DeveloperBillingShow ports Developers::BillingController#show.
func DeveloperBillingShow(c *web.Context) {
	d := developers.Authenticate(c)
	e := developers.EntitlementFor(c.Ctx, d)
	sub := developers.SubscriptionOf(c.Ctx, d.ID)
	var subscription any
	if sub != nil {
		subscription = rb.M("plan_code", sub.PlanCode, "status", sub.Status, "interval", sub.Interval, "currency", sub.Currency,
			"cancel_at_period_end", sub.CancelAtPeriodEnd, "trial_ends_at", timeAttr(timeOrNilPtr(sub.TrialEndsAt)),
			"current_period_end", timeAttr(timeOrNilPtr(sub.CurrentPeriodEnd)), "canceled_at", timeAttr(timeOrNilPtr(sub.CanceledAt)))
	}
	c.JSON(200, rb.M("entitlement", e.AsJSON(), "legacy_free", d.Get("legacy_free"),
		"trial_available", sub == nil && d.Get("legacy_free") != true, "trial_period_days", billing.TrialPeriodDays(),
		"can_create_key", developers.CanCreateKey(c.Ctx, d), "max_keys", d.Get("max_keys"),
		"keys_count", developers.KeysCount(c.Ctx, d), "support_email", billing.SupportEmail(), "subscription", subscription))
}

func timeOrNilPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// DeveloperBillingCheckout ports #checkout.
func DeveloperBillingCheckout(c *web.Context) {
	d := developers.Authenticate(c)
	result, err := developers.StartCheckout(c.Ctx, d, c.Param("plan_code"), c.Param("interval"), c.Param("currency"), c.Param("locale"))
	var unavailable developers.UnavailablePlan
	var pending developers.CheckoutPending
	switch {
	case err == nil:
		c.JSON(200, rb.M("url", result.URL, "plan_code", result.PlanCode, "trial_period_days", result.TrialPeriodDays,
			"reused", result.Reused))
	case errors.As(err, new(developers.AlreadySubscribed)):
		c.JSON(422, rb.M("error", "Change plans in the billing portal", "code", "ALREADY_SUBSCRIBED"))
	case errors.As(err, &unavailable):
		c.JSON(422, rb.M("error", unavailable.Message, "code", "PLAN_UNAVAILABLE"))
	case errors.As(err, &pending):
		s := pending.Session
		var url any
		if s.URL != nil {
			url = *s.URL
		}
		c.JSON(409, rb.M("error", pending.Error(), "code", "CHECKOUT_PENDING", "url", url, "plan_code", s.PlanCode,
			"interval", s.Interval, "currency", s.Currency, "expires_at", rb.FormatTime(s.ExpiresAt)))
	default:
		panic(err)
	}
}

// DeveloperBillingPortal ports #portal.
func DeveloperBillingPortal(c *web.Context) {
	d := developers.Authenticate(c)
	url, err := developers.OpenPortal(c.Ctx, d, c.Param("locale"))
	if errors.As(err, new(developers.NoCustomer)) {
		c.JSON(422, rb.M("error", "No subscription to manage yet", "code", "NO_STRIPE_CUSTOMER"))
		return
	}
	if err != nil {
		panic(err)
	}
	c.JSON(200, rb.M("url", url))
}

// WebhooksStripeCreate ports Webhooks::StripeController#create.
func WebhooksStripeCreate(c *web.Context) {
	payload := c.RawBody()
	if msg := developers.VerifySignature(payload, c.HeaderValue("Stripe-Signature")); msg != "" {
		c.JSON(400, rb.M("error", "Invalid signature"))
		return
	}
	parsed, err := rb.ParseJSON(payload)
	if err != nil {
		c.JSON(400, rb.M("error", "Invalid payload"))
		return
	}
	var event *rb.Map
	switch x := parsed.(type) {
	case *rb.Map:
		event = x
	case nil, string:
		// nil becomes {}; String#[]("type") is nil or "type", neither a
		// handled event.
		event = rb.NewMap()
	default:
		panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of String into Integer"})
	}
	status, err := developers.ProcessWebhookEvent(c.Ctx, event)
	if err != nil {
		panic(err)
	}
	c.JSON(200, rb.M("received", true, "status", status))
}
