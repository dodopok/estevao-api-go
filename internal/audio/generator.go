package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// ProfileVersion is Audio::Profile::VERSION.
const ProfileVersion = "v2"

// InstructionsSHA256 ports Audio::Profile#instructions_sha256.
func (p *Provider) InstructionsSHA256() string {
	sum := sha256.Sum256([]byte(p.Instructions))
	return hex.EncodeToString(sum[:])
}

// ConfigurationFingerprint ports Audio::Profile#configuration_fingerprint(normalized).
func (p *Provider) ConfigurationFingerprint(normalized *string) string {
	sum := sha256.Sum256([]byte(ProfileVersion + "\x00" + p.Voice + "\x00" + p.CacheSignature(normalized)))
	return hex.EncodeToString(sum[:])
}

// ProfileMap ports Audio::Profile#to_h.
func (p *Provider) ProfileMap() *rb.Map {
	return rb.M("provider", p.Name, "model", p.Model, "voice", p.Voice, "language", p.Language, "speed", p.Speed,
		"instructions_sha256", p.InstructionsSHA256(), "configuration_fingerprint", p.ConfigurationFingerprint(nil))
}

// Clip ports Audio::ClipGenerator::Clip.
type Clip struct {
	Key            string
	URL            string
	Duration       *float64
	CharacterCount int
	Generated      bool
}

type storedClip struct {
	filename     string
	duration     *float64
	customDigest *string
}

// Customization is an active AudioClipCustomization of the provider profile.
type Customization struct {
	Instructions       string
	InstructionsSHA256 string
}

// BudgetExceeded ports Audio::BudgetExceeded.
type BudgetExceeded struct{ Message string }

func (e *BudgetExceeded) Error() string     { return e.Message }
func (e *BudgetExceeded) RubyClass() string { return "Audio::BudgetExceeded" }

// Budget is a character_budget argument: nil, or its #to_i and #to_s.
type Budget struct {
	Limit int64
	Text  string
}

// NewBudget ports a character_budget value (nil means no ceiling).
func NewBudget(v any) *Budget {
	if v == nil {
		return nil
	}
	return &Budget{Limit: int64(rb.ToI(v)), Text: rb.ToS(v)}
}

// Generator ports Audio::ClipGenerator: segments a line, addresses each
// segment, and returns the catalogued clip or, when generating, buys it.
type Generator struct {
	provider      *Provider
	storage       *Storage
	segments      map[[4]string][]string
	preloaded     map[string]storedClip
	preloadedKeys map[string]bool
	trust         bool
	reuse         bool
	custom        map[string]Customization
	durations     map[string]*float64
	// Budget is character_budget (nil: no ceiling).
	Budget *Budget
	// Progress is the progress_callback, called after each recorded clip.
	Progress            func(g *Generator)
	CharactersGenerated int64
	GeneratedClips      int64
}

// NewGenerator ports Audio::ClipGenerator.new(provider:).
func NewGenerator(p *Provider) *Generator {
	return &Generator{provider: p, storage: NewStorage(p), segments: map[[4]string][]string{}, durations: map[string]*float64{}}
}

// NewReusingGenerator ports ClipGenerator.new(reuse_preloaded: true): the
// ledger preloaded by successive calls accumulates instead of being replaced.
func NewReusingGenerator(p *Provider) *Generator {
	g := NewGenerator(p)
	g.reuse = true
	g.preloadedKeys = map[string]bool{}
	return g
}

// Provider is the generator's speech backend.
func (g *Generator) Provider() *Provider { return g.provider }

// SegmentsFor ports segments_for.
func (g *Generator) SegmentsFor(e Entry) []string {
	if ContextRequired(e.Text, g.provider.Language) {
		return nil
	}
	key := [4]string{e.Text, e.Type, rb.ToS(e.Slug), rb.Inspect(e.VerseNumber)}
	if s, ok := g.segments[key]; ok {
		return s
	}
	normalized := Normalize(e.Text, e.Type, rb.ToS(e.Slug), e.VerseNumber, g.provider.Language)
	var segments []string
	if !rb.BlankString(normalized) {
		segments = Segment(normalized, g.provider.MaxInputCharacters(), g.provider.SplitTerminalResponse)
	}
	g.segments[key] = segments
	return segments
}

// Preload ports preload(entries): one ledger query for every clip the
// entries address.
func (g *Generator) Preload(ctx context.Context, entries []Entry) error {
	return g.preload(ctx, entries, false)
}

// PreloadForGeneration ports preload_for_generation: the same query, and the
// ledger is then trusted without probing storage.
func (g *Generator) PreloadForGeneration(ctx context.Context, entries []Entry) error {
	return g.preload(ctx, entries, true)
}

func (g *Generator) preload(ctx context.Context, entries []Entry, trust bool) error {
	if !g.reuse {
		g.segments = map[[4]string][]string{}
	}
	g.trust = trust
	var keys []string
	seen := map[string]bool{}
	for _, e := range entries {
		for _, s := range g.SegmentsFor(e) {
			k := ClipKey(g.provider, s)
			if !seen[k] && !(g.reuse && g.preloadedKeys[k]) {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	if !g.reuse || g.preloaded == nil {
		g.preloaded = map[string]storedClip{}
	}
	for _, k := range keys {
		if g.reuse {
			g.preloadedKeys[k] = true
		}
	}
	if len(keys) == 0 {
		return nil
	}
	rows, err := db.Q().Query(ctx,
		`SELECT key, filename, duration, custom_instructions_sha256 FROM audio_clips WHERE key = ANY($1) AND kind = 'line'`, keys)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var c storedClip
		if err := rows.Scan(&key, &c.filename, &c.duration, &c.customDigest); err != nil {
			return err
		}
		g.preloaded[key] = c
	}
	return rows.Err()
}

// customizations ports Audio::Customization.active_for(provider:), indexed
// by text digest.
func (g *Generator) customizations(ctx context.Context) (map[string]Customization, error) {
	if g.custom != nil {
		return g.custom, nil
	}
	rows, err := db.Q().Query(ctx, `SELECT text_digest, instructions, instructions_sha256 FROM audio_clip_customizations
		WHERE status = 'active' AND provider = $1 AND model = $2 AND voice = $3 AND language = $4 ORDER BY id`,
		g.provider.Name, g.provider.Model, g.provider.Voice, g.provider.Language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	g.custom = map[string]Customization{}
	for rows.Next() {
		var digest string
		var instructions *string
		var c Customization
		if err := rows.Scan(&digest, &instructions, &c.InstructionsSHA256); err != nil {
			return nil, err
		}
		if instructions != nil {
			c.Instructions = *instructions
		}
		g.custom[digest] = c
	}
	return g.custom, rows.Err()
}

func (g *Generator) customizationFor(ctx context.Context, normalized string) (*Customization, error) {
	custom, err := g.customizations(ctx)
	if err != nil {
		return nil, err
	}
	if c, ok := custom[TextDigest(normalized)]; ok {
		return &c, nil
	}
	return nil, nil
}

// SegmentClip is one provider-sized segment and its clip (nil when missing).
type SegmentClip struct {
	Text string
	Clip *Clip
}

// CallSegments ports call_segments(..., generate:).
func (g *Generator) CallSegments(ctx context.Context, e Entry, generate bool) ([]SegmentClip, error) {
	normalized := g.SegmentsFor(e)
	if len(normalized) == 0 && ContextRequired(e.Text, g.provider.Language) {
		return []SegmentClip{{Text: e.Text}}, nil
	}
	out := make([]SegmentClip, len(normalized))
	for i, n := range normalized {
		clip, err := g.callNormalized(ctx, n, e.Type, generate)
		if err != nil {
			return nil, err
		}
		out[i] = SegmentClip{Text: n, Clip: clip}
	}
	return out, nil
}

func (g *Generator) callNormalized(ctx context.Context, normalized, lineType string, generate bool) (*Clip, error) {
	key := ClipKey(g.provider, normalized)
	filename := g.storage.FilenameFor(key)
	custom, err := g.customizationFor(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if !generate {
		return g.readExisting(ctx, key, normalized, custom)
	}
	if g.trust {
		if stored, ok := g.preloaded[key]; ok && reusableDigest(custom, stored.customDigest) {
			return g.clipFor(ctx, key, stored.filename, normalized, false, stored.duration)
		}
	}
	var clip *Clip
	err = WithGenerationLock(ctx, key, func() error {
		if g.storage.Exists(ctx, filename) {
			reusable, err := g.reusable(ctx, key, custom)
			if err != nil {
				return err
			}
			if reusable {
				clip, err = g.clipFor(ctx, key, filename, normalized, false, nil)
				return err
			}
		}
		length := int64(rubyLen(normalized))
		if g.Budget != nil && g.CharactersGenerated+length > g.Budget.Limit {
			return &BudgetExceeded{Message: "stopped before generation: " + strconv.FormatInt(g.CharactersGenerated+length, 10) +
				" characters exceeds the budget of " + g.Budget.Text}
		}
		duration, err := g.synthesize(ctx, normalized, filename, custom)
		if err != nil {
			return err
		}
		g.CharactersGenerated += length
		if err := g.record(ctx, key, filename, normalized, lineType, duration, custom); err != nil {
			return err
		}
		g.remember(key, filename, duration, custom)
		clip, err = g.clipFor(ctx, key, filename, normalized, true, duration)
		return err
	})
	return clip, err
}

func reusableDigest(custom *Customization, stored *string) bool {
	return custom == nil || stored != nil && *stored == custom.InstructionsSHA256
}

// reusable ports reusable_clip?(key, customization) with the digest not loaded.
func (g *Generator) reusable(ctx context.Context, key string, custom *Customization) (bool, error) {
	if custom == nil {
		return true, nil
	}
	var digest *string
	err := db.Q().QueryRow(ctx, `SELECT custom_instructions_sha256 FROM audio_clips WHERE key = $1 LIMIT 1`, key).Scan(&digest)
	if err != nil && !db.NoRows(err) {
		return false, err
	}
	return reusableDigest(custom, digest), nil
}

// readExisting ports read_existing_clip against the preloaded ledger.
func (g *Generator) readExisting(ctx context.Context, key, normalized string, custom *Customization) (*Clip, error) {
	stored, ok := g.preloaded[key]
	if !ok || !reusableDigest(custom, stored.customDigest) {
		return nil, nil
	}
	return g.clipFor(ctx, key, stored.filename, normalized, false, stored.duration)
}

// clipFor ports clip_for.
func (g *Generator) clipFor(ctx context.Context, key, filename, normalized string, generated bool, duration *float64) (*Clip, error) {
	if duration == nil {
		var err error
		if duration, err = g.durationFor(ctx, key, filename); err != nil {
			return nil, err
		}
	}
	return &Clip{Key: key, URL: g.storage.URLFor(filename), Duration: duration, CharacterCount: rubyLen(normalized), Generated: generated}, nil
}

// durationFor ports duration_for(key, filename).
func (g *Generator) durationFor(ctx context.Context, key, filename string) (*float64, error) {
	if d, ok := g.durations[key]; ok {
		return d, nil
	}
	var d *float64
	err := db.Q().QueryRow(ctx, `SELECT duration FROM audio_clips WHERE key = $1 LIMIT 1`, key).Scan(&d)
	if err != nil && !db.NoRows(err) {
		return nil, err
	}
	if d == nil {
		d = g.storage.DurationFor(filename)
	}
	g.durations[key] = d
	return d, nil
}

// synthesize ports synthesize(normalized, filename, customization:): the
// provider call, the duration read from the bytes, then the upload.
func (g *Generator) synthesize(ctx context.Context, normalized, filename string, custom *Customization) (*float64, error) {
	var additional string
	if custom != nil && !rb.BlankString(custom.Instructions) {
		if !g.provider.SupportsCustomInstructions() {
			return nil, &rb.RubyError{Class: "ArgumentError", Message: g.provider.Name + " does not support per-clip audio instructions"}
		}
		additional = custom.Instructions
	}
	data, err := g.provider.Synthesize(ctx, normalized, additional)
	if err != nil {
		return nil, err
	}
	duration := Mp3Duration(data)
	if err := g.storage.Store(ctx, filename, data); err != nil {
		return nil, err
	}
	return duration, nil
}

// record ports record(...): AudioClip.find_or_initialize_by(key:) assigned
// and saved. Bookkeeping is best effort: an invalid or duplicate row is
// logged, not raised.
func (g *Generator) record(ctx context.Context, key, filename, normalized, lineType string, duration *float64, custom *Customization) error {
	p := g.provider
	for _, v := range []string{p.Name, p.Voice, p.Model, p.Language} {
		if rb.BlankString(v) {
			slog.Warn("Could not record audio clip " + key + ": ActiveRecord::RecordInvalid")
			return nil
		}
	}
	var customDigest *string
	if custom != nil {
		customDigest = &custom.InstructionsSHA256
	}
	instructions := p.InstructionsSHA256()
	fingerprint := p.ConfigurationFingerprint(&normalized)
	count := rubyLen(normalized)
	now := time.Now().UTC().Truncate(time.Microsecond)
	tag, err := db.Q().Exec(ctx, `UPDATE audio_clips SET filename = $2::varchar, text = $3::text, line_type = $4::varchar,
		kind = 'line', provider = $5::varchar, voice = $6::varchar, model = $7::varchar, speed = $8::float8, duration = $9::float8,
		character_count = $10::int, language = $11::varchar, instructions_sha256 = $12::varchar,
		configuration_fingerprint = $13::varchar, custom_instructions_sha256 = $14::varchar,
		updated_at = CASE WHEN (filename, text, line_type, kind, provider, voice, model, speed, character_count, language)
			IS DISTINCT FROM ($2::varchar, $3::text, $4::varchar, 'line', $5::varchar, $6::varchar, $7::varchar, $8::float8, $10::int, $11::varchar)
			OR duration IS DISTINCT FROM $9::float8 OR instructions_sha256 IS DISTINCT FROM $12::varchar
			OR configuration_fingerprint IS DISTINCT FROM $13::varchar OR custom_instructions_sha256 IS DISTINCT FROM $14::varchar
			THEN $15::timestamp ELSE updated_at END
		WHERE key = $1::varchar`, key, filename, normalized, lineType, p.Name, p.Voice, p.Model, p.Speed, duration, count, p.Language,
		instructions, fingerprint, customDigest, now)
	if err == nil && tag.RowsAffected() == 0 {
		_, err = db.Q().Exec(ctx, `INSERT INTO audio_clips (key, filename, text, line_type, kind, provider, voice, model, speed,
			duration, character_count, language, instructions_sha256, configuration_fingerprint, custom_instructions_sha256,
			created_at, updated_at) VALUES ($1, $2, $3, $4, 'line', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)`,
			key, filename, normalized, lineType, p.Name, p.Voice, p.Model, p.Speed, duration, count, p.Language,
			instructions, fingerprint, customDigest, now)
	}
	if err != nil {
		if db.UniqueViolation(err) {
			slog.Warn("Could not record audio clip " + key + ": ActiveRecord::RecordNotUnique")
			return nil
		}
		return err
	}
	g.GeneratedClips++
	if g.Progress != nil {
		g.Progress(g)
	}
	return nil
}

// remember ports remember_clip.
func (g *Generator) remember(key, filename string, duration *float64, custom *Customization) {
	if g.preloaded == nil {
		return
	}
	if g.reuse {
		g.preloadedKeys[key] = true
	}
	var digest *string
	if custom != nil {
		d := custom.InstructionsSHA256
		digest = &d
	}
	g.preloaded[key] = storedClip{filename: filename, duration: duration, customDigest: digest}
}

// WithGenerationLock ports Audio::GenerationLock.with(key): a session-level
// advisory lock held on one connection for the critical section.
func WithGenerationLock(ctx context.Context, key string, f func() error) error {
	sum := sha256.Sum256([]byte("audio-clip:" + key))
	id, _ := strconv.ParseInt(hex.EncodeToString(sum[:])[:15], 16, 64)
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, id); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, id) }()
	return f()
}
