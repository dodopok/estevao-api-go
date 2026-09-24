package rb

import "strings"

// Transliterate mirrors I18n.transliterate with the default approximations:
// every non-ASCII character without an approximation becomes "?".
func Transliterate(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 {
			b.WriteRune(r)
			continue
		}
		if a, ok := transliterations[r]; ok {
			b.WriteString(a)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// Parameterize mirrors ActiveSupport's String#parameterize (separator "-").
func Parameterize(s string) string {
	t := Transliterate(s)
	var b strings.Builder
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	return strings.ToLower(out)
}
