package v1

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// OfficeTypesAll is DailyOffice::OfficeTypes::ALL.
var OfficeTypesAll = []string{"morning", "prime", "terce", "midday", "none", "evening", "compline", "late_evening"}

func validOfficeType(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, o := range OfficeTypesAll {
		if o == s {
			return true
		}
	}
	return false
}

// argumentError is the ArgumentError a controller's rescue_from answers.
type argumentError struct{ msg string }

func raiseArgument(msg string) { panic(&argumentError{msg}) }

// rescueArgument ports `rescue_from ArgumentError` rendering 400 {error:}.
func rescueArgument(c *web.Context) {
	if rec := recover(); rec != nil {
		if a, ok := rec.(*argumentError); ok {
			c.JSON(400, rb.M("error", a.msg))
			return
		}
		if re, ok := rec.(*rb.RubyError); ok && re.Class == "ArgumentError" {
			c.JSON(400, rb.M("error", re.Message))
			return
		}
		panic(rec)
	}
}

// userToday ports User#user_today (raises ArgumentError for a zone Rails
// does not know).
func userToday(u *users.User, now time.Time) (int, int, int) {
	loc := rb.RailsZone(u.Timezone)
	if loc == nil {
		raiseArgument("Invalid Timezone: " + u.Timezone)
	}
	t := now.In(loc)
	return t.Year(), int(t.Month()), t.Day()
}

// updateStreak ports User#update_streak!.
func updateStreak(ctx context.Context, u *users.User) error {
	now := users.Now()
	cur, longest := 0, 0
	if u.CurrentStreak != nil {
		cur = *u.CurrentStreak
	}
	if u.LongestStreak != nil {
		longest = *u.LongestStreak
	}
	var nc, nl int
	if u.LastCompletedOfficeAt == nil {
		nc, nl = 1, max(1, longest)
	} else {
		loc := rb.RailsZone(u.Timezone)
		if loc == nil {
			raiseArgument("Invalid Timezone: " + u.Timezone)
		}
		l := u.LastCompletedOfficeAt.In(loc)
		t := now.In(loc)
		last := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
		today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		switch int(today.Sub(last).Hours() / 24) {
		case 0:
			nc, nl = cur, longest
		case 1:
			nc = cur + 1
			nl = max(nc, longest)
		default:
			nc, nl = 1, max(1, longest)
		}
	}
	if msgs := u.Errors(); len(msgs) > 0 {
		return &users.RecordInvalid{Messages: msgs}
	}
	return users.UpdateColumnsAt(ctx, db.Q(), u.ID, map[string]any{
		"current_streak": nc, "longest_streak": nl, "last_completed_office_at": now}, now)
}

// currentPrayerBookID ports User#current_prayer_book_id.
func currentPrayerBookID(ctx context.Context, u *users.User) *int64 {
	var code any
	if u.Preferences != nil {
		code = u.Preferences.Get("prayer_book_code")
		if rb.Blank(code) {
			code = u.Preferences.Get("version")
		}
	}
	pb, err := findBook(ctx, code)
	must(err)
	if pb == nil {
		return nil
	}
	return &pb.ID
}

func validationFailed(msgs []string) string {
	return "Validation failed: " + strings.Join(msgs, ", ")
}

// completionJSON ports completion.as_json(only: keys) (keys in that order).
func completionJSON(ctx context.Context, id int64, keys []string) *rb.Map {
	var date, created time.Time
	var office string
	var duration *int
	var pbID *int64
	must(db.Q().QueryRow(ctx, `SELECT date_reference, office_type, duration_seconds, prayer_book_id, created_at FROM completions WHERE id = $1`, id).
		Scan(&date, &office, &duration, &pbID, &created))
	vals := map[string]any{"id": id, "date_reference": date.Format("2006-01-02"), "office_type": office,
		"duration_seconds": intOrNil(duration), "prayer_book_id": rb.Deref(pbID), "created_at": rb.FormatTime(created)}
	cols, err := db.OrderedAttributes(ctx, "completions", keys)
	must(err)
	m := rb.NewMap()
	for _, k := range cols {
		m.Set(k, vals[k])
	}
	return m
}

// CompletionsCreate ports CompletionsController#create (User#complete_office!).
func CompletionsCreate(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	param := func(k string) any {
		v, ok := c.Params().Lookup(k)
		if !ok || !permittedScalar(v) {
			return nil
		}
		return v
	}
	var y, m, d int64
	if date := param("date"); rb.Present(date) {
		parsed, err := rb.DateParse(rb.ToS(date), true)
		if err != nil {
			raiseArgument(err.Error())
		}
		y, m, d = parsed.Y, parsed.M, parsed.D
	} else {
		ty, tm, td := userToday(u, time.Now())
		y, m, d = int64(ty), int64(tm), int64(td)
	}
	office := param("office_type")
	officeStr := castString(office)
	var duration any
	if v := param("duration_seconds"); v != nil {
		if n, ok := castIntegerParam(v); ok {
			duration = n
		}
	}
	var msgs []string
	if officeStr == nil || rb.BlankString(*officeStr) {
		msgs = append(msgs, "Office type can't be blank")
	}
	if officeStr == nil || !validOfficeType(*officeStr) {
		msgs = append(msgs, "Office type "+rb.ToS(rb.Deref(officeStr))+" is not a valid office type")
	}
	dateRef := rb.YMD{Y: y, M: m, D: d}.ISO()
	if officeStr != nil {
		var exists bool
		must(db.Q().QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM completions WHERE user_id = $1 AND date_reference = $2 AND office_type = $3)`,
			u.ID, dateRef, *officeStr).Scan(&exists))
		if exists {
			msgs = append(msgs, "User already completed this office today")
		}
	}
	if len(msgs) > 0 {
		c.JSON(422, rb.M("error", validationFailed(msgs)))
		return
	}
	now := users.Now()
	var id int64
	err := db.Q().QueryRow(c.Ctx, `INSERT INTO completions (user_id, date_reference, office_type, duration_seconds, prayer_book_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id`, u.ID, dateRef, *officeStr, duration, currentPrayerBookID(c.Ctx, u), now).Scan(&id)
	pgMust(err)
	if err := updateStreak(c.Ctx, u); err != nil {
		var ri *users.RecordInvalid
		if errors.As(err, &ri) {
			c.JSON(422, rb.M("error", validationFailed(ri.Messages)))
			return
		}
		pgMust(err)
	}
	fresh, err := u.Reload(c.Ctx)
	must(err)
	c.JSON(201, rb.M("message", "Office completed successfully",
		"completion", completionJSON(c.Ctx, id, []string{"id", "date_reference", "office_type", "duration_seconds", "created_at", "prayer_book_id"}),
		"current_streak", intOrNil(fresh.CurrentStreak), "longest_streak", intOrNil(fresh.LongestStreak)))
}

// castIntegerParam ports ActiveModel::Type::Integer#cast for a parameter.
func castIntegerParam(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		// Numeric#cast: a blank string is nil; anything else goes through
		// String#to_i, which is 0 when there are no leading digits.
		if rb.BlankString(x) {
			return 0, false
		}
		s := strings.TrimLeft(x, " \t\n\v\f\r")
		i := 0
		if i < len(s) && (s[i] == '-' || s[i] == '+') {
			i++
		}
		j := i
		isDigit := func(k int) bool { return k < len(s) && s[k] >= '0' && s[k] <= '9' }
		// A single underscore may separate digits ("1_000"); "1__2" stops at 1.
		for isDigit(j) || j > i && j < len(s) && s[j] == '_' && isDigit(j+1) {
			j++
		}
		if j == i {
			return 0, true
		}
		n := int64(0)
		for _, ch := range s[i:j] {
			if ch != '_' {
				n = n*10 + int64(ch-'0')
			}
		}
		if s[0] == '-' {
			n = -n
		}
		return n, true
	}
	return 0, false
}

// completionsDate ports CompletionsController#parse_date.
func completionsDate(c *web.Context) (int, int, int) {
	y, m, d := rb.StringToI(c.ParamS("year")), rb.StringToI(c.ParamS("month")), rb.StringToI(c.ParamS("day"))
	if y < 1900 || y > 2200 {
		raiseArgument("Year must be between 1900 and 2200")
	}
	if m < 1 || m > 12 {
		raiseArgument("Month must be between 1 and 12")
	}
	if d < 1 || d > 31 {
		raiseArgument("Invalid day for the specified month")
	}
	if _, ok := rb.CastDate(rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}.ISO()); !ok {
		raiseArgument("invalid date")
	}
	return y, m, d
}

// CompletionsDay ports CompletionsController#day.
func CompletionsDay(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	y, m, d := completionsDate(c)
	date := rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}.ISO()
	rows, err := db.Q().Query(c.Ctx, `SELECT id FROM completions WHERE user_id = $1 AND date_reference = $2 ORDER BY office_type ASC`, u.ID, date)
	must(err)
	var ids []int64
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	list := []any{}
	for _, id := range ids {
		list = append(list, completionJSON(c.Ctx, id, []string{"id", "date_reference", "office_type", "duration_seconds", "created_at"}))
	}
	c.JSON(200, rb.M("date", date, "completions", list))
}

// CompletionsShow ports CompletionsController#show.
func CompletionsShow(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	y, m, d := completionsDate(c)
	office := rb.ToS(c.Param("office_type"))
	if !validOfficeType(office) {
		raiseArgument("Invalid office type. Must be one of: " + strings.Join(OfficeTypesAll, ", "))
	}
	var id int64
	err := db.Q().QueryRow(c.Ctx, `SELECT id FROM completions WHERE user_id = $1 AND date_reference = $2 AND office_type = $3 LIMIT 1`,
		u.ID, rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}.ISO(), office).Scan(&id)
	if db.NoRows(err) {
		c.JSON(404, rb.NewMap())
		return
	}
	must(err)
	c.JSON(200, rb.M("message", "Office completed", "completion", completionJSON(c.Ctx, id, []string{"id", "created_at"})))
}

// CompletionsDestroy ports CompletionsController#destroy.
func CompletionsDestroy(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	id, ok := findID(c.Param("id"))
	var found int64
	if ok {
		err := db.Q().QueryRow(c.Ctx, `DELETE FROM completions WHERE user_id = $1 AND id = $2 RETURNING id`, u.ID, id).Scan(&found)
		if err != nil && !db.NoRows(err) {
			panic(err)
		}
	}
	if found == 0 {
		c.JSON(404, rb.M("error", "Completion not found"))
		return
	}
	if err := updateStreak(c.Ctx, u); err != nil {
		var ri *users.RecordInvalid
		if errors.As(err, &ri) {
			panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: validationFailed(ri.Messages)})
		}
		pgMust(err)
	}
	fresh, err := u.Reload(c.Ctx)
	must(err)
	c.JSON(200, rb.M("message", "Completion removed successfully",
		"current_streak", intOrNil(fresh.CurrentStreak), "longest_streak", intOrNil(fresh.LongestStreak)))
}

// findID ports the id cast of `find(params[:id])`: ActiveModel's integer
// cast of the parameter (nil, i.e. not found, when it is not numeric).
func findID(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	return castIntegerParam(v)
}

var _ = store.PrayerBookByCode
