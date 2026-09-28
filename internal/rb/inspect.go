package rb

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// InspectString ports String#inspect for a UTF-8 string: quotes, backslashes
// and interpolation openers are escaped, the named control escapes are used,
// and any other character Onigmo does not consider printable is written as
// \uXXXX (\u{X} above the BMP); invalid bytes as \xHH.
func InspectString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "\\x%02X", s[i])
			i++
			continue
		}
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '#':
			if i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '$' || s[i+1] == '@') {
				b.WriteString(`\#`)
			} else {
				b.WriteByte('#')
			}
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		case '\b':
			b.WriteString(`\b`)
		case '\a':
			b.WriteString(`\a`)
		case 0x1b:
			b.WriteString(`\e`)
		default:
			if printable(r) {
				b.WriteRune(r)
			} else if r < 0x10000 {
				fmt.Fprintf(&b, "\\u%04X", r)
			} else {
				fmt.Fprintf(&b, "\\u{%X}", r)
			}
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}

// printable approximates Onigmo's [[:print:]] for Unicode: every assigned,
// non-control, non-surrogate character except the line and paragraph
// separators.
func printable(r rune) bool {
	if r == 0x2028 || r == 0x2029 {
		return false
	}
	return unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Zs, unicode.Cf, unicode.Co)
}

// Downcase ports String#downcase (full Unicode case mapping), including
// the one special mapping Go's simple folding lacks: U+0130 becomes "i̇".
func Downcase(s string) string {
	if !strings.ContainsRune(s, 0x130) {
		return strings.ToLower(s)
	}
	return strings.ToLower(strings.ReplaceAll(s, "İ", "i̇"))
}

// CGIEscape ports CGI.escape: everything but ASCII letters, digits and
// _.-~ is percent-encoded (upper-case hex), and space becomes '+'.
func CGIEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '-', c == '~':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ToQuery ports Hash#to_query (Active Support) on parsed request
// parameters: nested hashes and arrays use bracketed keys, empty nested
// containers are skipped, and each level is sorted unless its namespace
// already names an array.
func ToQuery(m *Map, namespace string) string {
	var parts []string
	m.Each(func(k string, v any) {
		switch x := v.(type) {
		case *Map:
			if x.Len() == 0 {
				return
			}
		case []any:
			if len(x) == 0 {
				return
			}
		}
		key := k
		if namespace != "" {
			key = namespace + "[" + k + "]"
		}
		parts = append(parts, valueToQuery(v, key))
	})
	if !strings.Contains(namespace, "[]") {
		sort.Strings(parts)
	}
	return strings.Join(parts, "&")
}

func valueToQuery(v any, key string) string {
	switch x := v.(type) {
	case *Map:
		return ToQuery(x, key)
	case []any:
		prefix := key + "[]"
		if len(x) == 0 {
			return CGIEscape(prefix) + "="
		}
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = valueToQuery(e, prefix)
		}
		return strings.Join(parts, "&")
	case nil:
		return CGIEscape(key) + "="
	}
	return CGIEscape(key) + "=" + CGIEscape(ToS(v))
}
