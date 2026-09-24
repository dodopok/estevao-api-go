package v1

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// addAudioTrack ports DailyOfficeController#add_audio_track: the track is
// assembled only from clips already catalogued, never generated.
func addAudioTrack(c *web.Context, r *resolver, response *rb.Map, user *users.User) *rb.Map {
	var language any
	if meta, ok := response.Get("metadata").(*rb.Map); ok {
		language = meta.Get("language")
	}
	if language == nil {
		language = r.language()
	}
	lang := rb.ToS(language)
	provider := audio.Current(&lang)
	var keys []string
	var record audio.ClipKeyRecorder
	if user != nil {
		record = func(key string) { keys = append(keys, key) }
	}
	track, err := audio.BuildTrack(c.Ctx, audio.NewGenerator(provider), response, record)
	must(err)
	if user != nil {
		recordAudioClipUsage(c, r, user, provider, keys)
	}
	out := response.Dup()
	at := rb.M("provider", provider.Name, "model", provider.Model, "voice", provider.Voice, "language", provider.Language)
	track.Map().Each(func(k string, v any) { at.Set(k, v) })
	out.Set("audio_track", at)
	return out
}

// recordAudioClipUsage ports record_audio_clip_usage.
func recordAudioClipUsage(c *web.Context, r *resolver, user *users.User, provider *audio.Provider, keys []string) {
	var officeType any
	if v := c.Param("office_type"); rb.Present(v) {
		officeType = v
	}
	seen := map[string]bool{}
	var usages []*rb.Map
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		usages = append(usages, rb.M("audio_type", "audio_clip", "asset_key", k,
			"prayer_book_code", r.resolvedPreferences().Get("prayer_book_code"), "office_type", officeType, "voice", provider.Voice))
	}
	RecordAudioUsage(c.Ctx, user.ID, usages)
}

// RecordAudioUsage ports Audio::UserUsageRecorder.record_later. Rails enqueues
// Audio::RecordUserUsageJob; this runs the same upsert off the request, and
// like the enqueue it never fails the request.
func RecordAudioUsage(_ context.Context, userID int64, usages []*rb.Map) {
	normalized := audio.NormalizeUsages(usages)
	if userID == 0 || len(normalized) == 0 {
		return
	}
	audio.RecordUserUsageLater(userID, normalized)
}
