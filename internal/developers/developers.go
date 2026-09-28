// Package developers ports the developer portal: Developer and its API
// keys, the billing entitlement projected onto them
// (Billing::Entitlement / ApplyEntitlement), Stripe Checkout and Billing
// Portal sessions, and the Stripe webhook.
package developers

import (
	"context"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/billing"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var developerSchema = &ar.Schema{
	Table: "developers",
	Columns: []string{"approved", "company_name", "created_at", "email", "legacy_free", "max_keys", "name", "provider_uid",
		"stripe_customer_id", "updated_at", "use_case_description", "website"},
	Types: map[string]ar.ColType{"approved": ar.Boolean, "company_name": ar.String, "created_at": ar.Datetime,
		"email": ar.String, "legacy_free": ar.Boolean, "max_keys": ar.Integer, "name": ar.String, "provider_uid": ar.String,
		"stripe_customer_id": ar.String, "updated_at": ar.Datetime, "use_case_description": ar.String, "website": ar.String},
	Defaults: map[string]any{"approved": true, "legacy_free": false, "max_keys": int64(3)},
}

// Developer is a Developer row.
type Developer struct{ *ar.Record }

// mailToEmail ports URI::MailTo::EMAIL_REGEXP.
var mailToEmail = regexp.MustCompile(`\A[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" +
	`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\z`)

// EmailValid ports the URI::MailTo::EMAIL_REGEXP format check on to_s.
func EmailValid(v any) bool { return mailToEmail.MatchString(rb.ToS(v)) }

// LoadDevelopers runs `SELECT developers.* ... WHERE where suffix`.
func LoadDevelopers(ctx context.Context, q db.Querier, where, suffix string, args ...any) []*Developer {
	rows, err := q.Query(ctx, "SELECT "+developerSchema.SelectList("developers")+" FROM developers WHERE "+where+" "+suffix, args...)
	must(err)
	defer rows.Close()
	var out []*Developer
	for rows.Next() {
		r, err := ar.ScanRecord(developerSchema, rows)
		must(err)
		out = append(out, &Developer{r})
	}
	must(rows.Err())
	return out
}

func findDeveloper(ctx context.Context, q db.Querier, where string, args ...any) *Developer {
	list := LoadDevelopers(ctx, q, where, "LIMIT 1", args...)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// Validate ports Developer's validations.
func (d *Developer) Validate(ctx context.Context) *ar.Errors {
	e := &ar.Errors{}
	uniq := func(col string) bool {
		sql := `SELECT EXISTS(SELECT 1 FROM developers WHERE ` + col + ` = $1`
		args := []any{d.Get(col)}
		if !d.NewRecord {
			sql += ` AND id <> $2`
			args = append(args, d.ID)
		}
		var taken bool
		must(db.Conn(ctx).QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		return taken
	}
	if ar.Blank(d.Get("provider_uid")) {
		e.Add("provider_uid", "can't be blank")
	}
	if d.Get("provider_uid") != nil && uniq("provider_uid") {
		e.Add("provider_uid", "has already been taken")
	}
	if ar.Blank(d.Get("email")) {
		e.Add("email", "can't be blank")
	}
	if d.Get("email") != nil && uniq("email") {
		e.Add("email", "has already been taken")
	}
	if !EmailValid(d.Get("email")) {
		e.Add("email", "is invalid")
	}
	return e
}

// Save ports save (validations, then the insert or the changed columns);
// false when invalid. A change of legacy_free re-applies the entitlement
// after the commit, as the model's after_update_commit does.
func (d *Developer) Save(ctx context.Context) (*ar.Errors, error) {
	if errs := d.Validate(ctx); errs.Any() {
		return errs, nil
	}
	if d.NewRecord {
		return nil, d.Insert(ctx, db.Conn(ctx), users.Now())
	}
	legacyChanged := d.Changed("legacy_free")
	if _, err := d.Update(ctx, db.Conn(ctx), users.Now()); err != nil {
		return nil, err
	}
	if legacyChanged {
		ApplyEntitlement(ctx, d)
	}
	return nil, nil
}

// Subscription is a DeveloperSubscription row.
type Subscription struct {
	ID                                        int64
	StripeSubscriptionID, StripeCustomerID    string
	PlanCode, Status, Interval, Currency      string
	CancelAtPeriodEnd                         bool
	TrialEndsAt, CurrentPeriodEnd, CanceledAt *time.Time
	LastStripeEventCreatedAt                  *time.Time
}

// entitledStatuses ports DeveloperSubscription::ENTITLED_STATUSES.
var entitledStatuses = map[string]bool{"trialing": true, "active": true, "past_due": true}

// Entitled ports DeveloperSubscription#entitled?.
func (s *Subscription) Entitled() bool { return s != nil && entitledStatuses[s.Status] }

// SubscriptionOf loads the developer's subscription (has_one).
func SubscriptionOf(ctx context.Context, developerID int64) *Subscription {
	s := &Subscription{}
	err := db.Conn(ctx).QueryRow(ctx, `SELECT id, stripe_subscription_id, stripe_customer_id, plan_code, status, interval,
		currency, cancel_at_period_end, trial_ends_at, current_period_end, canceled_at, last_stripe_event_created_at
		FROM developer_subscriptions WHERE developer_id = $1 LIMIT 1`, developerID).
		Scan(&s.ID, &s.StripeSubscriptionID, &s.StripeCustomerID, &s.PlanCode, &s.Status, &s.Interval, &s.Currency,
			&s.CancelAtPeriodEnd, &s.TrialEndsAt, &s.CurrentPeriodEnd, &s.CanceledAt, &s.LastStripeEventCreatedAt)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return s
}

// Entitlement ports Billing::Entitlement.
type Entitlement struct {
	Source       string
	Plan         *billing.Plan
	Status       any
	Subscription *Subscription
}

// EntitlementFor ports Billing::Entitlement.for.
func EntitlementFor(ctx context.Context, d *Developer) *Entitlement {
	if d == nil {
		return &Entitlement{Source: "none"}
	}
	sub := SubscriptionOf(ctx, d.ID)
	if sub.Entitled() {
		if plan := billing.Find(sub.PlanCode); plan != nil {
			return &Entitlement{Source: "subscription", Plan: plan, Status: sub.Status, Subscription: sub}
		}
	}
	if d.Get("legacy_free") == true {
		plan := billing.Find(billing.LegacyPlanCode)
		if plan == nil {
			panic(&rb.RubyError{Class: "Billing::PlanCatalog::UnknownPlanError", Message: "Unknown plan: founder"})
		}
		return &Entitlement{Source: "legacy", Plan: plan, Status: "legacy"}
	}
	var status any
	if sub != nil {
		status = sub.Status
	}
	return &Entitlement{Source: "none", Status: status}
}

// Entitled ports Entitlement#entitled?.
func (e *Entitlement) Entitled() bool { return e.Plan != nil }

func (e *Entitlement) multiplier() int64 {
	if e.Plan == nil {
		return 1
	}
	return int64(e.Plan.Multiplier)
}

func (e *Entitlement) maxKeys() int64 {
	if e.Plan == nil {
		return 0
	}
	return int64(e.Plan.MaxKeys)
}

func timeValue(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}

// AsJSON ports Entitlement#as_json.
func (e *Entitlement) AsJSON() *rb.Map {
	var code, daily, minute any
	if e.Plan != nil {
		code, daily, minute = e.Plan.Code, e.Plan.DailyRequestLimit(), e.Plan.MinuteRequestLimit()
	}
	var trial, period any
	cancel := false
	if e.Subscription != nil {
		trial, period = timeValue(e.Subscription.TrialEndsAt), timeValue(e.Subscription.CurrentPeriodEnd)
		cancel = e.Subscription.CancelAtPeriodEnd
	}
	return rb.M("entitled", e.Entitled(), "source", e.Source, "status", e.Status, "plan_code", code,
		"multiplier", e.multiplier(), "max_keys", e.maxKeys(), "daily_request_limit", daily,
		"minute_request_limit", minute, "trialing", e.Status == "trialing", "trial_ends_at", trial,
		"current_period_end", period, "cancel_at_period_end", cancel)
}

// KeysCount ports developer.api_keys.count.
func KeysCount(ctx context.Context, d *Developer) int64 {
	var n int64
	must(db.Conn(ctx).QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE developer_id = $1`, d.ID).Scan(&n))
	return n
}

// CanCreateKey ports Developer#can_create_key?.
func CanCreateKey(ctx context.Context, d *Developer) bool {
	return d.Get("approved") == true && EntitlementFor(ctx, d).Entitled() && KeysCount(ctx, d) < d.Int("max_keys")
}

// KeysRemaining ports Developer#keys_remaining.
func KeysRemaining(ctx context.Context, d *Developer) int64 {
	return max(d.Int("max_keys")-KeysCount(ctx, d), 0)
}

// ApplyEntitlement ports Billing::ApplyEntitlement.call: max_keys follows a
// live plan, and each key (oldest first) gets the plan's multiplier and is
// switched on within the plan's allowance.
func ApplyEntitlement(ctx context.Context, d *Developer) *Entitlement {
	e := EntitlementFor(ctx, d)
	must(db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if e.Entitled() && d.Int("max_keys") != e.maxKeys() {
			d.Set("max_keys", e.maxKeys())
			if errs := d.Validate(ctx); errs.Any() {
				return &ar.RecordInvalid{Errors: errs}
			}
			if _, err := d.Update(ctx, tx, users.Now()); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT id, rate_limit_multiplier, billing_active FROM api_keys WHERE developer_id = $1
			ORDER BY created_at ASC, id ASC`, d.ID)
		if err != nil {
			return err
		}
		type key struct {
			id, mult int64
			active   bool
		}
		var keys []key
		for rows.Next() {
			var k key
			if err := rows.Scan(&k.id, &k.mult, &k.active); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
		}
		rows.Close()
		for i, k := range keys {
			allowed := e.Entitled() && int64(i) < e.maxKeys()
			if k.mult == e.multiplier() && k.active == allowed {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE api_keys SET rate_limit_multiplier = $1, billing_active = $2, updated_at = $3 WHERE id = $4`,
				e.multiplier(), allowed, users.Now(), k.id); err != nil {
				return err
			}
		}
		return nil
	}))
	clearKeyCaches()
	return e
}

// clearKeyCaches is set by the API layer to evict the rate-limit multiplier
// cache after key changes (the model's after_commit).
var clearKeyCaches = func() {}

// SetCacheEviction installs the key cache eviction hook.
func SetCacheEviction(f func()) { clearKeyCaches = f }
