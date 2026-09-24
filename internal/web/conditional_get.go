package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// WeakETag ports ActionDispatch's generate_weak_etag for a single string
// validator: W/"sha256(expand_cache_key([validator]))[0,32]" (Rails sets
// hash_digest_class to SHA256 since load_defaults 7.0).
func WeakETag(validator string) string {
	key := ""
	if prefix := firstEnv("RAILS_CACHE_ID", "RAILS_APP_VERSION"); prefix != "" {
		key = prefix + "/"
	}
	key += validator
	sum := sha256.Sum256([]byte(key))
	return `W/"` + hex.EncodeToString(sum[:])[:32] + `"`
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v, ok := os.LookupEnv(n); ok {
			return v
		}
	}
	return ""
}

// ExpiresIn ports ActionController::ConditionalGet#expires_in(seconds,
// public:), which also sets the Date header.
func (c *Context) ExpiresIn(seconds int, public bool) {
	vis := "private"
	if public {
		vis = "public"
	}
	c.Header.Set("Cache-Control", "max-age="+strconv.Itoa(seconds)+", "+vis)
	if c.Header.Get("Date") == "" {
		c.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
}

// Stale ports stale?(etag:): sets the weak ETag and answers 304 when the
// request's If-None-Match already names it (Rails renders head :not_modified).
func (c *Context) Stale(validator string) bool {
	etag := WeakETag(validator)
	c.Header.Set("Etag", etag)
	inm := c.R.Header.Get("If-None-Match")
	if inm == "" {
		return true
	}
	for _, v := range strings.Split(inm, ",") {
		v = strings.TrimSpace(v)
		if v == etag || v == "*" {
			c.HeadStatus(304)
			return false
		}
	}
	return true
}
