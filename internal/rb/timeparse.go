package rb

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeParse ports Ruby 3.2's Time.parse(s) (no block, so two-digit years
// are completed): Date._parse, then Time.make_time. Times without a zone are
// local to loc, the process time zone (ENV["TZ"]) in Ruby.
func TimeParse(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if n := len(s); n > 128 {
		return time.Time{}, errors.New("string length (" + strconv.Itoa(n) + ") exceeds the limit 128")
	}
	st := dateUnderscoreParseState(s, true)
	return makeTime(s, st.h, st.frac, st.zone, now, loc)
}

func fragPtr(h dateFrags, k string) *int64 {
	if v, ok := h[k]; ok {
		return &v
	}
	return nil
}

func makeTime(input string, h dateFrags, frac *big.Rat, zone *string, now time.Time, loc *time.Location) (time.Time, error) {
	year, yday, mon, day := fragPtr(h, "year"), fragPtr(h, "yday"), fragPtr(h, "mon"), fragPtr(h, "mday")
	hour, min, sec := fragPtr(h, "hour"), fragPtr(h, "min"), fragPtr(h, "sec")
	return makeTimeParts(input, year, yday, mon, day, hour, min, sec, frac, zone, now, loc)
}

func makeTimeParts(input string, year, yday, mon, day, hour, min, sec *int64, frac *big.Rat, zone *string,
	now time.Time, loc *time.Location) (time.Time, error) {
	if year == nil && yday == nil && mon == nil && day == nil && hour == nil && min == nil && sec == nil && frac == nil {
		return time.Time{}, errors.New("no time information in " + Inspect(input))
	}
	var off *int64
	offYear := now.In(loc).Year()
	if year != nil {
		offYear = int(*year)
	}
	if zone != nil {
		off = ZoneOffset(*zone, offYear, loc)
	}
	if yday != nil {
		yd := *yday
		if yd < 1 || yd > 366 {
			return time.Time{}, errors.New("yday " + strconv.FormatInt(yd, 10) + " out of range")
		}
		m, d := (yd-1)/31+1, (yd-1)%31+1
		t, err := makeTimeParts(input, year, nil, &m, &d, hour, min, sec, frac, zone, now, loc)
		if err != nil {
			return t, err
		}
		diff := yd - int64(t.YearDay())
		if diff == 0 {
			return t, nil
		}
		d += diff
		if d > 28 {
			if md := int64(monthDays(int64(offYear), m)); d > md {
				if m++; m > 12 {
					return time.Time{}, errors.New("yday " + strconv.FormatInt(yd, 10) + " out of range")
				}
				d -= md
			}
		}
		return makeTimeParts(input, year, nil, &m, &d, hour, min, sec, frac, zone, now, loc)
	}
	nowLocal := now.In(loc)
	if off != nil {
		_, o := nowLocal.Zone()
		if int64(o) != *off {
			nowLocal = now.In(time.FixedZone("", int(*off)))
		}
	}
	var usecNanos *big.Rat // usec expressed in nanoseconds
	if frac != nil {
		usecNanos = new(big.Rat).Mul(frac, big.NewRat(1_000_000_000, 1))
	}
	y, mo, d, hh, mi, ss := int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)
	has := [6]bool{year != nil, mon != nil, day != nil, hour != nil, min != nil, sec != nil}
	vals := [6]*int64{year, mon, day, hour, min, sec}
	nowVals := [6]int64{int64(nowLocal.Year()), int64(nowLocal.Month()), int64(nowLocal.Day()),
		int64(nowLocal.Hour()), int64(nowLocal.Minute()), int64(nowLocal.Second())}
	defaults := [6]int64{1970, 1, 1, 0, 0, 0}
	out := [6]int64{}
	filling := true
	for i := 0; i < 6; i++ {
		switch {
		case has[i]:
			out[i] = *vals[i]
			filling = false
		case filling:
			out[i] = nowVals[i]
		default:
			out[i] = defaults[i]
		}
	}
	if filling && frac == nil {
		usecNanos = big.NewRat(int64(nowLocal.Nanosecond()/1000*1000), 1)
	}
	y, mo, d, hh, mi, ss = out[0], out[1], out[2], out[3], out[4], out[5]
	if usecNanos == nil {
		usecNanos = new(big.Rat)
	}
	if y != int64(offYear) {
		off = nil
		if zone != nil {
			off = ZoneOffset(*zone, int(y), loc)
		}
	}
	if off != nil {
		y, mo, d, hh, mi, ss = applyOffset(y, mo, d, hh, mi, ss, *off)
		return timeArg(y, mo, d, hh, mi, ss, usecNanos, time.UTC)
	}
	return timeArg(y, mo, d, hh, mi, ss, usecNanos, loc)
}

func monthDays(y, m int64) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func divmod(a, b int64) (int64, int64) {
	q := floorDiv(a, b)
	return q, a - q*b
}

// applyOffset ports Time.apply_offset.
func applyOffset(year, mon, day, hour, min, sec, off int64) (int64, int64, int64, int64, int64, int64) {
	var o int64
	if off < 0 {
		off = -off
		off, o = divmod(off, 60)
		if o != 0 {
			sec += o
			o, sec = divmod(sec, 60)
			off += o
		}
		off, o = divmod(off, 60)
		if o != 0 {
			min += o
			o, min = divmod(min, 60)
			off += o
		}
		off, o = divmod(off, 24)
		if o != 0 {
			hour += o
			o, hour = divmod(hour, 24)
			off += o
		}
		if off != 0 {
			day += off
			if days := int64(monthDays(year, mon)); days < day {
				mon++
				if mon > 12 {
					mon = 1
					year++
				}
				day = 1
			}
		}
	} else if off > 0 {
		off, o = divmod(off, 60)
		if o != 0 {
			sec -= o
			o, sec = divmod(sec, 60)
			off -= o
		}
		off, o = divmod(off, 60)
		if o != 0 {
			min -= o
			o, min = divmod(min, 60)
			off -= o
		}
		off, o = divmod(off, 24)
		if o != 0 {
			hour -= o
			o, hour = divmod(hour, 24)
			off -= o
		}
		if off != 0 {
			day -= off
			if day < 1 {
				mon--
				if mon < 1 {
					year--
					mon = 12
				}
				day = int64(monthDays(year, mon))
			}
		}
	}
	return year, mon, day, hour, min, sec
}

var errOutOfRange = errors.New("argument out of range")

// timeArg ports Time.utc / Time.local argument handling (time_arg and
// validate_vtm): the month-day normalization and the range checks.
func timeArg(y, mon, day, hour, min, sec int64, nanos *big.Rat, loc *time.Location) (time.Time, error) {
	if mon < 0 || mon > 15 || day < 0 || day > 31 || hour < 0 || hour > 31 || min < 0 || min > 63 || sec < 0 || sec > 63 {
		return time.Time{}, errOutOfRange
	}
	switch mon {
	case 2:
		if md := int64(monthDays(y, 2)); day > md {
			day -= md
			mon++
		}
	case 4, 6, 9, 11:
		if day == 31 {
			mon++
			day = 1
		}
	}
	// validate_vtm
	rangeErr := func(name string) (time.Time, error) { return time.Time{}, errors.New(name + " out of range") }
	maxMin, maxSec := int64(59), int64(60)
	if hour == 24 {
		maxMin, maxSec = 0, 0
	}
	switch {
	case mon < 1 || mon > 12:
		return rangeErr("mon")
	case day < 1 || day > 31:
		return rangeErr("mday")
	case hour > 24:
		return rangeErr("hour")
	case min > maxMin:
		return rangeErr("min")
	case sec > maxSec:
		return rangeErr("sec")
	}
	ns := new(big.Int).Quo(nanos.Num(), nanos.Denom()) // floor for non-negative
	if nanos.Sign() < 0 || !ns.IsInt64() || ns.Int64() >= 1_000_000_000 {
		return rangeErr("subsecx")
	}
	return RubyLocalTime(int(y), time.Month(mon), int(day), int(hour), int(min), int(sec), int(ns.Int64()), loc), nil
}

// RubyLocalTime is time.Date resolved the way Ruby's Time.local resolves
// wall-clock times around a DST transition: a time that falls in a gap uses
// the offset in effect before the transition, and an ambiguous time is the
// later of its two instants (Go leaves both choices unspecified).
func RubyLocalTime(y int, mon time.Month, day, hour, min, sec, nsec int, loc *time.Location) time.Time {
	wall := time.Date(y, mon, day, hour, min, sec, nsec, time.UTC)
	if loc == time.UTC {
		return wall
	}
	offsetAt := func(t time.Time) int { _, o := t.In(loc).Zone(); return o }
	before, after := offsetAt(wall.Add(-36*time.Hour)), offsetAt(wall.Add(36*time.Hour))
	var best time.Time
	found := false
	for _, o := range []int{before, after, offsetAt(wall)} {
		t := wall.Add(-time.Duration(o) * time.Second)
		if offsetAt(t) == o && (!found || t.After(best)) {
			best, found = t, true
		}
	}
	if !found {
		best = wall.Add(-time.Duration(before) * time.Second)
	}
	return best.In(loc)
}

var (
	zoneHHMM = regexp.MustCompile(`\A([+-])(\d\d)(:?)(\d\d)(?:(:?)(\d\d))?\z`)
	zoneHH   = regexp.MustCompile(`\A[+-]\d\d\z`)
)

var zoneOffsets = map[string]int64{
	"UTC": 0, "Z": 0, "UT": 0, "GMT": 0,
	"EST": -5, "EDT": -4, "CST": -6, "CDT": -5, "MST": -7, "MDT": -6, "PST": -8, "PDT": -7,
	"A": 1, "B": 2, "C": 3, "D": 4, "E": 5, "F": 6, "G": 7, "H": 8, "I": 9, "K": 10, "L": 11, "M": 12,
	"N": -1, "O": -2, "P": -3, "Q": -4, "R": -5, "S": -6, "T": -7, "U": -8, "V": -9, "W": -10, "X": -11, "Y": -12,
}

// ZoneOffset ports Time.zone_offset (nil when the zone is unknown).
func ZoneOffset(zone string, year int, loc *time.Location) *int64 {
	z := strings.ToUpper(zone)
	var off int64
	if m := zoneHHMM.FindStringSubmatch(z); m != nil && m[3] == m[5] || m != nil && m[6] == "" {
		sign := int64(1)
		if m[1] == "-" {
			sign = -1
		}
		hh, _ := strconv.ParseInt(m[2], 10, 64)
		mm, _ := strconv.ParseInt(m[4], 10, 64)
		ss, _ := strconv.ParseInt(m[6], 10, 64)
		off = sign * ((hh*60+mm)*60 + ss)
		return &off
	}
	if zoneHH.MatchString(z) {
		n, _ := strconv.ParseInt(z, 10, 64)
		off = n * 3600
		return &off
	}
	if v, ok := zoneOffsets[z]; ok {
		off = v * 3600
		return &off
	}
	for _, m := range []time.Month{time.January, time.July} {
		name, o := time.Date(year, m, 1, 0, 0, 0, 0, loc).Zone()
		if strings.ToUpper(name) == z {
			off = int64(o)
			return &off
		}
	}
	return nil
}

// AdvanceYears ports ActiveSupport's `n.years.from_now` on a time in the
// app zone: the calendar year moves and a 29 February becomes the 28th
// (Date#>>), instead of rolling into March like time.AddDate.
func AdvanceYears(t time.Time, n int) time.Time {
	local := t.In(AppZone)
	y, m, d := local.Date()
	y += n
	if dim := monthDays(int64(y), int64(m)); d > dim {
		d = dim
	}
	h, mi, s := local.Clock()
	return RubyLocalTime(y, m, d, h, mi, s, local.Nanosecond(), AppZone)
}
