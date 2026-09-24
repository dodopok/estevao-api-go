// Package subscriptions ports Subscriptions::Verify and RevenueCatService.
package subscriptions

import (
	"context"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

const clientErrorClass = "Integrations::RevenueCat::Client::Error"

// info is the hash parse_subscription_response returns for an active
// entitlement.
type info struct {
	expiresAt         time.Time
	productIdentifier any
	willRenew         any
}

// apiKey ports RevenueCatService#initialize.
func apiKey() string {
	key := config.Get("REVENUECAT_API_KEY")
	if strings.TrimSpace(key) == "" {
		panic(web.NewInfraError("ExternalServiceUnavailable", "Subscription verification is temporarily unavailable", "REVENUECAT_NOT_CONFIGURED"))
	}
	return key
}

// subscriber ports Integrations::RevenueCat::Client#subscriber: the body of
// a 200, nil for a 404 (NotFound, rescued by verify_subscription).
func subscriber(ctx context.Context, key, userID string) (string, bool) {
	endpoint := integrations.URL("REVENUECAT_API_URL", "https://api.revenuecat.com/v1") + "/subscribers/" + userID
	client := integrations.Client("REVENUECAT_HTTP_TIMEOUT", integrations.DefaultTimeout)
	resp := integrations.Retry(ctx, "revenue_cat.subscriber", 2, 100*time.Millisecond, clientErrorClass, "REVENUECAT_UNAVAILABLE",
		func() (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
			return client.Do(req)
		})
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(web.NewInfraError(clientErrorClass, "revenue_cat is temporarily unavailable", "REVENUECAT_UNAVAILABLE"))
	}
	switch resp.StatusCode {
	case 200:
		return string(body), true
	case 404:
		return "", false
	}
	panic(web.NewInfraError(clientErrorClass, "RevenueCat request failed with status "+strconv.Itoa(resp.StatusCode), "REVENUECAT_REQUEST_FAILED"))
}

func fail(class, message string) { panic(&web.StandardError{Class: class, Message: message}) }

// index ports recv[key] for a decoded JSON value.
func index(recv any, key string) any {
	switch x := recv.(type) {
	case *rb.Map:
		return x.Get(key)
	case string:
		if strings.Contains(x, key) {
			return key
		}
		return nil
	case []any, int, int64:
		fail("TypeError", "no implicit conversion of String into Integer")
	}
	fail("NoMethodError", rb.NoMethodErrorMessage("[]", recv))
	return nil
}

func truthy(v any) bool { return v != nil && v != false }

// parse ports parse_subscription_response.
func parse(body string, now time.Time) *info {
	data, err := rb.ParseJSON([]byte(body))
	if err != nil {
		panic(web.NewInfraError(clientErrorClass, "RevenueCat returned an invalid response", "REVENUECAT_INVALID_RESPONSE"))
	}
	sub := index(data, "subscriber")
	if !truthy(sub) {
		return nil
	}
	var entitlements any
	switch x := sub.(type) {
	case *rb.Map:
		entitlements = x.Get("entitlements")
	case []any:
		fail("TypeError", "no implicit conversion of String into Integer")
	default:
		fail("NoMethodError", rb.NoMethodErrorMessage("dig", sub))
	}
	if !truthy(entitlements) {
		entitlements = rb.NewMap()
	}
	ents, ok := entitlements.(*rb.Map)
	if !ok {
		fail("NoMethodError", rb.NoMethodErrorMessage("values", entitlements))
	}
	for _, k := range ents.Keys() {
		e := ents.Get(k)
		exp := index(e, "expires_date")
		if !rb.Present(exp) {
			return &info{expiresAt: rb.AdvanceYears(now, 100), productIdentifier: index(e, "product_identifier"), willRenew: orFalse(index(e, "will_renew"))}
		}
		t := timeParse(exp, now)
		if t.After(now) {
			return &info{expiresAt: t, productIdentifier: index(e, "product_identifier"), willRenew: orFalse(index(e, "will_renew"))}
		}
	}
	return nil
}

func orFalse(v any) any {
	if truthy(v) {
		return v
	}
	return false
}

func timeParse(v any, now time.Time) time.Time {
	s, ok := v.(string)
	if !ok {
		fail("TypeError", rb.ImplicitConversionMessage(v, "String"))
	}
	t, err := rb.TimeParse(s, now, time.Local)
	if err != nil {
		fail("ArgumentError", err.Error())
	}
	return t
}

// UpdateUserPremiumStatus ports RevenueCatService#update_user_premium_status.
func UpdateUserPremiumStatus(ctx context.Context, u *users.User) {
	key := apiKey()
	if u.RevenueCatUserID == nil || rb.BlankString(*u.RevenueCatUserID) {
		return
	}
	previous := u.PremiumExpiresAt
	now := time.Now()
	var sub *info
	if body, found := subscriber(ctx, key, *u.RevenueCatUserID); found {
		sub = parse(body, now)
	}
	if sub != nil {
		setPremiumExpiresAt(ctx, u, &sub.expiresAt)
		recordEvent(ctx, u, previous, sub, true)
		return
	}
	if u.PremiumExpiresAt != nil {
		setPremiumExpiresAt(ctx, u, nil)
		recordEvent(ctx, u, previous, nil, false)
	}
}

// setPremiumExpiresAt ports user.update(premium_expires_at: v), which
// returns false (writing nothing) when the user is invalid.
func setPremiumExpiresAt(ctx context.Context, u *users.User, v *time.Time) {
	before := u.Clone()
	u.PremiumExpiresAt = v
	if _, err := users.Save(ctx, db.Q(), before, u); err != nil {
		if _, invalid := err.(*users.RecordInvalid); !invalid {
			panic(err)
		}
	}
}

// rubyToF is Time#to_f rendered by Float#to_s. Ruby keeps the time as an
// integer count of nanoseconds and, unless it is a whole second, converts
// that count to a double before dividing by 1e9 (rb_time_unmagnify_to_float).
func rubyToF(t time.Time) string {
	if t.Nanosecond() == 0 {
		return rb.FloatToS(float64(t.Unix()))
	}
	ns := new(big.Int).Mul(big.NewInt(t.Unix()), big.NewInt(1_000_000_000))
	ns.Add(ns, big.NewInt(int64(t.Nanosecond())))
	f, _ := new(big.Float).SetInt(ns).Float64()
	return rb.FloatToS(f / 1e9)
}

// recordEvent ports record_subscription_event.
func recordEvent(ctx context.Context, u *users.User, previous *time.Time, sub *info, active bool) {
	var eventType string
	switch {
	case !active:
		eventType = "expired"
	case previous == nil:
		eventType = "started"
	case sub.expiresAt.After(*previous):
		eventType = "renewed"
	default:
		return
	}
	var expiresAt *time.Time
	if sub != nil {
		expiresAt = &sub.expiresAt
	} else {
		expiresAt = previous
	}
	keyParts := []string{strconv.FormatInt(u.ID, 10), eventType, ""}
	if expiresAt != nil {
		keyParts[2] = rubyToF(*expiresAt)
	}
	eventKey := strings.Join(keyParts, ":")
	var exists bool
	must(db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM premium_subscription_events WHERE event_key = $1)`, eventKey).Scan(&exists))
	if exists {
		return
	}
	var product, willRenew any
	if sub != nil {
		product = castString(sub.productIdentifier)
		willRenew = castBool(sub.willRenew)
	}
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Truncate(time.Microsecond)
	}
	now := users.Now()
	_, err := db.Q().Exec(ctx, `INSERT INTO premium_subscription_events
		(user_id, event_type, event_key, product_identifier, expires_at, will_renew, occurred_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7) ON CONFLICT (event_key) DO NOTHING`,
		u.ID, eventType, eventKey, product, exp, willRenew, now)
	must(err)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// castString ports ActiveModel::Type::String#cast.
func castString(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case bool:
		if x {
			return "t"
		}
		return "f"
	}
	return rb.ToS(v)
}

// castBool ports ActiveModel::Type::Boolean#cast.
func castBool(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case string:
		if x == "" {
			return nil
		}
		switch x {
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false
		}
		return true
	case int:
		return x != 0
	case float64:
		return x != 0
	}
	return true
}
