package audio

import (
	"context"
	"strconv"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Seconds of silence (Audio::TrackBuilder::GAP_*).
const (
	gapLine     = 0.5
	gapSection  = 1.35
	gapResponse = 0.05
)

// Entry ports Audio::OfficeWalker::Entry.
type Entry struct {
	SectionSlug string
	Index       int
	Type        string
	Text        string
	Slug        any
	VerseNumber any
}

// read ports OfficeWalker.read for a serialized office node.
func read(node any, key string) any {
	if m, ok := node.(*rb.Map); ok && m != nil {
		return m.Get(key)
	}
	return nil
}

// EachLine ports Audio::OfficeWalker.each_line over a serialized office.
func EachLine(office *rb.Map) []Entry {
	sections := office.Get("modules")
	if !rb.Truthy(sections) {
		sections = office.Get("sections")
	}
	var out []Entry
	list, _ := sections.([]any)
	for _, section := range list {
		slug := rb.ToS(read(section, "slug"))
		lines, _ := read(section, "lines").([]any)
		for i, line := range lines {
			out = append(out, Entry{
				SectionSlug: slug, Index: i,
				Type: rb.ToS(read(line, "type")), Text: rb.ToS(read(line, "text")),
				Slug: read(line, "slug"), VerseNumber: read(line, "verse_number"),
			})
		}
	}
	return out
}

// Clip ports Audio::ClipGenerator::Clip.
type Clip struct {
	Key            string
	URL            string
	Duration       *float64
	CharacterCount int
}

type storedClip struct {
	filename     string
	duration     *float64
	customDigest *string
}

type customization struct {
	instructionsSHA256 string
}

// Generator ports the read-only side of Audio::ClipGenerator: segments a
// line, addresses each segment and returns the catalogued clip, never
// generating one.
type Generator struct {
	provider  *Provider
	storage   *Storage
	segments  map[[4]string][]string
	preloaded map[string]storedClip
	custom    map[string]customization
	durations map[string]*float64
}

// NewGenerator ports Audio::ClipGenerator.new(provider:).
func NewGenerator(p *Provider) *Generator {
	return &Generator{provider: p, storage: NewStorage(p), segments: map[[4]string][]string{}, durations: map[string]*float64{}}
}

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
	var keys []string
	seen := map[string]bool{}
	for _, e := range entries {
		for _, s := range g.SegmentsFor(e) {
			k := ClipKey(g.provider, s)
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	g.preloaded = map[string]storedClip{}
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

// customizations ports Audio::Customization.active_for(provider:).
func (g *Generator) customizations(ctx context.Context) (map[string]customization, error) {
	if g.custom != nil {
		return g.custom, nil
	}
	rows, err := db.Q().Query(ctx, `SELECT text_digest, instructions_sha256 FROM audio_clip_customizations
		WHERE status = 'active' AND provider = $1 AND model = $2 AND voice = $3 AND language = $4 ORDER BY id`,
		g.provider.Name, g.provider.Model, g.provider.Voice, g.provider.Language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	g.custom = map[string]customization{}
	for rows.Next() {
		var digest string
		var c customization
		if err := rows.Scan(&digest, &c.instructionsSHA256); err != nil {
			return nil, err
		}
		g.custom[digest] = c
	}
	return g.custom, rows.Err()
}

// SegmentClip is one provider-sized segment and its clip (nil when missing).
type SegmentClip struct {
	Text string
	Clip *Clip
}

// CallSegments ports call_segments(..., generate: false).
func (g *Generator) CallSegments(ctx context.Context, e Entry) ([]SegmentClip, error) {
	normalized := g.SegmentsFor(e)
	if len(normalized) == 0 && ContextRequired(e.Text, g.provider.Language) {
		return []SegmentClip{{Text: e.Text}}, nil
	}
	out := make([]SegmentClip, len(normalized))
	for i, n := range normalized {
		clip, err := g.readExisting(ctx, n)
		if err != nil {
			return nil, err
		}
		out[i] = SegmentClip{Text: n, Clip: clip}
	}
	return out, nil
}

func (g *Generator) readExisting(ctx context.Context, normalized string) (*Clip, error) {
	key := ClipKey(g.provider, normalized)
	custom, err := g.customizations(ctx)
	if err != nil {
		return nil, err
	}
	stored, ok := g.preloaded[key]
	if !ok {
		return nil, nil
	}
	if c, has := custom[TextDigest(normalized)]; has && (stored.customDigest == nil || *stored.customDigest != c.instructionsSHA256) {
		return nil, nil
	}
	duration := stored.duration
	if duration == nil {
		if duration, err = g.durationFor(ctx, key, stored.filename); err != nil {
			return nil, err
		}
	}
	return &Clip{Key: key, URL: g.storage.URLFor(stored.filename), Duration: duration, CharacterCount: rubyLen(normalized)}, nil
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

// Track ports Audio::TrackBuilder::Track#to_h.
type Track struct {
	Entries    []*rb.Map
	Duration   float64
	Characters int
	Missing    int
}

// Map renders the track the way the controller merges it.
func (t *Track) Map() *rb.Map {
	entries := make([]any, len(t.Entries))
	for i, e := range t.Entries {
		entries[i] = e
	}
	return rb.M("duration", t.Duration, "characters", t.Characters, "missing", t.Missing,
		"complete", t.Missing == 0, "entries", entries)
}

// ClipKeyRecorder receives each clip placed in the track
// (Audio::UserUsageRecorder::Collector#record).
type ClipKeyRecorder func(key string)

type trackState struct {
	entries             []*rb.Map
	missing, characters int
	elapsed             float64
	section             string
	started             bool
	lastSection         string
	lastIndex           int
	hasLast             bool
	lastType            string
	lastVerseNumber     any
}

// BuildTrack ports Audio::TrackBuilder#call(office, generate: false).
func BuildTrack(ctx context.Context, g *Generator, office *rb.Map, record ClipKeyRecorder) (*Track, error) {
	var entries []Entry
	for _, e := range EachLine(office) {
		if spokenTypes[e.Type] && !rb.BlankString(e.Text) {
			entries = append(entries, e)
		}
	}
	if err := g.Preload(ctx, entries); err != nil {
		return nil, err
	}
	st := &trackState{}
	for _, e := range entries {
		segments, err := g.CallSegments(ctx, e)
		if err != nil {
			return nil, err
		}
		for i, s := range segments {
			if s.Clip == nil {
				st.missing++
				continue
			}
			st.append(e, s.Clip, i, record)
		}
	}
	return &Track{Entries: st.entries, Duration: rb.RoundFloat(st.elapsed, 3), Characters: st.characters, Missing: st.missing}, nil
}

func (st *trackState) append(e Entry, clip *Clip, segmentIndex int, record ClipKeyRecorder) {
	gap := st.gapBefore(e, segmentIndex)
	if gap > 0 {
		st.entries = append(st.entries, rb.M("kind", "silence", "is_navigation_target", false,
			"duration", gap, "start_time", rb.RoundFloat(st.elapsed, 3)))
		st.elapsed += gap
	}
	lineID := e.SectionSlug + "_" + strconv.Itoa(e.Index)
	if segmentIndex > 0 {
		lineID += "_" + strconv.Itoa(segmentIndex)
	}
	var duration any
	if clip.Duration != nil {
		duration = *clip.Duration
	}
	st.entries = append(st.entries, rb.M("kind", "clip", "line_id", lineID, "section", e.SectionSlug, "type", e.Type,
		"is_navigation_target", !skipManualNavigation(e.Text), "url", clip.URL, "duration", duration,
		"start_time", rb.RoundFloat(st.elapsed, 3)))
	if clip.Duration != nil {
		st.elapsed += *clip.Duration
	}
	st.characters += clip.CharacterCount
	st.started = true
	st.section = e.SectionSlug
	st.lastSection, st.lastIndex, st.hasLast = e.SectionSlug, e.Index, true
	st.lastType = e.Type
	st.lastVerseNumber = e.VerseNumber
	if record != nil && !rb.BlankString(clip.Key) {
		record(clip.Key)
	}
}

func (st *trackState) gapBefore(e Entry, segmentIndex int) float64 {
	if !st.started {
		return 0
	}
	if segmentIndex > 0 && st.hasLast && st.lastSection == e.SectionSlug && st.lastIndex == e.Index {
		return 0
	}
	if st.section != e.SectionSlug {
		return gapSection
	}
	if st.verseContinuation(e) {
		return 0
	}
	if responsePair(st.lastType, e.Type) {
		return 0
	}
	if immediateResponse(e.Text) {
		return gapResponse
	}
	return gapLine
}

func (st *trackState) verseContinuation(e Entry) bool {
	if e.Type != "reading_text" || st.lastType != "reading_text" {
		return false
	}
	if rb.Blank(e.VerseNumber) || rb.Blank(st.lastVerseNumber) {
		return false
	}
	return st.hasLast && st.lastIndex+1 == e.Index
}
