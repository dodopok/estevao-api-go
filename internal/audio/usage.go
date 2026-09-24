package audio

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

var usageAudioTypes = map[string]bool{"background_track": true, "audio_clip": true, "liturgical_text": true}

var usageAttributes = []string{"audio_type", "asset_key", "background_track_id", "audio_clip_id", "liturgical_text_id",
	"prayer_book_code", "office_type", "voice"}

// NormalizeUsages ports Audio::UserUsageRecorder.normalize over a list.
func NormalizeUsages(usages []*rb.Map) []*rb.Map {
	var out []*rb.Map
	for _, u := range usages {
		audioType, assetKey := rb.ToS(u.Get("audio_type")), rb.ToS(u.Get("asset_key"))
		if !usageAudioTypes[audioType] || rb.BlankString(assetKey) {
			continue
		}
		n := rb.NewMap()
		for _, k := range usageAttributes {
			if v := u.Get(k); v != nil {
				n.Set(k, v)
			}
		}
		n.Set("audio_type", audioType)
		n.Set("asset_key", assetKey)
		out = append(out, n)
	}
	return out
}

type usageJob struct {
	userID int64
	usages []*rb.Map
}

var (
	usageQueue     = make(chan usageJob, 4096)
	usageQueueOnce sync.Once
)

// RecordUserUsageLater runs Audio::RecordUserUsageJob#perform off the
// request, in enqueue order on one worker. Rails persists the job in Solid
// Queue first; here a crash between the response and the upsert loses the
// usage rows (see docs/EQUIVALENCE.md).
func RecordUserUsageLater(userID int64, usages []*rb.Map) {
	usageQueueOnce.Do(func() { go usageWorker() })
	select {
	case usageQueue <- usageJob{userID, usages}:
	default:
		go performUsage(usageJob{userID, usages})
	}
}

func usageWorker() {
	for job := range usageQueue {
		performUsage(job)
	}
}

func performUsage(job usageJob) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("[Audio::RecordUserUsageJob] failed", "error", rec)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := RecordUserUsage(ctx, job.userID, job.usages, time.Now()); err != nil {
		slog.Warn("[Audio::RecordUserUsageJob] failed", "error", err)
	}
}

type usageEntry struct {
	audioType, assetKey                 string
	prayerBookCode, officeType, voice   any
	backgroundTrackID, liturgicalTextID any
	accessCount                         int
}

// RecordUserUsage ports Audio::RecordUserUsageJob#perform.
func RecordUserUsage(ctx context.Context, userID int64, usages []*rb.Map, now time.Time) error {
	var exists bool
	if err := db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil || !exists {
		return err
	}
	var order []string
	grouped := map[string]*usageEntry{}
	for _, u := range usages {
		audioType, assetKey := rb.ToS(u.Get("audio_type")), rb.ToS(u.Get("asset_key"))
		if !usageAudioTypes[audioType] || rb.BlankString(assetKey) {
			continue
		}
		id := audioType + "\x00" + assetKey
		e := grouped[id]
		if e == nil {
			e = &usageEntry{audioType: audioType, assetKey: assetKey, prayerBookCode: u.Get("prayer_book_code"),
				officeType: u.Get("office_type"), voice: u.Get("voice"),
				backgroundTrackID: u.Get("background_track_id"), liturgicalTextID: u.Get("liturgical_text_id")}
			grouped[id] = e
			order = append(order, id)
		}
		e.accessCount++
		for _, col := range []struct {
			dst *any
			key string
		}{{&e.prayerBookCode, "prayer_book_code"}, {&e.officeType, "office_type"}, {&e.voice, "voice"}} {
			if v := u.Get(col.key); rb.Present(v) {
				*col.dst = v
			}
		}
	}
	clipIDs, err := audioClipIDs(ctx, grouped)
	if err != nil {
		return err
	}
	trackIDs, err := existingIDs(ctx, "background_tracks", grouped, "background_track")
	if err != nil {
		return err
	}
	textIDs, err := existingIDs(ctx, "liturgical_texts", grouped, "liturgical_text")
	if err != nil {
		return err
	}
	stamp := now.UTC()
	for _, id := range order {
		e := grouped[id]
		var clipID, trackID, textID any
		switch e.audioType {
		case "audio_clip":
			v, ok := clipIDs[e.assetKey]
			if !ok {
				continue
			}
			clipID = v
		case "background_track":
			v, ok := trackIDs[rb.ToI(e.backgroundTrackID)]
			if !ok || e.backgroundTrackID == nil {
				continue
			}
			trackID = v
		case "liturgical_text":
			v, ok := textIDs[rb.ToI(e.liturgicalTextID)]
			if !ok || e.liturgicalTextID == nil {
				continue
			}
			textID = v
		}
		_, err := db.Q().Exec(ctx, `INSERT INTO user_audio_usages
			(audio_type, asset_key, prayer_book_code, office_type, voice, access_count, background_track_id,
			 audio_clip_id, liturgical_text_id, user_id, first_used_at, last_used_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, $11, $11)
			ON CONFLICT (user_id, audio_type, asset_key) DO UPDATE SET
			access_count = user_audio_usages.access_count + EXCLUDED.access_count,
			first_used_at = LEAST(user_audio_usages.first_used_at, EXCLUDED.first_used_at),
			last_used_at = GREATEST(user_audio_usages.last_used_at, EXCLUDED.last_used_at),
			prayer_book_code = COALESCE(EXCLUDED.prayer_book_code, user_audio_usages.prayer_book_code),
			office_type = COALESCE(EXCLUDED.office_type, user_audio_usages.office_type),
			voice = COALESCE(EXCLUDED.voice, user_audio_usages.voice),
			updated_at = EXCLUDED.updated_at`,
			e.audioType, e.assetKey, stringOrNil(e.prayerBookCode), stringOrNil(e.officeType), stringOrNil(e.voice),
			e.accessCount, trackID, clipID, textID, userID, stamp)
		if err != nil {
			return err
		}
	}
	return nil
}

func stringOrNil(v any) any {
	if v == nil {
		return nil
	}
	return rb.ToS(v)
}

func audioClipIDs(ctx context.Context, grouped map[string]*usageEntry) (map[string]int64, error) {
	var keys []string
	for _, e := range grouped {
		if e.audioType == "audio_clip" {
			keys = append(keys, e.assetKey)
		}
	}
	out := map[string]int64{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := db.Q().Query(ctx, `SELECT key, id FROM audio_clips WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = id
	}
	return out, rows.Err()
}

func existingIDs(ctx context.Context, table string, grouped map[string]*usageEntry, audioType string) (map[int]int64, error) {
	var ids []int64
	for _, e := range grouped {
		if e.audioType != audioType {
			continue
		}
		v := e.backgroundTrackID
		if audioType == "liturgical_text" {
			v = e.liturgicalTextID
		}
		if v != nil {
			ids = append(ids, int64(rb.ToI(v)))
		}
	}
	out := map[int]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Q().Query(ctx, `SELECT id FROM `+table+` WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[int(id)] = id
	}
	return out, rows.Err()
}
