// Package prayerrequests ports PrayerRequest, the PrayerRequests services,
// WeeklyPrayer and WeeklyPrayerGeneratorService.
package prayerrequests

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// MaxPerWeek is PrayerRequest::MAX_PER_WEEK.
const MaxPerWeek = 20

// Request is a prayer_requests row.
type Request struct {
	ID        int64
	UserID    int64
	WeekStart rb.YMD
	Title     *string
	Content   *string
	Position  *int64
	CreatedAt time.Time
	UpdatedAt time.Time

	// positionRaw is position_before_type_cast when the position came from
	// the user (the numericality validation reads it).
	positionRaw    any
	positionFromUs bool
}

// WeekStartFor ports PrayerRequest.week_start_for.
func WeekStartFor(d rb.YMD) rb.YMD { return d.AddDays(-d.Wday()) }

func ymdOf(t time.Time) rb.YMD {
	return rb.YMD{Y: int64(t.Year()), M: int64(t.Month()), D: int64(t.Day())}
}

const columns = `id, user_id, week_start, title, content, position, created_at, updated_at`

func scan(row interface{ Scan(...any) error }) (*Request, error) {
	var r Request
	var week time.Time
	var pos int64
	if err := row.Scan(&r.ID, &r.UserID, &week, &r.Title, &r.Content, &pos, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.WeekStart = ymdOf(week)
	r.Position = &pos
	return &r, nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ForWeek ports user.prayer_requests.for_week(week).ordered.
func ForWeek(ctx context.Context, userID int64, week rb.YMD) []*Request {
	rows, err := db.Q().Query(ctx, `SELECT `+columns+` FROM prayer_requests WHERE user_id = $1 AND week_start = $2
		ORDER BY position ASC, created_at ASC`, userID, week.ISO())
	must(err)
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		r, err := scan(rows)
		must(err)
		out = append(out, r)
	}
	must(rows.Err())
	return out
}

// Find ports PrayerRequestPolicy.scope(user).find(id) (nil when missing).
func Find(ctx context.Context, userID, id int64) *Request {
	r, err := scan(db.Q().QueryRow(ctx, `SELECT `+columns+` FROM prayer_requests WHERE user_id = $1 AND id = $2 LIMIT 1`, userID, id))
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return r
}

func countForWeek(ctx context.Context, userID int64, week rb.YMD) int {
	var n int
	must(db.Q().QueryRow(ctx, `SELECT COUNT(*) FROM prayer_requests WHERE user_id = $1 AND week_start = $2`, userID, week.ISO()).Scan(&n))
	return n
}

// castString ports ActiveModel::Type::String#cast.
func castString(v any) *string {
	var s string
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		s = x
	case bool:
		s = "f"
		if x {
			s = "t"
		}
	default:
		s = rb.ToS(x)
	}
	return &s
}

// assign ports assigning permitted attributes (title, content, position).
func (r *Request) assign(attrs *rb.Map) {
	attrs.Each(func(k string, v any) {
		switch k {
		case "title":
			r.Title = castString(v)
		case "content":
			r.Content = castString(v)
		case "position":
			r.positionRaw, r.positionFromUs = v, true
			if n, ok := rb.CastInteger(v); ok {
				r.Position = &n
			} else {
				r.Position = nil
			}
		}
	})
}

func (r *Request) setPosition(n int64) {
	r.Position = &n
	r.positionRaw, r.positionFromUs = n, true
}

var zero = int64(0)

// errors ports the model validations (full messages, in declaration order).
func (r *Request) errors(ctx context.Context, create bool) []string {
	var msgs []string
	if r.Title == nil || rb.BlankString(*r.Title) {
		msgs = append(msgs, "Title can't be blank")
	}
	if r.Title != nil && len([]rune(*r.Title)) > 255 {
		msgs = append(msgs, "Title is too long (maximum is 255 characters)")
	}
	if r.Content != nil && !rb.BlankString(*r.Content) && len([]rune(*r.Content)) > 1000 {
		msgs = append(msgs, "Content is too long (maximum is 1000 characters)")
	}
	var raw any
	if r.positionFromUs {
		raw = r.positionRaw
	}
	if raw == nil && r.Position != nil {
		raw = *r.Position
	}
	if m := rb.Numericality(raw, true, &zero); m != "" {
		msgs = append(msgs, "Position "+m)
	}
	if r.WeekStart.Wday() != 0 {
		msgs = append(msgs, "Week start must be a Sunday")
	}
	if create && countForWeek(ctx, r.UserID, r.WeekStart) >= MaxPerWeek {
		msgs = append(msgs, "Maximum of "+strconv.Itoa(MaxPerWeek)+" prayer requests per week")
	}
	return msgs
}

// checkIntegerRange ports ActiveModel::Type::Integer#serialize's range
// check for the 4-byte position column.
func (r *Request) checkIntegerRange() {
	if r.Position != nil && (*r.Position > 2147483647 || *r.Position < -2147483648) {
		panic(&web.StandardError{Class: "ActiveModel::RangeError",
			Message: strconv.FormatInt(*r.Position, 10) + " is out of range for ActiveModel::Type::Integer with limit 4 bytes"})
	}
}

// Create ports PrayerRequests::Create (returns the validation messages of
// a RecordInvalid).
func Create(ctx context.Context, u *users.User, attrs *rb.Map, week rb.YMD) (*Request, []string) {
	r := &Request{UserID: u.ID, WeekStart: week, Position: &zero}
	r.assign(attrs)
	if r.Position == nil {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("zero?", nil)})
	}
	if *r.Position == 0 {
		var max *int64
		must(db.Q().QueryRow(ctx, `SELECT MAX(position) FROM prayer_requests WHERE user_id = $1 AND week_start = $2`, u.ID, week.ISO()).Scan(&max))
		next := int64(0)
		if max != nil {
			next = *max + 1
		}
		r.setPosition(next)
	}
	if msgs := r.errors(ctx, true); len(msgs) > 0 {
		return nil, msgs
	}
	r.insert(ctx, db.Q())
	return r, nil
}

func (r *Request) insert(ctx context.Context, q db.Querier) {
	r.checkIntegerRange()
	now := users.Now()
	must(q.QueryRow(ctx, `INSERT INTO prayer_requests (user_id, week_start, title, content, position, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id`, r.UserID, r.WeekStart.ISO(), r.Title, r.Content, *r.Position, now).Scan(&r.ID))
	r.CreatedAt, r.UpdatedAt = now, now
}

func strEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Update ports PrayerRequests::Update (update!).
func Update(ctx context.Context, r *Request, attrs *rb.Map) []string {
	before := *r
	r.assign(attrs)
	if msgs := r.errors(ctx, false); len(msgs) > 0 {
		return msgs
	}
	cols := map[string]any{}
	if !strEq(before.Title, r.Title) {
		cols["title"] = r.Title
	}
	if !strEq(before.Content, r.Content) {
		cols["content"] = r.Content
	}
	if *before.Position != *r.Position {
		r.checkIntegerRange()
		cols["position"] = *r.Position
	}
	if len(cols) == 0 {
		return nil
	}
	now := users.Now()
	sql := "UPDATE prayer_requests SET "
	args := []any{}
	for _, k := range []string{"title", "content", "position"} {
		if v, ok := cols[k]; ok {
			args = append(args, v)
			sql += k + " = $" + strconv.Itoa(len(args)) + ", "
		}
	}
	args = append(args, now, r.ID)
	sql += "updated_at = $" + strconv.Itoa(len(args)-1) + " WHERE id = $" + strconv.Itoa(len(args))
	_, err := db.Q().Exec(ctx, sql, args...)
	must(err)
	r.UpdatedAt = now
	return nil
}

// Destroy ports PrayerRequests::Destroy.
func Destroy(ctx context.Context, r *Request) {
	_, err := db.Q().Exec(ctx, `DELETE FROM prayer_requests WHERE id = $1`, r.ID)
	must(err)
}

// ErrNoSource and ErrCapacity port CopyPreviousWeek::NoSource and
// CapacityExceeded.
type CopyError struct {
	NotFound bool
	Message  string
}

func (e *CopyError) Error() string { return e.Message }

// CopyPreviousWeek ports PrayerRequests::CopyPreviousWeek.
func CopyPreviousWeek(ctx context.Context, u *users.User, target rb.YMD) ([]*Request, error) {
	source := target.AddDays(-7)
	var copied []*Request
	err := db.InTx(ctx, func(q pgx.Tx) error {
		rows, err := q.Query(ctx, `SELECT `+columns+` FROM prayer_requests WHERE user_id = $1 AND week_start = $2
			ORDER BY position ASC, created_at ASC`, u.ID, source.ISO())
		if err != nil {
			return err
		}
		var sources []*Request
		for rows.Next() {
			r, err := scan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			sources = append(sources, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(sources) == 0 {
			return &CopyError{NotFound: true, Message: "No prayer requests found for previous week (" + source.ISO() + ")"}
		}
		var existing int
		if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM prayer_requests WHERE user_id = $1 AND week_start = $2`, u.ID, target.ISO()).Scan(&existing); err != nil {
			return err
		}
		slots := MaxPerWeek - existing
		if slots <= 0 {
			return &CopyError{Message: "Maximum prayer requests reached for this week"}
		}
		for i, s := range sources[:min(slots, len(sources))] {
			r := &Request{UserID: u.ID, WeekStart: target, Title: s.Title, Content: s.Content}
			r.setPosition(int64(existing + i))
			var n int
			if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM prayer_requests WHERE user_id = $1 AND week_start = $2`, u.ID, target.ISO()).Scan(&n); err != nil {
				return err
			}
			msgs := r.errorsWithCount(n)
			if len(msgs) > 0 {
				panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + strings.Join(msgs, ", ")})
			}
			r.insert(ctx, q)
			copied = append(copied, r)
		}
		return nil
	})
	return copied, err
}

// errorsWithCount is errors(create) with the week's count already known
// (inside the copy's transaction).
func (r *Request) errorsWithCount(count int) []string {
	var msgs []string
	if r.Title == nil || rb.BlankString(*r.Title) {
		msgs = append(msgs, "Title can't be blank")
	}
	if r.Title != nil && len([]rune(*r.Title)) > 255 {
		msgs = append(msgs, "Title is too long (maximum is 255 characters)")
	}
	if r.Content != nil && !rb.BlankString(*r.Content) && len([]rune(*r.Content)) > 1000 {
		msgs = append(msgs, "Content is too long (maximum is 1000 characters)")
	}
	if count >= MaxPerWeek {
		msgs = append(msgs, "Maximum of "+strconv.Itoa(MaxPerWeek)+" prayer requests per week")
	}
	return msgs
}

// JSON ports PrayerRequestsController#prayer_request_response.
func (r *Request) JSON() *rb.Map {
	return rb.M("id", r.ID, "week_start", r.WeekStart.ISO(), "title", rb.Deref(r.Title), "content", rb.Deref(r.Content),
		"position", rb.Deref(r.Position), "created_at", rb.FormatTime(r.CreatedAt), "updated_at", rb.FormatTime(r.UpdatedAt))
}

// Digest ports PrayerRequest.requests_digest: the MD5 of the week's
// [title, content] pairs rendered by to_json.
func Digest(ctx context.Context, userID int64, week rb.YMD) string {
	pairs := []any{}
	for _, r := range ForWeek(ctx, userID, week) {
		pairs = append(pairs, []any{rb.Deref(r.Title), rb.Deref(r.Content)})
	}
	sum := md5.Sum(rb.ToJSON(pairs))
	return hex.EncodeToString(sum[:])
}
