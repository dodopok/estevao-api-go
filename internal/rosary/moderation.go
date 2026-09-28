package rosary

import (
	"context"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
)

func equalSelection(a, b *rb.Map) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return users.Equal(a, b)
}

// saveLocked ports update! inside with_lock: callbacks and validations,
// then the prayer's columns; the after_commit callbacks run after the
// transaction.
func (p *Prayer) saveLocked(ctx context.Context, tx pgx.Tx) (map[string]any, error) {
	now := users.Now()
	p.beforeValidation(now)
	if errs := p.Validate(ctx, tx, false); errs.Any() {
		return nil, &ar.RecordInvalid{Errors: errs}
	}
	before := map[string]any{}
	for _, c := range []string{"moderation_decision", "reviewed_at", "publication_status"} {
		before[c] = p.Orig[c]
	}
	if _, err := p.Update(ctx, tx, now); err != nil {
		return nil, err
	}
	return before, nil
}

// Approve ports CustomRosaryPrayers::Moderate.approve. category is the
// params[:category] value.
func Approve(ctx context.Context, id int64, category any, strapiSlug any) error {
	selection, err := CategorySelection(category)
	if err != nil {
		return err
	}
	var before map[string]any
	var locked *Prayer
	err = withLock(ctx, id, func(tx pgx.Tx, p *Prayer) error {
		wasApproved := p.approved()
		var previous *rb.Map
		if c := p.Get("publication_category"); !ar.Blank(c) {
			prev, err := CategorySelection(c)
			if err != nil {
				return err
			}
			previous = prev
		}
		categoryChanged := !equalSelection(previous, selection)
		slugChanged := !ar.Blank(strapiSlug) && ar.CastString(strapiSlug) != p.Get("strapi_slug")
		revision := p.Get("updated_at").(time.Time).UTC().Format("2006-01-02T15:04:05.000000Z07:00")
		shouldEnqueue := !wasApproved || categoryChanged || slugChanged ||
			ar.Included(p.Get("publication_status"), []string{"pending", "failed", "unpublishing", "unpublished"})
		status := p.Get("publication_status")
		switch {
		case shouldEnqueue && status == "unpublishing" && p.Get("publication_started_at") != nil:
		case shouldEnqueue:
			status = "pending"
		}
		slug := p.Get("strapi_slug")
		if !ar.Blank(strapiSlug) {
			slug = ar.CastString(strapiSlug)
		}
		p.Set("moderation_decision", "approved")
		p.Set("moderation_note", nil)
		p.Set("strapi_slug", slug)
		p.Set("publication_category", selection)
		p.Set("source_revision", revision)
		p.Set("publication_status", status)
		if shouldEnqueue {
			p.Set("publication_error", nil)
			p.Set("publication_started_at", nil)
			p.Set("publication_retry_at", nil)
			p.Set("publication_attempts", int64(0))
		}
		p.Set("reviewed_at", users.Now())
		b, err := p.saveLocked(ctx, tx)
		before, locked = b, p
		return err
	})
	if err != nil {
		return err
	}
	locked.afterCommitUpdate(ctx, before)
	return nil
}

// Reject ports CustomRosaryPrayers::Moderate.reject.
func Reject(ctx context.Context, id int64, reason any) error {
	var before map[string]any
	var locked *Prayer
	err := withLock(ctx, id, func(tx pgx.Tx, p *Prayer) error {
		unpublish := !ar.Blank(p.Get("strapi_document_id")) &&
			ar.Included(p.Get("publication_status"), []string{"published", "failed", "publishing", "pending", "unpublishing"})
		p.Set("moderation_decision", "rejected")
		if ar.Blank(reason) {
			p.Set("moderation_note", nil)
		} else {
			p.Set("moderation_note", ar.CastString(reason))
		}
		if unpublish {
			p.Set("publication_status", "unpublishing")
			p.Set("publication_error", nil)
			p.Set("publication_retry_at", nil)
		}
		p.Set("reviewed_at", users.Now())
		b, err := p.saveLocked(ctx, tx)
		before, locked = b, p
		return err
	})
	if err != nil {
		return err
	}
	locked.afterCommitUpdate(ctx, before)
	return nil
}

// Author is the prayer's user as the moderation panel shows it.
type Author struct {
	ID          *int64
	Name, Email any
}

// ModerationJSON ports CustomRosaryPrayerModerationSerializer (expanded adds
// expanded_steps, as #as_json does; the list uses #list_as_json).
func (p *Prayer) ModerationJSON(author Author, expanded bool) (*rb.Map, error) {
	var category any
	if c := p.Get("publication_category"); !ar.Blank(c) {
		sel, err := CategorySelection(c)
		if err != nil {
			return nil, err
		}
		category = sel
	}
	m := p.JSON()
	m.Set("author", rb.M("id", rb.Deref(author.ID), "name", author.Name, "email", author.Email))
	m.Set("moderation_note", p.Get("moderation_note"))
	m.Set("strapi_slug", p.Get("strapi_slug"))
	m.Set("category", category)
	m.Set("reviewed_at", timeJSON(p.Get("reviewed_at")))
	m.Set("publication_status", p.Get("publication_status"))
	m.Set("publication_error", p.Get("publication_error"))
	m.Set("documentId", p.Get("strapi_document_id"))
	m.Set("expanded_steps_count", p.ExpandedStepCount())
	if expanded {
		steps := []any{}
		for _, s := range p.Expand() {
			steps = append(steps, rb.M("position", s.Position, "step_type", s.StepType, "title", s.Title,
				"display_title", s.DisplayTitle, "text", s.Text))
		}
		m.Set("expanded_steps", steps)
	}
	return m, nil
}

// Categories ports Integrations::Strapi::Client#categories.
func Categories(ctx context.Context, locale string) ([]any, error) {
	c := newStrapiClient()
	if strings.TrimSpace(c.baseURL) == "" || c.readToken == "" {
		return nil, &StrapiError{Message: "Strapi is not configured", Code: "STRAPI_NOT_CONFIGURED"}
	}
	pairs := [][2]string{{"locale", locale}, {"pagination[pageSize]", "100"}, {"sort[0]", "slug:asc"},
		{"fields[0]", "documentId"}, {"fields[1]", "slug"}, {"fields[2]", "name"}, {"fields[3]", "description"}, {"fields[4]", "icon"}}
	parts := make([]string, len(pairs))
	for i, kv := range pairs {
		parts[i] = url.QueryEscape(kv[0]) + "=" + url.QueryEscape(kv[1])
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/rosary-categories?"+strings.Join(parts, "&"), nil)
	if err != nil {
		return nil, &StrapiError{Message: "Strapi unavailable", Code: "STRAPI_UNAVAILABLE"}
	}
	req.Header.Set("Authorization", "Bearer "+c.readToken)
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req, "Strapi category lookup failed", "STRAPI_CATEGORY_LOOKUP_FAILED")
	if err != nil {
		return nil, err
	}
	var data any
	if m, ok := body.(*rb.Map); ok {
		data = m.Get("data")
	} else {
		fail("NoMethodError", rb.NoMethodErrorMessage("[]", body))
	}
	var entries []any
	switch x := data.(type) {
	case nil:
	case []any:
		entries = x
	default:
		entries = []any{x}
	}
	out := []any{}
	for _, e := range entries {
		entry, ok := e.(*rb.Map)
		if !ok {
			fail("NoMethodError", rb.NoMethodErrorMessage("[]", e))
		}
		fields := entry
		if attrs := entry.Get("attributes"); ar.Truthy(attrs) {
			if m, ok := attrs.(*rb.Map); ok {
				fields = m
			}
		}
		if ar.Blank(fields.Get("documentId")) || ar.Blank(fields.Get("slug")) || ar.Blank(fields.Get("name")) {
			continue
		}
		out = append(out, rb.M("documentId", fields.Get("documentId"), "slug", fields.Get("slug"), "name", fields.Get("name"),
			"description", fields.Get("description"), "icon", fields.Get("icon")))
	}
	return out, nil
}

var _ = db.Q
