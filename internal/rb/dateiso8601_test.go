package rb

import (
	"testing"
	"time"
)

// Expectations from Ruby's Date.iso8601 (inputs that do not depend on today).
func TestDateISO8601(t *testing.T) {
	cases := map[string]string{
		"2026-09-28": "2026-09-28", "20260928": "2026-09-28", "2026-W39-1": "2026-09-21", "2026W391": "2026-09-21",
		"2026-271": "2026-09-28", "2026271": "2026-09-28", "2026-09": "2026-09-01", "26-09-28": "2026-09-28",
		"69-01-01": "1969-01-01", "+2026-09-28": "2026-09-28", "-0001-01-01": "-0001-01-01",
		"2026-09-28T10:00:00Z": "2026-09-28", "2026-09-28t25:61": "2026-09-28", "20260928T1000": "2026-09-28",
		" 2026-09-28 ": "2026-09-28", "10000-01-01": "10000-01-01", "2026-W53-1": "2026-12-28",
		"2026-09-28T10:00:00+03:00": "2026-09-28",
		"2026-02-30":                "", "2026-9-28": "", "abc": "", "": "", "2026-13-01": "", "2026/09/28": "", "2026-00-10": "",
		"2026-09-28x": "",
	}
	for in, want := range cases {
		d, err := DateISO8601(in)
		got := ""
		if err == nil {
			got = d.ISO()
		}
		if got != want {
			t.Errorf("DateISO8601(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// Expectations from ActiveSupport's String#in_time_zone (Time.zone
// America/Sao_Paulo), as UTC.
func TestZoneParse(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"2026-12-31":                             "2026-12-31T03:00:00Z",
		"2026-12-31T10:00:00Z":                   "2026-12-31T10:00:00Z",
		"2026-12-31 10:00 EST":                   "2026-12-31T15:00:00Z",
		"2026-12-31 10:00 +0530":                 "2026-12-31T04:30:00Z",
		"2026-12-31 10:00 Eastern Standard Time": "2026-12-31T15:00:00Z",
		"2026-02-30":                             "2026-03-02T03:00:00Z",
		"2026":                                   "",
		"2026-13-01":                             "", "abc": "", "": "", "2026-12-31 25:00": "",
	}
	for in, want := range cases {
		got := ""
		if v, ok := ZoneParse(in, loc, now); ok {
			got = v.UTC().Format(time.RFC3339)
		}
		if got != want {
			t.Errorf("ZoneParse(%q) = %q, want %q", in, got, want)
		}
	}
}
