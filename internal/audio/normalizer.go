package audio

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// Patterns are Audio::Normalizer's, verbatim.
var (
	nInlineReference  = rx.MustCompile(`\s*__\([^)]+\)__`)
	nBold             = rx.MustCompile(`\*\*(.*?)\*\*`, "m")
	nUnderline        = rx.MustCompile(`__([^_]+)__`)
	nEmphasis         = rx.MustCompile(`\*([^*\n]+)\*`)
	nPointing         = rx.MustCompile(`(?<![\S])\*(?![\S])`)
	nScriptureHeading = rx.MustCompile(`(?<=\n)(?<![[:alnum:]])(?:[[:digit:]]{1,3}[[:space:]]+)?(?:[[:alpha:]À-ÿ][[:alpha:]À-ÿ’'‑-]*(?:[[:space:]]+|[.])){1,8}\d+[[:space:]]*[.‑–-][[:space:]]*\d`, "i")

	nEditorialAlternative         = rx.MustCompile(`\s*\(\s*(?:ou|or|neu)\b[^()]*\)`, "i")
	nSpanishEditorialAlternative  = rx.MustCompile(`\s*\(\s*o\b[^()]*\)`, "i")
	nSquareEditorialAlternative   = rx.MustCompile(`\s*\[\s*(?:ou|or|neu)\b[^\]]*\]`, "i")
	nResponsiveAlternative        = rx.MustCompile(`\s+(?:ou|or|neu)\s*[,;:]\s*.*?(?=\s+[℣℟](?:\.|\s)|\z)`, "i", "m")
	nRegionalEditorialAlternative = rx.MustCompile(`\s+Or,\s+in the parishes of Canada,.*?;\s*We beg\b[^.]*\.`, "i")
	nSpeakerMarker                = rx.MustCompile(`[℣℟]\.?(?:\s+|\z)`)
	nCrossMarker                  = rx.MustCompile(`✠`)
	nOptionalRole                 = rx.MustCompile(`(?:\[\s*(?:Bispo e|Bishop and)\s*\]|\(\s*(?:Bispo e|Bishop and)\s*\))\s*(?=(?:Pastor)\b)`, "i")
	nInclusiveForm                = rx.MustCompile(`\b([[:alpha:]][[:alpha:]À-ÿ]*)\(([[:alpha:]][[:alpha:]À-ÿ]*)\)`)
	nQualOption                   = rx.MustCompile(`\b(qual)\s+\(is\)`, "i")
	nParentheticalSlash           = rx.MustCompile(`(\s*)\(\s*([^()\/]+?)\s*\/[^()]*\)`)
	nUnresolvedBlankTemplate      = rx.MustCompile(`(?:\[[^\]]*_{3,}[^\]]*\]|\([^)]*_{3,}[^)]*\))`)
	nSpokenAnnotation             = rx.MustCompile(`\s*\[\s*(?:Pausa|Interlúdio|Interlude|Pause)\s*\]\s*\.?`, "i")
	nEmptyBrackets                = rx.MustCompile(`\s*\[\s*\]`)
	nFootnoteMarker               = rx.MustCompile(`\s*\[[[:lower:]]\](?=\s|[[:punct:]]|$)`)
	nUnresolvedTemplate           = rx.MustCompile(`\s*\[\s*(?:\bN\.?(?=\s|\])|São\/Santo(?:\(a\))?|Saint\s*[-—])[^\]]*\]`, "i")
	nParentheticalPlaceholder     = rx.MustCompile(`\s*\(\s*(?:and\s+)?N\.?(?:\s+(?:and|e)\s+N\.?)?\s*\)`, "i")
	nPersonPlaceholder            = rx.MustCompile(`\bN\.?(?=\s|[,:;.)]|$)`)
	nWelshPersonPlaceholder       = rx.MustCompile(`\bE\.(?=\s|[,:;.)]|$)`)
	nBlankPlaceholder             = rx.MustCompile(`_+`)
	nSlashedInclusiveForm         = rx.MustCompile(`\b(?:his\/her|her\/his|him\/her|her\/him|he\/she|she\/he)\b`, "i")
	nWelshGenderAlternative       = rx.MustCompile(`\s+\((?:wasanaethferch|gyda hi)\)`, "i")
	nWelshMutation                = rx.MustCompile(`\(([h])\)(?=[[:alpha:]])`, "i")
	nSpeakerSign                  = rx.MustCompile(`[℣℟]`)
	nDoubledWith                  = rx.MustCompile(`\b(com|with|con)\s+(?:e|and|y)\s+\1\b`, "i")
	nBracketed                    = rx.MustCompile(`\[([^\]]+)\]`)

	nShoutedWord = rx.MustCompile(`\b(?=[[:upper:]]*[AEIOUÁÉÍÓÚÂÊÔÃÕÀ])[[:upper:]ÁÉÍÓÚÂÊÔÃÕÀÇ]{4,}\b`)

	nPsalmVerseNumber = rx.MustCompile(`(^|\n)\s*\d{1,3}(?:[.)-])?\s+`)
	nPsalmType        = rx.MustCompile(`\Apsalms?\z`, "i")
	nPsalmSlug        = rx.MustCompile(`(?:\A|_)psalms?(?:_|\z)`, "i")
	nWhitespace       = rx.MustCompile(`\s+`)
	nSpaceBeforePunct = rx.MustCompile(`\s+([,.;:!?])`)

	// PRONUNCIATIONS, in insertion order.
	nPronunciations = []struct {
		re *rx.Regexp
		to string
	}{
		{rx.MustCompile(`\b`+rx.Quote("Javé")+`\b`, "i"), "Iavé"},
		{rx.MustCompile(`\b`+rx.Quote("YHWH")+`\b`, "i"), "Iavé"},
	}
)

// Normalize ports Audio::Normalizer.call. verseNumber is the line's raw
// verse_number (nil, an Integer or a String).
func Normalize(text, lineType, slug string, verseNumber any, language string) string {
	if rb.BlankString(text) {
		return ""
	}
	result := text
	result = nInlineReference.Gsub(result, "")
	result = nBold.Gsub(result, `\1`)
	result = nUnderline.Gsub(result, `\1`)
	result = nEmphasis.Gsub(result, `\1`)
	result = nPointing.Gsub(result, " ")
	if lineType == "prayer" {
		if m := nScriptureHeading.Find(result); m != nil {
			result = rstrip(string([]rune(result)[:m.Begin()]))
		}
	}
	result = removeEditorialAlternatives(result, lineType, language)
	result = removeVerseNumbers(result, lineType, slug, verseNumber)
	result = nShoutedWord.GsubFunc(result, func(m *rx.Match) string { return capitalize(m.String()) })
	for _, p := range nPronunciations {
		result = p.re.Gsub(result, p.to)
	}
	result = SpokenReference(result, language)
	result = nWhitespace.Gsub(result, " ")
	result = nSpaceBeforePunct.Gsub(result, `\1`)
	return rb.Strip(result)
}

func removeEditorialAlternatives(result, lineType, language string) string {
	result = nEditorialAlternative.Gsub(result, "")
	if strings.HasPrefix(language, "es") {
		result = nSpanishEditorialAlternative.Gsub(result, "")
	}
	result = nSquareEditorialAlternative.Gsub(result, " ")
	if nSpeakerSign.MatchString(result) {
		result = nResponsiveAlternative.Gsub(result, " ")
	}
	if strings.HasPrefix(language, "en") {
		result = nRegionalEditorialAlternative.Gsub(result, "")
	}
	result = nSpeakerMarker.Gsub(result, "")
	result = nCrossMarker.Gsub(result, "")
	result = nSpokenAnnotation.Gsub(result, " ")
	result = nEmptyBrackets.Gsub(result, " ")
	result = nFootnoteMarker.Gsub(result, " ")
	result = nUnresolvedTemplate.Gsub(result, " ")
	result = nParentheticalPlaceholder.Gsub(result, " ")
	result = nParentheticalSlash.Gsub(result, `\1\2`)
	result = nUnresolvedBlankTemplate.Gsub(result, " ")
	result = nDoubledWith.Gsub(result, `\1`)
	if language == "cy" {
		result = nWelshGenderAlternative.Gsub(result, " ")
		result = nWelshMutation.Gsub(result, "")
	}
	result = nOptionalRole.Gsub(result, "")
	result = nInclusiveForm.Gsub(result, `\1`)
	result = nQualOption.Gsub(result, `\1`)
	result = nSlashedInclusiveForm.GsubFunc(result, func(m *rx.Match) string {
		return strings.SplitN(m.String(), "/", 2)[0]
	})
	result = nBlankPlaceholder.Gsub(result, "")
	result = nBracketed.Gsub(result, `\1`)
	if lineType != "reading_text" {
		result = nPersonPlaceholder.Gsub(result, "")
		if language == "cy" {
			result = nWelshPersonPlaceholder.Gsub(result, "")
		}
	}
	return result
}

func removeVerseNumbers(text, lineType, slug string, verseNumber any) string {
	if nPsalmType.MatchString(lineType) || nPsalmSlug.MatchString(slug) {
		return nPsalmVerseNumber.Gsub(text, `\1`)
	}
	if lineType == "reading_text" {
		if n, ok := rubyIntegerValue(verseNumber); ok && n > 0 {
			re := rx.MustCompile(`\A\s*` + rx.Quote(strconv.Itoa(n)) + `(?:[.)-])?\s+`)
			return re.Sub(text, "")
		}
	}
	return text
}

// rubyIntegerValue ports Integer(value, exception: false) for the values a
// line's verse_number can hold.
func rubyIntegerValue(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		if x != x || x > 1e18 || x < -1e18 {
			return 0, false
		}
		return int(x), true
	case string:
		return rubyIntegerString(x)
	}
	return 0, false
}

// rubyIntegerString ports Kernel#Integer(String): optional sign, 0b/0o/0x
// and leading-zero octal prefixes, single underscores between digits.
func rubyIntegerString(s string) (int, bool) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		neg = s[0] == '-'
		s = s[1:]
	}
	base := 10
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, "0x"):
		base, s = 16, s[2:]
	case strings.HasPrefix(low, "0b"):
		base, s = 2, s[2:]
	case strings.HasPrefix(low, "0o"):
		base, s = 8, s[2:]
	case strings.HasPrefix(low, "0d"):
		base, s = 10, s[2:]
	case len(s) > 1 && s[0] == '0':
		base, s = 8, s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), base, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		n = -n
	}
	return int(n), true
}

// capitalize ports String#capitalize.
func capitalize(s string) string {
	r := []rune(strings.ToLower(s))
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToTitle(r[0])
	return string(r)
}

// rstrip ports String#rstrip.
func rstrip(s string) string { return strings.TrimRight(s, " \t\n\v\f\r\x00") }
