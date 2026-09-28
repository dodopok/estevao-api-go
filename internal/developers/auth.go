package developers

import (
	"context"
	"regexp"

	"github.com/golang-jwt/jwt/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var bearer = regexp.MustCompile(`(?i)\ABearer\s+(\S+)\z`)

// Authenticate ports DeveloperAuthenticatable#authenticate_developer!: it
// renders the 401 and halts when there is no developer.
func Authenticate(c *web.Context) *Developer {
	var token string
	if m := bearer.FindStringSubmatch(c.HeaderValue("Authorization")); m != nil {
		token = m[1]
	}
	if token == "" {
		c.RenderJSON(401, rb.M("error", "Unauthorized", "code", "AUTHENTICATION_REQUIRED", "request_id", c.RequestID))
		return nil
	}
	claims, err := auth.VerifyToken(token, true)
	if err != nil {
		c.RenderJSON(401, rb.M("error", "Authentication failed", "code", "AUTHENTICATION_FAILED", "request_id", c.RequestID))
		return nil
	}
	d := findOrCreate(c.Ctx, claims)
	if d == nil {
		c.RenderJSON(401, rb.M("error", "Unauthorized", "code", "AUTHENTICATION_REQUIRED", "request_id", c.RequestID))
	}
	return d
}

func claim(c jwt.MapClaims, k string) any { return c[k] }

// findOrCreate ports DeveloperAuthService.find_or_create_developer. An
// update that fails validation leaves the developer with the assigned (not
// saved) values, as `update` does.
func findOrCreate(ctx context.Context, claims jwt.MapClaims) *Developer {
	sub := claim(claims, "sub")
	email := claim(claims, "email")
	d := findDeveloper(ctx, db.Q(), "developers.provider_uid = $1", rb.ToS(sub))
	if d == nil && rb.Present(email) && ar.Truthy(claim(claims, "email_verified")) {
		d = findDeveloper(ctx, db.Q(), "developers.email = $1", rb.ToS(email))
	}
	if d != nil {
		d.Assign("provider_uid", sub)
		d.Assign("email", email)
		if name := claim(claims, "name"); ar.Blank(d.Get("name")) && rb.Present(name) {
			d.Assign("name", name)
		}
		_, err := d.Save(ctx)
		pgMust(err)
		return d
	}
	d = &Developer{ar.NewRecord(developerSchema)}
	d.Assign("provider_uid", sub)
	if email != nil && email != false {
		d.Assign("email", email)
	} else {
		d.Assign("email", rb.ToS(sub)+"@developer.portal")
	}
	d.Assign("name", claim(claims, "name"))
	errs, err := d.Save(ctx)
	if errs != nil {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + joinMessages(errs)})
	}
	pgMust(err)
	return d
}

func joinMessages(e *ar.Errors) string {
	out := ""
	for i, m := range e.FullMessages() {
		if i > 0 {
			out += ", "
		}
		out += m
	}
	return out
}

func pgMust(err error) {
	if err == nil {
		return
	}
	if class, msg, ok := db.RubyError(err); ok {
		panic(&web.StandardError{Class: class, Message: msg})
	}
	panic(err)
}
