package reading

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Record is a lectionary_readings row. Key columns map NULL to "" (none of
// them is ever compared with an empty string); published columns keep nil.
type Record struct {
	ID                       int64
	CelebrationID            *int64
	Cycle                    string
	DateReference            string
	ServiceType              string
	ServiceVariant           string
	ReadingType              string
	FirstReading             *string
	FirstReadingAlternative  *string
	FirstReadingTitle        *string
	Psalm                    *string
	PsalmAlternative         *string
	PsalmTitle               *string
	SecondReading            *string
	SecondReadingAlternative *string
	SecondReadingTitle       *string
	Gospel                   *string
	GospelTitle              *string
	Notes                    *string
}

const recordColumns = `id, celebration_id, COALESCE(cycle, ''), COALESCE(date_reference, ''),
COALESCE(service_type, ''), COALESCE(service_variant, ''), COALESCE(reading_type, ''),
first_reading, first_reading_alternative, first_reading_title, psalm, psalm_alternative, psalm_title,
second_reading, second_reading_alternative, second_reading_title, gospel, gospel_title, notes`

func scanRecord(row interface{ Scan(...any) error }) (*Record, error) {
	var r Record
	err := row.Scan(&r.ID, &r.CelebrationID, &r.Cycle, &r.DateReference, &r.ServiceType, &r.ServiceVariant,
		&r.ReadingType, &r.FirstReading, &r.FirstReadingAlternative, &r.FirstReadingTitle, &r.Psalm,
		&r.PsalmAlternative, &r.PsalmTitle, &r.SecondReading, &r.SecondReadingAlternative, &r.SecondReadingTitle,
		&r.Gospel, &r.GospelTitle, &r.Notes)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func present(p *string) bool { return p != nil && strings.TrimSpace(*p) != "" && !blankRuby(*p) }

func blankRuby(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r', ' ', ' ', ' ', ' ', ' ', ' ',
			' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ',
			' ', ' ', '　', '\u0085':
			continue
		}
		return false
	}
	return true
}

func (r *Record) dup() *Record {
	c := *r
	return &c
}

// sqlBuilder accumulates a WHERE clause and its arguments.
type sqlBuilder struct {
	where []string
	order []string
	args  []any
}

func (b *sqlBuilder) arg(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

func (b *sqlBuilder) eq(col string, v any) {
	if v == nil {
		b.where = append(b.where, col+" IS NULL")
		return
	}
	b.where = append(b.where, col+" = "+b.arg(v))
}

// in mirrors where(col: [values]) where a nil member adds OR IS NULL.
func (b *sqlBuilder) in(col string, values []*string) {
	var list []string
	hasNil := false
	for _, v := range values {
		if v == nil {
			hasNil = true
			continue
		}
		list = append(list, b.arg(*v))
	}
	var clause string
	switch {
	case len(list) == 0 && hasNil:
		clause = col + " IS NULL"
	case len(list) == 1:
		clause = col + " = " + list[0]
	default:
		clause = col + " IN (" + strings.Join(list, ", ") + ")"
	}
	if hasNil && len(list) > 0 {
		clause = "(" + clause + " OR " + col + " IS NULL)"
	}
	b.where = append(b.where, clause)
}

func (b *sqlBuilder) clone() *sqlBuilder {
	return &sqlBuilder{
		where: append([]string(nil), b.where...),
		order: append([]string(nil), b.order...),
		args:  append([]any(nil), b.args...),
	}
}

func (b *sqlBuilder) query(ctx context.Context, limit int) []*Record {
	sql := "SELECT " + recordColumns + " FROM lectionary_readings"
	if len(b.where) > 0 {
		sql += " WHERE " + strings.Join(b.where, " AND ")
	}
	if len(b.order) > 0 {
		sql += " ORDER BY " + strings.Join(b.order, ", ")
	}
	if limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Q().Query(ctx, sql, b.args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var out []*Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			panic(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

func (b *sqlBuilder) take(ctx context.Context) *Record {
	rows := b.query(ctx, 1)
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func sp(s string) *string { return &s }

// quote mirrors connection.quote for a string or nil.
func quote(v any) string {
	if v == nil {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(v.(string), "'", "''") + "'"
}

// nilIfEmpty maps the "" convention to a SQL NULL.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Query ports Reading::Query.
type Query struct {
	PrayerBookID         *int64
	Cycle                string
	Date                 civil.Date
	ReadingType          string
	ReadingTypeRaw       any
	ServiceType          string
	ServiceVariant       string
	StrictServiceVariant bool
}

func (q *Query) officeReadingRequested() bool {
	return q.ServiceType == "morning_prayer" || q.ServiceType == "evening_prayer"
}

func (q *Query) base(col string, value any, cycles []*string) *sqlBuilder {
	b := &sqlBuilder{}
	b.eq(col, value)
	b.in("cycle", cycles)
	if q.PrayerBookID == nil {
		b.eq("prayer_book_id", nil)
	} else {
		b.eq("prayer_book_id", *q.PrayerBookID)
	}
	// apply_cycle_priority
	b.order = append(b.order, "CASE WHEN lectionary_readings.cycle = "+quote(nilIfEmpty(q.Cycle))+" THEN 0 WHEN lectionary_readings.cycle = 'all' THEN 1 ELSE 2 END")
	if q.ReadingTypeRaw != nil && !rb.Blank(q.ReadingTypeRaw) {
		// apply_reading_type_filter quotes the value; Rails cannot quote a
		// list or a hash.
		panic(&rb.RubyError{Class: "TypeError", Message: "can't quote " + rb.ClassName(q.ReadingTypeRaw)})
	}
	if q.ReadingType != "" {
		// Rails interpolates these (where with "?" and Arel.sql), so they are
		// literals rather than binds here too.
		rt := quote(q.ReadingType)
		b.where = append(b.where, "(reading_type = "+rt+" OR reading_type IS NULL)")
		b.order = append(b.order, "CASE WHEN lectionary_readings.reading_type = "+rt+" THEN 0 ELSE 1 END")
	}
	if q.ServiceVariant != "" {
		q.applyServiceVariant(b)
	}
	return b
}

func (q *Query) applyServiceVariant(b *sqlBuilder) {
	if q.StrictServiceVariant {
		b.eq("service_variant", q.ServiceVariant)
		return
	}
	variants := []*string{sp(q.ServiceVariant)}
	weekday := q.officeReadingRequested()
	if weekday {
		variants = append(variants, sp("weekday_course"))
	}
	variants = append(variants, nil)
	b.in("service_variant", variants)
	weekdayPriority := ""
	if weekday || q.ServiceVariant == "weekday_course" {
		weekdayPriority = "WHEN lectionary_readings.service_variant = 'weekday_course' THEN 1"
	}
	b.order = append(b.order, "CASE WHEN lectionary_readings.service_variant = "+quote(q.ServiceVariant)+" THEN 0 "+weekdayPriority+" ELSE 2 END")
}

func (q *Query) cycles() []*string {
	return []*string{cyclePtr(q.Cycle), sp("all")}
}

func cyclePtr(c string) *string {
	if c == "" {
		return nil
	}
	return &c
}

func (q *Query) weeklyCycles() []*string {
	oddEven := "even"
	if q.Date.Year()%2 != 0 {
		oddEven = "odd"
	}
	return []*string{cyclePtr(q.Cycle), sp("A"), sp("B"), sp("C"), sp(oddEven), sp("all")}
}

func withServiceType(ctx context.Context, b *sqlBuilder, types ...string) *Record {
	c := b.clone()
	list := make([]*string, len(types))
	for i, t := range types {
		list[i] = sp(t)
	}
	c.in("service_type", list)
	return c.take(ctx)
}

func (q *Query) serviceTypeOrEucharist() string {
	if q.ServiceType == "" {
		return "eucharist"
	}
	return q.ServiceType
}

// FindByReference ports find_by_reference ("" reference == nil).
func (q *Query) FindByReference(ctx context.Context, reference string, isNil bool) *Record {
	if isNil {
		return nil
	}
	b := q.base("date_reference", reference, q.cycles())
	if q.officeReadingRequested() {
		if r := withServiceType(ctx, b, q.ServiceType); r != nil {
			return r
		}
		if r := withServiceType(ctx, b, "daily_office", "weekly"); r != nil {
			return r
		}
		return withServiceType(ctx, b, "eucharist")
	}
	return withServiceType(ctx, b, q.serviceTypeOrEucharist())
}

// FindByCelebrationID ports find_by_celebration_id.
func (q *Query) FindByCelebrationID(ctx context.Context, id int64) *Record {
	b := q.base("celebration_id", id, q.cycles())
	if q.officeReadingRequested() {
		if r := withServiceType(ctx, b, q.ServiceType); r != nil {
			return r
		}
		return withServiceType(ctx, b, "daily_office", "weekly")
	}
	return withServiceType(ctx, b, q.serviceTypeOrEucharist())
}

// FindByReferences ports find_by_references.
func (q *Query) FindByReferences(ctx context.Context, refs []string) *Record {
	for _, ref := range refs {
		if r := q.FindByReference(ctx, ref, false); r != nil {
			return r
		}
	}
	return nil
}

// FindWeekly ports find_weekly.
func (q *Query) FindWeekly(ctx context.Context, reference string) *Record {
	if reference == "" {
		return nil
	}
	b := q.base("date_reference", reference, q.weeklyCycles())
	if q.ServiceType != "" {
		if r := withServiceType(ctx, b, q.ServiceType); r != nil {
			return r
		}
	}
	if r := withServiceType(ctx, b, "weekly", "daily_office"); r != nil {
		return r
	}
	if q.ServiceType == "" {
		return withServiceType(ctx, b, "morning_prayer", "evening_prayer")
	}
	return nil
}

// FindWeeklyByReferences ports find_weekly_by_references.
func (q *Query) FindWeeklyByReferences(ctx context.Context, refs []string) *Record {
	for _, ref := range refs {
		if r := q.FindWeekly(ctx, ref); r != nil {
			return r
		}
	}
	return nil
}
