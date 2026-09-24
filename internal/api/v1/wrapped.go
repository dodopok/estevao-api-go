package v1

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
	"github.com/dodopok/estevao-api-go/internal/wrapped"
)

var fourDigits = regexp.MustCompile(`^\d{4}$`)

// UsersWrappedShow ports Users::WrappedController#show.
func UsersWrappedShow(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	fail := func(code string, status int) { c.JSON(status, rb.M("error", rb.M("code", code))) }
	raw := rb.ToS(c.Param("year"))
	now := time.Now()
	if !fourDigits.MatchString(raw) {
		fail("INVALID_YEAR", 400)
		return
	}
	year, _ := strconv.Atoi(raw)
	if year > now.In(rb.AppZone).Year() || year == 0 {
		fail("INVALID_YEAR", 400)
		return
	}
	if !features.RolloutEnabled(c.Ctx, "yearly_wrapped", u, false) {
		fail("WRAPPED_DISABLED", 403)
		return
	}
	snap, err := wrapped.Snapshot(c.Ctx, u, year, c.Param("locale"), now)
	if errors.Is(err, wrapped.ErrNotAvailable) {
		fail("WRAPPED_NOT_AVAILABLE", 404)
		return
	}
	must(err)
	c.JSON(200, rb.M("data", snap))
}
