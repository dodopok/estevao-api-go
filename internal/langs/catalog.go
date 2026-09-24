// Package langs ports PrayerBooks::LanguageCatalog: the language identity of
// a Prayer Book and the catalogues that serve it.
package langs

import "strings"

type Profile struct {
	Code, Family, BibleLanguage, TranslatorLanguage, ExplanationLocale, OfficeNamesLocale string
}

var definitions = []Profile{
	{"en", "en", "en", "en", "en-US", "en"},
	{"en-AU", "en", "en", "en", "en-US", "en"},
	{"en-CA", "en", "en", "en", "en-US", "en"},
	{"es", "es", "es", "es", "es", "es"},
	{"cy", "cy", "en", "cy", "cy", "cy"},
	{"pt-BR", "pt", "pt-BR", "pt-BR", "pt-BR", "pt-BR"},
	{"pt-PT", "pt", "pt-PT", "pt-PT", "pt-PT", "pt-PT"},
}

var profiles = func() map[string]Profile {
	m := map[string]Profile{}
	for _, p := range definitions {
		m[p.Code] = p
	}
	return m
}()

// PrayerBookLanguages are the accepted prayer_books.language values, in order.
var PrayerBookLanguages = func() []string {
	out := []string{}
	for _, p := range definitions {
		out = append(out, p.Code)
	}
	return out
}()

// ExplanationLocales are the distinct explanation locales, in order.
var ExplanationLocales = func() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range definitions {
		if !seen[p.ExplanationLocale] {
			seen[p.ExplanationLocale] = true
			out = append(out, p.ExplanationLocale)
		}
	}
	return out
}()

// Normalize ports LanguageCatalog.normalize.
func Normalize(language string) string {
	s := strings.ReplaceAll(strings.TrimSpace(language), "_", "-")
	var parts []string
	for _, p := range strings.Split(s, "-") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	out := []string{strings.ToLower(parts[0])}
	for _, p := range parts[1:] {
		if len(p) >= 2 && len(p) <= 3 {
			out = append(out, strings.ToUpper(p))
		} else {
			out = append(out, p)
		}
	}
	return strings.Join(out, "-")
}

// ProfileFor returns the profile or nil.
func ProfileFor(language string) *Profile {
	n := Normalize(language)
	if strings.TrimSpace(n) == "" {
		return nil
	}
	if p, ok := profiles[n]; ok {
		return &p
	}
	if English(n) {
		p := profiles["en"]
		p.Code = n
		return &p
	}
	return nil
}

func IsPrayerBookLanguage(language string) bool {
	_, ok := profiles[Normalize(language)]
	return ok
}

func ExplanationLocaleFor(language string) string {
	if p := ProfileFor(language); p != nil {
		return p.ExplanationLocale
	}
	return ""
}

func TranslatorLanguageFor(language string) string {
	if p := ProfileFor(language); p != nil {
		return p.TranslatorLanguage
	}
	return ""
}

func OfficeNamesLocaleFor(language string) string {
	if p := ProfileFor(language); p != nil {
		return p.OfficeNamesLocale
	}
	return ""
}

func BibleLanguageFor(language string) string {
	if p := ProfileFor(language); p != nil {
		return p.BibleLanguage
	}
	return ""
}

func BibleFallbackLanguageFor(language string) string {
	n := Normalize(language)
	f := BibleLanguageFor(n)
	if strings.TrimSpace(f) != "" && f != n {
		return f
	}
	return ""
}

// BibleLanguageCandidatesFor returns [normalized, fallback] without blanks.
func BibleLanguageCandidatesFor(language string) []string {
	n := Normalize(language)
	if strings.TrimSpace(n) == "" {
		return nil
	}
	out := []string{n}
	if f := BibleFallbackLanguageFor(n); f != "" && f != n {
		out = append(out, f)
	}
	return out
}

func CompatibleBibleLanguages(left, right string) bool {
	l, r := Normalize(left), Normalize(right)
	if strings.TrimSpace(l) == "" || strings.TrimSpace(r) == "" {
		return false
	}
	if l == r {
		return true
	}
	lp, rp := ProfileFor(l), ProfileFor(r)
	return lp != nil && rp != nil && lp.BibleLanguage == rp.BibleLanguage
}

// ColorLanguageFor returns "pt" for Portuguese variants, the family otherwise,
// and "en" when unknown.
func ColorLanguageFor(language string) string {
	p := ProfileFor(language)
	if p == nil {
		return "en"
	}
	if p.Family == "pt" {
		return "pt"
	}
	return p.Family
}

func English(language string) bool {
	n := Normalize(language)
	return n == "en" || strings.HasPrefix(n, "en-")
}
