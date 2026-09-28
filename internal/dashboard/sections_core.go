package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

const dateBetween = `BETWEEN $1 AND $2`

func (p *Period) dates() []any { return []any{p.Start.ISO(), p.End.ISO()} }
func (p *Period) stamps() []any {
	return []any{p.TSStart().Truncate(time.Microsecond), p.TSEnd().Truncate(time.Microsecond)}
}

// Overview ports Dashboard::OverviewMetrics.
func (p *Period) Overview() *rb.Map {
	total := p.count(`SELECT COUNT(*) FROM "users"`)
	premium := p.count(`SELECT COUNT(*) FROM "users" WHERE (premium_expires_at > $1)`, p.Now)
	completions := p.count(`SELECT COUNT(*) FROM "completions"`)
	completing := p.count(`SELECT COUNT(DISTINCT "completions"."user_id") FROM "completions"`)
	avg := 0.0
	if completing != 0 {
		avg = rb.RoundFloat(float64(completions)/float64(completing), 2)
	}
	return scopePayload("lifetime", rb.M(
		"total_users", total, "premium_users", premium, "free_users", total-premium,
		"premium_conversion_rate", percentage(float64(premium), float64(total)),
		"total_completions", completions,
		"total_journals", p.count(`SELECT COUNT(*) FROM "journals"`),
		"users_with_completions", completing, "avg_completions_per_user", avg,
		"avg_completions_denominator", "users_with_completions"))
}

// Users ports Dashboard::UserMetrics.
func (p *Period) Users() *rb.Map {
	var active int64
	rows := p.rows(`SELECT DISTINCT "completions"."user_id" FROM "completions" WHERE "completions"."date_reference" `+dateBetween, p.dates()...)
	for rows.Next() {
		active++
	}
	rows.Close()
	must(rows.Err())
	newUsers := p.count(`SELECT COUNT(*) FROM "users" WHERE "users"."created_at" `+dateBetween, p.stamps()...)
	daily := p.groupCount(`SELECT COUNT(*) AS "count_all", `+localDateSQL("users.created_at")+` AS "date_users_created_at_at_time_zone_utc_at_time_zone_america_sao" FROM "users" WHERE "users"."created_at" `+dateBetween+` GROUP BY `+localDateSQL("users.created_at"), p.stamps()...)
	cutoff := p.Now.Add(-60 * 24 * time.Hour)
	tokens := p.count(`SELECT COUNT(*) FROM "fcm_tokens" WHERE (updated_at > $1)`, cutoff)
	platforms := rb.NewMap()
	for _, r := range p.groupRows(`SELECT COUNT(*) AS "count_all", "fcm_tokens"."platform" AS "fcm_tokens_platform" FROM "fcm_tokens" WHERE (updated_at > $1) GROUP BY "fcm_tokens"."platform"`, 1, cutoff) {
		key := "unknown"
		if s, ok := r.Keys[0].(string); ok && !rb.BlankString(s) {
			key = s
		}
		platforms.Set(key, r.Count)
	}
	var streakers []any
	rows = p.rows(`SELECT "users"."id", "users"."name", "users"."longest_streak", "users"."current_streak" FROM "users" ORDER BY "users"."longest_streak" DESC, "users"."id" ASC LIMIT $1`, 10)
	for rows.Next() {
		var id int64
		var name *string
		var longest, current *int64
		must(rows.Scan(&id, &name, &longest, &current))
		streakers = append(streakers, rb.M("id", id, "name", rb.Deref(name), "longest_streak", rb.Deref(longest), "current_streak", rb.Deref(current)))
	}
	rows.Close()
	must(rows.Err())
	if streakers == nil {
		streakers = []any{}
	}
	return scopePayload("period", rb.M(
		"metric_scopes", rb.M("new_users_in_period", "period", "daily_new_users", "period", "active_users_in_period", "period",
			"active_fcm_tokens", "lifetime", "platform_breakdown", "lifetime", "top_streakers", "lifetime",
			"avg_current_streak", "lifetime", "avg_longest_streak", "lifetime"),
		"new_users_in_period", newUsers, "daily_new_users", daily, "active_users_in_period", active,
		"active_fcm_tokens", tokens, "platform_breakdown", platforms, "top_streakers", streakers,
		"avg_current_streak", p.average(`SELECT AVG("users"."current_streak")::text FROM "users"`),
		"avg_longest_streak", p.average(`SELECT AVG("users"."longest_streak")::text FROM "users"`)))
}

type bookCount struct {
	code, name any
	n          int64
}

func (p *Period) bookCounts(sql string, args ...any) []bookCount {
	var out []bookCount
	for _, r := range p.groupRows(sql, 2, args...) {
		out = append(out, bookCount{r.Keys[0], r.Keys[1], r.Count})
	}
	sortStable(out, func(a, b bookCount) bool { return a.n > b.n })
	return out
}

func bookEntries(list []bookCount, countKey string) []any {
	out := []any{}
	for _, b := range list {
		out = append(out, rb.M("code", b.code, "name", b.name, countKey, b.n))
	}
	return out
}

// topByCount ports the top_completers / top_writers queries.
func (p *Period) topByCount(sql, countKey string, args ...any) []any {
	rows := p.rows(sql, args...)
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, n int64
		var name *string
		must(rows.Scan(&id, &name, &n))
		out = append(out, rb.M("id", id, "name", rb.Deref(name), countKey, n))
	}
	must(rows.Err())
	return out
}

// dailyList ports `group(date).count.sort.to_h.map { {date:, count:} }`.
func (p *Period) dailyList(sql, countKey string, args ...any) []any {
	var rowsOut []pair
	rowsOut = p.groupRows(sql, 1, args...)
	sortStable(rowsOut, func(a, b pair) bool { return hashKey(a.Keys[0]) < hashKey(b.Keys[0]) })
	out := []any{}
	for _, r := range rowsOut {
		out = append(out, rb.M("date", hashKey(r.Keys[0]), countKey, r.Count))
	}
	return out
}

// Completions ports Dashboard::CompletionMetrics.
func (p *Period) Completions() *rb.Map {
	where := `"completions"."date_reference" ` + dateBetween
	total := p.count(`SELECT COUNT(*) FROM "completions" WHERE `+where, p.dates()...)
	byOffice := p.groupCount(`SELECT COUNT(*) AS "count_all", "completions"."office_type" AS "completions_office_type" FROM "completions" WHERE `+where+` GROUP BY "completions"."office_type"`, p.dates()...)
	byHour := rb.NewMap()
	for _, r := range p.groupRows(`SELECT COUNT(*) AS "count_all", EXTRACT(HOUR FROM (completions.created_at AT TIME ZONE 'UTC' AT TIME ZONE users.timezone))::text AS "extract_hour_from_completions_created_at_at_time_zone_utc_at_ti" FROM "completions" INNER JOIN "users" ON "users"."id" = "completions"."user_id" WHERE `+where+` GROUP BY EXTRACT(HOUR FROM (completions.created_at AT TIME ZONE 'UTC' AT TIME ZONE users.timezone))`, 1, p.dates()...) {
		key := ""
		if s, ok := r.Keys[0].(string); ok {
			key = rb.ToS(rb.StringToI(s))
		}
		byHour.Set(key, r.Count)
	}
	top := p.topByCount(`SELECT users.id, users.name, COUNT(completions.id) AS completion_count FROM "users" INNER JOIN "completions" ON "completions"."user_id" = "users"."id" WHERE "completions"."id" IN (SELECT "completions"."id" FROM "completions" WHERE `+where+`) GROUP BY "users"."id", "users"."name" ORDER BY completion_count DESC, users.id ASC LIMIT $3`, "completions", p.Start.ISO(), p.End.ISO(), 10)
	daily := p.dailyList(`SELECT COUNT(*) AS "count_all", "completions"."date_reference" AS "completions_date_reference" FROM "completions" WHERE `+where+` GROUP BY "completions"."date_reference"`, "count", p.dates()...)
	avg := p.average(`SELECT AVG("completions"."duration_seconds")::text FROM "completions" WHERE `+where+` AND "completions"."duration_seconds" IS NOT NULL`, p.dates()...)
	books := p.bookCounts(`SELECT COUNT(*) AS "count_all", "prayer_books"."code" AS "prayer_books_code", "prayer_books"."name" AS "prayer_books_name" FROM "completions" INNER JOIN "prayer_books" ON "prayer_books"."id" = "completions"."prayer_book_id" WHERE `+where+` GROUP BY "prayer_books"."code", "prayer_books"."name"`, p.dates()...)
	return scopePayload("period", rb.M("total_in_period", total, "by_office_type", byOffice, "by_hour", byHour,
		"top_completers", top, "daily_completions", daily, "avg_duration_seconds", avg,
		"by_prayer_book", bookEntries(books, "completions")))
}

// PrayerBooks ports Dashboard::PrayerBookMetrics.
func (p *Period) PrayerBooks() *rb.Map {
	onboarding := bookEntries(p.bookCounts(`SELECT COUNT(*) AS "count_all", "prayer_books"."code" AS "prayer_books_code", "prayer_books"."name" AS "prayer_books_name" FROM "user_onboardings" INNER JOIN "prayer_books" ON "prayer_books"."id" = "user_onboardings"."prayer_book_id" GROUP BY "prayer_books"."code", "prayer_books"."name"`), "users")
	completion := bookEntries(p.bookCounts(`SELECT COUNT(*) AS "count_all", "prayer_books"."code" AS "prayer_books_code", "prayer_books"."name" AS "prayer_books_name" FROM "completions" INNER JOIN "prayer_books" ON "prayer_books"."id" = "completions"."prayer_book_id" GROUP BY "prayer_books"."code", "prayer_books"."name"`), "completions")
	bibles := bookEntries(p.bookCounts(`SELECT COUNT(*) AS "count_all", "bible_versions"."code" AS "bible_versions_code", "bible_versions"."name" AS "bible_versions_name" FROM "user_onboardings" INNER JOIN "bible_versions" ON "bible_versions"."id" = "user_onboardings"."bible_version_id" GROUP BY "bible_versions"."code", "bible_versions"."name"`), "users")
	first := func(l []any) any {
		if len(l) == 0 {
			return nil
		}
		return l[0]
	}
	return scopePayload("lifetime", rb.M("usage_by_prayer_book", onboarding, "onboarding_choices_by_prayer_book", onboarding,
		"completion_usage_by_prayer_book", completion, "most_used", first(onboarding),
		"most_used_by_completions", first(completion), "bible_version_choices", bibles,
		"unattributed_completions", p.count(`SELECT COUNT(*) FROM "completions" WHERE "completions"."prayer_book_id" IS NULL`)))
}

// Journals ports Dashboard::JournalMetrics.
func (p *Period) Journals() *rb.Map {
	where := `"journals"."date_reference" ` + dateBetween
	return scopePayload("period", rb.M(
		"total_in_period", p.count(`SELECT COUNT(*) FROM "journals" WHERE `+where, p.dates()...),
		"by_entry_type", p.groupCount(`SELECT COUNT(*) AS "count_all", "journals"."entry_type" AS "journals_entry_type" FROM "journals" WHERE `+where+` GROUP BY "journals"."entry_type"`, p.dates()...),
		"by_office_type", p.groupCount(`SELECT COUNT(*) AS "count_all", "journals"."office_type" AS "journals_office_type" FROM "journals" WHERE `+where+` AND "journals"."office_type" IS NOT NULL GROUP BY "journals"."office_type"`, p.dates()...),
		"top_writers", p.topByCount(`SELECT users.id, users.name, COUNT(journals.id) AS journal_count FROM "users" INNER JOIN "journals" ON "journals"."user_id" = "users"."id" WHERE "journals"."id" IN (SELECT "journals"."id" FROM "journals" WHERE `+where+`) GROUP BY "users"."id", "users"."name" ORDER BY journal_count DESC, users.id ASC LIMIT $3`, "journals", p.Start.ISO(), p.End.ISO(), 10),
		"users_with_journals", p.count(`SELECT COUNT(DISTINCT "journals"."user_id") FROM "journals" WHERE `+where, p.dates()...)))
}

// Notifications ports Dashboard::NotificationMetrics.
func (p *Period) Notifications() *rb.Map {
	platforms := map[string]string{}
	rows := p.rows(`SELECT "fcm_tokens"."token", "fcm_tokens"."platform" FROM "fcm_tokens"`)
	for rows.Next() {
		var token string
		var platform *string
		must(rows.Scan(&token, &platform))
		sum := sha256.Sum256([]byte(token))
		pl := "unknown"
		if platform != nil && !rb.BlankString(*platform) {
			pl = *platform
		}
		platforms[hex.EncodeToString(sum[:])] = pl
	}
	rows.Close()
	must(rows.Err())
	statuses := rb.NewMap()
	byPlatform := rb.NewMap()
	rows = p.rows(`SELECT "notification_logs"."delivery_status"::text FROM "notification_logs" WHERE "notification_logs"."created_at" BETWEEN $1 AND $2 AND "notification_logs"."delivery_status" != $3`,
		append(p.stamps(), "{}")...)
	for rows.Next() {
		var raw string
		must(rows.Scan(&raw))
		parsed, err := rb.ParseJSON([]byte(raw))
		must(err)
		m, ok := parsed.(*rb.Map)
		if !ok {
			continue
		}
		m.Each(func(tokenHash string, v any) {
			status := rb.ToS(v)
			pl, ok := platforms[tokenHash]
			if !ok {
				pl = "unknown"
			}
			inc(statuses, status)
			inner, _ := byPlatform.Get(pl).(*rb.Map)
			if inner == nil {
				inner = rb.NewMap()
				byPlatform.Set(pl, inner)
			}
			inc(inner, status)
		})
	}
	rows.Close()
	must(rows.Err())
	where := `"notification_logs"."created_at" BETWEEN $1 AND $2`
	total := p.count(`SELECT COUNT(*) FROM "notification_logs" WHERE `+where, p.stamps()...)
	sent := p.count(`SELECT COUNT(*) FROM "notification_logs" WHERE `+where+` AND "notification_logs"."sent" = $3`, append(p.stamps(), true)...)
	failed := p.count(`SELECT COUNT(*) FROM "notification_logs" WHERE `+where+` AND "notification_logs"."sent" = $3`, append(p.stamps(), false)...)
	return scopePayload("period", rb.M("total_in_period", total, "sent", sent, "failed", failed,
		"success_rate", percentage(float64(sent), float64(total)),
		"by_type", p.groupCount(`SELECT COUNT(*) AS "count_all", "notification_logs"."notification_type" AS "notification_logs_notification_type" FROM "notification_logs" WHERE `+where+` GROUP BY "notification_logs"."notification_type"`, p.stamps()...),
		"failures_last_24_hours", p.count(`SELECT COUNT(*) FROM "notification_logs" WHERE `+where+` AND "notification_logs"."sent" = $3`, p.Now.Add(-24*time.Hour), p.Now, false),
		"delivery_status_counts", statuses, "delivery_status_by_platform", byPlatform))
}

func inc(m *rb.Map, key string) {
	n, _ := m.Get(key).(int64)
	m.Set(key, n+1)
}
