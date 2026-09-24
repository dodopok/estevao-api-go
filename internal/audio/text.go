package audio

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// --- Audio::TextSegmenter ----------------------------------------------------

// DefaultMaxCharacters is TextSegmenter::DEFAULT_MAX_CHARACTERS.
const DefaultMaxCharacters = 1800

var (
	terminalResponse = rx.MustCompile(`\A(.+[.!?])\s+((?:Amém|Amen|Amén|Aleluia|Alleluia|Haleliwia)[.!?]?)\z`, "i")
	sentenceBoundary = rx.MustCompile(`(?<=[.!?;:])\s+`)
	wordBoundary     = rx.MustCompile(`\s+`)
)

// Segment ports Audio::TextSegmenter.call.
func Segment(text string, maxCharacters int, splitTerminalResponse bool) []string {
	text = rb.Strip(text)
	if rb.BlankString(text) {
		return nil
	}
	limit := min(max(maxCharacters, 1), 4096)
	if splitTerminalResponse && rubyLen(text) <= limit {
		if m := terminalResponse.Find(text); m != nil {
			return []string{m.G(1), m.G(2)}
		}
		return []string{text}
	}
	var chunks []string
	current := ""
	for _, sentence := range sentenceBoundary.Split(text, 0) {
		if rb.BlankString(sentence) {
			continue
		}
		n := rubyLen(sentence)
		switch {
		case n > limit:
			if !rb.BlankString(current) {
				chunks = append(chunks, current)
			}
			current = ""
			chunks = append(chunks, splitWords(sentence, limit)...)
		case rb.BlankString(current):
			current = sentence
		case rubyLen(current)+n+1 <= limit:
			current = current + " " + sentence
		default:
			chunks = append(chunks, current)
			current = sentence
		}
	}
	if !rb.BlankString(current) {
		chunks = append(chunks, current)
	}
	return chunks
}

func splitWords(sentence string, limit int) []string {
	var chunks []string
	current := ""
	for _, word := range wordBoundary.Split(sentence, 0) {
		n := rubyLen(word)
		switch {
		case n > limit:
			if !rb.BlankString(current) {
				chunks = append(chunks, current)
			}
			current = ""
			r := []rune(word)
			for i := 0; i < len(r); i += limit {
				chunks = append(chunks, string(r[i:min(i+limit, len(r))]))
			}
		case rb.BlankString(current):
			current = word
		case rubyLen(current)+n+1 <= limit:
			current = current + " " + word
		default:
			chunks = append(chunks, current)
			current = word
		}
	}
	if !rb.BlankString(current) {
		chunks = append(chunks, current)
	}
	return chunks
}

// --- Audio::SpeakableLine ----------------------------------------------------

var spokenTypes = map[string]bool{
	"all": true, "anthem": true, "antiphon": true, "canticle": true, "congregation": true, "creed": true,
	"leader": true, "prayer": true, "reader": true, "reading_text": true, "responsive": true, "text": true,
}

var responseTypes = map[string]bool{"leader": true, "congregation": true, "responsive": true}

var nonAlpha = rx.MustCompile(`[^[:alpha:]\s]`)

func words(text string) []string {
	return strings.Fields(nonAlpha.Gsub(rubyDowncase(text), " "))
}

func allIn(ws []string, set ...string) bool {
	if len(ws) == 0 {
		return false
	}
	for _, w := range ws {
		found := false
		for _, s := range set {
			if w == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// immediateResponse ports SpeakableLine.immediate_response?.
func immediateResponse(text string) bool { return allIn(words(text), "amém", "amem", "aleluia") }

// skipManualNavigation ports SpeakableLine.skip_manual_navigation?.
func skipManualNavigation(text string) bool {
	return allIn(words(text), "amém", "amem", "amen", "amén")
}

// responsePair ports SpeakableLine.response_pair?.
func responsePair(previous, current string) bool {
	return previous != current && responseTypes[previous] && responseTypes[current]
}

// rubyDowncase is String#downcase (full Unicode case mapping).
func rubyDowncase(s string) string { return strings.ToLower(s) }

// --- Audio::ContextualText ---------------------------------------------------

var (
	ctxPersonPlaceholder      = rx.MustCompile(`\bN\.?(?=\s|[,:;.)\]}]|$)`)
	ctxWelshPersonPlaceholder = rx.MustCompile(`\bE\.(?=\s|[,:;.)\]}]|$)`)
	ctxOptionalBlankTemplate  = rx.MustCompile(`(?:\[[^\]]*_{3,}[^\]]*\]|\([^)]*_{3,}[^)]*\))`)
	ctxTemplatePlaceholder    = rx.MustCompile(`\{\{[^}]+\}\}`)
	ctxInclusiveForm          = rx.MustCompile(`\b(?:bispo|pastor|presb[ií]tero|di[aá]cono|servo|serva|santo|santa|teu|tua|pelo|pela|qual|seja|sejam|tenha|tenham)\([^)]{1,20}\)`, "i")
	ctxOptionalRole           = rx.MustCompile(`(?:\[\s*(?:Bispo e|Bishop and)\s*\]|\(\s*(?:Bispo e|Bishop and)\s*\))\s*(?:Pastor|Pastora|Pastor\(a\)|Pastor(?:s)?)`, "i")
	ctxSlashedPronoun         = rx.MustCompile(`\b(?:his\/her|her\/his|him\/her|her\/him|he\/she|she\/he)\b`, "i")
)

// ContextRequired ports Audio::ContextualText.context_required?.
func ContextRequired(text, language string) bool {
	value := ctxOptionalBlankTemplate.Gsub(text, " ")
	return ctxPersonPlaceholder.MatchString(value) ||
		(strings.HasPrefix(language, "cy") && ctxWelshPersonPlaceholder.MatchString(value)) ||
		ctxTemplatePlaceholder.MatchString(value) ||
		ctxInclusiveForm.MatchString(value) ||
		ctxOptionalRole.MatchString(value) ||
		ctxSlashedPronoun.MatchString(value)
}
