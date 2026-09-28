// Package exams ports the self-examinations of a Rule of Life:
// LifeRuleExam and LifeRuleExamItem, the LifeRuleExams services (create,
// update, complete, stats, streaks, trends) and LifeRuleExamSerializer.
package exams

import (
	"context"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var (
	Periods  = []string{"weekly", "monthly", "seasonal", "custom"}
	Statuses = []string{"draft", "completed"}
	// Ratings ports LifeRuleExamItem::RATINGS (-1 is nil: not applicable).
	Ratings    = map[string]int{"faithful": 4, "mostly": 3, "partial": 2, "rarely": 1, "missed": 0, "not_applicable": -1}
	ratingKeys = []string{"faithful", "mostly", "partial", "rarely", "missed", "not_applicable"}
)

const (
	maxPoints     = 4
	maxTextLength = 5000
	maxNoteLength = 2000
	trendWindow   = 12
)

var examSchema = &ar.Schema{
	Table: "life_rule_exams",
	Columns: []string{"band", "client_id", "completed_at", "created_at", "focus_step_id", "intention", "life_rule_id",
		"life_rule_title", "period", "period_end", "period_start", "reflection", "score", "season_name", "season_slug",
		"status", "updated_at", "user_id"},
	Types: map[string]ar.ColType{"band": ar.String, "client_id": ar.String, "completed_at": ar.Datetime, "created_at": ar.Datetime,
		"focus_step_id": ar.Integer, "intention": ar.String, "life_rule_id": ar.Integer, "life_rule_title": ar.String,
		"period": ar.String, "period_end": ar.Date, "period_start": ar.Date, "reflection": ar.String, "score": ar.Integer,
		"season_name": ar.String, "season_slug": ar.String, "status": ar.String, "updated_at": ar.Datetime, "user_id": ar.Integer},
	Defaults: map[string]any{"intention": "", "period": "monthly", "reflection": "", "status": "draft"},
}

var itemSchema = &ar.Schema{
	Table: "life_rule_exam_items",
	Columns: []string{"client_id", "created_at", "life_rule_exam_id", "life_rule_step_id", "note", "rating", "step_description",
		"step_order", "step_title", "updated_at"},
	Types: map[string]ar.ColType{"client_id": ar.String, "created_at": ar.Datetime, "life_rule_exam_id": ar.Integer,
		"life_rule_step_id": ar.Integer, "note": ar.String, "rating": ar.String, "step_description": ar.String,
		"step_order": ar.Integer, "step_title": ar.String, "updated_at": ar.Datetime},
	Defaults: map[string]any{"note": "", "step_description": "", "step_order": int64(0)},
}

// Exam is a LifeRuleExam with its items.
type Exam struct {
	*ar.Record
	Items []*Item
}

// Item is a LifeRuleExamItem.
type Item struct{ *ar.Record }

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// Errors of the services, as the controller's rescue_from handlers name them.
type (
	NoLifeRule      struct{}
	AlreadyComplete struct{}
	NothingScored   struct{}
)

func (NoLifeRule) Error() string      { return "no life rule" }
func (AlreadyComplete) Error() string { return "already completed" }
func (NothingScored) Error() string   { return "nothing scored" }

const RecentFirst = "life_rule_exams.completed_at DESC NULLS FIRST, life_rule_exams.created_at DESC"

// Load runs `SELECT life_rule_exams.* ... WHERE where suffix` (no items).
func Load(ctx context.Context, q db.Querier, where, suffix string, args ...any) []*Exam {
	rows, err := q.Query(ctx, "SELECT "+examSchema.SelectList("life_rule_exams")+" FROM life_rule_exams WHERE "+where+" "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*Exam
	for rows.Next() {
		r, err := ar.ScanRecord(examSchema, rows)
		must(err)
		out = append(out, &Exam{Record: r})
	}
	must(rows.Err())
	return out
}

// LoadItems ports the items association (ORDER BY step_order, id).
func LoadItems(ctx context.Context, q db.Querier, e *Exam) {
	e.Items = loadItems(ctx, q, "life_rule_exam_items.life_rule_exam_id = $1", "", e.ID)
}

func loadItems(ctx context.Context, q db.Querier, where, suffix string, args ...any) []*Item {
	rows, err := q.Query(ctx, "SELECT "+itemSchema.SelectList("life_rule_exam_items")+" FROM life_rule_exam_items WHERE "+where+
		" ORDER BY life_rule_exam_items.step_order ASC, life_rule_exam_items.id ASC "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		r, err := ar.ScanRecord(itemSchema, rows)
		must(err)
		out = append(out, &Item{r})
	}
	must(rows.Err())
	return out
}

func (i *Item) points() (int, bool) {
	s, ok := i.Get("rating").(string)
	if !ok {
		return 0, false
	}
	p, known := Ratings[s]
	if !known || p < 0 {
		return 0, false
	}
	return p, true
}

func (i *Item) low() bool { r := i.Get("rating"); return r == "missed" || r == "rarely" }

// --- validations --------------------------------------------------------------

func (i *Item) errors(ctx context.Context) *ar.Errors {
	e := &ar.Errors{}
	cid := i.Get("client_id")
	if ar.Blank(cid) {
		e.Add("client_id", "can't be blank")
	}
	ar.TooLong(e, "client_id", cid, 120)
	if cid != nil {
		var taken bool
		var args []any
		sql := `SELECT EXISTS(SELECT 1 FROM life_rule_exam_items WHERE client_id = $1 AND `
		args = append(args, cid)
		if eid := i.Get("life_rule_exam_id"); eid != nil {
			args = append(args, eid)
			sql += `life_rule_exam_id = $2`
		} else {
			sql += `life_rule_exam_id IS NULL`
		}
		if !i.NewRecord {
			args = append(args, i.ID)
			sql += ` AND id <> $` + strconv.Itoa(len(args))
		}
		must(db.Conn(ctx).QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("client_id", "has already been taken")
		}
	}
	if ar.Blank(i.Get("step_title")) {
		e.Add("step_title", "can't be blank")
	}
	ar.TooLong(e, "step_title", i.Get("step_title"), 255)
	ar.Numeric(e, i.Record, "step_order", rb.GTE(0))
	if r := i.Get("rating"); r != nil && !ar.Included(r, ratingKeys) {
		ar.NotIncluded(e, "rating", r)
	}
	ar.TooLong(e, "note", i.Get("note"), maxNoteLength)
	return e
}

// Validate ports LifeRuleExam#valid?.
func (x *Exam) Validate(ctx context.Context) *ar.Errors {
	e := &ar.Errors{}
	for _, it := range x.Items {
		if !(x.NewRecord || it.NewRecord || it.HasChanges()) {
			continue
		}
		if inner := it.errors(ctx); inner.Any() {
			e.ImportFrom("items", inner)
		}
	}
	cid := x.Get("client_id")
	if ar.Blank(cid) {
		e.Add("client_id", "can't be blank")
	}
	ar.TooLong(e, "client_id", cid, 120)
	if cid != nil {
		var taken bool
		sql := `SELECT EXISTS(SELECT 1 FROM life_rule_exams WHERE client_id = $1 AND user_id = $2`
		args := []any{cid, x.Get("user_id")}
		if !x.NewRecord {
			sql += ` AND id <> $3`
			args = append(args, x.ID)
		}
		must(db.Conn(ctx).QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("client_id", "has already been taken")
		}
	}
	if ar.Blank(x.Get("life_rule_id")) {
		e.Add("life_rule_id", "can't be blank")
	}
	if ar.Blank(x.Get("life_rule_title")) {
		e.Add("life_rule_title", "can't be blank")
	}
	ar.TooLong(e, "life_rule_title", x.Get("life_rule_title"), 255)
	if !ar.Included(x.Get("period"), Periods) {
		ar.NotIncluded(e, "period", x.Get("period"))
	}
	if !ar.Included(x.Get("status"), Statuses) {
		ar.NotIncluded(e, "status", x.Get("status"))
	}
	if ar.Blank(x.Get("period_start")) {
		e.Add("period_start", "can't be blank")
	}
	if ar.Blank(x.Get("period_end")) {
		e.Add("period_end", "can't be blank")
	}
	ar.TooLong(e, "reflection", x.Get("reflection"), maxTextLength)
	ar.TooLong(e, "intention", x.Get("intention"), maxTextLength)
	if x.Get("score") != nil {
		ar.Numeric(e, x.Record, "score", rb.GTE(0), rb.LTE(100))
	}
	if ps, pe := x.str("period_start"), x.str("period_end"); ps != "" && pe != "" && pe < ps {
		e.Add("period_end", "must be on or after period_start")
	}
	e.Uniq()
	return e
}

func (x *Exam) str(c string) string { s, _ := x.Get(c).(string); return s }

// FullMessages ports errors.full_messages.
func FullMessages(e *ar.Errors) []string {
	var out []string
	for _, en := range e.Entries() {
		if en.Attr == "base" {
			out = append(out, en.Msg)
			continue
		}
		out = append(out, rb.Humanize(replaceDots(en.Attr))+" "+en.Msg)
	}
	return out
}

func replaceDots(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == '.' {
			b[i] = '_'
		}
	}
	return string(b)
}

// --- dates ------------------------------------------------------------------------

func today() civil.Date { return civil.FromTime(time.Now().In(rb.AppZone)) }

// addMonths ports Date#>> (the day clamps to the end of the month).
func addMonths(d civil.Date, n int) civil.Date {
	y, m := d.Year(), d.Month()-1+n
	y += floorDiv(m, 12)
	m = m - floorDiv(m, 12)*12 + 1
	day := min(d.Day(), civil.DaysInMonth(y, m))
	return civil.MustNew(y, m, day)
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func beginningOfWeek(d civil.Date) civil.Date { return d.Add(-((d.Weekday() + 6) % 7)) }

func beginningOfMonth(d civil.Date) civil.Date { return civil.MustNew(d.Year(), d.Month(), 1) }

func endOfMonth(d civil.Date) civil.Date {
	return civil.MustNew(d.Year(), d.Month(), civil.DaysInMonth(d.Year(), d.Month()))
}

func parseDate(s string) (civil.Date, bool) { return civil.ParseISO(s) }

// --- Period / SeasonalPeriod ---------------------------------------------------------

type period struct {
	Period, Start, End string
	Slug, Name         any
}

// prayerBookCode ports SeasonalPeriod#prayer_book_code.
func prayerBookCode(ctx context.Context, u *users.User) string {
	if u.Onboarding != nil {
		if pb, err := store.PrayerBookByID(ctx, u.Onboarding.PrayerBookID); err == nil && pb != nil && !rb.BlankString(pb.Code) {
			return pb.Code
		}
	}
	if u.Preferences != nil {
		if v := u.Preferences.Get("prayer_book_code"); rb.Present(v) {
			return rb.ToS(v)
		}
	}
	return books.DefaultCode
}

// seasonal ports SeasonalPeriod.call.
func seasonal(ctx context.Context, u *users.User, date civil.Date) period {
	code := prayerBookCode(ctx, u)
	sd := liturgical.NewSeasonDeterminator(date.Year(), nil, code)
	r := sd.RangeFor(date)
	return period{Period: "seasonal", Start: r.Start.ISO(), End: r.End.ISO(), Slug: nilIfEmpty(sd.SeasonSlugFor(date)), Name: r.Name}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// resolvePeriod ports LifeRuleExams::Period.call.
func resolvePeriod(ctx context.Context, u *users.User, attrs *rb.Map) period {
	p := "monthly"
	if v := attrs.Get("period"); rb.Present(v) {
		if s, ok := v.(string); ok && ar.Included(s, Periods) {
			p = s
		}
	}
	if p == "seasonal" {
		return seasonal(ctx, u, today())
	}
	defStart := beginningOfMonth(today())
	defEnd := endOfMonth(defStart)
	if p == "weekly" {
		defStart = beginningOfWeek(today())
		defEnd = defStart.Add(6)
	}
	parse := func(v any) (string, bool) {
		if rb.Blank(v) {
			return "", false
		}
		d, err := rb.DateParse(rb.ToS(v), true)
		if err != nil {
			if err != rb.ErrInvalidDate {
				panic(&web.StandardError{Class: "ArgumentError", Message: err.Error()})
			}
			return "", false
		}
		return d.ISO(), true
	}
	start, ok := parse(attrs.Get("period_start"))
	if !ok {
		start = defStart.ISO()
	}
	end, ok := parse(attrs.Get("period_end"))
	if !ok {
		end = defEnd.ISO()
	}
	return period{Period: p, Start: start, End: end}
}

// --- services -------------------------------------------------------------------------

func findByClientID(ctx context.Context, u *users.User, clientID any) *Exam {
	if rb.Blank(clientID) {
		return nil
	}
	list := Load(ctx, db.Conn(ctx), "life_rule_exams.user_id = $1 AND life_rule_exams.client_id = $2", "LIMIT 1", u.ID, rb.ToS(ar.CastString(clientID)))
	if len(list) == 0 {
		return nil
	}
	LoadItems(ctx, db.Conn(ctx), list[0])
	return list[0]
}

type lifeRule struct {
	id    int64
	title string
}

func usersLifeRule(ctx context.Context, u *users.User) *lifeRule {
	var r lifeRule
	err := db.Conn(ctx).QueryRow(ctx, `SELECT id, title FROM life_rules WHERE user_id = $1 LIMIT 1`, u.ID).Scan(&r.id, &r.title)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return &r
}

// Create ports LifeRuleExams::Create.
func Create(ctx context.Context, u *users.User, attrs *rb.Map) (*Exam, bool, error) {
	rule := usersLifeRule(ctx, u)
	if rule == nil {
		return nil, false, NoLifeRule{}
	}
	if x := findByClientID(ctx, u, attrs.Get("client_id")); x != nil {
		return x, false, nil
	}
	if drafts := Load(ctx, db.Conn(ctx), "life_rule_exams.user_id = $1 AND life_rule_exams.status = $2", "ORDER BY "+RecentFirst+" LIMIT 1", u.ID, "draft"); len(drafts) > 0 {
		LoadItems(ctx, db.Conn(ctx), drafts[0])
		return drafts[0], false, nil
	}
	x := &Exam{Record: ar.NewRecord(examSchema)}
	x.Set("user_id", u.ID)
	x.Record.Assign("client_id", attrs.Get("client_id"))
	x.Set("life_rule_id", rule.id)
	x.Set("life_rule_title", rule.title)
	x.Set("status", "draft")
	p := resolvePeriod(ctx, u, attrs)
	x.Set("period", p.Period)
	x.Set("period_start", p.Start)
	x.Set("period_end", p.End)
	x.Set("season_slug", p.Slug)
	x.Set("season_name", p.Name)
	rows, err := db.Conn(ctx).Query(ctx, `SELECT id, title, description, "order" FROM life_rule_steps WHERE life_rule_id = $1 ORDER BY "order" ASC`, rule.id)
	must(err)
	for rows.Next() {
		var id, order int64
		var title string
		var desc *string
		must(rows.Scan(&id, &title, &desc, &order))
		it := &Item{ar.NewRecord(itemSchema)}
		it.Set("client_id", "step-"+strconv.FormatInt(id, 10))
		it.Set("life_rule_step_id", id)
		it.Set("step_title", title)
		it.Set("step_description", rb.ToS(rb.Deref(desc)))
		it.Set("step_order", order)
		x.Items = append(x.Items, it)
	}
	rows.Close()
	must(rows.Err())
	if errs := x.Validate(ctx); errs.Any() {
		if errs.Has("client_id", "has already been taken") {
			if found := findByClientID(ctx, u, attrs.Get("client_id")); found != nil {
				return found, false, nil
			}
		}
		return nil, false, &ar.RecordInvalid{Errors: errs}
	}
	err = db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		now := users.Now()
		if err := x.Insert(ctx, tx, now); err != nil {
			return err
		}
		for _, it := range x.Items {
			it.Set("life_rule_exam_id", x.ID)
			if err := it.Insert(ctx, tx, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	LoadItems(ctx, db.Conn(ctx), x)
	return x, true, nil
}

// ItemInvalid is RecordInvalid raised by an item's save! (its own messages).
type ItemInvalid struct{ Errors *ar.Errors }

func (e *ItemInvalid) Error() string { return "Validation failed" }

// Update ports LifeRuleExams::Update after the policy check.
func Update(ctx context.Context, x *Exam, attrs *rb.Map) error {
	if x.Get("status") == "completed" {
		return AlreadyComplete{}
	}
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, k := range []string{"reflection", "intention", "focus_step_id"} {
			if v, ok := attrs.Lookup(k); ok {
				x.Record.Assign(k, v)
			}
		}
		if v, ok := attrs.Lookup("items_attributes"); ok {
			for _, el := range arrayWrap(v) {
				m, ok := el.(*rb.Map)
				if !ok {
					panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of Symbol into Integer"})
				}
				var found []*Item
				if id := m.Get("id"); rb.Present(id) {
					if n, ok := rb.CastInteger(id); ok {
						found = loadItemsTx(ctx, tx, "life_rule_exam_items.life_rule_exam_id = $1 AND life_rule_exam_items.id = $2", x.ID, n)
					}
				} else if cid := m.Get("client_id"); rb.Present(cid) {
					found = loadItemsTx(ctx, tx, "life_rule_exam_items.life_rule_exam_id = $1 AND life_rule_exam_items.client_id = $2", x.ID, rb.ToS(ar.CastString(cid)))
				}
				if len(found) == 0 {
					continue
				}
				it := found[0]
				for _, k := range []string{"rating", "note"} {
					if v, ok := m.Lookup(k); ok {
						it.Record.Assign(k, v)
					}
				}
				if errs := it.errors(ctx); errs.Any() {
					errs.Uniq()
					return &ItemInvalid{errs}
				}
				if _, err := it.Update(ctx, tx, users.Now()); err != nil {
					return err
				}
			}
		}
		if errs := x.Validate(ctx); errs.Any() {
			return &ar.RecordInvalid{Errors: errs}
		}
		_, err := x.Update(ctx, tx, users.Now())
		return err
	})
}

func loadItemsTx(ctx context.Context, tx pgx.Tx, where string, args ...any) []*Item {
	return loadItems(ctx, tx, where, "LIMIT 1", args...)
}

// arrayWrap ports Array(value): nil is [], an Array is itself, a Hash
// becomes its [key, value] pairs.
func arrayWrap(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	case *rb.Map:
		var out []any
		x.Each(func(k string, v any) { out = append(out, []any{k, v}) })
		return out
	}
	return []any{v}
}

// Score ports LifeRuleExams::Score.call.
func Score(items []*Item) (any, any) {
	sum, n := 0, 0
	for _, it := range items {
		if p, ok := it.points(); ok {
			sum += p
			n++
		}
	}
	if n == 0 {
		return nil, nil
	}
	score := int(math.Round(100.0 * float64(sum) / float64(maxPoints*n)))
	return score, bandFor(score)
}

func bandFor(score int) any {
	switch {
	case score >= 85 && score <= 100:
		return "flourishing"
	case score >= 65 && score <= 84:
		return "steady"
	case score >= 40 && score <= 64:
		return "wavering"
	case score >= 0 && score <= 39:
		return "arid"
	}
	return nil
}

// Complete ports LifeRuleExams::Complete after the policy check.
func Complete(ctx context.Context, x *Exam, attrs *rb.Map) error {
	if x.Get("status") == "completed" {
		return AlreadyComplete{}
	}
	if attrs.Len() > 0 {
		if err := Update(ctx, x, attrs); err != nil {
			return err
		}
		fresh := Load(ctx, db.Conn(ctx), "life_rule_exams.id = $1", "LIMIT 1", x.ID)
		x.Record = fresh[0].Record
	}
	LoadItems(ctx, db.Conn(ctx), x)
	score, band := Score(x.Items)
	if score == nil {
		return NothingScored{}
	}
	x.Set("status", "completed")
	x.Set("score", int64(score.(int)))
	x.Set("band", band)
	x.Set("completed_at", users.Now())
	if errs := x.Validate(ctx); errs.Any() {
		return &ar.RecordInvalid{Errors: errs}
	}
	_, err := x.Update(ctx, db.Conn(ctx), users.Now())
	return err
}

// Destroy ports LifeRuleExams::Destroy after the policy check.
func Destroy(ctx context.Context, x *Exam) error {
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM life_rule_exam_items WHERE life_rule_exam_id = $1`, x.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM life_rule_exams WHERE id = $1`, x.ID)
		return err
	})
}

// --- serializer ---------------------------------------------------------------------

func timeJSON(v any) any {
	if t, ok := v.(time.Time); ok {
		return rb.FormatTime(t)
	}
	return nil
}

// previousRatings ports LifeRuleExams::PreviousRatings.
func previousRatings(ctx context.Context, x *Exam) map[int64]any {
	prev := Load(ctx, db.Conn(ctx), "life_rule_exams.user_id = $1 AND life_rule_exams.status = $2 AND life_rule_exams.id != $3",
		"ORDER BY life_rule_exams.completed_at DESC LIMIT 1", x.Get("user_id"), "completed", x.ID)
	out := map[int64]any{}
	if len(prev) == 0 {
		return out
	}
	for _, it := range loadItems(ctx, db.Conn(ctx), "life_rule_exam_items.life_rule_exam_id = $1 AND life_rule_exam_items.life_rule_step_id IS NOT NULL", "", prev[0].ID) {
		out[it.Int("life_rule_step_id")] = it.Get("rating")
	}
	return out
}

// JSON ports LifeRuleExamSerializer#as_json.
func (x *Exam) JSON(ctx context.Context, withItems bool) *rb.Map {
	m := rb.M("id", x.ID, "client_id", x.Get("client_id"), "life_rule_id", x.Get("life_rule_id"),
		"life_rule_title", x.Get("life_rule_title"), "period", x.Get("period"), "period_start", x.Get("period_start"),
		"period_end", x.Get("period_end"), "season_slug", x.Get("season_slug"), "season_name", x.Get("season_name"),
		"status", x.Get("status"), "score", x.Get("score"), "band", x.Get("band"), "reflection", x.Get("reflection"),
		"intention", x.Get("intention"), "focus_step_id", x.Get("focus_step_id"), "completed_at", timeJSON(x.Get("completed_at")),
		"created_at", timeJSON(x.Get("created_at")), "updated_at", timeJSON(x.Get("updated_at")))
	if withItems {
		var prev map[int64]any
		items := []any{}
		for _, it := range x.Items {
			if prev == nil {
				prev = previousRatings(ctx, x)
			}
			var previous any
			if sid, ok := it.Get("life_rule_step_id").(int64); ok {
				previous = prev[sid]
			}
			items = append(items, rb.M("id", it.ID, "client_id", it.Get("client_id"), "life_rule_step_id", it.Get("life_rule_step_id"),
				"step_title", it.Get("step_title"), "step_description", it.Get("step_description"), "step_order", it.Get("step_order"),
				"rating", it.Get("rating"), "note", it.Get("note"), "previous_rating", previous))
		}
		m.Set("items", items)
	}
	return m
}

// --- stats: Cycle, Streak, StepTrends ---------------------------------------------

type span struct{ start, end civil.Date }

func dateOf(v any) civil.Date { d, _ := parseDate(rb.ToS(v)); return d }

func seasonRange(ctx context.Context, u *users.User, d civil.Date) span {
	p := seasonal(ctx, u, d)
	s, _ := parseDate(p.Start)
	e, _ := parseDate(p.End)
	return span{s, e}
}

// previousWindow ports Cycle#previous (ok=false for custom).
func previousWindow(ctx context.Context, u *users.User, x *Exam) (span, bool) {
	ps := dateOf(x.Get("period_start"))
	switch x.Get("period") {
	case "weekly":
		return span{ps.Add(-7), ps.Add(-1)}, true
	case "monthly":
		return span{addMonths(ps, -1), ps.Add(-1)}, true
	case "seasonal":
		return span{seasonRange(ctx, u, ps.Add(-1)).start, ps.Add(-1)}, true
	}
	return span{}, false
}

// following ports Cycle#following.
func following(ctx context.Context, u *users.User, x *Exam) (span, bool) {
	ps, pe := dateOf(x.Get("period_start")), dateOf(x.Get("period_end"))
	switch x.Get("period") {
	case "weekly":
		return span{ps.Add(7), ps.Add(13)}, true
	case "monthly":
		start := addMonths(ps, 1)
		return span{start, addMonths(start, 1).Add(-1)}, true
	case "seasonal":
		return seasonRange(ctx, u, pe.Add(1)), true
	case "custom":
		days := pe.Sub(ps) + 1
		return span{pe.Add(1), pe.Add(days)}, true
	}
	return span{}, false
}

func completedRecent(ctx context.Context, u *users.User) []*Exam {
	return Load(ctx, db.Conn(ctx), "life_rule_exams.user_id = $1 AND life_rule_exams.status = $2", "ORDER BY "+RecentFirst, u.ID, "completed")
}

// Streak ports LifeRuleExams::Streak.
func Streak(ctx context.Context, u *users.User, exams []*Exam) int {
	if len(exams) == 0 {
		return 0
	}
	streak := 1
	for i := 0; i+1 < len(exams); i++ {
		later, earlier := exams[i], exams[i+1]
		if later.Get("period") != earlier.Get("period") {
			break
		}
		w, ok := previousWindow(ctx, u, later)
		if !ok {
			break
		}
		eps := dateOf(earlier.Get("period_start"))
		if eps < w.start || eps > w.end {
			break
		}
		streak++
	}
	return streak
}

type trend struct {
	stepID int64
	title  string
	scores []int
}

// StepTrends ports LifeRuleExams::StepTrends.
func StepTrends(ctx context.Context, u *users.User) []trend {
	type step struct {
		id    int64
		title string
	}
	var steps []step
	rows, err := db.Conn(ctx).Query(ctx, `SELECT life_rule_steps.id, life_rule_steps.title FROM life_rule_steps
		WHERE life_rule_steps.life_rule_id = (SELECT id FROM life_rules WHERE user_id = $1 LIMIT 1) ORDER BY life_rule_steps."order" ASC`, u.ID)
	must(err)
	titles := map[int64]string{}
	for rows.Next() {
		var s step
		must(rows.Scan(&s.id, &s.title))
		if _, dup := titles[s.id]; !dup {
			steps = append(steps, s)
		}
		titles[s.id] = s.title
	}
	rows.Close()
	must(rows.Err())
	var chrono []int64
	rows, err = db.Conn(ctx).Query(ctx, `SELECT id FROM life_rule_exams WHERE user_id = $1 AND status = $2
		ORDER BY completed_at ASC NULLS LAST, created_at ASC`, u.ID, "completed")
	must(err)
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		chrono = append(chrono, id)
	}
	rows.Close()
	must(rows.Err())
	if len(chrono) > trendWindow {
		chrono = chrono[len(chrono)-trendWindow:]
	}
	if len(steps) == 0 || len(chrono) == 0 {
		return nil
	}
	pos := map[int64]int{}
	for i, id := range chrono {
		pos[id] = i
	}
	stepIDs := make([]int64, len(steps))
	for i, s := range steps {
		stepIDs[i] = s.id
	}
	type row struct {
		step   int64
		rating string
		exam   int64
	}
	var rs []row
	q, err := db.Conn(ctx).Query(ctx, `SELECT life_rule_step_id, rating, life_rule_exam_id FROM life_rule_exam_items
		WHERE life_rule_exam_id = ANY($1) AND life_rule_step_id = ANY($2) AND NOT (rating IS NULL OR rating = 'not_applicable')`, chrono, stepIDs)
	must(err)
	for q.Next() {
		var r row
		must(q.Scan(&r.step, &r.rating, &r.exam))
		rs = append(rs, r)
	}
	q.Close()
	must(q.Err())
	sort.SliceStable(rs, func(i, j int) bool { return pos[rs[i].exam] < pos[rs[j].exam] })
	grouped := map[int64][]int{}
	for _, r := range rs {
		p := Ratings[r.rating]
		grouped[r.step] = append(grouped[r.step], int(math.Round(100.0*float64(p)/maxPoints)))
	}
	var out []trend
	for _, s := range steps {
		if sc := grouped[s.id]; len(sc) > 0 {
			out = append(out, trend{s.id, titles[s.id], sc})
		}
	}
	return out
}

func intsAny(v []int) []any {
	out := make([]any, len(v))
	for i, x := range v {
		out[i] = x
	}
	return out
}

// CompletionContext ports LifeRuleExams::CompletionContext.
func CompletionContext(ctx context.Context, u *users.User, x *Exam) *rb.Map {
	completed := completedRecent(ctx, u)
	prev := Load(ctx, db.Conn(ctx), "life_rule_exams.user_id = $1 AND life_rule_exams.status = $2 AND life_rule_exams.id != $3",
		"ORDER BY life_rule_exams.completed_at DESC LIMIT 1", u.ID, "completed", x.ID)
	var previousScore any
	var previous *Exam
	if len(prev) > 0 {
		previous = prev[0]
		previousScore = previous.Get("score")
	}
	low := 0
	for _, it := range x.Items {
		if it.low() {
			low++
		}
	}
	suggest := x.Get("band") == "arid" || (x.Get("band") == "wavering" && previous != nil && previous.Get("band") == "wavering") || low >= 3
	trends := []any{}
	for _, t := range StepTrends(ctx, u) {
		trends = append(trends, rb.M("life_rule_step_id", t.stepID, "scores", intsAny(t.scores)))
	}
	return rb.M("previous_score", previousScore, "exam_streak", Streak(ctx, u, completed), "total_exams", len(completed),
		"suggest_rule_review", suggest, "step_trends", trends)
}

// Stats ports LifeRuleExams::Stats.
func Stats(ctx context.Context, u *users.User) *rb.Map {
	exams := completedRecent(ctx, u)
	var avg, first, last, next any
	cadence := any("monthly")
	var scores []int64
	for _, x := range exams {
		if s, ok := x.Get("score").(int64); ok {
			scores = append(scores, s)
		}
	}
	if len(scores) > 0 {
		var sum int64
		for _, s := range scores {
			sum += s
		}
		avg = int(math.Round(float64(sum) / float64(len(scores))))
	}
	if len(exams) > 0 {
		first = timeJSON(exams[len(exams)-1].Get("completed_at"))
		latest := exams[0]
		last = timeJSON(latest.Get("completed_at"))
		if f, ok := following(ctx, u, latest); ok {
			next = f.end.Add(1).ISO()
		}
		cadence = latest.Get("period")
	}
	trends := []any{}
	for _, t := range StepTrends(ctx, u) {
		trends = append(trends, rb.M("life_rule_step_id", t.stepID, "step_title", t.title, "scores", intsAny(t.scores),
			"delta", t.scores[len(t.scores)-1]-t.scores[0]))
	}
	return rb.M("total_exams", len(exams), "exam_streak", Streak(ctx, u, exams), "average_score", avg,
		"first_exam_at", first, "last_exam_at", last, "suggested_next_at", next, "cadence", cadence, "step_trends", trends)
}
