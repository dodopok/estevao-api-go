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
func Parameterize(s string) string { return ParameterizeSep(s, "-") }

// ParameterizeSep ports String#parameterize(separator: sep) for a non-empty
// separator.
func ParameterizeSep(s, sep string) string {
	t := Transliterate(s)
	var b strings.Builder
	inRun := false
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			inRun = false
		} else if !inRun {
			b.WriteString(sep)
			inRun = true
		}
	}
	out := b.String()
	dup := sep + sep
	if strings.Contains(out, dup) {
		var c strings.Builder
		for i := 0; i < len(out); {
			if strings.HasPrefix(out[i:], sep) {
				c.WriteString(sep)
				for strings.HasPrefix(out[i:], sep) {
					i += len(sep)
				}
				continue
			}
			c.WriteByte(out[i])
			i++
		}
		out = c.String()
	}
	if sep == "-" {
		out = strings.TrimPrefix(out, "-")
		out = strings.TrimSuffix(out, "-")
	} else {
		// /^-?sep|sep-?$/
		if strings.HasPrefix(out, "-"+sep) {
			out = out[1+len(sep):]
		} else if strings.HasPrefix(out, sep) {
			out = out[len(sep):]
		}
		if strings.HasSuffix(out, sep+"-") {
			out = out[:len(out)-len(sep)-1]
		} else if strings.HasSuffix(out, sep) {
			out = out[:len(out)-len(sep)]
		}
	}
	return strings.ToLower(out)
}
