// Package auth ports Authenticatable, ApiKeyAuthenticatable,
// DeveloperAuthenticatable and the Firebase token services.
package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// GoogleCertsURL is where Firebase publishes its token signing certificates.
const GoogleCertsURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

// ErrInvalidToken mirrors FirebaseAuthService::InvalidTokenError.
var ErrInvalidToken = errors.New("invalid firebase token")

func certsURL() string {
	return config.PresenceOr("FIREBASE_CERTS_URL", GoogleCertsURL)
}

type certSet struct {
	fetched time.Time
	certs   map[string]string
}

// certCache holds certificates like the Rails cache entry (1h) and its
// stale copy (24h).
type certCache struct {
	mu    sync.Mutex
	fresh *certSet
	keys  map[string]*rsa.PublicKey
}

var appCerts = &certCache{keys: map[string]*rsa.PublicKey{}}
var developerCerts = &certCache{keys: map[string]*rsa.PublicKey{}}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func fetchCerts() (map[string]string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := httpClient.Get(certsURL())
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}
		var certs map[string]string
		if err := json.Unmarshal(body, &certs); err != nil {
			return nil, err
		}
		return certs, nil
	}
	return nil, lastErr
}

// certificates returns the current certificate set; stale (<24h) copies are
// served when a refresh fails, otherwise the failure is an
// ExternalServiceUnavailable (503), as in FirebaseAuthService.
func (c *certCache) certificates(strict bool) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fresh != nil && time.Since(c.fresh.fetched) < time.Hour {
		return c.fresh.certs, nil
	}
	certs, err := fetchCerts()
	if err != nil {
		if c.fresh != nil && time.Since(c.fresh.fetched) < 24*time.Hour {
			return c.fresh.certs, nil
		}
		if !strict {
			return map[string]string{}, nil
		}
		return nil, web.NewInfraError("ExternalServiceUnavailable", "Firebase certificates are temporarily unavailable", "FIREBASE_CERTIFICATES_UNAVAILABLE")
	}
	c.fresh = &certSet{fetched: time.Now(), certs: certs}
	c.keys = map[string]*rsa.PublicKey{}
	return certs, nil
}

func (c *certCache) key(kid string, strict bool) (*rsa.PublicKey, error) {
	certs, err := c.certificates(strict)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	pemStr, ok := certs[kid]
	if !ok {
		return nil, nil
	}
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("bad certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	pk, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	c.keys[kid] = pk
	return pk, nil
}

// VerifyToken ports FirebaseAuthService.verify_token (and the developer
// variant when developer is true). It returns the claims, ErrInvalidToken,
// or an *web.InfraError when certificates cannot be fetched.
func VerifyToken(token string, developer bool) (jwt.MapClaims, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrInvalidToken
	}
	projectKey := "FIREBASE_PROJECT_ID"
	cache := appCerts
	if developer {
		projectKey = "DEVELOPER_FIREBASE_PROJECT_ID"
		cache = developerCerts
	}
	projectID := config.Get(projectKey)
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrInvalidToken
	}
	parser := jwt.NewParser()
	unverified, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, ErrInvalidToken
	}
	kid, _ := unverified.Header["kid"].(string)
	key, err := cache.key(kid, !developer)
	if err != nil {
		var infra *web.InfraError
		if errors.As(err, &infra) {
			return nil, infra
		}
		return nil, ErrInvalidToken
	}
	if key == nil {
		return nil, ErrInvalidToken
	}
	claims := jwt.MapClaims{}
	verified, err := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuedAt(),
		jwt.WithIssuer("https://securetoken.google.com/"+projectID),
		jwt.WithAudience(projectID),
	).ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return key, nil })
	if err != nil || !verified.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ClaimString returns a string claim or "".
func ClaimString(c jwt.MapClaims, key string) string {
	s, _ := c[key].(string)
	return s
}

// ctxKey avoids collisions in context values.
type ctxKey string

var _ = context.Background
