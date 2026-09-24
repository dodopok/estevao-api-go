package bible

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// Segment is one parsed passage segment (Bible::ReferenceParser hash).
type Segment struct {
	Book              string
	Chapter           int
	VerseStart        int // 0 == nil
	VerseEnd          int // 0 == nil
	FetchAll          bool
	FetchAllFromVerse bool
}

// --- ReferenceNormalizer ---------------------------------------------------

var singleChapterBooksOnce sync.Once
var singleChapterBooksList []string

func singleChapterBooks() []string {
	singleChapterBooksOnce.Do(func() { singleChapterBooksList = computeSingleChapterBooks() })
	return singleChapterBooksList
}

func computeSingleChapterBooks() []string {
	ids := map[int]bool{}
	for _, n := range []string{"Obadiah", "Philemon", "2 John", "3 John", "Jude", "Song of the Three Holy Children", "Susanna", "Bel and the Dragon", "Prayer of Manasseh"} {
		if id := BookID(n); id != 0 {
			ids[id] = true
		}
	}
	seen := map[string]bool{}
	var names []string
	for _, p := range bookPairs {
		if ids[books[p.name]] && !seen[p.name] {
			seen[p.name] = true
			names = append(names, p.name)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		li, lj := len([]rune(names[i])), len([]rune(names[j]))
		if li != lj {
			return li > lj
		}
		return names[i] < names[j]
	})
	return names
}

// Normalize ports Bible::ReferenceNormalizer.call.
func Normalize(reference string) string {
	if rb.BlankString(reference) {
		return reference
	}
	n := NormalizePortugueseChapterComma(reference)
	n = rx.MustCompile(`^(I{1,3})\s+`).GsubFunc(n, func(m *rx.Match) string {
		return map[string]string{"I": "1 ", "II": "2 ", "III": "3 "}[rb.Strip(m.String())]
	})
	for _, book := range singleChapterBooks() {
		esc := rx.Quote(book)
		if rx.MustCompile(`^` + esc + `$`).MatchString(n) {
			n = book + " 1"
			break
		}
		if !rx.MustCompile(`^` + esc + `\s+\d`).MatchString(n) {
			continue
		}
		if m := rx.MustCompile(`^` + esc + `\s+(\d+):(\d+)\z`).Find(n); m != nil && rb.StringToI(m.G(1)) != 1 {
			n = book + " 1." + m.G(1) + "-" + m.G(2)
			break
		}
		if rx.MustCompile(`^` + esc + `\s+\d+[:.]`).MatchString(n) {
			break
		}
		n = rx.MustCompile(`^(`+esc+`)\s+(\d)`).Sub(n, `\1 1.\2`)
		break
	}
	return n
}

var reChapterBefore = rx.MustCompile(`(\d+)\s*[:.]\s*\d+[^:.]*\z`)

// NormalizePortugueseChapterComma ports normalize_portuguese_chapter_comma.
func NormalizePortugueseChapterComma(reference string) string {
	return rx.MustCompile(`-(\d+),(\d+[a-z]?)(?=\s*(?:,|;|\z))`, "i").GsubFunc(reference, func(m *rx.Match) string {
		pre := m.PreMatch()
		cm := reChapterBefore.Find(pre)
		if cm != nil && rb.StringToI(m.G(1)) > rb.StringToI(cm.G(1)) {
			return "-" + m.G(1) + ":" + m.G(2)
		}
		return m.String()
	})
}

// --- ReferenceParser -------------------------------------------------------

var (
	repeatedBookRef     = rx.MustCompile(`\A(?<book>\d*\s*[^\d:.\-]+?)\s+(?<reference>\d.*)\z`)
	continuationPrefix  = rx.MustCompile(`\A(?:and|&)\s+(?:(?<whole_chapter>chapter|ch\.?)\s+)?`, "i")
	alternativeSegment  = rx.MustCompile(`\A(?:or|ou|o)\s+`, "i")
	alternativeSuffix   = rx.MustCompile(`\s+(?:or|ou|o)\s+|\s+/\s+`, "i")
	unambiguousAlt      = rx.MustCompile(`\s+(?:or|ou)\s+`, "i")
	ambiguousAlt        = rx.MustCompile(`\s+o\s+|\s*/\s*`, "i")
	danglingConjunction = rx.MustCompile(`\s+(?:or|ou|o|and|e|&|/)\s*\z`, "i")
	footnoteMarker      = rx.MustCompile(`[\*\x{2020}\x{2021}<>\x{00a7}\x{00b6}]+\s*\z`)
	bookNameOnly        = rx.MustCompile(`\A\d?\s*\p{L}[\p{L}\s.]*\z`)
	leadingOptional     = rx.MustCompile(`\A(?<book>\d*\s*[^\d(]+?)\s*\((?<optional>\d+(?:[.:]\d+)?(?:-\d+(?:[.:]\d+)?)?)\)\s*[;,]?\s*(?<main>\d.*)`)
)

// ParseAll ports Bible::ReferenceParser.parse_all.
func ParseAll(reference string) []Segment {
	if rb.BlankString(reference) {
		return nil
	}
	clean := Normalize(rb.Strip(reference))
	clean = rx.MustCompile(`[–—]`).Gsub(clean, "-")
	clean = rx.MustCompile(`-{2,}`).Gsub(clean, "-")
	clean = rb.Strip(footnoteMarker.Sub(clean, ""))
	clean = rb.Strip(danglingConjunction.Sub(clean, ""))

	lo := leadingOptional.SubFunc(clean, func(m *rx.Match) string {
		book := rb.Strip(m.N("book"))
		optional := m.N("optional")
		main := m.N("main")
		if rx.MustCompile(`\A\d+(?:-\d+)?\s*(?:\(|\z)`).MatchString(main) {
			chapter := ""
			if mm := rx.MustCompile(`\A\d+[.:]\d+-(\d+)[.:]\d+\z`).Find(optional); mm != nil {
				chapter = mm.G(1)
			} else if mm := rx.MustCompile(`\A(\d+)[.:]\d+`).Find(optional); mm != nil {
				chapter = mm.G(1)
			}
			if chapter != "" {
				main = chapter + ":" + main
			}
		}
		return book + " " + main
	})
	if lo != clean {
		clean = Normalize(lo)
	}
	clean = rx.MustCompile(`\s*-\s*`).Gsub(clean, "-")
	clean = NormalizePortugueseChapterComma(clean)
	clean = rx.MustCompile(`:\s+(\d)`).Gsub(clean, `:\1`)
	clean = rx.MustCompile(`(\p{L})(\d+[:.]\d)`).Gsub(clean, `\1 \2`)
	clean = rx.MustCompile(`-(\d+)\s+(end|fim)\b`, "i").Gsub(clean, `-\1:\2`)
	clean = rx.MustCompile(`-(\d+)-(end|fim)\b`, "i").Gsub(clean, `-\1:\2`)
	clean = rx.MustCompile(`\A(?<book>\d*\s*[^\d:.]+?)\s+(?<start>\d+)-(?<end_chapter>\d+)(?<separator>[:.])(?<end_verse>\d+)\z`).
		Sub(clean, `\k<book> \k<start>:1-\k<end_chapter>\k<separator>\k<end_verse>`)
	clean = rx.MustCompile(`\s*&\s*`).Gsub(clean, "; ")
	clean = rx.MustCompile(`^(I{1,3})\s+`).GsubFunc(clean, func(m *rx.Match) string {
		return map[string]string{"I": "1 ", "II": "2 ", "III": "3 "}[rb.Strip(m.String())]
	})
	clean = firstAlternative(clean)
	if bookNameOnly.MatchString(clean) {
		clean = Normalize(clean)
	}
	clean = rx.MustCompile(`\d+[:.]\s*\([^)]*\)\s*[;,]\s*(?=\d+[:.]\d)`).Gsub(clean, "")
	clean = rx.MustCompile(`(\d+)([:.])\s*\([^)]*\)\s*,?\s*(\d)`).Gsub(clean, `\1\2\3`)
	clean = rx.MustCompile(`(\d+)([:.])\s*\[[^\]]*\]\s*,?\s*(\d)`).Gsub(clean, `\1\2\3`)
	clean = rx.MustCompile(`(\d+):\s*\[[^\]]+\]\s*(\d)`).Gsub(clean, `\1:\2`)
	clean = rx.MustCompile(`(\d+):\s*\([^)]+\)\s*(\d)`).Gsub(clean, `\1:\2`)
	clean = rx.MustCompile(`(\d+)\.\(([^)]+)\)(\d)`).Gsub(clean, `\1:\3`)
	clean = rx.MustCompile(`\[([^\]]+)\]\s*(\d+)`).Gsub(clean, `,\2`)
	clean = rx.MustCompile(`\(([^)]+)\)\s*(\d+)`).Gsub(clean, `,\2`)
	clean = rx.MustCompile(`\s*\([^)]+\)`).Gsub(clean, "")
	clean = rx.MustCompile(`\s*\[[^\]]+\]`).Gsub(clean, "")
	clean = rx.MustCompile(`\.\s+(\d)`).Gsub(clean, ` \1`)

	first := rx.MustCompile(`^(\d*\s*[^\d:.]+)\s+\d`).Find(clean)
	if first == nil {
		return nil
	}
	baseBook := rb.Strip(first.G(1))
	if !rx.MustCompile(`[[:alpha:]]`).MatchString(baseBook) {
		return nil
	}
	clean = rx.MustCompile(`(\d[a-z]?|end)\s+(and|&)\s+(?=(?:chapter\s|ch\.?\s)?\d)`, "i").Gsub(clean, `\1, \2 `)
	clean = rx.MustCompile(`(\d[a-z]?|end)\s+(and|&)\s+(?=\p{L})`, "i").GsubFunc(clean, func(m *rx.Match) string {
		following := ""
		if fm := rx.MustCompile(`\A(\d*\s*\p{L}[^\d,;]*)`).Find(m.PostMatch()); fm != nil {
			following = rb.Strip(fm.G(1))
		}
		if BookID(following) != 0 {
			return m.G(1) + ", " + m.G(2) + " "
		}
		return m.String()
	})

	segments := rb.StripAll(rx.MustCompile(`[,;]`).Split(clean, 0))
	var refs []Segment
	for index, segment := range segments {
		if bookNameOnly.MatchString(segment) {
			segment = Normalize(segment)
		}
		segment = rx.MustCompile(`(\d+)[a-z]`).Gsub(segment, `\1`)
		if index == 0 {
			whole := rx.MustCompile(`^(\d*\s*[^\d:.]+?)\s*(\d+)-(\d+)$`).Find(segment)
			cross := rx.MustCompile(`^(\d*\s*[^\d:.]+?)\s*(\d+)[:.](\d+)-(\d+)[:.](\d+|end|fim)$`).Find(segment)
			switch {
			case whole != nil:
				book := rb.Strip(whole.G(1))
				for ch := rb.StringToI(whole.G(2)); ch <= rb.StringToI(whole.G(3)); ch++ {
					refs = append(refs, build(book, ch, "", "", true, false))
				}
			case cross != nil:
				book := rb.Strip(cross.G(1))
				sc, sv, ec, ev := rb.StringToI(cross.G(2)), rb.StringToI(cross.G(3)), rb.StringToI(cross.G(4)), cross.G(5)
				refs = append(refs, build(book, sc, strconv.Itoa(sv), "", false, true))
				for ch := sc + 1; ch < ec; ch++ {
					refs = append(refs, build(book, ch, "", "", true, false))
				}
				if lower := strings.ToLower(ev); lower == "end" || lower == "fim" {
					refs = append(refs, build(book, ec, "", "", true, false))
				} else {
					refs = append(refs, build(book, ec, "1", ev, false, false))
				}
			default:
				m := rx.MustCompile(`^(\d*\s*[^\d:.]+?)\s*(\d+)(?:[:.](\d+)(?:-(\w+))?)?$`).Find(segment)
				if m == nil {
					continue
				}
				book := rb.Strip(m.G(1))
				g4, has4 := m.Group(4)
				g3, _ := m.Group(3)
				if has4 && (strings.ToLower(g4) == "end" || strings.ToLower(g4) == "fim") {
					refs = append(refs, build(book, rb.StringToI(m.G(2)), g3, "", false, true))
				} else {
					refs = append(refs, build(book, rb.StringToI(m.G(2)), g3, g4, false, false))
				}
			}
			continue
		}
		if alternativeSegment.MatchString(segment) {
			break
		}
		var wholeChapterContinuation bool
		segment, wholeChapterContinuation = stripContinuationPrefix(segment)
		segment = trimAlternativeSuffix(segment)
		segmentBook := baseBook
		explicit := repeatedBookReference(segment)
		if explicit != nil {
			segmentBook = rb.Strip(explicit.N("book"))
			segment = explicit.N("reference")
		}
		if cross := rx.MustCompile(`^(\d+)[:.](\d+)-(\d+)[:.](\d+|end|fim)$`).Find(segment); cross != nil {
			sc, sv, ec, ev := rb.StringToI(cross.G(1)), rb.StringToI(cross.G(2)), rb.StringToI(cross.G(3)), cross.G(4)
			refs = append(refs, build(segmentBook, sc, strconv.Itoa(sv), "", false, true))
			for ch := sc + 1; ch < ec; ch++ {
				refs = append(refs, build(segmentBook, ch, "", "", true, false))
			}
			if lower := strings.ToLower(ev); lower == "end" || lower == "fim" {
				refs = append(refs, build(segmentBook, ec, "", "", true, false))
			} else {
				refs = append(refs, build(segmentBook, ec, "1", ev, false, false))
			}
		} else if rx.MustCompile(`^\d+[:.]`).MatchString(segment) {
			m := rx.MustCompile(`^(\d+)[:.](\d+)(?:-(\d+|end|fim))?$`, "i").Find(segment)
			if m == nil {
				continue
			}
			g3, has3 := m.Group(3)
			if has3 && (strings.ToLower(g3) == "end" || strings.ToLower(g3) == "fim") {
				refs = append(refs, build(segmentBook, rb.StringToI(m.G(1)), m.G(2), "", false, true))
			} else {
				refs = append(refs, build(segmentBook, rb.StringToI(m.G(1)), m.G(2), g3, false, false))
			}
		} else if rx.MustCompile(`^\d+-(?:\d+|end|fim)$`, "i").MatchString(segment) {
			m := rx.MustCompile(`^(\d+)-(\d+|end|fim)$`, "i").Find(segment)
			if m == nil {
				continue
			}
			prev := 1
			if len(refs) > 0 {
				prev = refs[len(refs)-1].Chapter
			}
			if l := strings.ToLower(m.G(2)); l == "end" || l == "fim" {
				refs = append(refs, build(segmentBook, prev, m.G(1), "", false, true))
			} else {
				refs = append(refs, build(segmentBook, prev, m.G(1), m.G(2), false, false))
			}
		} else if rx.MustCompile(`^\d+$`).MatchString(segment) {
			prevWhole := false
			if len(refs) > 0 {
				last := refs[len(refs)-1]
				prevWhole = last.VerseStart == 0 && last.VerseEnd == 0
			}
			if explicit != nil || wholeChapterContinuation || prevWhole {
				refs = append(refs, build(segmentBook, rb.StringToI(segment), "", "", false, false))
			} else {
				prev := 1
				if len(refs) > 0 {
					prev = refs[len(refs)-1].Chapter
				}
				refs = append(refs, build(segmentBook, prev, segment, "", false, false))
			}
		}
	}
	return refs
}

// Parse returns the first segment (nil when unparseable).
func Parse(reference string) *Segment {
	refs := ParseAll(reference)
	if len(refs) == 0 {
		return nil
	}
	return &refs[0]
}

func firstAlternative(reference string) string {
	parts := unambiguousAlt.Split(reference, 0)
	ref := ""
	if len(parts) > 0 {
		ref = rb.Strip(parts[0])
	}
	offset := 0
	for {
		m := ambiguousAlt.FindFrom(ref, offset)
		if m == nil {
			break
		}
		prefix := string([]rune(ref)[:m.Begin()])
		if rx.MustCompile(`\d`).MatchString(prefix) {
			return rb.Strip(prefix)
		}
		offset = m.End()
		if m.End() == m.Begin() {
			offset++
		}
	}
	return ref
}

func repeatedBookReference(segment string) *rx.Match {
	m := repeatedBookRef.Find(segment)
	if m == nil || BookID(m.N("book")) == 0 {
		return nil
	}
	return m
}

func stripContinuationPrefix(segment string) (string, bool) {
	m := continuationPrefix.Find(segment)
	if m == nil {
		return segment, false
	}
	wc, ok := m.Named("whole_chapter")
	rest := string([]rune(segment)[len([]rune(m.String())):])
	return rb.Strip(rest), ok && !rb.BlankString(wc)
}

func trimAlternativeSuffix(segment string) string {
	if !rx.MustCompile(`\A\d`).MatchString(segment) {
		return segment
	}
	parts := alternativeSuffix.Split(segment, 2)
	if len(parts) == 0 {
		return ""
	}
	return rb.Strip(parts[0])
}

func build(book string, chapter int, vs, ve string, fetchAll, fromVerse bool) Segment {
	var start, end int
	hasStart, hasEnd := vs != "", ve != ""
	if hasStart {
		start = rb.StringToI(vs)
	}
	if hasEnd {
		end = rb.StringToI(ve)
	}
	if hasStart && hasEnd && end < start {
		start, end = end, start
	}
	return Segment{Book: CanonicalBookName(book), Chapter: chapter, VerseStart: start, VerseEnd: end, FetchAll: fetchAll, FetchAllFromVerse: fromVerse}
}
