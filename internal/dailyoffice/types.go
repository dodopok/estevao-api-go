// Package dailyoffice ports DailyOfficeService and the per-book Daily Office
// builders. The typed Line/Section/Office pipeline of the Rails code is kept
// (lines and sections are built, then serialized once at the boundary), so
// book code reads like the Ruby it ports.
package dailyoffice

import (
	"hash/crc32"
	"strconv"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Line ports DailyOffice::Line. Text is a string or nil.
type Line struct {
	Text any
	Type string
	Meta *rb.Map
}

// ToH ports Line#to_h: {text:, type:, **metadata}.
func (l *Line) ToH() *rb.Map {
	m := rb.M("text", l.Text, "type", l.Type)
	if l.Meta != nil {
		l.Meta.Each(func(k string, v any) { m.Set(k, v) })
	}
	return m
}

// Section ports DailyOffice::Section.
type Section struct {
	Name  string
	Slug  string
	Lines []*Line
	Meta  *rb.Map
}

// ToH ports Section#to_h: {name:, slug:, lines:, **metadata}.
func (s *Section) ToH() *rb.Map {
	lines := make([]any, len(s.Lines))
	for i, l := range s.Lines {
		lines[i] = l.ToH()
	}
	m := rb.M("name", s.Name, "slug", s.Slug, "lines", lines)
	if s.Meta != nil {
		s.Meta.Each(func(k string, v any) { m.Set(k, serialize(v)) })
	}
	return m
}

func serialize(v any) any {
	switch x := v.(type) {
	case *Line:
		return x.ToH()
	case *Section:
		return x.ToH()
	case []*Line:
		out := make([]any, len(x))
		for i, l := range x {
			out[i] = l.ToH()
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = serialize(e)
		}
		return out
	case *rb.Map:
		out := rb.NewMap()
		x.Each(func(k string, e any) { out.Set(k, serialize(e)) })
		return out
	}
	return v
}

// NewLine ports DailyOffice::Line.new (blank type raises ArgumentError).
func NewLine(text any, typ string, meta *rb.Map) *Line {
	if rb.BlankString(typ) {
		panic(&rb.RubyError{Class: "ArgumentError", Message: "line type cannot be blank"})
	}
	return &Line{Text: text, Type: typ, Meta: meta}
}

// NewSection ports DailyOffice::Section.new: nil lines are dropped.
func NewSection(name any, slug string, lines []*Line, meta *rb.Map) *Section {
	if rb.BlankString(slug) {
		panic(&rb.RubyError{Class: "ArgumentError", Message: "section slug cannot be blank"})
	}
	kept := make([]*Line, 0, len(lines))
	for _, l := range lines {
		if l != nil {
			kept = append(kept, l)
		}
	}
	return &Section{Name: rb.ToS(name), Slug: slug, Lines: kept, Meta: meta}
}

// Randomizer ports DailyOffice::Randomizer.
type Randomizer struct{ Seed int64 }

func crc(s string) int64 { return int64(crc32.ChecksumIEEE([]byte(s))) }

// Number ports #number(lo..hi, key:). Ruby seeds Random with the absolute
// value of a negative integer.
func (r Randomizer) Number(lo, hi int, key string) int {
	s := r.Seed + crc(key)
	if s < 0 {
		s = -s
	}
	return rb.NewRandom(uint64(s)).RandRange(lo, hi)
}

// PickIndex ports #pick: the index chosen among n values (-1 when empty).
func (r Randomizer) PickIndex(n int, key string) int {
	if n == 0 {
		return -1
	}
	return r.Number(0, n-1, key)
}

// DeterministicSeed ports Zlib.crc32("#{date}_#{office_type}").
func DeterministicSeed(dateISO, officeType string) int64 { return crc(dateISO + "_" + officeType) }

func itoa(n int) string { return strconv.Itoa(n) }
