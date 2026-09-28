package suites

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/dodopok/estevao-api-go/test/diff"
)

// backgroundMusicFixture publishes part of the seeded catalogue: tracks
// 1-55 get an mp3_96 asset, 56 only other profiles, 57 a withdrawn asset, 58
// an inactive one and 59-60 none; track 3 carries a non-commercial licence.
// The onboarded user and 990060 (English, loc_1662_en) have the feature flag;
// 990061 is premium without it.
var backgroundMusicFixture = userFixture + `
DELETE FROM background_track_assets WHERE id BETWEEN 990001 AND 990200;
UPDATE background_tracks SET license_type = 'Public domain dedication (source page)' WHERE id = 3;
UPDATE background_tracks SET published_at = NULL WHERE published_at IS NOT NULL;
UPDATE background_tracks SET published_at = '2026-01-01 00:00:00' WHERE id <= 60;
UPDATE background_tracks SET license_type = 'CC BY-NC 4.0' WHERE id = 3;
INSERT INTO background_track_assets (id, track_id, profile, object_key, mime_type, bytes, duration_ms, sha256, status, active,
  loop_start_ms, loop_end_ms, created_at, updated_at)
SELECT 990000 + id, id, 'mp3_96', 'background_music/' || slug || '/' || md5(slug) || '/mp3_96.mp3', 'audio/mpeg', 1000 + id,
  120000 + id, md5(slug) || md5(slug), 'ready', TRUE, CASE WHEN id % 2 = 0 THEN 1000 END, CASE WHEN id % 2 = 0 THEN 110000 END,
  '2026-01-01', '2026-01-01' FROM background_tracks WHERE id <= 55;
INSERT INTO background_track_assets (id, track_id, profile, object_key, mime_type, bytes, duration_ms, sha256, status, active,
  created_at, updated_at) VALUES
 (990101, 56, 'opus_64', 'background_music/t56/a/opus_64.ogg', 'audio/ogg', 900, 130000, 'aaaa56', 'ready', TRUE, '2026-01-01', '2026-01-01'),
 (990102, 56, 'aac_64', 'background_music/t56/b/aac_64.m4a', 'audio/mp4', 800, 130000, 'bbbb56', 'ready', TRUE, '2026-01-01', '2026-01-01'),
 (990103, 57, 'mp3_96', 'background_music/t57/c/mp3_96.mp3', 'audio/mpeg', 700, 130000, 'cccc57', 'withdrawn', TRUE, '2026-01-01', '2026-01-01'),
 (990104, 58, 'mp3_96', 'background_music/t58/d/mp3_96.mp3', 'audio/mpeg', 700, 130000, 'dddd58', 'ready', FALSE, '2026-01-01', '2026-01-01');
INSERT INTO users (id, provider_uid, email, name, preferences, premium_expires_at, timezone, created_at, updated_at) VALUES
 (990060, 'difftest-bg-en', 'bg-en@example.com', 'BG', '{"prayer_book_code":"loc_1662_en"}', '2099-01-01', 'UTC', '2026-01-01', '2026-01-01'),
 (990061, 'difftest-bg-noflag', 'bg-noflag@example.com', 'BG2', '{}', '2099-01-01', 'UTC', '2026-01-01', '2026-01-01');
INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at) VALUES
 ('background_music', 'user', 990002, TRUE, '2026-01-01', '2026-01-01'),
 ('background_music', 'user', 990060, TRUE, '2026-01-01', '2026-01-01');
INSERT INTO user_background_track_favorites (user_id, track_id, created_at, updated_at) VALUES
 (990002, 1, '2026-03-01', '2026-03-01'), (990002, 5, '2026-03-03', '2026-03-03'), (990002, 10, '2026-03-02', '2026-03-02'),
 (990002, 59, '2026-03-04', '2026-03-04'), (990002, 20, '2026-03-05', '2026-03-05');
`

func init() {
	registerScenarios("background_music", func() []diff.Scenario {
		onb := userHeaders("difftest-us-onb", "us-onb@example.com")
		en := userHeaders("difftest-bg-en", "bg-en@example.com")
		noflag := userHeaders("difftest-bg-noflag", "bg-noflag@example.com")
		plain := userHeaders("difftest-us-plain", "us-plain@example.com")
		anon := withHeaders(AppHeaders(), map[string]string{"Host": "api.example.test"})
		get := func(path string, h map[string]string) diff.Request { return diff.Request{Path: path, Headers: h} }
		sc := func(name string, steps ...diff.Request) diff.Scenario {
			return diff.Scenario{Name: name, Setup: backgroundMusicFixture,
				Tables: []string{"user_background_track_favorites", "user_audio_usages"},
				Settle: settleJobs("Audio::RecordUserUsageJob"),
				Snapshot: []string{
					`SELECT user_id, track_id FROM user_background_track_favorites WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, track_id`,
					`SELECT user_id, audio_type, asset_key, background_track_id, office_type, prayer_book_code, voice, access_count
					   FROM user_audio_usages WHERE user_id BETWEEN 990001 AND 990099 ORDER BY user_id, asset_key`,
				},
				Steps: steps}
		}
		cursor := func(json string) string {
			return url.QueryEscape(base64.RawURLEncoding.EncodeToString([]byte(json)))
		}

		var calendar []diff.Request
		dates := []string{"2026-11-29", "2026-12-24", "2026-12-25", "2027-01-06", "2027-01-10", "2027-02-10", "2027-03-21",
			"2027-03-25", "2027-03-26", "2027-03-27", "2027-03-28", "2027-05-06", "2027-05-16", "2027-05-23", "2026-11-01",
			"2026-11-02", "2026-09-28", "2026-07-15"}
		bookOffices := []string{"loc_2015/evening", "loc_1662_en/morning", "loc_1928_en/evening", "cw_2005_en/compline",
			"loc_2019_es/midday", "loc_1984_cy/morning", "awrv_2025_en/prime", "loc_1962_en/compline", "loc_1991_pt/morning",
			"loc_2019/morning"}
		for _, d := range dates {
			for _, bo := range bookOffices {
				b, o, _ := strings.Cut(bo, "/")
				calendar = append(calendar, get("/api/v1/background_music/home?date="+d+"&prayer_book_code="+b+"&office_type="+o, onb))
			}
			calendar = append(calendar, get("/api/v1/background_music/tracks?limit=50&date="+d, en))
		}

		return []diff.Scenario{
			sc("access",
				get("/api/v1/background_music/home", anon),
				get("/api/v1/background_music/home", plain),
				get("/api/v1/background_music/home", noflag),
				get("/api/v1/background_music/home", onb),
				get("/api/v1/background_music/home", en),
				get("/api/v1/background_music/favorites", onb),
				get("/api/v1/background_music/favorites", en),
				get("/api/v1/background_music/home?prayer_book_code=bogus", onb),
				get("/api/v1/background_music/home?prayer_book_code=loc_2027", onb),
				get("/api/v1/background_music/home?office_type=prime", onb),
				get("/api/v1/background_music/home?office_type=bogus&prayer_book_code=awrv_2025_en", onb),
				get("/api/v1/background_music/home?office_type=none&prayer_book_code=awrv_2025_en", onb),
				get("/api/v1/background_music/home?date=2026-02-30", onb),
				get("/api/v1/background_music/home?date=1800-01-01", onb),
				get("/api/v1/background_music/home?date=20261225", onb),
				get("/api/v1/background_music/home?date=2026-W52-5", onb),
				get("/api/v1/background_music/home?date[]=x", onb),
				get("/api/v1/background_music/home?date=%20", onb),
			),
			sc("calendar", calendar...),
			sc("tracks",
				get("/api/v1/background_music/tracks", onb),
				get("/api/v1/background_music/tracks?limit=5", onb),
				get("/api/v1/background_music/tracks?limit=5&cursor="+cursor(`{"order_key":[5,1,100000,130,12]}`), onb),
				get("/api/v1/background_music/tracks?limit=5&cursor="+cursor(`{"order_key":["2","0x1"]}`), onb),
				get("/api/v1/background_music/tracks?limit=3&cursor="+cursor(`{"sort_order":200,"id":"20"}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"order_key":[]}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"order_key":null}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"order_key":7}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"order_key":[1.9,true]}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"order_key":{"a":1}}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`{"sort_order":1}`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`[1,2]`), onb),
				get("/api/v1/background_music/tracks?cursor="+cursor(`not json`), onb),
				get("/api/v1/background_music/tracks?cursor=%%%", onb),
				get("/api/v1/background_music/tracks?cursor=eyJvcmRlcl9rZXkiOlsxXX0=", onb),
				get("/api/v1/background_music/tracks?limit=0", onb),
				get("/api/v1/background_music/tracks?limit=51", onb),
				get("/api/v1/background_music/tracks?limit=abc", onb),
				get("/api/v1/background_music/tracks?limit=0x10", onb),
				get("/api/v1/background_music/tracks?limit[]=1", onb),
				get("/api/v1/background_music/tracks?category=advent", onb),
				get("/api/v1/background_music/tracks?category=all&kind=chant", onb),
				get("/api/v1/background_music/tracks?category=bogus", onb),
				get("/api/v1/background_music/tracks?kind=bogus", onb),
				get("/api/v1/background_music/tracks?favorites=true", onb),
				get("/api/v1/background_music/tracks?favorites=true&category=lent", onb),
				get("/api/v1/background_music/tracks?q=bach", onb),
				get("/api/v1/background_music/tracks?q=MEMBETH&kind=chant", onb),
				get("/api/v1/background_music/tracks?q=%25", onb),
				get("/api/v1/background_music/tracks?q=_", onb),
				get("/api/v1/background_music/tracks?q=%20%20", onb),
				get("/api/v1/background_music/tracks?q="+strings.Repeat("a", 81), onb),
				get("/api/v1/background_music/tracks?q=%20"+strings.Repeat("a", 80)+"%20", onb),
				get("/api/v1/background_music/tracks?prayer_book_code=loc_1984_cy&office_type=evening", onb),
				get("/api/v1/background_music/tracks?category=bogus&date=bad", onb),
				get("/api/v1/background_music/tracks?date=bad&office_type=bogus", onb),
			),
			sc("favorites",
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/2", Headers: onb},
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/2", Headers: onb},
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/3", Headers: onb},
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/59", Headers: onb},
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/abc", Headers: onb},
				diff.Request{Method: "PUT", Path: "/api/v1/background_music/favorites/56", Headers: en},
				get("/api/v1/background_music/favorites", onb),
				diff.Request{Method: "DELETE", Path: "/api/v1/background_music/favorites/1", Headers: onb},
				diff.Request{Method: "DELETE", Path: "/api/v1/background_music/favorites/1", Headers: onb},
				diff.Request{Method: "DELETE", Path: "/api/v1/background_music/favorites/abc", Headers: onb},
				diff.Request{Method: "DELETE", Path: "/api/v1/background_music/favorites/1", Headers: noflag},
				get("/api/v1/background_music/home", onb),
				diff.Request{Method: "POST", Path: "/api/v1/background_music/tracks/2/playback?office_type=evening", Headers: onb,
					Volatile: []string{"expires_at"}},
				diff.Request{Method: "POST", Path: "/api/v1/background_music/tracks/56/playback", Headers: en,
					Volatile: []string{"expires_at"}},
				diff.Request{Method: "POST", Path: "/api/v1/background_music/tracks/57/playback", Headers: onb},
				diff.Request{Method: "POST", Path: "/api/v1/background_music/tracks/abc/playback", Headers: onb},
				diff.Request{Method: "POST", Path: "/api/v1/background_music/tracks/2/playback", Headers: plain},
			),
		}
	})
}
