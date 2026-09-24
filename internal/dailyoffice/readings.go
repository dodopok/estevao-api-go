package dailyoffice

import (
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rx"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// --- Components::OfficeLessons ---------------------------------------------------

var officeLessonOptions = map[string][]string{
	"ot_nt": {"first_reading", "second_reading"}, "ot_gospel": {"first_reading", "gospel"},
	"nt_gospel": {"second_reading", "gospel"}, "all": {"first_reading", "second_reading", "gospel"},
	"ot": {"first_reading"}, "nt": {"second_reading"}, "gospel": {"gospel"},
}

// OfficeLessonsDefault ports OfficeLessons.default_for.
func OfficeLessonsDefault(office string) string {
	switch office {
	case "morning":
		return "ot_nt"
	case "evening":
		return "gospel"
	}
	return "ot_nt"
}

// OfficeLessonsFor ports OfficeLessons.for.
func OfficeLessonsFor(sel *reading.Selection, option, office string) []*reading.Passage {
	slots, ok := officeLessonOptions[option]
	if !ok {
		slots = officeLessonOptions[OfficeLessonsDefault(office)]
	}
	var out []*reading.Passage
	for _, s := range slots {
		if p := sel.Slot(s); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// OfficeLessonsInTestament ports OfficeLessons.in_testament.
func OfficeLessonsInTestament(sel *reading.Selection, option, office, testament string) []*reading.Passage {
	wanted := []string{"first_reading"}
	if testament == "new" {
		wanted = []string{"second_reading", "gospel"}
	}
	var out []*reading.Passage
	for _, lesson := range OfficeLessonsFor(sel, option, office) {
		for _, w := range wanted {
			if sel.Slot(w) == lesson {
				out = append(out, lesson)
				break
			}
		}
	}
	return out
}

// --- Concerns::ReadingFormatter --------------------------------------------------------

// Rubrics are the rubric_slugs hash of build_reading_module.
type Rubrics struct{ Pre, Post, Response, End string }

// ReadingModule ports build_reading_module.
func (b *Base) ReadingModule(typ, announcementSlug string, rubrics Rubrics, readingKey, moduleName string, includeReferenceHeading bool) *Section {
	r := b.Readings.Slot(readingKey)
	meta := rb.NewMap()
	if r != nil {
		meta.Set("reference", r.Reference)
		if r.Translation != nil {
			meta.Set("translation", *r.Translation)
		}
		if alt := r.Alternative; alt != nil {
			am := rb.M("reference", alt.Reference, "translation", ptrVal(alt.Translation),
				"lines", b.ReadingLines(alt, announcementSlug, rubrics, readingKey, includeReferenceHeading))
			meta.Set("alternative", am)
		}
	}
	if rb.Present(b.Pref("preferred_audio_voice")) {
		// BibleAudioService is not defined in the application, so no URL.
	}
	return b.Section(moduleName, typ+"_reading", b.ReadingLines(r, announcementSlug, rubrics, readingKey, includeReferenceHeading), meta)
}

func ptrVal(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// ReadingExtras ports reading_module_extras; altLines renders the
// alternative (nil uses format_bible_content).
func (b *Base) ReadingExtras(readingKey string, altLines func(*reading.Passage) []*Line) *rb.Map {
	r := b.Readings.Slot(readingKey)
	extras := rb.NewMap()
	if r == nil {
		return extras
	}
	extras.Set("reference", r.Reference)
	extras.Set("translation", ptrVal(r.Translation))
	if alt := r.Alternative; alt != nil {
		var lines []*Line
		if altLines != nil {
			lines = altLines(alt)
		} else {
			lines = b.BibleContent(alt.Content)
		}
		extras.Set("alternative", rb.M("reference", alt.Reference, "translation", ptrVal(alt.Translation), "lines", lines))
	}
	return extras
}

// SectionWithReadingExtras ports build_section_with_reading_extras.
func (b *Base) SectionWithReadingExtras(name any, slug string, lines []*Line, readingKey string, meta *rb.Map, altLines func(*reading.Passage) []*Line) *Section {
	extras := b.ReadingExtras(readingKey, altLines)
	m := rb.NewMap()
	if meta != nil {
		meta.Each(func(k string, v any) { m.Set(k, v) })
	}
	extras.Each(func(k string, v any) { m.Set(k, v) })
	return b.Section(name, slug, lines, m)
}

// ReadingLines ports format_reading_lines.
func (b *Base) ReadingLines(r *reading.Passage, announcementSlug string, rubrics Rubrics, readingKey string, includeReferenceHeading bool) []*Line {
	var lines []*Line
	if rubrics.Pre != "" {
		if t := b.T(rubrics.Pre); t != nil {
			lines = append(lines, b.I(t.Content, "rubric"), b.Spacer())
		}
	}
	if includeReferenceHeading && r != nil {
		lines = append(lines, b.I(r.Reference, "heading"))
	}
	if t := b.T(announcementSlug); t != nil {
		announcement := b.FormatAnnouncement(t, r, readingKey)
		category := t.Category
		if rb.BlankString(category) {
			category = "text"
		}
		typ := category
		if category == "invocation" {
			typ = "leader"
		}
		lines = append(lines, b.Item(announcement, typ, t.Slug, ""), b.Spacer())
	}
	if r != nil {
		if r.Content != nil {
			lines = append(lines, b.BibleContent(r.Content)...)
		}
		lines = append(lines, b.Spacer())
	}
	if rubrics.Post != "" {
		if t := b.T(rubrics.Post); t != nil {
			lines = append(lines, b.I(t.Content, "rubric"), b.Spacer())
		}
	}
	if rubrics.Response != "" {
		if t := b.T(rubrics.Response); t != nil {
			lines = append(lines, b.I(t.Content, "responsive"))
		}
	}
	if rubrics.End != "" {
		if t := b.T(rubrics.End); t != nil {
			lines = append(lines, b.Spacer(), b.I(t.Content, "rubric"))
		}
	}
	return lines
}

// BibleContent ports format_bible_content.
func (b *Base) BibleContent(content *rb.Map) []*Line {
	if content == nil {
		return nil
	}
	verses, ok := content.Get("verses").([]any)
	if !ok || verses == nil {
		return nil
	}
	lines := make([]*Line, 0, len(verses))
	for _, v := range verses {
		vm := v.(*rb.Map)
		meta := rb.M("verse_number", vm.Get("number"))
		if cn := vm.Get("chapter_number"); rb.Present(cn) {
			meta.Set("chapter_number", cn)
		}
		lines = append(lines, b.Line(SanitizeBibleText(rb.ToS(vm.Get("text"))), "reading_text", meta))
	}
	return lines
}

var (
	strongNumbersRe  = rx.MustCompile(`<S>.*?<\/S>`, "i")
	publisherNotesRe = rx.MustCompile(`<sup>.*?<\/sup>`, "im")
	whitespaceRe     = rx.MustCompile(`\s+`)
)

// SanitizeBibleText ports Bible::TextSanitizer.call.
func SanitizeBibleText(text string) string {
	s := strongNumbersRe.Gsub(text, "")
	s = publisherNotesRe.Gsub(s, "")
	s = whitespaceRe.Gsub(s, " ")
	return rb.Strip(s)
}

// FormatAnnouncement ports format_reading_announcement.
func (b *Base) FormatAnnouncement(t *store.LiturgicalText, r *reading.Passage, readingKey string) string {
	if r == nil {
		s := strings.ReplaceAll(t.Content, "{{book_name}}", "_______________")
		s = strings.ReplaceAll(s, "{{chapter}}", "___")
		return strings.ReplaceAll(s, "{{verse}}", "___")
	}
	verses := VerseRange(r)
	s := resolveChapterVerseAnnouncement(t.Content, r, readingKey)
	s = strings.ReplaceAll(s, "{{book_name}}", ptrString(r.BookName))
	chapter := ""
	if r.Chapter != nil {
		chapter = strconv.Itoa(*r.Chapter)
	}
	s = strings.ReplaceAll(s, "{{chapter}}", chapter)
	s = strings.ReplaceAll(s, "{{verse}}", verses)
	return resolveReadingTypeAlternatives(s, r)
}

// SubstituteReadingPlaceholders ports substitute_reading_placeholders.
func SubstituteReadingPlaceholders(text string, r *reading.Passage) string {
	if rb.BlankString(text) {
		return text
	}
	if r == nil {
		s := strings.ReplaceAll(text, "{{book_name}}", "_______________")
		s = strings.ReplaceAll(s, "{{chapter}}", "___")
		return strings.ReplaceAll(s, "{{verse}}", "___")
	}
	s := strings.ReplaceAll(text, "{{book_name}}", ptrString(r.BookName))
	chapter := ""
	if r.Chapter != nil {
		chapter = strconv.Itoa(*r.Chapter)
	}
	s = strings.ReplaceAll(s, "{{chapter}}", chapter)
	s = strings.ReplaceAll(s, "{{verse}}", VerseRange(r))
	return resolveReadingTypeAlternatives(s, r)
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// VerseRange ports build_verse_range.
func VerseRange(r *reading.Passage) string {
	switch {
	case r.VerseStart != nil && r.VerseEnd != nil:
		return strconv.Itoa(*r.VerseStart) + "-" + strconv.Itoa(*r.VerseEnd)
	case r.VerseStart != nil:
		return strconv.Itoa(*r.VerseStart)
	}
	return ""
}

var (
	chapterVerseRe   = rx.MustCompile(`Aqui começa o capítulo \{\{chapter\}\} \(ou o versículo \{\{verse\}\} do capítulo \{\{chapter\}\}\)`, "i")
	firstOrSecondRe  = rx.MustCompile(`Primeira \(ou a Segunda\) Lição`, "i")
	altPT            = rx.MustCompile(`\bno\s+Livro\s+\(\s*Epístola\s+ou\s+Evangelho\s*\)\s+de`, "i")
	altES            = rx.MustCompile(`\ben\s+el\s+Libro\s+\(\s*Epístola\s+o\s+Evangelio\s*\)\s+de`, "i")
	altEN            = rx.MustCompile(`\bthe\s+Book\s+\(\s*Epistle\s+or\s+Gospel\s*\)\s+of`, "i")
	altPT2           = rx.MustCompile(`\bna\s+Epístola\s+\(\s*ou\s+Evangelho\s*\)`, "i")
	altES2           = rx.MustCompile(`\ben\s+la\s+Epístola\s+\(\s*o\s+Evangelio\s*\)`, "i")
	labelAltRe       = rx.MustCompile(`\bEpístola\s*\(\s*(?:ou|o)\s+(?:Evangelho|Evangelio)\s*\)`, "i")
	labelAltENRe     = rx.MustCompile(`\bEpistle\s*\(\s*or\s+Gospel\s*\)`, "i")
	langESRe         = rx.MustCompile(`\b(?:Evangelio|Libro)\b`, "i")
	langPTRe         = rx.MustCompile(`\b(?:Evangelho|Epístola|Livro)\b`, "i")
	langENRe         = rx.MustCompile(`\b(?:Gospel|Epistle|Book)\b`, "i")
	gospelNameRe     = rx.MustCompile(`\b(?:Mateus|Marcos|Lucas|João|Mateo|Juan|Matthew|Mark|Luke|John)\b`, "i")
	readingTypeLabel = map[string]map[string]string{
		"pt": {"gospel": "Evangelho", "epistle": "Epístola", "other": "Livro"},
		"es": {"gospel": "Evangelio", "epistle": "Epístola", "other": "Libro"},
		"en": {"gospel": "Gospel", "epistle": "Epistle", "other": "Book"},
	}
)

func resolveChapterVerseAnnouncement(text string, r *reading.Passage, readingKey string) string {
	if !chapterVerseRe.MatchString(text) {
		return text
	}
	chapter := ""
	if r.Chapter != nil {
		chapter = strconv.Itoa(*r.Chapter)
	}
	beginning := "o capítulo " + chapter
	if r.VerseStart != nil && *r.VerseStart > 1 {
		beginning = "o versículo " + strconv.Itoa(*r.VerseStart) + " do capítulo " + chapter
	}
	ordinal := "Primeira"
	if readingKey == "second_reading" {
		ordinal = "Segunda"
	}
	s := chapterVerseRe.GsubFunc(text, func(*rx.Match) string { return "Aqui começa " + beginning })
	return firstOrSecondRe.GsubFunc(s, func(*rx.Match) string { return ordinal + " Lição" })
}

func resolveReadingTypeAlternatives(text string, r *reading.Passage) string {
	kind := ReadingKind(r)
	if kind == "" {
		return text
	}
	label := readingTypeLabelFor(text, kind)
	if label == "" {
		return text
	}
	phrases := []struct {
		re  *rx.Regexp
		out map[string]string
	}{
		{altPT, map[string]string{"gospel": "no Evangelho de", "epistle": "na Epístola de", "other": "no Livro de"}},
		{altES, map[string]string{"gospel": "en el Evangelio de", "epistle": "en la Epístola de", "other": "en el Libro de"}},
		{altEN, map[string]string{"gospel": "the Gospel of", "epistle": "the Epistle of", "other": "the Book of"}},
		{altPT2, map[string]string{"gospel": "no Evangelho", "epistle": "na Epístola", "other": "no Livro"}},
		{altES2, map[string]string{"gospel": "en el Evangelio", "epistle": "en la Epístola", "other": "en el Libro"}},
	}
	for _, p := range phrases {
		if p.re.MatchString(text) {
			return p.re.GsubFunc(text, func(*rx.Match) string { return p.out[kind] })
		}
	}
	s := labelAltRe.GsubFunc(text, func(*rx.Match) string { return label })
	return labelAltENRe.GsubFunc(s, func(*rx.Match) string { return label })
}

func readingTypeLabelFor(template, kind string) string {
	lang := ""
	switch {
	case langESRe.MatchString(template):
		lang = "es"
	case langPTRe.MatchString(template):
		lang = "pt"
	case langENRe.MatchString(template):
		lang = "en"
	}
	return readingTypeLabel[lang][kind]
}

// ReadingKind ports reading_kind ("" == nil).
func ReadingKind(r *reading.Passage) string {
	id := readingBookID(r)
	switch {
	case id >= 40 && id <= 43:
		return "gospel"
	case id >= 45 && id <= 65:
		return "epistle"
	case id > 0:
		return "other"
	}
	ref := ""
	if r != nil {
		ref = r.Reference
	}
	if gospelNameRe.MatchString(ref) {
		return "gospel"
	}
	return ""
}

// GospelReading ports gospel_reading?.
func GospelReading(r *reading.Passage) bool { return ReadingKind(r) == "gospel" }

func readingBookID(r *reading.Passage) int {
	if r == nil {
		return 0
	}
	id := 0
	if r.BookName != nil && !rb.BlankString(*r.BookName) {
		id = bible.BookID(*r.BookName)
	}
	if id == 0 {
		if p := bible.Parse(r.Reference); p != nil {
			id = bible.BookID(p.Book)
		}
	}
	return id
}
