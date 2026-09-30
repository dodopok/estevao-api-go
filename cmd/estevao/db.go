package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbfiles "github.com/dodopok/estevao-api-go/db"
	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/migrate"
	"github.com/dodopok/estevao-api-go/internal/seed"
)

func dbCommand(args []string) error {
	if len(args) == 0 {
		usage()
	}
	sub, args := args[0], args[1:]
	ctx := context.Background()
	switch sub {
	case "new":
		return dbNew(args)
	case "dump":
		fs := flag.NewFlagSet("dump", flag.ExitOnError)
		out := fs.String("o", "db/schema.sql", "output file")
		_ = fs.Parse(args)
		url, err := databaseURL()
		if err != nil {
			return err
		}
		sql, err := migrate.Dump(ctx, url)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*out, []byte(sql), 0o644); err != nil {
			return err
		}
		logf("wrote %s", *out)
		return nil
	}

	migrations, err := migrate.Parse(dbfiles.Migrations, "migrations")
	if err != nil {
		return err
	}
	url, err := databaseURL()
	if err != nil {
		return err
	}
	switch sub {
	case "prepare":
		fs := flag.NewFlagSet("prepare", flag.ExitOnError)
		seeds := fs.String("seeds", "seeds", "dataset loaded into a database this command creates")
		noSeed := fs.Bool("no-seed", false, "leave the reference tables of a new database empty")
		_ = fs.Parse(args)
		return dbPrepare(ctx, url, migrations, *seeds, *noSeed)
	case "migrate":
		return withMigrator(ctx, url, migrations, func(m *migrate.Migrator) error {
			_, err := m.Migrate(ctx)
			return err
		})
	case "status":
		return withMigrator(ctx, url, migrations, func(m *migrate.Migrator) error {
			state, why, err := m.Inspect(ctx)
			if err != nil {
				return err
			}
			if state != migrate.Managed {
				return fmt.Errorf("not a managed database: %s", why)
			}
			lines, history, err := m.Status(ctx)
			if err != nil {
				return err
			}
			logf("%d earlier versions applied (the Rails history up to %s)", history, migrate.Baseline)
			for _, l := range lines {
				st := "pending"
				if l.Applied {
					st = "applied"
				}
				logf("%-8s %s_%s", st, l.Version, l.Name)
			}
			if len(lines) == 0 {
				logf("no migration in db/migrations yet")
			}
			return nil
		})
	case "rollback":
		fs := flag.NewFlagSet("rollback", flag.ExitOnError)
		steps := fs.Int("steps", 1, "migrations to revert")
		_ = fs.Parse(args)
		return withMigrator(ctx, url, migrations, func(m *migrate.Migrator) error {
			_, err := m.Rollback(ctx, *steps)
			return err
		})
	}
	usage()
	return nil
}

func withMigrator(ctx context.Context, url string, migrations []migrate.Migration, f func(*migrate.Migrator) error) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	return f(migrate.New(conn, migrations, migrate.Options{LockTimeout: lockTimeout(), Log: logf}))
}

func lockTimeout() time.Duration {
	if v := os.Getenv("MIGRATION_LOCK_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 5 * time.Second
}

// dbPrepare ports `rails db:prepare`: create the database if it does not
// exist, load the schema and the seeds into a database with no tables,
// otherwise apply the pending migrations.
func dbPrepare(ctx context.Context, url string, migrations []migrate.Migration, seeds string, noSeed bool) error {
	if err := createIfMissing(ctx, url); err != nil {
		return err
	}
	loaded := false
	err := withMigrator(ctx, url, migrations, func(m *migrate.Migrator) error {
		state, why, err := m.Inspect(ctx)
		if err != nil {
			return err
		}
		switch state {
		case migrate.Foreign:
			return fmt.Errorf("cannot prepare: %s", why)
		case migrate.Empty:
			if err := m.LoadSchema(ctx, dbfiles.Schema); err != nil {
				return err
			}
			loaded = true
		}
		_, err = m.Migrate(ctx)
		return err
	})
	if err != nil || !loaded || noSeed {
		return err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	return seed.Load(ctx, pool, seeds, logf)
}

// createIfMissing creates the database of url when PostgreSQL reports it
// does not exist (3D000), connecting to the maintenance database.
func createIfMissing(ctx context.Context, url string) error {
	conn, err := pgx.Connect(ctx, url)
	if err == nil {
		return conn.Close(ctx)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "3D000" {
		return err
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return err
	}
	name := cfg.Database
	cfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database %q does not exist and the maintenance database is unreachable: %w", name, err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return err
	}
	logf("created database %s", name)
	return nil
}

var nameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

func dbNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	dir := fs.String("dir", "db/migrations", "migrations directory")
	_ = fs.Parse(args)
	if fs.NArg() != 1 || !nameRe.MatchString(fs.Arg(0)) {
		return fmt.Errorf("usage: estevao db new <snake_case_name>")
	}
	version := clock.Now().UTC().Format("20060102150405")
	file := filepath.Join(*dir, version+"_"+fs.Arg(0)+".sql")
	body := "-- migrate:up\n\n\n-- migrate:down\n\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return err
	}
	logf("created %s", file)
	return nil
}
