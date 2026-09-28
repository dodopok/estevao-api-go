package v1

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liferules"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// lifeRuleScope ports LifeRulePolicy.scope as SQL (with the bind values
// Active Record sends).
func lifeRuleScope(u *users.User) (string, []any) {
	if u == nil {
		return "life_rules.is_public = $1 AND life_rules.approved = $2", []any{true, true}
	}
	return "(life_rules.user_id = $1 OR life_rules.is_public = $2 AND life_rules.approved = $3)", []any{u.ID, true, true}
}

func currentUserID(c *web.Context) *int64 {
	if u := auth.CurrentUser(c); u != nil {
		return &u.ID
	}
	return nil
}

// lifeRuleLocale ports locale (requested_locale normalized).
func lifeRuleLocale(c *web.Context) string {
	var requested any
	if v := c.Param("locale"); rb.Present(v) {
		requested = v
	} else if v := c.Param("language"); rb.Present(v) {
		requested = v
	} else if u := auth.CurrentUser(c); u != nil && u.Preferences != nil {
		requested = u.Preferences.Get("language")
	}
	return liferules.NormalizeLocale(requested)
}

func lifeRuleJSON(c *web.Context, r *liferules.Rule, steps bool) *rb.Map {
	return r.JSON(currentUserID(c), steps)
}

func lifeRuleNotFound(c *web.Context, msg string) {
	c.RenderJSON(404, rb.M("error", msg))
}

func lifeRuleFullMessages(err error) ([]string, bool) {
	var inv *ar.RecordInvalid
	if errors.As(err, &inv) {
		return liferules.FullMessages(inv.Errors), true
	}
	return nil, false
}

// LifeRulesIndex ports LifeRulesController#index.
func LifeRulesIndex(c *web.Context) {
	auth.AuthenticateOptional(c)
	where, args := lifeRuleScope(auth.CurrentUser(c))
	candidates := liferules.Load(c.Ctx, db.Q(), where, "", args...)
	liferules.LoadSteps(c.Ctx, db.Q(), candidates)
	locale := lifeRuleLocale(c)
	var keys []string
	groups := map[string][]*liferules.Rule{}
	for _, r := range candidates {
		k := rb.ToS(r.Get("translation_key"))
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], r)
	}
	var rules []*liferules.Rule
	for _, k := range keys {
		rules = append(rules, liferules.Localized(groups[k], locale))
	}
	if v := c.Param("search"); rb.Present(v) {
		query := strings.ToLower(rb.ToS(v))
		var kept []*liferules.Rule
		for _, r := range rules {
			if strings.Contains(strings.ToLower(rb.ToS(r.Get("title"))), query) {
				kept = append(kept, r)
			}
		}
		rules = kept
	}
	if s := rb.ToS(c.Param("sort")); s == "popular" || s == "popularity" {
		sort.SliceStable(rules, func(i, j int) bool { return rules[i].Int("adoption_count") > rules[j].Int("adoption_count") })
	}
	total := len(rules)
	limit := min(max(paramToI(c.Param("limit"), 20), 1), 100)
	offset := max(paramToI(c.Param("offset"), 0), 0)
	var page []*liferules.Rule
	if offset <= int64(total) {
		page = rules[offset:min(int64(total), offset+limit)]
	}
	out := []any{}
	for _, r := range page {
		out = append(out, lifeRuleJSON(c, r, false))
	}
	c.JSON(200, rb.M("life_rules", out, "pagination", rb.M("total", total, "limit", limit, "offset", offset, "count", len(page))))
}

func findRuleID(c *web.Context) (int64, bool) { return findID(c.Param("id")) }

// LifeRulesShow ports #show (set_life_rule).
func LifeRulesShow(c *web.Context) {
	auth.AuthenticateOptional(c)
	where, args := lifeRuleScope(auth.CurrentUser(c))
	id, ok := findRuleID(c)
	var found []*liferules.Rule
	if ok {
		found = liferules.Load(c.Ctx, db.Q(), where+" AND life_rules.id = $"+itoa(len(args)+1), "LIMIT 1", append(args, id)...)
	}
	if len(found) == 0 {
		lifeRuleNotFound(c, "Life rule not found")
	}
	variants := liferules.Load(c.Ctx, db.Q(), where+" AND life_rules.translation_key = $"+itoa(len(args)+1), "",
		append(args, found[0].Get("translation_key"))...)
	r := liferules.Localized(variants, lifeRuleLocale(c))
	r.ReloadSteps(c.Ctx)
	c.JSON(200, rb.M("life_rule", lifeRuleJSON(c, r, true)))
}

// LifeRulesMy ports #my.
func LifeRulesMy(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	rules := liferules.Load(c.Ctx, db.Q(), "life_rules.user_id = $1", "", u.ID)
	liferules.LoadSteps(c.Ctx, db.Q(), rules)
	r := liferules.Localized(rules, lifeRuleLocale(c))
	if r == nil {
		c.JSON(404, rb.M("error", "Life rule not found"))
		return
	}
	c.JSON(200, rb.M("life_rule", lifeRuleJSON(c, r, true)))
}

// LifeRulesCreate ports #create (LifeRules::Create).
func LifeRulesCreate(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	attrs := liferules.Permit(c.Params())
	r, err := liferules.Create(c.Ctx, u, attrs)
	if msgs, ok := lifeRuleFullMessages(err); ok {
		c.JSON(422, rb.M("error", msgs))
		return
	}
	var nf *liferules.NotFound
	if errors.As(err, &nf) {
		panic(&web.StandardError{Class: "ActiveRecord::RecordNotFound", Message: nf.Message})
	}
	pgMust(err)
	r.ReloadSteps(c.Ctx)
	c.JSON(201, rb.M("message", "Life rule created successfully", "life_rule", lifeRuleJSON(c, r, true)))
}

// setOwnedLifeRule ports set_life_rule_for_owner_action + authorize_owner!.
func setOwnedLifeRule(c *web.Context, u *users.User) *liferules.Rule {
	var found []*liferules.Rule
	if id, ok := findRuleID(c); ok {
		found = liferules.Load(c.Ctx, db.Q(), "life_rules.id = $1", "LIMIT 1", id)
	}
	if len(found) == 0 {
		lifeRuleNotFound(c, "Life rule not found")
	}
	if found[0].Int("user_id") != u.ID {
		c.RenderJSON(403, rb.M("error", "Unauthorized"))
	}
	return found[0]
}

// LifeRulesUpdate ports #update (LifeRules::Update).
func LifeRulesUpdate(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	r := setOwnedLifeRule(c, u)
	attrs := liferules.Permit(c.Params())
	err := func() (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				if nf, ok := rec.(*liferules.NotFound); ok {
					err = nf
					return
				}
				panic(rec)
			}
		}()
		return liferules.Update(c.Ctx, r, attrs)
	}()
	if msgs, ok := lifeRuleFullMessages(err); ok {
		c.JSON(422, rb.M("error", msgs))
		return
	}
	var nf *liferules.NotFound
	if errors.As(err, &nf) {
		c.JSON(422, rb.M("error", []string{"One or more life rule steps no longer exist. Reload the life rule and try again."}))
		return
	}
	pgMust(err)
	r.ReloadSteps(c.Ctx)
	c.JSON(200, rb.M("message", "Life rule updated successfully", "life_rule", lifeRuleJSON(c, r, true)))
}

// LifeRulesDestroy ports #destroy.
func LifeRulesDestroy(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	r := setOwnedLifeRule(c, u)
	pgMust(liferules.Destroy(c.Ctx, r))
	c.JSON(200, rb.M("message", "Life rule deleted successfully"))
}

// LifeRulesAdopt ports #adopt (LifeRules::Adopt).
func LifeRulesAdopt(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	public := "life_rules.is_public = $1 AND life_rules.approved = $2"
	var found []*liferules.Rule
	if id, ok := findRuleID(c); ok {
		found = liferules.Load(c.Ctx, db.Q(), public+" AND life_rules.id = $3", "LIMIT 1", true, true, id)
	}
	if len(found) == 0 {
		c.JSON(404, rb.M("error", "Life rule not found or not public"))
		return
	}
	variants := liferules.Load(c.Ctx, db.Q(), public+" AND life_rules.translation_key = $3", "", true, true, found[0].Get("translation_key"))
	original := liferules.Localized(variants, lifeRuleLocale(c))
	adopted, err := liferules.Adopt(c.Ctx, u, original)
	if msgs, ok := lifeRuleFullMessages(err); ok {
		c.JSON(422, rb.M("error", msgs))
		return
	}
	var nf *liferules.NotFound
	if errors.As(err, &nf) {
		c.JSON(404, rb.M("error", "Life rule not found or not public"))
		return
	}
	pgMust(err)
	adopted.ReloadSteps(c.Ctx)
	c.JSON(201, rb.M("message", "Life rule adopted successfully", "life_rule", lifeRuleJSON(c, adopted, true)))
}

// LifeRulesApprove ports #approve.
func LifeRulesApprove(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	var found []*liferules.Rule
	if id, ok := findRuleID(c); ok {
		found = liferules.Load(c.Ctx, db.Q(), "life_rules.is_public = $1 AND life_rules.id = $2", "LIMIT 1", true, id)
	}
	if len(found) == 0 {
		lifeRuleNotFound(c, "Life rule not found")
	}
	if !auth.IsAdmin(u) {
		c.RenderJSON(403, rb.M("error", "Unauthorized - Admin only"))
	}
	r := found[0]
	err := liferules.Approve(c.Ctx, r)
	if msgs, ok := lifeRuleFullMessages(err); ok {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + strings.Join(msgs, ", ")})
	}
	pgMust(err)
	r.ReloadSteps(c.Ctx)
	c.JSON(200, rb.M("message", "Life rule approved successfully", "life_rule", lifeRuleJSON(c, r, true)))
}

// AdminLifeRulesIndex ports Admin::LifeRulesController#index.
func AdminLifeRulesIndex(c *web.Context) {
	auth.AuthenticateAdmin(c)
	status := c.Param("status")
	if rb.Present(status) {
		s, ok := status.(string)
		if !ok || (s != "all" && s != "pending" && s != "approved") {
			c.JSON(400, rb.M("error", "Invalid status. Use pending, approved, or all"))
			return
		}
	}
	where := "life_rules.is_public = $1"
	args := []any{true}
	switch rb.Presence(status) {
	case "pending":
		args = append(args, false)
		where += " AND life_rules.approved = $2"
	case "approved":
		args = append(args, true)
		where += " AND life_rules.approved = $2"
	}
	if v := c.Param("search"); rb.Present(v) {
		args = append(args, "%"+rb.ToS(v)+"%")
		where += " AND (life_rules.title ILIKE $" + itoa(len(args)) + ")"
	}
	var total int64
	must(db.Q().QueryRow(c.Ctx, "SELECT COUNT(life_rules.id) FROM life_rules WHERE "+where, args...).Scan(&total))
	limit := min(max(paramToI(c.Param("limit"), 20), 1), 100)
	offset := max(paramToI(c.Param("offset"), 0), 0)
	n := len(args)
	rules := liferules.Load(c.Ctx, db.Q(), where, "ORDER BY life_rules.created_at ASC LIMIT $"+itoa(n+1)+" OFFSET $"+itoa(n+2),
		append(args, limit, offset)...)
	liferules.LoadSteps(c.Ctx, db.Q(), rules)
	out := []any{}
	for _, r := range rules {
		var name any
		var nm *string
		err := db.Q().QueryRow(c.Ctx, `SELECT name FROM users WHERE id = $1`, r.Get("user_id")).Scan(&nm)
		if err != nil && !db.NoRows(err) {
			panic(err)
		}
		name = rb.Deref(nm)
		var pending any
		if r.Get("approved") != true {
			pending = timeOrNil(timePtr(r.Get("created_at")))
		}
		steps := []any{}
		for _, s := range r.Steps {
			steps = append(steps, rb.M("id", s.ID, "order", s.Get("order"), "title", s.Get("title"), "description", s.Get("description")))
		}
		out = append(out, rb.M("id", r.ID, "icon", r.Get("icon"), "locale", r.Get("locale"), "title", r.Get("title"),
			"description", r.Get("description"), "is_public", r.Get("is_public"), "approved", r.Get("approved"),
			"adoption_count", r.Get("adoption_count"), "created_at", timeOrNil(timePtr(r.Get("created_at"))),
			"updated_at", timeOrNil(timePtr(r.Get("updated_at"))), "pending_since", pending,
			"owner", rb.M("id", r.Get("user_id"), "name", name), "steps", steps))
	}
	c.JSON(200, rb.M("life_rules", out, "pagination", rb.M("total", total, "limit", limit, "offset", offset, "count", len(rules))))
}

func timePtr(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}
