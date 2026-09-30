// Package ar holds the small part of Active Record the ported models share:
// rows with cast attributes, dirty tracking against the persisted values,
// the values before type cast that validations read, inserts and updates of
// the changed columns, and ActiveModel::Errors.
package ar

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

type ColType int

const (
	String ColType = iota
	Integer
	Boolean
	Datetime
	JSON
	// Date columns are kept as their ISO string ("YYYY-MM-DD").
	Date
)

type Schema struct {
	Table   string
	Columns []string // in table order, id excluded
	Types   map[string]ColType
	// defaults are the column defaults a new record starts with.
	Defaults map[string]any
}

func (s *Schema) SelectList(alias string) string {
	cols := []string{alias + ".id"}
	for _, c := range s.Columns {
		if s.Types[c] == JSON {
			cols = append(cols, alias+"."+quote(c)+"::text")
		} else {
			cols = append(cols, alias+"."+quote(c))
		}
	}
	return strings.Join(cols, ", ")
}

// record is an Active Record-like row: cast values, the persisted values
// they are compared with (dirty tracking) and the values before type cast
// that numericality validations read.
type Record struct {
	S         *Schema
	ID        int64
	NewRecord bool
	Attrs     map[string]any
	Orig      map[string]any
	Raw       map[string]any
	Marked    bool // marked_for_destruction?
}

func NewRecord(s *Schema) *Record {
	r := &Record{S: s, NewRecord: true, Attrs: map[string]any{}, Orig: map[string]any{}, Raw: map[string]any{}}
	for k, v := range s.Defaults {
		r.Attrs[k] = v
	}
	return r
}

func ScanRecord(s *Schema, rows pgx.Rows) (*Record, error) {
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	r := &Record{S: s, Attrs: map[string]any{}, Orig: map[string]any{}, Raw: map[string]any{}}
	r.ID = vals[0].(int64)
	for i, c := range s.Columns {
		v := vals[i+1]
		switch x := v.(type) {
		case int32:
			v = int64(x)
		case string:
			if s.Types[c] == JSON {
				v, _ = rb.ParseJSON([]byte(x))
			}
		case time.Time:
			if s.Types[c] == Date {
				v = x.Format("2006-01-02")
			}
		}
		r.Attrs[c] = v
		r.Orig[c] = v
	}
	return r, nil
}

func (r *Record) Get(c string) any { return r.Attrs[c] }

func (r *Record) Str(c string) string {
	s, _ := r.Attrs[c].(string)
	return s
}

func (r *Record) StrPtr(c string) *string {
	if s, ok := r.Attrs[c].(string); ok {
		return &s
	}
	return nil
}

func (r *Record) Int(c string) int64 {
	n, _ := r.Attrs[c].(int64)
	return n
}

func (r *Record) Bool(c string) bool { return r.Attrs[c] == true }

// set writes an attribute from code (not user input).
func (r *Record) Set(c string, v any) {
	r.Attrs[c] = v
	delete(r.Raw, c)
}

// assign ports writing an attribute from user input: the value is cast by
// the column type and kept before type cast.
func (r *Record) Assign(c string, v any) {
	r.Raw[c] = v
	switch r.S.Types[c] {
	case String:
		r.Attrs[c] = CastString(v)
	case Integer:
		if n, ok := rb.CastInteger(v); ok {
			r.Attrs[c] = n
		} else {
			r.Attrs[c] = nil
		}
	case Boolean:
		r.Attrs[c] = CastBool(v)
	case Datetime:
		r.Attrs[c] = CastDatetime(v)
	default:
		r.Attrs[c] = v
	}
}

// CastDatetime ports the time-zone-aware datetime cast of user input: a
// String is parsed by Time.zone.parse (nil when it has no date parts or is
// out of range), a Time is kept at microsecond precision.
func CastDatetime(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x.Truncate(time.Microsecond)
	case string:
		t, ok := rb.ZoneParse(x, rb.AppZone, clock.Now())
		if !ok {
			return nil
		}
		return t.Truncate(time.Microsecond)
	}
	return nil
}

// rawValue is the value the numericality validator reads.
func (r *Record) RawValue(c string) any {
	if v, ok := r.Raw[c]; ok && v != nil {
		return v
	}
	return r.Attrs[c]
}

func (r *Record) Changed(c string) bool {
	if r.NewRecord {
		d, hasDefault := r.S.Defaults[c]
		if !hasDefault {
			return r.Attrs[c] != nil
		}
		return !SameValue(r.Attrs[c], d)
	}
	return !SameValue(r.Attrs[c], r.Orig[c])
}

func (r *Record) HasChanges() bool {
	for _, c := range r.S.Columns {
		if c == "created_at" || c == "updated_at" {
			continue
		}
		if r.Changed(c) {
			return true
		}
	}
	return false
}

func SameValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if ta, ok := a.(time.Time); ok {
		tb, ok := b.(time.Time)
		return ok && ta.Truncate(time.Microsecond).Equal(tb.Truncate(time.Microsecond))
	}
	return string(rb.JSON(a)) == string(rb.JSON(b))
}

func CastString(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case bool:
		if x {
			return "t"
		}
		return "f"
	}
	return rb.ToS(v)
}

// castBool ports ActiveModel::Type::Boolean#cast.
func CastBool(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case string:
		if x == "" {
			return nil
		}
		switch x {
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false
		}
		return true
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	}
	return true
}

func Truthy(v any) bool { return v != nil && v != false }

// dbValue converts an attribute for a query parameter.
func (r *Record) DbValue(c string) any {
	v := r.Attrs[c]
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Truncate(time.Microsecond)
	case nil:
		return nil
	}
	if r.S.Types[c] == JSON {
		return string(rb.JSON(v))
	}
	return v
}

// insert writes a new record (all columns, timestamps set now).
func (r *Record) Insert(ctx context.Context, q db.Querier, now time.Time) error {
	r.Attrs["created_at"], r.Attrs["updated_at"] = now, now
	var cols, marks []string
	var args []any
	for _, c := range r.S.Columns {
		cols = append(cols, quote(c))
		args = append(args, r.DbValue(c))
		marks = append(marks, "$"+Itoa(len(args)))
	}
	err := q.QueryRow(ctx, "INSERT INTO "+r.S.Table+" ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(marks, ", ")+") RETURNING id", args...).Scan(&r.ID)
	if err != nil {
		return err
	}
	r.Persisted()
	return nil
}

// update writes the changed columns and updated_at; no-op when unchanged.
func (r *Record) Update(ctx context.Context, q db.Querier, now time.Time) (bool, error) {
	var sets []string
	var args []any
	for _, c := range r.S.Columns {
		if c == "created_at" || c == "updated_at" || !r.Changed(c) {
			continue
		}
		args = append(args, r.DbValue(c))
		sets = append(sets, quote(c)+" = $"+Itoa(len(args)))
	}
	if len(sets) == 0 {
		return false, nil
	}
	r.Attrs["updated_at"] = now
	args = append(args, r.DbValue("updated_at"), r.ID)
	sets = append(sets, "updated_at = $"+Itoa(len(args)-1))
	_, err := q.Exec(ctx, "UPDATE "+r.S.Table+" SET "+strings.Join(sets, ", ")+" WHERE id = $"+Itoa(len(args)), args...)
	if err != nil {
		return false, err
	}
	r.Persisted()
	return true, nil
}

// updateColumns ports update_columns: no callbacks, no updated_at.
func (r *Record) UpdateColumns(ctx context.Context, q db.Querier, values map[string]any) error {
	var sets []string
	var args []any
	for _, c := range r.S.Columns {
		v, ok := values[c]
		if !ok {
			continue
		}
		r.Attrs[c] = v
		r.Orig[c] = v
		args = append(args, r.DbValue(c))
		sets = append(sets, quote(c)+" = $"+Itoa(len(args)))
	}
	args = append(args, r.ID)
	_, err := q.Exec(ctx, "UPDATE "+r.S.Table+" SET "+strings.Join(sets, ", ")+" WHERE id = $"+Itoa(len(args)), args...)
	return err
}

func (r *Record) Persisted() {
	r.NewRecord = false
	for k, v := range r.Attrs {
		r.Orig[k] = v
	}
	r.Raw = map[string]any{}
}

func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// quote quotes a column name (some, like "order", are reserved words).
func quote(c string) string { return pgx.Identifier{c}.Sanitize() }
