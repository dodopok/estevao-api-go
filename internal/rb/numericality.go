package rb

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	integerRegex     = regexp.MustCompile(`\A[+-]?[0-9]+\z`)
	hexadecimalRegex = regexp.MustCompile(`\A[+-]?0[xX]`)
	kernelFloatRegex = regexp.MustCompile(`\A[+-]?([0-9]+(_[0-9]+)*)?(\.[0-9]+(_[0-9]+)*)?([eE][+-]?[0-9]+(_[0-9]+)*)?\z`)
)

// KernelFloat ports Kernel#Float on a decimal String (hexadecimal literals
// are handled by the caller): ASCII whitespace around the number, single
// underscores between digits, no trailing dot.
func KernelFloat(s string) (float64, bool) {
	t := strings.Trim(s, " \t\n\v\f\r")
	m := kernelFloatRegex.FindStringSubmatch(t)
	if m == nil || (m[1] == "" && m[3] == "") {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(t, "_", ""), 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, false
		}
	}
	return f, true
}

// Numericality ports ActiveModel's NumericalityValidator for the options
// the models use (only_integer, greater_than_or_equal_to), given the value
// before type cast. It returns the error message, or "" when valid.
func Numericality(raw any, onlyInteger bool, gte *int64) string {
	var value *big.Rat
	switch x := raw.(type) {
	case int:
		value = new(big.Rat).SetInt64(int64(x))
	case int64:
		value = new(big.Rat).SetInt64(x)
	case float64:
		if onlyInteger {
			if !integerRegex.MatchString(FloatToS(x)) {
				return "must be an integer"
			}
		}
		value = new(big.Rat)
		if _, ok := value.SetString(strconv.FormatFloat(x, 'g', -1, 64)); !ok {
			return "is not a number"
		}
	case string:
		switch {
		case integerRegex.MatchString(x):
			n, _ := new(big.Int).SetString(strings.TrimPrefix(x, "+"), 10)
			value = new(big.Rat).SetInt(n)
		case hexadecimalRegex.MatchString(x):
			return "is not a number"
		default:
			f, ok := KernelFloat(x)
			if !ok {
				return "is not a number"
			}
			if onlyInteger {
				return "must be an integer"
			}
			value = new(big.Rat)
			if _, ok := value.SetString(strconv.FormatFloat(f, 'g', -1, 64)); !ok {
				value = nil // infinities compare as their sign below
				if f < 0 && gte != nil {
					return "must be greater than or equal to " + strconv.FormatInt(*gte, 10)
				}
				return ""
			}
		}
	default:
		return "is not a number"
	}
	if gte != nil && value.Cmp(new(big.Rat).SetInt64(*gte)) < 0 {
		return "must be greater than or equal to " + strconv.FormatInt(*gte, 10)
	}
	return ""
}
