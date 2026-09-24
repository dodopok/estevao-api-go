package users

import (
	"context"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// ValidModes is UserOnboarding::VALID_MODES.
var ValidModes = []string{"basic", "advanced", "simplified"}

func validMode(m string) bool {
	for _, v := range ValidModes {
		if v == m {
			return true
		}
	}
	return false
}

// PreferencesWithDefaults ports UserOnboarding#preferences_with_defaults.
func (o *Onboarding) PreferencesWithDefaults(ctx context.Context, pb *store.PrayerBook) (*rb.Map, error) {
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return nil, err
	}
	out := rb.NewMap()
	for _, d := range defs.Definitions {
		if v, ok := o.Preferences.Lookup(d.Key); ok {
			out.Set(d.Key, v)
		} else {
			out.Set(d.Key, d.TypedDefaultValue())
		}
	}
	return out, nil
}

// Errors ports the onboarding validations (mode, then
// validate_preferences_against_definitions) as full messages. pb is the
// onboarding's (possibly just assigned) prayer book.
func (o *Onboarding) Errors(ctx context.Context, pb *store.PrayerBook) ([]string, error) {
	var out []string
	if rb.BlankString(o.Mode) {
		out = append(out, "Mode can't be blank")
	}
	if !validMode(o.Mode) {
		out = append(out, "Mode is not included in the list")
	}
	if o.Preferences == nil || o.Preferences.Len() == 0 {
		return out, nil
	}
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return nil, err
	}
	defs.CanonicalizeLegacyKeys(o.Preferences).Each(func(key string, value any) {
		d := defs.Get(key)
		if d == nil {
			out = append(out, "Preferences key '"+key+"' does not exist for this prayer book")
			return
		}
		if d.ValidValue(value) {
			return
		}
		if d.PrefType == "select_one" || d.PrefType == "select_multiple" {
			out = append(out, "Preferences invalid value '"+rb.ToS(value)+"' for '"+key+"'. Allowed values: "+strings.Join(d.ValidOptionValues(), ", "))
		} else {
			out = append(out, "Preferences invalid value '"+rb.ToS(value)+"' for '"+key+"' (expected "+d.PrefType+")")
		}
	})
	return out, nil
}

// applyModeDefaults ports before_save :apply_mode_defaults.
func (o *Onboarding) applyModeDefaults(ctx context.Context, pb *store.PrayerBook) error {
	if o.Mode != "basic" && o.Mode != "simplified" {
		return nil
	}
	if o.Preferences != nil && o.Preferences.Len() > 0 {
		return nil
	}
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return err
	}
	p := rb.NewMap()
	for _, d := range defs.Definitions {
		if o.Mode == "simplified" {
			p.Set(d.Key, d.TypedSimpleValue())
		} else {
			p.Set(d.Key, d.TypedDefaultValue())
		}
	}
	o.Preferences = p
	return nil
}

// SaveOnboarding ports onboarding.save! (insert or update): validations,
// before_save callbacks, then the row. before is nil for a new record.
func SaveOnboarding(ctx context.Context, q db.Querier, before, o *Onboarding, pb *store.PrayerBook) error {
	msgs, err := o.Errors(ctx, pb)
	if err != nil {
		return err
	}
	if len(msgs) > 0 {
		return &RecordInvalid{Messages: msgs}
	}
	if err := o.applyModeDefaults(ctx, pb); err != nil {
		return err
	}
	now := Now()
	if o.CompletedAt == nil && o.OnboardingCompleted != nil && *o.OnboardingCompleted {
		o.CompletedAt = &now
	}
	if before == nil {
		o.CreatedAt, o.UpdatedAt = now, now
		return q.QueryRow(ctx, `INSERT INTO user_onboardings (user_id, prayer_book_id, bible_version_id, mode, preferences,
			onboarding_completed, completed_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
			o.UserID, o.PrayerBookID, o.BibleVersionID, o.Mode, string(rb.JSON(o.Preferences)), o.OnboardingCompleted,
			o.CompletedAt, now).Scan(&o.ID)
	}
	changed := before.PrayerBookID != o.PrayerBookID || before.BibleVersionID != o.BibleVersionID || before.Mode != o.Mode ||
		!Equal(before.Preferences, o.Preferences) || !timeEq(before.CompletedAt, o.CompletedAt) ||
		!boolEq(before.OnboardingCompleted, o.OnboardingCompleted)
	if !changed {
		return nil
	}
	o.UpdatedAt = now
	_, err = q.Exec(ctx, `UPDATE user_onboardings SET prayer_book_id = $1, bible_version_id = $2, mode = $3, preferences = $4,
		onboarding_completed = $5, completed_at = $6, updated_at = $7 WHERE id = $8`,
		o.PrayerBookID, o.BibleVersionID, o.Mode, string(rb.JSON(o.Preferences)), o.OnboardingCompleted, o.CompletedAt, now, o.ID)
	return err
}

func timeEq(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func boolEq(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Clone copies an onboarding (the "before" of an update).
func (o *Onboarding) Clone() *Onboarding {
	c := *o
	if o.Preferences != nil {
		c.Preferences = o.Preferences.Dup()
	}
	return &c
}
