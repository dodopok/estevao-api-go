package v1

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

type journal struct {
	id                   int64
	date                 *rb.YMD
	entryType, office    *string
	content              *string
	createdAt, updatedAt time.Time
}

const journalColumns = `id, date_reference, entry_type, office_type, content, created_at, updated_at`

func scanJournal(row interface{ Scan(...any) error }) (*journal, error) {
	var j journal
	var d time.Time
	var entry, content string
	if err := row.Scan(&j.id, &d, &entry, &j.office, &content, &j.createdAt, &j.updatedAt); err != nil {
		return nil, err
	}
	j.date = &rb.YMD{Y: int64(d.Year()), M: int64(d.Month()), D: int64(d.Day())}
	j.entryType, j.content = &entry, &content
	return &j, nil
}

func journalJSON(j *journal) *rb.Map {
	var date any
	if j.date != nil {
		date = j.date.ISO()
	}
	return rb.M("id", j.id, "date_reference", date, "entry_type", rb.Deref(j.entryType), "office_type", rb.Deref(j.office),
		"content", rb.Deref(j.content), "created_at", rb.FormatTime(j.createdAt), "updated_at", rb.FormatTime(j.updatedAt))
}

// journalErrors ports the Journal validations as full messages.
func journalErrors(j *journal) []string {
	var out []string
	if j.date == nil {
		out = append(out, "Date reference can't be blank")
	}
	if j.entryType == nil || rb.BlankString(*j.entryType) {
		out = append(out, "Entry type can't be blank")
	}
	if j.entryType == nil || (*j.entryType != "daily_office" && *j.entryType != "life_rule") {
		out = append(out, "Entry type "+rb.ToS(rb.Deref(j.entryType))+" is not a valid entry type")
	}
	if j.entryType != nil && *j.entryType == "daily_office" && (j.office == nil || !validOfficeType(*j.office)) {
		out = append(out, "Office type "+rb.ToS(rb.Deref(j.office))+" is not a valid office type")
	}
	if j.content == nil || rb.BlankString(*j.content) {
		out = append(out, "Content can't be blank")
	}
	n := 0
	if j.content != nil {
		n = utf8.RuneCountInString(*j.content)
	}
	if n < 1 {
		out = append(out, "Content is too short (minimum is 1 character)")
	}
	if n > 10000 {
		out = append(out, "Content is too long (maximum is 10000 characters)")
	}
	return out
}

// journalParams ports journal_params: the permitted keys (params[:journal]
// or, without it, the top level) that are present.
func journalParams(c *web.Context) *rb.Map {
	return requirePermit(c, "journal", "date_reference", "entry_type", "office_type", "content")
}

func applyJournal(j *journal, attrs *rb.Map) {
	attrs.Each(func(k string, v any) {
		switch k {
		case "date_reference":
			j.date = nil
			if s, ok := v.(string); ok {
				if d, ok := rb.CastDate(s); ok {
					j.date = &d
				}
			} else if v != nil {
				if d, ok := rb.CastDate(rb.ToS(v)); ok {
					j.date = &d
				}
			}
		case "entry_type":
			j.entryType = castString(v)
		case "office_type":
			j.office = castString(v)
		case "content":
			j.content = castString(v)
		}
	})
}

func journalInvalid(c *web.Context, msgs []string) {
	c.JSON(422, rb.M("error", strings.Join(msgs, ", ")))
}

// JournalsCreate ports JournalsController#create.
func JournalsCreate(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	j := &journal{}
	applyJournal(j, journalParams(c))
	if msgs := journalErrors(j); len(msgs) > 0 {
		journalInvalid(c, msgs)
		return
	}
	now := users.Now()
	err := db.Q().QueryRow(c.Ctx, `INSERT INTO journals (user_id, date_reference, entry_type, office_type, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id, created_at, updated_at`, u.ID, j.date.ISO(), *j.entryType, j.office, *j.content, now).
		Scan(&j.id, &j.createdAt, &j.updatedAt)
	pgMust(err)
	c.JSON(201, rb.M("message", "Journal entry created successfully", "journal", journalJSON(j)))
}

// setJournal ports set_journal (JournalPolicy.scope(user).find(params[:id])).
func setJournal(c *web.Context, u *users.User) *journal {
	id, ok := findID(c.Param("id"))
	if ok {
		j, err := scanJournal(db.Q().QueryRow(c.Ctx, `SELECT `+journalColumns+` FROM journals WHERE user_id = $1 AND id = $2 LIMIT 1`, u.ID, id))
		if err == nil {
			return j
		}
		if !db.NoRows(err) {
			panic(err)
		}
	}
	c.RenderJSON(404, rb.M("error", "Journal entry not found"))
	return nil
}

// JournalsUpdate ports JournalsController#update.
func JournalsUpdate(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	j := setJournal(c, u)
	before := *j
	applyJournal(j, journalParams(c))
	if msgs := journalErrors(j); len(msgs) > 0 {
		journalInvalid(c, msgs)
		return
	}
	changed := before.date.ISO() != j.date.ISO() || !strPtrEq(before.entryType, j.entryType) ||
		!strPtrEq(before.office, j.office) || !strPtrEq(before.content, j.content)
	if changed {
		j.updatedAt = users.Now()
		_, err := db.Q().Exec(c.Ctx, `UPDATE journals SET date_reference = $1, entry_type = $2, office_type = $3, content = $4, updated_at = $5 WHERE id = $6`,
			j.date.ISO(), *j.entryType, j.office, *j.content, j.updatedAt, j.id)
		pgMust(err)
	}
	c.JSON(200, rb.M("message", "Journal entry updated successfully", "journal", journalJSON(j)))
}

// JournalsDestroy ports JournalsController#destroy.
func JournalsDestroy(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	j := setJournal(c, u)
	_, err := db.Q().Exec(c.Ctx, `DELETE FROM journals WHERE id = $1`, j.id)
	must(err)
	c.JSON(200, rb.M("message", "Journal entry deleted successfully"))
}

// journalDate ports DateValidations#parse_date (InvalidDate domain errors).
func journalDate(c *web.Context, withDay bool) (int, int, int) {
	y, m := rb.StringToI(c.ParamS("year")), rb.StringToI(c.ParamS("month"))
	invalid := func(msg string) { web.Raise("InvalidDate", msg, "") }
	if y < 1900 || y > 2200 {
		invalid("Ano deve estar entre 1900 e 2200")
	}
	if m < 1 || m > 12 {
		invalid("Mês deve estar entre 1 e 12")
	}
	if !withDay {
		return y, m, 0
	}
	d := rb.StringToI(c.ParamS("day"))
	if d < 1 || d > 31 {
		invalid("Dia inválido para o mês especificado")
	}
	if _, ok := rb.CastDate(rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}.ISO()); !ok {
		invalid("Invalid date: " + itoa(y) + "-" + itoa(m) + "-" + itoa(d))
	}
	return y, m, d
}

// JournalsDay ports JournalsController#day.
func JournalsDay(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	y, m, d := journalDate(c, true)
	date := rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}.ISO()
	rows, err := db.Q().Query(c.Ctx, `SELECT `+journalColumns+` FROM journals WHERE user_id = $1 AND date_reference = $2
		ORDER BY date_reference DESC, created_at DESC`, u.ID, date)
	must(err)
	list := []any{}
	for rows.Next() {
		j, err := scanJournal(rows)
		must(err)
		list = append(list, journalJSON(j))
	}
	rows.Close()
	c.JSON(200, rb.M("date", date, "count", len(list), "entries", list))
}

// JournalsMonth ports JournalsController#month.
func JournalsMonth(c *web.Context) {
	defer rescueArgument(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	y, m, _ := journalDate(c, false)
	first := rb.YMD{Y: int64(y), M: int64(m), D: 1}.ISO()
	last := rb.YMD{Y: int64(y), M: int64(m), D: int64(time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day())}.ISO()
	// No ORDER BY, like the relation Rails groups: first-seen order. The
	// query selects every column as Active Record does, so PostgreSQL picks
	// the same plan (and row order) instead of an index-only scan.
	rows, err := db.Q().Query(c.Ctx, `SELECT journals.* FROM journals WHERE journals.user_id = $1
		AND journals.date_reference BETWEEN $2 AND $3`, u.ID, first, last)
	must(err)
	cols := rows.FieldDescriptions()
	dateIdx, typeIdx := -1, -1
	for i, f := range cols {
		switch f.Name {
		case "date_reference":
			dateIdx = i
		case "entry_type":
			typeIdx = i
		}
	}
	byDate := rb.NewMap()
	total := 0
	for rows.Next() {
		vals, err := rows.Values()
		must(err)
		d := vals[dateIdx].(time.Time)
		entry, _ := vals[typeIdx].(string)
		total++
		key := d.Format("2006-01-02")
		e, _ := byDate.Get(key).(*rb.Map)
		if e == nil {
			e = rb.M("count", 0, "types", rb.NewMap())
			byDate.Set(key, e)
		}
		e.Set("count", e.Get("count").(int)+1)
		types := e.Get("types").(*rb.Map)
		n, _ := types.Get(entry).(int)
		types.Set(entry, n+1)
	}
	rows.Close()
	dates := []any{}
	for _, k := range rb.SortedKeys(byDate) {
		dates = append(dates, k)
	}
	c.JSON(200, rb.M("year", y, "month", m, "total_entries", total, "dates_with_entries", dates, "entries_by_date", byDate))
}
