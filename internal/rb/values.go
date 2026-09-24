package rb

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

// Raw is pre-encoded JSON inserted verbatim.
type Raw string

// Decimal is a BigDecimal-like value rendered as a JSON string.
type Decimal struct {
	R     *big.Rat
	Scale int
}

// Symbol marks a Ruby Symbol; JSON renders it as a string.
type Symbol string

// Present mirrors ActiveSupport's Object#present?.
func Present(v any) bool { return !Blank(v) }

// Blank mirrors ActiveSupport's Object#blank?.
func Blank(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case string:
		return BlankString(x)
	case Symbol:
		return BlankString(string(x))
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case *Map:
		return x == nil || x.Len() == 0
	}
	return false
}

// BlankString is String#blank?: empty or only whitespace.
func BlankString(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Presence returns v when present, else nil.
func Presence(v any) any {
	if Present(v) {
		return v
	}
	return nil
}

// Truthy is Ruby truthiness: only nil and false are falsy.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	}
	return true
}

// ToS mirrors Ruby #to_s for JSON-like values.
func ToS(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case Symbol:
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case float64:
		return FloatToS(x)
	case civil.Date:
		return x.ISO()
	case []any, *Map:
		return Inspect(v)
	}
	return fmt.Sprint(v)
}

// Inspect renders a Ruby #inspect-like string (used where Ruby would call
// to_s on an Array or Hash).
func Inspect(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = Inspect(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *Map:
		parts := []string{}
		x.Each(func(k string, e any) { parts = append(parts, strconv.Quote(k)+"=>"+Inspect(e)) })
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ToS(v)
}

// FloatToS mirrors Ruby's Float#to_s.
func FloatToS(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	epos := strings.IndexByte(s, 'e')
	mant, expS := s[:epos], s[epos+1:]
	exp, _ := strconv.Atoi(expS)
	digits := strings.Replace(mant, ".", "", 1)
	decpt := exp + 1
	var out string
	if decpt < -3 || decpt > 16 {
		rest := digits[1:]
		if rest == "" {
			rest = "0"
		}
		sign := "+"
		e := decpt - 1
		if e < 0 {
			sign = "-"
			e = -e
		}
		out = fmt.Sprintf("%s.%se%s%02d", digits[:1], rest, sign, e)
	} else if decpt <= 0 {
		out = "0." + strings.Repeat("0", -decpt) + digits
	} else if decpt >= len(digits) {
		out = digits + strings.Repeat("0", decpt-len(digits)) + ".0"
	} else {
		out = digits[:decpt] + "." + digits[decpt:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ToI mirrors Ruby's #to_i: String#to_i parses a leading integer, nil is 0.
func ToI(v any) int {
	switch x := v.(type) {
	case nil:
		return 0
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case bool:
		return 0
	case string:
		return StringToI(x)
	}
	return 0
}

// StringToI is Ruby String#to_i (base 10): skips leading whitespace, accepts
// an optional sign and underscores between digits, stops at the first
// non-digit.
func StringToI(s string) int {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f' || s[i] == '\v') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	n := 0
	seenDigit := false
	for i < len(s) {
		c := s[i]
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			seenDigit = true
			if n > math.MaxInt32*1024 {
				// Ruby has bignums; saturate to keep behaviour sane.
				n = math.MaxInt32 * 1024
			}
		} else if c == '_' && seenDigit && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			// underscore separator
		} else {
			break
		}
		i++
	}
	if neg {
		return -n
	}
	return n
}

// BoolCast mirrors ActiveModel::Type::Boolean#cast: nil and "" are nil;
// the listed false values are false; everything else is true.
func BoolCast(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case string:
		if x == "" {
			return nil
		}
		switch x {
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false
		}
		return true
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case Symbol:
		return BoolCast(string(x))
	}
	return true
}

// SortedKeys returns the keys of m sorted as Ruby sorts symbols/strings.
func SortedKeys(m *Map) []string {
	ks := m.Keys()
	sort.Strings(ks)
	return ks
}

// Now is overridable for tests.
var Now = time.Now

// Strip mirrors String#strip (ASCII whitespace and NUL).
func Strip(s string) string { return strings.Trim(s, " \t\n\v\f\r\x00") }

// Squish mirrors ActiveSupport's String#squish.
func Squish(s string) string { return strings.Join(strings.Fields(s), " ") }

// SplitString mirrors String#split(str) with a literal separator: trailing
// empty fields are removed; " " splits on runs of whitespace (awk style).
func SplitString(s, sep string) []string {
	if sep == " " {
		return strings.Fields(s)
	}
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, sep)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// SplitStringN mirrors String#split(str, limit) for limit > 0.
func SplitStringN(s, sep string, limit int) []string {
	if s == "" {
		return []string{}
	}
	return strings.SplitN(s, sep, limit)
}

// StripAll maps Strip over a slice.
func StripAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = Strip(s)
	}
	return out
}

// ClassName is the Ruby class name of a decoded JSON value (v.class.name).
func ClassName(v any) string {
	switch x := v.(type) {
	case nil:
		return "NilClass"
	case bool:
		if x {
			return "TrueClass"
		}
		return "FalseClass"
	case int, int64:
		return "Integer"
	case float64:
		return "Float"
	case string:
		return "String"
	case []any, []string:
		return "Array"
	case map[string]any, *Map:
		return "Hash"
	}
	return "Object"
}

// RoundFloat ports Float#round(ndigits) for ndigits in 1..14 (round half up,
// with Ruby's overflow/underflow short cuts and its correction step).
func RoundFloat(number float64, ndigits int) float64 {
	if number == 0 || ndigits <= 0 || ndigits > 14 {
		if ndigits <= 0 {
			return math.Round(number)
		}
		return number
	}
	_, binexp := math.Frexp(number)
	var over, under int
	if binexp > 0 {
		over, under = binexp/4, binexp/3+1
	} else {
		over, under = binexp/3-1, binexp/4
	}
	if ndigits >= 17-over {
		return number
	}
	if number > 0 && ndigits < -under {
		return 0
	}
	s := math.Pow(10, float64(ndigits))
	x := number
	f := math.Round(x * s)
	if x > 0 {
		if (f+0.5)/s <= x {
			f++
		}
	} else if (f-0.5)/s >= x {
		f--
	}
	return f / s
}
