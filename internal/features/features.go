// Package features ports FeatureFlags (app/services/feature_flags.rb) and the
// FeatureFlag overrides it reads.
package features

import (
	"context"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
)

// Definition is one FeatureFlags::DEFINITIONS entry.
type Definition struct {
	RequiresPremium bool
	Targeted        bool
	DefaultEnabled  bool
	TestEnabled     bool
}

// Keys lists the registered flags in DEFINITIONS order.
var Keys = []string{"daily_office_audio", "background_music", "yearly_wrapped"}

// Definitions ports FeatureFlags::DEFINITIONS.
var Definitions = map[string]Definition{
	"daily_office_audio": {RequiresPremium: true, Targeted: true, TestEnabled: true},
	"background_music":   {RequiresPremium: true, Targeted: true, TestEnabled: true},
	"yearly_wrapped":     {RequiresPremium: false, Targeted: true, TestEnabled: true},
}

// YearlyWrappedOfficeTypes ports YEARLY_WRAPPED_OFFICE_TYPES.
var YearlyWrappedOfficeTypes = []string{"morning", "prime", "terce", "midday", "evening", "compline", "late_evening"}

// TestEnv reports Rails.env.test? for the running deployment.
var TestEnv = false

// Overrides ports FeatureFlag.overrides_for (read fresh; Rails keeps them for
// one minute).
type Overrides struct {
	Global *bool
	Users  map[int64]bool
}

// OverridesFor loads the persisted overrides of a feature.
func OverridesFor(ctx context.Context, feature string) Overrides {
	rows, err := db.Q().Query(ctx, `SELECT "feature_flags"."target_type", "feature_flags"."user_id", "feature_flags"."enabled"
FROM "feature_flags" WHERE "feature_flags"."feature_key" = $1`, feature)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	o := Overrides{Users: map[int64]bool{}}
	for rows.Next() {
		var target string
		var userID *int64
		var enabled bool
		if err := rows.Scan(&target, &userID, &enabled); err != nil {
			panic(err)
		}
		if target == "global" {
			e := enabled
			o.Global = &e
		} else if userID != nil {
			o.Users[*userID] = enabled
		}
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return o
}

// RolloutEnabled ports rollout_enabled?.
func RolloutEnabled(ctx context.Context, feature string, user *users.User, premium bool) bool {
	def, ok := Definitions[feature]
	if !ok {
		return false
	}
	if def.RequiresPremium && !premium {
		return false
	}
	if def.Targeted && user == nil {
		return false
	}
	o := OverridesFor(ctx, feature)
	if user != nil {
		if v, ok := o.Users[user.ID]; ok {
			return v
		}
	}
	if o.Global != nil {
		return *o.Global
	}
	if TestEnv {
		return def.TestEnabled
	}
	return def.DefaultEnabled
}

// EnabledFor ports enabled_for? (year and as_of are Time.current's).
func EnabledFor(ctx context.Context, feature string, user *users.User, premium bool, now time.Time) bool {
	if _, ok := Definitions[feature]; !ok {
		return false
	}
	if !RolloutEnabled(ctx, feature, user, premium) {
		return false
	}
	if feature != "yearly_wrapped" {
		return true
	}
	return WrappedAvailableFor(ctx, user, now.In(rb.AppZone).Year(), now)
}

// ForUser ports for_user.
func ForUser(ctx context.Context, user *users.User, premium bool, now time.Time) *rb.Map {
	out := rb.NewMap()
	for _, k := range Keys {
		out.Set(k, EnabledFor(ctx, k, user, premium, now))
	}
	return out
}

// WrappedAvailableFor ports wrapped_available_for?: a completion of a
// counted office in the user's year so far.
func WrappedAvailableFor(ctx context.Context, user *users.User, year int, asOf time.Time) bool {
	if user == nil {
		return false
	}
	zone := rb.RailsZone(user.Timezone)
	if zone == nil {
		zone = time.UTC
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, zone)
	next := time.Date(year+1, 1, 1, 0, 0, 0, 0, zone)
	end := next
	if asOf.Before(end) {
		end = asOf
	}
	if !end.After(start) {
		return false
	}
	var one int
	err := db.Q().QueryRow(ctx, `SELECT 1 AS one FROM "completions" WHERE "completions"."user_id" = $1
AND "completions"."created_at" >= $2 AND "completions"."created_at" < $3
AND "completions"."office_type" = ANY($4) LIMIT 1`, user.ID, start.UTC(), end.UTC(), YearlyWrappedOfficeTypes).Scan(&one)
	if db.NoRows(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}
