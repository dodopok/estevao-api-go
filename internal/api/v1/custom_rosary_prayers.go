package v1

import (
	"errors"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rosary"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// rescueRosary ports the controller's rescue_from handlers (RecordNotFound,
// RecordInvalid, ParameterMissing).
func rescueRosary(c *web.Context) {
	rec := recover()
	if rec == nil {
		return
	}
	switch e := rec.(type) {
	case *rosary.NotFound:
		c.JSON(404, rb.M("error", "not_found"))
		return
	case *ar.RecordInvalid:
		c.JSON(422, rb.M("errors", e.Errors.Hash()))
		return
	case *rosary.ParameterMissing:
		c.JSON(422, rb.M("errors", rb.M(e.Param, []string{"is missing"})))
		return
	}
	panic(rec)
}

func rosaryMust(err error) {
	if err == nil {
		return
	}
	var invalid *ar.RecordInvalid
	if errors.As(err, &invalid) {
		panic(invalid)
	}
	pgMust(err)
}

// setRosaryPrayer ports set_prayer (looked up outside the owner scope).
func setRosaryPrayer(c *web.Context) *rosary.Prayer {
	var p *rosary.Prayer
	if id, ok := findID(c.Param("id")); ok {
		var err error
		p, err = rosary.Find(c.Ctx, db.Q(), id)
		must(err)
	}
	if p == nil {
		panic(&rosary.NotFound{Message: "Couldn't find CustomRosaryPrayer"})
	}
	return p
}

func rosaryForbidden() { web.Raise("AuthorizationError", "forbidden", "FORBIDDEN") }

func serializeRosary(c *web.Context, id int64) *rb.Map {
	p, err := rosary.Find(c.Ctx, db.Q(), id)
	must(err)
	if p == nil {
		panic(&rosary.NotFound{Message: "Couldn't find CustomRosaryPrayer"})
	}
	return p.JSON()
}

// paramToI ports `(param || default).to_i` of Paginatable.
func paramToI(v any, def int64) int64 {
	if v == nil || v == false {
		return def
	}
	switch x := v.(type) {
	case string:
		return int64(rb.StringToI(x))
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	}
	panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_i", v)})
}

// CustomRosaryPrayersIndex ports CustomRosaryPrayersController#index.
func CustomRosaryPrayersIndex(c *web.Context) {
	defer rescueRosary(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	paginated := c.ParamPresent("limit") || c.ParamPresent("offset") || c.ParamPresent("updated_after")
	if !paginated {
		list, err := rosary.LoadWithStructure(c.Ctx, db.Q(), "custom_rosary_prayers.user_id = $1",
			"custom_rosary_prayers.created_at DESC", u.ID)
		must(err)
		c.JSON(200, rb.M("custom_rosary_prayers", rosaryList(list)))
		return
	}
	where := "custom_rosary_prayers.user_id = $1"
	order := "custom_rosary_prayers.created_at DESC"
	args := []any{u.ID}
	if v := c.Param("updated_after"); rb.Present(v) {
		if t, err := rb.TimeXMLSchema(rb.ToS(v), time.Local); err == nil {
			args = append(args, t.UTC().Truncate(time.Microsecond))
			where += " AND (custom_rosary_prayers.updated_at >= $2)"
			order = "custom_rosary_prayers.updated_at ASC, custom_rosary_prayers.id ASC"
		}
	}
	var total int64
	must(db.Q().QueryRow(c.Ctx, "SELECT COUNT(custom_rosary_prayers.id) FROM custom_rosary_prayers WHERE "+where, args...).Scan(&total))
	limit := min(max(paramToI(c.Param("limit"), 20), 1), 100)
	offset := max(paramToI(c.Param("offset"), 0), 0)
	n := len(args)
	args = append(args, limit, offset)
	list, err := rosary.LoadWithStructure(c.Ctx, db.Q(), where,
		order+" LIMIT $"+itoa(n+1)+" OFFSET $"+itoa(n+2), args...)
	must(err)
	c.JSON(200, rb.M("custom_rosary_prayers", rosaryList(list), "pagination", rb.M(
		"total", total, "limit", limit, "offset", offset, "count", len(list), "has_next", offset+int64(len(list)) < total)))
}

func rosaryList(list []*rosary.Prayer) []any {
	out := []any{}
	for _, p := range list {
		out = append(out, p.JSON())
	}
	return out
}

func findExistingRosary(c *web.Context, u *users.User, clientID any) *rosary.Prayer {
	if rb.Blank(clientID) {
		return nil
	}
	cid := castString(clientID)
	list, err := rosary.LoadWithStructure(c.Ctx, db.Q(),
		"custom_rosary_prayers.user_id = $1 AND custom_rosary_prayers.client_id = $2 LIMIT 1", "", u.ID, *cid)
	must(err)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// CustomRosaryPrayersCreate ports #create (CustomRosaryPrayers::Create).
func CustomRosaryPrayersCreate(c *web.Context) {
	defer rescueRosary(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	attrs := rosary.Permit(c.Params())
	apply := func(p *rosary.Prayer) error {
		p.ApplyStructure(attrs)
		return p.Save(c.Ctx)
	}
	clientID := attrs.Get("client_id")
	created := false
	p := findExistingRosary(c, u, clientID)
	if p == nil {
		p, created = rosary.NewPrayer(u.ID), true
	}
	err := apply(p)
	var invalid *ar.RecordInvalid
	if errors.As(err, &invalid) && created && invalid.Errors.Has("client_id", "has already been taken") {
		existing := findExistingRosary(c, u, clientID)
		if existing == nil {
			rosaryMust(err)
		}
		p, created = existing, false
		err = apply(p)
	}
	rosaryMust(err)
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, rb.M("custom_rosary_prayer", serializeRosary(c, p.ID)))
}

// CustomRosaryPrayersUpdate ports #update (CustomRosaryPrayers::Update).
func CustomRosaryPrayersUpdate(c *web.Context) {
	defer rescueRosary(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	p := setRosaryPrayer(c)
	attrs := rosary.Permit(c.Params())
	if p.UserID() != u.ID {
		rosaryForbidden()
	}
	p.ApplyStructure(attrs)
	rosaryMust(p.Save(c.Ctx))
	c.JSON(200, rb.M("custom_rosary_prayer", serializeRosary(c, p.ID)))
}

// CustomRosaryPrayersDestroy ports #destroy (CustomRosaryPrayers::Destroy).
func CustomRosaryPrayersDestroy(c *web.Context) {
	defer rescueRosary(c)
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	p := setRosaryPrayer(c)
	if p.UserID() != u.ID {
		rosaryForbidden()
	}
	must(p.Destroy(c.Ctx))
	c.HeadStatus(204) // head :no_content sets no Content-Type

}
