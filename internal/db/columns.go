package db

import (
	"context"
	"sync"
)

var (
	columnsMu    sync.Mutex
	columnsCache = map[string][]string{}
)

// Columns returns a table's columns in ordinal order: the order in which
// ActiveRecord's #attributes, and so #as_json, list them.
func Columns(ctx context.Context, table string) ([]string, error) {
	columnsMu.Lock()
	cols, ok := columnsCache[table]
	columnsMu.Unlock()
	if ok {
		return cols, nil
	}
	rows, err := Q().Query(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	columnsMu.Lock()
	columnsCache[table] = cols
	columnsMu.Unlock()
	return cols, nil
}

// OrderedAttributes ports as_json(only: keys): the listed keys that are
// columns, in the order of keys (Rails 7.1+ serializable_hash).
func OrderedAttributes(ctx context.Context, table string, keys []string) ([]string, error) {
	cols, err := Columns(ctx, table)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, c := range cols {
		have[c] = true
	}
	var out []string
	for _, k := range keys {
		if have[k] {
			out = append(out, k)
		}
	}
	return out, nil
}
