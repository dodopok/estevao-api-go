// Package diff is the differential test harness: it replays the same HTTP
// scenario against the Rails oracle and the Go server and reports every
// observable difference, after normalizing only values that are volatile
// by nature (request ids, New Relic trace ids, runtimes, dates).
package diff

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Request is one HTTP call of a scenario.
type Request struct {
	Name    string
	Method  string
	Path    string
	Headers map[string]string
	Body    string
	// Volatile lists extra JSON keys whose values are expected to differ
	// between runs (e.g. created_at of a record created by this request).
	Volatile []string
}

// Result is a normalized response.
type Result struct {
	Status  int
	Header  http.Header
	Body    []byte
	Changed bool // normalization replaced something in the body
}

var client = &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// Do executes req against base.
func Do(base string, req Request) (*Result, error) {
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	hr, err := http.NewRequest(method, base+req.Path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		hr.Header.Set(k, v)
	}
	resp, err := client.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return &Result{Status: resp.StatusCode, Header: resp.Header, Body: b}, nil
}

var alwaysVolatile = []string{"request_id", "trace_id"}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// Normalize replaces volatile JSON values with a placeholder.
func Normalize(r *Result, volatile []string) {
	keys := append(append([]string{}, alwaysVolatile...), volatile...)
	b := r.Body
	for _, k := range keys {
		re := regexp.MustCompile(`"` + regexp.QuoteMeta(k) + `":("(?:[^"\\]|\\.)*"|-?[0-9.eE+]+|null|true|false)`)
		nb := re.ReplaceAll(b, []byte(`"`+k+`":"<volatile>"`))
		if !bytes.Equal(nb, b) {
			r.Changed = true
		}
		b = nb
	}
	r.Body = b
}

var ignoredHeaders = map[string]bool{"Date": true, "X-Request-Id": true, "X-Runtime": true, "Last-Modified": true}

// Compare returns human-readable differences (empty when equivalent).
func Compare(a, b *Result) []string {
	var out []string
	if a.Status != b.Status {
		out = append(out, fmt.Sprintf("status rails=%d go=%d", a.Status, b.Status))
	}
	names := map[string]bool{}
	for k := range a.Header {
		names[k] = true
	}
	for k := range b.Header {
		names[k] = true
	}
	var sorted []string
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		if ignoredHeaders[k] {
			continue
		}
		if (k == "Etag" || k == "Content-Length") && (a.Changed || b.Changed) {
			continue
		}
		av, bv := strings.Join(a.Header.Values(k), ", "), strings.Join(b.Header.Values(k), ", ")
		if av != bv {
			out = append(out, fmt.Sprintf("header %s rails=%q go=%q", k, av, bv))
		}
	}
	if !bytes.Equal(a.Body, b.Body) {
		out = append(out, "body: "+bodyDiff(a.Body, b.Body))
	}
	return out
}

func bodyDiff(a, b []byte) string {
	var av, bv any
	if json.Unmarshal(a, &av) == nil && json.Unmarshal(b, &bv) == nil {
		if d := firstJSONDiff("$", av, bv); d != "" {
			return d
		}
		return "same JSON value, different bytes (key order or formatting)"
	}
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := i - 60
	if lo < 0 {
		lo = 0
	}
	return fmt.Sprintf("bytes differ at %d: rails=%q go=%q", i, snippet(a, lo, i+80), snippet(b, lo, i+80))
}

func snippet(b []byte, lo, hi int) string {
	if hi > len(b) {
		hi = len(b)
	}
	if lo > hi {
		lo = hi
	}
	return string(b[lo:hi])
}

func firstJSONDiff(path string, a, b any) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: rails object, go %T", path, b)
		}
		var keys []string
		for k := range x {
			keys = append(keys, k)
		}
		for k := range y {
			if _, ok := x[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			xv, xok := x[k]
			yv, yok := y[k]
			if !xok {
				return fmt.Sprintf("%s.%s: missing in rails (go=%s)", path, k, short(yv))
			}
			if !yok {
				return fmt.Sprintf("%s.%s: missing in go (rails=%s)", path, k, short(xv))
			}
			if d := firstJSONDiff(path+"."+k, xv, yv); d != "" {
				return d
			}
		}
		return ""
	case []any:
		y, ok := b.([]any)
		if !ok {
			return fmt.Sprintf("%s: rails array, go %T", path, b)
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			if d := firstJSONDiff(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); d != "" {
				return d
			}
		}
		if len(x) != len(y) {
			return fmt.Sprintf("%s: length rails=%d go=%d", path, len(x), len(y))
		}
		return ""
	}
	as, _ := json.Marshal(a)
	bs, _ := json.Marshal(b)
	if !bytes.Equal(as, bs) {
		return fmt.Sprintf("%s: rails=%s go=%s", path, short(a), short(b))
	}
	return ""
}

func short(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

// Token mints a Firebase ID token signed with the test key.
func Token(keyPath, projectID string, claims map[string]any) (string, error) {
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(pemBytes)
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	c := jwt.MapClaims{
		"iss": "https://securetoken.google.com/" + projectID,
		"aud": projectID,
		"iat": now - 60,
		"exp": now + 3600,
	}
	for kk, v := range claims {
		c[kk] = v
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	t.Header["kid"] = "test-kid"
	return t.SignedString(k.(*rsa.PrivateKey))
}
