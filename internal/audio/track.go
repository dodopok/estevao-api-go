package audio

import (
	"context"
	"strconv"

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

// ClipKeyRecorder receives each clip placed in the track and its place in
// the office (a usage recorder's record(key, source_key:)).
type ClipKeyRecorder func(key, sourceKey string)

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

// BuildTrack ports Audio::TrackBuilder#call(office, generate:, usage_recorder:).
func BuildTrack(ctx context.Context, g *Generator, office *rb.Map, generate bool, record ClipKeyRecorder) (*Track, error) {
	var entries []Entry
	for _, e := range EachLine(office) {
		if spokenTypes[e.Type] && !rb.BlankString(e.Text) {
			entries = append(entries, e)
		}
	}
	var err error
	if generate {
		err = g.PreloadForGeneration(ctx, entries)
	} else {
		err = g.Preload(ctx, entries)
	}
	if err != nil {
		return nil, err
	}
	st := &trackState{}
	for _, e := range entries {
		segments, err := g.CallSegments(ctx, e, generate)
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
		record(clip.Key, e.SectionSlug+":"+strconv.Itoa(e.Index)+":"+rb.ToS(e.VerseNumber)+":"+strconv.Itoa(segmentIndex))
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
