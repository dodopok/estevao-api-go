package v1

import (
	"errors"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/exams"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var (
	examFields     = []string{"client_id", "period", "period_start", "period_end", "reflection", "intention", "focus_step_id"}
	examItemFields = []string{"id", "client_id", "rating", "note"}
)

type examNotFound struct{}

// rescueExam ports LifeRuleExamsController's rescue_from handlers.
func rescueExam(c *web.Context) {
	rec := recover()
	if rec == nil {
		return
	}
	if _, ok := rec.(examNotFound); ok {
		c.JSON(404, rb.M("error", "Life rule exam not found"))
		return
	}
	if err, ok := rec.(error); ok && renderExamError(c, err) {
		return
	}
	panic(rec)
}

func renderExamError(c *web.Context, err error) bool {
	var invalid *ar.RecordInvalid
	var itemInvalid *exams.ItemInvalid
	switch {
	case errors.As(err, &invalid):
		c.JSON(422, rb.M("errors", exams.FullMessages(invalid.Errors)))
	case errors.As(err, &itemInvalid):
		c.JSON(422, rb.M("errors", exams.FullMessages(itemInvalid.Errors)))
	case errors.As(err, new(exams.NoLifeRule)):
		c.JSON(422, rb.M("error", "You need an adopted life rule before examining it"))
	case errors.As(err, new(exams.AlreadyComplete)):
		c.JSON(409, rb.M("error", "This exam is already completed and cannot be changed"))
	case errors.As(err, new(exams.NothingScored)):
		c.JSON(422, rb.M("error", "An exam needs at least one answered step before it can be completed"))
	default:
		return false
	}
	return true
}

func examMust(err error) {
	if err == nil {
		return
	}
	if !errors.As(err, new(*ar.RecordInvalid)) && !errors.As(err, new(*exams.ItemInvalid)) &&
		!errors.As(err, new(exams.NoLifeRule)) && !errors.As(err, new(exams.AlreadyComplete)) &&
		!errors.As(err, new(exams.NothingScored)) {
		pgMust(err)
	}
	panic(err)
}

// examParams ports `params.fetch(:life_rule_exam, {}).permit(...)`.
func examParams(c *web.Context) *rb.Map {
	raw, ok := c.Params().Lookup("life_rule_exam")
	if !ok {
		return rb.NewMap()
	}
	m, isMap := raw.(*rb.Map)
	if !isMap {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("permit", raw)})
	}
	out := permitScalars(m, examFields)
	if v, ok := m.Lookup("items_attributes"); ok && v != nil && v != false {
		if p, ok := web.PermitHashOrArray(v, func(x *rb.Map) *rb.Map { return permitScalars(x, examItemFields) }); ok {
			out.Set("items_attributes", p)
		}
	}
	return out
}

func permitScalars(m *rb.Map, fields []string) *rb.Map {
	out := rb.NewMap()
	for _, f := range fields {
		if v, ok := m.Lookup(f); ok && web.PermittedScalar(v) {
			out.Set(f, v)
		}
	}
	return out
}

// loadExam loads one of the current user's exams by id, with its items.
func loadExam(c *web.Context, id int64) *exams.Exam {
	u := auth.CurrentUser(c)
	found := exams.Load(c.Ctx, db.Q(), "life_rule_exams.user_id = $1 AND life_rule_exams.id = $2", "LIMIT 1", u.ID, id)
	if len(found) == 0 {
		panic(examNotFound{})
	}
	exams.LoadItems(c.Ctx, db.Q(), found[0])
	return found[0]
}

// setExam ports set_exam (scoped to the user by LifeRuleExamPolicy.scope).
func setExam(c *web.Context) *exams.Exam {
	id, ok := findID(c.Param("id"))
	if !ok {
		panic(examNotFound{})
	}
	return loadExam(c, id)
}

// whereLifeRuleID ports `where(life_rule_id: value)` for a request param.
func whereLifeRuleID(v any, args []any) (string, []any) {
	col := "life_rule_exams.life_rule_id"
	switch x := v.(type) {
	case []any:
		var in []string
		null := false
		for _, e := range x {
			n, ok := rb.CastInteger(e)
			if !ok {
				null = true
				continue
			}
			args = append(args, n)
			in = append(in, "$"+itoa(len(args)))
		}
		var parts []string
		if len(in) > 0 {
			parts = append(parts, col+" IN ("+strings.Join(in, ", ")+")")
		}
		if null {
			parts = append(parts, col+" IS NULL")
		}
		if len(parts) == 0 {
			return "1=0", args
		}
		if len(parts) == 2 {
			return "(" + strings.Join(parts, " OR ") + ")", args
		}
		return parts[0], args
	case *rb.Map:
		// A hash names a table: where(life_rule_id: {k => v}) filters on
		// "life_rule_id"."k", which PostgreSQL rejects.
		var parts []string
		x.Each(func(k string, e any) {
			args = append(args, rb.ToS(e))
			parts = append(parts, `"life_rule_id"."`+strings.ReplaceAll(k, `"`, `""`)+`"`+" = $"+itoa(len(args)))
		})
		return strings.Join(parts, " AND "), args
	}
	n, ok := rb.CastInteger(v)
	if !ok {
		return col + " IS NULL", args
	}
	args = append(args, n)
	return col + " = $" + itoa(len(args)), args
}

// LifeRuleExamsIndex ports LifeRuleExamsController#index.
func LifeRuleExamsIndex(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	where, args := "life_rule_exams.user_id = $1", []any{u.ID}
	if v := c.Param("life_rule_id"); rb.Present(v) {
		var clause string
		clause, args = whereLifeRuleID(v, args)
		where += " AND " + clause
	}
	var total int64
	pgMust(db.Q().QueryRow(c.Ctx, "SELECT COUNT(life_rule_exams.id) FROM life_rule_exams WHERE "+where, args...).Scan(&total))
	limit := min(max(paramToI(c.Param("limit"), 20), 1), 100)
	offset := max(paramToI(c.Param("offset"), 0), 0)
	n := len(args)
	page := exams.Load(c.Ctx, db.Q(), where, "ORDER BY "+exams.RecentFirst+" LIMIT $"+itoa(n+1)+" OFFSET $"+itoa(n+2),
		append(args, limit, offset)...)
	out := []any{}
	for _, x := range page {
		out = append(out, x.JSON(c.Ctx, false))
	}
	c.JSON(200, rb.M("exams", out, "pagination", rb.M("total", total, "limit", limit, "offset", offset, "count", len(page))))
}

// LifeRuleExamsShow ports #show.
func LifeRuleExamsShow(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	x := setExam(c)
	c.JSON(200, rb.M("life_rule_exam", x.JSON(c.Ctx, true)))
}

// LifeRuleExamsCurrent ports #current.
func LifeRuleExamsCurrent(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	found := exams.Load(c.Ctx, db.Q(), "life_rule_exams.user_id = $1 AND life_rule_exams.status = $2",
		"ORDER BY "+exams.RecentFirst+" LIMIT 1", u.ID, "draft")
	if len(found) == 0 {
		panic(examNotFound{})
	}
	exams.LoadItems(c.Ctx, db.Q(), found[0])
	c.JSON(200, rb.M("life_rule_exam", found[0].JSON(c.Ctx, true)))
}

// LifeRuleExamsCreate ports #create (LifeRuleExams::Create).
func LifeRuleExamsCreate(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	x, created, err := exams.Create(c.Ctx, u, examParams(c))
	examMust(err)
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, rb.M("life_rule_exam", x.JSON(c.Ctx, true)))
}

// LifeRuleExamsUpdate ports #update (LifeRuleExams::Update, then reload).
func LifeRuleExamsUpdate(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	x := setExam(c)
	examMust(exams.Update(c.Ctx, x, examParams(c)))
	x = loadExam(c, x.ID)
	c.JSON(200, rb.M("life_rule_exam", x.JSON(c.Ctx, true)))
}

// LifeRuleExamsComplete ports #complete.
func LifeRuleExamsComplete(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	x := setExam(c)
	examMust(exams.Complete(c.Ctx, x, examParams(c)))
	c.JSON(200, rb.M("life_rule_exam", x.JSON(c.Ctx, true), "context", exams.CompletionContext(c.Ctx, u, x)))
}

// LifeRuleExamsDestroy ports #destroy.
func LifeRuleExamsDestroy(c *web.Context) {
	defer rescueExam(c)
	auth.AuthenticateRequired(c)
	x := setExam(c)
	pgMust(exams.Destroy(c.Ctx, x))
	c.HeadStatus(204)
}

// LifeRuleExamsStats ports #stats.
func LifeRuleExamsStats(c *web.Context) {
	auth.AuthenticateRequired(c)
	c.JSON(200, exams.Stats(c.Ctx, auth.CurrentUser(c)))
}
