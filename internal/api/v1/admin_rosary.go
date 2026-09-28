package v1

import (
	"errors"
	"slices"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rosary"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var adminRosarySort = map[string]string{
	"created_at": "custom_rosary_prayers.created_at", "updated_at": "custom_rosary_prayers.updated_at",
	"title": "custom_rosary_prayers.title", "author": "users.name", "locale": "custom_rosary_prayers.locale",
	"status": "custom_rosary_prayers.share_status", "publication_status": "custom_rosary_prayers.publication_status",
	"cycle_repeat": "custom_rosary_prayers.cycle_repeat", "is_public": "custom_rosary_prayers.is_public",
	"strapi_slug": "custom_rosary_prayers.strapi_slug", "reviewed_at": "custom_rosary_prayers.reviewed_at",
	"reentries": "custom_rosary_prayers.moderation_reentry_count",
}

// rescueAdminRosary ports rescue_from RecordNotFound and
// CategorySelection::Invalid.
func rescueAdminRosary(c *web.Context) {
	rec := recover()
	if rec == nil {
		return
	}
	switch e := rec.(type) {
	case *rosary.NotFound:
		c.JSON(404, rb.M("error", "not_found"))
		return
	case *rosary.InvalidCategory:
		c.JSON(422, rb.M("error", e.Message, "errors", e.ErrorsHash()))
		return
	}
	panic(rec)
}

func adminRosaryMust(err error) {
	var cat *rosary.InvalidCategory
	if errors.As(err, &cat) {
		panic(cat)
	}
	rosaryMust(err)
}

func rosaryAuthor(c *web.Context, userID int64) rosary.Author {
	var a rosary.Author
	var id int64
	var name *string
	var email string
	err := db.Q().QueryRow(c.Ctx, `SELECT id, name, email FROM users WHERE id = $1`, userID).Scan(&id, &name, &email)
	if db.NoRows(err) {
		return a
	}
	must(err)
	a.ID, a.Name, a.Email = &id, rb.Deref(name), email
	return a
}

func moderationJSON(c *web.Context, p *rosary.Prayer, expanded bool) *rb.Map {
	m, err := p.ModerationJSON(rosaryAuthor(c, p.UserID()), expanded)
	adminRosaryMust(err)
	return m
}

// sanitizeSQLLike ports sanitize_sql_like.
func sanitizeSQLLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// AdminCustomRosaryPrayersIndex ports Admin::CustomRosaryPrayersController#index.
func AdminCustomRosaryPrayersIndex(c *web.Context) {
	defer rescueAdminRosary(c)
	auth.AuthenticateAdmin(c)
	where := "custom_rosary_prayers.is_public = TRUE"
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + itoa(len(args)) }
	if v := c.Param("share_status"); rb.Present(v) {
		switch x := v.(type) {
		case []any:
			var marks []string
			for _, s := range x {
				marks = append(marks, arg(rb.ToS(s)))
			}
			where += " AND custom_rosary_prayers.share_status IN (" + strings.Join(marks, ", ") + ")"
		default:
			where += " AND custom_rosary_prayers.share_status = " + arg(rb.ToS(x))
		}
	}
	joinUsers := false
	if q := strings.TrimSpace(rb.ToS(c.Param("search"))); q != "" {
		joinUsers = true
		m := arg("%" + sanitizeSQLLike(q) + "%")
		where += " AND (custom_rosary_prayers.title ILIKE " + m + " OR custom_rosary_prayers.description ILIKE " + m +
			" OR custom_rosary_prayers.locale ILIKE " + m + " OR custom_rosary_prayers.strapi_slug ILIKE " + m +
			" OR users.name ILIKE " + m + " OR users.email ILIKE " + m + ")"
	}
	column, ok := adminRosarySort[rb.ToS(c.Param("sort"))]
	if !ok {
		column = adminRosarySort["created_at"]
	}
	if column == "users.name" {
		joinUsers = true
	}
	direction := "DESC"
	if strings.EqualFold(rb.ToS(c.Param("direction")), "asc") {
		direction = "ASC"
	}
	from := "custom_rosary_prayers"
	if joinUsers {
		from += " LEFT OUTER JOIN users ON users.id = custom_rosary_prayers.user_id"
	}
	var total int64
	must(db.Q().QueryRow(c.Ctx, "SELECT COUNT(custom_rosary_prayers.id) FROM "+from+" WHERE "+where, args...).Scan(&total))
	limit := min(max(paramToI(c.Param("limit"), 20), 1), 100)
	offset := max(paramToI(c.Param("offset"), 0), 0)
	idsSQL := "SELECT custom_rosary_prayers.id FROM " + from + " WHERE " + where + " ORDER BY " + column + " " + direction +
		" NULLS LAST, custom_rosary_prayers.id " + direction + " LIMIT " + arg(limit) + " OFFSET " + arg(offset)
	rows, err := db.Q().Query(c.Ctx, idsSQL, args...)
	must(err)
	var ids []int64
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	must(rows.Err())
	list, err := rosary.LoadWithStructure(c.Ctx, db.Q(), "custom_rosary_prayers.id = ANY($1)", "", ids)
	must(err)
	slices.SortFunc(list, func(a, b *rosary.Prayer) int {
		return slices.Index(ids, a.ID) - slices.Index(ids, b.ID)
	})
	out := []any{}
	for _, p := range list {
		out = append(out, moderationJSON(c, p, false))
	}
	c.JSON(200, rb.M("custom_rosary_prayers", out, "pagination", rb.M("total", total, "limit", limit, "offset", offset,
		"count", len(list), "has_next", offset+int64(len(list)) < total)))
}

func adminSetRosary(c *web.Context) *rosary.Prayer {
	return setRosaryPrayer(c)
}

// AdminCustomRosaryPrayersShow ports #show.
func AdminCustomRosaryPrayersShow(c *web.Context) {
	defer rescueAdminRosary(c)
	auth.AuthenticateAdmin(c)
	p := adminSetRosary(c)
	c.JSON(200, rb.M("custom_rosary_prayer", moderationJSON(c, p, true)))
}

func reloadedModeration(c *web.Context, id int64) *rb.Map {
	p, err := rosary.Find(c.Ctx, db.Q(), id)
	must(err)
	if p == nil {
		panic(&rosary.NotFound{Message: "Couldn't find CustomRosaryPrayer"})
	}
	return moderationJSON(c, p, true)
}

// AdminCustomRosaryPrayersApprove ports #approve.
func AdminCustomRosaryPrayersApprove(c *web.Context) {
	defer rescueAdminRosary(c)
	auth.AuthenticateAdmin(c)
	p := adminSetRosary(c)
	adminRosaryMust(rosary.Approve(c.Ctx, p.ID, c.Param("category"), c.Param("strapi_slug")))
	c.JSON(200, rb.M("custom_rosary_prayer", reloadedModeration(c, p.ID)))
}

// AdminCustomRosaryPrayersReject ports #reject.
func AdminCustomRosaryPrayersReject(c *web.Context) {
	defer rescueAdminRosary(c)
	auth.AuthenticateAdmin(c)
	p := adminSetRosary(c)
	adminRosaryMust(rosary.Reject(c.Ctx, p.ID, c.Param("reason")))
	c.JSON(200, rb.M("custom_rosary_prayer", reloadedModeration(c, p.ID)))
}

// AdminRosaryCategoriesIndex ports Admin::RosaryCategoriesController#index.
func AdminRosaryCategoriesIndex(c *web.Context) {
	auth.AuthenticateAdmin(c)
	locale := "pt-BR"
	if v := c.Param("locale"); rb.Present(v) {
		locale = rb.ToS(v)
	}
	if !slices.Contains(rosary.Locales, locale) {
		c.JSON(422, rb.M("error", "unsupported locale"))
		return
	}
	list, err := rosary.Categories(c.Ctx, locale)
	var se *rosary.StrapiError
	if errors.As(err, &se) {
		panic(web.NewInfraError("Integrations::Strapi::Client::Error", se.Message, se.Code))
	}
	must(err)
	c.JSON(200, rb.M("rosary_categories", list))
}
