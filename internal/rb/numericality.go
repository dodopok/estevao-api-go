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

// NumCheck is one comparison option of the numericality validator.
type NumCheck struct {
	Op string // "gt", "gte", "lt", "lte"
	N  int64
}

// GT, GTE, LT and LTE build the comparison options.
func GT(n int64) NumCheck  { return NumCheck{"gt", n} }
func GTE(n int64) NumCheck { return NumCheck{"gte", n} }
func LT(n int64) NumCheck  { return NumCheck{"lt", n} }
func LTE(n int64) NumCheck { return NumCheck{"lte", n} }

var numMessages = map[string]string{
	"gt": "must be greater than ", "gte": "must be greater than or equal to ",
	"lt": "must be less than ", "lte": "must be less than or equal to ",
}

// failed reports the first failing check (Rails checks them in the order
// greater_than, greater_than_or_equal_to, less_than, less_than_or_equal_to,
// and a value can fail at most one of a consistent range).
func failed(cmp func(n int64) int, checks []NumCheck) string {
	for _, op := range []string{"gt", "gte", "lt", "lte"} {
		for _, c := range checks {
			if c.Op != op {
				continue
			}
			r := cmp(c.N)
			if (op == "gt" && r <= 0) || (op == "gte" && r < 0) || (op == "lt" && r >= 0) || (op == "lte" && r > 0) {
				return numMessages[op] + strconv.FormatInt(c.N, 10)
			}
		}
	}
	return ""
}

// Numericality ports ActiveModel's NumericalityValidator for the options
// the models use (only_integer and comparisons), given the value before
// type cast. It returns the error message, or "" when valid.
func Numericality(raw any, onlyInteger bool, checks ...NumCheck) string {
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
				// +/-Infinity compares as its sign.
				sign := int64(1)
				if f < 0 {
					sign = -1
				}
				return failed(func(int64) int { return int(sign) }, checks)
			}
		}
	default:
		return "is not a number"
	}
	return failed(func(n int64) int { return value.Cmp(new(big.Rat).SetInt64(n)) }, checks)
}
