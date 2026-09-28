package developers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

const stripeAPIVersion = "2024-06-20"

// StripeError ports Billing::StripeClient::Error.
type StripeError struct {
	Message   string
	Status    int
	Uncertain bool
}

func (e *StripeError) Error() string { return e.Message }

// StripeClient ports Billing::StripeClient.
type StripeClient struct{ secret string }

// NewStripeClient ports StripeClient.new (ConfigurationError without a key).
func NewStripeClient() (*StripeClient, error) {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if strings.TrimSpace(key) == "" {
		return nil, &StripeError{Message: "STRIPE_SECRET_KEY is not set"}
	}
	return &StripeClient{secret: key}, nil
}

func stripeBase() string { return integrations.URL("STRIPE_API_URL", "https://api.stripe.com/v1") }

// kv is one ordered form field (Rack::Utils.build_nested_query input).
type kv struct {
	K string
	V any // string, bool, int, nil, []kv
}

// buildNestedQuery ports Rack::Utils.build_nested_query on hashes (arrays
// are indexed into hashes first, as StripeClient#indexed does): the full
// bracketed key is escaped at each leaf.
func buildNestedQuery(fields []kv, prefix string) string {
	var parts []string
	for _, f := range fields {
		key := f.K
		if prefix != "" {
			key = prefix + "[" + f.K + "]"
		}
		switch v := f.V.(type) {
		case []kv:
			if s := buildNestedQuery(v, key); s != "" {
				parts = append(parts, s)
			}
		case nil:
			parts = append(parts, formEscape(key))
		default:
			parts = append(parts, formEscape(key)+"="+formEscape(rb.ToS(v)))
		}
	}
	return strings.Join(parts, "&")
}

// formEscape ports URI.encode_www_form_component.
func formEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '*', c == '-', c == '.', c == '_':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *StripeClient) request(ctx context.Context, method, path, body, idempotencyKey string) (*rb.Map, error) {
	var reader io.Reader
	if method == http.MethodPost {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, stripeBase()+"/"+path, reader)
	if err != nil {
		return nil, &StripeError{Message: "Stripe request failed: " + err.Error(), Uncertain: true}
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if strings.TrimSpace(idempotencyKey) == "" {
			idempotencyKey = uuid()
		}
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	req.Header.Set("Authorization", "Bearer "+s.secret)
	req.Header.Set("Stripe-Version", stripeAPIVersion)
	client := &http.Client{Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &StripeError{Message: "Stripe request failed: " + err.Error(), Uncertain: true}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	parsed := rb.NewMap()
	if v, err := rb.ParseJSON(raw); err == nil {
		if m, ok := v.(*rb.Map); ok {
			parsed = m
		}
	}
	if resp.StatusCode/100 == 2 {
		return parsed, nil
	}
	msg := "Stripe request failed (" + strconv.Itoa(resp.StatusCode) + ")"
	if e, ok := parsed.Get("error").(*rb.Map); ok {
		if m, ok := e.Get("message").(string); ok && strings.TrimSpace(m) != "" {
			msg = m
		}
	}
	return nil, &StripeError{Message: msg, Status: resp.StatusCode, Uncertain: resp.StatusCode >= 500}
}

// CreateCustomer ports create_customer.
func (s *StripeClient) CreateCustomer(ctx context.Context, email string, name any, developerID int64, idempotencyKey string) (*rb.Map, error) {
	fields := []kv{{"email", email}}
	if name != nil {
		fields = append(fields, kv{"name", name})
	}
	fields = append(fields, kv{"metadata", []kv{{"developer_id", strconv.FormatInt(developerID, 10)}}})
	return s.request(ctx, http.MethodPost, "customers", buildNestedQuery(fields, ""), idempotencyKey)
}

// CheckoutParams are create_checkout_session's arguments.
type CheckoutParams struct {
	PriceID, CustomerID, SuccessURL, CancelURL, Locale, IdempotencyKey string
	TrialPeriodDays                                                    int64
	Metadata                                                           []kv
	ExpiresAt                                                          time.Time
}

// CreateCheckoutSession ports create_checkout_session.
func (s *StripeClient) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (*rb.Map, error) {
	sub := []kv{{"metadata", p.Metadata}}
	if p.TrialPeriodDays > 0 {
		sub = append(sub, kv{"trial_period_days", p.TrialPeriodDays})
	}
	fields := []kv{{"mode", "subscription"}, {"customer", p.CustomerID}, {"success_url", p.SuccessURL},
		{"cancel_url", p.CancelURL}, {"allow_promotion_codes", true},
		{"line_items", []kv{{"0", []kv{{"price", p.PriceID}, {"quantity", 1}}}}},
		{"subscription_data", sub}, {"metadata", p.Metadata}}
	if strings.TrimSpace(p.Locale) != "" {
		fields = append(fields, kv{"locale", p.Locale})
	}
	fields = append(fields, kv{"expires_at", p.ExpiresAt.Unix()})
	return s.request(ctx, http.MethodPost, "checkout/sessions", buildNestedQuery(fields, ""), p.IdempotencyKey)
}

// CreateBillingPortalSession ports create_billing_portal_session.
func (s *StripeClient) CreateBillingPortalSession(ctx context.Context, customer, returnURL, locale string) (*rb.Map, error) {
	fields := []kv{{"customer", customer}, {"return_url", returnURL}}
	if strings.TrimSpace(locale) != "" {
		fields = append(fields, kv{"locale", locale})
	}
	return s.request(ctx, http.MethodPost, "billing_portal/sessions", buildNestedQuery(fields, ""), "")
}

// RetrieveSubscription ports retrieve_subscription.
func (s *StripeClient) RetrieveSubscription(ctx context.Context, id string) (*rb.Map, error) {
	return s.request(ctx, http.MethodGet, "subscriptions/"+url.QueryEscape(id), "", "")
}

// --- PortalUrls -------------------------------------------------------------

var localePrefixes = map[string]string{"pt-BR": "", "en": "/en", "es": "/es"}

// NormalizeLocale ports PortalUrls.normalize_locale.
func NormalizeLocale(locale any) string {
	if rb.Blank(locale) {
		return "pt-BR"
	}
	if _, ok := localePrefixes[rb.ToS(locale)]; ok {
		return rb.ToS(locale)
	}
	return "pt-BR"
}

// BillingURL ports PortalUrls.billing_url (query is {checkout: value} or empty).
func BillingURL(locale any, checkout string) string {
	base := "https://estevao.caminhoanglicano.com.br"
	if v, ok := os.LookupEnv("PORTAL_BASE_URL"); ok {
		base = v
	}
	base = strings.TrimSuffix(base, "/")
	u := base + localePrefixes[NormalizeLocale(locale)] + "/dashboard/billing"
	if checkout != "" {
		u += "?checkout=" + url.QueryEscape(checkout)
	}
	return u
}

// --- StripeSignature ----------------------------------------------------------

// VerifySignature ports Billing::StripeSignature.verify! (nil when valid,
// else the VerificationError message).
func VerifySignature(payload []byte, header string) string {
	secret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if strings.TrimSpace(secret) == "" {
		return "STRIPE_WEBHOOK_SECRET is not set"
	}
	if strings.TrimSpace(header) == "" {
		return "Missing signature"
	}
	var ts *int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "t" && ts == nil {
			n := rb.StringToI(v)
			n64 := int64(n)
			ts = &n64
		}
		if k == "v1" {
			sigs = append(sigs, v)
		}
	}
	if ts == nil || len(sigs) == 0 {
		return "Malformed signature"
	}
	if d := time.Now().Unix() - *ts; d > 300 || d < -300 {
		return "Signature timestamp outside tolerance"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(*ts, 10) + "." + string(payload)))
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, s := range sigs {
		if len(s) == len(expected) && subtle.ConstantTimeCompare([]byte(s), []byte(expected)) == 1 {
			return ""
		}
	}
	return "Signature mismatch"
}

var _ = sort.Strings
