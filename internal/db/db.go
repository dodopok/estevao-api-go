// Package db owns the PostgreSQL connection pool. The schema is the one the
// Rails application manages (db/schema.rb); this service reads and writes it
// but never migrates it.
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the process-wide pool, set by Open.
var Pool *pgxpool.Pool

// Open connects using a DATABASE_URL-style DSN.
func Open(ctx context.Context, dsn string, maxConns int32) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 300 * time.Second
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	// Production safeguards mirrored from config/database.yml.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "10000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	Pool = p
	return nil
}

// Querier is satisfied by the pool and by transactions.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Q returns the pool as a Querier.
func Q() Querier { return Pool }

// NoRows reports whether err is pgx.ErrNoRows.
func NoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// UniqueViolation reports a unique constraint error.
func UniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// InTx runs f in a transaction.
func InTx(ctx context.Context, f func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, Pool, f)
}
