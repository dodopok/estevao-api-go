package rb

import (
	"errors"
	"strconv"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rx"
)

// The four patterns of date__iso8601 (date_parse.c), tried in order.
var (
	isoExtDatetime = rx.MustCompile(`\A\s*(?:([-+]?\d{2,}|-)-(\d{2})?(?:-(\d{2}))?|` +
		`([-+]?\d{2,})?-(\d{3})|` +
		`(\d{4}|\d{2})?-w(\d{2})-(\d)|` +
		`-w-(\d))` +
		`(?:t` +
		`(\d{2}):(\d{2})(?::(\d{2})(?:[,.](\d+))?)?` +
		`(z|[-+]\d{2}(?::?\d{2})?)?)?\s*\z`, "i")
	isoBasDatetime = rx.MustCompile(`\A\s*(?:([-+]?(?:\d{4}|\d{2})|--)(\d{2}|-)(\d{2})|` +
		`([-+]?(?:\d{4}|\d{2}))(\d{3})|` +
		`-(\d{3})|` +
		`(\d{4}|\d{2})w(\d{2})(\d)|` +
		`-w(\d{2})(\d)|` +
		`-w-(\d))` +
		`(?:t?` +
		`(\d{2})(\d{2})(?:(\d{2})(?:[,.](\d+))?)?` +
		`(z|[-+]\d{2}(?:\d{2})?)?)?\s*\z`, "i")
	isoExtTime = rx.MustCompile(`\A\s*(\d{2}):(\d{2})(?::(\d{2})(?:[,.](\d+))?` +
		`(z|[-+]\d{2}(:?\d{2})?)?)?\s*\z`, "i")
	isoBasTime = rx.MustCompile(`\A\s*(\d{2})(\d{2})(?:(\d{2})(?:[,.](\d+))?` +
		`(z|[-+]\d{2}(\d{2})?)?)?\s*\z`, "i")
)

// DateISO8601 ports Date.iso8601(str): date__iso8601 then d_new_by_frags,
// completing missing fields from today as Date.iso8601 does.
func DateISO8601(s string) (YMD, error) {
	if n := len(s); n > 128 {
		return YMD{}, errors.New("string length (" + strconv.Itoa(n) + ") exceeds the limit 128")
	}
	return newByFrags(dateUnderscoreISO8601(s), time.Now())
}

func compYear69(s string, y int64) int64 {
	if len(s) >= 4 {
		return y
	}
	if y >= 69 {
		return y + 1900
	}
	return y + 2000
}

func dateUnderscoreISO8601(s string) dateFrags {
	h := dateFrags{}
	set := func(k, v string) { h[k] = dpToI(v) }
	g := func(m *rx.Match, i int) (string, bool) { return m.Group(i) }
	if m := isoExtDatetime.Find(s); m != nil {
		if s1, ok := g(m, 1); ok {
			if s3, ok := g(m, 3); ok {
				set("mday", s3)
			}
			if s1 != "-" {
				h["year"] = compYear69(s1, dpToI(s1))
			}
			if s2, ok := g(m, 2); ok {
				set("mon", s2)
			} else if s1 != "-" {
				return dateFrags{}
			}
		} else if s5, ok := g(m, 5); ok {
			set("yday", s5)
			if s4, ok := g(m, 4); ok {
				h["year"] = compYear69(s4, dpToI(s4))
			}
		} else if s8, ok := g(m, 8); ok {
			set("cweek", m.G(7))
			set("cwday", s8)
			if s6, ok := g(m, 6); ok {
				h["cwyear"] = compYear69(s6, dpToI(s6))
			}
		} else if s9, ok := g(m, 9); ok {
			set("cwday", s9)
		}
		if s10, ok := g(m, 10); ok {
			set("hour", s10)
			set("min", m.G(11))
			if s12, ok := g(m, 12); ok {
				set("sec", s12)
			}
		}
		return h
	}
	if m := isoBasDatetime.Find(s); m != nil {
		if s3, ok := g(m, 3); ok {
			s1, s2 := m.G(1), m.G(2)
			set("mday", s3)
			if s1 != "--" {
				h["year"] = compYear69(s1, dpToI(s1))
			}
			if s2 == "-" {
				if s1 != "--" {
					return dateFrags{}
				}
			} else {
				set("mon", s2)
			}
		} else if s5, ok := g(m, 5); ok {
			set("yday", s5)
			s4 := m.G(4)
			h["year"] = compYear69(s4, dpToI(s4))
		} else if s6, ok := g(m, 6); ok {
			set("yday", s6)
		} else if s9, ok := g(m, 9); ok {
			set("cweek", m.G(8))
			set("cwday", s9)
			s7 := m.G(7)
			h["cwyear"] = compYear69(s7, dpToI(s7))
		} else if s11, ok := g(m, 11); ok {
			set("cweek", m.G(10))
			set("cwday", s11)
		} else if s12, ok := g(m, 12); ok {
			set("cwday", s12)
		}
		if s13, ok := g(m, 13); ok {
			set("hour", s13)
			set("min", m.G(14))
			if s15, ok := g(m, 15); ok {
				set("sec", s15)
			}
		}
		return h
	}
	for _, re := range []*rx.Regexp{isoExtTime, isoBasTime} {
		if m := re.Find(s); m != nil {
			set("hour", m.G(1))
			set("min", m.G(2))
			if s3, ok := g(m, 3); ok {
				set("sec", s3)
			}
			return h
		}
	}
	return h
}
