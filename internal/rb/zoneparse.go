package rb

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ZoneParse ports ActiveSupport::TimeZone#parse on loc (Date._parse without
// year completion, then Time.new): missing fields come from now in loc,
// except the day, which is 1 when a year or month is given; a zone offset in
// the string wins over loc. ok is false when the string has no date or time
// parts or its fields are out of range (the ArgumentError Active Record's
// datetime cast turns into nil).
func ZoneParse(s string, loc *time.Location, now time.Time) (time.Time, bool) {
	if len(s) > 128 {
		return time.Time{}, false
	}
	st := dateUnderscoreParseState(s, false)
	h := st.h
	if len(h) == 0 && st.frac == nil && st.zone == nil {
		return time.Time{}, false
	}
	nowLocal := now.In(loc)
	get := func(k string, def int64) int64 {
		if v, ok := h[k]; ok {
			return v
		}
		return def
	}
	_, hasYear := h["year"]
	_, hasMon := h["mon"]
	dayDefault := int64(nowLocal.Day())
	if hasYear || hasMon {
		dayDefault = 1
	}
	y, mo, d := get("year", int64(nowLocal.Year())), get("mon", int64(nowLocal.Month())), get("mday", dayDefault)
	hh, mi, ss := get("hour", 0), get("min", 0), get("sec", 0)
	nanos := new(big.Rat)
	if st.frac != nil {
		nanos = new(big.Rat).Mul(st.frac, big.NewRat(1_000_000_000, 1))
	}
	var off *int64
	if st.zone != nil {
		off = DateZoneToDiff(*st.zone)
	}
	if off != nil {
		t, err := timeArg(y, mo, d, hh, mi, ss, nanos, time.FixedZone("", int(*off)))
		if err != nil {
			return time.Time{}, false
		}
		return t.In(loc), true
	}
	t, err := timeArg(y, mo, d, hh, mi, ss, nanos, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

var (
	zoneNumeric = regexp.MustCompile(`\A(?:gmt|utc)?([-+])(\d+)(?:(:)(\d+)(?::(\d+))?|[,.](\d+))?\z`)
	// dateZoneAbbrevs are the zone names of the date gem's zonetab that map
	// to fixed offsets (hours); a daylight-saving name already includes its
	// hour.
	dateZoneAbbrevs = map[string]float64{
		"ut": 0, "gmt": 0, "est": -5, "edt": -4, "cst": -6, "cdt": -5, "mst": -7, "mdt": -6, "pst": -8, "pdt": -7,
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8, "i": 9, "k": 10, "l": 11, "m": 12,
		"n": -1, "o": -2, "p": -3, "q": -4, "r": -5, "s": -6, "t": -7, "u": -8, "v": -9, "w": -10, "x": -11, "y": -12,
		"z": 0, "utc": 0, "wet": 0, "at": -2, "brst": -2, "ndt": -2.5, "art": -3, "adt": -3, "brt": -3, "clst": -3,
		"nst": -3.5, "ast": -4, "clt": -4, "akdt": -8, "ydt": -8, "akst": -9, "hadt": -9, "hdt": -9, "yst": -9,
		"ahst": -10, "cat": -10, "hast": -10, "hst": -10, "nt": -11, "idlw": -12, "bst": 1, "cet": 1, "fwt": 1,
		"met": 1, "mewt": 1, "mez": 1, "swt": 1, "wat": 1, "west": 1, "cest": 2, "eet": 2, "fst": 2, "mest": 2,
		"mesz": 2, "sast": 2, "sst": 2, "bt": 3, "eat": 3, "eest": 3, "msk": 3, "msd": 4, "zp4": 4, "zp5": 5,
		"ist": 5.5, "zp6": 6, "wast": 7, "cct": 8, "sgt": 8, "wadt": 8, "jst": 9, "kst": 9, "east": 10, "gst": 10,
		"eadt": 11, "idle": 12, "nzst": 12, "nzt": 12, "nzdt": 13,
		"eastern": -5, "central": -6, "mountain": -7, "pacific": -8, "alaskan": -9, "hawaiian": -10, "atlantic": -4,
		"newfoundland": -3.5, "e. south america": -3, "sa eastern": -3, "argentina": -3, "greenland": -3,
		"sa western": -4, "sa pacific": -5, "central america": -6, "mexico": -6, "gmt standard": 0, "greenwich": 0,
		"w. europe": 1, "romance": 1, "central europe": 1, "central european": 1, "e. europe": 2, "gtb": 2,
		"fle": 2, "south africa": 2, "egypt": 2, "israel": 2, "russian": 3, "arab": 3, "arabian": 4, "india": 5.5,
		"china": 8, "singapore": 8, "taipei": 8, "w. australia": 8, "tokyo": 9, "korea": 9, "aus eastern": 10,
		"e. australia": 10, "new zealand": 12,
	}
)

// DateZoneToDiff ports date_zone_to_diff (date_parse.c): numeric offsets
// (optionally after GMT/UTC) and the zonetab abbreviations with fixed
// offsets, with " standard time" / " daylight time" / " dst" suffixes.
// Unknown names give nil, as in Ruby.
func DateZoneToDiff(zone string) *int64 {
	z := strings.ToLower(strings.Join(strings.Fields(zone), " "))
	dst := false
	for _, suffix := range []string{" standard time", " daylight time", " dst"} {
		if strings.HasSuffix(z, suffix) {
			dst = suffix != " standard time"
			z = strings.TrimSuffix(z, suffix)
			break
		}
	}
	if hours, ok := dateZoneAbbrevs[z]; ok {
		off := int64(hours * 3600)
		if dst {
			off += 3600
		}
		return &off
	}
	m := zoneNumeric.FindStringSubmatch(z)
	if m == nil {
		return nil
	}
	sign := int64(1)
	if m[1] == "-" {
		sign = -1
	}
	var hour, min, sec int64
	switch {
	case m[3] == ":":
		hour, _ = strconv.ParseInt(m[2], 10, 64)
		min, _ = strconv.ParseInt(m[4], 10, 64)
		if m[5] != "" {
			sec, _ = strconv.ParseInt(m[5], 10, 64)
		}
	case m[6] != "":
		hour, _ = strconv.ParseInt(m[2], 10, 64)
		frac, _ := new(big.Rat).SetString("0." + m[6])
		secs := new(big.Rat).Mul(frac, big.NewRat(3600, 1))
		n := new(big.Int).Quo(secs.Num(), secs.Denom())
		off := sign * (hour*3600 + n.Int64())
		return &off
	default:
		digits := m[2]
		switch n := len(digits); {
		case n <= 2:
			hour, _ = strconv.ParseInt(digits, 10, 64)
		case n <= 4:
			hour, _ = strconv.ParseInt(digits[:n-2], 10, 64)
			min, _ = strconv.ParseInt(digits[n-2:], 10, 64)
		default:
			hour, _ = strconv.ParseInt(digits[:n-4], 10, 64)
			min, _ = strconv.ParseInt(digits[n-4:n-2], 10, 64)
			sec, _ = strconv.ParseInt(digits[n-2:], 10, 64)
		}
	}
	off := sign * (hour*3600 + min*60 + sec)
	return &off
}
