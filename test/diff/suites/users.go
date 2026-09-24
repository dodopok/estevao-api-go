package suites

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// userFixture recreates the users the users scenarios act as, at fixed ids
// so every id in a response is the same on both sides.
const userFixture = `
DELETE FROM active_storage_attachments WHERE record_type = 'User' AND record_id BETWEEN 990001 AND 990099;
DELETE FROM active_storage_blobs b WHERE NOT EXISTS (SELECT 1 FROM active_storage_attachments a WHERE a.blob_id = b.id) AND b.id NOT BETWEEN 900001 AND 900099;
DELETE FROM completions WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM fcm_tokens WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM user_onboardings WHERE user_id BETWEEN 990001 AND 990099 OR id = 990002;
DELETE FROM feature_flags WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM user_audio_usages WHERE user_id BETWEEN 990001 AND 990099;
DELETE FROM users WHERE id BETWEEN 990001 AND 990099;
INSERT INTO users (id, provider_uid, email, name, photo_url, preferences, premium_expires_at, current_streak, longest_streak,
  last_completed_office_at, timezone, country_code, created_at, updated_at) VALUES
 (990001, 'difftest-us-plain', 'us-plain@example.com', 'Plain', 'https://example.com/p.png',
  '{"prayer_book_code":"loc_2015","bible_version":"nvi","language":"pt-BR","version":"loc_2015","notifications":true,"preferred_audio_voice":"male_1","lords_prayer_version":"traditional"}',
  NULL, 4, 9, '2026-04-05 12:00:00', 'America/Sao_Paulo', 'BR', '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
 (990002, 'difftest-us-onb', 'us-onb@example.com', 'Onboarded', NULL,
  '{"prayer_book_code":"loc_2015","bible_version":"nvi","language":"pt-BR","mode":"basic","creed_type":"apostles"}',
  '2099-01-01', 0, 0, NULL, 'Brasilia', NULL, '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
 (990003, 'difftest-us-badvoice', 'us-badvoice@example.com', NULL, NULL,
  '{"prayer_book_code":"loc_2015","preferred_audio_voice":"robot"}', NULL, 0, 0, NULL, 'UTC', NULL, '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
 (990004, 'difftest-us-new', 'us-new@example.com', 'New', NULL, '{}', NULL, 0, 0, NULL, 'America/Sao_Paulo', NULL, '2026-01-01 00:00:00', '2026-01-01 00:00:00');
INSERT INTO user_onboardings (id, user_id, prayer_book_id, bible_version_id, mode, onboarding_completed, completed_at, preferences, created_at, updated_at)
SELECT 990002, 990002, pb.id, bv.id, 'basic', TRUE, '2026-01-02 00:00:00', '{}', '2026-01-02 00:00:00', '2026-01-02 00:00:00'
FROM prayer_books pb, bible_versions bv WHERE pb.code = 'loc_2015' AND bv.code = 'nvi';
DELETE FROM completions WHERE id BETWEEN 990001 AND 990099;
INSERT INTO completions (id, user_id, date_reference, office_type, duration_seconds, prayer_book_id, created_at, updated_at)
SELECT 990000 + row_number() OVER (ORDER BY d, o), 990001, d::date, o, 300, (SELECT id FROM prayer_books WHERE code = 'loc_2015'), d + interval '8 hours', d + interval '8 hours'
FROM generate_series('2026-03-01'::timestamp, '2026-03-06'::timestamp, interval '1 day') d, unnest(ARRAY['morning','evening']) o;
DELETE FROM fcm_tokens WHERE id = 990001;
INSERT INTO fcm_tokens (id, user_id, token, platform, created_at, updated_at) VALUES (990001, 990001, 'tok-existing', 'android', '2026-01-01', '2026-01-01');
`

var userSnapshots = []string{
	`SELECT id, email, name, photo_url, preferences::text, timezone, country_code, current_streak, longest_streak,
	   updated_at > '2026-01-01' FROM users WHERE id BETWEEN 990001 AND 990099 ORDER BY id`,
	`SELECT user_id, prayer_book_id, bible_version_id, mode, preferences::text, onboarding_completed, completed_at IS NOT NULL
	   FROM user_onboardings WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id`,
	`SELECT user_id, token, platform, updated_at > '2026-01-01' FROM fcm_tokens WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, token`,
	`SELECT a.record_id, a.name, b.filename, b.content_type, b.byte_size, b.checksum, b.service_name,
	   b.metadata::jsonb ->> 'identified' FROM active_storage_attachments a JOIN active_storage_blobs b ON b.id = a.blob_id
	   WHERE a.record_type = 'User' AND a.record_id BETWEEN 990001 AND 990099 ORDER BY a.record_id, b.id`,
	`SELECT count(*) FROM active_storage_blobs b WHERE NOT EXISTS (SELECT 1 FROM active_storage_attachments a WHERE a.blob_id = b.id)
	   AND b.id NOT BETWEEN 900001 AND 900099`,
}

func userHeaders(uid, email string, extra ...string) map[string]string {
	h := withHeaders(AppHeaders(), map[string]string{"Authorization": "Bearer " + TokenFor(uid, email), "Host": "api.example.test"})
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	return h
}

func jsonReq(method, path string, h map[string]string, body string, volatile ...string) diff.Request {
	return diff.Request{Method: method, Path: path, Headers: withHeaders(h, map[string]string{"Content-Type": "application/json"}), Body: body, Volatile: volatile}
}

// multipartBody builds a multipart/form-data body; files are
// {field, filename, content type ("" for none), bytes}.
func multipartBody(fields map[string]string, files ...[4]any) (string, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary("difftestboundary0123456789")
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f[0], f[1]))
		if ct := f[2].(string); ct != "" {
			h.Set("Content-Type", ct)
		}
		p, _ := w.CreatePart(h)
		_, _ = p.Write(f[3].([]byte))
	}
	w.Close()
	return buf.String(), w.FormDataContentType()
}

func testPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.RGBA{200, 10, 10, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func testJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 5, 4))
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
	return b.Bytes()
}

// resetFakeS3 empties the fake S3 and puts the fixture objects back.
func resetFakeS3() error {
	endpoint := envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000")
	req, _ := http.NewRequest(http.MethodDelete, endpoint+"/__objects", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	putFixtureObjects()
	return nil
}

// settleJobs performs, on the oracle, the jobs it enqueued (it runs no
// worker); the Go server runs them in-process, so it only needs a moment.
func settleJobs(classes ...string) func(side diff.Side) error {
	return func(side diff.Side) error {
		if side.Name != "rails" {
			time.Sleep(1500 * time.Millisecond)
			return nil
		}
		runner := os.Getenv("RAILS_RUNNER")
		if runner == "" {
			return fmt.Errorf("RAILS_RUNNER is required to perform the oracle's jobs")
		}
		_, file, _, _ := runtime.Caller(0)
		script := filepath.Join(filepath.Dir(file), "..", "..", "effects", "perform_jobs.rb")
		out, err := exec.Command(runner, append([]string{script}, classes...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	}
}

func init() {
	registerScenarios("users", func() []diff.Scenario {
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		onb := userHeaders("difftest-us-onb", "us-onb@example.com")
		bad := userHeaders("difftest-us-badvoice", "us-badvoice@example.com")
		fresh := userHeaders("difftest-us-new", "us-new@example.com")
		base := diff.Scenario{Setup: userFixture, Tables: []string{"completions", "fcm_tokens", "user_onboardings", "active_storage_blobs", "active_storage_attachments"},
			Snapshot: userSnapshots}
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			s := base
			s.Name, s.Steps = name, steps
			return s
		}
		get := func(path string, h map[string]string) diff.Request {
			return diff.Request{Path: path, Headers: h}
		}

		png, jpg := testPNG(), testJPEG()
		big := make([]byte, 5<<20+1)
		copy(big, png)
		upload := func(h map[string]string, files ...[4]any) diff.Request {
			body, ct := multipartBody(map[string]string{"note": "x"}, files...)
			return diff.Request{Method: "POST", Path: "/api/v1/users/avatar", Headers: withHeaders(h, map[string]string{"Content-Type": ct}), Body: body}
		}
		avatar := sc("avatar",
			upload(plain, [4]any{"avatar", "me.png", "image/png", png}),
			get("/api/v1/users/me", plain),
			upload(plain, [4]any{"avatar", "second photo.jpg", "image/jpeg", jpg}),
			get("/api/v1/users/me", plain),
			upload(plain, [4]any{"avatar", "a.gif", "image/gif", png}),
			upload(plain, [4]any{"avatar", "a.png", "", png}),
			upload(plain, [4]any{"avatar", "declared.png", "image/png", jpg}),
			upload(plain, [4]any{"avatar", "big.png", "image/png", big}),
			upload(plain, [4]any{"other", "a.png", "image/png", png}),
			diff.Request{Method: "POST", Path: "/api/v1/users/avatar", Headers: withHeaders(plain, map[string]string{"Content-Type": "application/json"}), Body: `{"avatar":"text"}`},
			diff.Request{Method: "DELETE", Path: "/api/v1/users/avatar", Headers: plain},
			diff.Request{Method: "DELETE", Path: "/api/v1/users/avatar", Headers: plain},
			get("/api/v1/users/me", plain),
			upload(bad, [4]any{"avatar", "b.png", "image/png", png}),
		)
		avatar.Prepare = resetFakeS3
		avatar.Settle = settleJobs("ActiveStorage::PurgeJob", "ActiveStorage::AnalyzeJob")
		avatar.HTTPSnapshots = []string{envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000") + "/__objects?summary=1"}

		return []diff.Scenario{
			sc("profile",
				get("/api/v1/users/me", plain),
				get("/api/v1/users/me", onb),
				get("/api/v1/users/me", bad),
				get("/api/v1/users/me", map[string]string{"Accept": "application/json"}),
				jsonReq("PATCH", "/api/v1/users/profile", plain, `{"name":"Renamed","photo_url":"https://example.com/q.png","email":"hijack@example.com"}`),
				get("/api/v1/users/me", plain),
				jsonReq("PATCH", "/api/v1/users/profile", plain, `{"name":null}`),
				jsonReq("PATCH", "/api/v1/users/profile", plain, `{"name":{"x":1},"photo_url":42}`),
				jsonReq("PATCH", "/api/v1/users/profile", plain, `{}`),
				jsonReq("PATCH", "/api/v1/users/profile", bad, `{"name":"X"}`),
				diff.Request{Method: "PATCH", Path: "/api/v1/users/profile?name=Query", Headers: onb},
			),
			sc("preferences",
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":{"notifications":false,"creed_type":"nicene","morning_3_canticle_before_reading":true,"weird-key":1,"float_pref":1.5,"list_pref":["a",2],"nested":{"a":1},"prayer_times":[{"office_id":"morning","hour":7,"minute":30,"enabled":true,"extra":1},"x"]}}`),
				get("/api/v1/users/me", plain),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":{"preferred_audio_voice":"robot"}}`),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":{"prayer_book_code":"nope"}}`),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":{"prayer_book_code":"loc_2027"}}`),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"other":1}`),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":"x"}`),
				jsonReq("PATCH", "/api/v1/users/preferences", plain, `{"preferences":{"prayer_book_code":"loc_1979_en"}}`),
				get("/api/v1/users/me", plain),
				jsonReq("PATCH", "/api/v1/users/preferences", onb, `{"preferences":{"creed_type":"nicene","bible_version":"arc","mode":"advanced"}}`),
				get("/api/v1/users/me/onboarding", onb),
				jsonReq("PATCH", "/api/v1/users/preferences", onb, `{"preferences":{"prayer_book_code":"loc_2019","mode":"expert"}}`),
				jsonReq("PATCH", "/api/v1/users/preferences", onb, `{"preferences":{"prayer_book_code":"loc_2019","prayer_times":{"0":{"hour":6},"1":{"hour":18}}}}`),
				get("/api/v1/users/me/onboarding", onb),
				jsonReq("PATCH", "/api/v1/users/preferences", bad, `{"preferences":{"notifications":true}}`),
				jsonReq("PATCH", "/api/v1/users/preferences", bad, `{"preferences":{"preferred_audio_voice":"female_1"}}`),
			),
			sc("timezone and tokens",
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"Europe/London","country_code":"en-gb"}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"Brasilia"}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"Mars/Olympus"}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":""}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"UTC","country_code":"BRA"}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"UTC","country_code":""}`),
				jsonReq("PATCH", "/api/v1/users/timezone", plain, `{"timezone":"UTC","country_code":null}`),
				jsonReq("PATCH", "/api/v1/users/timezone", bad, `{"timezone":"UTC"}`),
				get("/api/v1/users/me", plain),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{"fcm_token":"tok-new","platform":"ios"}`),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{"fcm_token":"tok-existing"}`),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{"fcm_token":"tok-web","platform":"web"}`),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{"fcm_token":"tok-bad","platform":"windows"}`),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{"fcm_token":""}`),
				jsonReq("POST", "/api/v1/users/fcm_token", plain, `{}`),
				diff.Request{Method: "DELETE", Path: "/api/v1/users/fcm_token?fcm_token=tok-web", Headers: plain},
				jsonReq("DELETE", "/api/v1/users/fcm_token", plain, `{"fcm_token":"tok-missing"}`),
				diff.Request{Method: "DELETE", Path: "/api/v1/users/fcm_token", Headers: plain},
			),
			sc("completions",
				get("/api/v1/users/completions", plain),
				get("/api/v1/users/completions?limit=3", plain),
				get("/api/v1/users/completions?limit=abc", plain),
				get("/api/v1/users/completions?limit=-1", plain),
				get("/api/v1/users/completions?limit=010", plain),
				get("/api/v1/users/completions?limit=0", plain),
				get("/api/v1/users/completions", fresh),
			),
			sc("onboarding",
				get("/api/v1/users/me/onboarding", fresh),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"nope","bible_version_id":"nvi"}`),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_2015"}`),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_2027","bible_version_id":"nvi"}`),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_2015","bible_version_id":"NVI","mode":"simplified"}`, "completed_at", "created_at", "updated_at"),
				get("/api/v1/users/me/onboarding", fresh),
				get("/api/v1/users/me", fresh),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_2015","bible_version_id":"nvi","mode":"advanced","preferences":{"nope":1,"creed_type":"zzz"}}`),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_2019","bible_version_id":"nvi","mode":"","completed_at":"2026-02-03T04:05:06Z"}`),
				jsonReq("POST", "/api/v1/users/onboarding", fresh, `{"prayer_book_id":"loc_1979_en","bible_version_id":"kjv","mode":"advanced","completed_at":"2026-02-03T04:05:06Z","preferences":{"daily_office_rite":"1"}}`, "created_at", "updated_at"),
				get("/api/v1/users/me/onboarding", fresh),
				get("/api/v1/users/me", fresh),
			),
			avatar,
		}
	})
}
