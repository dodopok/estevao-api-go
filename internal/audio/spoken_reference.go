package audio

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

type referenceWords struct {
	chapter, chapters, verse, verses, to, toEnd string
}

// referenceVocabulary is Audio::SpokenReference::WORDS. toEnd takes the verse
// through %s (the Ruby template's %<verse>s).
var referenceVocabulary = map[string]referenceWords{
	"pt": {"capítulo", "capítulos", "versículo", "versículos", "a", "do versículo %s ao fim"},
	"en": {"chapter", "chapters", "verse", "verses", "to", "verse %s to the end"},
	"es": {"capítulo", "capítulos", "versículo", "versículos", "a", "del versículo %s al final"},
	"cy": {"pennod", "penodau", "adnod", "adnodau", "i", "adnod %s i'r diwedd"},
}

var romanOrdinals = map[string]string{"I": "1", "II": "2", "III": "3"}

// The Ruby constants interpolate TOKEN, NUMBER and NUMBERS into each other;
// NUMBERS is an /x pattern, written here without its layout whitespace.
const (
	refToken   = `(?:[1-3]|I{1,3}|\p{L}[\p{L}'’-]*\.?)`
	refNumber  = `(?:\d+(?:\s*[:.]\s*\d+)?[a-z]?)`
	refNumbers = `(?:` + refNumber + `(?:\s*(?:[-–—]+\s*(?:` + refNumber + `|end|fim)\b|[,;]\s*` + refNumber +
		`|(?:[,;]\s*)?[(\[][\d\s:.,;\-–—a-z]*[)\]](?:\s*` + refNumber + `)?|\*))*)`
)

var (
	refSpan          = rx.MustCompile(`(?<book>` + refToken + `(?:\s+` + refToken + `){0,5})\s+(?<numbers>` + refNumbers + `)(?![\p{L}\d]|\s+(?:[1-3]\s+)?\p{Lu})`)
	refChapterVerse  = rx.MustCompile(`(?<!\d)(\d{1,3})\s*[:.]\s*(\d{1,3}[a-z]?)(?:\s*[-–—]+\s*(?:(\d{1,3})\s*[:.]\s*)?(\d{1,3}[a-z]?))?(?![\d:]|\.\d)`)
	refVerseRange    = rx.MustCompile(`(?<![\d.:])(\d{1,3}[a-z]?)\s*[-–—]+\s*(\d{1,3}[a-z]?)(?!\d|[.:]\d)`)
	refRomanOrdinal  = rx.MustCompile(`\A(I{1,3})\s+`)
	refPsalmBrackets = rx.MustCompile(`\[([^\]]+)\]`)
	refLeadingComma  = rx.MustCompile(`\A\s*,\s*`)
	refSpaceComma    = rx.MustCompile(`\s+,`)
	refWordSplit     = rx.MustCompile(`\s+`)
)

var psalmsID = bible.BookID("Salmos")

// SpokenReference ports Audio::SpokenReference.call.
func SpokenReference(text, language string) string {
	w, ok := referenceVocabulary[language]
	if !ok {
		w, ok = referenceVocabulary[strings.SplitN(language, "-", 2)[0]]
	}
	if !ok || language == "" {
		return text
	}
	s := &spokenRef{words: w, language: language}
	spoken := refSpan.GsubFunc(text, func(m *rx.Match) string {
		if v, ok := s.speakSpan(m.N("book"), m.N("numbers")); ok {
			return v
		}
		return m.String()
	})
	return s.spellOutLeftovers(spoken)
}

type spokenRef struct {
	words    referenceWords
	language string
}

func (s *spokenRef) speakSpan(book, numbers string) (string, bool) {
	words := refWordSplit.Split(book, 0)
	for index := range words {
		candidate := strings.Join(words[index:], " ")
		id := s.bookID(candidate)
		if id == 0 {
			continue
		}
		if spoken, ok := s.speak(candidate, id, numbers); ok {
			var parts []string
			if prefix := strings.Join(words[:index], " "); !rb.BlankString(prefix) {
				parts = append(parts, prefix)
			}
			if !rb.BlankString(spoken) {
				parts = append(parts, spoken)
			}
			return strings.Join(parts, " "), true
		}
	}
	return "", false
}

func (s *spokenRef) speak(original string, id int, numbers string) (string, bool) {
	canonical := bible.BookNameByID(id)
	psalm := id == psalmsID
	if psalm {
		numbers = refPsalmBrackets.Gsub(numbers, `\1`)
	}
	segments := bible.ParseAll(canonical + " " + numbers)
	if len(segments) == 0 {
		return "", false
	}
	r := &referenceRenderer{words: s.words, book: s.spokenBookName(original, canonical, psalm), psalm: psalm}
	return r.render(segments), true
}

func (s *spokenRef) bookID(name string) int {
	return bible.BookID(strings.TrimSuffix(arabicOrdinal(name), "."))
}

func (s *spokenRef) spokenBookName(original, canonical string, psalm bool) string {
	original = arabicOrdinal(original)
	if psalm {
		return original
	}
	localized := liturgical.TranslateBookName(canonical, s.language)
	if abbreviation(original, localized) {
		return localized
	}
	return original
}

func abbreviation(original, localized string) bool {
	if strings.HasSuffix(original, ".") {
		return true
	}
	printed, full := fold(original), fold(localized)
	return rubyLen(printed) < rubyLen(full) && strings.HasPrefix(full, printed)
}

func fold(name string) string {
	return rb.Strip(strings.ToLower(rb.Transliterate(strings.TrimSuffix(name, "."))))
}

func arabicOrdinal(name string) string {
	return refRomanOrdinal.SubFunc(name, func(m *rx.Match) string { return romanOrdinals[m.G(1)] + " " })
}

func (s *spokenRef) spellOutLeftovers(text string) string {
	spoken := refChapterVerse.GsubFunc(text, func(m *rx.Match) string {
		chapter, verse := m.G(1), m.G(2)
		endChapter, hasEndChapter := m.Group(3)
		endVerse, hasEndVerse := m.Group(4)
		start := s.words.chapter + " " + chapter + ", "
		if !hasEndVerse {
			return ", " + start + s.words.verse + " " + verse
		}
		if hasEndChapter {
			return ", " + start + s.words.verse + " " + verse + " " + s.words.to + " " +
				s.words.chapter + " " + endChapter + ", " + s.words.verse + " " + endVerse
		}
		return ", " + start + s.words.verses + " " + verse + " " + s.words.to + " " + endVerse
	})
	spoken = refVerseRange.GsubFunc(spoken, func(m *rx.Match) string {
		return m.G(1) + " " + s.words.to + " " + m.G(2)
	})
	return refSpaceComma.Gsub(refLeadingComma.Sub(spoken, ""), ",")
}

// --- Audio::SpokenReference::Renderer ----------------------------------------

type unitKind int

const (
	unitVerses unitKind = iota
	unitToEnd
	unitWhole
	unitCross
)

type referenceUnit struct {
	kind                 unitKind
	chapter, endChapter  int // endChapter 0 == nil
	verseStart, verseEnd int // 0 == nil
}

type referenceRenderer struct {
	words referenceWords
	book  string
	psalm bool
}

func (r *referenceRenderer) render(segments []bible.Segment) string {
	var text strings.Builder
	var previous *referenceUnit
	for _, unit := range r.units(segments) {
		unit := unit
		continuation := previous != nil && previous.chapter == unit.chapter && isVerses(previous) && isVerses(&unit)
		text.WriteString(r.joiner(text.Len() == 0, continuation))
		if continuation {
			text.WriteString(r.versesPhrase(&unit, true))
		} else {
			text.WriteString(r.phrase(&unit))
		}
		previous = &unit
	}
	return text.String()
}

func isVerses(u *referenceUnit) bool { return u.kind == unitVerses || u.kind == unitToEnd }

func (r *referenceRenderer) joiner(empty, continuation bool) string {
	if empty && r.psalm {
		return ""
	}
	if empty {
		return r.book + ", "
	}
	if continuation {
		return ", "
	}
	return "; "
}

func whole(s bible.Segment) bool { return s.FetchAll || s.VerseStart == 0 }

func (r *referenceRenderer) units(segments []bible.Segment) []referenceUnit {
	var units []referenceUnit
	for index := 0; index < len(segments); {
		unit, consumed := r.unitAt(segments, index)
		units = append(units, unit)
		index += consumed
	}
	return units
}

func (r *referenceRenderer) unitAt(segments []bible.Segment, index int) (referenceUnit, int) {
	segment := segments[index]
	if whole(segment) {
		return r.wholeRun(segments, index)
	}
	if segment.FetchAllFromVerse {
		return r.openRange(segments, index)
	}
	return referenceUnit{kind: unitVerses, chapter: segment.Chapter, verseStart: segment.VerseStart, verseEnd: segment.VerseEnd}, 1
}

func (r *referenceRenderer) wholeRun(segments []bible.Segment, index int) (referenceUnit, int) {
	last := index
	if !r.psalm {
		last = lastConsecutiveWhole(segments, index)
	}
	u := referenceUnit{kind: unitWhole, chapter: segments[index].Chapter}
	if last > index {
		u.endChapter = segments[last].Chapter
	}
	return u, last - index + 1
}

func lastConsecutiveWhole(segments []bible.Segment, index int) int {
	last := index
	for last+1 < len(segments) && whole(segments[last+1]) && segments[last+1].Chapter == segments[last].Chapter+1 {
		last++
	}
	return last
}

func (r *referenceRenderer) openRange(segments []bible.Segment, index int) (referenceUnit, int) {
	start := segments[index]
	last := lastConsecutiveWhole(segments, index)
	if last+1 < len(segments) {
		finish := segments[last+1]
		if finish.VerseStart == 1 && finish.VerseEnd != 0 && finish.Chapter == segments[last].Chapter+1 {
			return referenceUnit{kind: unitCross, chapter: start.Chapter, endChapter: finish.Chapter,
				verseStart: start.VerseStart, verseEnd: finish.VerseEnd}, last - index + 2
		}
	}
	return referenceUnit{kind: unitToEnd, chapter: start.Chapter, verseStart: start.VerseStart}, 1
}

func (r *referenceRenderer) heading(chapter int) string {
	if r.psalm {
		return r.book + " " + strconv.Itoa(chapter)
	}
	return r.words.chapter + " " + strconv.Itoa(chapter)
}

func (r *referenceRenderer) phrase(u *referenceUnit) string {
	switch u.kind {
	case unitWhole:
		if u.endChapter == 0 {
			return r.heading(u.chapter)
		}
		return r.words.chapters + " " + strconv.Itoa(u.chapter) + " " + r.words.to + " " + strconv.Itoa(u.endChapter)
	case unitCross:
		return r.heading(u.chapter) + ", " + r.words.verse + " " + strconv.Itoa(u.verseStart) + " " + r.words.to + " " +
			r.heading(u.endChapter) + ", " + r.words.verse + " " + strconv.Itoa(u.verseEnd)
	}
	return r.heading(u.chapter) + ", " + r.versesPhrase(u, false)
}

func (r *referenceRenderer) versesPhrase(u *referenceUnit, bare bool) string {
	if u.kind == unitToEnd {
		return fmt.Sprintf(r.words.toEnd, strconv.Itoa(u.verseStart))
	}
	start := strconv.Itoa(u.verseStart)
	switch {
	case bare && u.verseEnd != 0:
		return start + " " + r.words.to + " " + strconv.Itoa(u.verseEnd)
	case bare:
		return start
	case u.verseEnd != 0:
		return r.words.verses + " " + start + " " + r.words.to + " " + strconv.Itoa(u.verseEnd)
	}
	return r.words.verse + " " + start
}
