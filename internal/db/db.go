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

// Tracer, when set before Open, traces every query (New Relic segments).
var Tracer pgx.QueryTracer

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
	// Unnamed statements are planned with their actual parameters on every
	// execution (a custom plan), as ActiveRecord's unprepared queries are.
	// Named cached statements switch to a generic plan after five runs, which
	// can break ties differently in ORDER BY ... LIMIT queries whose order is
	// not total - and those ties are observable in responses.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	// Production safeguards mirrored from config/database.yml.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "10000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	if Tracer != nil {
		cfg.ConnConfig.Tracer = Tracer
	}
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

type txKey struct{}

// Conn is the connection Active Record would use here: the transaction
// Transaction opened on ctx, or the pool outside one.
func Conn(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return Pool
}

// Transaction ports `transaction do ... end`: f runs in a transaction that
// every Conn(ctx) query inside it shares, and a nested call joins the
// enclosing one instead of opening its own.
func Transaction(ctx context.Context, f func(ctx context.Context, tx pgx.Tx) error) error {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return f(ctx, tx)
	}
	return pgx.BeginFunc(ctx, Pool, func(tx pgx.Tx) error { return f(context.WithValue(ctx, txKey{}, tx), tx) })
}
