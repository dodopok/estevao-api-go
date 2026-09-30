package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbfiles "github.com/dodopok/estevao-api-go/db"
)

// seededDatabase creates a scratch database on MIGRATE_TEST_DATABASE_URL's
// server with the schema and the dataset loaded.
func seededDatabase(t *testing.T) *pgxpool.Pool {
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
	name := fmt.Sprintf("estevao_sync_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg, _ := pgxpool.ParseConfig(base)
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close(ctx)
	})
	if _, err := pool.Exec(ctx, dbfiles.Schema); err != nil {
		t.Fatal(err)
	}
	if err := Load(ctx, pool, "../../seeds", nil); err != nil {
		t.Fatal(err)
	}
	return pool
}

// editFile rewrites one dataset file through f.
func editFile(t *testing.T, path string, f func([]map[string]any) []map[string]any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	out, _ := json.MarshalIndent(f(rows), "", " ")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func changedTables(reports []TableReport) map[string]TableReport {
	out := map[string]TableReport{}
	for _, r := range reports {
		if r.Changed() {
			out[r.Table] = r
		}
	}
	return out
}

func TestSync(t *testing.T) {
	pool := seededDatabase(t)
	ctx := context.Background()

	reports, err := Sync(ctx, pool, "../../seeds", SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := changedTables(reports); len(c) > 0 {
		t.Fatalf("a database loaded from the dataset differs from it: %+v", c)
	}

	dir := filepath.Join(t.TempDir(), "seeds")
	if out, err := exec.Command("cp", "-r", "../../seeds", dir).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	book := filepath.Join(dir, "prayer_books", "loc_2015")
	var editedSlug string
	editFile(t, filepath.Join(book, "liturgical_texts.json"), func(rows []map[string]any) []map[string]any {
		editedSlug = rows[0]["slug"].(string)
		rows[0]["content"] = rows[0]["content"].(string) + " (sync)"
		rows[0]["audio_url"] = "https://example.org/must-not-be-written.mp3" // a runtime column
		return rows
	})
	editFile(t, filepath.Join(book, "celebrations.json"), func(rows []map[string]any) []map[string]any {
		c := map[string]any{}
		for k, v := range rows[len(rows)-1] {
			c[k] = v
		}
		c["name"] = "Celebração do Teste de Sync"
		return append(rows, c)
	})
	editFile(t, filepath.Join(book, "collects.json"), func(rows []map[string]any) []map[string]any {
		c := map[string]any{}
		for k, v := range rows[0] {
			c[k] = v
		}
		c["celebration"], c["season"], c["sunday_reference"], c["text"] = "Celebração do Teste de Sync", nil, nil, "Coleta do teste"
		return append(rows, c)
	})

	var idBefore int64
	var audioBefore *string
	textQuery := `SELECT t.id, t.audio_url FROM liturgical_texts t JOIN prayer_books p ON p.id = t.prayer_book_id WHERE p.code = 'loc_2015' AND t.slug = $1`
	if err := pool.QueryRow(ctx, textQuery, editedSlug).Scan(&idBefore, &audioBefore); err != nil {
		t.Fatal(err)
	}
	touchedBefore := map[string]time.Time{}
	rows, _ := pool.Query(ctx, `SELECT code, updated_at FROM prayer_books WHERE code IN ('loc_2015', 'loc_2019')`)
	for rows.Next() {
		var code string
		var at time.Time
		_ = rows.Scan(&code, &at)
		touchedBefore[code] = at
	}
	rows.Close()

	plan, err := Sync(ctx, pool, dir, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := changedTables(plan)
	if c["liturgical_texts"].Updated != 1 || c["celebrations"].Inserted != 1 || c["collects"].GroupsReplaced != 1 || len(c) != 3 {
		t.Fatalf("plan: %+v", c)
	}
	var n int64
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM celebrations WHERE name = 'Celebração do Teste de Sync'`).Scan(&n)
	if n != 0 {
		t.Fatal("a report-only sync wrote")
	}

	if _, err := Sync(ctx, pool, dir, SyncOptions{Apply: true}); err != nil {
		t.Fatal(err)
	}
	again, err := Sync(ctx, pool, dir, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := changedTables(again); len(c) > 0 {
		t.Fatalf("not converged after apply: %+v", c)
	}

	var idAfter int64
	var audioAfter *string
	var content string
	if err := pool.QueryRow(ctx, `SELECT t.id, t.audio_url, t.content FROM liturgical_texts t JOIN prayer_books p ON p.id = t.prayer_book_id WHERE p.code = 'loc_2015' AND t.slug = $1`, editedSlug).Scan(&idAfter, &audioAfter, &content); err != nil {
		t.Fatal(err)
	}
	if idAfter != idBefore || !strings.HasSuffix(content, " (sync)") {
		t.Fatalf("text id %d -> %d, content %q", idBefore, idAfter, content)
	}
	if (audioBefore == nil) != (audioAfter == nil) || (audioAfter != nil && *audioAfter != *audioBefore) {
		t.Fatalf("the runtime column audio_url was written: %v -> %v", audioBefore, audioAfter)
	}
	var collectText string
	if err := pool.QueryRow(ctx, `SELECT c.text FROM collects c JOIN celebrations e ON e.id = c.celebration_id WHERE e.name = 'Celebração do Teste de Sync'`).Scan(&collectText); err != nil {
		t.Fatalf("the new collect does not point at the new celebration: %v", err)
	}
	for code, before := range touchedBefore {
		var after time.Time
		_ = pool.QueryRow(ctx, `SELECT updated_at FROM prayer_books WHERE code = $1`, code).Scan(&after)
		if touched := after.After(before); touched != (code == "loc_2015") {
			t.Errorf("%s touched = %v", code, touched)
		}
	}
}

func TestSyncRejectsARowOutsideItsBook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prayer_books", "loc_x")
	_ = os.MkdirAll(path, 0o755)
	_ = os.WriteFile(filepath.Join(path, "celebrations.json"), []byte(`[{"name": "A", "prayer_book": "loc_y"}]`), 0o644)
	if _, err := readDataset(dir, tableByName("celebrations"), ""); err == nil || !strings.Contains(err.Error(), "loc_y") {
		t.Fatalf("got %v", err)
	}
}
