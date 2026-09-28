package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const insertBatch = 1000

// ErrNotEmpty reports a target database that already holds reference data.
type ErrNotEmpty struct{ Table string }

func (e *ErrNotEmpty) Error() string {
	return "refusing to seed: " + e.Table + " already has rows (the loader only fills an empty database)"
}

// Load inserts the dataset in dir into the database behind pool, in one
// transaction. Every seeded table must be empty: rebuilding reference data
// that user data already points at is not something a loader should decide.
func Load(ctx context.Context, pool *pgxpool.Pool, dir string, logf func(string, ...any)) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	for _, t := range tables {
		var n int64
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+t.name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return &ErrNotEmpty{Table: t.name}
		}
	}
	cache := map[string][]map[string]json.RawMessage{}
	read := func(file string) ([]map[string]json.RawMessage, error) {
		if rows, ok := cache[file]; ok {
			return rows, nil
		}
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(b, &rows); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		cache[file] = rows
		return rows, nil
	}
	now := marshal(time.Now().UTC().Format("2006-01-02T15:04:05.999999"))
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	ids := map[string]map[string]int64{} // table -> natural key -> id
	for _, mt := range manifest.Tables {
		t := tableByName(mt.Name)
		if t == nil {
			return fmt.Errorf("manifest names an unknown table %q", mt.Name)
		}
		cols, err := columns(ctx, pool, t.name)
		if err != nil {
			return err
		}
		var batch []map[string]json.RawMessage
		total := 0
		for _, seg := range mt.Segments {
			rows, err := read(seg.File)
			if err != nil {
				return err
			}
			if seg.Start+seg.Count > len(rows) {
				return fmt.Errorf("%s: segment beyond the file", seg.File)
			}
			book := ""
			if t.book != "" {
				book = filepath.Base(filepath.Dir(seg.File))
			}
			for _, row := range rows[seg.Start : seg.Start+seg.Count] {
				out := map[string]json.RawMessage{"created_at": now, "updated_at": now}
				for k, v := range row {
					out[k] = v
				}
				for _, r := range t.refs {
					v, ok := row[r.field]
					delete(out, r.field)
					if !ok || string(v) == "null" {
						out[r.column] = json.RawMessage("null")
						continue
					}
					var key []string
					if len(v) > 0 && v[0] == '[' {
						if err := json.Unmarshal(v, &key); err != nil {
							return err
						}
					} else {
						key = []string{scalar(v)}
					}
					if r.scope != "" && len(key) < len(r.keys) {
						key = append([]string{book}, key...)
					}
					id, ok := ids[r.table][strings.Join(key, "\x00")]
					if !ok {
						return fmt.Errorf("%s: %s %q not found", seg.File, r.field, key)
					}
					out[r.column] = marshal(id)
				}
				batch = append(batch, out)
				if len(batch) == insertBatch {
					if err := insert(ctx, tx, t.name, cols, batch); err != nil {
						return err
					}
					total += len(batch)
					batch = batch[:0]
				}
			}
		}
		if len(batch) > 0 {
			if err := insert(ctx, tx, t.name, cols, batch); err != nil {
				return err
			}
			total += len(batch)
		}
		if len(t.keys) > 0 {
			if ids[t.name], err = naturalKeys(ctx, tx, t); err != nil {
				return err
			}
		}
		if logf != nil {
			logf("%-28s %7d rows", t.name, total)
		}
	}
	return tx.Commit(ctx)
}

func insert(ctx context.Context, tx pgx.Tx, name string, cols []string, rows []map[string]json.RawMessage) error {
	var list []string
	for _, c := range cols {
		if c != "id" {
			list = append(list, `"`+c+`"`)
		}
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	joined := strings.Join(list, ", ")
	_, err = tx.Exec(ctx, `INSERT INTO `+name+` (`+joined+`) SELECT `+joined+`
		FROM jsonb_populate_recordset(NULL::`+name+`, $1::jsonb) WITH ORDINALITY AS r ORDER BY r.ordinality`, string(payload))
	if err != nil {
		return fmt.Errorf("insert %s: %w", name, err)
	}
	return nil
}

// naturalKeys maps each row's natural key (foreign keys resolved to theirs)
// to its id.
func naturalKeys(ctx context.Context, tx pgx.Tx, t *table) (map[string]int64, error) {
	var sel []string
	var joins []string
	for i, k := range t.keys {
		if r := refFor(t, k); r != nil {
			alias := fmt.Sprintf("k%d", i)
			if len(r.keys) != 1 {
				return nil, fmt.Errorf("%s: nested composite key %s", t.name, k)
			}
			joins = append(joins, " LEFT JOIN "+r.table+" "+alias+" ON "+alias+".id = t."+k)
			sel = append(sel, "COALESCE("+alias+"."+r.keys[0]+"::text, '')")
		} else {
			sel = append(sel, "COALESCE(t."+k+"::text, '')")
		}
	}
	rows, err := tx.Query(ctx, `SELECT t.id, `+strings.Join(sel, ", ")+` FROM `+t.name+` t`+strings.Join(joins, ""))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		vals := make([]any, len(t.keys)+1)
		var id int64
		strs := make([]string, len(t.keys))
		vals[0] = &id
		for i := range strs {
			vals[i+1] = &strs[i]
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		out[strings.Join(strs, "\x00")] = id
	}
	return out, rows.Err()
}
