package rb

import "strings"

// CastInteger ports ActiveModel::Type::Integer#cast (ok=false for nil).
func CastInteger(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		// Numeric#cast: a blank string is nil; anything else goes through
		// String#to_i, which is 0 when there are no leading digits.
		if BlankString(x) {
			return 0, false
		}
		s := strings.TrimLeft(x, " \t\n\v\f\r")
		i := 0
		if i < len(s) && (s[i] == '-' || s[i] == '+') {
			i++
		}
		j := i
		isDigit := func(k int) bool { return k < len(s) && s[k] >= '0' && s[k] <= '9' }
		// A single underscore may separate digits ("1_000"); "1__2" stops at 1.
		for isDigit(j) || j > i && j < len(s) && s[j] == '_' && isDigit(j+1) {
			j++
		}
		if j == i {
			return 0, true
		}
		n := int64(0)
		for _, ch := range s[i:j] {
			if ch != '_' {
				n = n*10 + int64(ch-'0')
			}
		}
		if s[0] == '-' {
			n = -n
		}
		return n, true
	}
	return 0, false
}
