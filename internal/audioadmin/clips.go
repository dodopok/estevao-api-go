package audioadmin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Clip is an audio_clips row.
type Clip struct {
	ID                                                   int64
	Key, Filename, Text, LineType, Kind, Provider, Voice string
	Model, Language                                      string
	Speed                                                float64
	Duration                                             *float64
	CharacterCount                                       int64
	InstructionsSHA, Fingerprint, CustomInstructionsSHA  *string
	CreatedAt, UpdatedAt                                 time.Time
}

// clipColumns is audio_clips.* in table order: a SELECT DISTINCT sorts on
// the selected columns, so ties in ORDER BY come back as Rails' do.
const clipColumns = `audio_clips.id, audio_clips.character_count, audio_clips.configuration_fingerprint, audio_clips.created_at,
 audio_clips.custom_instructions_sha256, audio_clips.duration, audio_clips.filename, audio_clips.instructions_sha256,
 audio_clips.key, audio_clips.kind, audio_clips.language, audio_clips.line_type, audio_clips.model, audio_clips.provider,
 audio_clips.speed, audio_clips.text, audio_clips.updated_at, audio_clips.voice`

func scanClips(rows pgx.Rows) []*Clip {
	defer rows.Close()
	var out []*Clip
	for rows.Next() {
		c := &Clip{}
		must(rows.Scan(&c.ID, &c.CharacterCount, &c.Fingerprint, &c.CreatedAt, &c.CustomInstructionsSHA, &c.Duration,
			&c.Filename, &c.InstructionsSHA, &c.Key, &c.Kind, &c.Language, &c.LineType, &c.Model, &c.Provider, &c.Speed,
			&c.Text, &c.UpdatedAt, &c.Voice))
		out = append(out, c)
	}
	must(rows.Err())
	return out
}

func FindClip(ctx context.Context, id int64) *Clip {
	rows, err := db.Conn(ctx).Query(ctx, `SELECT `+clipColumns+` FROM audio_clips WHERE audio_clips.id = $1 LIMIT 1`, id)
	must(err)
	list := scanClips(rows)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// ProfileClass ports AudioClip#profile_class.
func (c *Clip) ProfileClass(fps map[string]Expectation) string {
	if c.Fingerprint == nil || rb.BlankString(*c.Fingerprint) {
		return "legacy"
	}
	e, ok := fps[c.Language]
	if !ok {
		return "stale"
	}
	want := e.Long
	if e.ShortMax > 0 && c.CharacterCount <= int64(e.ShortMax) {
		want = e.Short
	}
	if want == *c.Fingerprint {
		return "current"
	}
	return "stale"
}

// --- ClipQuery -------------------------------------------------------------------

// ClipQuery ports Audio::Admin::ClipQuery over string-keyed filters.
type ClipQuery struct {
	Filters *rb.Map
}

func (q *ClipQuery) str(k string) string { return rb.ToS(q.Filters.Get(k)) }

func (q *ClipQuery) present(k string) bool { return !rb.BlankString(rb.Strip(q.str(k))) }

var clipKinds = map[string]bool{"line": true, "silence": true}

// where builds the FROM/WHERE of relation (DISTINCT applied by callers).
func (q *ClipQuery) where(ctx context.Context) (string, []any) {
	var conds []string
	var args []any
	add := func(sql string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(sql, "?", "$"+strconv.Itoa(len(args))))
	}
	if clipKinds[q.str("kind")] {
		add(`audio_clips.kind = ?`, q.str("kind"))
	}
	for _, kv := range [][2]string{{"provider", "provider"}, {"model", "model"}, {"voice", "voice"}, {"language", "language"},
		{"speed", "speed"}, {"fingerprint", "configuration_fingerprint"}} {
		if !q.present(kv[0]) {
			continue
		}
		v := q.Filters.Get(kv[0])
		if kv[0] == "speed" {
			v = speedValue(v)
		}
		if list, ok := v.([]any); ok {
			add(`audio_clips.`+kv[1]+` = ANY(?)`, list)
		} else {
			add(`audio_clips.`+kv[1]+` = ?`, v)
		}
	}
	if n, ok := rb.CastInteger(q.Filters.Get("max_characters")); ok && n > 0 && strictInteger(q.Filters.Get("max_characters")) {
		add(`audio_clips.character_count <= ?`, n)
	}
	if q.present("q") {
		add(`audio_clips.text ILIKE ?`, "%"+likeEscape(rb.Strip(q.str("q")))+"%")
	}
	if q.present("created_after") {
		add(`audio_clips.created_at >= ?`, q.Filters.Get("created_after"))
	}
	if q.present("created_before") {
		add(`audio_clips.created_at <= ?`, q.Filters.Get("created_before"))
	}
	from := `audio_clips`
	var usage []string
	for _, k := range []string{"prayer_book_code", "source_name"} {
		if v := q.Filters.Get(k); rb.Present(v) {
			args = append(args, v)
			usage = append(usage, `audio_clip_usages.`+k+` = $`+strconv.Itoa(len(args)))
		}
	}
	if len(usage) > 0 {
		from += ` INNER JOIN audio_clip_usages ON audio_clip_usages.audio_clip_id = audio_clips.id`
		conds = append(conds, usage...)
	}
	switch q.str("profile_status") {
	case "legacy":
		conds = append(conds, `audio_clips.configuration_fingerprint IS NULL`)
	case "current":
		conds = append(conds, CurrentProfileScope(ctx))
	case "stale":
		conds = append(conds, StaleProfileScope(ctx))
	}
	sql := from
	if len(conds) > 0 {
		sql += ` WHERE ` + strings.Join(conds, " AND ")
	}
	return sql, args
}

func speedValue(v any) any {
	if f, ok := v.(float64); ok {
		return f
	}
	if n, ok := v.(int); ok {
		return float64(n)
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(rb.ToS(v)), 64)
	return f
}

// strictInteger reports whether Integer(value, exception: false) parses.
func strictInteger(v any) bool {
	switch x := v.(type) {
	case int, int64:
		return true
	case string:
		_, msg := rb.KernelInteger(x)
		return msg == ""
	}
	return false
}

// likeEscape ports sanitize_sql_like.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

var clipSorts = map[string]string{"created_at": "created_at", "character_count": "character_count", "duration": "duration",
	"provider": "provider", "voice": "voice"}

func (q *ClipQuery) order() string {
	col, ok := clipSorts[q.str("sort")]
	if !ok {
		col = "created_at"
	}
	dir := "DESC"
	if strings.ToLower(q.str("direction")) == "asc" {
		dir = "ASC"
	}
	return "audio_clips." + col + " " + dir + ", audio_clips.id DESC"
}

// Count ports relation.count (COUNT(DISTINCT id)).
func (q *ClipQuery) Count(ctx context.Context) int64 {
	from, args := q.where(ctx)
	var n int64
	must(db.Q().QueryRow(ctx, `SELECT COUNT(DISTINCT audio_clips.id) FROM `+from, args...).Scan(&n))
	return n
}

// Sum ports relation.sum(column) on the distinct relation (SUM(DISTINCT)).
func (q *ClipQuery) Sum(ctx context.Context, column string) any {
	from, args := q.where(ctx)
	var v *float64
	must(db.Q().QueryRow(ctx, `SELECT SUM(DISTINCT audio_clips.`+column+`)::float8 FROM `+from, args...).Scan(&v))
	if v == nil {
		// sum casts `value || 0` through the column type.
		if column == "character_count" {
			return 0
		}
		return 0.0
	}
	if column == "character_count" {
		return int64(*v)
	}
	return *v
}

// Records loads the distinct clips in order.
func (q *ClipQuery) Records(ctx context.Context, order string, limit, offset int) []*Clip {
	from, args := q.where(ctx)
	sql := `SELECT DISTINCT ` + clipColumns + ` FROM ` + from + ` ORDER BY ` + order
	if limit > 0 {
		sql += ` LIMIT ` + strconv.Itoa(limit)
	}
	if offset > 0 {
		sql += ` OFFSET ` + strconv.Itoa(offset)
	}
	rows, err := db.Q().Query(ctx, sql, args...)
	must(err)
	return scanClips(rows)
}

// All loads every clip of the relation by id (find_each order).
func (q *ClipQuery) All(ctx context.Context) []*Clip {
	return q.Records(ctx, "audio_clips.id ASC", 0, 0)
}

func toInt(v any, def int) int {
	if v == nil {
		return def
	}
	return rb.StringToI(rb.ToS(v))
}

// Paginated ports ClipQuery#paginated.
func (q *ClipQuery) Paginated(ctx context.Context) ([]*Clip, int64, int, int) {
	total := q.Count(ctx)
	limit := min(max(toInt(q.Filters.Get("limit"), 25), 1), 100)
	if _, ok := q.Filters.Lookup("limit"); !ok {
		limit = 25
	}
	offset := 0
	if v, ok := q.Filters.Lookup("offset"); ok {
		offset = max(toInt(v, 0), 0)
	}
	return q.Records(ctx, q.order(), limit, offset), total, limit, offset
}

// --- clip payload -------------------------------------------------------------

func textDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func customizationPayload(ctx context.Context, c *Clip) any {
	var instructions, digest string
	err := db.Conn(ctx).QueryRow(ctx, `SELECT instructions, instructions_sha256 FROM audio_clip_customizations
		WHERE status = 'active' AND text_digest = $1 AND provider = $2 AND model = $3 AND voice = $4 AND language = $5 LIMIT 1`,
		textDigest(c.Text), c.Provider, c.Model, c.Voice, c.Language).Scan(&instructions, &digest)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return rb.M("instructions", instructions, "instructions_sha256", digest,
		"applied", c.CustomInstructionsSHA != nil && *c.CustomInstructionsSHA == digest)
}

func candidatesPayload(ctx context.Context, c *Clip) []any {
	rows, err := db.Conn(ctx).Query(ctx, `SELECT id, status, duration, character_count, custom_instructions, filename, provider,
		language, created_at, updated_at FROM audio_clip_candidates WHERE audio_clip_id = $1 ORDER BY created_at DESC LIMIT 5`, c.ID)
	must(err)
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, count int64
		var status, filename, provider, language string
		var duration *float64
		var custom *string
		var created, updated time.Time
		must(rows.Scan(&id, &status, &duration, &count, &custom, &filename, &provider, &language, &created, &updated))
		lang := language
		p := audio.Build(provider, &lang)
		out = append(out, rb.M("id", id, "status", status, "duration", rb.Deref(duration), "character_count", count,
			"custom_instructions", rb.Deref(custom), "audio_url", audio.NewStorage(p).URLFor(filename),
			"created_at", rb.FormatTime(created), "updated_at", rb.FormatTime(updated)))
	}
	must(rows.Err())
	return out
}

func usagesPayload(ctx context.Context, c *Clip, filters *rb.Map) []any {
	sql := `SELECT prayer_book_code, source_name, source_key FROM audio_clip_usages WHERE audio_clip_usages.audio_clip_id = $1`
	args := []any{c.ID}
	if filters != nil {
		// includes(:usages) under a where on audio_clip_usages is an
		// eager_load: only the usages matching the filter are loaded.
		for _, k := range []string{"prayer_book_code", "source_name"} {
			if v := filters.Get(k); rb.Present(v) {
				args = append(args, v)
				sql += ` AND audio_clip_usages.` + k + ` = $` + strconv.Itoa(len(args))
			}
		}
	}
	rows, err := db.Conn(ctx).Query(ctx, sql, args...)
	must(err)
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var code, name, key string
		must(rows.Scan(&code, &name, &key))
		out = append(out, rb.M("prayer_book_code", code, "source_name", name, "source_key", key))
	}
	must(rows.Err())
	return out
}

// ClipPayload ports AudioController#clip_payload.
func ClipPayload(ctx context.Context, c *Clip, fps map[string]Expectation) *rb.Map {
	return ClipPayloadFiltered(ctx, c, fps, nil)
}

// ClipPayloadFiltered is clip_payload for a clip loaded through a ClipQuery
// whose usage filters restrict its eager-loaded usages.
func ClipPayloadFiltered(ctx context.Context, c *Clip, fps map[string]Expectation, filters *rb.Map) *rb.Map {
	if fps == nil {
		_, fps = CurrentFingerprints(ctx)
	}
	return rb.M(
		"id", c.ID, "text", c.Text, "line_type", c.LineType, "kind", c.Kind, "filename", c.Filename,
		"provider", c.Provider, "voice", c.Voice, "model", c.Model, "language", c.Language, "speed", c.Speed,
		"duration", rb.Deref(c.Duration), "character_count", c.CharacterCount,
		"instructions_sha256", rb.Deref(c.InstructionsSHA), "configuration_fingerprint", rb.Deref(c.Fingerprint),
		"custom_instructions_sha256", rb.Deref(c.CustomInstructionsSHA),
		"customization", customizationPayload(ctx, c), "profile_status", c.ProfileClass(fps),
		"created_at", rb.FormatTime(c.CreatedAt), "updated_at", rb.FormatTime(c.UpdatedAt),
		"audio_url", audio.PlainStorage().URLFor(c.Filename),
		"candidates", candidatesPayload(ctx, c), "usages", usagesPayload(ctx, c, filters),
	)
}
