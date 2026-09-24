package bible

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// Verse is a bible_texts row.
type Verse struct {
	Book       string
	BookNumber int
	Chapter    int
	Verse      int
	Text       string
}

// Passage ports Bible::Passage.
type Passage struct {
	Reference   string
	Translation string
	Verses      []Verse
}

// Available ports available?.
func (p *Passage) Available() bool { return p != nil && len(p.Verses) > 0 }

// Structured ports Bible::PassageFormatter.structured.
func (p *Passage) Structured() *rb.Map {
	if !p.Available() {
		return nil
	}
	verses := make([]any, len(p.Verses))
	for i, v := range p.Verses {
		verses[i] = rb.M("number", v.Verse, "text", v.Text)
	}
	return rb.M("reference", rb.Squish(p.Reference), "translation", p.Translation, "verses", verses)
}

type pair struct {
	book    string
	chapter int
}

// segmentCache is the per-request cache of loaded chapters.
type SegmentCache struct {
	mu sync.Mutex
	m  map[string][]Verse
}

// NewSegmentCache builds an empty per-request cache.
func NewSegmentCache() *SegmentCache { return &SegmentCache{m: map[string][]Verse{}} }

func loadSegments(ctx context.Context, cache *SegmentCache, segs []Segment, translation string) ([]Verse, error) {
	var pairs []pair
	seen := map[pair]bool{}
	for _, s := range segs {
		p := pair{s.Book, s.Chapter}
		if !seen[p] {
			seen[p] = true
			pairs = append(pairs, p)
		}
	}
	sortedPairs := append([]pair(nil), pairs...)
	sort.Slice(sortedPairs, func(i, j int) bool {
		if sortedPairs[i].book != sortedPairs[j].book {
			return sortedPairs[i].book < sortedPairs[j].book
		}
		return sortedPairs[i].chapter < sortedPairs[j].chapter
	})
	var key strings.Builder
	key.WriteString(translation)
	for _, p := range sortedPairs {
		key.WriteString("|" + p.book + "#" + strconv.Itoa(p.chapter))
	}
	if cache != nil {
		cache.mu.Lock()
		if v, ok := cache.m[key.String()]; ok {
			cache.mu.Unlock()
			return v, nil
		}
		cache.mu.Unlock()
	}
	var sql string
	var args []any
	if len(pairs) == 1 {
		sql = `SELECT book, book_number, chapter, verse, text FROM bible_texts WHERE book = $1 AND chapter = $2 AND translation = $3 ORDER BY verse ASC`
		args = []any{pairs[0].book, pairs[0].chapter, translation}
	} else {
		var or []string
		args = []any{translation}
		for _, p := range pairs {
			args = append(args, p.book, p.chapter)
			or = append(or, "(book = $"+strconv.Itoa(len(args)-1)+" AND chapter = $"+strconv.Itoa(len(args))+")")
		}
		sql = `SELECT book, book_number, chapter, verse, text FROM bible_texts WHERE translation = $1 AND (` +
			strings.Join(or, " OR ") + `) ORDER BY book_number ASC, chapter ASC, verse ASC`
	}
	rows, err := db.Q().Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Verse
	for rows.Next() {
		var v Verse
		if err := rows.Scan(&v.Book, &v.BookNumber, &v.Chapter, &v.Verse, &v.Text); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cache != nil {
		cache.mu.Lock()
		cache.m[key.String()] = out
		cache.mu.Unlock()
	}
	return out, nil
}

func inSegment(v int, s Segment) bool {
	switch {
	case s.FetchAll:
		return true
	case s.FetchAllFromVerse && s.VerseStart != 0:
		return v >= s.VerseStart
	case s.VerseStart != 0 && s.VerseEnd != 0:
		return v >= s.VerseStart && v <= s.VerseEnd
	case s.VerseStart != 0:
		return v == s.VerseStart
	}
	return true
}

func selectVerses(segs []Segment, records []Verse) []Verse {
	var out []Verse
	for _, s := range segs {
		for _, v := range records {
			if v.Book == s.Book && v.Chapter == s.Chapter && inSegment(v.Verse, s) {
				out = append(out, v)
			}
		}
	}
	return out
}

// FetchBatch ports Bible::PassageQuery#fetch_batch.
func FetchBatch(ctx context.Context, cache *SegmentCache, refs []string, translation string) (map[string]*Passage, error) {
	type entry struct {
		ref  string
		segs []Segment
	}
	var valid []entry
	seen := map[string]bool{}
	var all []Segment
	for _, r := range refs {
		if rb.BlankString(r) || seen[r] {
			continue
		}
		seen[r] = true
		segs := ParseAll(r)
		if len(segs) == 0 {
			continue
		}
		valid = append(valid, entry{r, segs})
		all = append(all, segs...)
	}
	if len(valid) == 0 {
		return map[string]*Passage{}, nil
	}
	records, err := loadSegments(ctx, cache, all, translation)
	if err != nil {
		return nil, err
	}
	out := map[string]*Passage{}
	for _, e := range valid {
		out[e.ref] = &Passage{Reference: e.ref, Translation: translation, Verses: selectVerses(e.segs, records)}
	}
	return out, nil
}

// Fetch ports Bible::PassageQuery#fetch.
func Fetch(ctx context.Context, cache *SegmentCache, ref, translation string) (*Passage, error) {
	segs := ParseAll(ref)
	if len(segs) == 0 {
		return nil, nil
	}
	records, err := loadSegments(ctx, cache, segs, translation)
	if err != nil {
		return nil, err
	}
	return &Passage{Reference: ref, Translation: translation, Verses: selectVerses(segs, records)}, nil
}

// --- BibleTextService -------------------------------------------------------

var deuterocanonicalFallbacks = map[string]string{"en": "kjva", "pt-BR": "bjrd", "pt-PT": "bpt"}

var chapterAlignments = map[string]map[string]string{
	"bpt": {"Números 16:36-50": "Números 17:1-15", "Joel 3:9-17": "Joel 4:9-17", "Malaquias 4:1-6": "Malaquias 3:19-24"},
}

// VersionInfo is what the text service needs from bible_versions.
type VersionInfo struct {
	Language            *string
	VersificationSystem string
}

// VersionLookup resolves a Bible version by code (nil when absent).
var VersionLookup func(ctx context.Context, code string) (*VersionInfo, error)

// TextService ports BibleTextService.
type TextService struct {
	Translation         string
	SourceVersification string
	Cache               *SegmentCache
	langResolved        bool
	lang                *string
}

// NewTextService builds the service (translation is downcased like Rails).
func NewTextService(translation string, cache *SegmentCache) *TextService {
	return &TextService{Translation: strings.ToLower(translation), Cache: cache}
}

func (s *TextService) resolveTranslation(ctx context.Context, reference string) (string, error) {
	p := Parse(reference)
	if p == nil {
		return s.Translation, nil
	}
	if IsDeuterocanonical(p.Book) {
		if !s.langResolved {
			s.langResolved = true
			if VersionLookup != nil {
				v, err := VersionLookup(ctx, s.Translation)
				if err != nil {
					return "", err
				}
				if v != nil {
					s.lang = v.Language
				}
			}
		}
		if s.lang != nil {
			if f, ok := deuterocanonicalFallbacks[*s.lang]; ok {
				return f, nil
			}
		}
		return "bjrd", nil
	}
	return s.Translation, nil
}

func (s *TextService) convertVersification(ctx context.Context, reference, translation string) (string, error) {
	if a, ok := chapterAlignments[translation][reference]; ok {
		return a, nil
	}
	if rb.BlankString(s.SourceVersification) || VersionLookup == nil {
		return reference, nil
	}
	v, err := VersionLookup(ctx, translation)
	if err != nil || v == nil || rb.BlankString(v.VersificationSystem) {
		return reference, err
	}
	return ConvertVersification(reference, s.SourceVersification, v.VersificationSystem), nil
}

// FetchPassagesBatch ports fetch_passages_batch: original reference ->
// structured passage (only available ones).
func (s *TextService) FetchPassagesBatch(ctx context.Context, refs []string) (map[string]*rb.Map, error) {
	type prep struct{ original, converted, translation string }
	var prepared []prep
	seen := map[string]int{}
	for _, orig := range refs {
		if rb.BlankString(orig) {
			continue
		}
		normalized := Normalize(orig)
		tr, err := s.resolveTranslation(ctx, normalized)
		if err != nil {
			return nil, err
		}
		conv, err := s.convertVersification(ctx, normalized, tr)
		if err != nil {
			return nil, err
		}
		if i, ok := seen[orig]; ok {
			prepared[i] = prep{orig, conv, tr}
			continue
		}
		seen[orig] = len(prepared)
		prepared = append(prepared, prep{orig, conv, tr})
	}
	var order []string
	groups := map[string][]prep{}
	for _, p := range prepared {
		if _, ok := groups[p.translation]; !ok {
			order = append(order, p.translation)
		}
		groups[p.translation] = append(groups[p.translation], p)
	}
	out := map[string]*rb.Map{}
	for _, tr := range order {
		entries := groups[tr]
		var keys []string
		for _, e := range entries {
			keys = append(keys, e.converted)
		}
		passages, err := FetchBatch(ctx, s.Cache, keys, tr)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if st := passages[e.converted].Structured(); st != nil {
				out[e.original] = st
			}
		}
	}
	return out, nil
}

// FetchPassageStructured ports fetch_passage_structured.
func (s *TextService) FetchPassageStructured(ctx context.Context, reference string) (*rb.Map, error) {
	ref := Normalize(reference)
	tr, err := s.resolveTranslation(ctx, ref)
	if err != nil {
		return nil, err
	}
	ref, err = s.convertVersification(ctx, ref, tr)
	if err != nil {
		return nil, err
	}
	p, err := Fetch(ctx, s.Cache, ref, tr)
	if err != nil {
		return nil, err
	}
	return p.Structured(), nil
}

// FetchPassage returns the passage (for text/html formatting).
func (s *TextService) FetchPassage(ctx context.Context, reference string) (*Passage, error) {
	ref := Normalize(reference)
	tr, err := s.resolveTranslation(ctx, ref)
	if err != nil {
		return nil, err
	}
	ref, err = s.convertVersification(ctx, ref, tr)
	if err != nil {
		return nil, err
	}
	return Fetch(ctx, s.Cache, ref, tr)
}

// --- versification ----------------------------------------------------------

var psalmBookNames = []string{"Salmo", "Salmos", "Psalm", "Psalms", "Sl", "Ps"}

// ConvertVersification ports BibleVersificationMapper#convert.
func ConvertVersification(reference, from, to string) string {
	if from == to || rb.BlankString(reference) {
		return reference
	}
	if !(from == "protestant" || from == "catholic") || !(to == "protestant" || to == "catholic") {
		return reference
	}
	isPsalm := false
	for _, n := range psalmBookNames {
		if rx.MustCompile(`\b`+n+`\b`, "i").MatchString(reference) {
			isPsalm = true
			break
		}
	}
	if !isPsalm {
		return reference
	}
	segs := ParseAll(reference)
	if len(segs) == 0 || segs[0].Book != "Salmos" {
		return reference
	}
	p := segs[0]
	ch, vs, ve := p.Chapter, p.VerseStart, p.VerseEnd
	if from == "protestant" {
		switch {
		case ch >= 1 && ch <= 8, ch >= 148 && ch <= 150:
		case ch == 9 || ch == 10:
			ch = 9
		case ch >= 11 && ch <= 113, ch >= 117 && ch <= 146:
			ch--
		case ch == 114 || ch == 115:
			ch = 113
		case ch == 116:
			return formatSplit(p.Book, vs, ve, 9, 114, 115)
		case ch == 147:
			return formatSplit(p.Book, vs, ve, 11, 146, 147)
		}
		return formatRef(p.Book, ch, vs, ve, true)
	}
	switch {
	case ch >= 1 && ch <= 8, ch >= 148 && ch <= 150, ch == 9:
	case ch >= 10 && ch <= 112, ch >= 116 && ch <= 145:
		ch++
	case ch == 113:
		ch = 114
	case ch == 114:
		ch = 116
	case ch == 115:
		ch = 116
		if vs != 0 {
			vs += 9
		} else {
			vs = 10
		}
		if ve != 0 {
			ve += 9
		}
	case ch == 146:
		ch = 147
	case ch == 147:
		if vs != 0 {
			vs += 11
		} else {
			vs = 12
		}
		if ve != 0 {
			ve += 11
		}
	}
	return formatRef(p.Book, ch, vs, ve, true)
}

func formatSplit(book string, vs, ve, split, first, second int) string {
	if vs == 0 {
		return formatRef(book, first, 0, 0, true) + ", " + formatRef(book, second, 0, 0, false)
	}
	if ve == 0 {
		if vs <= split {
			return formatRef(book, first, vs, 0, true)
		}
		return formatRef(book, second, vs-split, 0, true)
	}
	if ve <= split {
		return formatRef(book, first, vs, ve, true)
	}
	if vs > split {
		return formatRef(book, second, vs-split, ve-split, true)
	}
	return formatRef(book, first, vs, split, true) + ", " + formatRef(book, second, 1, ve-split, false)
}

func formatRef(book string, ch, vs, ve int, includeBook bool) string {
	prefix := ""
	if includeBook {
		prefix = book + " "
	}
	switch {
	case vs == 0:
		return prefix + strconv.Itoa(ch)
	case ve == 0 || vs == ve:
		return prefix + strconv.Itoa(ch) + ":" + strconv.Itoa(vs)
	}
	return prefix + strconv.Itoa(ch) + ":" + strconv.Itoa(vs) + "-" + strconv.Itoa(ve)
}
