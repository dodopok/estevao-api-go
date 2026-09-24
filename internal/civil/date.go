// Package civil provides a calendar date type without time or zone, the unit
// every liturgical calculation works in.
package civil

import (
	"fmt"
	"strconv"
	"time"
)

// Date is a proleptic Gregorian calendar date, stored as days since
// 1970-01-01. The zero value is 1970-01-01; comparisons and arithmetic are
// plain integer operations.
type Date int32

// New returns the date for year, month and day, and false when the
// combination does not exist (Ruby's Date.new raises in that case).
func New(year, month, day int) (Date, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return 0, false
	}
	return fromTime(t), true
}

// MustNew is New for dates known to be valid; it panics otherwise.
func MustNew(year, month, day int) Date {
	d, ok := New(year, month, day)
	if !ok {
		panic(fmt.Sprintf("civil: invalid date %d-%d-%d", year, month, day))
	}
	return d
}

// FromTime returns the calendar date of t in t's own location.
func FromTime(t time.Time) Date {
	y, m, d := t.Date()
	return MustNew(y, int(m), d)
}

func fromTime(t time.Time) Date {
	return Date(t.Unix() / 86400)
}

// Time returns midnight UTC of the date.
func (d Date) Time() time.Time { return time.Unix(int64(d)*86400, 0).UTC() }

func (d Date) Year() int  { return d.Time().Year() }
func (d Date) Month() int { return int(d.Time().Month()) }
func (d Date) Day() int   { return d.Time().Day() }

// Weekday returns 0 for Sunday through 6 for Saturday, like Ruby's wday.
func (d Date) Weekday() int {
	// 1970-01-01 was a Thursday (4).
	w := (int(d) + 4) % 7
	if w < 0 {
		w += 7
	}
	return w
}

func (d Date) IsSunday() bool    { return d.Weekday() == 0 }
func (d Date) IsMonday() bool    { return d.Weekday() == 1 }
func (d Date) IsWednesday() bool { return d.Weekday() == 3 }
func (d Date) IsFriday() bool    { return d.Weekday() == 5 }
func (d Date) IsSaturday() bool  { return d.Weekday() == 6 }

// Add returns the date n days later (earlier for negative n).
func (d Date) Add(n int) Date { return d + Date(n) }

// Sub returns the number of days from o to d.
func (d Date) Sub(o Date) int { return int(d - o) }

// Between reports whether lo <= d <= hi.
func (d Date) Between(lo, hi Date) bool { return d >= lo && d <= hi }

// YearDay is the 1-based ordinal day of the year.
func (d Date) YearDay() int { return d.Time().YearDay() }

// ISO formats as YYYY-MM-DD.
func (d Date) ISO() string {
	t := d.Time()
	return fmt.Sprintf("%04d-%02d-%02d", t.Year(), int(t.Month()), t.Day())
}

// DMY formats as DD/MM/YYYY.
func (d Date) DMY() string {
	t := d.Time()
	return fmt.Sprintf("%02d/%02d/%04d", t.Day(), int(t.Month()), t.Year())
}

func (d Date) String() string { return d.ISO() }

// MarshalJSON renders the ISO date, as Rails does for a Date.
func (d Date) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(d.ISO())), nil }

// IsLeap reports whether year is a Gregorian leap year.
func IsLeap(year int) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

// DaysInMonth returns the number of days in month of year.
func DaysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ParseISO parses YYYY-MM-DD strictly.
func ParseISO(s string) (Date, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, false
	}
	return fromTime(t), true
}

// MonthNamesEN are Ruby's Date::MONTHNAMES (index 0 is empty).
var MonthNamesEN = [...]string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
