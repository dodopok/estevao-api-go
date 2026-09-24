// Package activestorage ports the parts of Active Storage the API exposes:
// blob signed ids and URLs, the blob redirect/proxy endpoints, and avatar
// attach/purge against the configured S3 service.
package activestorage

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

var (
	secretOnce sync.Once
	secret     []byte
	sgidOnce   sync.Once
	sgidSecret []byte
)

// verifierSecret ports Rails.application.message_verifier("ActiveStorage"):
// key_generator.generate_key("ActiveStorage") — PBKDF2-HMAC-SHA256 of
// secret_key_base, 1000 iterations, 64 bytes.
func verifierSecret() []byte {
	secretOnce.Do(func() {
		secret = pbkdf2.Key([]byte(config.Get("SECRET_KEY_BASE")), []byte("ActiveStorage"), 1000, 64, sha256.New)
	})
	return secret
}

func digest(data string) string { return digestWith(verifierSecret(), data) }

func digestWith(key []byte, data string) string {
	h := hmac.New(sha1.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// Generate ports ActiveStorage.verifier.generate(id, purpose:) with the
// JSON message serializer and metadata inside the envelope.
func Generate(id int64, purpose string) string { return GenerateData(id, purpose, nil) }

// GenerateData ports ActiveStorage.verifier.generate(data, purpose:,
// expires_at:). data is any rb value (a *rb.Map keeps Ruby's key order).
func GenerateData(data any, purpose string, expiresAt *time.Time) string {
	meta := rb.M("data", data)
	if expiresAt != nil {
		meta.Set("exp", expiresAt.UTC().Format("2006-01-02T15:04:05.000Z"))
	}
	meta.Set("pur", purpose)
	encoded := base64.StdEncoding.EncodeToString(rb.JSON(rb.M("_rails", meta)))
	return encoded + "--" + digest(encoded)
}

// Verified ports verified(message, purpose:) for an Integer payload.
func Verified(message, purpose string, now time.Time) (int64, bool) {
	v, ok := VerifiedData(message, purpose, now)
	if !ok {
		return 0, false
	}
	return intValue(v)
}

// VerifiedData ports ActiveStorage.verifier.verified(message, purpose:). It
// accepts the current JSON envelope, the Rails 5.2-7.0 envelope
// ({"_rails":{"message":base64(serialized)}}) and a Marshal-serialized
// envelope, as the json_allow_marshal serializer does. JSON objects come
// back as *rb.Map, Marshal hashes as map[string]any.
func VerifiedData(message, purpose string, now time.Time) (any, bool) {
	i := strings.LastIndex(message, "--")
	if i <= 0 {
		return nil, false
	}
	data, sig := message[:i], message[i+2:]
	if subtle.ConstantTimeCompare([]byte(sig), []byte(digest(data))) != 1 {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, false
	}
	env, ok := deserialize(raw)
	if !ok {
		return nil, false
	}
	meta, ok := field(env, "_rails")
	if !ok {
		return nil, false
	}
	if p, _ := fieldValue(meta, "pur").(string); p != purpose {
		return nil, false
	}
	if exp, ok := fieldValue(meta, "exp").(string); ok && exp != "" {
		t, err := time.Parse(time.RFC3339Nano, exp)
		if err != nil || !now.Before(t) {
			return nil, false
		}
	}
	if d, ok := lookup(meta, "data"); ok {
		return d, true
	}
	if m, ok := fieldValue(meta, "message").(string); ok {
		b, err := base64.StdEncoding.DecodeString(m)
		if err != nil {
			return nil, false
		}
		return deserialize(b)
	}
	return nil, false
}

func deserialize(b []byte) (any, bool) {
	if strings.HasPrefix(string(b), "\x04\x08") {
		return unmarshalRuby(b)
	}
	v, err := rb.ParseJSON(b)
	return v, err == nil
}

func lookup(v any, key string) (any, bool) {
	switch m := v.(type) {
	case *rb.Map:
		return m.Lookup(key)
	case map[string]any:
		x, ok := m[key]
		return x, ok
	}
	return nil, false
}

func fieldValue(v any, key string) any { x, _ := lookup(v, key); return x }

func field(v any, key string) (any, bool) {
	x, ok := lookup(v, key)
	if !ok {
		return nil, false
	}
	switch x.(type) {
	case *rb.Map, map[string]any:
		return x, true
	}
	return nil, false
}

func intValue(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	}
	return 0, false
}

// AttachableSGID ports blob.to_signed_global_id(for: "attachable",
// expires_in: nil) as Blob#as_json renders it (attachable_sgid): the
// signed_global_ids verifier over "gid://estevao-api/ActiveStorage::Blob/ID"
// (with the "?expires_in" suffix GlobalID leaves for a nil expiry).
func AttachableSGID(id int64) string {
	sgidOnce.Do(func() {
		sgidSecret = pbkdf2.Key([]byte(config.Get("SECRET_KEY_BASE")), []byte("signed_global_ids"), 1000, 64, sha256.New)
	})
	body := `{"_rails":{"data":"gid://estevao-api/ActiveStorage::Blob/` + itoa(id) + `?expires_in","pur":"attachable"}}`
	data := base64.StdEncoding.EncodeToString([]byte(body))
	return data + "--" + digestWith(sgidSecret, data)
}
