package v1

import (
	"slices"
	"time"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/billing"
	"github.com/dodopok/estevao-api-go/internal/features"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/subscriptions"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// --- PlansController --------------------------------------------------------

// PlansIndex ports PlansController#index.
func PlansIndex(c *web.Context) {
	data := []any{}
	for _, p := range billing.Purchasable() {
		data = append(data, p.AsJSON())
	}
	c.JSON(200, rb.M("data", data, "metadata", rb.M(
		"trial_period_days", billing.TrialPeriodDays(),
		"currencies", billing.SupportedCurrencies(),
		"intervals", billing.SupportedIntervals,
		"base_daily_request_limit", billing.BaseDailyLimit(),
		"contact_email", billing.SupportEmail(),
	)))
}

// --- SubscriptionsController ------------------------------------------------

// SubscriptionsVerify ports SubscriptionsController#verify.
func SubscriptionsVerify(c *web.Context) {
	auth.AuthenticateRequired(c)
	if !c.ParamPresent("revenue_cat_user_id") {
		c.JSON(422, rb.M("error", "revenue_cat_user_id is required"))
		return
	}
	c.JSON(200, subscriptions.Verify(c.Ctx, auth.CurrentUser(c), c.ParamS("revenue_cat_user_id")))
}

// SubscriptionsPremiumStatus ports SubscriptionsController#premium_status.
func SubscriptionsPremiumStatus(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	c.JSON(200, rb.M(
		"premium", u.Premium(),
		"expires_at", timeOrNil(u.PremiumExpiresAt),
		"preferred_voice", u.PreferredAudioVoice(),
		"features", features.ForUser(c.Ctx, u, u.Premium(), time.Now()),
		"available_voices", voiceInfo(false, ""),
	))
}

func voiceInfo(samples bool, base string) []any {
	out := []any{}
	for _, v := range audio.Voices {
		m := rb.M("key", v.Key, "name", v.Name, "gender", v.Gender)
		if samples {
			m.Set("sample_url", base+"/audio/samples/sample_"+v.Key+".mp3")
		}
		out = append(out, m)
	}
	return out
}

// --- AudioController --------------------------------------------------------

// AudioVoiceSamples ports AudioController#voice_samples.
func AudioVoiceSamples(c *web.Context) {
	auth.AuthenticateRequired(c)
	c.JSON(200, rb.M("voices", voiceInfo(true, c.BaseURL()), "default_voice", "male_1"))
}

// AudioURL ports AudioController#url.
func AudioURL(c *web.Context) {
	auth.AuthenticateRequired(c)
	auth.RequirePremium(c)
	u := auth.CurrentUser(c)
	if !features.EnabledFor(c.Ctx, "daily_office_audio", u, u != nil && u.Premium(), time.Now()) {
		c.RenderJSON(403, rb.M("error", "Audio feature is not enabled for this user", "code", "FEATURE_NOT_AVAILABLE",
			"feature", "daily_office_audio"))
	}
	code, voice, slug := c.ParamS("prayer_book"), c.ParamS("voice"), c.ParamS("slug")
	if !slices.Contains(users.AvailableVoices, voice) {
		c.JSON(422, rb.M("error", "Invalid voice", "available_voices", users.AvailableVoices))
		return
	}
	pb, err := store.PrayerBookByCode(c.Ctx, code)
	must(err)
	text := store.FindLiturgicalText(c.Ctx, pb, slug)
	if text == nil {
		c.JSON(404, rb.M("error", "Liturgical text not found"))
		return
	}
	if url := AudioURLForVoice(text, voice); url != "" {
		RecordAudioUsage(c.Ctx, u.ID, []*rb.Map{rb.M("audio_type", "liturgical_text",
			"asset_key", itoa64(text.ID)+":"+voice, "liturgical_text_id", text.ID, "prayer_book_code", code, "voice", voice)})
		c.JSON(200, rb.M("audio_url", c.BaseURL()+url, "voice", voice, "slug", slug, "title", rb.Deref(text.Title)))
		return
	}
	generated := []string{}
	for _, v := range users.AvailableVoices {
		if text.AudioURLs != nil && rb.Present(text.AudioURLs.Get(v)) {
			generated = append(generated, v)
		}
	}
	c.JSON(404, rb.M("error", "Audio not yet generated for this voice", "voice", voice, "available_voices", generated))
}
