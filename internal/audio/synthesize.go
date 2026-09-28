package audio

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// SupportsCustomInstructions ports supports_custom_instructions?.
func (p *Provider) SupportsCustomInstructions() bool { return p.Name == "openai" || p.Name == "google" }

// InstructionsWith ports Base#instructions_with(additional, base:): a
// per-clip note appended to the steering prompt.
func InstructionsWith(base, additional string) string {
	note := rb.Strip(additional)
	if rb.BlankString(note) {
		return base
	}
	var parts []string
	for _, s := range []string{base, "Orientação exclusiva deste áudio — não leia esta orientação nem acrescente texto: " + note} {
		if !rb.BlankString(s) {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// runtimeError is a bare `raise "message"`.
func runtimeError(message string) error {
	return &rb.RubyError{Class: "RuntimeError", Message: message}
}

// Synthesize ports Provider#synthesize(text:, path:, additional_instructions:):
// the audio bytes the provider returned for text.
func (p *Provider) Synthesize(ctx context.Context, text, additional string) ([]byte, error) {
	switch p.Name {
	case "openai":
		return p.synthesizeOpenAI(ctx, text, additional)
	case "google":
		return p.synthesizeGoogle(ctx, text, additional)
	default:
		return synthesizeElevenLabs(ctx, text, p.Voice)
	}
}

// --- transport -----------------------------------------------------------------

type response struct {
	status int
	body   []byte
	header http.Header
}

// postJSON ports `http.headers(...).post(url, json: payload)` inside
// RetryPolicy.call(max_attempts: 1): a transport failure is raised as the
// client's Unavailable class.
func postJSON(ctx context.Context, envKey string, timeout int, target string, headers map[string]string, payload *rb.Map,
	operation, class, code string) (res *response, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if e, ok := rec.(error); ok {
				err = e
				return
			}
			panic(rec)
		}
	}()
	client := integrations.Client(envKey, timeout)
	resp := integrations.Retry(ctx, operation, 1, 0, class, code, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(rb.JSON(payload)))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		return client.Do(req)
	})
	defer resp.Body.Close()
	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, web.NewInfraError(class, strings.SplitN(operation, ".", 2)[0]+" is temporarily unavailable", code)
	}
	return &response{status: resp.StatusCode, body: body, header: resp.Header}, nil
}

func infra(class, message, code string) error { return web.NewInfraError(class, message, code) }

// --- OpenAI ------------------------------------------------------------------------

func (p *Provider) synthesizeOpenAI(ctx context.Context, text, additional string) ([]byte, error) {
	payload := rb.M("model", p.Model, "voice", p.Voice, "input", text, "speed", p.Speed, "response_format", "mp3")
	if prompt := InstructionsWith(p.Instructions, additional); !rb.BlankString(prompt) {
		payload.Set("instructions", prompt)
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if rb.BlankString(apiKey) {
		return nil, runtimeError("OPENAI_API_KEY not configured")
	}
	const prefix = "Integrations::OpenAi::Client::"
	res, err := postJSON(ctx, "OPENAI_HTTP_TIMEOUT", integrations.DefaultTimeout,
		integrations.URL("OPENAI_API_URL", "https://api.openai.com")+"/v1/audio/speech",
		map[string]string{"Authorization": "Bearer " + apiKey}, payload,
		"openai.speech", prefix+"Unavailable", "OPENAI_SERVICE_UNAVAILABLE")
	if err != nil {
		return nil, err
	}
	status := strconv.Itoa(res.status)
	switch {
	case res.status == 200:
		return res.body, nil
	case res.status == 429:
		return nil, infra(prefix+"RateLimited", "OpenAI rate limit exceeded", "OPENAI_RATE_LIMITED")
	case res.status >= 500 && res.status <= 599:
		return nil, infra(prefix+"Unavailable", "OpenAI request failed with status "+status, "OPENAI_SERVICE_UNAVAILABLE")
	}
	return nil, infra(prefix+"Error", "OpenAI request failed with status "+status, "OPENAI_REQUEST_FAILED")
}

// --- ElevenLabs --------------------------------------------------------------------

var (
	elevenReferenceRe = rx.MustCompile(`\s*__\([^)]+\)__`)
	elevenBoldRe      = rx.MustCompile(`\*\*([^*]+)\*\*`)
	elevenUnderlineRe = rx.MustCompile(`__([^_]+)__`)
	elevenItalicRe    = rx.MustCompile(`\*([^*]+)\*`)
	elevenNewlinesRe  = rx.MustCompile(`\n{3,}`)
	elevenSpacesRe    = rx.MustCompile(` {2,}`)
)

// sanitizeForElevenLabs ports ElevenlabsAudioService#sanitize_text_for_audio.
func sanitizeForElevenLabs(text string) string {
	if rb.BlankString(text) {
		return ""
	}
	s := elevenReferenceRe.Gsub(text, "")
	s = elevenBoldRe.Gsub(s, `\1`)
	s = elevenUnderlineRe.Gsub(s, `\1`)
	s = elevenItalicRe.Gsub(s, `\1`)
	s = elevenNewlinesRe.Gsub(s, "\n\n")
	s = elevenSpacesRe.Gsub(s, " ")
	return rb.Strip(s)
}

// synthesizeElevenLabs ports ElevenlabsAudioService#generate_audio.
func synthesizeElevenLabs(ctx context.Context, text, voiceKey string) ([]byte, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if rb.BlankString(apiKey) {
		return nil, runtimeError("ELEVENLABS_API_KEY not configured")
	}
	var voiceID string
	for _, v := range Voices {
		if v.Key == voiceKey {
			voiceID = v.ID
		}
	}
	if voiceID == "" {
		return nil, &rb.RubyError{Class: "ArgumentError", Message: "Invalid voice_key: " + voiceKey}
	}
	payload := rb.M("text", sanitizeForElevenLabs(text), "model_id", elevenLabsModelID, "language_code", "pt",
		"voice_settings", rb.M("stability", 0.5, "similarity_boost", 0.75, "style", 0.0, "use_speaker_boost", true),
		"output_format", "mp3_44100_64")
	const prefix = "Integrations::ElevenLabs::Client::"
	res, err := postJSON(ctx, "ELEVENLABS_HTTP_TIMEOUT", integrations.DefaultTimeout,
		integrations.URL("ELEVENLABS_API_URL", "https://api.elevenlabs.io")+"/v1/text-to-speech/"+voiceID,
		map[string]string{"xi-api-key": apiKey}, payload,
		"elevenlabs.text_to_speech", prefix+"Unavailable", "ELEVENLABS_SERVICE_UNAVAILABLE")
	if err != nil {
		return nil, err
	}
	status := strconv.Itoa(res.status)
	switch {
	case res.status == 200:
		return res.body, nil
	case res.status == 429:
		return nil, infra("ElevenlabsAudioService::RateLimitError", "ElevenLabs rate limit exceeded. Please try again later.",
			"ELEVENLABS_RATE_LIMITED")
	case res.status >= 500 && res.status <= 599:
		return nil, infra(prefix+"Unavailable", "ElevenLabs request failed with status "+status, "ELEVENLABS_SERVICE_UNAVAILABLE")
	}
	return nil, infra(prefix+"Error", "ElevenLabs API error ("+status+"): request rejected", "ELEVENLABS_REQUEST_FAILED")
}

// --- Google Cloud Text-to-Speech -----------------------------------------------------

const (
	googleTTSPrefix          = "Integrations::GoogleCloud::TextToSpeech::Client::"
	googleRequestInterval    = 13 * time.Second
	googleRateLimitRetry     = 60.0
	googleRateLimitAttempts  = 3
	googleMaxRateLimitRetry  = 300.0
	googleCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
)

// googleClient ports Integrations::GoogleCloud::TextToSpeech::Client, which
// keeps one request every 13 seconds for the life of the provider.
type googleClient struct {
	projectID   string
	credentials string
	credsPath   string
	accessToken string
	lastStart   time.Time
	sleep       func(context.Context, time.Duration)
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func presentEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); !rb.BlankString(v) {
			return v
		}
	}
	return ""
}

func (p *Provider) googleTTS() *googleClient {
	if p.google != nil {
		return p.google
	}
	creds := presentEnv("GOOGLE_TTS_CREDENTIALS", "FIREBASE_CREDENTIALS")
	project := presentEnv("GOOGLE_TTS_PROJECT_ID", "GOOGLE_CLOUD_PROJECT", "GOOGLE_PROJECT_ID", "FIREBASE_PROJECT_ID")
	if project == "" && creds != "" {
		var parsed any
		if json.Unmarshal([]byte(creds), &parsed) == nil {
			if m, ok := parsed.(map[string]any); ok {
				project = rb.ToS(m["project_id"])
			}
		}
	}
	p.google = &googleClient{projectID: project, credentials: creds, credsPath: presentEnv("GOOGLE_APPLICATION_CREDENTIALS"),
		accessToken: presentEnv("GOOGLE_TTS_ACCESS_TOKEN"), sleep: sleepCtx}
	return p.google
}

func (p *Provider) synthesizeGoogle(ctx context.Context, text, additional string) ([]byte, error) {
	input := rb.M("text", text)
	payload := rb.M("input", input,
		"voice", rb.M("languageCode", p.Language, "name", p.Voice, "modelName", p.Model),
		"audioConfig", rb.M("audioEncoding", "MP3", "sampleRateHertz", 24000, "speakingRate", p.Speed))
	base := p.Instructions
	if p.shortInput(text) {
		base = p.shortInstructions
	}
	if prompt := InstructionsWith(base, additional); !rb.BlankString(prompt) {
		input.Set("prompt", prompt)
	}
	return p.googleTTS().speech(ctx, payload)
}

func (g *googleClient) speech(ctx context.Context, payload *rb.Map) ([]byte, error) {
	if rb.BlankString(g.projectID) {
		return nil, infra(googleTTSPrefix+"Error", "Google TTS project is not configured", "GOOGLE_TTS_PROJECT_MISSING")
	}
	for attempts := 1; ; attempts++ {
		res, err := g.post(ctx, payload)
		if err != nil {
			return nil, err
		}
		if res.status != 429 || attempts >= googleRateLimitAttempts {
			return g.interpret(res)
		}
		delay := googleRateLimitDelay(res, attempts)
		if delay > 0 {
			g.sleep(ctx, time.Duration(delay*float64(time.Second)))
		}
	}
}

// googleRateLimitDelay ports rate_limit_delay.
func googleRateLimitDelay(res *response, attempts int) float64 {
	if ra := res.header.Get("Retry-After"); !rb.BlankString(ra) {
		f, ok := rubyFloat(ra)
		if !ok {
			return googleRateLimitRetry
		}
		return min(max(f, 0), googleMaxRateLimitRetry)
	}
	return min(googleRateLimitRetry*float64(attempts), googleMaxRateLimitRetry)
}

func (g *googleClient) post(ctx context.Context, payload *rb.Map) (*response, error) {
	if !g.lastStart.IsZero() {
		if remaining := googleRequestInterval - time.Since(g.lastStart); remaining > 0 {
			g.sleep(ctx, remaining)
		}
	}
	g.lastStart = time.Now()
	token, err := g.token(ctx)
	if err != nil {
		return nil, err
	}
	return postJSON(ctx, "GOOGLE_TTS_HTTP_TIMEOUT", 120,
		integrations.URL("GOOGLE_TTS_API_URL", "https://texttospeech.googleapis.com")+"/v1/text:synthesize",
		map[string]string{"Authorization": "Bearer " + token, "x-goog-user-project": g.projectID}, payload,
		"google_tts.synthesize", googleTTSPrefix+"Unavailable", "GOOGLE_TTS_SERVICE_UNAVAILABLE")
}

func (g *googleClient) interpret(res *response) ([]byte, error) {
	status := strconv.Itoa(res.status)
	switch {
	case res.status == 200:
		return decodeGoogleAudio(res.body)
	case res.status == 429:
		return nil, infra(googleTTSPrefix+"RateLimited", "Google TTS rate limit exceeded", "GOOGLE_TTS_RATE_LIMITED")
	case res.status == 408 || res.status >= 500 && res.status <= 599:
		return nil, infra(googleTTSPrefix+"Unavailable", "Google TTS request failed with status "+status, "GOOGLE_TTS_SERVICE_UNAVAILABLE")
	}
	return nil, infra(googleTTSPrefix+"Error", "Google TTS request failed with status "+status, "GOOGLE_TTS_REQUEST_FAILED")
}

// decodeGoogleAudio ports decode_audio: JSON.parse(body).fetch("audioContent")
// through Base64.strict_decode64.
func decodeGoogleAudio(body []byte) ([]byte, error) {
	invalid := infra(googleTTSPrefix+"Error", "Google TTS returned an invalid audio response", "GOOGLE_TTS_INVALID_RESPONSE")
	parsed, err := rb.ParseJSON(body)
	if err != nil {
		return nil, invalid
	}
	switch v := parsed.(type) {
	case *rb.Map:
		if !v.Has("audioContent") {
			return nil, invalid
		}
		s, ok := v.Get("audioContent").(string)
		if !ok {
			return nil, &rb.RubyError{Class: "TypeError", Message: rb.ImplicitConversionMessage(v.Get("audioContent"), "String")}
		}
		out, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil {
			return nil, invalid
		}
		return out, nil
	case []any:
		return nil, &rb.RubyError{Class: "TypeError", Message: "no implicit conversion of String into Integer"}
	}
	return nil, &rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("fetch", parsed)}
}

// token ports access_token: GOOGLE_TTS_ACCESS_TOKEN, or a fresh exchange with
// the service account (or application default credentials).
func (g *googleClient) token(ctx context.Context) (string, error) {
	if !rb.BlankString(g.accessToken) {
		return g.accessToken, nil
	}
	creds := g.credentials
	if rb.BlankString(creds) {
		path := g.credsPath
		if path != "" {
			if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
				return "", infra(googleTTSPrefix+"Error", "Google TTS credentials file not found", "GOOGLE_TTS_CREDENTIALS_MISSING")
			}
		} else if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", infra(googleTTSPrefix+"Error", "Google TTS credentials are invalid or unavailable", "GOOGLE_TTS_CREDENTIALS_INVALID")
		}
		creds = string(b)
	}
	var key struct {
		Type         string `json:"type"`
		ClientEmail  string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(creds), &key) != nil {
		return "", infra(googleTTSPrefix+"Error", "Google TTS credentials are invalid or unavailable", "GOOGLE_TTS_CREDENTIALS_INVALID")
	}
	var form url.Values
	if key.Type == "authorized_user" {
		form = url.Values{"grant_type": {"refresh_token"}, "client_id": {key.ClientID}, "client_secret": {key.ClientSecret},
			"refresh_token": {key.RefreshToken}}
	} else {
		assertion, ok := serviceAccountAssertion(key.ClientEmail, key.PrivateKey, googleCloudPlatformScope)
		if !ok {
			return "", infra(googleTTSPrefix+"Error", "Google TTS credentials are invalid or unavailable", "GOOGLE_TTS_CREDENTIALS_INVALID")
		}
		form = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	}
	failed := infra(googleTTSPrefix+"Error", "Google TTS authentication failed", "GOOGLE_TTS_AUTH_FAILED")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", failed
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", failed
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode/100 != 2 || json.Unmarshal(raw, &body) != nil || body.AccessToken == "" {
		return "", failed
	}
	return body.AccessToken, nil
}

// googleTokenURL is googleauth's token_credential_uri.
func googleTokenURL() string {
	return config.PresenceOr("FIREBASE_OAUTH_TOKEN_URL", "https://oauth2.googleapis.com/token")
}

// serviceAccountAssertion signs the JWT bearer grant of a service account.
func serviceAccountAssertion(email, privateKey, scope string) (string, bool) {
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil || email == "" {
		return "", false
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		k1, err1 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err1 != nil {
			return "", false
		}
		key = k1
	}
	now := time.Now()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": email, "scope": scope, "aud": "https://oauth2.googleapis.com/token",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(key)
	return signed, err == nil
}
