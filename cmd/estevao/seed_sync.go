package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dodopok/estevao-api-go/internal/seed"
)

// seedSync ports the incremental seed tasks (prayer_books:seed[code],
// liturgical_texts:sync_catalog, import:collects, psalters:seed,
// background_music:seed): it reports what the dataset would change and,
// with -apply, changes it.
func seedSync(ctx context.Context, pool *pgxpool.Pool, dir, book string, apply bool) error {
	reports, err := seed.Sync(ctx, pool, dir, seed.SyncOptions{Book: book, Apply: apply, LockTimeout: lockTimeout()})
	if err != nil {
		return err
	}
	changed, only := false, 0
	for _, r := range reports {
		only += r.OnlyInDatabase
		if !r.Changed() && r.OnlyInDatabase == 0 {
			continue
		}
		changed = changed || r.Changed()
		logf("%-28s %5d new  %5d changed  %4d groups replaced (%d rows)  %5d unchanged  %4d only in the database",
			r.Table, r.Inserted, r.Updated, r.GroupsReplaced, r.RowsReplaced, r.Unchanged, r.OnlyInDatabase)
		for _, s := range r.Samples {
			logf("    %s", s)
		}
	}
	switch {
	case !changed:
		logf("the database matches the dataset")
	case apply:
		logf("applied; the Prayer Books whose content changed were touched (their caches refresh)")
	default:
		logf("nothing written: run again with -apply")
	}
	if only > 0 {
		logf("%d rows exist only in the database; sync never deletes (write a migration if they must go)", only)
	}
	return nil
}
