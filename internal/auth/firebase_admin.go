package auth

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dodopok/estevao-api-go/internal/config"
)

// FirebaseDeleteResult ports the hash FirebaseAuthService.delete_user returns.
type FirebaseDeleteResult struct {
	Success   bool
	ErrorCode string
}

// Test-only overrides for the Google endpoints (the Rails app hard-codes
// them); unset in production.
func oauthTokenURL() string {
	return config.PresenceOr("FIREBASE_OAUTH_TOKEN_URL", "https://oauth2.googleapis.com/token")
}

func identityToolkitURL() string {
	return config.PresenceOr("FIREBASE_IDENTITY_TOOLKIT_URL", "https://identitytoolkit.googleapis.com")
}

// outboundTimeout ports Integrations::HttpClient.timeout_seconds.
func outboundTimeout(envKey string) time.Duration {
	n := 30
	if v := strings.TrimSpace(config.Get(envKey)); v != "" {
		if parsed, ok := parseRubyInteger(v); ok {
			n = parsed
		}
	}
	return time.Duration(min(max(n, 1), 120)) * time.Second
}

func parseRubyInteger(s string) (int, bool) {
	neg := false
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

// adminAccessToken ports fetch_admin_access_token: a service-account JWT
// exchanged for an OAuth2 access token with the identitytoolkit scope.
func adminAccessToken(ctx context.Context) (string, error) {
	email, key := config.Get("FIREBASE_CLIENT_EMAIL"), config.Get("FIREBASE_PRIVATE_KEY")
	if strings.TrimSpace(email) == "" || strings.TrimSpace(key) == "" {
		return "", errors.New("Firebase service account credentials not configured")
	}
	block, _ := pem.Decode([]byte(strings.ReplaceAll(key, `\n`, "\n")))
	if block == nil {
		return "", errors.New("invalid private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if rsaKey, err2 := x509.ParsePKCS1PrivateKey(block.Bytes); err2 == nil {
			parsed = rsaKey
		} else {
			return "", err
		}
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": email, "scope": "https://www.googleapis.com/auth/identitytoolkit", "aud": oauthTokenURL(),
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(parsed)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode/100 != 2 || body.AccessToken == "" {
		return "", errors.New("token exchange failed")
	}
	return body.AccessToken, nil
}

// DeleteFirebaseUser ports FirebaseAuthService.delete_user for a present uid.
func DeleteFirebaseUser(ctx context.Context, uid string) FirebaseDeleteResult {
	unavailable := FirebaseDeleteResult{ErrorCode: "FIREBASE_ACCOUNT_DELETE_UNAVAILABLE"}
	token, err := adminAccessToken(ctx)
	if err != nil {
		return unavailable
	}
	payload, _ := json.Marshal(map[string]string{"localId": uid})
	endpoint := identityToolkitURL() + "/v1/projects/" + config.Get("FIREBASE_PROJECT_ID") + "/accounts:delete"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return unavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: outboundTimeout("FIREBASE_HTTP_TIMEOUT")}).Do(req)
	if err != nil {
		return unavailable
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 == 2 {
		return FirebaseDeleteResult{Success: true}
	}
	return FirebaseDeleteResult{ErrorCode: "FIREBASE_ACCOUNT_DELETE_REJECTED"}
}
