package audioadmin

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
)

// FindOperation ports AudioOperation.find_by(id:).
func FindOperation(ctx context.Context, id int64) *Operation {
	list := loadOperations(ctx, `id = $1`, `LIMIT 1`, id)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// RecentOperations ports AudioOperation.recent.limit(n).
func RecentOperations(ctx context.Context, limit int) []*Operation {
	return loadOperations(ctx, `TRUE`, `ORDER BY "audio_operations"."created_at" DESC LIMIT `+strconv.Itoa(limit))
}

// Status ports OperationStatus.call(operation).
func Status(ctx context.Context, o *Operation) *rb.Map {
	return OperationStatuses(ctx, []*Operation{o})[0].(*rb.Map)
}

// EnqueueOperation ports OperationEnqueuer#enqueue: the operation row, its
// job, then the job id on the row; a failed enqueue marks the operation
// failed. parameters keeps its given key order in the returned record, as
// the in-memory Active Record object does.
func EnqueueOperation(ctx context.Context, kind string, parameters *rb.Map, prayerBookCode any, requestedBy string,
	job func(operationID int64) solidqueue.Job) (*Operation, error) {
	now := clock.Now().UTC().Truncate(time.Microsecond)
	o := &Operation{Kind: kind, Status: "queued", Parameters: rb.JSON(parameters), Result: []byte("{}"), CreatedAt: now}
	if prayerBookCode != nil {
		s := rb.ToS(prayerBookCode)
		o.PrayerBookCode = &s
	}
	if requestedBy != "" {
		o.RequestedBy = &requestedBy
	}
	if err := db.Q().QueryRow(ctx, `INSERT INTO audio_operations (kind, status, parameters, prayer_book_code, requested_by,
		created_at, updated_at) VALUES ($1, 'queued', $2::jsonb, $3, $4, $5, $5) RETURNING id`,
		kind, string(o.Parameters), o.PrayerBookCode, o.RequestedBy, now).Scan(&o.ID); err != nil {
		return nil, err
	}
	enq, err := solidqueue.Enqueue(ctx, job(o.ID))
	if err == nil {
		o.ActiveJobID = &enq.ActiveJobID
		_, err = db.Q().Exec(ctx, `UPDATE audio_operations SET active_job_id = $2, updated_at = $3 WHERE id = $1`,
			o.ID, enq.ActiveJobID, clock.Now().UTC())
	}
	if err != nil {
		MarkFailed(ctx, o.ID, solidqueue.ClassOf(err), err.Error())
		return nil, err
	}
	return o, nil
}

// --- the lifecycle Audio::OperationJob drives ---------------------------------------

// MarkRunning ports mark_running!.
func MarkRunning(ctx context.Context, id int64) {
	t := clock.Now().UTC()
	_, _ = db.Q().Exec(ctx, `UPDATE audio_operations SET status = 'running', started_at = COALESCE(started_at, $2),
		updated_at = CASE WHEN status <> 'running' OR started_at IS NULL THEN $2 ELSE updated_at END WHERE id = $1`, id, t)
}

// Progress is update_progress!'s values (nil fields keep the stored value).
type Progress struct {
	Processed, Total, Generated, Skipped, Failed, Characters *int64
}

// UpdateProgress ports update_progress! (updated_at moves only when a
// counter changed, as a no-op update! leaves the row alone).
func UpdateProgress(ctx context.Context, id int64, p Progress) error {
	_, err := db.Q().Exec(ctx, `UPDATE audio_operations SET processed_items = COALESCE($2, processed_items),
		total_items = COALESCE($3, total_items), generated_clips = COALESCE($4, generated_clips),
		skipped_clips = COALESCE($5, skipped_clips), failed_items = COALESCE($6, failed_items),
		generated_characters = COALESCE($7, generated_characters),
		updated_at = CASE WHEN (processed_items, total_items, generated_clips, skipped_clips, failed_items, generated_characters)
			IS DISTINCT FROM (COALESCE($2, processed_items), COALESCE($3, total_items), COALESCE($4, generated_clips),
			COALESCE($5, skipped_clips), COALESCE($6, failed_items), COALESCE($7, generated_characters))
			THEN $8 ELSE updated_at END WHERE id = $1`,
		id, p.Processed, p.Total, p.Generated, p.Skipped, p.Failed, p.Characters, clock.Now().UTC())
	return err
}

// MarkCompleted ports mark_completed!(values): progress, then status and
// the values as the result.
func MarkCompleted(ctx context.Context, id int64, p Progress, result *rb.Map) error {
	if err := UpdateProgress(ctx, id, p); err != nil {
		return err
	}
	_, err := db.Q().Exec(ctx, `UPDATE audio_operations SET status = 'completed', completed_at = $2, result = $3::jsonb, updated_at = $2 WHERE id = $1`,
		id, clock.Now().UTC(), string(rb.JSON(result)))
	return err
}

// MarkFailed ports mark_failed!(error): "Class: message".
func MarkFailed(ctx context.Context, id int64, class, message string) {
	msg := class + ": " + message
	_, _ = db.Q().Exec(ctx, `UPDATE audio_operations SET status = 'failed', completed_at = $2, error_message = $3, updated_at = $2 WHERE id = $1`,
		id, clock.Now().UTC(), msg)
}

// SetParameter merges one key into parameters (operation.update!(parameters: ...merge)).
func SetParameter(ctx context.Context, id int64, key string, value any) {
	_, _ = db.Q().Exec(ctx, `UPDATE audio_operations SET parameters = COALESCE(parameters, '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb),
		updated_at = $4 WHERE id = $1`, id, key, string(rb.JSON(value)), clock.Now().UTC())
}

// --- WorkerQueue.purge -----------------------------------------------------------

// PurgeWorkerQueue ports WorkerQueue.purge(scope:).
func PurgeWorkerQueue(ctx context.Context, scope string) *rb.Map {
	if scope != "dead" && scope != "all" {
		scope = "dead"
	}
	var targets []classifiedJob
	for _, j := range classified(ctx) {
		if scope == "dead" && deadState(j.state) || scope == "all" && j.state != "running" {
			targets = append(targets, j)
		}
	}
	if len(targets) == 0 {
		return rb.M("scope", scope, "purged_jobs", 0, "cancelled_operations", 0)
	}
	cancelled := 0
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var activeIDs []string
		var ids []int64
		for _, t := range targets {
			if t.job.ActiveJobID != nil {
				activeIDs = append(activeIDs, *t.job.ActiveJobID)
			}
			ids = append(ids, t.job.ID)
		}
		if len(activeIDs) > 0 {
			rows, err := tx.Query(ctx, `SELECT id FROM audio_operations WHERE status IN ('queued', 'running') AND active_job_id = ANY($1)`, activeIDs)
			if err != nil {
				return err
			}
			var opIDs []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				opIDs = append(opIDs, id)
			}
			rows.Close()
			t := clock.Now().UTC()
			for _, id := range opIDs {
				if _, err := tx.Exec(ctx, `UPDATE audio_operations SET status = 'cancelled', completed_at = COALESCE(completed_at, $2),
					error_message = 'job removed from the worker queue', updated_at = $2 WHERE id = $1`, id, t); err != nil {
					return err
				}
			}
			cancelled = len(opIDs)
		}
		_, err := tx.Exec(ctx, `DELETE FROM solid_queue_jobs WHERE id = ANY($1)`, ids)
		return err
	})
	must(err)
	return rb.M("scope", scope, "purged_jobs", len(targets), "cancelled_operations", cancelled)
}
