package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbfiles "github.com/dodopok/estevao-api-go/db"
)

func TestLockIDMatchesActiveRecord(t *testing.T) {
	// 2053462845 * Zlib.crc32(name), computed by Ruby.
	for name, want := range map[string]int64{
		"estevao_api_production": 6532090544949902205,
		"estevao_mig_test":       4857406234455398400,
	} {
		if got := LockID(name); got != want {
			t.Errorf("LockID(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	fsys := fstest.MapFS{
		"m/20270101000000_add_nickname.sql": {Data: []byte("-- migrate:up\nALTER TABLE users ADD COLUMN nickname varchar;\n\n-- migrate:down\nALTER TABLE users DROP COLUMN nickname;\n")},
		"m/20260930120000_index_users.sql":  {Data: []byte("-- migrate:up transaction:false\nCREATE INDEX CONCURRENTLY idx ON users (email);\n")},
		"m/README.md":                       {Data: []byte("ignored")},
	}
	got, err := Parse(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Version != "20260930120000" || !got[0].NoTransaction || got[0].Down != "" {
		t.Fatalf("first: %+v", got)
	}
	if got[1].Name != "add_nickname" || got[1].Up != "ALTER TABLE users ADD COLUMN nickname varchar;" || got[1].Down != "ALTER TABLE users DROP COLUMN nickname;" {
		t.Fatalf("second: %+v", got[1])
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"20270101000000_x.sql":   "ALTER TABLE t ADD c int;",               // no directive
		"20270101000001_y.sql":   "-- migrate:down\nx;\n-- migrate:up\ny;", // down first
		"20270101000002_z.sql":   "-- migrate:up transaction:maybe\nx;",    // bad option
		"20270101000003_w.sql":   "-- migrate:up\n\n-- migrate:down\nx;",   // empty up
		"20200101000000_old.sql": "-- migrate:up\nx;",                      // before the baseline
		"2027_short.sql":         "-- migrate:up\nx;",                      // bad name
		"20270101000004_Bad.sql": "-- migrate:up\nx;",                      // bad name
		"20270101000005_two.sql": "-- migrate:up\nx;\n-- migrate:up\ny;",   // two ups
	}
	for name, body := range cases {
		if _, err := Parse(fstest.MapFS{"m/" + name: {Data: []byte(body)}}, "m"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// scratch creates an empty database on MIGRATE_TEST_DATABASE_URL's server
// and returns a connection to it.
func scratch(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	base := os.Getenv("MIGRATE_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("MIGRATE_TEST_DATABASE_URL (a server where the test may create databases) is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("estevao_migrate_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg, _ := pgx.ParseConfig(base)
	cfg.Database = name
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close(ctx)
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close(ctx)
	})
	return conn, name
}

func migrations(t *testing.T, files map[string]string) []Migration {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["m/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	got, err := Parse(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMigrateAndRollback(t *testing.T) {
	conn, _ := scratch(t)
	ctx := context.Background()
	migs := migrations(t, map[string]string{
		"20270101000000_add_nickname.sql": "-- migrate:up\nALTER TABLE users ADD COLUMN nickname varchar;\nUPDATE users SET nickname = 'x';\n-- migrate:down\nALTER TABLE users DROP COLUMN nickname;",
		"20270101000001_index_nickname.sql": "-- migrate:up transaction:false\nCREATE INDEX CONCURRENTLY index_users_on_nickname ON users (nickname);\n" +
			"-- migrate:down transaction:false\nDROP INDEX CONCURRENTLY index_users_on_nickname;",
	})
	m := New(conn, migs, Options{})
	if state, _, _ := m.Inspect(ctx); state != Empty {
		t.Fatalf("new database state %v", state)
	}
	if err := m.LoadSchema(ctx, dbfiles.Schema); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadSchema(ctx, dbfiles.Schema); err == nil {
		t.Fatal("schema loaded twice")
	}
	n, err := m.Migrate(ctx)
	if err != nil || n != 2 {
		t.Fatalf("migrate: %d %v", n, err)
	}
	var idx bool
	_ = conn.QueryRow(ctx, "SELECT to_regclass('index_users_on_nickname') IS NOT NULL").Scan(&idx)
	if !idx {
		t.Fatal("index not created")
	}
	if n, err := m.Migrate(ctx); err != nil || n != 0 {
		t.Fatalf("second migrate: %d %v", n, err)
	}
	if n, err := m.Rollback(ctx, 2); err != nil || n != 2 {
		t.Fatalf("rollback: %d %v", n, err)
	}
	var col bool
	_ = conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'nickname')").Scan(&col)
	if col {
		t.Fatal("column not dropped")
	}
	pending, _ := m.Pending(ctx)
	if len(pending) != 2 {
		t.Fatalf("pending after rollback: %d", len(pending))
	}
}

func TestFailedMigrationLeavesNoTrace(t *testing.T) {
	conn, _ := scratch(t)
	ctx := context.Background()
	migs := migrations(t, map[string]string{
		"20270101000000_ok.sql":     "-- migrate:up\nCREATE TABLE t_ok (id int);",
		"20270101000001_broken.sql": "-- migrate:up\nCREATE TABLE t_broken (id int);\nSELECT * FROM does_not_exist;",
		"20270101000002_after.sql":  "-- migrate:up\nCREATE TABLE t_after (id int);",
	})
	m := New(conn, migs, Options{})
	if err := m.LoadSchema(ctx, dbfiles.Schema); err != nil {
		t.Fatal(err)
	}
	n, err := m.Migrate(ctx)
	if err == nil || n != 1 || !strings.Contains(err.Error(), "20270101000001_broken") {
		t.Fatalf("migrate: %d %v", n, err)
	}
	var broken, after bool
	_ = conn.QueryRow(ctx, "SELECT to_regclass('t_broken') IS NOT NULL, to_regclass('t_after') IS NOT NULL").Scan(&broken, &after)
	if broken || after {
		t.Fatalf("partial effects: broken=%v after=%v", broken, after)
	}
	pending, _ := m.Pending(ctx)
	if len(pending) != 2 {
		t.Fatalf("pending: %d", len(pending))
	}
}

func TestIrreversibleRollbackRefused(t *testing.T) {
	conn, _ := scratch(t)
	ctx := context.Background()
	m := New(conn, migrations(t, map[string]string{"20270101000000_x.sql": "-- migrate:up\nCREATE TABLE t_x (id int);"}), Options{})
	if err := m.LoadSchema(ctx, dbfiles.Schema); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Rollback(ctx, 1); err == nil || !strings.Contains(err.Error(), "irreversible") {
		t.Fatalf("rollback: %v", err)
	}
}

func TestConcurrentMigratorIsRefused(t *testing.T) {
	conn, name := scratch(t)
	ctx := context.Background()
	m := New(conn, nil, Options{})
	if err := m.LoadSchema(ctx, dbfiles.Schema); err != nil {
		t.Fatal(err)
	}
	cfg := conn.Config().Copy()
	other, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(ctx)
	// A Rails migrator holding its lock.
	if _, err := other.Exec(ctx, "SELECT pg_advisory_lock($1)", LockID(name)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); !errors.Is(err, ErrLocked) {
		t.Fatalf("migrate under a held lock: %v", err)
	}
}

func TestForeignDatabasesAreRefused(t *testing.T) {
	conn, _ := scratch(t)
	ctx := context.Background()
	m := New(conn, nil, Options{})
	if _, err := conn.Exec(ctx, "CREATE TABLE something (id int)"); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := m.Inspect(ctx); state != Foreign {
		t.Fatalf("state %v", state)
	}
	if _, err := m.Migrate(ctx); err == nil {
		t.Fatal("migrated a database without schema_migrations")
	}
	if _, err := conn.Exec(ctx, "CREATE TABLE schema_migrations (version varchar PRIMARY KEY); INSERT INTO schema_migrations VALUES ('20200101000000')"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(ctx); err == nil || !strings.Contains(err.Error(), Baseline) {
		t.Fatalf("migrated a database behind the baseline: %v", err)
	}
}
