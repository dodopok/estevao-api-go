package v1

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// serializeOnboarding ports OnboardingController#serialize_onboarding.
func serializeOnboarding(ctx context.Context, u *users.User, o *users.Onboarding) *rb.Map {
	pb, err := store.PrayerBookByID(ctx, o.PrayerBookID)
	must(err)
	bvs, err := store.BibleVersionsWhere(ctx, "WHERE id = $1", o.BibleVersionID)
	must(err)
	if pb == nil || len(bvs) == 0 {
		panic(&web.StandardError{Class: "NoMethodError", Message: "undefined method 'code' for nil"})
	}
	withDefaults, err := o.PreferencesWithDefaults(ctx, pb)
	must(err)
	var completed any
	if o.CompletedAt != nil {
		completed = iso8601(*o.CompletedAt)
	}
	var done any
	if o.OnboardingCompleted != nil {
		done = *o.OnboardingCompleted
	}
	return rb.M("id", "onb_"+itoa64(o.ID), "user_id", rb.Deref(u.ProviderUID), "prayer_book_id", pb.Code,
		"bible_version_id", bvs[0].Code, "mode", o.Mode, "preferences", withDefaults, "onboarding_completed", done,
		"completed_at", completed, "created_at", iso8601(o.CreatedAt), "updated_at", iso8601(o.UpdatedAt))
}

// OnboardingShow ports OnboardingController#show.
func OnboardingShow(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	if u.Onboarding == nil {
		c.JSON(404, rb.M("success", false, "error", rb.M("code", "ONBOARDING_NOT_FOUND", "message", "User has not completed onboarding yet")))
		return
	}
	c.JSON(200, rb.M("success", true, "data", serializeOnboarding(c.Ctx, u, u.Onboarding)))
}

// castTime ports ActiveModel's datetime cast of a parameter (nil when it
// does not parse), in the application time zone.
func castTime(v any) *time.Time {
	s, ok := v.(string)
	if !ok || rb.BlankString(s) {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		var t time.Time
		var err error
		if layout == time.RFC3339Nano {
			t, err = time.Parse(layout, s)
		} else {
			t, err = time.ParseInLocation(layout, s, rb.AppZone)
		}
		if err == nil {
			t = t.UTC().Truncate(time.Microsecond)
			return &t
		}
	}
	return nil
}

// OnboardingCreate ports OnboardingController#create (Users::CompleteOnboarding).
func OnboardingCreate(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	param := func(k string) any {
		v, ok := c.Params().Lookup(k)
		if !ok || !permittedScalar(v) {
			return nil
		}
		return v
	}
	pbCode, bvCode := param("prayer_book_id"), param("bible_version_id")
	pb, err := findBook(c.Ctx, pbCode)
	must(err)
	var bv *store.BibleVersion
	if _, isMap := bvCode.(*rb.Map); !isMap {
		bv, err = store.BibleVersionByCode(c.Ctx, rb.ToS(bvCode))
		must(err)
	}
	if pb == nil || pb.ExternalOnly || bv == nil {
		c.JSON(404, rb.M("success", false, "error", rb.M("code", "RESOURCE_NOT_FOUND",
			"message", "Prayer Book or Bible version not found",
			"details", rb.M("prayer_book_id", pbCode, "bible_version_id", bvCode))))
		return
	}
	defs, err := prefs.For(c.Ctx, pb)
	must(err)
	submitted := rb.NewMap()
	if m, ok := c.Param("preferences").(*rb.Map); ok {
		submitted = m
	}
	mode := "basic"
	if v := param("mode"); v != nil {
		mode = rb.ToS(v)
	}
	completedAt := castTime(param("completed_at"))
	if completedAt == nil {
		now := users.Now()
		completedAt = &now
	}
	yes := true
	var before *users.Onboarding
	o := &users.Onboarding{UserID: u.ID}
	if u.Onboarding != nil {
		before = u.Onboarding.Clone()
		o = u.Onboarding.Clone()
	}
	o.PrayerBookID, o.BibleVersionID, o.Mode = pb.ID, bv.ID, mode
	o.Preferences = defs.CanonicalizeLegacyKeys(submitted)
	o.OnboardingCompleted, o.CompletedAt = &yes, completedAt

	err = db.InTx(c.Ctx, func(tx pgx.Tx) error {
		if err := users.SaveOnboarding(c.Ctx, tx, before, o, pb); err != nil {
			return err
		}
		withDefaults, err := o.PreferencesWithDefaults(c.Ctx, pb)
		if err != nil {
			return err
		}
		overrides := rb.M("prayer_book_code", pb.Code, "version", pb.Code, "language", pb.Language,
			"bible_version", bv.Code, "mode", o.Mode).Merge(withDefaults)
		resolved, err := prefs.Resolve(c.Ctx, pb, u.Preferences, overrides)
		if err != nil {
			return err
		}
		after := u.Clone()
		after.Preferences = resolved.Values.Dup()
		after.Onboarding = o
		if _, err := users.Save(c.Ctx, tx, u, after); err != nil {
			return err
		}
		*u = *after
		return nil
	})
	var ri *users.RecordInvalid
	if errors.As(err, &ri) {
		msgs := make([]any, len(ri.Messages))
		for i, m := range ri.Messages {
			msgs[i] = m
		}
		c.JSON(400, rb.M("success", false, "error", rb.M("code", "INVALID_PREFERENCES",
			"message", "Some preferences are invalid for the selected Prayer Book",
			"details", rb.M("prayer_book_id", pbCode, "invalid_preferences", msgs))))
		return
	}
	must(err)
	c.JSON(200, rb.M("success", true, "message", "Onboarding preferences saved successfully",
		"data", serializeOnboarding(c.Ctx, u, o)))
}

func findBook(ctx context.Context, code any) (*store.PrayerBook, error) {
	switch code.(type) {
	case *rb.Map, []any:
		return nil, nil
	}
	return store.PrayerBookByCode(ctx, rb.ToS(code))
}
