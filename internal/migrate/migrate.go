// Package migrate owns the database schema: it creates a database from the
// schema snapshot and applies the SQL migrations in db/migrations.
//
// Migrations are recorded in the schema_migrations table the Rails app
// created, with versions in the same format, so production keeps one
// history: the Rails migrations up to Baseline, then these. The advisory
// lock is the one ActiveRecord::Migrator takes, so a Go and a Rails
// migrator can never run against the same database at once.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Baseline is the last Rails migration. A database that has it applied is
// the schema db/schema.sql started from; one without it was neither created
// by Rails nor by this package, and is refused.
const Baseline = "20260922160000"

// migratorSalt is ActiveRecord::Migrator::MIGRATOR_SALT.
const migratorSalt = 2053462845

// Migration is one db/migrations file.
type Migration struct {
	Version       string
	Name          string
	Up, Down      string
	NoTransaction bool
}

var fileRe = regexp.MustCompile(`^(\d{14})_([a-z0-9_]+)\.sql$`)

// Parse reads every migration of dir in fsys, ordered by version.
func Parse(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Migration
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("%s: migration files are named <14-digit version>_<snake_case_name>.sql", e.Name())
		}
		if prev, dup := seen[m[1]]; dup {
			return nil, fmt.Errorf("%s and %s share version %s", prev, e.Name(), m[1])
		}
		seen[m[1]] = e.Name()
		src, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		mig, err := parseBody(string(src))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		mig.Version, mig.Name = m[1], m[2]
		if mig.Version <= Baseline {
			return nil, fmt.Errorf("%s: version must be later than the Rails baseline %s", e.Name(), Baseline)
		}
		out = append(out, mig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

var directiveRe = regexp.MustCompile(`(?m)^--\s*migrate:(up|down)\b(.*)$`)

// parseBody splits the `-- migrate:up` and `-- migrate:down` sections
// (dbmate's format, which dbmate itself can also run against this table).
func parseBody(src string) (Migration, error) {
	var m Migration
	locs := directiveRe.FindAllStringSubmatchIndex(src, -1)
	if len(locs) == 0 || src[locs[0][2]:locs[0][3]] != "up" {
		return m, errors.New("must start with a `-- migrate:up` line")
	}
	if strings.TrimSpace(src[:locs[0][0]]) != "" {
		return m, errors.New("text before `-- migrate:up`")
	}
	for i, loc := range locs {
		kind := src[loc[2]:loc[3]]
		opts := strings.Fields(src[loc[4]:loc[5]])
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		body := strings.TrimSpace(src[loc[1]:end])
		switch {
		case kind == "up" && i == 0:
			m.Up = body
			for _, o := range opts {
				switch o {
				case "transaction:false":
					m.NoTransaction = true
				case "transaction:true":
				default:
					return m, fmt.Errorf("unknown option %q", o)
				}
			}
		case kind == "down" && i == 1:
			m.Down = body
		default:
			return m, errors.New("expected one `-- migrate:up` section, optionally followed by one `-- migrate:down`")
		}
	}
	if m.Up == "" {
		return m, errors.New("empty `-- migrate:up` section")
	}
	return m, nil
}

// Options tune a Migrator.
type Options struct {
	// LockTimeout bounds how long a migration waits for a table lock, so a
	// migration stuck behind a long query fails instead of queueing every
	// request behind it. Zero means 5 s.
	LockTimeout time.Duration
	// Log receives progress lines.
	Log func(format string, args ...any)
}

// Migrator applies migrations over one connection.
type Migrator struct {
	conn       *pgx.Conn
	migrations []Migration
	opts       Options
}

// New prepares a migrator. conn must be dedicated to it (session settings
// and the advisory lock live on the connection).
func New(conn *pgx.Conn, migrations []Migration, opts Options) *Migrator {
	if opts.LockTimeout == 0 {
		opts.LockTimeout = 5 * time.Second
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	return &Migrator{conn: conn, migrations: migrations, opts: opts}
}

// ErrLocked means another migrator (Go or Rails) holds the lock.
var ErrLocked = errors.New("another migration is running against this database (ActiveRecord::ConcurrentMigrationError)")

// LockID is ActiveRecord::Migrator#generate_migrator_advisory_lock_id.
func LockID(database string) int64 {
	return migratorSalt * int64(crc32.ChecksumIEEE([]byte(database)))
}

// withLock runs f holding the migrator advisory lock.
func (m *Migrator) withLock(ctx context.Context, f func() error) error {
	var db string
	if err := m.conn.QueryRow(ctx, "SELECT current_database()").Scan(&db); err != nil {
		return err
	}
	id := LockID(db)
	var got bool
	if err := m.conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", id).Scan(&got); err != nil {
		return err
	}
	if !got {
		return ErrLocked
	}
	defer m.conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", id)
	return f()
}

// State is what the database looks like to the migrator.
type State int

const (
	// Empty: no table in the public schema.
	Empty State = iota
	// Managed: schema_migrations exists and has the Baseline.
	Managed
	// Foreign: tables exist but not a schema this package can manage.
	Foreign
)

// Inspect classifies the database.
func (m *Migrator) Inspect(ctx context.Context) (State, string, error) {
	var tables int
	if err := m.conn.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public'`).Scan(&tables); err != nil {
		return Foreign, "", err
	}
	if tables == 0 {
		return Empty, "", nil
	}
	var exists bool
	if err := m.conn.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return Foreign, "", err
	}
	if !exists {
		return Foreign, "tables exist but schema_migrations does not", nil
	}
	var baseline bool
	if err := m.conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, Baseline).Scan(&baseline); err != nil {
		return Foreign, "", err
	}
	if !baseline {
		return Foreign, "schema_migrations lacks the Rails baseline " + Baseline + ": run the Rails migrations up to it first", nil
	}
	return Managed, "", nil
}

// Applied returns the versions in schema_migrations.
func (m *Migrator) Applied(ctx context.Context) (map[string]bool, error) {
	rows, err := m.conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(versions))
	for _, v := range versions {
		out[v] = true
	}
	return out, nil
}

// Pending returns the migrations not yet applied, in order.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	applied, err := m.Applied(ctx)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, mig := range m.migrations {
		if !applied[mig.Version] {
			out = append(out, mig)
		}
	}
	return out, nil
}

// LoadSchema creates every table of an empty database from schema (the
// db/schema.sql snapshot, which also inserts the versions it contains).
func (m *Migrator) LoadSchema(ctx context.Context, schema string) error {
	return m.withLock(ctx, func() error {
		state, why, err := m.Inspect(ctx)
		if err != nil {
			return err
		}
		if state != Empty {
			return fmt.Errorf("refusing to load the schema into a database that has tables (%s)", describe(state, why))
		}
		m.opts.Log("loading db/schema.sql")
		return pgx.BeginFunc(ctx, m.conn, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, schema)
			return err
		})
	})
}

// Migrate applies every pending migration, each in its own transaction
// together with its schema_migrations row (unless it opts out).
func (m *Migrator) Migrate(ctx context.Context) (applied int, err error) {
	err = m.withLock(ctx, func() error {
		state, why, err := m.Inspect(ctx)
		if err != nil {
			return err
		}
		if state != Managed {
			return fmt.Errorf("cannot migrate: %s", describe(state, why))
		}
		pending, err := m.Pending(ctx)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			m.opts.Log("schema is up to date")
			return nil
		}
		if _, err := m.conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = 0; SET lock_timeout = %d", m.opts.LockTimeout.Milliseconds())); err != nil {
			return err
		}
		for _, mig := range pending {
			start := time.Now()
			if err := m.apply(ctx, mig.Up, mig.NoTransaction, func(q execer) error {
				_, err := q.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", mig.Version)
				return err
			}); err != nil {
				return fmt.Errorf("%s_%s: %w", mig.Version, mig.Name, err)
			}
			applied++
			m.opts.Log("migrated %s_%s (%s)", mig.Version, mig.Name, time.Since(start).Round(time.Millisecond))
		}
		return nil
	})
	return applied, err
}

// Rollback reverts the last steps migrations applied by this package. Rails
// migrations (up to Baseline) have no down section here and are never
// reverted.
func (m *Migrator) Rollback(ctx context.Context, steps int) (int, error) {
	reverted := 0
	err := m.withLock(ctx, func() error {
		state, why, err := m.Inspect(ctx)
		if err != nil {
			return err
		}
		if state != Managed {
			return fmt.Errorf("cannot roll back: %s", describe(state, why))
		}
		applied, err := m.Applied(ctx)
		if err != nil {
			return err
		}
		var done []Migration
		for _, mig := range m.migrations {
			if applied[mig.Version] {
				done = append(done, mig)
			}
		}
		if _, err := m.conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = 0; SET lock_timeout = %d", m.opts.LockTimeout.Milliseconds())); err != nil {
			return err
		}
		for i := len(done) - 1; i >= 0 && reverted < steps; i-- {
			mig := done[i]
			if mig.Down == "" {
				return fmt.Errorf("%s_%s has no `-- migrate:down` section: it is irreversible", mig.Version, mig.Name)
			}
			if err := m.apply(ctx, mig.Down, mig.NoTransaction, func(q execer) error {
				_, err := q.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", mig.Version)
				return err
			}); err != nil {
				return fmt.Errorf("rolling back %s_%s: %w", mig.Version, mig.Name, err)
			}
			reverted++
			m.opts.Log("rolled back %s_%s", mig.Version, mig.Name)
		}
		if reverted == 0 {
			m.opts.Log("nothing to roll back: no migration of db/migrations is applied")
		}
		return nil
	})
	return reverted, err
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// apply runs sql and then record; both in one transaction unless noTx.
func (m *Migrator) apply(ctx context.Context, sql string, noTx bool, record func(execer) error) error {
	if noTx {
		// A multi-statement string is one implicit transaction in
		// PostgreSQL, so a transaction:false migration holds a single
		// statement (CREATE INDEX CONCURRENTLY refuses anything else).
		if _, err := m.conn.Exec(ctx, sql); err != nil {
			return err
		}
		return record(m.conn)
	}
	return pgx.BeginFunc(ctx, m.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, sql); err != nil {
			return err
		}
		return record(tx)
	})
}

// StatusLine is one row of Status.
type StatusLine struct {
	Version, Name string
	Applied       bool
}

// Status lists every migration of db/migrations and whether it is applied,
// and counts the applied versions that have no file (the Rails history).
func (m *Migrator) Status(ctx context.Context) (lines []StatusLine, railsHistory int, err error) {
	applied, err := m.Applied(ctx)
	if err != nil {
		return nil, 0, err
	}
	known := map[string]bool{}
	for _, mig := range m.migrations {
		known[mig.Version] = true
		lines = append(lines, StatusLine{mig.Version, mig.Name, applied[mig.Version]})
	}
	for v := range applied {
		if !known[v] {
			railsHistory++
		}
	}
	return lines, railsHistory, nil
}

func describe(s State, why string) string {
	switch s {
	case Empty:
		return "the database is empty"
	case Managed:
		return "the database is managed"
	}
	return why
}
