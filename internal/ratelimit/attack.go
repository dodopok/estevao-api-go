// Package ratelimit ports config/initializers/rack_attack.rb. Counters live
// in the shared Redis under the same keys Rack::Attack writes through the
// Rails cache, so both stacks meter one budget during a transition.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/web"
)

const stripeWebhookPath = "/api/v1/webhooks/stripe"

type throttle struct {
	name    string
	period  int
	limit   func(r *request) int
	discrim func(r *request) string
}

type request struct {
	*http.Request
	path       string
	multiplier *int
}

func (r *request) apiKeyMultiplier() int {
	if r.multiplier == nil {
		m := auth.RateLimitMultiplierFor(r.Context(), r.Header.Get("X-API-Key"))
		r.multiplier = &m
	}
	return *r.multiplier
}

func sha32(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:32]
}

func apiKeyDailyLimit() int { return config.PositiveInt("API_KEY_DAILY_LIMIT", 1000) }

func trustedServerLimit() int { return config.PositiveInt("TRUSTED_SERVER_RATE_LIMIT", 6000) }

func trustedServer(r *request) bool {
	expected := config.Get("TRUSTED_SERVER_KEY")
	if strings.TrimSpace(expected) == "" {
		return false
	}
	provided := r.Header.Get("X-Trusted-Server-Key")
	if strings.TrimSpace(provided) == "" {
		return false
	}
	return auth.SecureCompare(provided, expected)
}

var throttles = []throttle{
	{"api_key/daily", 86400, func(r *request) int { return apiKeyDailyLimit() * r.apiKeyMultiplier() }, func(r *request) string {
		if k := r.Header.Get("X-API-Key"); strings.TrimSpace(k) != "" {
			return sha32(k)
		}
		return ""
	}},
	{"api_key/minute", 60, func(r *request) int { return 60 * r.apiKeyMultiplier() }, func(r *request) string {
		if k := r.Header.Get("X-API-Key"); strings.TrimSpace(k) != "" {
			return sha32(k)
		}
		return ""
	}},
	{"onboarding/ip", 3600, func(*request) int { return 60 }, func(r *request) string {
		if r.path == "/api/v1/users/onboarding" && r.Method == http.MethodPost {
			return ClientIP(r.Request)
		}
		return ""
	}},
	{"onboarding/user", 3600, func(*request) int { return 10 }, func(r *request) string {
		if r.path == "/api/v1/users/onboarding" && r.Method == http.MethodPost {
			h := r.Header.Get("Authorization")
			if strings.HasPrefix(h, "Bearer ") {
				parts := strings.Split(h, "Bearer ")
				return sha32(parts[len(parts)-1])
			}
		}
		return ""
	}},
	{"stripe_webhook/ip", 60, func(*request) int { return 300 }, func(r *request) string {
		if r.path == stripeWebhookPath && r.Method == http.MethodPost {
			return ClientIP(r.Request)
		}
		return ""
	}},
	{"billing/developer", 60, func(*request) int { return 12 }, func(r *request) string {
		if r.Method == http.MethodPost && strings.HasPrefix(r.path, "/api/v1/developers/billing") {
			h := r.Header.Get("Authorization")
			if strings.HasPrefix(h, "Bearer ") {
				return sha32(h)
			}
		}
		return ""
	}},
	{"trusted_server/minute", 60, func(*request) int { return trustedServerLimit() }, func(r *request) string {
		if strings.HasPrefix(r.path, "/api/") && trustedServer(r) {
			return "trusted_server"
		}
		return ""
	}},
	{"api/ip", 60, func(*request) int { return 100 }, func(r *request) string {
		if strings.HasPrefix(r.path, "/api/") && r.path != stripeWebhookPath {
			key := r.Header.Get("X-API-Key")
			privileged := strings.TrimSpace(key) != "" && r.apiKeyMultiplier() > 1
			if !privileged && !trustedServer(r) {
				return ClientIP(r.Request)
			}
		}
		return ""
	}},
}

// normalizePath ports Rack::Attack::PathNormalizer (squeeze '/', drop the
// trailing slash, unescape).
func normalizePath(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// Middleware returns the Rack::Attack handler for web.Server.
func Middleware() web.Middleware {
	return func(hr *http.Request) *web.Response {
		r := &request{Request: hr, path: normalizePath(hr.URL.Path)}
		if r.path == "/up" || r.path == "/ready" {
			return nil
		}
		now := time.Now().Unix()
		for _, t := range throttles {
			d := t.discrim(r)
			if d == "" {
				continue
			}
			period := int64(t.period)
			expires := period - (now % period) + 1
			key := "rack::attack:" + strconv.FormatInt(now/period, 10) + ":" + t.name + ":" + d
			count, ok := rediscache.Increment(context.Background(), key, 1, time.Duration(expires)*time.Second)
			if !ok {
				count = 1
			}
			if count > int64(t.limit(r)) {
				return throttled(t, now)
			}
		}
		return nil
	}
}

func throttled(t throttle, now int64) *web.Response {
	period := int64(t.period)
	retryAfter := period - (now % period)
	msg := "Too many requests. Please try again later."
	if strings.HasPrefix(t.name, "api_key/") {
		msg = "API key rate limit exceeded. Please try again later."
	} else if t.name == "trusted_server/minute" {
		msg = "Trusted server rate limit exceeded. Please try again later."
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
	body := rb.JSON(rb.M("success", false, "error", rb.M("code", "RATE_LIMIT_EXCEEDED", "message", msg, "retry_after", int(retryAfter))))
	return &web.Response{Status: 429, Header: h, Body: body}
}

var octet = `\.(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])`
var trustedProxy = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\A127` + octet + `{3}\z`, `\A::1\z`, `\Af[cd][0-9a-f]{2}(?::[0-9a-f]{0,4}){0,7}\z`,
	`\A10` + octet + `{3}\z`, `\A172\.(1[6-9]|2[0-9]|3[01])` + octet + `{2}\z`,
	`\A192\.168` + octet + `{2}\z`, `\Alocalhost\z`, `\Aunix(\z|:)`,
}, "|"))

func splitHeader(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hostOf(authority string) string {
	a := strings.TrimSpace(authority)
	if strings.HasPrefix(a, "[") {
		if i := strings.Index(a, "]"); i > 0 {
			return a[1:i]
		}
	}
	if strings.Count(a, ":") == 1 {
		return a[:strings.Index(a, ":")]
	}
	return a
}

// ClientIP ports Rack::Request#ip.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := splitHeader(host)
	for i := len(remote) - 1; i >= 0; i-- {
		if !trustedProxy.MatchString(remote[i]) {
			return remote[i]
		}
	}
	var fwd []string
	if f := r.Header.Get("Forwarded"); f != "" {
		for _, part := range strings.Split(f, ",") {
			for _, kv := range strings.Split(part, ";") {
				kv = strings.TrimSpace(kv)
				if strings.HasPrefix(strings.ToLower(kv), "for=") {
					fwd = append(fwd, hostOf(strings.Trim(kv[4:], `"`)))
				}
			}
		}
	}
	if fwd == nil {
		if v, ok := r.Header["X-Forwarded-For"]; ok {
			for _, a := range splitHeader(strings.Join(v, ",")) {
				fwd = append(fwd, hostOf(a))
			}
		}
	}
	if len(fwd) > 0 {
		for i := len(fwd) - 1; i >= 0; i-- {
			if !trustedProxy.MatchString(fwd[i]) {
				return fwd[i]
			}
		}
		return fwd[0]
	}
	if len(remote) > 0 {
		return remote[0]
	}
	return ""
}
