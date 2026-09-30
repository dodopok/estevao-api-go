// Package audioadmin ports Audio::Admin: the clip catalogue summary, the
// provider profile catalogue, operation status and the audio worker queue
// (read from Solid Queue's tables).
package audioadmin

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// Expectation ports ProfileCatalog::Expectation.
type Expectation struct {
	Long     string `json:"long"`
	Short    string `json:"short,omitempty"`
	ShortMax int    `json:"short_max,omitempty"`
}

// Fingerprints ports Expectation#fingerprints.
func (e Expectation) Fingerprints() []string {
	if e.Short == "" || e.Short == e.Long {
		return []string{e.Long}
	}
	return []string{e.Long, e.Short}
}

// Fingerprint ports Audio::Profile#configuration_fingerprint(normalized).
func Fingerprint(p *audio.Provider, normalized *string) string {
	return p.ConfigurationFingerprint(normalized)
}

// ExpectationFor ports Audio::Profile#expectation.
func ExpectationFor(p *audio.Provider) Expectation {
	e := Expectation{Long: Fingerprint(p, nil)}
	if p.ShortInputMax > 0 {
		dot := "."
		e.Short, e.ShortMax = Fingerprint(p, &dot), p.ShortInputMax
	}
	return e
}

// Languages ports the language list of ProfileCatalog.current_by_language
// (row order of the DISTINCT query; pt-BR when there is none).
func Languages(ctx context.Context) []string {
	rows, err := db.Q().Query(ctx, `SELECT DISTINCT "prayer_books"."language" FROM "prayer_books" WHERE "prayer_books"."language" IS NOT NULL`)
	must(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		must(rows.Scan(&l))
		out = append(out, l)
	}
	must(rows.Err())
	if len(out) == 0 {
		out = []string{"pt-BR"}
	}
	return out
}

// CurrentFingerprints ports ProfileCatalog.current_fingerprints.
func CurrentFingerprints(ctx context.Context) ([]string, map[string]Expectation) {
	langs := Languages(ctx)
	out := map[string]Expectation{}
	for _, l := range langs {
		lang := l
		out[l] = ExpectationFor(audio.Current(&lang))
	}
	return langs, out
}

// CurrentProfileScope ports ClipQuery#current_profile_scope as SQL with
// literals (sanitize_sql_array quoting), for the language/fingerprint
// pairs of the current profiles.
func CurrentProfileScope(ctx context.Context) string {
	langs, fps := CurrentFingerprints(ctx)
	var clauses []string
	for _, l := range langs {
		e := fps[l]
		if e.ShortMax == 0 {
			clauses = append(clauses, "audio_clips.language = "+lit(l)+" AND audio_clips.configuration_fingerprint = "+lit(e.Long))
			continue
		}
		n := strconv.Itoa(e.ShortMax)
		clauses = append(clauses, "audio_clips.language = "+lit(l)+" AND ((audio_clips.character_count <= "+n+
			" AND audio_clips.configuration_fingerprint = "+lit(e.Short)+") OR (audio_clips.character_count > "+n+
			" AND audio_clips.configuration_fingerprint = "+lit(e.Long)+"))")
	}
	if len(clauses) == 0 {
		return "1 = 0"
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}

func lit(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// StaleProfileScope ports ClipQuery#stale_profile_scope.
func StaleProfileScope(ctx context.Context) string {
	return `audio_clips.configuration_fingerprint IS NOT NULL AND audio_clips.id NOT IN (SELECT "audio_clips"."id" FROM "audio_clips" WHERE (` +
		CurrentProfileScope(ctx) + `))`
}

func count(ctx context.Context, sql string, args ...any) int64 {
	var n *int64
	must(db.Q().QueryRow(ctx, sql, args...).Scan(&n))
	if n == nil {
		return 0
	}
	return *n
}

// floatSum runs a SUM over a float column (0 when NULL, as Active Record's
// sum returns).
func floatSum(ctx context.Context, sql string, args ...any) any {
	var f *float64
	must(db.Q().QueryRow(ctx, sql, args...).Scan(&f))
	if f == nil {
		// sum casts `value || 0` through the float column: 0.0.
		return 0.0
	}
	return *f
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}

// Summary ports Audio::Admin::Summary.call (cached 10 seconds).
func Summary(ctx context.Context) *rb.Map {
	raw := rediscache.FetchJSON(ctx, "audio/admin/summary/v2", 10*time.Second, func() []byte {
		return rb.JSON(summaryUncached(ctx))
	})
	v, err := rb.ParseJSON(raw)
	must(err)
	return v.(*rb.Map)
}

func summaryUncached(ctx context.Context) *rb.Map {
	langs, fps := CurrentFingerprints(ctx)
	_ = langs
	spoken := `"audio_clips"."kind" = $1`
	current := count(ctx, `SELECT COUNT(DISTINCT "audio_clips"."id") FROM "audio_clips" WHERE (`+CurrentProfileScope(ctx)+`)`)
	stale := count(ctx, `SELECT COUNT(DISTINCT "audio_clips"."id") FROM "audio_clips" WHERE (`+StaleProfileScope(ctx)+`)`)
	legacy := count(ctx, `SELECT COUNT(DISTINCT "audio_clips"."id") FROM "audio_clips" WHERE "audio_clips"."configuration_fingerprint" IS NULL`)
	since := clock.Now().Add(-7 * 24 * time.Hour)
	recent := `"audio_clips"."kind" = $1 AND "audio_clips"."created_at" >= $2`
	wq := WorkerQueue(ctx)
	return rb.M(
		"scope", "lifetime",
		"total_clips", count(ctx, `SELECT COUNT(*) FROM "audio_clips" WHERE `+spoken, "line"),
		"total_characters", count(ctx, `SELECT SUM("audio_clips"."character_count") FROM "audio_clips" WHERE `+spoken, "line"),
		"total_duration_seconds", floatSum(ctx, `SELECT SUM("audio_clips"."duration") FROM "audio_clips" WHERE `+spoken, "line"),
		"current_clips", current, "stale_clips", stale, "legacy_clips", legacy,
		"silence_clips", count(ctx, `SELECT COUNT(*) FROM "audio_clips" WHERE `+spoken, "silence"),
		"pending_candidates", count(ctx, `SELECT COUNT(*) FROM "audio_clip_candidates" WHERE "audio_clip_candidates"."status" = $1`, "pending"),
		"recent", rb.M("window_days", 7,
			"clips", count(ctx, `SELECT COUNT(*) FROM "audio_clips" WHERE `+recent, "line", since),
			"characters", count(ctx, `SELECT SUM("audio_clips"."character_count") FROM "audio_clips" WHERE `+recent, "line", since),
			"duration_seconds", floatSum(ctx, `SELECT SUM("audio_clips"."duration") FROM "audio_clips" WHERE `+recent, "line", since)),
		"by_prayer_book", prayerBookRows(ctx),
		"operations", operationSummary(ctx),
		"user_usage", userUsageSummary(ctx),
		"active_operations", count(ctx, `SELECT COUNT(*) FROM "audio_operations" WHERE "audio_operations"."status" IN ($1, $2)`, "queued", "running"),
		"profiles", profileRows(ctx, fps),
		"recent_operations", OperationStatuses(ctx, loadOperations(ctx, `TRUE`, `ORDER BY "audio_operations"."created_at" DESC LIMIT 10`)),
		"worker_queue", wq,
		"active_jobs", wq.Get("jobs"))
}

func stableSort[T any](list []T, less func(a, b T) bool) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func prayerBookRows(ctx context.Context) []any {
	rows, err := db.Q().Query(ctx, `SELECT "audio_clip_usages"."prayer_book_code", COUNT(DISTINCT audio_clip_usages.audio_clip_id), COUNT(DISTINCT audio_clip_usages.source_name) FROM "audio_clip_usages" INNER JOIN "audio_clips" ON "audio_clips"."id" = "audio_clip_usages"."audio_clip_id" WHERE "audio_clips"."kind" = $1 GROUP BY "audio_clip_usages"."prayer_book_code"`, "line")
	must(err)
	defer rows.Close()
	type row struct {
		code           *string
		clips, sources int64
	}
	var list []row
	for rows.Next() {
		var r row
		must(rows.Scan(&r.code, &r.clips, &r.sources))
		list = append(list, r)
	}
	must(rows.Err())
	stableSort(list, func(a, b row) bool { return a.clips > b.clips })
	out := []any{}
	for i, r := range list {
		if i == 12 {
			break
		}
		out = append(out, rb.M("prayer_book_code", rb.Deref(r.code), "clips", r.clips, "sources", r.sources))
	}
	return out
}

func groupCount(ctx context.Context, sql string, args ...any) *rb.Map {
	rows, err := db.Q().Query(ctx, sql, args...)
	must(err)
	defer rows.Close()
	out := rb.NewMap()
	for rows.Next() {
		var n int64
		var k *string
		must(rows.Scan(&n, &k))
		out.Set(rb.ToS(rb.Deref(k)), n)
	}
	must(rows.Err())
	return out
}

func operationSummary(ctx context.Context) *rb.Map {
	var last *time.Time
	must(db.Q().QueryRow(ctx, `SELECT MAX("audio_operations"."completed_at") FROM "audio_operations" WHERE "audio_operations"."status" = $1`, "completed").Scan(&last))
	return rb.M(
		"by_status", groupCount(ctx, `SELECT COUNT(*) AS "count_all", "audio_operations"."status" AS "audio_operations_status" FROM "audio_operations" GROUP BY "audio_operations"."status"`),
		"by_kind", groupCount(ctx, `SELECT COUNT(*) AS "count_all", "audio_operations"."kind" AS "audio_operations_kind" FROM "audio_operations" GROUP BY "audio_operations"."kind"`),
		"failed_last_24_hours", count(ctx, `SELECT COUNT(*) FROM "audio_operations" WHERE "audio_operations"."status" = $1 AND "audio_operations"."completed_at" >= $2`, "failed", clock.Now().Add(-24*time.Hour)),
		"last_completed_at", timeOrNil(last))
}

func userUsageSummary(ctx context.Context) *rb.Map {
	type bg struct {
		id    *int64
		count int64
	}
	var counts []bg
	rows, err := db.Q().Query(ctx, `SELECT SUM("user_audio_usages"."access_count") AS "sum_access_count", "user_audio_usages"."background_track_id" AS "user_audio_usages_background_track_id" FROM "user_audio_usages" WHERE "user_audio_usages"."audio_type" = $1 GROUP BY "user_audio_usages"."background_track_id"`, "background_track")
	must(err)
	for rows.Next() {
		var b bg
		must(rows.Scan(&b.count, &b.id))
		counts = append(counts, b)
	}
	rows.Close()
	must(rows.Err())
	type track struct{ slug, title string }
	tracks := map[int64]track{}
	var ids []int64
	for _, b := range counts {
		if b.id != nil {
			ids = append(ids, *b.id)
		}
	}
	if len(ids) > 0 {
		rows, err = db.Q().Query(ctx, `SELECT id, slug, title FROM background_tracks WHERE id = ANY($1)`, ids)
		must(err)
		for rows.Next() {
			var id int64
			var t track
			must(rows.Scan(&id, &t.slug, &t.title))
			tracks[id] = t
		}
		rows.Close()
		must(rows.Err())
	}
	idOf := func(b bg) int64 {
		if b.id == nil {
			return math.MinInt64
		}
		return *b.id
	}
	stableSort(counts, func(a, b bg) bool { return a.count > b.count || a.count == b.count && idOf(a) < idOf(b) })
	top := []any{}
	for i, b := range counts {
		if i == 20 {
			break
		}
		var slug, title any
		if b.id != nil {
			if t, ok := tracks[*b.id]; ok {
				slug, title = t.slug, t.title
			}
		}
		top = append(top, rb.M("id", rb.Deref(b.id), "slug", slug, "title", title, "accesses", b.count))
	}
	var assets int64
	rows, err = db.Q().Query(ctx, `SELECT COUNT(*) AS "count_all", "user_audio_usages"."audio_type" AS "user_audio_usages_audio_type", "user_audio_usages"."asset_key" AS "user_audio_usages_asset_key" FROM "user_audio_usages" GROUP BY "user_audio_usages"."audio_type", "user_audio_usages"."asset_key"`)
	must(err)
	for rows.Next() {
		assets++
	}
	rows.Close()
	must(rows.Err())
	byType := rb.NewMap()
	rows, err = db.Q().Query(ctx, `SELECT SUM("user_audio_usages"."access_count") AS "sum_access_count", "user_audio_usages"."audio_type" AS "user_audio_usages_audio_type" FROM "user_audio_usages" GROUP BY "user_audio_usages"."audio_type"`)
	must(err)
	for rows.Next() {
		var n int64
		var t *string
		must(rows.Scan(&n, &t))
		byType.Set(rb.ToS(rb.Deref(t)), n)
	}
	rows.Close()
	must(rows.Err())
	return rb.M(
		"unique_users", count(ctx, `SELECT COUNT(DISTINCT "user_audio_usages"."user_id") FROM "user_audio_usages"`),
		"used_assets", assets,
		"total_accesses", count(ctx, `SELECT SUM("user_audio_usages"."access_count") FROM "user_audio_usages"`),
		"by_type", byType, "top_background_tracks", top)
}

func profileRows(ctx context.Context, fps map[string]Expectation) []any {
	rows, err := db.Q().Query(ctx, `SELECT "audio_clips"."provider", "audio_clips"."model", "audio_clips"."voice", "audio_clips"."language", "audio_clips"."speed", "audio_clips"."configuration_fingerprint", "audio_clips"."instructions_sha256", COUNT(*), SUM(character_count) FROM "audio_clips" WHERE "audio_clips"."kind" = $1 GROUP BY "audio_clips"."provider", "audio_clips"."model", "audio_clips"."voice", "audio_clips"."language", "audio_clips"."speed", "audio_clips"."configuration_fingerprint", "audio_clips"."instructions_sha256"`, "line")
	must(err)
	defer rows.Close()
	type row struct {
		provider, model, voice, language string
		speed                            float64
		fingerprint, instructions        *string
		clips, characters                int64
		status                           string
	}
	var list []row
	for rows.Next() {
		var r row
		must(rows.Scan(&r.provider, &r.model, &r.voice, &r.language, &r.speed, &r.fingerprint, &r.instructions, &r.clips, &r.characters))
		r.status = "legacy"
		if r.fingerprint != nil && !rb.BlankString(*r.fingerprint) {
			r.status = "stale"
			if e, ok := fps[r.language]; ok {
				for _, f := range e.Fingerprints() {
					if f == *r.fingerprint {
						r.status = "current"
					}
				}
			}
		}
		list = append(list, r)
	}
	must(rows.Err())
	stableSort(list, func(a, b row) bool {
		if a.status != b.status {
			return a.status < b.status
		}
		if a.provider != b.provider {
			return a.provider < b.provider
		}
		return a.voice < b.voice
	})
	out := []any{}
	for _, r := range list {
		out = append(out, rb.M("provider", r.provider, "model", r.model, "voice", r.voice, "language", r.language,
			"speed", r.speed, "configuration_fingerprint", rb.Deref(r.fingerprint), "instructions_sha256", rb.Deref(r.instructions),
			"profile_status", r.status, "clips", r.clips, "characters", r.characters))
	}
	return out
}

// Operation is an AudioOperation row.
type Operation struct {
	ID                                                                    int64
	Kind, Status                                                          string
	ActiveJobID, PrayerBookCode, ErrorMessage, RequestedBy                *string
	Parameters, Result                                                    []byte
	TotalItems, ProcessedItems, GeneratedClips, SkippedClips, FailedItems int64
	GeneratedCharacters                                                   int64
	CreatedAt                                                             time.Time
	StartedAt, CompletedAt                                                *time.Time
}

const operationColumns = `id, kind, status, active_job_id, prayer_book_code, error_message, requested_by, parameters::text,
 result::text, total_items, processed_items, generated_clips, skipped_clips, failed_items, generated_characters, created_at,
 started_at, completed_at`

func loadOperations(ctx context.Context, where, suffix string, args ...any) []*Operation {
	rows, err := db.Q().Query(ctx, `SELECT `+operationColumns+` FROM "audio_operations" WHERE `+where+" "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*Operation
	for rows.Next() {
		o := &Operation{}
		var params, result string
		must(rows.Scan(&o.ID, &o.Kind, &o.Status, &o.ActiveJobID, &o.PrayerBookCode, &o.ErrorMessage, &o.RequestedBy, &params,
			&result, &o.TotalItems, &o.ProcessedItems, &o.GeneratedClips, &o.SkippedClips, &o.FailedItems,
			&o.GeneratedCharacters, &o.CreatedAt, &o.StartedAt, &o.CompletedAt))
		o.Parameters, o.Result = []byte(params), []byte(result)
		out = append(out, o)
	}
	must(rows.Err())
	return out
}

func jsonValue(raw []byte) any {
	v, err := rb.ParseJSON(raw)
	if err != nil {
		return nil
	}
	return v
}

type queueJob struct {
	ID                   int64
	ActiveJobID          *string
	ClassName, QueueName string
	CreatedAt            time.Time
	ScheduledAt          *time.Time
	FinishedAt           *time.Time
}

func scanJobs(rows pgx.Rows) []*queueJob {
	defer rows.Close()
	var out []*queueJob
	for rows.Next() {
		j := &queueJob{}
		must(rows.Scan(&j.ID, &j.ActiveJobID, &j.ClassName, &j.QueueName, &j.CreatedAt, &j.ScheduledAt, &j.FinishedAt))
		out = append(out, j)
	}
	must(rows.Err())
	return out
}

const jobColumns = `id, active_job_id, class_name, queue_name, created_at, scheduled_at, finished_at`

// OperationStatuses ports OperationStatus.call_many.
func OperationStatuses(ctx context.Context, ops []*Operation) []any {
	out := []any{}
	if len(ops) == 0 {
		return out
	}
	var jobIDs []string
	for _, o := range ops {
		if o.ActiveJobID != nil {
			jobIDs = append(jobIDs, *o.ActiveJobID)
		}
	}
	jobs := map[string]*queueJob{}
	var ids []int64
	if len(jobIDs) > 0 {
		rows, err := db.Q().Query(ctx, `SELECT `+jobColumns+` FROM solid_queue_jobs WHERE active_job_id = ANY($1)`, jobIDs)
		must(err)
		for _, j := range scanJobs(rows) {
			jobs[*j.ActiveJobID] = j
		}
		for _, j := range jobs {
			ids = append(ids, j.ID)
		}
	}
	failed := map[int64]*string{}
	claimed := map[int64]bool{}
	if len(ids) > 0 {
		rows, err := db.Q().Query(ctx, `SELECT job_id, error FROM solid_queue_failed_executions WHERE job_id = ANY($1)`, ids)
		must(err)
		for rows.Next() {
			var id int64
			var e *string
			must(rows.Scan(&id, &e))
			failed[id] = e
		}
		rows.Close()
		must(rows.Err())
		rows, err = db.Q().Query(ctx, `SELECT job_id FROM solid_queue_claimed_executions WHERE job_id = ANY($1)`, ids)
		must(err)
		for rows.Next() {
			var id int64
			must(rows.Scan(&id))
			claimed[id] = true
		}
		rows.Close()
		must(rows.Err())
	}
	for _, o := range ops {
		var job *queueJob
		if o.ActiveJobID != nil {
			job = jobs[*o.ActiveJobID]
		}
		var failure *string
		hasFailure := false
		if job != nil {
			failure, hasFailure = failed[job.ID]
		}
		out = append(out, operationPayload(o, job, failure, hasFailure, job != nil && claimed[job.ID]))
	}
	return out
}

func operationPayload(o *Operation, job *queueJob, failure *string, hasFailure, claimed bool) *rb.Map {
	status := o.Status
	switch {
	case hasFailure:
		status = "failed"
	case o.Status != "queued" && o.Status != "running":
	case job != nil && job.FinishedAt != nil:
		status = "completed"
	case job != nil && claimed:
		status = "running"
	default:
		status = "queued"
	}
	var progress any
	if o.TotalItems != 0 {
		progress = math.Min(rb.RoundFloat(float64(o.ProcessedItems)/float64(o.TotalItems)*100, 2), 100.0)
	}
	var errMsg any = rb.Deref(o.ErrorMessage)
	if hasFailure {
		// FailedExecution#error is the JSON coder's value: the parsed hash.
		errMsg = nil
		if failure != nil {
			errMsg = jsonValue([]byte(*failure))
		}
	}
	return rb.M("id", o.ID, "kind", o.Kind, "status", status, "active_job_id", rb.Deref(o.ActiveJobID),
		"prayer_book_code", rb.Deref(o.PrayerBookCode), "parameters", jsonValue(o.Parameters),
		"total_items", o.TotalItems, "processed_items", o.ProcessedItems, "progress_percentage", progress,
		"generated_clips", o.GeneratedClips, "skipped_clips", o.SkippedClips, "failed_items", o.FailedItems,
		"generated_characters", o.GeneratedCharacters, "result", jsonValue(o.Result), "error_message", errMsg,
		"requested_by", rb.Deref(o.RequestedBy), "created_at", rb.FormatTime(o.CreatedAt),
		"started_at", timeOrNil(o.StartedAt), "completed_at", timeOrNil(o.CompletedAt))
}

// JobClasses ports WorkerQueue::JOB_CLASSES.
var JobClasses = []string{"PrewarmOfficeAudioJob", "GenerateBookAudioJob", "RegenerateAudioClipJob", "CleanupAudioClipsJob", "IndexAudioCatalogJob"}

var states = []string{"running", "orphaned", "ready", "scheduled", "blocked", "failed"}

func deadState(s string) bool { return s == "orphaned" || s == "failed" }

// classifiedJob is one WorkerQueue#payload.
type classifiedJob struct {
	job   *queueJob
	state string
}

func idSet(ctx context.Context, table string, ids []int64) map[int64]bool {
	rows, err := db.Q().Query(ctx, `SELECT job_id FROM `+table+` WHERE job_id = ANY($1)`, ids)
	must(err)
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		out[id] = true
	}
	must(rows.Err())
	return out
}

func classified(ctx context.Context) []classifiedJob {
	rows, err := db.Q().Query(ctx, `SELECT `+jobColumns+` FROM "solid_queue_jobs" WHERE "solid_queue_jobs"."class_name" = ANY($1) AND "solid_queue_jobs"."finished_at" IS NULL ORDER BY "solid_queue_jobs"."created_at" DESC`, JobClasses)
	must(err)
	jobs := scanJobs(rows)
	if len(jobs) == 0 {
		return nil
	}
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	claims := map[int64]*int64{}
	crows, err := db.Q().Query(ctx, `SELECT job_id, process_id FROM solid_queue_claimed_executions WHERE job_id = ANY($1)`, ids)
	must(err)
	for crows.Next() {
		var id int64
		var pid *int64
		must(crows.Scan(&id, &pid))
		claims[id] = pid
	}
	crows.Close()
	must(crows.Err())
	ready, scheduled := idSet(ctx, "solid_queue_ready_executions", ids), idSet(ctx, "solid_queue_scheduled_executions", ids)
	blocked, failed := idSet(ctx, "solid_queue_blocked_executions", ids), idSet(ctx, "solid_queue_failed_executions", ids)
	live := map[int64]bool{}
	prows, err := db.Q().Query(ctx, `SELECT id FROM solid_queue_processes WHERE last_heartbeat_at >= $1`, clock.Now().Add(-5*time.Minute))
	must(err)
	for prows.Next() {
		var id int64
		must(prows.Scan(&id))
		live[id] = true
	}
	prows.Close()
	must(prows.Err())
	var out []classifiedJob
	for _, j := range jobs {
		state := "orphaned"
		if pid, ok := claims[j.ID]; ok {
			if pid != nil && live[*pid] {
				state = "running"
			}
		} else if failed[j.ID] {
			state = "failed"
		} else if ready[j.ID] {
			state = "ready"
		} else if scheduled[j.ID] {
			state = "scheduled"
		} else if blocked[j.ID] {
			state = "blocked"
		}
		out = append(out, classifiedJob{j, state})
	}
	return out
}

func jobPayload(c classifiedJob) *rb.Map {
	return rb.M("id", c.job.ID, "active_job_id", rb.Deref(c.job.ActiveJobID), "class_name", c.job.ClassName,
		"queue_name", c.job.QueueName, "created_at", rb.FormatTime(c.job.CreatedAt), "scheduled_at", timeOrNil(c.job.ScheduledAt),
		"state", c.state, "claimed", c.state == "running", "purgeable", deadState(c.state))
}

// WorkerQueue ports Audio::Admin::WorkerQueue.call.
func WorkerQueue(ctx context.Context) *rb.Map {
	jobs := classified(ctx)
	byState := rb.NewMap()
	purgeable := 0
	for _, s := range states {
		n := 0
		for _, j := range jobs {
			if j.state == s {
				n++
			}
		}
		byState.Set(s, n)
	}
	list := []any{}
	for i, j := range jobs {
		if deadState(j.state) {
			purgeable++
		}
		if i < 20 {
			list = append(list, jobPayload(j))
		}
	}
	return rb.M("total", len(jobs), "by_state", byState, "purgeable", purgeable, "jobs", list)
}

var _ = json.Marshal
