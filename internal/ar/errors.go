package ar

import (
	"slices"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Errors is ActiveModel::Errors reduced to what to_hash renders: messages
// grouped by attribute, attributes in the order they first failed.
type Errors struct {
	list []errEntry
}

// errEntry is one ActiveModel::Error; key stands for what makes two errors
// equal (type and options, which carry the invalid value for numericality
// and inclusion).
type errEntry struct{ attr, msg, key string }

func (e *Errors) Add(attr, msg string) { e.list = append(e.list, errEntry{attr, msg, msg}) }

// addValue adds an error whose options include the offending value.
func (e *Errors) AddValue(attr, msg string, value any) {
	e.list = append(e.list, errEntry{attr, msg, msg + "\x00" + rb.ClassName(value) + "\x00" + string(rb.JSON(value))})
}

func (e *Errors) ImportFrom(prefix string, inner *Errors) {
	for _, en := range inner.list {
		e.list = append(e.list, errEntry{prefix + "." + en.attr, en.msg, en.key})
	}
}

// uniq ports errors.uniq! (_ensure_no_duplicate_errors, run after the
// validation of a record with autosaved associations).
func (e *Errors) Uniq() {
	seen := map[string]bool{}
	var out []errEntry
	for _, en := range e.list {
		k := en.attr + "\x00" + en.key
		if !seen[k] {
			seen[k] = true
			out = append(out, en)
		}
	}
	e.list = out
}

// Any reports whether there are errors.
func (e *Errors) Any() bool { return len(e.list) > 0 }

// Has reports whether attr failed with msg.
func (e *Errors) Has(attr, msg string) bool {
	return slices.ContainsFunc(e.list, func(en errEntry) bool { return en.attr == attr && en.msg == msg })
}

// Hash ports errors.to_hash.
func (e *Errors) Hash() *rb.Map {
	out := rb.NewMap()
	for _, en := range e.list {
		msgs, _ := out.Get(en.attr).([]string)
		out.Set(en.attr, append(msgs, en.msg))
	}
	return out
}

// RecordInvalid is ActiveRecord::RecordInvalid.
type RecordInvalid struct{ Errors *Errors }

func (e *RecordInvalid) Error() string { return "Validation failed" }

func Runes(v any) int {
	s, _ := v.(string)
	return len([]rune(s))
}

func Blank(v any) bool { return rb.Blank(v) }

func TooLong(e *Errors, attr string, v any, max int) {
	if Runes(v) > max {
		e.Add(attr, "is too long (maximum is "+Itoa(max)+" characters)")
	}
}

func Included(v any, list []string) bool {
	s, ok := v.(string)
	return ok && slices.Contains(list, s)
}

func Numeric(e *Errors, r *Record, attr string, checks ...rb.NumCheck) {
	raw := r.RawValue(attr)
	if m := rb.Numericality(raw, true, checks...); m != "" {
		e.AddValue(attr, m, raw)
	}
}

func NotIncluded(e *Errors, attr string, v any) { e.AddValue(attr, "is not included in the list", v) }

// Entry is one error (attribute and message).
type Entry struct{ Attr, Msg string }

// Entries lists the errors in order.
func (e *Errors) Entries() []Entry {
	out := make([]Entry, len(e.list))
	for i, en := range e.list {
		out[i] = Entry{en.attr, en.msg}
	}
	return out
}
