package v1

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var apiKeySchema = &ar.Schema{
	Table: "api_keys",
	Columns: []string{"active", "billing_active", "contact_email", "created_at", "developer_id", "expires_at", "key",
		"last_used_at", "name", "rate_limit_multiplier", "requests_count", "updated_at"},
	Types: map[string]ar.ColType{"active": ar.Boolean, "billing_active": ar.Boolean, "contact_email": ar.String,
		"created_at": ar.Datetime, "developer_id": ar.Integer, "expires_at": ar.Datetime, "key": ar.String,
		"last_used_at": ar.Datetime, "name": ar.String, "rate_limit_multiplier": ar.Integer, "requests_count": ar.Integer,
		"updated_at": ar.Datetime},
	Defaults: map[string]any{"active": true, "billing_active": true, "rate_limit_multiplier": int64(1), "requests_count": int64(0)},
}

// mailToEmail ports URI::MailTo::EMAIL_REGEXP.
var mailToEmail = regexp.MustCompile(`\A[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" +
	`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\z`)

func loadAPIKeys(ctx context.Context, where, suffix string, args ...any) []*ar.Record {
	rows, err := db.Q().Query(ctx, "SELECT "+apiKeySchema.SelectList("api_keys")+" FROM api_keys WHERE "+where+" "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*ar.Record
	for rows.Next() {
		r, err := ar.ScanRecord(apiKeySchema, rows)
		must(err)
		out = append(out, r)
	}
	must(rows.Err())
	return out
}

// apiKeyPreview ports ApiKey#key_preview.
func apiKeyPreview(key any) any {
	s, _ := key.(string)
	if rb.Blank(s) {
		return nil
	}
	r := []rune(s)
	first, last := r, r
	if len(r) > 16 {
		first = r[:16]
	}
	if len(r) > 4 {
		last = r[len(r)-4:]
	}
	return string(first) + "..." + string(last)
}

func apiKeyExpired(k *ar.Record) bool {
	t, ok := k.Get("expires_at").(time.Time)
	return ok && t.Before(clock.Now())
}

func timeValue(v any) any {
	if t, ok := v.(time.Time); ok {
		return rb.FormatTime(t)
	}
	return nil
}

// serializeAPIKey ports serialize_api_key.
func serializeAPIKey(k *ar.Record) *rb.Map {
	expired := apiKeyExpired(k)
	return rb.M("id", k.ID, "name", k.Get("name"), "key_preview", apiKeyPreview(k.Get("key")),
		"contact_email", k.Get("contact_email"), "active", k.Get("active"), "billing_active", k.Get("billing_active"),
		"rate_limit_multiplier", k.Get("rate_limit_multiplier"), "requests_count", k.Get("requests_count"),
		"last_used_at", timeValue(k.Get("last_used_at")), "expires_at", timeValue(k.Get("expires_at")),
		"expired", expired, "usable", k.Get("active") == true && k.Get("billing_active") == true && !expired,
		"created_at", timeValue(k.Get("created_at")), "updated_at", timeValue(k.Get("updated_at")))
}

// validateAPIKey ports ApiKey's validations (the key is generated after
// them, so its uniqueness check sees nil on create).
func validateAPIKey(ctx context.Context, k *ar.Record) *ar.Errors {
	e := &ar.Errors{}
	if ar.Blank(k.Get("name")) {
		e.Add("name", "can't be blank")
	}
	if key := k.Get("key"); key != nil {
		sql := `SELECT EXISTS(SELECT 1 FROM api_keys WHERE key = $1`
		args := []any{key}
		if !k.NewRecord {
			sql += ` AND id <> $2`
			args = append(args, k.ID)
		}
		var taken bool
		must(db.Q().QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("key", "has already been taken")
		}
	}
	email := k.Get("contact_email")
	if ar.Blank(email) {
		e.Add("contact_email", "can't be blank")
	}
	if !mailToEmail.MatchString(rb.ToS(email)) {
		e.Add("contact_email", "is invalid")
	}
	ar.Numeric(e, k, "rate_limit_multiplier", rb.GTE(1), rb.LTE(1000))
	return e
}

// requireAPIKeyParams ports params.require(:api_key).permit(fields...).
func requireAPIKeyParams(c *web.Context, fields ...string) *rb.Map {
	v, ok := c.Params().Lookup("api_key")
	if !ok || !(rb.Present(v) || v == false) {
		panic(&web.StandardError{Class: "ActionController::ParameterMissing",
			Message: "param is missing or the value is empty or invalid: api_key"})
	}
	m, isMap := v.(*rb.Map)
	if !isMap {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("permit", v)})
	}
	out := rb.NewMap()
	for _, f := range fields {
		if x, ok := m.Lookup(f); ok && web.PermittedScalar(x) {
			out.Set(f, x)
		}
	}
	return out
}

// setAPIKey ports set_api_key.
func setAPIKey(c *web.Context) *ar.Record {
	if id, ok := findID(c.Param("id")); ok {
		if found := loadAPIKeys(c.Ctx, "api_keys.id = $1", "LIMIT 1", id); len(found) > 0 {
			return found[0]
		}
	}
	c.RenderJSON(404, rb.M("error", "API key not found"))
	return nil
}

// AdminAPIKeysIndex ports Admin::ApiKeysController#index.
func AdminAPIKeysIndex(c *web.Context) {
	auth.AuthenticateAdmin(c)
	out := []any{}
	for _, k := range loadAPIKeys(c.Ctx, "TRUE", "ORDER BY api_keys.created_at DESC") {
		out = append(out, serializeAPIKey(k))
	}
	c.JSON(200, out)
}

// AdminAPIKeysShow ports #show.
func AdminAPIKeysShow(c *web.Context) {
	auth.AuthenticateAdmin(c)
	c.JSON(200, serializeAPIKey(setAPIKey(c)))
}

// AdminAPIKeysCreate ports #create.
func AdminAPIKeysCreate(c *web.Context) {
	auth.AuthenticateAdmin(c)
	attrs := requireAPIKeyParams(c, "name", "contact_email", "expires_at")
	k := ar.NewRecord(apiKeySchema)
	attrs.Each(func(f string, v any) { k.Assign(f, v) })
	if errs := validateAPIKey(c.Ctx, k); errs.Any() {
		c.JSON(422, rb.M("errors", errs.FullMessages()))
		return
	}
	if rb.Blank(k.Get("key")) {
		var b [24]byte
		_, _ = rand.Read(b[:])
		k.Set("key", "estevao_"+hex.EncodeToString(b[:]))
	}
	pgMust(k.Insert(c.Ctx, db.Q(), users.Now()))
	auth.ClearRateLimitMultiplierCache()
	c.JSON(201, serializeAPIKey(k).Merge(rb.M("full_key", k.Get("key"))))
}

// AdminAPIKeysUpdate ports #update.
func AdminAPIKeysUpdate(c *web.Context) {
	auth.AuthenticateAdmin(c)
	k := setAPIKey(c)
	attrs := requireAPIKeyParams(c, "name", "contact_email", "active", "expires_at")
	attrs.Each(func(f string, v any) { k.Assign(f, v) })
	if errs := validateAPIKey(c.Ctx, k); errs.Any() {
		c.JSON(422, rb.M("errors", errs.FullMessages()))
		return
	}
	_, err := k.Update(c.Ctx, db.Q(), users.Now())
	pgMust(err)
	auth.ClearRateLimitMultiplierCache()
	c.JSON(200, serializeAPIKey(k))
}

// AdminAPIKeysDestroy ports #destroy (usage logs are destroyed with it).
func AdminAPIKeysDestroy(c *web.Context) {
	auth.AuthenticateAdmin(c)
	k := setAPIKey(c)
	pgMust(db.Transaction(c.Ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM api_key_usage_logs WHERE api_key_id = $1`, k.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, k.ID)
		return err
	}))
	auth.ClearRateLimitMultiplierCache()
	c.JSON(200, rb.M("message", "API key deleted"))
}

// usageDate ports parse_date (Date.parse, nil when blank or invalid).
func usageDate(v any) *civil.Date {
	if rb.Blank(v) {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of " + rb.ClassName(v) + " into String"})
	}
	d, err := rb.DateParse(s, true)
	if err != nil {
		return nil
	}
	cd, ok := d.Civil()
	if !ok {
		return nil
	}
	return &cd
}

// AdminAPIKeysUsage ports #usage.
func AdminAPIKeysUsage(c *web.Context) {
	auth.AuthenticateAdmin(c)
	renderAPIKeyUsage(c, setAPIKey(c))
}

// renderAPIKeyUsage ports the usage action shared by the admin and the
// developer API key controllers.
func renderAPIKeyUsage(c *web.Context, k *ar.Record) {
	today := civil.FromTime(clock.Now().In(rb.AppZone))
	start := civil.FromTime(clock.Now().In(rb.AppZone).Add(-30 * 24 * time.Hour))
	end := today
	if d := usageDate(c.Param("start_date")); d != nil {
		start = *d
	}
	if d := usageDate(c.Param("end_date")); d != nil {
		end = *d
	}
	daily := rb.NewMap()
	rows, err := db.Q().Query(c.Ctx, `SELECT SUM(requests_count) AS sum_requests_count, date AS api_key_usage_logs_date
		FROM api_key_usage_logs WHERE api_key_usage_logs.api_key_id = $1 AND api_key_usage_logs.date BETWEEN $2 AND $3
		GROUP BY api_key_usage_logs.date ORDER BY api_key_usage_logs.date ASC`, k.ID, start.ISO(), end.ISO())
	must(err)
	for rows.Next() {
		var n int64
		var d time.Time
		must(rows.Scan(&n, &d))
		daily.Set(d.Format("2006-01-02"), n)
	}
	rows.Close()
	must(rows.Err())
	endpoints := rb.NewMap()
	rows, err = db.Q().Query(c.Ctx, `SELECT SUM(requests_count) AS sum_requests_count, endpoint AS api_key_usage_logs_endpoint
		FROM api_key_usage_logs WHERE api_key_usage_logs.api_key_id = $1 AND api_key_usage_logs.date BETWEEN $2 AND $3
		GROUP BY api_key_usage_logs.endpoint ORDER BY sum_requests_count DESC`, k.ID, start.ISO(), end.ISO())
	must(err)
	for rows.Next() {
		var n int64
		var e string
		must(rows.Scan(&n, &e))
		endpoints.Set(e, n)
	}
	rows.Close()
	must(rows.Err())
	c.JSON(200, rb.M(
		"api_key", rb.M("id", k.ID, "name", k.Get("name"), "key_preview", apiKeyPreview(k.Get("key")),
			"total_requests", k.Get("requests_count"), "last_used_at", timeValue(k.Get("last_used_at"))),
		"period", rb.M("start_date", start.ISO(), "end_date", end.ISO()),
		"daily_totals", daily, "endpoints", endpoints))
}
