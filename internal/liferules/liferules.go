// Package liferules ports LifeRule and LifeRuleStep (nested steps,
// validations, callbacks) and the LifeRules services.
package liferules

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Locales and DefaultLocale port LifeRule::LOCALES and DEFAULT_LOCALE.
var Locales = []string{"pt-BR", "pt-PT", "en", "es"}

const DefaultLocale = "pt-BR"

var ruleSchema = &ar.Schema{
	Table: "life_rules",
	Columns: []string{"adoption_count", "approved", "created_at", "description", "icon", "is_public", "locale",
		"original_life_rule_id", "title", "translation_key", "updated_at", "user_id"},
	Types: map[string]ar.ColType{"adoption_count": ar.Integer, "approved": ar.Boolean, "created_at": ar.Datetime,
		"description": ar.String, "icon": ar.String, "is_public": ar.Boolean, "locale": ar.String,
		"original_life_rule_id": ar.Integer, "title": ar.String, "translation_key": ar.String, "updated_at": ar.Datetime,
		"user_id": ar.Integer},
	Defaults: map[string]any{"adoption_count": int64(0), "approved": false, "is_public": false, "locale": "pt-BR"},
}

var stepSchema = &ar.Schema{
	Table:   "life_rule_steps",
	Columns: []string{"created_at", "description", "life_rule_id", "order", "title", "updated_at"},
	Types: map[string]ar.ColType{"created_at": ar.Datetime, "description": ar.String, "life_rule_id": ar.Integer,
		"order": ar.Integer, "title": ar.String, "updated_at": ar.Datetime},
}

// Rule is a LifeRule with the steps loaded or assigned so far.
type Rule struct {
	*ar.Record
	Steps []*Step
}

// Step is a LifeRuleStep.
type Step struct{ *ar.Record }

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// Load runs `SELECT life_rules.* FROM life_rules WHERE <where> <suffix>`
// (no steps).
func Load(ctx context.Context, q db.Querier, where, suffix string, args ...any) []*Rule {
	sql := "SELECT " + ruleSchema.SelectList("life_rules") + " FROM life_rules"
	if where != "" {
		sql += " WHERE " + where
	}
	rows, err := q.Query(ctx, sql+" "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*Rule
	for rows.Next() {
		r, err := ar.ScanRecord(ruleSchema, rows)
		must(err)
		out = append(out, &Rule{Record: r})
	}
	must(rows.Err())
	return out
}

// LoadSteps ports includes(:life_rule_steps) (default scope ORDER BY order).
func LoadSteps(ctx context.Context, q db.Querier, rules []*Rule) {
	if len(rules) == 0 {
		return
	}
	byID := map[int64]*Rule{}
	ids := make([]int64, len(rules))
	for i, r := range rules {
		byID[r.ID] = r
		ids[i] = r.ID
		r.Steps = nil
	}
	rows, err := q.Query(ctx, "SELECT "+stepSchema.SelectList("life_rule_steps")+
		` FROM life_rule_steps WHERE life_rule_steps.life_rule_id = ANY($1) ORDER BY life_rule_steps."order" ASC`, ids)
	must(err)
	defer rows.Close()
	for rows.Next() {
		rec, err := ar.ScanRecord(stepSchema, rows)
		must(err)
		r := byID[rec.Int("life_rule_id")]
		r.Steps = append(r.Steps, &Step{rec})
	}
	must(rows.Err())
}

var (
	enPrefix = regexp.MustCompile(`(?im)^en-`)
	esPrefix = regexp.MustCompile(`(?im)^es-`)
)

// NormalizeLocale ports LifeRule.normalize_locale.
func NormalizeLocale(v any) string {
	s := strings.ReplaceAll(strings.Trim(rb.ToS(v), " \t\n\v\f\r\x00"), "_", "-")
	switch {
	case s == "pt" || s == "pt-BR" || s == "pt-br":
		return DefaultLocale
	case s == "pt-PT" || s == "pt-pt":
		return "pt-PT"
	case s == "en" || enPrefix.MatchString(s):
		return "en"
	case s == "es" || esPrefix.MatchString(s):
		return "es"
	}
	return DefaultLocale
}

func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- nested steps -------------------------------------------------------------

// NotFound is the ActiveRecord::RecordNotFound of a stale nested step id.
type NotFound struct{ Message string }

func (e *NotFound) Error() string { return e.Message }

func destroyFlag(attrs *rb.Map) bool { return ar.Truthy(ar.CastBool(attrs.Get("_destroy"))) }

func noMethod(method string, recv any) {
	panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage(method, recv)})
}

// Assign ports assign_attributes with life_rule_steps_attributes (nested
// attributes with allow_destroy; hashes are assigned after the plain
// attributes).
func (r *Rule) Assign(ctx context.Context, attrs *rb.Map) {
	var nested any
	has := false
	attrs.Each(func(k string, v any) {
		if k == "life_rule_steps_attributes" {
			if _, isMap := v.(*rb.Map); isMap {
				nested, has = v, true
				return
			}
			r.assignSteps(ctx, v)
			return
		}
		r.Record.Assign(k, v)
	})
	if has {
		r.assignSteps(ctx, nested)
	}
}

func (r *Rule) assignSteps(ctx context.Context, v any) {
	var items []any
	switch x := v.(type) {
	case []any:
		items = x
	case *rb.Map:
		if x.Has("id") {
			items = []any{x}
		} else {
			x.Each(func(_ string, v any) { items = append(items, v) })
		}
	default:
		panic(&web.StandardError{Class: "ArgumentError", Message: "Hash or Array expected for `life_rule_steps` attributes, got " + rb.ClassName(v)})
	}
	// association.scope.where(id: ids) when not loaded
	var ids []any
	for _, el := range items {
		switch e := el.(type) {
		case *rb.Map:
			if id := e.Get("id"); ar.Truthy(id) {
				ids = append(ids, id)
			}
		case string:
			if strings.Contains(e, "id") {
				ids = append(ids, "id")
			}
		case int, int64, []any:
			panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of String into Integer"})
		default:
			noMethod("[]", e)
		}
	}
	var existing []*Step
	if !r.NewRecord && len(ids) > 0 {
		var cast []int64
		for _, id := range ids {
			if n, ok := rb.CastInteger(id); ok {
				cast = append(cast, n)
			}
		}
		rows, err := db.Conn(ctx).Query(ctx, "SELECT "+stepSchema.SelectList("life_rule_steps")+
			` FROM life_rule_steps WHERE life_rule_steps.life_rule_id = $1 AND life_rule_steps.id = ANY($2) ORDER BY life_rule_steps."order" ASC`, r.ID, cast)
		must(err)
		for rows.Next() {
			rec, err := ar.ScanRecord(stepSchema, rows)
			must(err)
			existing = append(existing, &Step{rec})
		}
		rows.Close()
		must(rows.Err())
	}
	for _, el := range items {
		attrs, ok := el.(*rb.Map)
		if !ok {
			noMethod("with_indifferent_access", el)
		}
		id := attrs.Get("id")
		if rb.Blank(id) {
			if !destroyFlag(attrs) {
				s := &Step{ar.NewRecord(stepSchema)}
				if !r.NewRecord {
					s.Set("life_rule_id", r.ID)
				}
				s.assign(attrs)
				r.Steps = append(r.Steps, s)
			}
			continue
		}
		var found *Step
		for _, s := range append(append([]*Step{}, r.Steps...), existing...) {
			if !s.NewRecord && strconv.FormatInt(s.ID, 10) == rb.ToS(id) {
				found = s
				break
			}
		}
		if found == nil {
			owner := strconv.FormatInt(r.ID, 10)
			if r.NewRecord {
				owner = ""
			}
			panic(&NotFound{"Couldn't find LifeRuleStep with ID=" + rb.ToS(id) + " for LifeRule with ID=" + owner})
		}
		inTarget := false
		for _, s := range r.Steps {
			if s == found {
				inTarget = true
			}
		}
		if !inTarget {
			r.Steps = append(r.Steps, found)
		}
		found.assign(attrs)
		if destroyFlag(attrs) {
			found.Marked = true
		}
	}
}

func (s *Step) assign(attrs *rb.Map) {
	attrs.Each(func(k string, v any) {
		if k != "id" && k != "_destroy" {
			s.Assign(k, v)
		}
	})
}

// --- validations --------------------------------------------------------------

func (s *Step) changedForAutosave() bool { return s.NewRecord || s.HasChanges() || s.Marked }

func (s *Step) errors(ctx context.Context) *ar.Errors {
	e := &ar.Errors{}
	if ar.Blank(s.Get("order")) {
		e.Add("order", "can't be blank")
	}
	ar.Numeric(e, s.Record, "order", rb.GTE(0))
	if ar.Blank(s.Get("title")) {
		e.Add("title", "can't be blank")
	}
	if rid, order := s.Get("life_rule_id"), s.Get("order"); rid != nil && order != nil {
		sql := `SELECT EXISTS(SELECT 1 FROM life_rule_steps WHERE life_rule_id = $1 AND "order" = $2`
		args := []any{rid, order}
		if !s.NewRecord {
			sql += ` AND id <> $3`
			args = append(args, s.ID)
		}
		var taken bool
		must(db.Conn(ctx).QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("order", "has already been taken")
		}
	}
	return e
}

// Validate ports LifeRule#valid?.
func (r *Rule) Validate(ctx context.Context) *ar.Errors {
	e := &ar.Errors{}
	for _, s := range r.Steps {
		if s.Marked || !(r.NewRecord || s.changedForAutosave()) {
			continue
		}
		if inner := s.errors(ctx); inner.Any() {
			e.ImportFrom("life_rule_steps", inner)
		}
	}
	if ar.Blank(r.Get("icon")) {
		e.Add("icon", "can't be blank")
	}
	if ar.Blank(r.Get("title")) {
		e.Add("title", "can't be blank")
	}
	if _, ok := r.Get("is_public").(bool); !ok {
		ar.NotIncluded(e, "is_public", r.Get("is_public"))
	}
	if _, ok := r.Get("approved").(bool); !ok {
		ar.NotIncluded(e, "approved", r.Get("approved"))
	}
	if uid := r.Get("user_id"); uid != nil {
		sql := `SELECT EXISTS(SELECT 1 FROM life_rules WHERE user_id = $1`
		args := []any{uid}
		if !r.NewRecord {
			sql += ` AND id <> $2`
			args = append(args, r.ID)
		}
		var taken bool
		must(db.Conn(ctx).QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("user_id", "has already been taken")
		}
	}
	if !ar.Included(r.Get("locale"), Locales) {
		ar.NotIncluded(e, "locale", r.Get("locale"))
	}
	if ar.Blank(r.Get("translation_key")) {
		e.Add("translation_key", "can't be blank")
	}
	e.Uniq()
	return e
}

// FullMessages ports errors.full_messages ("life_rule_steps.order" reads
// "Life rule steps order ...", "base" messages stand alone).
func FullMessages(e *ar.Errors) []string {
	var out []string
	for _, en := range e.Entries() {
		if en.Attr == "base" {
			out = append(out, en.Msg)
			continue
		}
		out = append(out, rb.Humanize(strings.ReplaceAll(en.Attr, ".", "_"))+" "+en.Msg)
	}
	return out
}

// --- persistence --------------------------------------------------------------

// Save ports save!: before_validation callbacks, validations, then the rule
// and its steps (inside the caller's transaction when q is one).
func (r *Rule) Save(ctx context.Context, q db.Querier) error {
	r.Set("locale", NormalizeLocale(r.Get("locale")))
	create := r.NewRecord
	if create && r.Get("translation_key") == nil {
		r.Set("translation_key", "rule:"+uuid())
	}
	if errs := r.Validate(ctx); errs.Any() {
		return &ar.RecordInvalid{Errors: errs}
	}
	now := users.Now()
	if create {
		if err := r.Insert(ctx, q, now); err != nil {
			return err
		}
	} else if _, err := r.Update(ctx, q, now); err != nil {
		return err
	}
	var kept []*Step
	for _, s := range r.Steps {
		if s.Marked {
			if !s.NewRecord {
				if _, err := q.Exec(ctx, `DELETE FROM life_rule_steps WHERE id = $1`, s.ID); err != nil {
					return err
				}
			}
			continue
		}
		kept = append(kept, s)
	}
	for _, s := range kept {
		switch {
		case s.NewRecord || create:
			s.Set("life_rule_id", r.ID)
			if err := s.Insert(ctx, q, now); err != nil {
				return err
			}
		case s.HasChanges():
			if _, err := s.Update(ctx, q, now); err != nil {
				return err
			}
		}
	}
	r.Steps = kept
	return nil
}

// destroy ports life_rule.destroy! (steps destroyed, adoptions nullified).
func destroy(ctx context.Context, q db.Querier, id int64) error {
	for _, sql := range []string{
		`DELETE FROM life_rule_steps WHERE life_rule_id = $1`,
		`UPDATE life_rules SET original_life_rule_id = NULL WHERE original_life_rule_id = $1`,
		`DELETE FROM life_rules WHERE id = $1`,
	} {
		if _, err := q.Exec(ctx, sql, id); err != nil {
			return err
		}
	}
	return nil
}

// Destroy ports LifeRules::Destroy.
func Destroy(ctx context.Context, r *Rule) error {
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { return destroy(ctx, tx, r.ID) })
}

func destroyUsersRule(ctx context.Context, q db.Querier, userID int64) error {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM life_rules WHERE user_id = $1 LIMIT 1`, userID).Scan(&id)
	if db.NoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return destroy(ctx, q, id)
}

// Create ports LifeRules::Create: the user's rule is replaced.
func Create(ctx context.Context, u *users.User, attrs *rb.Map) (*Rule, error) {
	r := &Rule{Record: ar.NewRecord(ruleSchema)}
	r.Set("user_id", u.ID)
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := destroyUsersRule(ctx, tx, u.ID); err != nil {
			return err
		}
		r.Assign(ctx, attrs)
		return r.Save(ctx, tx)
	})
	return r, err
}

// Update ports LifeRules::Update after the policy check.
func Update(ctx context.Context, r *Rule, attrs *rb.Map) error {
	r.Assign(ctx, attrs)
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { return r.Save(ctx, tx) })
}

// Approve ports update!(approved: true).
func Approve(ctx context.Context, r *Rule) error {
	r.Set("approved", true)
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { return r.Save(ctx, tx) })
}

// Adopt ports LifeRules::Adopt.
func Adopt(ctx context.Context, u *users.User, original *Rule) (*Rule, error) {
	var adopted *Rule
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		locked := Load(ctx, tx, "life_rules.id = $1", "LIMIT 1 FOR UPDATE", original.ID)
		if len(locked) == 0 {
			return &NotFound{"Couldn't find LifeRule"}
		}
		o := locked[0]
		if err := destroyUsersRule(ctx, tx, u.ID); err != nil {
			return err
		}
		adopted = &Rule{Record: ar.NewRecord(ruleSchema)}
		for k, v := range map[string]any{"user_id": u.ID, "original_life_rule_id": o.ID, "icon": o.Get("icon"),
			"title": o.Get("title"), "description": o.Get("description"), "locale": o.Get("locale"),
			"is_public": false, "approved": false} {
			adopted.Set(k, v)
		}
		if err := adopted.Save(ctx, tx); err != nil {
			return err
		}
		LoadSteps(ctx, tx, []*Rule{o})
		for _, step := range o.Steps {
			s := &Step{ar.NewRecord(stepSchema)}
			s.Set("life_rule_id", adopted.ID)
			s.Set("order", step.Get("order"))
			s.Set("title", step.Get("title"))
			s.Set("description", step.Get("description"))
			if errs := s.errors(ctx); errs.Any() {
				return &ar.RecordInvalid{Errors: errs}
			}
			if err := s.Insert(ctx, tx, users.Now()); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE life_rules SET adoption_count = COALESCE(adoption_count, 0) + 1 WHERE life_rules.id = $1`, o.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	fresh := Load(ctx, db.Conn(ctx), "life_rules.id = $1", "LIMIT 1", adopted.ID)
	return fresh[0], nil
}

// JSON ports serialize_life_rule.
func (r *Rule) JSON(currentUserID *int64, withSteps bool) *rb.Map {
	uid, _ := r.Get("user_id").(int64)
	isOwner := currentUserID != nil && uid == *currentUserID
	m := rb.M("id", r.ID, "icon", r.Get("icon"), "locale", r.Get("locale"), "title", r.Get("title"),
		"description", r.Get("description"), "is_public", r.Get("is_public"), "approved", r.Get("approved"),
		"adoption_count", r.Get("adoption_count"), "is_owner", isOwner, "created_at", timeJSON(r.Get("created_at")))
	if withSteps {
		steps := []any{}
		for _, s := range r.Steps {
			steps = append(steps, rb.M("id", s.ID, "order", s.Get("order"), "title", s.Get("title"), "description", s.Get("description")))
		}
		m.Set("steps", steps)
	}
	return m
}

func timeJSON(v any) any {
	if t, ok := v.(time.Time); ok {
		return rb.FormatTime(t)
	}
	return nil
}

var stepFields = []string{"id", "order", "title", "description", "_destroy"}

// Permit ports params.require(:life_rule).permit(:icon, :title,
// :description, :locale, :is_public, life_rule_steps_attributes: [...]).
func Permit(params *rb.Map) *rb.Map {
	v, ok := params.Lookup("life_rule")
	if !ok || !(rb.Present(v) || v == false) {
		panic(&web.StandardError{Class: "ActionController::ParameterMissing",
			Message: "param is missing or the value is empty or invalid: life_rule"})
	}
	m, isMap := v.(*rb.Map)
	if !isMap {
		noMethod("permit", v)
	}
	out := rb.NewMap()
	for _, f := range []string{"icon", "title", "description", "locale", "is_public"} {
		if v, ok := m.Lookup(f); ok && scalar(v) {
			out.Set(f, v)
		}
	}
	if v, ok := m.Lookup("life_rule_steps_attributes"); ok {
		if p, ok := permitNested(v); ok {
			out.Set("life_rule_steps_attributes", p)
		}
	}
	return out
}

func scalar(v any) bool {
	switch v.(type) {
	case *rb.Map, []any:
		return false
	}
	return true
}

func permitStep(m *rb.Map) *rb.Map {
	out := rb.NewMap()
	for _, f := range stepFields {
		if v, ok := m.Lookup(f); ok && scalar(v) {
			out.Set(f, v)
		}
	}
	return out
}

func permitNested(v any) (any, bool) { return web.PermitHashOrArray(v, permitStep) }

// ReloadSteps replaces the steps with the persisted ones (by order), as
// reading life_rule_steps after a save does.
func (r *Rule) ReloadSteps(ctx context.Context) { LoadSteps(ctx, db.Conn(ctx), []*Rule{r}) }

// Localized ports localized_rule_for.
func Localized(variants []*Rule, locale string) *Rule {
	if len(variants) == 0 {
		return nil
	}
	for _, want := range []string{locale, DefaultLocale} {
		for _, r := range variants {
			if r.Get("locale") == want {
				return r
			}
		}
	}
	return variants[0]
}
