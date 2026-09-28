package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// APIKeyRecord is the authentication slice of an api_keys row.
type APIKeyRecord struct {
	ID            int64
	Key           string
	Active        bool
	BillingActive bool
	ExpiresAt     *time.Time
	RequestsCount int
	LastUsedAt    *time.Time
}

// Usable ports ApiKey#usable?.
func (k *APIKeyRecord) Usable() bool {
	expired := k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now())
	return k.Active && k.BillingActive && !expired
}

const apiKeyValue = "api_key"

// APIKey returns the request's authenticated key or nil.
func APIKey(c *web.Context) *APIKeyRecord {
	k, _ := c.Get(apiKeyValue).(*APIKeyRecord)
	return k
}

// AuthenticateAPIKeyValue ports ApiKey.authenticate(key, track_usage: false).
func AuthenticateAPIKeyValue(ctx context.Context, value string) (*APIKeyRecord, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var k APIKeyRecord
	err := db.Q().QueryRow(ctx, `SELECT id, key, active, billing_active, expires_at, requests_count, last_used_at FROM api_keys WHERE key = $1 LIMIT 1`, value).
		Scan(&k.ID, &k.Key, &k.Active, &k.BillingActive, &k.ExpiresAt, &k.RequestsCount, &k.LastUsedAt)
	if db.NoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !k.Usable() {
		return nil, nil
	}
	return &k, nil
}

// AuthenticateAPIKey ports authenticate_api_key.
func AuthenticateAPIKey(c *web.Context) bool {
	v := c.HeaderValue("X-API-Key")
	if strings.TrimSpace(v) == "" {
		return false
	}
	k, err := AuthenticateAPIKeyValue(c.Ctx, v)
	if err != nil {
		panic(err)
	}
	if k != nil {
		c.Set(apiKeyValue, k)
		return true
	}
	return false
}

// AuthenticateUserOrAPIKey ports authenticate_user_or_api_key.
func AuthenticateUserOrAPIKey(c *web.Context) {
	if AuthenticateAPIKey(c) {
		return
	}
	AuthenticateOptional(c)
}

// RecordAPIKeyUsage ports the after_action log_api_key_usage:
// ApiKeyUsageCounter.record!(id, "controller#action").
func RecordAPIKeyUsage(c *web.Context, controllerName string) {
	k := APIKey(c)
	if k == nil {
		return
	}
	endpoint := controllerName + "#" + c.Action
	date := time.Now().In(appZone()).Format("2006-01-02")
	key := "v8/api_key_usage/counter/" + itoa(k.ID) + "/" + date + "/" + base64.RawURLEncoding.EncodeToString([]byte(endpoint))
	rediscache.Increment(c.Ctx, key, 1, 48*time.Hour)
}

// multiplierCacheKey holds the active keys' multipliers, shared by every
// instance through Redis as Rails shares ApiKey's through Rails.cache.
const multiplierCacheKey = "api_key/rate_limit_multipliers/v1"

// RateLimitMultiplierFor ports ApiKey.rate_limit_multiplier_for, with the
// same 5-minute cache of active keys.
func RateLimitMultiplierFor(ctx context.Context, value string) int {
	if strings.TrimSpace(value) == "" {
		return 1
	}
	failed := false
	raw := rediscache.FetchJSON(ctx, multiplierCacheKey, 5*time.Minute, func() []byte {
		rows, err := db.Q().Query(ctx, `SELECT key, rate_limit_multiplier, expires_at FROM api_keys
			WHERE active = TRUE AND billing_active = TRUE AND (expires_at IS NULL OR expires_at > $1)`, time.Now().UTC())
		if err != nil {
			failed = true
			return nil
		}
		defer rows.Close()
		m := map[string]multEntry{}
		for rows.Next() {
			var key string
			var e multEntry
			if err := rows.Scan(&key, &e.Multiplier, &e.ExpiresAt); err == nil {
				sum := sha256.Sum256([]byte(key))
				m[hex.EncodeToString(sum[:])] = e
			}
		}
		b, _ := json.Marshal(m)
		return b
	})
	if failed {
		rediscache.DeleteJSON(ctx, multiplierCacheKey)
		return 1
	}
	var byHash map[string]multEntry
	if json.Unmarshal(raw, &byHash) != nil {
		return 1
	}
	sum := sha256.Sum256([]byte(value))
	e, ok := byHash[hex.EncodeToString(sum[:])]
	if !ok || e.Multiplier == 0 || (e.ExpiresAt != nil && !e.ExpiresAt.After(time.Now())) {
		return 1
	}
	return e.Multiplier
}

// ClearRateLimitMultiplierCache ports ApiKey#invalidate_caches (after every
// api_keys commit): the multiplier cache of both stacks and the Rails
// authentication cache of the keys, which would otherwise let the Rails
// stack keep accepting a revoked key until the entry expires.
func ClearRateLimitMultiplierCache() {
	ctx := context.Background()
	rediscache.DeleteJSON(ctx, multiplierCacheKey)
	rediscache.DeleteRails(ctx, "v8/api_keys/rate_limit_multipliers", "v8/api_keys/authentication/v2/*")
}

type multEntry struct {
	Multiplier int        `json:"m"`
	ExpiresAt  *time.Time `json:"e"`
}
