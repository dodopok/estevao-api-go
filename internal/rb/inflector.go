package rb

import (
	"strings"
	"unicode"
)

// Humanize ports ActiveSupport::Inflector#humanize (Rails 8.1, no custom
// human rules or acronyms are configured by the application).
func Humanize(s string) string {
	result := strings.ReplaceAll(s, "_", " ")
	result = strings.TrimLeft(result, " \t\n\v\f\r\x00")
	if strings.HasSuffix(s, "_id") {
		result = strings.TrimSuffix(result, " id")
	}
	isAlnum := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.Is(unicode.Nl, r) || (r >= '0' && r <= '9')
	}
	rs := []rune(result)
	for i, r := range rs {
		if isAlnum(r) {
			rs[i] = unicode.ToLower(r)
		}
	}
	if len(rs) > 0 && (unicode.IsLetter(rs[0]) || unicode.IsMark(rs[0]) || unicode.Is(unicode.Nl, rs[0])) {
		rs[0] = unicode.ToUpper(rs[0])
	}
	return string(rs)
}
