package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Manifest records, per table, the order in which the rows of each file
// were inserted: a table's rows may alternate between Prayer Books, and the
// ids (hence every ORDER BY id) follow that order.
type Manifest struct {
	Tables []ManifestTable `json:"tables"`
}

// ManifestTable is one table's segments, in insertion order.
type ManifestTable struct {
	Name     string    `json:"name"`
	Segments []Segment `json:"segments"`
	// Gaps are ids the seeds consumed without keeping a row (rows created
	// and deleted while seeding): before the row at Row (in the table's
	// insertion order), Skip ids are skipped, so the loaded ids are the
	// seeded ones.
	Gaps []Gap `json:"gaps,omitempty"`
	// Sequence is the id sequence's last value after seeding.
	Sequence int64 `json:"sequence"`
}

// Gap is a run of skipped ids.
type Gap struct {
	Row  int   `json:"row"`
	Skip int64 `json:"skip"`
}

// Segment is a run of consecutive rows from one file.
type Segment struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	Count int    `json:"count"`
}

type keyIndex map[int64][]string // id -> natural key (foreign keys as their natural keys)

func columns(ctx context.Context, pool *pgxpool.Pool, name string) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND is_generated = 'NEVER' ORDER BY ordinal_position`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func refFor(t *table, column string) *ref {
	for i := range t.refs {
		if t.refs[i].column == column {
			return &t.refs[i]
		}
	}
	return nil
}

func idOf(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	return n, true
}

func scalar(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// Export writes the dataset of the database behind pool into dir.
func Export(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	indexes := map[string]keyIndex{}
	manifest := Manifest{}
	files := map[string][][]byte{}
	var order []string
	for i := range tables {
		t := &tables[i]
		cols, err := columns(ctx, pool, t.name)
		if err != nil {
			return err
		}
		rows, err := pool.Query(ctx, `SELECT to_jsonb(t)::text FROM `+t.name+` t ORDER BY id`)
		if err != nil {
			return err
		}
		var raws []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				rows.Close()
				return err
			}
			raws = append(raws, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		index := keyIndex{}
		mt := ManifestTable{Name: t.name, Segments: []Segment{}}
		expected := int64(1)
		for rowNumber, raw := range raws {
			var row map[string]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &row); err != nil {
				return err
			}
			id, _ := idOf(row["id"])
			if id > expected {
				mt.Gaps = append(mt.Gaps, Gap{Row: rowNumber, Skip: id - expected})
			}
			expected = id + 1
			natural := func(r *ref, fkID int64) ([]string, error) {
				key, ok := indexes[r.table][fkID]
				if !ok {
					return nil, fmt.Errorf("%s.%s=%d: no %s row", t.name, r.column, fkID, r.table)
				}
				return key, nil
			}
			// the row's own natural key
			if len(t.keys) > 0 {
				var key []string
				for _, k := range t.keys {
					if r := refFor(t, k); r != nil {
						fk, _ := idOf(row[k])
						nk, err := natural(r, fk)
						if err != nil {
							return err
						}
						key = append(key, nk...)
					} else {
						key = append(key, scalar(row[k]))
					}
				}
				index[id] = key
			}
			book := ""
			var buf bytes.Buffer
			buf.WriteByte('{')
			first := true
			write := func(field string, value []byte) {
				if !first {
					buf.WriteString(", ")
				}
				first = false
				k := marshal(field)
				buf.Write(k)
				buf.WriteString(": ")
				buf.Write(value)
			}
			if t.book == "prayer_book_id" {
				if fk, ok := idOf(row["prayer_book_id"]); ok {
					book = indexes["prayer_books"][fk][0]
				}
			}
			for _, c := range cols {
				if skipped[c] {
					continue
				}
				r := refFor(t, c)
				if r == nil {
					if names, ok := t.enums[c]; ok {
						write(c, enumName(names, row[c]))
						continue
					}
					if containsString(t.relative, c) {
						write(c, relativeDate(row[c], row["created_at"]))
						continue
					}
					write(c, compact(row[c]))
					continue
				}
				fk, ok := idOf(row[c])
				if !ok {
					write(r.field, []byte("null"))
					continue
				}
				nk, err := natural(r, fk)
				if err != nil {
					return err
				}
				if t.book == c {
					book = nk[0]
				}
				if r.scope != "" && len(nk) > 1 && nk[0] == book {
					nk = nk[1:]
				}
				var v []byte
				if len(nk) == 1 {
					v = marshal(nk[0])
				} else {
					v = marshal(nk)
				}
				write(r.field, v)
			}
			buf.WriteByte('}')
			file := t.name + ".json"
			if t.book != "" {
				if book == "" {
					return fmt.Errorf("%s id %d belongs to no prayer book", t.name, id)
				}
				file = filepath.Join("prayer_books", book, t.name+".json")
			}
			if _, ok := files[file]; !ok {
				order = append(order, file)
			}
			n := len(files[file])
			files[file] = append(files[file], buf.Bytes())
			if s := len(mt.Segments); s > 0 && mt.Segments[s-1].File == file && mt.Segments[s-1].Start+mt.Segments[s-1].Count == n {
				mt.Segments[s-1].Count++
			} else {
				mt.Segments = append(mt.Segments, Segment{File: file, Start: n, Count: 1})
			}
		}
		var last int64
		var called bool
		if err := pool.QueryRow(ctx, `SELECT last_value, is_called FROM `+sequenceOf(t.name)).Scan(&last, &called); err != nil {
			return err
		}
		if called {
			mt.Sequence = last
		}
		indexes[t.name] = index
		manifest.Tables = append(manifest.Tables, mt)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, file := range order {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString("[\n")
		for i, line := range files[file] {
			b.Write(line)
			if i < len(files[file])-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("]\n")
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	m, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(m, '\n'), 0o644)
}

// compact normalizes a JSON value to one line (jsonb text already is,
// except for the spaces it puts after separators).
func compact(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	return b.Bytes()
}

// marshal encodes v without escaping HTML characters.
func marshal(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimRight(b.Bytes(), "\n")
}

// sequenceOf names a table's id sequence (Rails' <table>_id_seq).
func sequenceOf(table string) string { return table + "_id_seq" }

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// relativeDate writes a date as its distance from the day the row was
// created ("@today", "@today-2").
func relativeDate(value, createdAt json.RawMessage) []byte {
	d, err1 := time.Parse("2006-01-02", scalar(value))
	c, err2 := time.Parse("2006-01-02", scalar(createdAt)[:min(10, len(scalar(createdAt)))])
	if err1 != nil || err2 != nil {
		return compact(value)
	}
	days := int(d.Sub(c).Hours() / 24)
	switch {
	case days == 0:
		return marshal("@today")
	case days > 0:
		return marshal(fmt.Sprintf("@today+%d", days))
	}
	return marshal(fmt.Sprintf("@today%d", days))
}

// enumName writes an enum column by name (the raw value if unmapped).
func enumName(names map[string]int, raw json.RawMessage) []byte {
	var n int
	if json.Unmarshal(raw, &n) == nil {
		for name, v := range names {
			if v == n {
				return marshal(name)
			}
		}
	}
	return compact(raw)
}
