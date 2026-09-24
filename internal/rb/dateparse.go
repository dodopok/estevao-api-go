package rb

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

// ErrInvalidDate is Date::Error "invalid date".
var ErrInvalidDate = errors.New("invalid date")

// DateParse ports Date.parse(str, comp) from the date gem (date__parse in
// date_parse.c, then d_new_by_frags in date_core.c). Missing fields are
// completed from today in the process time zone, as Date.today does.
// Dates are proleptic Gregorian (the ITALY reform only matters before
// 1582-10-15).
func DateParse(s string, comp bool) (YMD, error) {
	if n := len(s); n > 128 {
		return YMD{}, errors.New("string length (" + strconv.Itoa(n) + ") exceeds the limit 128")
	}
	return dateParseAt(s, comp, time.Now())
}

func dateParseAt(s string, comp bool, now time.Time) (YMD, error) {
	if n := len(s); n > 128 {
		return YMD{}, errors.New("string length (" + strconv.Itoa(n) + ") exceeds the limit 128")
	}
	return newByFrags(dateUnderscoreParse(s, comp), now)
}

// YMD is a Ruby Date as its calendar fields (Julian before 1582-10-15, as
// with Date::ITALY), which is what its #to_s prints and what ActiveRecord
// writes to a date column.
type YMD struct{ Y, M, D int64 }

// ISO ports Date#iso8601 ("%Y-%m-%d": four-digit years, signed when negative).
func (d YMD) ISO() string {
	y := d.Y
	sign := ""
	if y < 0 {
		sign, y = "-", -y
	}
	ys := strconv.FormatInt(y, 10)
	for len(ys) < 4 {
		ys = "0" + ys
	}
	return sign + ys + "-" + pad2(d.M) + "-" + pad2(d.D)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// Civil converts to a civil.Date (proleptic Gregorian labels), false when
// the year is outside what civil.Date holds.
func (d YMD) Civil() (civil.Date, bool) {
	if d.Y < 1 || d.Y > 9999 {
		return 0, false
	}
	return civil.New(int(d.Y), int(d.M), int(d.D))
}

// --- Julian Day arithmetic with the ITALY reform (date_core.c) -------------

const italy = 2299161 // first Gregorian day: 1582-10-15

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// civilToJD ports c_civil_to_jd with Date::ITALY: Gregorian from the
// reform on, Julian before it.
func civilToJD(y, m, d int64) (int64, bool) {
	if m <= 2 {
		y--
		m += 12
	}
	a := floorDiv(y, 100)
	b := 2 - a + floorDiv(a, 4)
	jd := int64(floorFloat(365.25*float64(y+4716))) + int64(floorFloat(30.6001*float64(m+1))) + d + b - 1524
	if jd < italy {
		jd -= b
		return jd, true
	}
	return jd, false
}

func floorFloat(f float64) float64 {
	i := float64(int64(f))
	if f < 0 && i != f {
		return i - 1
	}
	return i
}

// jdToCivil ports c_jd_to_civil with Date::ITALY.
func jdToCivil(jd int64) (int64, int64, int64) {
	var a int64
	if jd < italy {
		a = jd
	} else {
		x := int64(floorFloat((float64(jd) - 1867216.25) / 36524.25))
		a = jd + 1 + x - int64(floorFloat(float64(x)/4.0))
	}
	b := a + 1524
	c := int64(floorFloat((float64(b) - 122.1) / 365.25))
	d := int64(floorFloat(365.25 * float64(c)))
	e := int64(floorFloat(float64(b-d) / 30.6001))
	dom := b - d - int64(floorFloat(30.6001*float64(e)))
	var m, y int64
	if e <= 13 {
		m = e - 1
		y = c - 4716
	} else {
		m = e - 13
		y = c - 4715
	}
	return y, m, dom
}

func ymdOf(jd int64) YMD { y, m, d := jdToCivil(jd); return YMD{y, m, d} }

func lastDayOfMonth(y, m int64) int64 {
	for d := int64(31); d >= 28; d-- {
		jd, _ := civilToJD(y, m, d)
		if yy, mm, dd := jdToCivil(jd); yy == y && mm == m && dd == d {
			return d
		}
	}
	return 28
}

// dateFrags is the hash date__parse returns (only the fields Date uses).
type dateFrags map[string]int64

const (
	hasAlpha = 1 << iota
	hasDigit
	hasDash
	hasDot
	hasSlash
)

func dpClass(s string) int {
	f := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			f |= hasAlpha
		case c >= '0' && c <= '9':
			f |= hasDigit
		case c == '-':
			f |= hasDash
		case c == '.':
			f |= hasDot
		case c == '/':
			f |= hasSlash
		}
	}
	return f
}

const (
	dpAbbrDays   = "sun|mon|tue|wed|thu|fri|sat"
	dpAbbrMonths = "jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec"
	dpNumber     = `(?<!\d)\d`
	dpEra        = `(c(?:e|\.e\.)|b(?:ce|\.c\.e\.)|a(?:d|\.d\.)|b(?:c|\.c\.))`
)

var (
	dpClean = rx.MustCompile(`[^-+',./:@[:alnum:]\[\]]+`)
	dpDay   = rx.MustCompile(`\b(`+dpAbbrDays+`)[^-/\d\s]*`, "i")
	dpTime  = rx.MustCompile(`(`+dpNumber+`+\s*(?:(?::\s*\d+(?:\s*:\s*\d+(?:[,.]\d*)?)?|h(?:\s*\d+m?(?:\s*\d+s?)?)?)(?:\s*[ap](?:m\b|\.m\.))?|[ap](?:m\b|\.m\.)))`+
		`(?:\s*((?:gmt|utc?)?[-+]\d+(?:[,.:]\d+(?::\d+)?)?|(?-i:[[:alpha:].\s]+)(?:standard|daylight)\stime\b|(?-i:[[:alpha:]]+)(?:\sdst)?\b))?`, "i")
	dpTime2  = rx.MustCompile(`\A(\d+)h?(?:\s*:?\s*(\d+)m?(?:\s*:?\s*(\d+)(?:[,.](\d+))?s?)?)?(?:\s*([ap])(?:m\b|\.m\.))?`, "i")
	dpEU     = rx.MustCompile(`('?`+dpNumber+`+)[^-\d\s]*\s*(`+dpAbbrMonths+`)[^-\d\s']*(?:\s*(?:\b`+dpEra+`(?!(?<!\.)[a-z]))?\s*('?-?\d+(?:(?:st|nd|rd|th)\b)?))?`, "i")
	dpUS     = rx.MustCompile(`\b(`+dpAbbrMonths+`)[^-\d\s']*\s*('?\d+)[^-\d\s']*(?:(?>\s*),?(?>\s*)`+dpEra+`?\s*('?-?\d+))?`, "i")
	dpISO    = rx.MustCompile(`('?[-+]?` + dpNumber + `+)-(\d+)-('?-?\d+)`)
	dpISO21  = rx.MustCompile(`\b(\d{2}|\d{4})?-?w(\d{2})(?:-?(\d))?\b`, "i")
	dpISO22  = rx.MustCompile(`-w-(\d)\b`, "i")
	dpISO23  = rx.MustCompile(`--(\d{2})?-(\d{2})\b`)
	dpISO24  = rx.MustCompile(`--(\d{2})(\d{2})?\b`)
	dpISO25a = rx.MustCompile(`[,.](\d{2}|\d{4})-\d{3}\b`)
	dpISO25  = rx.MustCompile(`\b(\d{2}|\d{4})-(\d{3})\b`)
	dpISO26a = rx.MustCompile(`\d-\d{3}\b`)
	dpISO26  = rx.MustCompile(`\b-(\d{3})\b`)
	dpJIS    = rx.MustCompile(`\b([mtshr])(\d+)\.(\d+)\.(\d+)`, "i")
	dpVMS11  = rx.MustCompile(`('?-?`+dpNumber+`+)-(`+dpAbbrMonths+`)[^-/.]*-('?-?\d+)`, "i")
	dpVMS12  = rx.MustCompile(`\b(`+dpAbbrMonths+`)[^-/.]*-('?-?\d+)(?:-('?-?\d+))?`, "i")
	dpSla    = rx.MustCompile(`('?-?`+dpNumber+`+)/\s*('?\d+)(?:\D\s*('?-?\d+))?`, "i")
	dpDot    = rx.MustCompile(`('?-?`+dpNumber+`+)\.\s*('?\d+)\.\s*('?-?\d+)`, "i")
	dpYear   = rx.MustCompile(`'(\d+)\b`)
	dpMon    = rx.MustCompile(`\b(`+dpAbbrMonths+`)\S*`, "i")
	dpMday   = rx.MustCompile(`(`+dpNumber+`+)(st|nd|rd|th)\b`, "i")
	dpDDD    = rx.MustCompile(`([-+]?)(`+dpNumber+`{2,14})(?:\s*t?\s*(\d{2,6})?(?:[,.](\d*))?)?(?:\s*(z\b|[-+]\d{1,4}\b|\[[-+]?\d[^\]]*\]))?`, "i")
	dpBC     = rx.MustCompile(`\b(bc\b|bce\b|b\.c\.|b\.c\.e\.)`, "i")
	dpFrag   = rx.MustCompile(`\A\s*(\d{1,2})\s*\z`, "i")
)

type dpState struct {
	str  []rune
	h    dateFrags
	comp *bool // _comp: nil (absent), true, false
	bc   bool
	// The two non-integer fields of the hash, which only Time.parse reads.
	zone *string
	frac *big.Rat
}

// secFraction ports Rational(str2num(s), 10 ** s.length).
func secFraction(s string) *big.Rat {
	num, ok := new(big.Int).SetString(s, 10)
	if !ok {
		num = big.NewInt(0)
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(s))), nil)
	return new(big.Rat).SetFrac(num, den)
}

func (st *dpState) s() string { return string(st.str) }

// subs ports SUBS: on a match, blank the matched span and run cb.
func (st *dpState) subs(re *rx.Regexp, cb func(m *rx.Match)) bool {
	m := re.Find(st.s())
	if m == nil {
		return false
	}
	b, e := m.Begin(), m.End()
	out := make([]rune, 0, len(st.str))
	out = append(out, st.str[:b]...)
	out = append(out, ' ')
	out = append(out, st.str[e:]...)
	cb(m)
	st.str = out
	return true
}

// dpToI ports String#to_i on a (signed) digit string; huge values saturate.
func dpToI(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimLeft(s, "+"), 10, 64)
	if err != nil {
		if strings.HasPrefix(s, "-") {
			return -1 << 62
		}
		return 1 << 62
	}
	return n
}

func dpDayNum(s string) int64 {
	s = strings.ToLower(s)
	for i, d := range strings.Split(dpAbbrDays, "|") {
		if strings.HasPrefix(s, d) {
			return int64(i)
		}
	}
	return 7
}

func dpMonNum(s string) int64 {
	s = strings.ToLower(s)
	for i, d := range strings.Split(dpAbbrMonths, "|") {
		if strings.HasPrefix(s, d) {
			return int64(i + 1)
		}
	}
	return 13
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// s3e ports the y/m/d disambiguation of date_parse.c. nil strings are
// passed as "" with the matching has* flag false.
func (st *dpState) s3e(y, m, d *string, bc bool) {
	c := (*bool)(nil)
	if y != nil && m != nil && d == nil {
		y, m, d = d, y, m
	}
	if y == nil {
		if d != nil && len(*d) > 2 {
			y, d = d, nil
		}
		if d != nil && len(*d) > 0 && (*d)[0] == '\'' {
			y, d = d, nil
		}
	}
	if y != nil {
		s := *y
		i := 0
		for i < len(s) && s[i] != '-' && s[i] != '+' && !isDigitByte(s[i]) {
			i++
		}
		if i < len(s) {
			bp := i
			if s[i] == '-' || s[i] == '+' {
				i++
			}
			for i < len(s) && isDigitByte(s[i]) {
				i++
			}
			if i < len(s) { // trailing characters after the number
				nd := s[bp:i]
				y, d = d, &nd
			}
		}
	}
	if m != nil && ((*m != "" && (*m)[0] == '\'') || len(*m) > 2) {
		y, m, d = m, d, y
	}
	if d != nil && ((*d != "" && (*d)[0] == '\'') || len(*d) > 2) {
		y, d = d, y
	}
	if y != nil {
		s := *y
		i := 0
		for i < len(s) && s[i] != '-' && s[i] != '+' && !isDigitByte(s[i]) {
			i++
		}
		if i < len(s) {
			bp := i
			sign := false
			if s[i] == '-' || s[i] == '+' {
				i++
				sign = true
			}
			f := false
			if sign {
				c = &f
			}
			start := i
			for i < len(s) && isDigitByte(s[i]) {
				i++
			}
			if i-start > 2 {
				c = &f
			}
			st.h["year"] = dpToI(s[bp:i])
		}
	}
	if bc {
		st.bc = true
	}
	digits := func(s string) (int64, bool) {
		i := 0
		for i < len(s) && !isDigitByte(s[i]) {
			i++
		}
		if i >= len(s) {
			return 0, false
		}
		j := i
		for j < len(s) && isDigitByte(s[j]) {
			j++
		}
		return dpToI(s[i:j]), true
	}
	if m != nil {
		if n, ok := digits(*m); ok {
			st.h["mon"] = n
		}
	}
	if d != nil {
		if n, ok := digits(*d); ok {
			st.h["mday"] = n
		}
	}
	if c != nil {
		st.comp = c
	}
}

func grp(m *rx.Match, i int) *string {
	s, ok := m.Group(i)
	if !ok {
		return nil
	}
	return &s
}

func dateUnderscoreParse(input string, comp bool) dateFrags {
	return dateUnderscoreParseState(input, comp).h
}

func dateUnderscoreParseState(input string, comp bool) *dpState {
	st := &dpState{str: []rune(dpClean.Gsub(input, " ")), h: dateFrags{}}
	st.comp = &comp
	has := func(x int) bool { return dpClass(st.s())&x == x }

	if has(hasAlpha) {
		st.subs(dpDay, func(m *rx.Match) { st.h["wday"] = dpDayNum(m.G(1)) })
	}
	if has(hasDigit) {
		st.subs(dpTime, func(m *rx.Match) {
			if t := dpTime2.Find(m.G(1)); t != nil {
				h := dpToI(t.G(1))
				if p, ok := t.Group(5); ok {
					h %= 12
					if p == "P" || p == "p" {
						h += 12
					}
				}
				st.h["hour"] = h
				if v, ok := t.Group(2); ok {
					st.h["min"] = dpToI(v)
				}
				if v, ok := t.Group(3); ok {
					st.h["sec"] = dpToI(v)
				}
				if v, ok := t.Group(4); ok {
					st.frac = secFraction(v)
				}
			}
			if z, ok := m.Group(2); ok {
				st.zone = &z
			}
		})
	}
	era := func(m *rx.Match, i int) bool {
		b, ok := m.Group(i)
		return ok && (b[0] == 'B' || b[0] == 'b')
	}
	monStr := func(s string) *string { v := strconv.FormatInt(dpMonNum(s), 10); return &v }
	ok := false
	switch {
	case has(hasAlpha|hasDigit) && st.subs(dpEU, func(m *rx.Match) { st.s3e(grp(m, 4), monStr(m.G(2)), grp(m, 1), era(m, 3)) }):
		ok = true
	case has(hasAlpha|hasDigit) && st.subs(dpUS, func(m *rx.Match) { st.s3e(grp(m, 4), monStr(m.G(1)), grp(m, 2), era(m, 3)) }):
		ok = true
	case has(hasDigit|hasDash) && st.subs(dpISO, func(m *rx.Match) { st.s3e(grp(m, 1), grp(m, 2), grp(m, 3), false) }):
		ok = true
	case has(hasDigit|hasDot) && st.subs(dpJIS, func(m *rx.Match) {
		st.h["year"] = dpToI(m.G(2)) + map[byte]int64{'m': 1867, 't': 1911, 's': 1925, 'h': 1988, 'r': 2018}[strings.ToLower(m.G(1))[0]]
		st.h["mon"], st.h["mday"] = dpToI(m.G(3)), dpToI(m.G(4))
	}):
		ok = true
	case has(hasAlpha|hasDigit|hasDash) && (st.subs(dpVMS11, func(m *rx.Match) { st.s3e(grp(m, 3), monStr(m.G(2)), grp(m, 1), false) }) ||
		st.subs(dpVMS12, func(m *rx.Match) { st.s3e(grp(m, 3), monStr(m.G(1)), grp(m, 2), false) })):
		ok = true
	case has(hasDigit|hasSlash) && st.subs(dpSla, func(m *rx.Match) { st.s3e(grp(m, 1), grp(m, 2), grp(m, 3), false) }):
		ok = true
	case has(hasDigit|hasDot) && st.subs(dpDot, func(m *rx.Match) { st.s3e(grp(m, 1), grp(m, 2), grp(m, 3), false) }):
		ok = true
	case has(hasDigit) && st.iso2():
		ok = true
	case has(hasDigit) && st.subs(dpYear, func(m *rx.Match) { st.h["year"] = dpToI(m.G(1)) }):
		ok = true
	case has(hasAlpha) && st.subs(dpMon, func(m *rx.Match) { st.h["mon"] = dpMonNum(m.G(1)) }):
		ok = true
	case has(hasDigit) && st.subs(dpMday, func(m *rx.Match) { st.h["mday"] = dpToI(m.G(1)) }):
		ok = true
	case has(hasDigit) && st.subs(dpDDD, st.ddd):
		ok = true
	}
	_ = ok
	if has(hasAlpha) {
		st.subs(dpBC, func(*rx.Match) { st.bc = true })
	}
	if has(hasDigit) {
		st.subs(dpFrag, func(m *rx.Match) {
			n := dpToI(m.G(1))
			_, hasHour := st.h["hour"]
			_, hasMday := st.h["mday"]
			if hasHour && !hasMday && n >= 1 && n <= 31 {
				st.h["mday"] = n
			}
			_, hasHour = st.h["hour"]
			_, hasMday = st.h["mday"]
			if hasMday && !hasHour && n >= 0 && n <= 24 {
				st.h["hour"] = n
			}
		})
	}
	if st.bc {
		for _, k := range []string{"cwyear", "year"} {
			if y, ok := st.h[k]; ok {
				st.h[k] = -y + 1
			}
		}
	}
	if st.comp != nil && *st.comp {
		for _, k := range []string{"cwyear", "year"} {
			if y, ok := st.h[k]; ok && y >= 0 && y <= 99 {
				if y >= 69 {
					st.h[k] = y + 1900
				} else {
					st.h[k] = y + 2000
				}
			}
		}
	}
	return st
}

func (st *dpState) iso2() bool {
	return st.subs(dpISO21, func(m *rx.Match) {
		if v, ok := m.Group(1); ok {
			st.h["cwyear"] = dpToI(v)
		}
		st.h["cweek"] = dpToI(m.G(2))
		if v, ok := m.Group(3); ok {
			st.h["cwday"] = dpToI(v)
		}
	}) || st.subs(dpISO22, func(m *rx.Match) { st.h["cwday"] = dpToI(m.G(1)) }) ||
		st.subs(dpISO23, func(m *rx.Match) {
			if v, ok := m.Group(1); ok {
				st.h["mon"] = dpToI(v)
			}
			st.h["mday"] = dpToI(m.G(2))
		}) || st.subs(dpISO24, func(m *rx.Match) {
		st.h["mon"] = dpToI(m.G(1))
		if v, ok := m.Group(2); ok {
			st.h["mday"] = dpToI(v)
		}
	}) || (dpISO25a.Find(st.s()) == nil && st.subs(dpISO25, func(m *rx.Match) {
		st.h["year"], st.h["yday"] = dpToI(m.G(1)), dpToI(m.G(2))
	})) || (dpISO26a.Find(st.s()) == nil && st.subs(dpISO26, func(m *rx.Match) { st.h["yday"] = dpToI(m.G(1)) }))
}

func n2i(s string, f, w int) int64 {
	var v int64
	for i := f; i < f+w; i++ {
		v = v*10 + int64(s[i]-'0')
	}
	return v
}

// ddd ports parse_ddd_cb (the time and zone parts only matter for which
// frags are present).
func (st *dpState) ddd(m *rx.Match) {
	s1, s2 := m.G(1), m.G(2)
	s3, has3 := m.Group(3)
	_, has4 := m.Group(4)
	timeOnly := !has3 && has4
	neg := s1 == "-"
	year := func(y int64) int64 {
		if neg {
			return -y
		}
		return y
	}
	f := false
	l2 := len(s2)
	switch l2 {
	case 2:
		if timeOnly {
			st.h["sec"] = n2i(s2, l2-2, 2)
		} else {
			st.h["mday"] = n2i(s2, 0, 2)
		}
	case 4:
		if timeOnly {
			st.h["sec"], st.h["min"] = n2i(s2, l2-2, 2), n2i(s2, l2-4, 2)
		} else {
			st.h["mon"], st.h["mday"] = n2i(s2, 0, 2), n2i(s2, 2, 2)
		}
	case 6:
		if timeOnly {
			st.h["sec"], st.h["min"], st.h["hour"] = n2i(s2, l2-2, 2), n2i(s2, l2-4, 2), n2i(s2, l2-6, 2)
		} else {
			st.h["year"], st.h["mon"], st.h["mday"] = year(n2i(s2, 0, 2)), n2i(s2, 2, 2), n2i(s2, 4, 2)
		}
	case 8, 10, 12, 14:
		if timeOnly {
			st.h["sec"], st.h["min"], st.h["hour"], st.h["mday"] = n2i(s2, l2-2, 2), n2i(s2, l2-4, 2), n2i(s2, l2-6, 2), n2i(s2, l2-8, 2)
			if l2 >= 10 {
				st.h["mon"] = n2i(s2, l2-10, 2)
			}
			if l2 == 12 {
				st.h["year"] = year(n2i(s2, l2-12, 2))
			}
			if l2 == 14 {
				st.h["year"] = year(n2i(s2, l2-14, 4))
				st.comp = &f
			}
		} else {
			st.h["year"], st.h["mon"], st.h["mday"] = year(n2i(s2, 0, 4)), n2i(s2, 4, 2), n2i(s2, 6, 2)
			if l2 >= 10 {
				st.h["hour"] = n2i(s2, 8, 2)
			}
			if l2 >= 12 {
				st.h["min"] = n2i(s2, 10, 2)
			}
			if l2 >= 14 {
				st.h["sec"] = n2i(s2, 12, 2)
			}
			st.comp = &f
		}
	case 3:
		if timeOnly {
			st.h["sec"], st.h["min"] = n2i(s2, l2-2, 2), n2i(s2, l2-3, 1)
		} else {
			st.h["yday"] = n2i(s2, 0, 3)
		}
	case 5:
		if timeOnly {
			st.h["sec"], st.h["min"], st.h["hour"] = n2i(s2, l2-2, 2), n2i(s2, l2-4, 2), n2i(s2, l2-5, 1)
		} else {
			st.h["year"], st.h["yday"] = year(n2i(s2, 0, 2)), n2i(s2, 2, 3)
		}
	case 7:
		if timeOnly {
			st.h["sec"], st.h["min"], st.h["hour"], st.h["mday"] = n2i(s2, l2-2, 2), n2i(s2, l2-4, 2), n2i(s2, l2-6, 2), n2i(s2, l2-7, 1)
		} else {
			st.h["year"], st.h["yday"] = year(n2i(s2, 0, 4)), n2i(s2, 4, 3)
		}
	}
	if has3 {
		l3 := len(s3)
		if has4 {
			if l3 == 2 || l3 == 4 || l3 == 6 {
				st.h["sec"] = n2i(s3, l3-2, 2)
				if l3 >= 4 {
					st.h["min"] = n2i(s3, l3-4, 2)
				}
				if l3 >= 6 {
					st.h["hour"] = n2i(s3, l3-6, 2)
				}
			}
		} else if l3 == 2 || l3 == 4 || l3 == 6 {
			st.h["hour"] = n2i(s3, 0, 2)
			if l3 >= 4 {
				st.h["min"] = n2i(s3, 2, 2)
			}
			if l3 >= 6 {
				st.h["sec"] = n2i(s3, 4, 2)
			}
		}
	}
	if s4, ok := m.Group(4); ok {
		st.frac = secFraction(s4)
	}
	if s5, ok := m.Group(5); ok {
		zone := s5
		if strings.HasPrefix(s5, "[") {
			inner := s5[1 : len(s5)-1]
			if i := strings.IndexByte(inner, ':'); i >= 0 {
				zone = inner[i+1:]
			} else {
				zone = inner
			}
		}
		st.zone = &zone
	}
}

// --- d_new_by_frags ---------------------------------------------------------

var fragTables = []struct {
	kind   string
	fields []string
}{
	{"time", []string{"hour", "min", "sec"}},
	{"", []string{"jd"}},
	{"ordinal", []string{"year", "yday", "hour", "min", "sec"}},
	{"civil", []string{"year", "mon", "mday", "hour", "min", "sec"}},
	{"commercial", []string{"cwyear", "cweek", "cwday", "hour", "min", "sec"}},
	{"wday", []string{"wday", "hour", "min", "sec"}},
	{"wnum0", []string{"year", "wnum0", "wday", "hour", "min", "sec"}},
	{"wnum1", []string{"year", "wnum1", "wday", "hour", "min", "sec"}},
	{"", []string{"cwyear", "cweek", "wday", "hour", "min", "sec"}},
	{"", []string{"year", "wnum0", "cwday", "hour", "min", "sec"}},
	{"", []string{"year", "wnum1", "cwday", "hour", "min", "sec"}},
}

func today(now time.Time) int64 {
	n := now
	if now.Location() != time.UTC {
		n = now.In(time.Local)
	}
	jd, _ := civilToJD(int64(n.Year()), int64(n.Month()), int64(n.Day()))
	return jd
}

// commercialOf ports c_jd_to_commercial.
func commercialOf(jd int64) (int64, int64, int64) {
	y, _, _ := jdToCivil(jd - 3)
	a := y
	j := commercialToJD(a+1, 1, 1)
	if jd >= j {
		a++
	}
	w := 1 + floorDiv(jd-commercialToJD(a, 1, 1), 7)
	d := (jd + 1) % 7
	if d == 0 {
		d = 7
	}
	return a, w, d
}

// commercialToJD ports c_commercial_to_jd.
func commercialToJD(y, w, d int64) int64 {
	fdoy, _ := civilToJD(y, 1, 1)
	rjd2 := fdoy + 3
	return (rjd2 - floorMod(rjd2-1+1, 7)) + 7*(w-1) + (d - 1)
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

func newByFrags(h dateFrags, now time.Time) (YMD, error) {
	_, hasYday := h["yday"]
	y, hasY := h["year"]
	m, hasM := h["mon"]
	d, hasD := h["mday"]
	if !hasYday && hasY && hasM && hasD {
		if v, ok := validCivil(y, m, d); ok {
			return v, nil
		}
		return YMD{}, ErrInvalidDate
	}
	// rt_complete_frags
	best, bestN := -1, 0
	for i, t := range fragTables {
		n := 0
		for _, f := range t.fields {
			if _, ok := h[f]; ok {
				n++
			}
		}
		if n > bestN {
			best, bestN = i, n
		}
	}
	if best >= 0 && len(fragTables[best].fields) > bestN {
		tjd := today(now)
		ty, tm, td := jdToCivil(tjd)
		switch fragTables[best].kind {
		case "ordinal":
			if _, ok := h["year"]; !ok {
				h["year"] = ty
			}
			if _, ok := h["yday"]; !ok {
				h["yday"] = 1
			}
		case "civil":
			for _, f := range fragTables[best].fields {
				if _, ok := h[f]; ok {
					break
				}
				switch f {
				case "year":
					h[f] = ty
				case "mon":
					h[f] = tm
				case "mday":
					h[f] = td
				case "hour", "min", "sec":
					h[f] = 0
				}
			}
			if _, ok := h["mon"]; !ok {
				h["mon"] = 1
			}
			if _, ok := h["mday"]; !ok {
				h["mday"] = 1
			}
		case "commercial":
			cy, cw, cwd := commercialOf(tjd)
			for _, f := range fragTables[best].fields {
				if _, ok := h[f]; ok {
					break
				}
				switch f {
				case "cwyear":
					h[f] = cy
				case "cweek":
					h[f] = cw
				case "cwday":
					h[f] = cwd
				case "hour", "min", "sec":
					h[f] = 0
				}
			}
			if _, ok := h["cweek"]; !ok {
				h["cweek"] = 1
			}
			if _, ok := h["cwday"]; !ok {
				h["cwday"] = 1
			}
		case "wday":
			wd := floorMod(tjd+1, 7)
			return ymdOf(tjd - wd + h["wday"]), nil
		case "wnum0", "wnum1":
			f := int64(0)
			if fragTables[best].kind == "wnum1" {
				f = 1
			}
			wk, wd := weeknumOf(tjd, f)
			for _, fl := range fragTables[best].fields {
				if _, ok := h[fl]; ok {
					break
				}
				switch fl {
				case "year":
					h[fl] = ty
				case "wnum0", "wnum1":
					h[fl] = wk
				case "wday":
					h[fl] = floorMod(tjd+1, 7)
					_ = wd
				case "hour", "min", "sec":
					h[fl] = 0
				}
			}
			key := fragTables[best].kind
			if _, ok := h[key]; !ok {
				h[key] = 0
			}
			if _, ok := h["wday"]; !ok {
				h["wday"] = f
			}
		}
	}
	// rt__valid_date_frags_p
	if y, ok := h["year"]; ok {
		if yd, ok := h["yday"]; ok {
			if v, ok := validOrdinal(y, yd); ok {
				return v, nil
			}
		}
	}
	if y, ok := h["year"]; ok {
		m, okm := h["mon"]
		d, okd := h["mday"]
		if okm && okd {
			if v, ok := validCivil(y, m, d); ok {
				return v, nil
			}
		}
	}
	wday, okw := h["cwday"]
	if !okw {
		if w, ok := h["wday"]; ok {
			wday, okw = w, true
			if wday == 0 {
				wday = 7
			}
		}
	}
	cy, oky := h["cwyear"]
	cw, okc := h["cweek"]
	if okw && oky && okc {
		if v, ok := validCommercial(cy, cw, wday); ok {
			return v, nil
		}
	}
	for _, f := range []int64{0, 1} {
		key := "wnum0"
		if f == 1 {
			key = "wnum1"
		}
		w, okw := h["wday"]
		if !okw {
			if cw, ok := h["cwday"]; ok {
				w, okw = cw, true
				if f == 0 && w == 7 {
					w = 0
				}
			}
		}
		if okw && f == 1 {
			w = floorMod(w-1, 7)
		}
		week, okk := h[key]
		y, oky := h["year"]
		if okw && okk && oky {
			if v, ok := validWeeknum(y, week, w, f); ok {
				return v, nil
			}
		}
	}
	return YMD{}, ErrInvalidDate
}

// weeknumToJD ports c_weeknum_to_jd.
func weeknumToJD(y, w, d, f int64) int64 {
	fdoy, _ := civilToJD(y, 1, 1)
	rjd2 := fdoy + 6
	return (rjd2 - floorMod((rjd2-f)+1, 7) - 7) + 7*w + d
}

// weeknumOf ports c_jd_to_weeknum (week and day).
func weeknumOf(jd, f int64) (int64, int64) {
	y, _, _ := jdToCivil(jd)
	fdoy, _ := civilToJD(y, 1, 1)
	rjd := fdoy + 6
	j := jd - (rjd - floorMod((rjd-f)+1, 7)) + 7
	return floorDiv(j, 7), floorMod(j, 7)
}

// validWeeknum ports valid_weeknum_p.
func validWeeknum(y, w, d, f int64) (YMD, bool) {
	if d < 0 {
		d += 7
	}
	if w < 0 {
		lastW, _ := weeknumOf(weeknumToJD(y+1, 1, f, f)+7*-1, f)
		w = lastW + w + 1
	}
	jd := weeknumToJD(y, w, d, f)
	ry, _, _ := jdToCivil(jd)
	rw, rd := weeknumOf(jd, f)
	if ry != y || rw != w || rd != d {
		return YMD{}, false
	}
	return ymdOf(jd), true
}

// validCivil ports valid_civil_p (negative month and day count from the
// end; days in the 1582 reform gap are invalid).
func validCivil(y, m, d int64) (YMD, bool) {
	if m < 0 {
		m += 13
	}
	if m < 1 || m > 12 {
		return YMD{}, false
	}
	if d < 0 {
		d = lastDayOfMonth(y, m) + d + 1
	}
	if d < 1 || d > 31 {
		return YMD{}, false
	}
	jd, _ := civilToJD(y, m, d)
	if got := ymdOf(jd); got != (YMD{y, m, d}) {
		return YMD{}, false
	}
	return YMD{y, m, d}, true
}

func validOrdinal(y, yd int64) (YMD, bool) {
	first, _ := civilToJD(y, 1, 1)
	next, _ := civilToJD(y+1, 1, 1)
	n := next - first
	if yd < 0 {
		yd = n + yd + 1
	}
	if yd < 1 || yd > n {
		return YMD{}, false
	}
	return ymdOf(first + yd - 1), true
}

func validCommercial(y, w, d int64) (YMD, bool) {
	if d < 0 {
		d += 8
	}
	if w < 0 {
		_, lw, _ := commercialOf(commercialToJD(y+1, 1, 1) - 1)
		w = lw + w + 1
	}
	jd := commercialToJD(y, w, d)
	if cy, cw, cd := commercialOf(jd); cy != y || cw != w || cd != d {
		return YMD{}, false
	}
	return ymdOf(jd), true
}

// CastDate ports ActiveModel::Type::Date#cast for a String: the ISO fast
// path, else Date._parse(string, false) without completion; nil (false)
// when the fields are missing or not a valid date.
func CastDate(s string) (YMD, bool) {
	if s == "" {
		return YMD{}, false
	}
	var y, m, d int64
	var ok bool
	if len(s) == 10 && s[4] == '-' && s[7] == '-' && allDigits(s[:4]) && allDigits(s[5:7]) && allDigits(s[8:]) {
		y, m, d = dpToI(s[:4]), dpToI(s[5:7]), dpToI(s[8:])
		if v, valid := validCivil(y, m, d); valid && !(y == 0 && m == 0 && d == 0) {
			return v, true
		}
	}
	h := dateUnderscoreParse(s, false)
	if y, ok = h["year"]; !ok {
		return YMD{}, false
	}
	if m, ok = h["mon"]; !ok {
		return YMD{}, false
	}
	if d, ok = h["mday"]; !ok {
		return YMD{}, false
	}
	if y == 0 && m == 0 && d == 0 {
		return YMD{}, false
	}
	return validCivil(y, m, d)
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigitByte(s[i]) {
			return false
		}
	}
	return s != ""
}

// JD is the Julian Day Number of the date (Julian calendar before the
// ITALY reform, as with Ruby's Date).
func (d YMD) JD() int64 {
	jd, _ := civilToJD(d.Y, d.M, d.D)
	return jd
}

// YMDFromJD is the calendar date of a Julian Day Number.
func YMDFromJD(jd int64) YMD { return ymdOf(jd) }

// AddDays ports Date#+ / Date#- with an integer.
func (d YMD) AddDays(n int64) YMD { return ymdOf(d.JD() + n) }

// Wday ports Date#wday (0 is Sunday).
func (d YMD) Wday() int64 { return floorMod(d.JD()+1, 7) }

// Cweek ports Date#cweek.
func (d YMD) Cweek() int64 { _, w, _ := commercialOf(d.JD()); return w }
