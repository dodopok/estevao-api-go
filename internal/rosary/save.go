package rosary

import (
	"context"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/users"
)

func (p *Prayer) contentModified() bool {
	for _, a := range contentAttributes {
		if p.Changed(a) {
			return true
		}
	}
	for _, b := range p.Blocks {
		if b.NewRecord || b.HasChanges() || b.Marked {
			return true
		}
		for _, s := range b.Steps {
			if s.NewRecord || s.HasChanges() || s.Marked {
				return true
			}
		}
	}
	return false
}

func (p *Prayer) approved() bool { return p.Get("share_status") == "approved" }

// beforeValidation runs the before_validation callbacks in order.
func (p *Prayer) beforeValidation(now time.Time) {
	// reset_moderation_decision
	if !p.NewRecord && !ar.Blank(p.Get("moderation_decision")) && p.contentModified() {
		p.Set("moderation_reentry_count", toI(p.Get("moderation_reentry_count"))+1)
		p.Set("last_moderation_reentry_at", now)
		p.Set("moderation_decision", nil)
		p.Set("reviewed_at", nil)
	}
	// reset_publication_status_on_content_change
	if !p.NewRecord && p.contentModified() && ar.Included(p.Get("publication_status"), []string{"published", "publishing"}) {
		p.Set("publication_status", "pending")
		p.Set("publication_error", nil)
	}
	// apply_share_status
	if p.Get("is_public") == true {
		if d := p.Get("moderation_decision"); !ar.Blank(d) {
			p.Set("share_status", d)
		} else {
			p.Set("share_status", "pending_review")
		}
	} else {
		p.Set("share_status", "private")
	}
	// mark_publication_for_unpublish
	if p.NewRecord || ar.Blank(p.Get("strapi_document_id")) {
		return
	}
	public := p.Get("is_public") == true
	if public && p.Get("moderation_decision") == "approved" {
		switch p.Get("publication_status") {
		case "unpublishing":
			return
		case "unpublished":
			p.Set("publication_status", "pending")
			p.Set("publication_error", nil)
			p.Set("publication_retry_at", nil)
			p.Set("publication_attempts", int64(0))
			return
		}
	}
	if !ar.Included(p.Get("publication_status"), []string{"published", "failed", "publishing", "pending"}) {
		return
	}
	if public && p.Get("moderation_decision") == "approved" {
		return
	}
	p.Set("publication_status", "unpublishing")
	p.Set("publication_error", nil)
	p.Set("publication_retry_at", nil)
}

// Save ports save!: callbacks, validations, then the prayer, its blocks and
// their steps in one transaction; after the commit, the publication jobs of
// an update. It returns *ar.RecordInvalid when a validation fails.
func (p *Prayer) Save(ctx context.Context) error {
	now := users.Now()
	p.beforeValidation(now)
	create := p.NewRecord
	if errs := p.Validate(ctx, db.Conn(ctx), create); errs.Any() {
		return &ar.RecordInvalid{Errors: errs}
	}
	before := map[string]any{}
	for _, c := range []string{"moderation_decision", "reviewed_at", "publication_status"} {
		before[c] = p.Orig[c]
	}
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if create {
			if err := p.Insert(ctx, tx, now); err != nil {
				return err
			}
		} else if _, err := p.Update(ctx, tx, now); err != nil {
			return err
		}
		return p.saveBlocks(ctx, tx, create, now)
	})
	if err != nil {
		return err
	}
	if !create {
		p.afterCommitUpdate(ctx, before)
	}
	return nil
}

func (p *Prayer) saveBlocks(ctx context.Context, tx pgx.Tx, prayerWasNew bool, now time.Time) error {
	var records []*Block
	for _, b := range p.Blocks {
		if prayerWasNew || b.changedForAutosave() {
			records = append(records, b)
		}
	}
	for _, b := range records {
		if b.Marked {
			if err := b.destroy(ctx, tx); err != nil {
				return err
			}
		}
	}
	for _, b := range records {
		if b.Marked {
			continue
		}
		wasNew := b.NewRecord
		if wasNew || prayerWasNew {
			b.Set("custom_rosary_prayer_id", p.ID)
			if err := b.Insert(ctx, tx, now); err != nil {
				return err
			}
		} else if _, err := b.Update(ctx, tx, now); err != nil {
			return err
		}
		if err := b.saveSteps(ctx, tx, wasNew, now); err != nil {
			return err
		}
	}
	p.Blocks = slices.DeleteFunc(p.Blocks, func(b *Block) bool { return b.Marked })
	return nil
}

func (b *Block) saveSteps(ctx context.Context, tx pgx.Tx, blockWasNew bool, now time.Time) error {
	var records []*Step
	for _, s := range b.Steps {
		if blockWasNew || s.changedForAutosave() {
			records = append(records, s)
		}
	}
	for _, s := range records {
		if s.Marked && !s.NewRecord {
			if _, err := tx.Exec(ctx, `DELETE FROM custom_rosary_steps WHERE id = $1`, s.ID); err != nil {
				return err
			}
		}
	}
	for _, s := range records {
		if s.Marked {
			continue
		}
		if s.NewRecord || blockWasNew {
			s.Set("custom_rosary_block_id", b.ID)
			if err := s.Insert(ctx, tx, now); err != nil {
				return err
			}
		} else if _, err := s.Update(ctx, tx, now); err != nil {
			return err
		}
	}
	b.Steps = slices.DeleteFunc(b.Steps, func(s *Step) bool { return s.Marked })
	return nil
}

// destroy ports block.destroy (dependent: :destroy on its steps).
func (b *Block) destroy(ctx context.Context, q db.Querier) error {
	if b.NewRecord {
		return nil
	}
	for _, s := range b.Steps {
		if s.NewRecord {
			continue
		}
		if _, err := q.Exec(ctx, `DELETE FROM custom_rosary_steps WHERE id = $1`, s.ID); err != nil {
			return err
		}
	}
	_, err := q.Exec(ctx, `DELETE FROM custom_rosary_blocks WHERE id = $1`, b.ID)
	return err
}

// Destroy ports CustomRosaryPrayers::Destroy after the policy check: the
// unpublication of a published copy is queued, then the prayer, its blocks
// and steps are deleted.
//
// Rails enqueues before deleting and a worker picks the job up later; the
// in-process job starts immediately, so it is enqueued once the row is gone
// (otherwise it would find the prayer still published and skip the call).
func (p *Prayer) Destroy(ctx context.Context) error {
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, b := range p.Blocks {
			if err := b.destroy(ctx, tx); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM custom_rosary_prayers WHERE id = $1`, p.ID)
		return err
	})
	if err == nil {
		if doc := p.Get("strapi_document_id"); !ar.Blank(doc) {
			EnqueueUnpublish(p.ID, doc.(string))
		}
	}
	return err
}

// afterCommitUpdate ports the after_commit (on: :update) callbacks.
func (p *Prayer) afterCommitUpdate(ctx context.Context, before map[string]any) {
	savedChange := func(c string) bool { return !ar.SameValue(before[c], p.Get(c)) }
	// enqueue_publication_job
	if p.approved() && p.Get("publication_status") == "pending" &&
		(savedChange("moderation_decision") || savedChange("reviewed_at") || savedChange("publication_status")) {
		EnqueuePublish(p.ID)
	}
	// enqueue_unpublication_job
	if p.Get("publication_status") == "unpublishing" && savedChange("publication_status") {
		must(p.UpdateColumns(ctx, db.Conn(ctx), map[string]any{"publication_started_at": users.Now()}))
		EnqueueUnpublish(p.ID, p.Str("strapi_document_id"))
	}
}
