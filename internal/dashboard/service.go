package dashboard

import (
	"context"
	"time"

	"github.com/dodopok/estevao-api-go/internal/audioadmin"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
)

// Sections ports DashboardDataService::SECTION_CLASSES, in order.
var Sections = []string{"overview", "users", "completions", "prayer_books", "journals", "audio", "notifications",
	"life_rules", "shared_offices", "engagement", "moderation", "health", "retention", "onboarding", "premium",
	"geography", "custom_rosaries", "weekly_prayers", "favorites", "api", "developers"}

func (p *Period) section(name string) *rb.Map {
	switch name {
	case "overview":
		return p.Overview()
	case "users":
		return p.Users()
	case "completions":
		return p.Completions()
	case "prayer_books":
		return p.PrayerBooks()
	case "journals":
		return p.Journals()
	case "audio":
		return audioadmin.Summary(p.Ctx)
	case "notifications":
		return p.Notifications()
	case "life_rules":
		return p.LifeRules()
	case "shared_offices":
		return p.SharedOffices()
	case "engagement":
		return p.Engagement()
	case "moderation":
		return p.Moderation()
	case "health":
		return p.Health()
	case "retention":
		return p.Retention()
	case "onboarding":
		return p.Onboarding()
	case "premium":
		return p.Premium()
	case "geography":
		return p.Geography()
	case "custom_rosaries":
		return p.CustomRosaries()
	case "weekly_prayers":
		return p.WeeklyPrayers()
	case "favorites":
		return p.Favorites()
	case "api":
		return p.API()
	case "developers":
		return p.Developers()
	}
	panic("dashboard: unknown section " + name)
}

// Call ports DashboardDataService#call: each section cached ten minutes
// under the dates it was computed for.
func Call(ctx context.Context, start, end civil.Date, sections []string) *rb.Map {
	out := rb.NewMap()
	for _, s := range sections {
		key := "dashboard/" + s + "/" + start.ISO() + "/" + end.ISO()
		raw := rediscache.FetchJSON(ctx, key, 10*time.Minute, func() []byte {
			p := &Period{Ctx: ctx, Start: start, End: end, Now: time.Now()}
			return rb.JSON(p.section(s))
		})
		v, err := rb.ParseJSON(raw)
		must(err)
		out.Set(s, v)
	}
	return out
}
