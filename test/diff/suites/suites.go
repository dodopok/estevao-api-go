// Package suites defines the request matrices replayed by cmd/difftest.
package suites

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/dodopok/estevao-api-go/test/diff"
)

var registry = map[string]func() []diff.Request{}

func register(name string, f func() []diff.Request) { registry[name] = f }

// Names lists registered suites.
func Names() []string {
	var out []string
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Build returns the requests of a suite.
func Build(name string) ([]diff.Request, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown suite %q (known: %v)", name, Names())
	}
	return f(), nil
}

// Books are all seeded prayer book codes.
var Books = []string{
	"awrv_2025_en", "awrv_2025_pt", "cw_2005_en", "dwdo_2021", "loc_1549", "loc_1662", "loc_1662_en",
	"loc_1928_en", "loc_1928_pt", "loc_1962_en", "loc_1979_en", "loc_1979_es", "loc_1984_cy", "loc_1984_en",
	"loc_1987", "loc_1991_pt", "loc_2015", "loc_2019", "loc_2019_en", "loc_2019_es", "loc_2021", "loc_2027",
	"locb_2008", "rec_2005_en",
}

// AppHeaders identify an anonymous app request and bypass the per-IP
// throttle through the trusted-server budget.
func AppHeaders() map[string]string {
	return map[string]string{
		"X-App-Internal-Id":    "test-app-internal-id",
		"X-Trusted-Server-Key": "trusted-server-test-key",
		"Accept":               "application/json",
	}
}

func withHeaders(h map[string]string, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func pref(code string) string { return "preferences%5Bprayer_book_code%5D=" + url.QueryEscape(code) }

// TokenFor mints a test token for a uid.
func TokenFor(uid, email string) string {
	_, file, _, _ := runtime.Caller(0)
	key := filepath.Join(filepath.Dir(file), "..", "..", "oracle", "test_firebase_key.pem")
	t, err := diff.Token(key, envOr("FIREBASE_PROJECT_ID", "estevao-test"), map[string]any{"sub": uid, "email": email, "user_id": uid})
	if err != nil {
		panic(err)
	}
	return t
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
