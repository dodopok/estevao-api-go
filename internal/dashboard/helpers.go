// Package dashboard ports DashboardDataService and its Dashboard::*Metrics
// sections. Each query mirrors the SQL Active Record sends (including the
// GROUP BY expressions and the absence of an ORDER BY), because the key
// order of the grouped counts in the payload is PostgreSQL's row order.
package dashboard

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Period is a BaseMetrics: the requested dates and the instant the metrics
// are computed at.
type Period struct {
	Ctx        context.Context
	Start, End civil.Date
	Now        time.Time
}

// TSStart and TSEnd port period_timestamps (beginning_of_day..end_of_day in
// Time.zone), as the microsecond bounds Active Record binds.
func (p *Period) TSStart() time.Time {
	return time.Date(p.Start.Year(), time.Month(p.Start.Month()), p.Start.Day(), 0, 0, 0, 0, rb.AppZone).UTC()
}

func (p *Period) TSEnd() time.Time {
	return time.Date(p.End.Year(), time.Month(p.End.Month()), p.End.Day(), 23, 59, 59, 999_999_000, rb.AppZone).UTC()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func (p *Period) rows(sql string, args ...any) pgx.Rows {
	rows, err := db.Q().Query(p.Ctx, sql, args...)
	must(err)
	return rows
}

// count runs a single-value integer query (COUNT, or a SUM Active Record
// turns into 0 when NULL).
func (p *Period) count(sql string, args ...any) int64 {
	var n *int64
	must(db.Q().QueryRow(p.Ctx, sql, args...).Scan(&n))
	if n == nil {
		return 0
	}
	return *n
}

// average runs an AVG query cast to text and ports rounded_average
// (BigDecimal#to_f then round(2); nil is 0.0).
func (p *Period) average(sql string, args ...any) float64 {
	var s *string
	must(db.Q().QueryRow(p.Ctx, sql, args...).Scan(&s))
	return roundedAverage(s)
}

func roundedAverage(s *string) float64 {
	if s == nil {
		return 0
	}
	f, err := strconv.ParseFloat(*s, 64)
	must(err)
	return rb.RoundFloat(f, 2)
}

// minTime runs a MIN(timestamp) query.
func (p *Period) minTime(sql string, args ...any) *time.Time {
	var t *time.Time
	must(db.Q().QueryRow(p.Ctx, sql, args...).Scan(&t))
	return t
}

// percentage ports BaseMetrics#percentage (Integer 0 for a zero
// denominator, else a Float rounded to 2 places).
func percentage(num, den float64) any {
	if den == 0 {
		return 0
	}
	return rb.RoundFloat(num/den*100, 2)
}

// scopePayload ports scope_payload (metrics.merge(scope:)).
func scopePayload(scope string, m *rb.Map) *rb.Map {
	m.Set("scope", scope)
	return m
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}

// ageSeconds ports (Time.current - t).to_i.
func (p *Period) ageSeconds(t *time.Time) any {
	if t == nil {
		return nil
	}
	return int64(p.Now.Sub(*t) / time.Second)
}

// hashKey is the String a grouped key becomes in JSON (nil is "").
func hashKey(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case time.Time:
		return x.Format("2006-01-02")
	}
	return rb.ToS(v)
}

// groupCount runs `SELECT COUNT(*) AS count_all, <key> ... GROUP BY <key>`
// and returns the Hash Active Record builds from it, in row order.
func (p *Period) groupCount(sql string, args ...any) *rb.Map {
	rows := p.rows(sql, args...)
	defer rows.Close()
	out := rb.NewMap()
	for rows.Next() {
		var n int64
		var key any
		must(rows.Scan(&n, &key))
		out.Set(hashKey(key), n)
	}
	must(rows.Err())
	return out
}

// pair is one grouped row whose key has more than one column.
type pair struct {
	Keys  []any
	Count int64
}

func (p *Period) groupRows(sql string, nkeys int, args ...any) []pair {
	rows := p.rows(sql, args...)
	defer rows.Close()
	var out []pair
	for rows.Next() {
		var n int64
		keys := make([]any, nkeys)
		dest := []any{&n}
		for i := range keys {
			dest = append(dest, &keys[i])
		}
		must(rows.Scan(dest...))
		out = append(out, pair{keys, n})
	}
	must(rows.Err())
	return out
}

// sortStable ports sort_by on a stable qsort (the oracle's and
// production's): an insertion sort by less.
func sortStable[T any](list []T, less func(a, b T) bool) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// dateRange lists start..end.
func dateRange(start, end civil.Date) []civil.Date {
	var out []civil.Date
	for d := start; d <= end; d = d.Add(1) {
		out = append(out, d)
	}
	return out
}

// localDateSQL ports local_date_sql.
func localDateSQL(column string) string {
	return "DATE(" + column + " AT TIME ZONE 'UTC' AT TIME ZONE " + quote(rb.AppZone.String()) + ")"
}

func localDatetimeSQL(column string) string {
	return column + " AT TIME ZONE 'UTC' AT TIME ZONE " + quote(rb.AppZone.String())
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
