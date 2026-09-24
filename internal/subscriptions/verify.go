package subscriptions

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// userDataTTL is Cacheable::TTL::USER_DATA.
const userDataTTL = 5 * time.Minute

// Verify ports Subscriptions::Verify.call. rcID is the parameter's to_s.
func Verify(ctx context.Context, u *users.User, rcID string) *rb.Map {
	if rb.BlankString(rcID) {
		web.Raise("InvalidPreference", "revenue_cat_user_id is required", "INVALID_REVENUECAT_USER_ID")
	}
	if u.RevenueCatUserID == nil || *u.RevenueCatUserID != rcID {
		before := u.Clone()
		u.RevenueCatUserID = &rcID
		if _, err := users.Save(ctx, db.Q(), before, u); err != nil {
			if inv, ok := err.(*users.RecordInvalid); ok {
				panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + strings.Join(inv.Messages, ", ")})
			}
			panic(err)
		}
	}
	key := "v8/user_data/subscription_verify/" + strconv.FormatInt(u.ID, 10) + "/" + rcID
	actor := u
	raw := rediscache.FetchJSON(ctx, key, userDataTTL, func() []byte {
		UpdateUserPremiumStatus(ctx, u)
		fresh, err := u.Reload(ctx)
		if err != nil {
			panic(err)
		}
		actor = fresh
		result := rb.M("premium", fresh.Premium(), "expires_at", timeOrNil(fresh.PremiumExpiresAt),
			"preferred_voice", fresh.PreferredAudioVoice())
		if !fresh.Premium() {
			result.Set("message", "No active subscription found")
		}
		return rb.JSON(result)
	})
	parsed, err := rb.ParseJSON(raw)
	if err != nil {
		panic(err)
	}
	response := parsed.(*rb.Map)
	response.Set("features", features.ForUser(ctx, actor, response.Get("premium") == true, time.Now()))
	return response
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}
