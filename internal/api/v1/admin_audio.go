package v1

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/audioadmin"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// withAdminAudio runs Admin::AudioController's before_actions:
// authenticate_admin!, then (for the polled actions) Cache-Control: no-store.
func withAdminAudio(noStore bool, action func(c *web.Context)) web.HandlerFunc {
	return func(c *web.Context) {
		auth.AuthenticateAdmin(c)
		if noStore {
			c.Header.Set("Cache-Control", "no-store")
		}
		action(c)
	}
}

// requestPayload ports params.to_unsafe_h.except("controller", "action").
func requestPayload(c *web.Context) *rb.Map { return c.Params().Except("controller", "action") }

// arFindID ports the integer cast Active Record applies to a find(id).
func arFindID(v any) int64 {
	s := strings.TrimSpace(rb.ToS(v))
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
		if n > math.MaxInt32*1024 {
			return -1
		}
	}
	return int64(n)
}

// findClip ports AudioClip.find(params[:id]); quoted says whether the
// message inspects the id (Model.find) or not (a relation's find, as
// includes(...).find).
func findClip(c *web.Context, quoted bool) *audioadmin.Clip {
	raw := c.Param("id")
	clip := audioadmin.FindClip(c.Ctx, arFindID(raw))
	if clip == nil {
		web.RecordNotFound("Couldn't find AudioClip with 'id'=" + idText(raw, quoted))
	}
	return clip
}

func idText(raw any, quoted bool) string {
	if quoted {
		return rb.Inspect(raw)
	}
	return rb.ToS(raw)
}

func adminUserID(c *web.Context) string {
	return strconv.FormatInt(auth.CurrentUser(c).ID, 10)
}

func invalidAudio(c *web.Context, message, code string) {
	c.JSON(422, rb.M("error", message, "code", code))
}

// AdminAudioGenerationStatus ports #generation_status.
var AdminAudioGenerationStatus = withAdminAudio(false, func(c *web.Context) {
	type session struct {
		id                                    int64
		code, status                          string
		voiceKeys                             []string
		voice, errorLog                       *string
		textID                                *int64
		total, processed, failed              *int64
		startedAt, completedAt                *time.Time
	}
	load := func(sql string) []session {
		rows, err := db.Q().Query(c.Ctx, `SELECT id, prayer_book_code, status, voice_keys, current_voice_key, error_log, current_text_id,
			total_texts, processed_count, failed_count, started_at, completed_at FROM audio_generation_sessions `+sql)
		pgMust(err)
		defer rows.Close()
		var out []session
		for rows.Next() {
			var s session
			pgMust(rows.Scan(&s.id, &s.code, &s.status, &s.voiceKeys, &s.voice, &s.errorLog, &s.textID, &s.total,
				&s.processed, &s.failed, &s.startedAt, &s.completedAt))
			out = append(out, s)
		}
		pgMust(rows.Err())
		return out
	}
	summary := func(s session) *rb.Map {
		total, processed := int64(0), int64(0)
		if s.total != nil {
			total = *s.total
		}
		if s.processed != nil {
			processed = *s.processed
		}
		var progress any = 0
		if total > 0 {
			progress = rb.RoundFloat(float64(processed)/float64(total)*100, 2)
		}
		keys := []any{}
		for _, k := range s.voiceKeys {
			keys = append(keys, k)
		}
		return rb.M("id", s.id, "prayer_book_code", s.code, "voice_keys", keys, "status", s.status,
			"current_voice", rb.Deref(s.voice), "current_text_id", rb.Deref(s.textID), "total_texts", rb.Deref(s.total),
			"processed_count", rb.Deref(s.processed), "failed_count", rb.Deref(s.failed), "progress_percentage", progress,
			"started_at", timeJSON(s.startedAt), "completed_at", timeJSON(s.completedAt), "error_log", rb.Deref(s.errorLog))
	}
	recent := load(`ORDER BY created_at DESC LIMIT 10`)
	var active any
	if list := load(`WHERE status = 'running' ORDER BY id ASC LIMIT 1`); len(list) > 0 {
		active = summary(list[0])
	}
	var totalTexts, completedTexts int64
	voices := []string{"male_1", "female_1", "male_2"}
	pgMust(db.Q().QueryRow(c.Ctx, `SELECT COUNT(*) FROM liturgical_texts`).Scan(&totalTexts))
	pgMust(db.Q().QueryRow(c.Ctx, `SELECT COUNT(*) FROM liturgical_texts WHERE (audio_urls ?& $1::text[])`, voices).Scan(&completedTexts))
	possible := totalTexts * 3
	completed := completedTexts * 3
	var percentage any = 0
	if possible > 0 {
		percentage = rb.RoundFloat(float64(completed)/float64(possible)*100, 2)
	}
	sessions := []any{}
	for _, s := range recent {
		sessions = append(sessions, summary(s))
	}
	c.JSON(200, rb.M(
		"overall_progress", rb.M("total_texts", totalTexts, "total_voices", 3, "total_possible_audio_files", possible,
			"completed_audio_files", completed, "completion_percentage", percentage),
		"active_session", active,
		"recent_sessions", sessions,
		"available_voices", []any{rb.M("key", "male_1", "name", "Victor Power"), rb.M("key", "female_1", "name", "Rita"),
			rb.M("key", "male_2", "name", "Will")},
	))
})

func timeJSON(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}

// AdminAudioSummary ports #summary.
var AdminAudioSummary = withAdminAudio(true, func(c *web.Context) { c.JSON(200, audioadmin.Summary(c.Ctx)) })

var clipFilterKeys = []string{"kind", "provider", "model", "voice", "language", "speed", "fingerprint", "max_characters", "q",
	"created_after", "created_before", "prayer_book_code", "source_name", "profile_status", "sort", "direction", "limit", "offset"}

var cleanupFilterKeys = []string{"kind", "provider", "model", "voice", "language", "speed", "fingerprint", "q", "created_after",
	"created_before", "prayer_book_code", "source_name", "profile_status", "sort", "direction"}

// AdminAudioClips ports #clips.
var AdminAudioClips = withAdminAudio(true, func(c *web.Context) {
	q := &audioadmin.ClipQuery{Filters: requestPayload(c).Slice(clipFilterKeys...)}
	records, total, limit, offset := q.Paginated(c.Ctx)
	_, fps := audioadmin.CurrentFingerprints(c.Ctx)
	clips := []any{}
	for _, clip := range records {
		clips = append(clips, audioadmin.ClipPayloadFiltered(c.Ctx, clip, fps, q.Filters))
	}
	c.JSON(200, rb.M("clips", clips, "pagination", rb.M("total", total, "limit", limit, "offset", offset, "count", len(records))))
})

// AdminAudioClip ports #clip.
var AdminAudioClip = withAdminAudio(true, func(c *web.Context) {
	clip := findClip(c, true)
	c.JSON(200, rb.M("clip", audioadmin.ClipPayload(c.Ctx, clip, nil)))
})

// AdminAudioClipURL ports #clip_url.
var AdminAudioClipURL = withAdminAudio(true, func(c *web.Context) {
	clip := findClip(c, true)
	c.JSON(200, rb.M("id", clip.ID, "url", audio.PlainStorage().URLFor(clip.Filename), "expires_in", audio.URLExpiresIn()))
})

// AdminAudioOperations ports #operations.
var AdminAudioOperations = withAdminAudio(true, func(c *web.Context) {
	limit := 20
	if v := c.Param("limit"); v != nil {
		if n, errClass := rb.KernelInteger(v); errClass == "" {
			limit = int(min(max(n, 1), 100))
		}
	}
	ops := audioadmin.RecentOperations(c.Ctx, limit)
	c.JSON(200, rb.M("operations", audioadmin.OperationStatuses(c.Ctx, ops)))
})

// AdminAudioOperation ports #operation.
var AdminAudioOperation = withAdminAudio(true, func(c *web.Context) {
	raw := c.Param("id")
	o := audioadmin.FindOperation(c.Ctx, arFindID(raw))
	if o == nil {
		web.RecordNotFound("Couldn't find AudioOperation with 'id'=" + rb.Inspect(raw))
	}
	c.JSON(200, rb.M("operation", audioadmin.Status(c.Ctx, o)))
})

func operationResponse(c *web.Context, o *audioadmin.Operation, err error) {
	pgMust(err)
	c.JSON(202, rb.M("operation", audioadmin.Status(c.Ctx, o)))
}

// AdminAudioRegenerate ports #regenerate.
var AdminAudioRegenerate = withAdminAudio(false, func(c *web.Context) {
	findClip(c, true)
	raw := c.Param("additional_instructions")
	instructions := strings.TrimSpace(rb.ToS(raw))
	if raw != nil {
		instructions = rb.Strip(rb.ToS(raw))
	}
	if len([]rune(instructions)) > 1000 {
		invalidAudio(c, "custom audio instructions are too long", "INVALID_AUDIO_CUSTOMIZATION")
		return
	}
	clipID := arFindID(c.Param("id"))
	params := rb.M("clip_id", clipID)
	var arg any
	if !rb.BlankString(instructions) {
		params.Set("additional_instructions", instructions)
		arg = instructions
	}
	o, err := audioadmin.EnqueueOperation(c.Ctx, "regenerate_clip", params, nil, adminUserID(c), func(id int64) solidqueue.Job {
		return solidqueue.Job{Class: "RegenerateAudioClipJob", Queue: "maintenance",
			Arguments: []any{clipID, solidqueue.Kwargs("additional_instructions", arg, "operation_id", id)}}
	})
	operationResponse(c, o, err)
})

// AdminAudioAcceptCandidate ports #accept_candidate.
var AdminAudioAcceptCandidate = withAdminAudio(false, func(c *web.Context) {
	clip := findClip(c, true)
	raw := c.Param("candidate_id")
	cand := audioadmin.FindCandidate(c.Ctx, clip.ID, arFindID(raw))
	if cand == nil {
		web.RecordNotFound(`Couldn't find AudioClipCandidate with 'id'=` + rb.Inspect(raw) + ` [WHERE "audio_clip_candidates"."audio_clip_id" = $1]`)
	}
	replacement, err := audioadmin.AcceptCandidate(c.Ctx, cand)
	if ae, ok := err.(*audioadmin.ArgumentError); ok {
		panic(&web.StandardError{Class: "ArgumentError", Message: ae.Message})
	}
	pgMust(err)
	c.JSON(200, rb.M("clip", audioadmin.ClipPayload(c.Ctx, replacement, nil)))
})

// AdminAudioRejectCandidate ports #reject_candidate.
var AdminAudioRejectCandidate = withAdminAudio(false, func(c *web.Context) {
	clip := findClip(c, true)
	raw := c.Param("candidate_id")
	cand := audioadmin.FindCandidate(c.Ctx, clip.ID, arFindID(raw))
	if cand == nil {
		web.RecordNotFound(`Couldn't find AudioClipCandidate with 'id'=` + rb.Inspect(raw) + ` [WHERE "audio_clip_candidates"."audio_clip_id" = $1]`)
	}
	pgMust(audioadmin.RejectCandidate(c.Ctx, cand))
	c.HeadStatus(204)
})

// nested ports `request_payload[key].to_h.stringify_keys.presence ||
// request_payload`: a nested hash (whose values Hash#to_h turns plain) or
// the whole payload (whose nested hashes stay indifferent).
func nested(c *web.Context, key string) (*rb.Map, bool) {
	p := requestPayload(c)
	switch v := p.Get(key).(type) {
	case *rb.Map:
		if v.Len() > 0 {
			return v, false
		}
	case nil:
	default:
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_h", v)})
	}
	return p, true
}

// cleanupFilters ports #cleanup_filters.
func cleanupFilters(c *web.Context) *rb.Map {
	p, _ := nested(c, "cleanup")
	return p.Slice(cleanupFilterKeys...)
}

// AdminAudioCleanupPreview ports #cleanup_preview.
var AdminAudioCleanupPreview = withAdminAudio(false, func(c *web.Context) {
	filters := cleanupFilters(c)
	q := &audioadmin.ClipQuery{Filters: filters}
	_, fps := audioadmin.CurrentFingerprints(c.Ctx)
	sample := []any{}
	for _, clip := range q.Records(c.Ctx, "audio_clips.created_at ASC", 10, 0) {
		sample = append(sample, audioadmin.ClipPayloadFiltered(c.Ctx, clip, fps, filters))
	}
	c.JSON(200, rb.M("filters", filters, "profile_status", filters.Get("profile_status"), "total_clips", q.Count(c.Ctx),
		"total_characters", q.Sum(c.Ctx, "character_count"), "total_duration_seconds", q.Sum(c.Ctx, "duration"),
		"sample", sample))
})

var cleanupAllowedKeys = []string{"created_at", "character_count", "duration", "provider", "voice", "kind", "model", "language",
	"speed", "fingerprint", "q", "created_after", "created_before", "prayer_book_code", "source_name", "profile_status"}

// normalizeParameters ports OperationEnqueuer#normalize.
func normalizeParameters(p *rb.Map, keys []string) *rb.Map {
	return p.Slice(keys...).Compact()
}

// AdminAudioCleanup ports #cleanup.
var AdminAudioCleanup = withAdminAudio(false, func(c *web.Context) {
	filters := normalizeParameters(cleanupFilters(c), cleanupAllowedKeys)
	if s := rb.ToS(filters.Get("profile_status")); s != "legacy" && s != "stale" {
		invalidAudio(c, "cleanup requires profile_status=legacy or profile_status=stale", "INVALID_AUDIO_CLEANUP")
		return
	}
	o, err := audioadmin.EnqueueOperation(c.Ctx, "cleanup_clips", filters, filters.Get("prayer_book_code"), adminUserID(c),
		func(id int64) solidqueue.Job {
			return solidqueue.Job{Class: "CleanupAudioClipsJob", Queue: "maintenance",
				Arguments: []any{solidqueue.StringHash(filters), solidqueue.Kwargs("operation_id", id)}}
		})
	operationResponse(c, o, err)
})

// AdminAudioReindexCatalog ports #reindex_catalog.
var AdminAudioReindexCatalog = withAdminAudio(false, func(c *web.Context) {
	code := presence(c.Param("prayer_book_code"))
	params := rb.NewMap()
	var arg any
	if code != nil {
		params.Set("prayer_book_code", rb.ToS(code))
		arg = rb.ToS(code)
	}
	o, err := audioadmin.EnqueueOperation(c.Ctx, "index_catalog", params, code, adminUserID(c), func(id int64) solidqueue.Job {
		return solidqueue.Job{Class: "IndexAudioCatalogJob", Queue: "maintenance",
			Arguments: []any{solidqueue.Kwargs("prayer_book_code", arg, "operation_id", id)}}
	})
	operationResponse(c, o, err)
})

func presence(v any) any {
	if rb.Blank(v) {
		return nil
	}
	return v
}

// booleanCast ports ActiveModel::Type::Boolean#cast.
func booleanCast(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case string:
		if x == "" {
			return nil
		}
		switch x {
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false
		}
		return true
	case int:
		return x != 0
	}
	return true
}

// AdminAudioGenerateCatalog ports #generate_catalog.
var AdminAudioGenerateCatalog = withAdminAudio(false, func(c *web.Context) {
	p, _ := nested(c, "catalog")
	code := books.DefaultCode
	if v := presence(p.Get("prayer_book_code")); v != nil {
		code = rb.ToS(v)
	}
	pb, err := store.PrayerBookByCode(c.Ctx, code)
	pgMust(err)
	if pb == nil {
		invalidAudio(c, "unknown prayer book: "+code, "INVALID_AUDIO_GENERATION")
		return
	}
	params := rb.M("prayer_book_code", code)
	var translations []any
	switch v := p.Get("translations").(type) {
	case []any:
		for _, t := range v {
			if s := rb.ToS(t); !rb.BlankString(s) {
				translations = append(translations, s)
			}
		}
	case nil:
	default:
		if s := rb.ToS(v); !rb.BlankString(s) {
			translations = append(translations, s)
		}
	}
	if len(translations) > 0 {
		params.Set("translations", translations)
	}
	generate := booleanCast(p.Get("dry_run"))
	params.Set("generate", generate == nil || generate == false)
	o, err := audioadmin.EnqueueOperation(c.Ctx, "generate_catalog", params, code, adminUserID(c), func(id int64) solidqueue.Job {
		var tr any
		if translations != nil {
			tr = translations
		}
		return solidqueue.Job{Class: "GenerateBookAudioJob", Queue: "maintenance",
			Arguments: []any{solidqueue.Kwargs("prayer_book_code", code, "translations", tr, "generate", params.Get("generate"), "operation_id", id)}}
	})
	operationResponse(c, o, err)
})

// generationArguments ports #generation_arguments; ok=false when the
// request was answered with 422.
func generationArguments(c *web.Context) (*rb.Map, bool, bool) {
	p, indifferent := nested(c, "generation")
	args := rb.NewMap()
	code := books.DefaultCode
	if v := presence(p.Get("prayer_book_code")); v != nil {
		code = rb.ToS(v)
	}
	args.Set("prayer_book_code", code)
	start := today().ISO()
	if v := presence(p.Get("start_date")); v != nil {
		s, ok := v.(string)
		if !ok {
			panic(&web.StandardError{Class: "TypeError", Message: rb.ImplicitConversionMessage(v, "String")})
		}
		ymd, err := rb.DateISO8601(s)
		if err != nil {
			invalidAudio(c, "invalid date", "INVALID_AUDIO_GENERATION")
			return nil, false, false
		}
		start = ymd.ISO()
	}
	args.Set("start_date", start)
	var days any = 1
	if v := presence(p.Get("days")); v != nil {
		days = v
	}
	n, errClass := rb.KernelInteger(days)
	switch errClass {
	case "":
	case "ArgumentError":
		invalidAudio(c, "invalid value for Integer(): "+rb.Inspect(days), "INVALID_AUDIO_GENERATION")
		return nil, false, false
	default:
		panic(&web.StandardError{Class: errClass, Message: "can't convert " + rb.ClassName(days) + " into Integer"})
	}
	args.Set("days", n)
	if v := presence(p.Get("offices")); v != nil {
		args.Set("offices", v)
	}
	if m, ok := p.Get("preferences").(*rb.Map); ok {
		args.Set("preferences", m)
	} else {
		args.Set("preferences", rb.NewMap())
		indifferentPrefs := false
		_ = indifferentPrefs
	}
	if v := presence(p.Get("variants")); v != nil {
		args.Set("variants", v)
	}
	return args, indifferent, true
}

// AdminAudioGenerate ports #generate.
var AdminAudioGenerate = withAdminAudio(false, func(c *web.Context) {
	args, indifferent, ok := generationArguments(c)
	if !ok {
		return
	}
	days := args.Get("days").(int64)
	if days < 1 || days > 31 {
		invalidAudio(c, "days must be between 1 and 31", "INVALID_AUDIO_GENERATION")
		return
	}
	code := rb.ToS(args.Get("prayer_book_code"))
	pb, err := store.PrayerBookByCode(c.Ctx, code)
	pgMust(err)
	if pb == nil {
		web.Raise("UnsupportedPrayerBook", "unknown prayer book: "+code, "")
	}
	caps := books.For(pb.Code, pb.Features)
	available := caps.AvailableOffices(false)
	if caps.SupportsFamilyRite() {
		for _, o := range caps.AvailableOffices(true) {
			if !containsStr(available, o) {
				available = append(available, o)
			}
		}
	}
	var offices []string
	switch v := args.Get("offices").(type) {
	case []any:
		for _, o := range v {
			offices = append(offices, rb.ToS(o))
		}
	case nil:
	default:
		offices = []string{rb.ToS(v)}
	}
	var invalid []string
	for _, o := range offices {
		if !containsStr(available, o) {
			invalid = append(invalid, o)
		}
	}
	if len(invalid) > 0 {
		invalidAudio(c, "unsupported offices: "+strings.Join(invalid, ", "), "INVALID_AUDIO_GENERATION")
		return
	}
	params := normalizeParameters(args, []string{"start_date", "days", "prayer_book_code", "offices", "preferences", "variants"})
	serialize := func(v any) any {
		if indifferent {
			switch x := v.(type) {
			case *rb.Map:
				return solidqueue.IndifferentHash(x)
			case []any:
				out := make([]any, len(x))
				for i, e := range x {
					if m, ok := e.(*rb.Map); ok {
						out[i] = solidqueue.IndifferentHash(m)
					} else {
						out[i] = e
					}
				}
				return out
			}
		}
		return solidqueue.SerializeValue(v)
	}
	o, err := audioadmin.EnqueueOperation(c.Ctx, "generate_office", params, code, adminUserID(c), func(id int64) solidqueue.Job {
		var kv []any
		params.Each(func(k string, v any) {
			if k == "preferences" && v.(*rb.Map).Len() == 0 {
				kv = append(kv, k, solidqueue.StringHash(rb.NewMap()))
				return
			}
			kv = append(kv, k, serialize(v))
		})
		kv = append(kv, "operation_id", id)
		return solidqueue.Job{Class: "PrewarmOfficeAudioJob", Queue: "maintenance", Arguments: []any{solidqueue.Kwargs(kv...)}}
	})
	operationResponse(c, o, err)
})

// AdminAudioWorkerQueue ports #worker_queue.
var AdminAudioWorkerQueue = withAdminAudio(true, func(c *web.Context) { c.JSON(200, audioadmin.WorkerQueue(c.Ctx)) })

// AdminAudioPurgeWorkerQueue ports #purge_worker_queue.
var AdminAudioPurgeWorkerQueue = withAdminAudio(false, func(c *web.Context) {
	scope := rb.ToS(c.Param("scope"))
	if rb.BlankString(scope) {
		scope = "dead"
	}
	c.JSON(200, audioadmin.PurgeWorkerQueue(c.Ctx, scope))
})

var _ = civil.New
var _ context.Context

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
