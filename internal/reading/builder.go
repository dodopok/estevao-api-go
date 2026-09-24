package reading

import (
	"context"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// --- Reading::AlternativeParser -------------------------------------------

var (
	psalmNumberPart     = rx.MustCompile(`\A\d+(?:[.:](\d+)(-\d+)?)?\z`)
	optionalPsalmRe     = rx.MustCompile(`\A(?:Psalm|Psalms|Salmo|Salmos)\b\s*\[`, "i")
	bracketGroupRe      = rx.MustCompile(`\[([^\]]+)\]`)
	leadingOptionalRe   = rx.MustCompile(`(\d+)([:.])\s*\[[^\]]+\]\s*,?\s*(\d)`)
	optionalBeforeNumRe = rx.MustCompile(`\[[^\]]+\]\s*(\d+)`)
	optionalRe          = rx.MustCompile(`\[[^\]]+\]`)
	digitOuRe           = rx.MustCompile(`(\d)ou\b`, "i")
	ampersandRe         = rx.MustCompile(`\s*,?\s*&\s*`)
	spacesRe            = rx.MustCompile(`\s+`)
	pluralCumulativeRe  = rx.MustCompile(`\A(Salmos?|Psalms?)\s+(.+)\z`, "i")
	pluralRe            = rx.MustCompile(`\A(Salmos|Psalms)\s+(.+)\z`, "i")
	prefixedPsalmRe     = rx.MustCompile(`\A(Salmo|Psalm)\s+(.+)\z`, "i")
	chapterSepRe        = rx.MustCompile(`[.:]`)
	chapterStartRe      = rx.MustCompile(`\A\d+[.:]`)
	psalmPartCharsRe    = rx.MustCompile(`\A[\d:.()\-]+\z`)
	slashRe             = rx.MustCompile(`/`)
	slashSplitRe        = rx.MustCompile(`\s*/\s*`)
	orOuSplitRe         = rx.MustCompile(`\s+(?:or|ou)\s+`, "i")
	numericRefRe        = rx.MustCompile(`\A\d+(?:[.:]\d+)?(?:-\d+)?\z`)
	psalmPrefixRe       = rx.MustCompile(`\A(?:Psalm|Psalms|Salmo|Salmos)\b`, "i")
	bookNameRe          = rx.MustCompile(`\A(\d*\s*[^\d:.]+?)\s+\d`)
	psalmWordRe         = rx.MustCompile(`\APsalms?\b`, "i")
	orOuSlashRe         = rx.MustCompile(`\s+(?:or|ou|\/)\s+`, "i")
	firstNumberRe       = rx.MustCompile(`(\d+)`)
)

// AlternativeParser ports Reading::AlternativeParser.
type AlternativeParser struct {
	PsalmPrefix     string
	CumulativeLists bool
}

// Call ports #call.
func (a *AlternativeParser) Call(reference string) []string {
	var ref string
	if optionalPsalmRe.MatchString(reference) {
		ref = a.StripOptionalMarkers(reference)
	} else {
		ref = normalizeSpacing(reference)
	}
	if plural := a.PluralPrefixedPsalmList(ref); plural != nil {
		return plural
	}
	if a.PsalmNumberList(ref) {
		return a.ExpandPsalmNumberList(ref)
	}
	if psalmNumberPart.MatchString(rb.Strip(ref)) {
		return []string{a.PsalmPrefix + " " + rb.Strip(ref)}
	}
	if prefixed := a.splitPrefixedPsalmList(ref); prefixed != nil {
		return prefixed
	}
	parts := splitOuterAlternatives(ref)
	if len(parts) == 0 {
		rb.RaiseNoMethodOnNil("split")
	}
	primaryParts := orOuSplitRe.Split(parts[0], 2)
	if len(primaryParts) == 0 {
		rb.RaiseNoMethodOnNil("strip")
	}
	primary := rb.Strip(primaryParts[0])
	out := []string{primary}
	for _, alt := range parts[1:] {
		out = append(out, a.normalizeAlternative(alt, primary)...)
	}
	return out
}

// StripOptionalMarkers ports strip_optional_markers. Blank input is
// returned unchanged.
func (a *AlternativeParser) StripOptionalMarkers(reference string) string {
	if rb.BlankString(reference) {
		return reference
	}
	var normalized string
	if optionalPsalmRe.MatchString(reference) {
		normalized = bracketGroupRe.Gsub(reference, `\1`)
	} else {
		normalized = leadingOptionalRe.Gsub(reference, `\1\2\3`)
		normalized = optionalBeforeNumRe.Gsub(normalized, `,\1`)
		normalized = optionalRe.Gsub(normalized, "")
	}
	return normalizeSpacing(normalized)
}

// PsalmNumberList ports psalm_number_list?.
func (a *AlternativeParser) PsalmNumberList(reference string) bool {
	parts := rb.StripAll(rb.SplitString(reference, ","))
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if !psalmNumberPart.MatchString(p) {
			return false
		}
	}
	return true
}

// ExpandPsalmNumberList ports expand_psalm_number_list.
func (a *AlternativeParser) ExpandPsalmNumberList(reference string) []string {
	parts := rb.SplitString(reference, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = a.PsalmPrefix + " " + rb.Strip(p)
	}
	return out
}

// PluralPrefixedPsalmList ports plural_prefixed_psalm_list (nil when not a
// cumulative list).
func (a *AlternativeParser) PluralPrefixedPsalmList(reference string) []string {
	re := pluralRe
	if a.CumulativeLists {
		re = pluralCumulativeRe
	}
	m := re.Find(reference)
	if m == nil {
		return nil
	}
	parts := rb.StripAll(rb.SplitString(m.G(2), ","))
	if len(parts) < 2 {
		return nil
	}
	for _, p := range parts {
		if !psalmNumberPart.MatchString(p) {
			return nil
		}
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = m.G(1) + " " + p
	}
	return out
}

func normalizeSpacing(reference string) string {
	s := digitOuRe.Gsub(reference, `\1 ou`)
	s = ampersandRe.Gsub(s, ", ")
	s = spacesRe.Gsub(s, " ")
	return rb.Strip(s)
}

func (a *AlternativeParser) splitPrefixedPsalmList(reference string) []string {
	m := prefixedPsalmRe.Find(reference)
	if m == nil {
		return nil
	}
	parts := rb.StripAll(rb.SplitString(m.G(2), ","))
	if len(parts) == 0 {
		rb.RaiseNoMethodOnNil("match?")
	}
	distinct := !chapterSepRe.MatchString(parts[0])
	if !distinct {
		distinct = true
		for _, p := range parts[1:] {
			if !chapterStartRe.MatchString(p) {
				distinct = false
				break
			}
		}
	}
	if len(parts) < 2 {
		return nil
	}
	for _, p := range parts {
		if !psalmPartCharsRe.MatchString(p) {
			return nil
		}
	}
	if !distinct {
		return nil
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = m.G(1) + " " + p
	}
	return out
}

func splitOuterAlternatives(reference string) []string {
	if slashRe.MatchString(reference) {
		return rb.StripAll(slashSplitRe.Split(reference, 2))
	}
	return rb.StripAll(orOuSplitRe.Split(reference, 0))
}

func (a *AlternativeParser) normalizeAlternative(alternative, primary string) []string {
	if numericReference(alternative) {
		if book, ok := bibleBookName(primary); ok {
			return []string{book + " " + alternative}
		}
	}
	if a.PsalmNumberList(alternative) {
		return a.ExpandPsalmNumberList(alternative)
	}
	if psalmNumberPart.MatchString(alternative) {
		return []string{a.PsalmPrefix + " " + alternative}
	}
	return []string{alternative}
}

func numericReference(reference string) bool {
	for _, p := range rb.StripAll(rb.SplitString(reference, ",")) {
		if !numericRefRe.MatchString(p) {
			return false
		}
	}
	return true
}

func bibleBookName(reference string) (string, bool) {
	if psalmPrefixRe.MatchString(reference) {
		return "", false
	}
	m := bookNameRe.Find(reference)
	if m == nil {
		return "", false
	}
	name := rb.Strip(m.G(1))
	// Ruby: `if numeric && (book_name = ...)` - an empty string is truthy.
	return name, true
}

// --- Reading::Presenter ----------------------------------------------------

// Presenter ports Reading::Presenter.
type Presenter struct {
	Translation string
	Language    string
}

// Passage ports #passage.
func (p *Presenter) Passage(reference string, content *rb.Map) *Passage {
	parsed := bible.Parse(bible.Normalize(reference))
	if parsed == nil {
		return &Passage{Reference: newReference(reference), Translation: translationPtr(p.Translation)}
	}
	translation := translationPtr(p.Translation)
	if content != nil {
		if v, ok := content.Lookup("translation"); ok && v != nil {
			s := rb.ToS(v)
			translation = &s
		}
	}
	bookName := liturgical.TranslateBookName(parsed.Book, p.Language)
	chapter := parsed.Chapter
	out := &Passage{
		Reference:   newReference(reference),
		Translation: translation,
		BookName:    &bookName,
		Chapter:     &chapter,
		Content:     content,
	}
	if parsed.VerseStart != 0 {
		v := parsed.VerseStart
		out.VerseStart = &v
	}
	if parsed.VerseEnd != 0 {
		v := parsed.VerseEnd
		out.VerseEnd = &v
	}
	return out
}

// --- Reading::ContentLoader ------------------------------------------------

// PsalterCatalog ports Bible::PsalterCatalog::ALL.
var PsalterCatalog = []string{"coverdale", "coverdale_1928", "new_coverdale", "ieab_1987", "ieab_2015"}

// IsPsalter ports Bible::PsalterCatalog.psalter?.
func IsPsalter(value string) bool {
	for _, p := range PsalterCatalog {
		if p == value {
			return true
		}
	}
	return false
}

// ContentLoader ports Reading::ContentLoader.
type ContentLoader struct {
	Translation      string
	PsalmTranslation string // "" == nil
	Cache            *bible.SegmentCache
}

// Load ports #load.
func (c *ContentLoader) Load(ctx context.Context, references []string) map[string]*rb.Map {
	if len(references) == 0 {
		return map[string]*rb.Map{}
	}
	if !IsPsalter(c.PsalmTranslation) {
		return mustFetch(ctx, bible.NewTextService(c.Translation, c.Cache), references)
	}
	var psalms, others []string
	for _, r := range references {
		if psalmReference(r) {
			psalms = append(psalms, r)
		} else {
			others = append(others, r)
		}
	}
	out := mustFetch(ctx, bible.NewTextService(c.Translation, c.Cache), others)
	for k, v := range mustFetch(ctx, bible.NewTextService(c.PsalmTranslation, c.Cache), psalms) {
		out[k] = v
	}
	return out
}

func mustFetch(ctx context.Context, svc *bible.TextService, refs []string) map[string]*rb.Map {
	out, err := svc.FetchPassagesBatch(ctx, refs)
	if err != nil {
		panic(err)
	}
	if out == nil {
		out = map[string]*rb.Map{}
	}
	return out
}

func psalmReference(reference string) bool {
	parsed := bible.ParseAll(reference)
	if len(parsed) == 0 {
		return false
	}
	for _, s := range parsed {
		if bible.BookID(s.Book) != 19 {
			return false
		}
	}
	return true
}

// --- Reading::SelectionBuilder ---------------------------------------------

// SelectionBuilder ports Reading::SelectionBuilder.
type SelectionBuilder struct {
	Translation          string
	PsalmTranslation     string
	Language             string
	CumulativePsalmLists bool
	LoadContent          bool
	Cache                *bible.SegmentCache

	parser    *AlternativeParser
	presenter *Presenter
}

// Presenter returns the passage presenter.
func (b *SelectionBuilder) Presenter() *Presenter {
	if b.presenter == nil {
		b.presenter = &Presenter{Translation: b.Translation, Language: b.Language}
	}
	return b.presenter
}

func (b *SelectionBuilder) alternativeParser() *AlternativeParser {
	if b.parser == nil {
		prefix := "Salmo"
		if langs.English(b.Language) {
			prefix = "Psalm"
		}
		b.parser = &AlternativeParser{PsalmPrefix: prefix, CumulativeLists: b.CumulativePsalmLists}
	}
	return b.parser
}

// SplitAlternatives ports split_alternatives.
func (b *SelectionBuilder) SplitAlternatives(reference string) []string {
	return b.alternativeParser().Call(reference)
}

// Call ports #call.
func (b *SelectionBuilder) Call(ctx context.Context, r *Record) *Selection {
	if r == nil {
		return nil
	}
	contents := map[string]*rb.Map{}
	if b.LoadContent {
		contents = b.fetchContents(ctx, b.allReferences(r))
	}
	return &Selection{
		FirstReading:     b.Enrich(r.FirstReading, contents, r.FirstReadingTitle),
		Psalm:            b.Enrich(r.Psalm, contents, r.PsalmTitle),
		PsalmAlternative: b.Enrich(r.PsalmAlternative, contents, nil),
		SecondReading:    b.Enrich(r.SecondReading, contents, r.SecondReadingTitle),
		Gospel:           b.Enrich(r.Gospel, contents, r.GospelTitle),
		Notes:            notesValue(r.Notes),
	}
}

func notesValue(n *string) any {
	if n == nil {
		return nil
	}
	return *n
}

func (b *SelectionBuilder) allReferences(r *Record) []string {
	var out []string
	seen := map[string]bool{}
	for _, ref := range []*string{r.FirstReading, r.Psalm, r.PsalmAlternative, r.SecondReading, r.Gospel} {
		if ref == nil {
			continue
		}
		for _, s := range b.SplitAlternatives(*ref) {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func (b *SelectionBuilder) fetchContents(ctx context.Context, refs []string) map[string]*rb.Map {
	if len(refs) == 0 {
		return map[string]*rb.Map{}
	}
	loader := &ContentLoader{Translation: b.Translation, PsalmTranslation: b.PsalmTranslation, Cache: b.Cache}
	return loader.Load(ctx, refs)
}

// Enrich ports #enrich. reference nil or blank yields nil.
func (b *SelectionBuilder) Enrich(reference *string, contents map[string]*rb.Map, title *string) *Passage {
	if reference == nil || rb.BlankString(*reference) {
		return nil
	}
	ref := *reference
	if group := b.psalmGroup(ref); group != nil {
		return b.groupedPsalmPassage(ref, group, contents, title)
	}
	split := b.SplitAlternatives(ref)
	primary := split[0]
	result := b.singlePassage(primary, contents)
	if result == nil {
		return &Passage{Reference: newReference(primary), Translation: translationPtr(b.Translation), Title: title}
	}
	var alternatives []*Passage
	for _, alt := range split[1:] {
		if p := b.singlePassage(alt, contents); p != nil {
			alternatives = append(alternatives, p)
		}
	}
	out := result.dup()
	if len(alternatives) > 0 {
		out.Alternative = alternatives[0]
	} else {
		out.Alternative = nil
	}
	if title != nil && !rb.BlankString(*title) {
		out.Title = title
	} else {
		out.Title = nil
	}
	if len(alternatives) > 1 {
		out.Alternatives = alternatives
	}
	return out
}

func (b *SelectionBuilder) psalmGroup(reference string) []string {
	p := b.alternativeParser()
	stripped := p.StripOptionalMarkers(reference)
	if p.PsalmNumberList(stripped) {
		return p.ExpandPsalmNumberList(stripped)
	}
	split := b.SplitAlternatives(reference)
	if len(split) > 1 {
		grouped := true
		for _, s := range split {
			if !psalmWordRe.MatchString(s) {
				grouped = false
				break
			}
		}
		if grouped && !orOuSlashRe.MatchString(reference) {
			return split
		}
	}
	return p.PluralPrefixedPsalmList(stripped)
}

func (b *SelectionBuilder) groupedPsalmPassage(reference string, refs []string, contents map[string]*rb.Map, title *string) *Passage {
	if !b.LoadContent {
		return &Passage{Reference: newReference(reference), Translation: translationPtr(b.Translation), Title: title}
	}
	verses := []any{}
	for _, ref := range refs {
		m := firstNumberRe.Find(ref)
		if m == nil {
			rb.RaiseNoMethodOnNil("[]")
		}
		chapter := rb.StringToI(m.G(1))
		c := contents[ref]
		if c == nil {
			continue
		}
		list, _ := c.Get("verses").([]any)
		for _, v := range list {
			vm, ok := v.(*rb.Map)
			if !ok {
				continue
			}
			d := vm.Dup()
			d.Set("chapter_number", chapter)
			verses = append(verses, d)
		}
	}
	contentTranslation := b.Translation
	for _, ref := range refs {
		if c := contents[ref]; c != nil {
			if v := c.Get("translation"); rb.Present(v) {
				contentTranslation = rb.ToS(v)
				break
			}
		}
	}
	content := rb.M("reference", reference, "translation", translationValue(contentTranslation))
	if len(verses) > 0 {
		content.Set("verses", verses)
	}
	return &Passage{Reference: newReference(reference), Translation: translationPtr(contentTranslation), Content: content, Title: title}
}

func translationValue(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (b *SelectionBuilder) singlePassage(reference string, contents map[string]*rb.Map) *Passage {
	if rb.BlankString(reference) {
		return nil
	}
	return b.Presenter().Passage(reference, contents[reference])
}

var _ = strings.TrimSpace
