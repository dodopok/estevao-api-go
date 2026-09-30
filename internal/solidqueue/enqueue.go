package solidqueue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Enqueued is the persisted job (provider_job_id and job_id).
type Enqueued struct {
	ID          int64
	ActiveJobID string
}

func now() time.Time { return clock.Now().UTC().Truncate(time.Microsecond) }

// Enqueue ports SolidQueue::Job.enqueue (perform_later / set(wait:)): the
// job row and its ready or scheduled execution, in one transaction (the
// ctx's, when Transaction opened one).
func Enqueue(ctx context.Context, j Job) (*Enqueued, error) {
	var out *Enqueued
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = enqueueIn(ctx, tx, j)
		return err
	})
	return out, err
}

func enqueueIn(ctx context.Context, q db.Querier, j Job) (*Enqueued, error) {
	t := now()
	j.defaults(t)
	scheduled := j.ScheduledAt.UTC().Truncate(time.Microsecond)
	priority := 0
	if j.Priority != nil {
		priority = *j.Priority
	}
	args := rb.JSONGenerate(j.Serialize(t))
	var id int64
	if err := q.QueryRow(ctx, `INSERT INTO solid_queue_jobs
		(queue_name, active_job_id, priority, scheduled_at, class_name, arguments, concurrency_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $7) RETURNING id`,
		j.Queue, j.JobID, priority, scheduled, j.Class, args, t).Scan(&id); err != nil {
		return nil, err
	}
	if !scheduled.After(t) {
		if _, err := q.Exec(ctx, `INSERT INTO solid_queue_ready_executions (job_id, queue_name, priority, created_at)
			VALUES ($1, $2, $3, $4) ON CONFLICT (job_id) DO NOTHING`, id, j.Queue, priority, t); err != nil {
			return nil, err
		}
	} else {
		if _, err := q.Exec(ctx, `INSERT INTO solid_queue_scheduled_executions (job_id, queue_name, priority, scheduled_at, created_at)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (job_id) DO NOTHING`, id, j.Queue, priority, scheduled, t); err != nil {
			return nil, err
		}
	}
	return &Enqueued{ID: id, ActiveJobID: j.JobID}, nil
}
