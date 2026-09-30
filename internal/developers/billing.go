package developers

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/billing"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
)

// CheckoutSession is a DeveloperCheckoutSession row.
type CheckoutSession struct {
	ID                                           int64
	IdempotencyKey, PlanCode, Interval, Currency string
	StripeCheckoutSessionID, URL                 *string
	TrialPeriodDays                              int64
	ExpiresAt                                    time.Time
}

// CheckoutSessionOf loads the developer's checkout session (has_one).
func CheckoutSessionOf(ctx context.Context, q db.Querier, developerID int64) *CheckoutSession {
	s := &CheckoutSession{}
	err := q.QueryRow(ctx, `SELECT id, idempotency_key, plan_code, interval, currency, stripe_checkout_session_id, url,
		trial_period_days, expires_at FROM developer_checkout_sessions WHERE developer_id = $1 LIMIT 1`, developerID).
		Scan(&s.ID, &s.IdempotencyKey, &s.PlanCode, &s.Interval, &s.Currency, &s.StripeCheckoutSessionID, &s.URL,
			&s.TrialPeriodDays, &s.ExpiresAt)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return s
}

// Checkout errors (Billing::StartCheckout).
type (
	AlreadySubscribed struct{}
	UnavailablePlan   struct{ Message string }
	CheckoutPending   struct{ Session *CheckoutSession }
)

func (AlreadySubscribed) Error() string { return "Developer already has a live subscription" }
func (e UnavailablePlan) Error() string { return e.Message }
func (CheckoutPending) Error() string   { return "A checkout for another plan is already pending" }

// CheckoutResult ports StartCheckout::Result.
type CheckoutResult struct {
	URL             any
	PlanCode        string
	TrialPeriodDays int64
	Reused          bool
}

func supportedInterval(i string) bool {
	for _, s := range billing.SupportedIntervals {
		if s == i {
			return true
		}
	}
	return false
}

// StartCheckout ports Billing::StartCheckout.call: under the developer's
// row lock it reuses a matching pending Checkout, or creates the Stripe
// customer and session. A Stripe failure is raised after the lock (so the
// attempt's cleanup commits); plan errors roll back.
func StartCheckout(ctx context.Context, d *Developer, planCode, interval, currency, locale any) (*CheckoutResult, error) {
	code := rb.ToS(planCode)
	iv := "month"
	if rb.Present(interval) {
		iv = rb.ToS(interval)
	}
	cur := "brl"
	if rb.Present(currency) {
		cur = strings.ToLower(rb.ToS(currency))
	}
	var result *CheckoutResult
	var stripeErr error
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		locked := LoadDevelopers(ctx, tx, "developers.id = $1", "LIMIT 1 FOR UPDATE", d.ID)
		if len(locked) == 0 {
			return errors.New("developer vanished")
		}
		*d = *locked[0]
		if SubscriptionOf(ctx, d.ID).Entitled() {
			return AlreadySubscribed{}
		}
		pending := CheckoutSessionOf(ctx, tx, d.ID)
		if pending != nil {
			if !pending.ExpiresAt.After(clock.Now()) {
				if _, err := tx.Exec(ctx, `DELETE FROM developer_checkout_sessions WHERE id = $1`, pending.ID); err != nil {
					return err
				}
				pending = nil
			} else if pending.PlanCode != code || pending.Interval != iv || pending.Currency != cur {
				return CheckoutPending{pending}
			}
		}
		if pending != nil && pending.URL != nil && strings.TrimSpace(*pending.URL) != "" {
			result = resultFor(pending, true)
			return nil
		}
		plan := billing.Find(code)
		switch {
		case plan == nil || !plan.Purchasable:
			return UnavailablePlan{"Plan is not purchasable"}
		case !supportedInterval(iv):
			return UnavailablePlan{"Unsupported interval"}
		case len(plan.PriceFor(cur, iv)) == 0:
			return UnavailablePlan{"Unsupported currency"}
		case plan.StripePriceID(cur, iv) == "":
			return UnavailablePlan{"Plan has no configured Stripe price"}
		}
		if pending == nil {
			trial := int64(billing.TrialPeriodDays())
			if SubscriptionOf(ctx, d.ID) != nil {
				trial = 0
			}
			pending = &CheckoutSession{IdempotencyKey: uuid(), PlanCode: plan.Code, Interval: iv, Currency: cur,
				TrialPeriodDays: trial, ExpiresAt: clock.Now().Add(31 * time.Minute).Truncate(time.Microsecond)}
			now := users.Now()
			if err := tx.QueryRow(ctx, `INSERT INTO developer_checkout_sessions (developer_id, idempotency_key, plan_code, interval,
				currency, trial_period_days, expires_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
				d.ID, pending.IdempotencyKey, pending.PlanCode, pending.Interval, pending.Currency, pending.TrialPeriodDays,
				pending.ExpiresAt, now).Scan(&pending.ID); err != nil {
				return err
			}
		}
		session, err := createStripeSession(ctx, tx, d, plan, cur, iv, locale, pending)
		if err == nil {
			url, _ := session.Get("url").(string)
			if strings.TrimSpace(url) == "" {
				err = &StripeError{Message: "Stripe returned no checkout URL"}
			} else {
				expires := pending.ExpiresAt
				if v := session.Get("expires_at"); rb.Present(v) {
					expires = time.Unix(int64(rb.StringToI(rb.ToS(v))), 0)
				}
				if _, err := tx.Exec(ctx, `UPDATE developer_checkout_sessions SET stripe_checkout_session_id = $1, url = $2,
					expires_at = $3, updated_at = $4 WHERE id = $5`, session.Get("id"), url, expires, users.Now(), pending.ID); err != nil {
					return err
				}
				pending.URL, pending.ExpiresAt = &url, expires
				result = resultFor(pending, false)
				return nil
			}
		}
		var se *StripeError
		if errors.As(err, &se) {
			if !se.Uncertain {
				if _, err := tx.Exec(ctx, `DELETE FROM developer_checkout_sessions WHERE id = $1`, pending.ID); err != nil {
					return err
				}
			}
			stripeErr = se
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if stripeErr != nil {
		return nil, stripeErr
	}
	return result, nil
}

func resultFor(s *CheckoutSession, reused bool) *CheckoutResult {
	var url any
	if s.URL != nil {
		url = *s.URL
	}
	if billing.Find(s.PlanCode) == nil {
		panic(&rb.RubyError{Class: "Billing::PlanCatalog::UnknownPlanError", Message: "Unknown plan: " + s.PlanCode})
	}
	return &CheckoutResult{URL: url, PlanCode: s.PlanCode, TrialPeriodDays: s.TrialPeriodDays, Reused: reused}
}

func createStripeSession(ctx context.Context, tx pgx.Tx, d *Developer, plan *billing.Plan, cur, iv string, locale any,
	pending *CheckoutSession) (*rb.Map, error) {
	client, err := NewStripeClient()
	if err != nil {
		return nil, err
	}
	priceID := plan.StripePriceID(cur, iv)
	customer := rb.ToS(d.Get("stripe_customer_id"))
	if strings.TrimSpace(customer) == "" {
		var name any = d.Get("name")
		if rb.Blank(name) {
			name = d.Get("company_name")
		}
		c, err := client.CreateCustomer(ctx, rb.ToS(d.Get("email")), name, d.ID, "developer-customer-v1-"+strconv.FormatInt(d.ID, 10))
		if err != nil {
			return nil, err
		}
		id, ok := c.Lookup("id")
		if !ok {
			panic(&rb.RubyError{Class: "KeyError", Message: `key not found: "id"`})
		}
		d.Assign("stripe_customer_id", id)
		if errs := d.Validate(ctx); errs.Any() {
			panic(&rb.RubyError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + joinMessages(errs)})
		}
		if _, err := d.Update(ctx, tx, users.Now()); err != nil {
			return nil, err
		}
		customer = rb.ToS(d.Get("stripe_customer_id"))
	}
	meta := []kv{{"developer_id", strconv.FormatInt(d.ID, 10)}, {"plan_code", plan.Code}}
	return client.CreateCheckoutSession(ctx, CheckoutParams{PriceID: priceID, CustomerID: customer,
		SuccessURL: BillingURL(locale, "success"), CancelURL: BillingURL(locale, "cancelled"),
		TrialPeriodDays: pending.TrialPeriodDays, Locale: NormalizeLocale(locale), Metadata: meta,
		ExpiresAt: pending.ExpiresAt, IdempotencyKey: pending.IdempotencyKey})
}

// NoCustomer ports OpenPortal::NoCustomerError.
type NoCustomer struct{}

func (NoCustomer) Error() string { return "Developer has no Stripe customer" }

// OpenPortal ports Billing::OpenPortal.call.
func OpenPortal(ctx context.Context, d *Developer, locale any) (string, error) {
	customer := rb.ToS(d.Get("stripe_customer_id"))
	if strings.TrimSpace(customer) == "" {
		return "", NoCustomer{}
	}
	client, err := NewStripeClient()
	if err != nil {
		return "", err
	}
	session, err := client.CreateBillingPortalSession(ctx, customer, BillingURL(locale, ""), NormalizeLocale(locale))
	if err != nil {
		return "", err
	}
	url, _ := session.Get("url").(string)
	if strings.TrimSpace(url) == "" {
		return "", &StripeError{Message: "Stripe returned no portal URL"}
	}
	return url, nil
}

// --- webhook -------------------------------------------------------------------

var handledEvents = map[string]bool{"customer.subscription.created": true, "customer.subscription.updated": true,
	"customer.subscription.deleted": true, "checkout.session.completed": true}

func dig(m *rb.Map, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(*rb.Map)
		if !ok {
			if cur == nil {
				return nil
			}
			panic(&rb.RubyError{Class: "TypeError", Message: rb.ClassName(cur) + " does not have #dig method"})
		}
		cur = mm.Get(k)
	}
	return cur
}

// ProcessWebhookEvent ports Billing::ProcessWebhookEvent.call; the status
// is ignored, duplicate, unmatched or processed.
func ProcessWebhookEvent(ctx context.Context, event *rb.Map) (string, error) {
	eventType := rb.ToS(event.Get("type"))
	if !handledEvents[eventType] {
		return "ignored", nil
	}
	var processed bool
	must(db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stripe_webhook_events WHERE event_id = $1 AND processed_at IS NOT NULL)`,
		rb.ToS(event.Get("id"))).Scan(&processed))
	if processed {
		return "duplicate", nil
	}
	object, _ := dig(event, "data", "object").(*rb.Map)
	if object == nil {
		object = rb.NewMap()
	}
	var developer *Developer
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		eventID := event.Get("id")
		if rb.Blank(eventID) {
			panic(&rb.RubyError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: Event can't be blank"})
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stripe_webhook_events WHERE event_id = $1)`, rb.ToS(eventID)).Scan(&taken); err != nil {
			return err
		}
		if taken {
			panic(&rb.RubyError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: Event has already been taken"})
		}
		now := users.Now()
		var entryID int64
		if err := tx.QueryRow(ctx, `INSERT INTO stripe_webhook_events (event_id, event_type, created_at, updated_at) VALUES ($1, $2, $3, $3) RETURNING id`,
			rb.ToS(eventID), eventType, now).Scan(&entryID); err != nil {
			return err
		}
		developer = resolveDeveloper(ctx, tx, object)
		if developer != nil {
			if err := handleSubscriptionEvent(ctx, developer, eventType, object, event.Get("created")); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE stripe_webhook_events SET processed_at = $1, updated_at = $1 WHERE id = $2`, users.Now(), entryID)
		return err
	})
	if db.UniqueViolation(err) {
		return "duplicate", nil
	}
	if err != nil {
		return "", err
	}
	if developer == nil {
		return "unmatched", nil
	}
	return "processed", nil
}

func resolveDeveloper(ctx context.Context, q db.Querier, object *rb.Map) *Developer {
	id := dig(object, "metadata", "developer_id")
	if !rb.Present(id) {
		id = dig(object, "subscription_details", "metadata", "developer_id")
	}
	if rb.Present(id) {
		if n, ok := rb.CastInteger(id); ok {
			if d := findDeveloper(ctx, q, "developers.id = $1", n); d != nil {
				return d
			}
		}
	}
	customer := object.Get("customer")
	if !rb.Present(customer) {
		return nil
	}
	return findDeveloper(ctx, q, "developers.stripe_customer_id = $1", rb.ToS(customer))
}

func handleSubscriptionEvent(ctx context.Context, d *Developer, eventType string, object *rb.Map, created any) error {
	var subID any = object.Get("id")
	if eventType == "checkout.session.completed" {
		subID = object.Get("subscription")
	}
	if rb.Blank(subID) {
		return nil
	}
	client, err := NewStripeClient()
	if err != nil {
		return err
	}
	sub, err := client.RetrieveSubscription(ctx, rb.ToS(subID))
	if err != nil {
		return err
	}
	if sub.Len() == 0 {
		return nil
	}
	return SyncSubscription(ctx, d, sub, created, eventType == "checkout.session.completed" || eventType == "customer.subscription.created")
}

func stripeTime(v any) *time.Time {
	if rb.Blank(v) {
		return nil
	}
	t := time.Unix(int64(rb.StringToI(rb.ToS(v))), 0)
	return &t
}

// UnresolvablePlan ports SyncSubscription::UnresolvablePlanError.
type UnresolvablePlan struct{ Message string }

func (e UnresolvablePlan) Error() string { return e.Message }

// SyncSubscription ports Billing::SyncSubscription.call.
func SyncSubscription(ctx context.Context, d *Developer, sub *rb.Map, eventCreated any, allowReplacement bool) error {
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if locked := LoadDevelopers(ctx, tx, "developers.id = $1", "LIMIT 1 FOR UPDATE", d.ID); len(locked) > 0 {
			*d = *locked[0]
		}
		current := SubscriptionOf(ctx, d.ID)
		created := stripeTime(eventCreated)
		if current != nil {
			if created != nil && current.LastStripeEventCreatedAt != nil && created.Before(*current.LastStripeEventCreatedAt) {
				return nil
			}
			if current.StripeSubscriptionID != rb.ToS(sub.Get("id")) && !allowReplacement {
				return nil
			}
		}
		item, _ := dig(sub, "items", "data").([]any)
		var itemMap *rb.Map
		if len(item) > 0 {
			itemMap, _ = item[0].(*rb.Map)
		}
		if itemMap == nil {
			itemMap = rb.NewMap()
		}
		price, _ := itemMap.Get("price").(*rb.Map)
		if price == nil {
			price = rb.NewMap()
		}
		planCode := ""
		if priceID := rb.ToS(price.Get("id")); strings.TrimSpace(priceID) != "" {
			for _, p := range billing.All() {
				for _, c := range p.Prices {
					for _, iv := range c.Intervals {
						if planCode == "" && p.StripePriceID(c.Code, iv.Name) == priceID {
							planCode = p.Code
						}
					}
				}
			}
		}
		if planCode == "" {
			if m := rb.ToS(dig(sub, "metadata", "plan_code")); billing.Find(m) != nil {
				planCode = m
			} else if current != nil && current.PlanCode != "" {
				planCode = current.PlanCode
			} else {
				return UnresolvablePlan{"No plan matches Stripe price " + rb.Inspect(price.Get("id")) + " for subscription " + rb.Inspect(sub.Get("id"))}
			}
		}
		customer := rb.ToS(sub.Get("customer"))
		if strings.TrimSpace(customer) == "" {
			customer = rb.ToS(d.Get("stripe_customer_id"))
		}
		status := rb.ToS(sub.Get("status"))
		if strings.TrimSpace(status) == "" {
			status = "incomplete"
		}
		interval := rb.ToS(dig(price, "recurring", "interval"))
		if !supportedInterval(interval) {
			interval = "month"
		}
		currency := rb.ToS(price.Get("currency"))
		if strings.TrimSpace(currency) == "" {
			currency = "brl"
		}
		cancel := castBool(sub.Get("cancel_at_period_end"))
		periodEnd := sub.Get("current_period_end")
		if !rb.Present(periodEnd) {
			periodEnd = itemMap.Get("current_period_end")
		}
		subID := rb.ToS(sub.Get("id"))
		if strings.TrimSpace(subID) == "" || strings.TrimSpace(customer) == "" {
			panic(&rb.RubyError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed"})
		}
		now := users.Now()
		args := []any{subID, customer, planCode, status, interval, currency, cancel, stripeTime(sub.Get("trial_end")),
			stripeTime(periodEnd), stripeTime(sub.Get("canceled_at")), created, now}
		var err error
		if current == nil {
			_, err = tx.Exec(ctx, `INSERT INTO developer_subscriptions (stripe_subscription_id, stripe_customer_id, plan_code, status,
				interval, currency, cancel_at_period_end, trial_ends_at, current_period_end, canceled_at, last_stripe_event_created_at,
				created_at, updated_at, developer_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, $13)`,
				append(args, d.ID)...)
		} else {
			_, err = tx.Exec(ctx, `UPDATE developer_subscriptions SET stripe_subscription_id = $1, stripe_customer_id = $2,
				plan_code = $3, status = $4, interval = $5, currency = $6, cancel_at_period_end = $7, trial_ends_at = $8,
				current_period_end = $9, canceled_at = $10, last_stripe_event_created_at = $11, updated_at = $12 WHERE id = $13`,
				append(args, current.ID)...)
		}
		if err != nil {
			return err
		}
		if entitledStatuses[status] {
			if _, err := tx.Exec(ctx, `DELETE FROM developer_checkout_sessions WHERE developer_id = $1`, d.ID); err != nil {
				return err
			}
		}
		ApplyEntitlement(ctx, d)
		return nil
	})
}

// castBool ports `value || false` for a JSON boolean-ish value stored in a
// boolean column (Active Model's boolean cast).
func castBool(v any) bool {
	if v == nil || v == false {
		return false
	}
	switch x := v.(type) {
	case string:
		switch x {
		case "", "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false
		}
	case int:
		return x != 0
	case float64:
		return x != 0
	}
	return true
}
