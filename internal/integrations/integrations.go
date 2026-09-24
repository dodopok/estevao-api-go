// Package integrations ports the shared outbound-HTTP policy of the Rails
// app (Integrations::HttpClient and Integrations::RetryPolicy).
package integrations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/web"
)

const (
	DefaultTimeout = 30
	minTimeout     = 1
	maxTimeout     = 120
)

// TimeoutSeconds ports HttpClient.timeout_seconds:
// Integer(ENV[key].presence || default).clamp(1, 120), falling back to the
// default when the value is not an integer literal.
func TimeoutSeconds(envKey string, def int) int {
	n := def
	if v := config.Get(envKey); strings.TrimSpace(v) != "" {
		parsed, ok := rubyInteger(v)
		if ok {
			n = parsed
		}
	}
	return min(max(n, minTimeout), maxTimeout)
}

// Client is an http.Client with the connect/read/write timeout of envKey.
func Client(envKey string, def int) *http.Client {
	return &http.Client{Timeout: time.Duration(TimeoutSeconds(envKey, def)) * time.Second}
}

// rubyInteger is Kernel#Integer on a String: optional surrounding
// whitespace, sign, digits with single underscores, 0x/0b/0o/0 prefixes.
func rubyInteger(s string) (int, bool) {
	s = strings.TrimSpace(s)
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	base := 10
	switch {
	case len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X"):
		base, s = 16, s[2:]
	case len(s) > 2 && (s[:2] == "0b" || s[:2] == "0B"):
		base, s = 2, s[2:]
	case len(s) > 2 && (s[:2] == "0o" || s[:2] == "0O"):
		base, s = 8, s[2:]
	case len(s) > 1 && s[0] == '0':
		base, s = 8, s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c == '_' {
			continue
		}
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		if d >= base {
			return 0, false
		}
		n = n*base + d
		if n > 1<<40 {
			n = 1 << 40
		}
	}
	if neg {
		n = -n
	}
	return n, true
}

// Retry ports RetryPolicy.call: transport failures (timeouts, refused or
// reset connections, DNS errors) are retried up to maxAttempts times with a
// linear backoff, then raised as the given infrastructure error class.
// HTTP responses of any status are returned as they are.
func Retry(ctx context.Context, operation string, maxAttempts int, backoff time.Duration, class, code string,
	do func() (*http.Response, error)) *http.Response {
	maxAttempts = min(max(maxAttempts, 1), 5)
	integration := strings.SplitN(operation, ".", 2)[0]
	for attempt := 1; ; attempt++ {
		resp, err := do()
		if err == nil {
			return resp
		}
		if errors.Is(err, context.Canceled) || attempt >= maxAttempts {
			panic(web.NewInfraError(class, integration+" is temporarily unavailable", code))
		}
		if backoff > 0 {
			select {
			case <-time.After(backoff * time.Duration(attempt)):
			case <-ctx.Done():
			}
		}
	}
}

// URL returns the base URL of an external service, overridable through
// envKey so the differential tests can point both stacks at a local fake.
func URL(envKey, def string) string {
	if v := strings.TrimSpace(config.Get(envKey)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return def
}
