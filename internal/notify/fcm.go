// Package notify ports push notifications: FcmService (FCM HTTP v1 with a
// service-account token), NotificationService and NotificationLog.
package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// googleTokenURI is googleauth's token_credential_uri (and JWT audience).
const googleTokenURI = "https://oauth2.googleapis.com/token"

// credentialsJSON ports FcmService.credentials_json.
func credentialsJSON() string {
	if v := config.Get("FIREBASE_CREDENTIALS"); strings.TrimSpace(v) != "" {
		return v
	}
	if p := config.Get("GOOGLE_APPLICATION_CREDENTIALS"); strings.TrimSpace(p) != "" {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return ""
}

// projectID ports FcmService.project_id.
func projectID() string {
	if v := config.Get("FIREBASE_PROJECT_ID"); strings.TrimSpace(v) != "" {
		return v
	}
	creds := credentialsJSON()
	if strings.TrimSpace(creds) == "" {
		return ""
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(creds), &parsed) != nil {
		return ""
	}
	s, _ := parsed["project_id"].(string)
	return s
}

// Configured ports FcmService.configured?.
func Configured() bool {
	return strings.TrimSpace(credentialsJSON()) != "" && strings.TrimSpace(projectID()) != ""
}

// accessToken ports ServiceAccountCredentials#fetch_access_token! with the
// firebase.messaging scope (a fresh exchange on every call, as FcmService
// makes one).
func accessToken(ctx context.Context) string {
	var creds struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON()), &creds); err != nil {
		panic(&web.StandardError{Class: "JSON::ParserError", Message: err.Error()})
	}
	block, _ := pem.Decode([]byte(creds.PrivateKey))
	if block == nil {
		panic(&web.StandardError{Class: "OpenSSL::PKey::RSAError", Message: "Neither PUB key nor PRIV key"})
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		k1, err1 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err1 != nil {
			panic(&web.StandardError{Class: "OpenSSL::PKey::RSAError", Message: "Neither PUB key nor PRIV key"})
		}
		key = k1
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": creds.ClientEmail, "scope": "https://www.googleapis.com/auth/firebase.messaging", "aud": googleTokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		panic(&web.StandardError{Class: "Signet::AuthorizationError", Message: err.Error()})
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, config.PresenceOr("FIREBASE_OAUTH_TOKEN_URL", googleTokenURI),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		panic(&web.StandardError{Class: "Signet::AuthorizationError", Message: err.Error()})
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode/100 != 2 || json.Unmarshal(raw, &body) != nil || body.AccessToken == "" {
		panic(&web.StandardError{Class: "Signet::AuthorizationError",
			Message: "Authorization failed.  Server message:\n" + string(raw)})
	}
	return body.AccessToken
}

// Notification is the hash NotificationService builds. Params marks data
// that is still ActionController::Parameters (a request's permitted data),
// whose nested hashes print differently from a job's plain hashes.
type Notification struct {
	Title  any
	Body   any
	Data   *rb.Map
	Params bool
}

// dataToS ports `(data || {}).transform_values(&:to_s)`.
func dataToS(data *rb.Map, params bool) *rb.Map {
	out := rb.NewMap()
	if data == nil {
		return out
	}
	data.Each(func(k string, v any) {
		if params {
			out.Set(k, paramsToS(v))
		} else {
			out.Set(k, rb.ToS(v))
		}
	})
	return out
}

// paramsToS ports #to_s on a permitted params value: a Parameters prints its
// hash, in which every nested Parameters prints as
// #<ActionController::Parameters {...} permitted: true>.
func paramsToS(v any) string {
	switch x := v.(type) {
	case *rb.Map:
		return paramsHashInspect(x)
	case []any:
		return paramsInspect(x)
	}
	return rb.ToS(v)
}

func paramsHashInspect(m *rb.Map) string {
	var parts []string
	m.Each(func(k string, v any) { parts = append(parts, rb.Inspect(k)+"=>"+paramsInspect(v)) })
	return "{" + strings.Join(parts, ", ") + "}"
}

func paramsInspect(v any) string {
	switch x := v.(type) {
	case *rb.Map:
		return "#<ActionController::Parameters " + paramsHashInspect(x) + " permitted: true>"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = paramsInspect(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return rb.Inspect(v)
}

// message ports FcmService.build_message.
func message(token string, n *Notification) *rb.Map {
	return rb.M("token", token,
		"notification", rb.M("title", n.Title, "body", n.Body),
		"data", dataToS(n.Data, n.Params),
		"android", rb.M("priority", "high", "notification", rb.M("channel_id", "ordo_channel", "sound", "default",
			"default_vibrate_timings", true, "default_light_settings", true)),
		"apns", rb.M("headers", rb.M("apns-priority", "10", "apns-push-type", "alert"),
			"payload", rb.M("aps", rb.M("alert", rb.M("title", n.Title, "body", n.Body), "sound", "default", "badge", 1,
				"mutable-content", 1))))
}

func fcmUnavailable(message, code string) *web.InfraError {
	return &web.InfraError{Class: "ExternalServiceUnavailable", Code: code, Message: message}
}

// sendMessage ports Integrations::Fcm::Client#send_message.
func sendMessage(ctx context.Context, project, token string, msg *rb.Map) (int, []byte) {
	endpoint := integrations.URL("FCM_API_URL", "https://fcm.googleapis.com") + "/v1/projects/" + project + "/messages:send"
	body := []byte(rb.ToJSON(rb.M("message", msg)))
	client := integrations.Client("FCM_HTTP_TIMEOUT", 30)
	resp := integrations.Retry(ctx, "fcm.send_message", 1, 0, "ExternalServiceUnavailable", "FCM_UNAVAILABLE",
		func() (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			return client.Do(req)
		})
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	status := resp.StatusCode
	invalid := status == 404
	if !invalid {
		if parsed, err := rb.ParseJSON(raw); err == nil {
			if m, ok := parsed.(*rb.Map); ok {
				if e, ok := m.Get("error").(*rb.Map); ok {
					if details, ok := e.Get("details").([]any); ok {
						for _, d := range details {
							if dm, ok := d.(*rb.Map); ok && dm.Get("errorCode") == "UNREGISTERED" {
								invalid = true
							}
						}
					}
				}
			}
		}
	}
	if invalid {
		panic(fcmUnavailable("FCM registration token is no longer valid", "FCM_INVALID_TOKEN"))
	}
	if status == 429 || (status >= 500 && status <= 599) {
		panic(fcmUnavailable("FCM is temporarily unavailable", "FCM_UNAVAILABLE"))
	}
	return status, raw
}

type tokenResult struct {
	Success bool
	Token   string
	Error   any
	Invalid bool
}

func isUnavailable(rec any) (*web.InfraError, bool) {
	e, ok := rec.(*web.InfraError)
	return e, ok && e.Class == "ExternalServiceUnavailable"
}

func rescuedMessage(rec any) string {
	switch e := rec.(type) {
	case error:
		return e.Error()
	}
	return fmt.Sprint(rec)
}

// sendToToken ports FcmService.send_to_token.
func sendToToken(ctx context.Context, token string, n *Notification) (res tokenResult) {
	defer func() {
		if rec := recover(); rec != nil {
			if e, ok := isUnavailable(rec); ok {
				if e.Code == "FCM_INVALID_TOKEN" {
					res = tokenResult{Token: token, Error: e.Message, Invalid: true}
					return
				}
				panic(rec)
			}
			res = tokenResult{Token: token, Error: rescuedMessage(rec)}
		}
	}()
	msg := message(token, n)
	status, raw := sendMessage(ctx, projectID(), accessToken(ctx), msg)
	if status == 200 {
		return tokenResult{Success: true, Token: token}
	}
	var errorBody any = rb.NewMap()
	if parsed, err := rb.ParseJSON(raw); err == nil {
		errorBody = parsed
	}
	var errorMsg any
	switch b := errorBody.(type) {
	case *rb.Map:
		if e, ok := b.Get("error").(*rb.Map); ok {
			errorMsg = e.Get("message")
		} else if e := b.Get("error"); e != nil {
			panic(&web.StandardError{Class: "TypeError", Message: rb.NoMethodErrorMessage("dig", e)})
		}
	case []any:
		panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of String into Integer"})
	default:
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("dig", b)})
	}
	if errorMsg == nil || errorMsg == false {
		errorMsg = string(raw)
	}
	s, ok := errorMsg.(string)
	if !ok {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("include?", errorMsg)})
	}
	invalid := strings.Contains(s, "not a valid FCM registration token") || status == 404
	return tokenResult{Token: token, Error: s, Invalid: invalid}
}

// Response is the hash FcmService.send_to_user returns.
type Response struct {
	Success bool
	Error   any
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// sendToUser ports FcmService.send_to_user (log is the delivery log of an
// idempotent delivery, or nil).
func sendToUser(ctx context.Context, u *users.User, n *Notification, log *Log) (resp Response) {
	defer func() {
		if rec := recover(); rec != nil {
			if _, ok := isUnavailable(rec); ok {
				panic(rec)
			}
			resp = Response{Error: rescuedMessage(rec)}
		}
	}()
	if !Configured() {
		return Response{Error: "FCM not configured. Set FIREBASE_CREDENTIALS or GOOGLE_APPLICATION_CREDENTIALS"}
	}
	rows, err := db.Q().Query(ctx, `SELECT token FROM fcm_tokens WHERE user_id = $1 AND (updated_at > $2)`, u.ID,
		time.Now().Add(-60*24*time.Hour))
	must(err)
	var tokens []string
	for rows.Next() {
		var t string
		must(rows.Scan(&t))
		tokens = append(tokens, t)
	}
	rows.Close()
	must(rows.Err())
	if len(tokens) == 0 {
		return Response{Error: "No active tokens"}
	}
	status := map[string]any{}
	if log != nil {
		status = log.deliveryStatus()
	}
	previouslySent := 0
	var pending []string
	for _, t := range tokens {
		s := status[tokenKey(t)]
		if s == "sent" {
			previouslySent++
		}
		if s != "sent" && s != "invalid" {
			pending = append(pending, t)
		}
	}
	if len(pending) == 0 {
		if previouslySent > 0 {
			return Response{Success: true}
		}
		return Response{Error: "All tokens failed"}
	}
	var results []tokenResult
	for _, t := range pending {
		r := sendToToken(ctx, t, n)
		if log != nil {
			log.persistDeliveryStatus(ctx, t, r)
		}
		results = append(results, r)
	}
	var invalid []string
	for _, r := range results {
		if r.Invalid {
			invalid = append(invalid, r.Token)
		}
	}
	if len(invalid) > 0 {
		_, err := db.Q().Exec(ctx, `DELETE FROM fcm_tokens WHERE id IN (SELECT id FROM fcm_tokens WHERE user_id = $1 AND token = ANY($2))`,
			u.ID, invalid)
		must(err)
	}
	success := previouslySent
	for _, r := range results {
		if r.Success {
			success++
		}
	}
	if success > 0 {
		return Response{Success: true}
	}
	var e any = results[0].Error
	if e == nil {
		e = "All tokens failed"
	}
	return Response{Error: e}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var errNotUnique = errors.New("RecordNotUnique")
