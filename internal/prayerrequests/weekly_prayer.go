package prayerrequests

import (
	"context"
	_ "embed"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/perplexity"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// generic_collects.yml is a copy of the Rails config/generic_collects.yml.
//
//go:embed generic_collects.yml
var genericCollectsYAML []byte

const collectsCount = 20

var (
	collectsOnce sync.Once
	collects     map[string][]string
)

func genericCollects() map[string][]string {
	collectsOnce.Do(func() {
		collects = map[string][]string{}
		if err := yaml.Unmarshal(genericCollectsYAML, &collects); err != nil {
			panic(err)
		}
	})
	return collects
}

func languageKey(language string) string {
	switch {
	case strings.HasPrefix(language, "pt"):
		return "pt-BR"
	case strings.HasPrefix(language, "es"):
		return "es"
	}
	return "en"
}

func localized(language, pt, es, en string) string {
	switch {
	case strings.HasPrefix(language, "pt"):
		return pt
	case strings.HasPrefix(language, "es"):
		return es
	}
	return en
}

type weeklyPrayer struct {
	id              int64
	generatedPrayer string
	digest          string
	generatedAt     time.Time
}

// Generator ports WeeklyPrayerGeneratorService.
type Generator struct {
	User           *users.User
	WeekStart      rb.YMD
	PrayerBookCode string
}

func (g *Generator) language(ctx context.Context) string {
	pb, err := store.PrayerBookByCode(ctx, g.PrayerBookCode)
	must(err)
	if pb != nil {
		return pb.Language
	}
	return "pt-BR"
}

// Call ports #call: nil when the week has no requests; a *perplexity.APIError
// when the summary cannot be produced or fails validation.
func (g *Generator) Call(ctx context.Context) (*rb.Map, error) {
	requests := ForWeek(ctx, g.User.ID, g.WeekStart)
	if len(requests) == 0 {
		return nil, nil
	}
	digest := Digest(ctx, g.User.ID, g.WeekStart)
	existing := g.findExisting(ctx)
	if existing != nil && existing.digest == digest {
		return g.format(ctx, existing, len(requests))
	}
	input := make([]perplexity.Request, len(requests))
	for i, r := range requests {
		input[i] = perplexity.Request{Title: rb.ToS(rb.Deref(r.Title)), Content: r.Content}
	}
	summary, err := perplexity.Summarize(ctx, input, g.language(ctx))
	if err != nil {
		return nil, err
	}
	wp := g.save(ctx, summary, digest)
	return g.format(ctx, wp, len(requests))
}

func (g *Generator) findExisting(ctx context.Context) *weeklyPrayer {
	var wp weeklyPrayer
	err := db.Q().QueryRow(ctx, `SELECT id, generated_prayer, requests_digest, generated_at FROM weekly_prayers
		WHERE user_id = $1 AND week_start = $2 AND prayer_book_code = $3 ORDER BY id ASC LIMIT 1`,
		g.User.ID, g.WeekStart.ISO(), g.PrayerBookCode).Scan(&wp.id, &wp.generatedPrayer, &wp.digest, &wp.generatedAt)
	if db.NoRows(err) {
		return nil
	}
	must(err)
	return &wp
}

// save ports save_weekly_prayer (find_or_initialize_by + update!).
func (g *Generator) save(ctx context.Context, summary, digest string) *weeklyPrayer {
	language := g.language(ctx)
	now := users.Now()
	var msgs []string
	if rb.BlankString(g.PrayerBookCode) {
		msgs = append(msgs, "Prayer book code can't be blank")
	}
	if rb.BlankString(language) {
		msgs = append(msgs, "Language can't be blank")
	}
	if len(msgs) > 0 {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: " + strings.Join(msgs, ", ")})
	}
	wp := &weeklyPrayer{generatedPrayer: summary, digest: digest, generatedAt: now}
	existing := g.findExisting(ctx)
	if existing == nil {
		must(db.Q().QueryRow(ctx, `INSERT INTO weekly_prayers (user_id, week_start, prayer_book_code, generated_prayer, requests_digest,
			language, generated_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7) RETURNING id`,
			g.User.ID, g.WeekStart.ISO(), g.PrayerBookCode, summary, digest, language, now).Scan(&wp.id))
		return wp
	}
	wp.id = existing.id
	_, err := db.Q().Exec(ctx, `UPDATE weekly_prayers SET generated_prayer = $1, requests_digest = $2, language = $3,
		generated_at = $4, updated_at = $4 WHERE id = $5`, summary, digest, language, now, existing.id)
	must(err)
	return wp
}

func (g *Generator) format(ctx context.Context, wp *weeklyPrayer, count int) (*rb.Map, error) {
	generated, err := perplexity.ValidateSummary(wp.generatedPrayer, count)
	if err != nil {
		return nil, err
	}
	return rb.M(
		"week_start", g.WeekStart.ISO(),
		"week_end", g.WeekStart.AddDays(6).ISO(),
		"prayer_requests_count", count,
		"generated_at", rb.FormatTime(wp.generatedAt),
		"prayer_book_code", g.PrayerBookCode,
		"module", g.module(ctx, generated),
		"placement", g.placement(ctx),
	), nil
}

func (g *Generator) module(ctx context.Context, generated string) *rb.Map {
	language := g.language(ctx)
	heading := localized(language, "Pedidos de Oração", "Peticiones de Oración", "Prayer Requests")
	lines := []any{
		rb.M("text", heading, "type", "heading"),
		rb.M("text", g.collect(language), "type", "leader"),
		rb.M("text", "", "type", "spacer"),
		rb.M("text", localized(language, "Pedidos desta semana:", "Peticiones de esta semana:", "Requests for this week:"), "type", "subheading"),
	}
	for _, l := range strings.Split(generated, "\n") {
		if l = strings.Trim(l, " \t\n\v\f\r\x00"); !rb.BlankString(l) {
			lines = append(lines, rb.M("text", l, "type", "text"))
		}
	}
	lines = append(lines, rb.M("text", "", "type", "spacer"),
		rb.M("text", localized(language, "Amém.", "Amén.", "Amen."), "type", "congregation"))
	return rb.M("name", heading, "slug", "weekly_prayer_requests", "lines", lines)
}

// collect ports generic_collect_for_week.
func (g *Generator) collect(language string) any {
	list := genericCollects()[languageKey(language)]
	if len(list) == 0 {
		return nil
	}
	i := int(g.WeekStart.Cweek() % collectsCount)
	if i < len(list) {
		return list[i]
	}
	return list[0]
}

func (g *Generator) placement(ctx context.Context) any {
	pb, err := store.PrayerBookByCode(ctx, g.PrayerBookCode)
	must(err)
	if pb == nil {
		return books.DefaultPrayerRequestsPlacement()
	}
	return books.For(pb.Code, pb.Features).PrayerRequestsPlacement()
}
