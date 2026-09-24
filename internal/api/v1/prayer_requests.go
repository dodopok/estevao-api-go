package v1

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/perplexity"
	"github.com/dodopok/estevao-api-go/internal/prayerrequests"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// prayerRequestsFilters runs authenticate_user! and the controller's own
// require_premium!, inside rescue_from ArgumentError.
func prayerRequestsFilters(c *web.Context) *users.User {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	if u == nil || !u.Premium() {
		c.RenderJSON(403, rb.M("error", "Premium subscription required"))
	}
	return u
}

func todayYMD(u *users.User) rb.YMD {
	y, m, d := userToday(u, time.Now())
	return rb.YMD{Y: int64(y), M: int64(m), D: int64(d)}
}

// resolveWeekStart ports resolve_week_start.
func resolveWeekStart(c *web.Context, u *users.User) rb.YMD {
	if v := c.Param("week_start"); rb.Present(v) {
		return prayerrequests.WeekStartFor(dateParseParam(v, "Invalid week_start date format"))
	}
	return prayerrequests.WeekStartFor(todayYMD(u))
}

// resolveWeekStartFromParams ports resolve_week_start_from_params.
func resolveWeekStartFromParams(c *web.Context, u *users.User) rb.YMD {
	nested := paramsDig(c, "prayer_request", "week_start")
	if rb.Present(nested) || rb.Present(c.Param("week_start")) {
		v := nested
		if v == nil || v == false {
			v = c.Param("week_start")
		}
		return prayerrequests.WeekStartFor(dateParseParam(v, "Invalid week_start date format"))
	}
	return prayerrequests.WeekStartFor(todayYMD(u))
}

func prayerRequestParams(c *web.Context) *rb.Map {
	return requirePermit(c, "prayer_request", "title", "content", "position")
}

func prayerRequestsJSON(list []*prayerrequests.Request) []any {
	out := make([]any, len(list))
	for i, r := range list {
		out[i] = r.JSON()
	}
	return out
}

// setPrayerRequest ports set_prayer_request.
func setPrayerRequest(c *web.Context, u *users.User) *prayerrequests.Request {
	var r *prayerrequests.Request
	if id, ok := findID(c.Param("id")); ok {
		r = prayerrequests.Find(c.Ctx, u.ID, id)
	}
	if r == nil {
		c.RenderJSON(404, rb.M("error", "Prayer request not found"))
	}
	return r
}

// PrayerRequestsIndex ports PrayerRequestsController#index.
func PrayerRequestsIndex(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	week := resolveWeekStart(c, u)
	list := prayerrequests.ForWeek(c.Ctx, u.ID, week)
	c.JSON(200, rb.M("week_start", week.ISO(), "week_end", week.AddDays(6).ISO(), "count", len(list),
		"prayer_requests", prayerRequestsJSON(list)))
}

// PrayerRequestsCreate ports PrayerRequestsController#create.
func PrayerRequestsCreate(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	week := resolveWeekStartFromParams(c, u)
	r, msgs := prayerrequests.Create(c.Ctx, u, prayerRequestParams(c), week)
	if msgs != nil {
		c.JSON(422, rb.M("error", strings.Join(msgs, ", ")))
		return
	}
	c.JSON(201, rb.M("message", "Prayer request created successfully", "prayer_request", r.JSON()))
}

// PrayerRequestsUpdate ports PrayerRequestsController#update.
func PrayerRequestsUpdate(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	r := setPrayerRequest(c, u)
	if msgs := prayerrequests.Update(c.Ctx, r, prayerRequestParams(c)); msgs != nil {
		c.JSON(422, rb.M("error", strings.Join(msgs, ", ")))
		return
	}
	c.JSON(200, rb.M("message", "Prayer request updated successfully", "prayer_request", r.JSON()))
}

// PrayerRequestsDestroy ports PrayerRequestsController#destroy.
func PrayerRequestsDestroy(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	r := setPrayerRequest(c, u)
	prayerrequests.Destroy(c.Ctx, r)
	c.JSON(200, rb.M("message", "Prayer request deleted successfully"))
}

// PrayerRequestsCopyPreviousWeek ports #copy_previous_week.
func PrayerRequestsCopyPreviousWeek(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	week := resolveWeekStart(c, u)
	copied, err := prayerrequests.CopyPreviousWeek(c.Ctx, u, week)
	var ce *prayerrequests.CopyError
	if errors.As(err, &ce) {
		status := 422
		if ce.NotFound {
			status = 404
		}
		c.JSON(status, rb.M("error", ce.Message))
		return
	}
	must(err)
	c.JSON(201, rb.M("message", strconv.Itoa(len(copied))+" prayer requests copied from previous week",
		"week_start", week.ISO(), "week_end", week.AddDays(6).ISO(), "copied_count", len(copied),
		"prayer_requests", prayerRequestsJSON(copied)))
}

// PrayerRequestsWeeklyPrayer ports #weekly_prayer.
func PrayerRequestsWeeklyPrayer(c *web.Context) {
	defer rescueArgument(c)
	u := prayerRequestsFilters(c)
	week := resolveWeekStart(c, u)
	code := c.Param("prayer_book_code")
	if code == nil || code == false {
		code = nil
		if u.Preferences != nil {
			code = u.Preferences.Get("prayer_book_code")
		}
		if code == nil || code == false {
			code = books.DefaultCode
		}
	}
	g := &prayerrequests.Generator{User: u, WeekStart: week, PrayerBookCode: rb.ToS(code)}
	result, err := g.Call(c.Ctx)
	var apiErr *perplexity.APIError
	if errors.As(err, &apiErr) {
		c.JSON(503, rb.M("error", "Failed to generate prayer. Please try again later."))
		return
	}
	must(err)
	if result == nil {
		c.JSON(200, rb.M("week_start", week.ISO(), "week_end", week.AddDays(6).ISO(), "prayer_requests_count", 0,
			"module", nil, "placement", nil))
		return
	}
	c.JSON(200, result)
}
