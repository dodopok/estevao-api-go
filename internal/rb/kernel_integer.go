package rb

import (
	"math"
	"math/big"
	"strings"
)

// KernelInteger ports Kernel#Integer(value) for the values parsed JSON and
// request params carry. On failure it returns the Ruby exception class
// Integer would raise: "ArgumentError" (a malformed String), "TypeError"
// (nil, booleans, arrays and hashes) or "FloatDomainError" (NaN, Infinity).
// Values beyond int64 saturate, which keeps their ordering.
func KernelInteger(v any) (int64, string) {
	switch x := v.(type) {
	case int:
		return int64(x), ""
	case int64:
		return x, ""
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, "FloatDomainError"
		}
		t := math.Trunc(x)
		if t >= math.MaxInt64 {
			return math.MaxInt64, ""
		}
		if t <= math.MinInt64 {
			return math.MinInt64, ""
		}
		return int64(t), ""
	case string:
		n, ok := integerString(x)
		if !ok {
			return 0, "ArgumentError"
		}
		return n, ""
	}
	return 0, "TypeError"
}

func isRubySpace(c byte) bool { return c == ' ' || (c >= '\t' && c <= '\r') }

// integerString ports rb_str_to_inum(str, 0, badcheck: true).
func integerString(s string) (int64, bool) {
	if strings.IndexByte(s, 0) >= 0 {
		return 0, false
	}
	i, j := 0, len(s)
	for i < j && isRubySpace(s[i]) {
		i++
	}
	for j > i && isRubySpace(s[j-1]) {
		j--
	}
	s = s[i:j]
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	base := 10
	if len(s) >= 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			base, s = 16, s[2:]
		case 'b', 'B':
			base, s = 2, s[2:]
		case 'o', 'O':
			base, s = 8, s[2:]
		case 'd', 'D':
			base, s = 10, s[2:]
		default:
			base, s = 8, s[1:]
			// "0_7" is octal 7: an underscore may follow the leading zero.
			if s != "" && s[0] == '_' {
				s = s[1:]
				if s == "" {
					return 0, false
				}
			}
		}
	}
	if s == "" || s[0] == '_' || s[0] == '+' || s[0] == '-' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	n, ok := new(big.Int).SetString(strings.ReplaceAll(s, "_", ""), base)
	if !ok {
		return 0, false
	}
	if neg {
		n.Neg(n)
	}
	if n.IsInt64() {
		return n.Int64(), true
	}
	if n.Sign() > 0 {
		return math.MaxInt64, true
	}
	return math.MinInt64, true
}
