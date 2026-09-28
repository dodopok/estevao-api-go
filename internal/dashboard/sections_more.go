package dashboard

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rosary"
)

// LifeRules ports Dashboard::LifeRuleMetrics.
func (p *Period) LifeRules() *rb.Map {
	pending := `"life_rules"."is_public" = $1 AND "life_rules"."approved" = $2`
	oldest := p.minTime(`SELECT MIN("life_rules"."created_at") FROM "life_rules" WHERE `+pending, true, false)
	var top []any
	rows := p.rows(`SELECT "life_rules"."id", "life_rules"."title", "life_rules"."adoption_count" FROM "life_rules" WHERE "life_rules"."is_public" = $1 AND "life_rules"."approved" = $2 ORDER BY "life_rules"."adoption_count" DESC, "life_rules"."id" ASC LIMIT $3`, true, true, 10)
	for rows.Next() {
		var id int64
		var title *string
		var n *int64
		must(rows.Scan(&id, &title, &n))
		top = append(top, rb.M("id", id, "title", rb.Deref(title), "adoptions", rb.Deref(n)))
	}
	rows.Close()
	must(rows.Err())
	if top == nil {
		top = []any{}
	}
	exams := `"life_rule_exams"."status" = $1 AND "life_rule_exams"."completed_at" BETWEEN $2 AND $3`
	args := append([]any{"completed"}, p.stamps()...)
	return scopePayload("lifetime", rb.M(
		"metric_scopes", rb.M("rules", "lifetime", "adoptions", "lifetime", "exams", "period"),
		"total_rules", p.count(`SELECT COUNT(*) FROM "life_rules"`),
		"public_rules", p.count(`SELECT COUNT(*) FROM "life_rules" WHERE "life_rules"."is_public" = $1`, true),
		"approved_rules", p.count(`SELECT COUNT(*) FROM "life_rules" WHERE "life_rules"."approved" = $1`, true),
		"pending_rules", p.count(`SELECT COUNT(*) FROM "life_rules" WHERE `+pending, true, false),
		"oldest_pending_at", timeOrNil(oldest), "oldest_pending_age_seconds", p.ageSeconds(oldest),
		"top_adopted", top,
		"total_adoptions", p.count(`SELECT SUM("life_rules"."adoption_count") FROM "life_rules"`),
		"exams", rb.M(
			"completed_in_period", p.count(`SELECT COUNT(*) FROM "life_rule_exams" WHERE `+exams, args...),
			"users_with_completed_exams", p.count(`SELECT COUNT(DISTINCT "life_rule_exams"."user_id") FROM "life_rule_exams" WHERE `+exams, args...),
			"average_score", p.average(`SELECT AVG("life_rule_exams"."score")::text FROM "life_rule_exams" WHERE `+exams+` AND "life_rule_exams"."score" IS NOT NULL`, args...),
			"by_period", p.groupCount(`SELECT COUNT(*) AS "count_all", "life_rule_exams"."period" AS "life_rule_exams_period" FROM "life_rule_exams" WHERE `+exams+` GROUP BY "life_rule_exams"."period"`, args...),
			"by_band", p.groupCount(`SELECT COUNT(*) AS "count_all", "life_rule_exams"."band" AS "life_rule_exams_band" FROM "life_rule_exams" WHERE `+exams+` GROUP BY "life_rule_exams"."band"`, args...))))
}

// SharedOffices ports Dashboard::SharedOfficeMetrics.
func (p *Period) SharedOffices() *rb.Map {
	where := `"shared_offices"."created_at" ` + dateBetween
	return scopePayload("period", rb.M(
		"total_in_period", p.count(`SELECT COUNT(*) FROM "shared_offices" WHERE `+where, p.stamps()...),
		"by_office_type", p.groupCount(`SELECT COUNT(*) AS "count_all", "shared_offices"."office_type" AS "shared_offices_office_type" FROM "shared_offices" WHERE `+where+` GROUP BY "shared_offices"."office_type"`, p.stamps()...),
		"by_prayer_book", p.groupCount(`SELECT COUNT(*) AS "count_all", "shared_offices"."prayer_book_code" AS "shared_offices_prayer_book_code" FROM "shared_offices" WHERE `+where+` GROUP BY "shared_offices"."prayer_book_code"`, p.stamps()...),
		"daily_shares", p.dailyList(`SELECT COUNT(*) AS "count_all", `+localDateSQL("shared_offices.created_at")+` AS "date_shared_offices_created_at_at_time_zone_utc_at_time_zone_am" FROM "shared_offices" WHERE `+where+` GROUP BY `+localDateSQL("shared_offices.created_at"), "count", p.stamps()...),
		"unique_users_sharing", p.count(`SELECT COUNT(DISTINCT "shared_offices"."user_id") FROM "shared_offices" WHERE `+where+` AND "shared_offices"."user_id" IS NOT NULL`, p.stamps()...)))
}

// Engagement ports Dashboard::EngagementMetrics.
func (p *Period) Engagement() *rb.Map {
	daily := p.groupCount(`SELECT COUNT(DISTINCT "completions"."user_id") AS "count_user_id", "completions"."date_reference" AS "completions_date_reference" FROM "completions" WHERE "completions"."date_reference" `+dateBetween+` GROUP BY "completions"."date_reference"`, p.dates()...)
	active := func(from, to civil.Date) int64 {
		if from == to {
			return p.count(`SELECT COUNT(DISTINCT "completions"."user_id") FROM "completions" WHERE "completions"."date_reference" = $1`, to.ISO())
		}
		return p.count(`SELECT COUNT(DISTINCT "completions"."user_id") FROM "completions" WHERE "completions"."date_reference" `+dateBetween, from.ISO(), to.ISO())
	}
	dau, wau, mau := active(p.End, p.End), active(p.End.Add(-6), p.End), active(p.End.Add(-29), p.End)
	trend := []any{}
	for _, d := range dateRange(p.Start, p.End) {
		n, _ := daily.Get(d.ISO()).(int64)
		trend = append(trend, rb.M("date", d.ISO(), "active_users", n))
	}
	return scopePayload("period", rb.M("dau", dau, "wau", wau, "mau", mau,
		"dau_wau_ratio", percentage(float64(dau), float64(wau)), "wau_mau_ratio", percentage(float64(wau), float64(mau)),
		"daily_active_trend", trend))
}

// Moderation ports Dashboard::ModerationMetrics.
func (p *Period) Moderation() *rb.Map {
	awaiting := `"custom_rosary_prayers"."is_public" = $1 AND "custom_rosary_prayers"."share_status" = $2`
	oldest := p.minTime(`SELECT MIN("custom_rosary_prayers"."created_at") FROM "custom_rosary_prayers" WHERE `+awaiting, true, "pending_review")
	reviewed := `"custom_rosary_prayers"."moderation_decision" IN ($1, $2) AND "custom_rosary_prayers"."reviewed_at" BETWEEN $3 AND $4`
	rargs := append([]any{"approved", "rejected"}, p.stamps()...)
	counts := p.groupCount(`SELECT COUNT(*) AS "count_all", "custom_rosary_prayers"."moderation_decision" AS "custom_rosary_prayers_moderation_decision" FROM "custom_rosary_prayers" WHERE `+reviewed+` GROUP BY "custom_rosary_prayers"."moderation_decision"`, rargs...)
	approved, _ := counts.Get("approved").(int64)
	rejected, _ := counts.Get("rejected").(int64)
	pendingNow := p.count(`SELECT COUNT(*) FROM "custom_rosary_prayers" WHERE `+awaiting, true, "pending_review")
	avg := p.average(`SELECT AVG(EXTRACT(EPOCH FROM (reviewed_at - created_at)))::text FROM "custom_rosary_prayers" WHERE `+reviewed+` AND "custom_rosary_prayers"."reviewed_at" IS NOT NULL`, rargs...)
	rules := `"life_rules"."is_public" = $1 AND "life_rules"."approved" = $2`
	oldestRule := p.minTime(`SELECT MIN("life_rules"."created_at") FROM "life_rules" WHERE `+rules, true, false)
	return scopePayload("period", rb.M(
		"metric_scopes", rb.M("pending_now", "lifetime", "reviews", "period", "reentries", "period", "approved_without_strapi", "lifetime"),
		"custom_rosaries", rb.M(
			"pending_now", pendingNow, "oldest_pending_at", timeOrNil(oldest), "oldest_pending_age_seconds", p.ageSeconds(oldest),
			"approved_in_period", approved, "rejected_in_period", rejected,
			"approval_rate", percentage(float64(approved), float64(approved+rejected)),
			"average_response_time_seconds", avg,
			"reentries_in_period", p.count(`SELECT COUNT(*) FROM "custom_rosary_prayers" WHERE "custom_rosary_prayers"."last_moderation_reentry_at" `+dateBetween, p.stamps()...),
			"total_reentries", p.count(`SELECT SUM("custom_rosary_prayers"."moderation_reentry_count") FROM "custom_rosary_prayers"`),
			"approved_without_strapi", p.count(`SELECT COUNT(*) FROM "custom_rosary_prayers" WHERE "custom_rosary_prayers"."moderation_decision" = $1 AND "custom_rosary_prayers"."publication_status" NOT IN ($2, $3)`, "approved", "published", "unpublished")),
		"life_rules", rb.M(
			"pending_now", p.count(`SELECT COUNT(*) FROM "life_rules" WHERE `+rules, true, false),
			"oldest_pending_at", timeOrNil(oldestRule), "oldest_pending_age_seconds", p.ageSeconds(oldestRule))))
}

// Health ports Dashboard::HealthMetrics.
func (p *Period) Health() *rb.Map {
	now := p.Now
	failures := `"notification_logs"."created_at" BETWEEN $1 AND $2 AND "notification_logs"."sent" = $3`
	fargs := []any{now.Add(-24 * time.Hour), now, false}
	stale := `"audio_generation_sessions"."status" = $1 AND (COALESCE(started_at, created_at) < $2)`
	sargs := []any{"running", now.Add(-time.Hour)}
	sessions := []any{}
	rows := p.rows(`SELECT "audio_generation_sessions"."id", "audio_generation_sessions"."prayer_book_code", "audio_generation_sessions"."started_at" FROM "audio_generation_sessions" WHERE `+stale+` ORDER BY "audio_generation_sessions"."started_at" ASC LIMIT $3`, append(sargs, 20)...)
	for rows.Next() {
		var id int64
		var code *string
		var started *time.Time
		must(rows.Scan(&id, &code, &started))
		sessions = append(sessions, rb.M("id", id, "prayer_book_code", rb.Deref(code), "started_at", timeOrNil(started)))
	}
	rows.Close()
	must(rows.Err())
	expiring := `"api_keys"."active" = $1 AND "api_keys"."expires_at" BETWEEN $2 AND $3`
	eargs := []any{true, now, now.Add(30 * 24 * time.Hour)}
	keys := []any{}
	rows = p.rows(`SELECT "api_keys"."id", "api_keys"."name", "api_keys"."expires_at" FROM "api_keys" WHERE `+expiring+` ORDER BY "api_keys"."expires_at" ASC LIMIT $4`, append(eargs, 20)...)
	for rows.Next() {
		var id int64
		var name string
		var exp *time.Time
		must(rows.Scan(&id, &name, &exp))
		keys = append(keys, rb.M("id", id, "name", name, "expires_at", timeOrNil(exp)))
	}
	rows.Close()
	must(rows.Err())
	return scopePayload("period", rb.M(
		"metric_scopes", rb.M("notifications", "period", "audio_sessions", "lifetime", "api_keys", "lifetime"),
		"notifications", rb.M(
			"failures_last_24_hours", p.count(`SELECT COUNT(*) FROM "notification_logs" WHERE `+failures, fargs...),
			"failed_by_type", p.groupCount(`SELECT COUNT(*) AS "count_all", "notification_logs"."notification_type" AS "notification_logs_notification_type" FROM "notification_logs" WHERE `+failures+` GROUP BY "notification_logs"."notification_type"`, fargs...)),
		"audio_sessions", rb.M(
			"failed", p.count(`SELECT COUNT(*) FROM "audio_generation_sessions" WHERE "audio_generation_sessions"."status" = $1`, "failed"),
			"running", p.count(`SELECT COUNT(*) FROM "audio_generation_sessions" WHERE "audio_generation_sessions"."status" = $1`, "running"),
			"stale_running", p.count(`SELECT COUNT(*) FROM "audio_generation_sessions" WHERE `+stale, sargs...),
			"stale_running_sessions", sessions),
		"api_keys", rb.M(
			"expiring_next_30_days", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE `+expiring, eargs...),
			"expiring_keys", keys)))
}

type userCreated struct {
	id      int64
	created civil.Date
}

// periodUsers ports User.where(created_at: period).pluck(:id, :created_at),
// with each creation as its date in Time.zone.
func (p *Period) periodUsers() []userCreated {
	rows := p.rows(`SELECT "users"."id", "users"."created_at" FROM "users" WHERE "users"."created_at" `+dateBetween, p.stamps()...)
	defer rows.Close()
	var out []userCreated
	for rows.Next() {
		var u userCreated
		var t time.Time
		must(rows.Scan(&u.id, &t))
		u.created = civil.FromTime(t.In(rb.AppZone))
		out = append(out, u)
	}
	must(rows.Err())
	return out
}

// completionDates ports the pluck(:user_id, :date_reference).group_by
// with unique dates per user, keeping first-seen order.
func (p *Period) completionDates(users []userCreated) (map[int64][]civil.Date, []int64) {
	out := map[int64][]civil.Date{}
	var order []int64
	if len(users) == 0 {
		return out, order
	}
	ids := make([]int64, len(users))
	for i, u := range users {
		ids[i] = u.id
	}
	rows := p.rows(`SELECT "completions"."user_id", "completions"."date_reference" FROM "completions" WHERE "completions"."user_id" = ANY($1)`, ids)
	defer rows.Close()
	for rows.Next() {
		var id int64
		var d time.Time
		must(rows.Scan(&id, &d))
		cd := civil.FromTime(d)
		if _, ok := out[id]; !ok {
			order = append(order, id)
		}
		dup := false
		for _, x := range out[id] {
			dup = dup || x == cd
		}
		if !dup {
			out[id] = append(out[id], cd)
		}
	}
	must(rows.Err())
	return out, order
}

func beginningOfWeek(d civil.Date) civil.Date { return d.Add(-((d.Weekday() + 6) % 7)) }

// Retention ports Dashboard::RetentionMetrics.
func (p *Period) Retention() *rb.Map {
	users := p.periodUsers()
	dates, _ := p.completionDates(users)
	var weeks []civil.Date
	cohorts := map[civil.Date][]userCreated{}
	for _, u := range users {
		w := beginningOfWeek(u.created)
		if _, ok := cohorts[w]; !ok {
			weeks = append(weeks, w)
		}
		cohorts[w] = append(cohorts[w], u)
	}
	sortStable(weeks, func(a, b civil.Date) bool { return a < b })
	out := []any{}
	for _, w := range weeks {
		c := cohorts[w]
		entry := rb.M("week_start", w.ISO(), "users", len(c))
		for _, rd := range []struct {
			label string
			days  int
		}{{"d1", 1}, {"d7", 7}, {"d30", 30}} {
			retained := 0
			for _, u := range c {
				for _, d := range dates[u.id] {
					if d == u.created.Add(rd.days) {
						retained++
						break
					}
				}
			}
			entry.Set(rd.label, rb.M("users", retained, "rate", percentage(float64(retained), float64(len(c)))))
		}
		out = append(out, entry)
	}
	return scopePayload("period", rb.M("cohorts", out))
}

// Onboarding ports Dashboard::OnboardingMetrics.
func (p *Period) Onboarding() *rb.Map {
	users := p.periodUsers()
	dates, order := p.completionDates(users)
	created := map[int64]civil.Date{}
	ids := []int64{}
	for _, u := range users {
		created[u.id] = u.created
		ids = append(ids, u.id)
	}
	modes, books, bibles, languages := rb.NewMap(), rb.NewMap(), rb.NewMap(), rb.NewMap()
	completed := 0
	if len(ids) > 0 {
		// The onboardings in their own row order, then their books and Bible
		// versions (includes(:prayer_book, :bible_version)).
		type onboarding struct {
			done        *bool
			mode        *string
			book, bible *int64
		}
		var list []onboarding
		rows := p.rows(`SELECT onboarding_completed, mode, prayer_book_id, bible_version_id FROM "user_onboardings" WHERE "user_onboardings"."user_id" = ANY($1)`, ids)
		for rows.Next() {
			var o onboarding
			must(rows.Scan(&o.done, &o.mode, &o.book, &o.bible))
			list = append(list, o)
		}
		rows.Close()
		must(rows.Err())
		bookCode, bookLang, bibleCode := map[int64]any{}, map[int64]any{}, map[int64]any{}
		rows = p.rows(`SELECT id, code, language FROM prayer_books`)
		for rows.Next() {
			var id int64
			var code, lang *string
			must(rows.Scan(&id, &code, &lang))
			bookCode[id], bookLang[id] = rb.Deref(code), rb.Deref(lang)
		}
		rows.Close()
		must(rows.Err())
		rows = p.rows(`SELECT id, code FROM bible_versions`)
		for rows.Next() {
			var id int64
			var code *string
			must(rows.Scan(&id, &code))
			bibleCode[id] = rb.Deref(code)
		}
		rows.Close()
		must(rows.Err())
		lookup := func(m map[int64]any, id *int64, what string) any {
			if id == nil {
				panic(&rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage(what, nil)})
			}
			v, ok := m[*id]
			if !ok {
				panic(&rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage(what, nil)})
			}
			return v
		}
		for _, o := range list {
			if o.done != nil && *o.done {
				completed++
			}
			inc(modes, hashKey(rb.Deref(o.mode)))
		}
		for _, o := range list {
			inc(books, hashKey(lookup(bookCode, o.book, "code")))
		}
		for _, o := range list {
			inc(bibles, hashKey(lookup(bibleCode, o.bible, "code")))
		}
		for _, o := range list {
			inc(languages, hashKey(lookup(bookLang, o.book, "language")))
		}
	}
	first, seven := 0, 0
	for _, id := range order {
		n := 0
		for _, d := range dates[id] {
			if d >= created[id] {
				n++
			}
		}
		if n > 0 {
			first++
		}
		if n >= 7 {
			seven++
		}
	}
	registered := len(ids)
	funnel := rb.M("registered", registered, "onboarding_completed", completed, "first_prayer", first, "prayed_seven_times", seven)
	rates := rb.NewMap()
	funnel.Each(func(k string, v any) { rates.Set(k, percentage(float64(v.(int)), float64(registered))) })
	funnel.Set("rates", rates)
	return scopePayload("period", rb.M("funnel", funnel,
		"choices", rb.M("modes", modes, "prayer_books", books, "bible_versions", bibles, "languages", languages)))
}

// Premium ports Dashboard::PremiumMetrics.
func (p *Period) Premium() *rb.Map {
	now := p.Now
	expired := map[int64]bool{}
	for _, sql := range []string{
		`SELECT "users"."id" FROM "users" WHERE "users"."premium_expires_at" IS NOT NULL AND (premium_expires_at <= $1)`,
		`SELECT DISTINCT "premium_subscription_events"."user_id" FROM "premium_subscription_events" WHERE "premium_subscription_events"."event_type" = $1`,
	} {
		arg := any(now)
		if strings.Contains(sql, "event_type") {
			arg = "expired"
		}
		rows := p.rows(sql, arg)
		for rows.Next() {
			var id *int64
			must(rows.Scan(&id))
			if id != nil {
				expired[*id] = true
			}
		}
		rows.Close()
		must(rows.Err())
	}
	events := `"premium_subscription_events"."occurred_at" BETWEEN $1 AND $2 AND "premium_subscription_events"."event_type" = $3`
	started := p.count(`SELECT COUNT(*) FROM "premium_subscription_events" WHERE `+events, append(p.stamps(), "started")...)
	renewals := p.count(`SELECT COUNT(*) FROM "premium_subscription_events" WHERE `+events, append(p.stamps(), "renewed")...)
	expiredIn := p.count(`SELECT COUNT(*) FROM "premium_subscription_events" WHERE `+events, append(p.stamps(), "expired")...)
	var exists bool
	must(db.Q().QueryRow(p.Ctx, `SELECT EXISTS(SELECT 1 AS one FROM "premium_subscription_events" LIMIT 1)`).Scan(&exists))
	active := p.count(`SELECT COUNT(*) FROM "users" WHERE (premium_expires_at > $1)`, now)
	newIn := started
	if !exists {
		newIn = p.count(`SELECT COUNT(*) FROM "users" WHERE "users"."created_at" BETWEEN $1 AND $2 AND "users"."premium_expires_at" IS NOT NULL`, p.stamps()...)
	}
	expiring := p.count(`SELECT COUNT(*) FROM "users" WHERE (premium_expires_at > $1) AND (premium_expires_at <= $2)`, now, now.Add(30*24*time.Hour))
	expiredCount := int64(len(expired))
	renewalEvents := renewals + expiredIn
	var rate, note any
	if renewalEvents == 0 {
		note = "No subscription transition events in this period."
	} else {
		rate = percentage(float64(renewals), float64(renewalEvents))
	}
	return scopePayload("period", rb.M(
		"metric_scopes", rb.M("active_now", "lifetime", "expiring_next_30_days", "lifetime", "expired", "lifetime",
			"new_in_period", "period", "subscription_events", "period"),
		"active_now", active, "new_in_period", newIn, "expiring_next_30_days", expiring, "expired", expiredCount,
		"churn_rate", percentage(float64(expiredCount), float64(active+expiredCount)),
		"renewals_in_period", renewals, "expired_in_period", expiredIn, "renewal_rate", rate,
		"renewal_rate_available", renewalEvents > 0, "renewal_rate_note", note))
}

// CustomRosaries ports Dashboard::CustomRosaryMetrics.
func (p *Period) CustomRosaries() *rb.Map {
	where := `"custom_rosary_prayers"."created_at" ` + dateBetween
	prayers, err := rosary.LoadWithStructure(p.Ctx, db.Q(), where, "", p.stamps()...)
	must(err)
	var blocks, steps int64
	for _, pr := range prayers {
		blocks += int64(len(pr.Blocks))
		steps += pr.ExpandedStepCount()
	}
	n := int64(len(prayers))
	avg := func(total int64) float64 {
		if n == 0 {
			return 0
		}
		return rb.RoundFloat(float64(total)/float64(n), 2)
	}
	nearLimit := int64(math.Ceil(float64(rosary.MaxPerUser) * 0.8))
	users := p.count(`SELECT COUNT(*) FROM (SELECT 1 FROM "custom_rosary_prayers" GROUP BY "custom_rosary_prayers"."user_id" HAVING (COUNT(*) >= $1)) t`, nearLimit)
	return scopePayload("period", rb.M(
		"metric_scopes", rb.M("created_in_period", "period", "public_in_period", "period", "by_share_status", "period",
			"by_locale", "period", "average_blocks", "period", "average_expanded_steps", "period", "users_near_limit", "lifetime"),
		"created_in_period", p.count(`SELECT COUNT(*) FROM "custom_rosary_prayers" WHERE `+where, p.stamps()...),
		"public_in_period", p.count(`SELECT COUNT(*) FROM "custom_rosary_prayers" WHERE `+where+` AND "custom_rosary_prayers"."is_public" = $3`, append(p.stamps(), true)...),
		"by_share_status", p.groupCount(`SELECT COUNT(*) AS "count_all", "custom_rosary_prayers"."share_status" AS "custom_rosary_prayers_share_status" FROM "custom_rosary_prayers" WHERE `+where+` GROUP BY "custom_rosary_prayers"."share_status"`, p.stamps()...),
		"by_locale", p.groupCount(`SELECT COUNT(*) AS "count_all", "custom_rosary_prayers"."locale" AS "custom_rosary_prayers_locale" FROM "custom_rosary_prayers" WHERE `+where+` GROUP BY "custom_rosary_prayers"."locale"`, p.stamps()...),
		"average_blocks", avg(blocks), "average_expanded_steps", avg(steps),
		"users_near_limit", users, "near_limit_threshold", nearLimit))
}

// WeeklyPrayers ports Dashboard::WeeklyPrayerMetrics.
func (p *Period) WeeklyPrayers() *rb.Map {
	req := `"prayer_requests"."created_at" ` + dateBetween
	gen := `"weekly_prayers"."created_at" ` + dateBetween
	trunc := "DATE_TRUNC('week', " + localDatetimeSQL("weekly_prayers.created_at") + ")"
	generated := p.count(`SELECT COUNT(*) FROM "weekly_prayers" WHERE `+gen, p.stamps()...)
	return scopePayload("period", rb.M(
		"prayer_requests_created", p.count(`SELECT COUNT(*) FROM "prayer_requests" WHERE `+req, p.stamps()...),
		"prayer_request_users", p.count(`SELECT COUNT(DISTINCT "prayer_requests"."user_id") FROM "prayer_requests" WHERE `+req, p.stamps()...),
		"prayer_requests_by_week", p.groupCount(`SELECT COUNT(*) AS "count_all", "prayer_requests"."week_start" AS "prayer_requests_week_start" FROM "prayer_requests" WHERE `+req+` GROUP BY "prayer_requests"."week_start"`, p.stamps()...),
		"weekly_prayers_generated", generated,
		"weekly_prayer_users", p.count(`SELECT COUNT(DISTINCT "weekly_prayers"."user_id") FROM "weekly_prayers" WHERE `+gen, p.stamps()...),
		"generated_by_week", p.groupCount(`SELECT COUNT(*) AS "count_all", `+trunc+` AS "date_trunc_week_weekly_prayers_created_at_at_time_zone_utc_at_t" FROM "weekly_prayers" WHERE `+gen+` GROUP BY `+trunc, p.stamps()...),
		"generated_for_week", p.groupCount(`SELECT COUNT(*) AS "count_all", "weekly_prayers"."week_start" AS "weekly_prayers_week_start" FROM "weekly_prayers" WHERE `+gen+` GROUP BY "weekly_prayers"."week_start"`, p.stamps()...),
		"generated_by_language", p.groupCount(`SELECT COUNT(*) AS "count_all", "weekly_prayers"."language" AS "weekly_prayers_language" FROM "weekly_prayers" WHERE `+gen+` GROUP BY "weekly_prayers"."language"`, p.stamps()...),
		"generated_by_prayer_book", p.groupCount(`SELECT COUNT(*) AS "count_all", "weekly_prayers"."prayer_book_code" AS "weekly_prayers_prayer_book_code" FROM "weekly_prayers" WHERE `+gen+` GROUP BY "weekly_prayers"."prayer_book_code"`, p.stamps()...),
		"daily_generated", p.dailyList(`SELECT COUNT(*) AS "count_all", `+localDateSQL("weekly_prayers.created_at")+` AS "date_weekly_prayers_created_at_at_time_zone_utc_at_time_zone_am" FROM "weekly_prayers" WHERE `+gen+` GROUP BY `+localDateSQL("weekly_prayers.created_at"), "count", p.stamps()...),
		"perplexity_usage_proxy", generated))
}

// Favorites ports Dashboard::FavoriteMetrics.
func (p *Period) Favorites() *rb.Map {
	where := `"user_favorites"."created_at" ` + dateBetween
	top := []any{}
	for _, r := range p.groupRows(`SELECT COUNT(*) AS "count_all", "user_favorites"."post_slug" AS "user_favorites_post_slug" FROM "user_favorites" WHERE `+where+` GROUP BY "user_favorites"."post_slug" ORDER BY COUNT(*) DESC, "user_favorites"."post_slug" ASC LIMIT $3`, 1, append(p.stamps(), 20)...) {
		top = append(top, rb.M("post_slug", r.Keys[0], "favorites", r.Count))
	}
	return scopePayload("period", rb.M(
		"created_in_period", p.count(`SELECT COUNT(*) FROM "user_favorites" WHERE `+where, p.stamps()...),
		"users_with_favorites", p.count(`SELECT COUNT(DISTINCT "user_favorites"."user_id") FROM "user_favorites" WHERE `+where, p.stamps()...),
		"by_kind", p.groupCount(`SELECT COUNT(*) AS "count_all", "user_favorites"."kind" AS "user_favorites_kind" FROM "user_favorites" WHERE `+where+` GROUP BY "user_favorites"."kind"`, p.stamps()...),
		"top_posts", top))
}

func (p *Period) sums(sql string, args ...any) []pair {
	return p.groupRows(sql, 1, args...)
}

// API ports Dashboard::ApiMetrics.
func (p *Period) API() *rb.Map {
	where := `"api_key_usage_logs"."date" ` + dateBetween
	daily := p.sums(`SELECT SUM("api_key_usage_logs"."requests_count") AS "sum_requests_count", "api_key_usage_logs"."date" AS "api_key_usage_logs_date" FROM "api_key_usage_logs" WHERE `+where+` GROUP BY "api_key_usage_logs"."date"`, p.dates()...)
	endpoints := p.sums(`SELECT SUM("api_key_usage_logs"."requests_count") AS "sum_requests_count", "api_key_usage_logs"."endpoint" AS "api_key_usage_logs_endpoint" FROM "api_key_usage_logs" WHERE `+where+` GROUP BY "api_key_usage_logs"."endpoint"`, p.dates()...)
	keyUsage := p.sums(`SELECT SUM("api_key_usage_logs"."requests_count") AS "sum_requests_count", "api_key_usage_logs"."api_key_id" AS "api_key_usage_logs_api_key_id" FROM "api_key_usage_logs" WHERE `+where+` GROUP BY "api_key_usage_logs"."api_key_id"`, p.dates()...)
	type keyRow struct {
		name   string
		active bool
	}
	keys := map[int64]keyRow{}
	loadKeys := func(ids []int64) {
		if len(ids) == 0 {
			return
		}
		rows := p.rows(`SELECT "api_keys"."id", "api_keys"."name", "api_keys"."active" FROM "api_keys" WHERE "api_keys"."id" = ANY($1)`, ids)
		for rows.Next() {
			var id int64
			var k keyRow
			must(rows.Scan(&id, &k.name, &k.active))
			keys[id] = k
		}
		rows.Close()
		must(rows.Err())
	}
	var ids []int64
	for _, r := range keyUsage {
		ids = append(ids, toI64(r.Keys[0]))
	}
	loadKeys(ids)
	limit := int64(1000)
	if v := strings.TrimSpace(os.Getenv("API_KEY_DAILY_LIMIT")); v != "" {
		if n := int64(rb.StringToI(v)); n > 0 {
			limit = n
		}
	}
	today := civil.FromTime(time.Now().In(rb.AppZone))
	todayUsage := p.sums(`SELECT SUM("api_key_usage_logs"."requests_count") AS "sum_requests_count", "api_key_usage_logs"."api_key_id" AS "api_key_usage_logs_api_key_id" FROM "api_key_usage_logs" WHERE "api_key_usage_logs"."date" = $1 GROUP BY "api_key_usage_logs"."api_key_id"`, today.ISO())
	sortStable(daily, func(a, b pair) bool { return hashKey(a.Keys[0]) < hashKey(b.Keys[0]) })
	byDay := []any{}
	for _, r := range daily {
		byDay = append(byDay, rb.M("date", hashKey(r.Keys[0]), "requests", r.Count))
	}
	sortStable(endpoints, func(a, b pair) bool {
		return a.Count > b.Count || a.Count == b.Count && rb.ToS(a.Keys[0]) < rb.ToS(b.Keys[0])
	})
	topEndpoints := []any{}
	for i, r := range endpoints {
		if i == 20 {
			break
		}
		topEndpoints = append(topEndpoints, rb.M("endpoint", r.Keys[0], "requests", r.Count))
	}
	sortStable(keyUsage, func(a, b pair) bool {
		return a.Count > b.Count || a.Count == b.Count && toI64(a.Keys[0]) < toI64(b.Keys[0])
	})
	topKeys := []any{}
	for i, r := range keyUsage {
		if i == 20 {
			break
		}
		id := toI64(r.Keys[0])
		var name, active any
		if k, ok := keys[id]; ok {
			name, active = k.name, k.active
		}
		topKeys = append(topKeys, rb.M("id", id, "name", name, "requests", r.Count, "active", active))
	}
	threshold := int64(math.Ceil(float64(limit) * 0.8))
	near := []any{}
	for _, r := range todayUsage {
		if r.Count < threshold {
			continue
		}
		id := toI64(r.Keys[0])
		if _, ok := keys[id]; !ok {
			loadKeys([]int64{id})
		}
		var name any
		if k, ok := keys[id]; ok {
			name = k.name
		}
		near = append(near, rb.M("id", id, "name", name, "requests_today", r.Count, "daily_limit", limit))
	}
	return scopePayload("period", rb.M(
		"requests_in_period", p.count(`SELECT SUM("api_key_usage_logs"."requests_count") FROM "api_key_usage_logs" WHERE `+where, p.dates()...),
		"requests_by_day", byDay, "top_endpoints", topEndpoints, "top_keys", topKeys,
		"keys", rb.M(
			"active", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE "api_keys"."active" = $1 AND "api_keys"."billing_active" = $2 AND (expires_at IS NULL OR expires_at > $3)`, true, true, p.Now),
			"inactive", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE "api_keys"."active" = $1`, false),
			"expired", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE "api_keys"."expires_at" IS NOT NULL AND (expires_at <= $1)`, p.Now)),
		"near_daily_limit", near))
}

func toI64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	}
	n, _ := strconv.ParseInt(rb.ToS(v), 10, 64)
	return n
}

// Developers ports Dashboard::DeveloperMetrics.
func (p *Period) Developers() *rb.Map {
	rows := p.groupRows(`SELECT COUNT("api_keys"."id") AS "count_api_keys_id", "developers"."id" AS "developers_id", "developers"."name" AS "developers_name" FROM "developers" LEFT OUTER JOIN "api_keys" ON "api_keys"."developer_id" = "developers"."id" GROUP BY "developers"."id", "developers"."name"`, 2)
	sortStable(rows, func(a, b pair) bool { return a.Count > b.Count })
	byDev := []any{}
	for i, r := range rows {
		if i == 20 {
			break
		}
		byDev = append(byDev, rb.M("id", toI64(r.Keys[0]), "name", r.Keys[1], "keys", r.Count))
	}
	return scopePayload("lifetime", rb.M(
		"registered", p.count(`SELECT COUNT(*) FROM "developers"`),
		"approved", p.count(`SELECT COUNT(*) FROM "developers" WHERE "developers"."approved" = $1`, true),
		"pending_approval", p.count(`SELECT COUNT(*) FROM "developers" WHERE "developers"."approved" = $1`, false),
		"with_api_keys", p.count(`SELECT COUNT(DISTINCT "developers"."id") FROM "developers" INNER JOIN "api_keys" ON "api_keys"."developer_id" = "developers"."id"`),
		"api_keys", rb.M(
			"total", p.count(`SELECT COUNT(*) FROM "api_keys"`),
			"active", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE "api_keys"."active" = $1`, true),
			"inactive", p.count(`SELECT COUNT(*) FROM "api_keys" WHERE "api_keys"."active" = $1`, false)),
		"keys_by_developer", byDev))
}
