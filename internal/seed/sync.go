package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// syncSpec says how Sync matches a table's dataset rows with its database
// rows. identity fields (dataset names) identify one row; group fields
// identify a set of rows that is replaced as a whole when it changes (for
// tables with no row identity). Either way the row's Prayer Book directory
// is part of the identity.
type syncSpec struct {
	identity []string
	group    []string
	// runtime columns belong to a process, not to the dataset: an
	// existing row keeps its value (a new row still gets the dataset's).
	runtime []string
}

// syncSpecs covers the reference content. Tables absent here are never
// synchronized: feature_flags and background_track_assets are runtime
// state, and the system users, their built-in life rules and the sample
// journals are created once by `seed load`.
var syncSpecs = map[string]syncSpec{
	"liturgical_colors":      {identity: []string{"name"}},
	"liturgical_seasons":     {identity: []string{"name"}},
	"bible_versions":         {identity: []string{"code"}},
	"bible_texts":            {identity: []string{"translation", "book", "chapter", "verse"}},
	"prayer_books":           {identity: []string{"code"}},
	"celebrations":           {identity: []string{"name"}},
	"preference_categories":  {identity: []string{"key"}},
	"preference_definitions": {identity: []string{"category", "key"}},
	// The legacy narration (GenerateLiturgicalAudioJob) writes the audio
	// columns.
	"liturgical_texts":      {identity: []string{"slug"}, runtime: []string{"audio_generation_status", "audio_url", "audio_urls"}},
	"collects":              {group: []string{"celebration", "season", "sunday_reference", "language_style"}},
	"lectionary_readings":   {identity: []string{"date_reference", "cycle", "service_type", "reading_type", "service_variant", "celebration"}},
	"psalms":                {identity: []string{"number"}},
	"psalm_cycles":          {identity: []string{"cycle_type", "week_number", "day_of_week", "office_type"}},
	"background_categories": {identity: []string{"slug"}},
	// BackgroundMusic::Publisher sets published_at, and the importer the
	// measured duration (BackgroundMusic::Seed keeps it too).
	"background_tracks":           {identity: []string{"slug"}, runtime: []string{"published_at", "duration_ms"}},
	"background_track_categories": {identity: []string{"track", "category"}},
	"background_track_placements": {identity: []string{"track", "facet", "key", "prayer_book_code"}},
}

// SyncOptions select what Sync does.
type SyncOptions struct {
	// Book limits the sync to one Prayer Book: its row in prayer_books and
	// its directory's tables. Empty is every table of syncSpecs.
	Book string
	// Apply writes the plan; without it Sync only reports.
	Apply bool
	// LockTimeout bounds each wait for a row or table lock (default 5 s).
	LockTimeout time.Duration
}

// TableReport is what Sync found (or did) in one table.
type TableReport struct {
	Table          string
	Inserted       int
	Updated        int
	GroupsReplaced int
	RowsReplaced   int
	Unchanged      int
	// OnlyInDatabase rows are not in the dataset. Sync never deletes them:
	// user data may point at them (user_audio_usages cascades from
	// liturgical_texts) and clients may hold their ids. Removing one is a
	// migration someone reviews.
	OnlyInDatabase int
	Samples        []string
}

// Changed reports whether the table has anything to write.
func (r TableReport) Changed() bool { return r.Inserted+r.Updated+r.GroupsReplaced > 0 }

// syncRow is a row in dataset form.
type syncRow struct {
	file   string
	book   string
	id     int64 // database rows only
	fields map[string]json.RawMessage
	order  []string
}

// Sync reconciles the reference content of a live database with the
// dataset in dir: rows are matched on their natural identity; a changed row
// is updated in place (its id, which user data and clients may hold, is
// kept); a new row is inserted; a changed collect group is replaced in the
// dataset's order. Everything is one transaction, and every Prayer Book
// whose content changed is touched, which moves every cache keyed on its
// updated_at (the Redis caches and each instance's in-process ones).
func Sync(ctx context.Context, pool *pgxpool.Pool, dir string, opts SyncOptions) ([]TableReport, error) {
	if opts.LockTimeout == 0 {
		opts.LockTimeout = 5 * time.Second
	}
	indexes := map[string]keyIndex{}
	type planned struct {
		t       *table
		spec    syncSpec
		inserts []*syncRow
		updates []update
		groups  []groupReplace
	}
	var plans []planned
	var reports []TableReport
	touched := map[string]bool{}
	for i := range tables {
		t := &tables[i]
		cols, err := columns(ctx, pool, t.name)
		if err != nil {
			return nil, err
		}
		spec, managed := syncSpecs[t.name]
		skipTable := !managed || (opts.Book != "" && t.book == "" && t.name != "prayer_books")
		var want []*syncRow
		if !skipTable {
			if want, err = readDataset(dir, t, opts.Book); err != nil {
				return nil, err
			}
		}
		// Referenced tables are indexed even when not synchronized.
		have, err := readDatabase(ctx, pool, t, cols, indexes, want, skipTable)
		if err != nil {
			return nil, err
		}
		if skipTable {
			continue
		}
		if opts.Book != "" {
			have = filterBook(t, have, opts.Book)
		}
		p := planned{t: t, spec: spec}
		rep := TableReport{Table: t.name}
		if spec.group != nil {
			p.groups, rep = planGroups(t, spec, want, have)
		} else {
			p.inserts, p.updates, rep, err = planRows(t, spec, want, have)
			if err != nil {
				return nil, err
			}
		}
		rep.Table = t.name
		if rep.Changed() {
			for _, r := range p.inserts {
				touched[bookOf(t, r)] = true
			}
			for _, u := range p.updates {
				touched[bookOf(t, u.want)] = true
			}
			for _, g := range p.groups {
				touched[g.book] = true
			}
		}
		plans = append(plans, p)
		reports = append(reports, rep)
	}
	delete(touched, "")
	if !opts.Apply {
		return reports, nil
	}
	changed := false
	for _, r := range reports {
		changed = changed || r.Changed()
	}
	if !changed {
		return reports, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = %d", opts.LockTimeout.Milliseconds())); err != nil {
		return nil, err
	}
	ids := map[string]map[string]int64{}
	for i := range tables {
		if len(tables[i].keys) > 0 {
			if ids[tables[i].name], err = naturalKeys(ctx, tx, &tables[i]); err != nil {
				return nil, err
			}
		}
	}
	now := marshal(time.Now().UTC().Format("2006-01-02T15:04:05.999999"))
	for _, p := range plans {
		t := p.t
		for _, g := range p.groups {
			if len(g.deleteIDs) > 0 {
				if _, err := tx.Exec(ctx, `DELETE FROM `+t.name+` WHERE id = ANY($1)`, g.deleteIDs); err != nil {
					return nil, fmt.Errorf("%s: %w", t.name, err)
				}
			}
			p.inserts = append(p.inserts, g.rows...)
		}
		if err := insertRows(ctx, tx, t, p.inserts, ids, now); err != nil {
			return nil, err
		}
		for _, u := range p.updates {
			if err := updateRow(ctx, tx, t, u, ids, now); err != nil {
				return nil, err
			}
		}
		if len(t.keys) > 0 && (len(p.inserts) > 0 || len(p.updates) > 0) {
			if ids[t.name], err = naturalKeys(ctx, tx, t); err != nil {
				return nil, err
			}
		}
	}
	if len(touched) > 0 {
		codes := make([]string, 0, len(touched))
		for c := range touched {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		if _, err := tx.Exec(ctx, `UPDATE prayer_books SET updated_at = $2::timestamp WHERE code = ANY($1)`, codes, scalar(now)); err != nil {
			return nil, err
		}
	}
	return reports, tx.Commit(ctx)
}

// bookOf is the Prayer Book a row's change invalidates.
func bookOf(t *table, r *syncRow) string {
	if t.name == "prayer_books" {
		return scalar(r.fields["code"])
	}
	return r.book
}

// readDataset reads a table's dataset rows (one book's, when book is set).
func readDataset(dir string, t *table, book string) ([]*syncRow, error) {
	var files []string
	switch {
	case t.book != "" && book != "":
		files = []string{filepath.Join("prayer_books", book, t.name+".json")}
	case t.book != "":
		matches, err := filepath.Glob(filepath.Join(dir, "prayer_books", "*", t.name+".json"))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			rel, _ := filepath.Rel(dir, m)
			files = append(files, rel)
		}
		sort.Strings(files)
	default:
		files = []string{t.name + ".json"}
	}
	var out []*syncRow
	for _, file := range files {
		b, err := os.ReadFile(filepath.Join(dir, file))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows, err := decodeOrdered(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		bookDir := ""
		if t.book != "" {
			bookDir = filepath.Base(filepath.Dir(file))
		}
		for i, r := range rows {
			r.file, r.book = file, bookDir
			if t.book == "prayer_book_id" {
				if pb := scalar(r.fields["prayer_book"]); pb != bookDir {
					return nil, fmt.Errorf("%s row %d: prayer_book %q is not the directory's book %q", file, i+1, pb, bookDir)
				}
			}
			normalizeScoped(t, r)
			if t.name == "prayer_books" && book != "" && scalar(r.fields["code"]) != book {
				continue
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// normalizeScoped writes a same-book scoped reference the way Export does
// (without the book), so a hand-written full key still matches.
func normalizeScoped(t *table, r *syncRow) {
	for _, ref := range t.refs {
		v := r.fields[ref.field]
		if ref.scope == "" || len(v) == 0 || v[0] != '[' {
			continue
		}
		var key []string
		if json.Unmarshal(v, &key) == nil && len(key) == len(ref.keys) && key[0] == r.book {
			if len(key) == 2 {
				r.fields[ref.field] = marshal(key[1])
			} else {
				r.fields[ref.field] = marshal(key[1:])
			}
		}
	}
}

// decodeOrdered parses a dataset file keeping each row's field order.
func decodeOrdered(b []byte) ([]*syncRow, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return nil, fmt.Errorf("expected a JSON array of rows")
	}
	var out []*syncRow
	for dec.More() {
		if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
			return nil, fmt.Errorf("row %d is not a JSON object", len(out)+1)
		}
		r := &syncRow{fields: map[string]json.RawMessage{}}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := tok.(string)
			var v json.RawMessage
			if err := dec.Decode(&v); err != nil {
				return nil, err
			}
			if _, dup := r.fields[key]; dup {
				return nil, fmt.Errorf("row %d repeats %q", len(out)+1, key)
			}
			r.fields[key] = v
			r.order = append(r.order, key)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// readDatabase reads a table in dataset form and records its natural keys
// for the tables that reference it. bible_texts is read only for the
// translations the dataset has (the full Bibles come from the bible import,
// not from seeds).
func readDatabase(ctx context.Context, pool *pgxpool.Pool, t *table, cols []string, indexes map[string]keyIndex, want []*syncRow, indexOnly bool) ([]*syncRow, error) {
	if indexOnly && len(t.keys) == 0 {
		return nil, nil
	}
	query := `SELECT to_jsonb(t)::text FROM ` + t.name + ` t`
	var args []any
	if t.name == "bible_texts" {
		set := map[string]bool{}
		for _, r := range want {
			set[scalar(r.fields["translation"])] = true
		}
		translations := make([]string, 0, len(set))
		for k := range set {
			translations = append(translations, k)
		}
		query += ` WHERE translation = ANY($1)`
		args = append(args, translations)
	}
	rows, err := pool.Query(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	var raws []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		raws = append(raws, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	index := keyIndex{}
	var out []*syncRow
	for _, raw := range raws {
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			return nil, err
		}
		id, _ := idOf(row["id"])
		if len(t.keys) > 0 {
			key, err := ownKey(t, row, indexes)
			if err != nil {
				return nil, err
			}
			index[id] = key
		}
		if indexOnly {
			continue
		}
		book, fields, err := encodeRow(t, cols, row, indexes)
		if err != nil {
			return nil, err
		}
		r := &syncRow{id: id, book: book, fields: map[string]json.RawMessage{}}
		for _, f := range fields {
			r.fields[f.name] = f.value
			r.order = append(r.order, f.name)
		}
		out = append(out, r)
	}
	indexes[t.name] = index
	return out, nil
}

func filterBook(t *table, rows []*syncRow, book string) []*syncRow {
	var out []*syncRow
	for _, r := range rows {
		if (t.name == "prayer_books" && scalar(r.fields["code"]) == book) || (t.name != "prayer_books" && r.book == book) {
			out = append(out, r)
		}
	}
	return out
}

func identityOf(r *syncRow, fields []string) string {
	var b strings.Builder
	b.WriteString(r.book)
	for _, f := range fields {
		b.WriteByte(0)
		b.Write(canonical(r.fields[f]))
	}
	return b.String()
}

// canonical re-encodes a JSON value so formatting and key order do not
// count as differences (numbers keep their text: 1.0 is not 1).
func canonical(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return raw
	}
	return marshal(v)
}

func sameValue(a, b json.RawMessage) bool {
	dec := func(raw json.RawMessage) any {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if len(raw) == 0 || d.Decode(&v) != nil {
			return nil
		}
		return v
	}
	return reflect.DeepEqual(dec(a), dec(b))
}

type update struct {
	want    *syncRow
	id      int64
	changed []string // dataset field names
}

// planRows matches keyed rows.
func planRows(t *table, spec syncSpec, want, have []*syncRow) (inserts []*syncRow, updates []update, rep TableReport, err error) {
	byID := map[string]*syncRow{}
	for _, h := range have {
		byID[identityOf(h, spec.identity)] = h
	}
	seen := map[string]bool{}
	for _, w := range want {
		key := identityOf(w, spec.identity)
		if seen[key] {
			return nil, nil, rep, fmt.Errorf("%s: two rows share the identity %s", w.file, describeIdentity(w, spec.identity))
		}
		seen[key] = true
		h, ok := byID[key]
		if !ok {
			inserts = append(inserts, w)
			rep.Inserted++
			rep.sample("new " + describeIdentity(w, spec.identity))
			continue
		}
		var changed []string
		for _, f := range w.order {
			if _, known := h.fields[f]; !known {
				return nil, nil, rep, fmt.Errorf("%s: %q is not a column of %s", w.file, f, t.name)
			}
			if containsString(spec.runtime, f) {
				continue
			}
			if !sameValue(w.fields[f], h.fields[f]) {
				changed = append(changed, f)
			}
		}
		if len(changed) == 0 {
			rep.Unchanged++
			continue
		}
		updates = append(updates, update{want: w, id: h.id, changed: changed})
		rep.Updated++
		rep.sample("changed " + describeIdentity(w, spec.identity) + ": " + strings.Join(changed, ", "))
	}
	for key, h := range byID {
		if !seen[key] {
			rep.OnlyInDatabase++
			rep.sample("only in the database: " + describeIdentity(h, spec.identity))
		}
	}
	return inserts, updates, rep, nil
}

type groupReplace struct {
	book      string
	deleteIDs []int64
	rows      []*syncRow
}

// planGroups compares collect groups as ordered lists.
func planGroups(t *table, spec syncSpec, want, have []*syncRow) ([]groupReplace, TableReport) {
	var rep TableReport
	type group struct {
		book string
		rows []*syncRow
	}
	collect := func(rows []*syncRow) (map[string]*group, []string) {
		m := map[string]*group{}
		var order []string
		for _, r := range rows {
			k := identityOf(r, spec.group)
			g, ok := m[k]
			if !ok {
				g = &group{book: r.book}
				m[k] = g
				order = append(order, k)
			}
			g.rows = append(g.rows, r)
		}
		return m, order
	}
	wantG, wantOrder := collect(want)
	haveG, _ := collect(have)
	var out []groupReplace
	for _, k := range wantOrder {
		w := wantG[k]
		h := haveG[k]
		if h != nil && sameRows(w.rows, h.rows) {
			rep.Unchanged += len(w.rows)
			continue
		}
		g := groupReplace{book: w.book, rows: w.rows}
		if h != nil {
			for _, r := range h.rows {
				g.deleteIDs = append(g.deleteIDs, r.id)
			}
		}
		out = append(out, g)
		rep.GroupsReplaced++
		rep.RowsReplaced += len(w.rows)
		rep.sample(fmt.Sprintf("group %s: %d rows in the database, %d in the dataset", describeIdentity(w.rows[0], spec.group), len(g.deleteIDs), len(w.rows)))
	}
	for k, h := range haveG {
		if wantG[k] == nil {
			rep.OnlyInDatabase += len(h.rows)
			rep.sample("only in the database: group " + describeIdentity(h.rows[0], spec.group))
		}
	}
	return out, rep
}

func sameRows(want, have []*syncRow) bool {
	if len(want) != len(have) {
		return false
	}
	for i := range want {
		for _, f := range want[i].order {
			if !sameValue(want[i].fields[f], have[i].fields[f]) {
				return false
			}
		}
	}
	return true
}

func describeIdentity(r *syncRow, fields []string) string {
	parts := []string{}
	if r.book != "" {
		parts = append(parts, r.book)
	}
	for _, f := range fields {
		parts = append(parts, f+"="+string(canonical(r.fields[f])))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func (r *TableReport) sample(s string) {
	if len(r.Samples) < 5 {
		r.Samples = append(r.Samples, s)
	}
}

// columnsOf maps dataset fields to the columns they fill.
func columnsOf(t *table, fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		col := f
		for _, r := range t.refs {
			if r.field == f {
				col = r.column
			}
		}
		out = append(out, col)
	}
	return out
}

// insertRows inserts in dataset order, batching consecutive rows that fill
// the same columns (columns a row does not name keep their defaults).
func insertRows(ctx context.Context, tx pgx.Tx, t *table, rows []*syncRow, ids map[string]map[string]int64, now json.RawMessage) error {
	var batch []map[string]json.RawMessage
	var batchCols []string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := insert(ctx, tx, t.name, batchCols, batch)
		batch = nil
		return err
	}
	for _, r := range rows {
		values, err := decodeRow(t, r.fields, r.book, ids, now, r.file)
		if err != nil {
			return err
		}
		cols := append(columnsOf(t, r.order), "created_at", "updated_at")
		if strings.Join(cols, ",") != strings.Join(batchCols, ",") {
			if err := flush(); err != nil {
				return err
			}
			batchCols = cols
		}
		batch = append(batch, values)
		if len(batch) == insertBatch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// updateRow writes the changed columns of one row and its updated_at.
func updateRow(ctx context.Context, tx pgx.Tx, t *table, u update, ids map[string]map[string]int64, now json.RawMessage) error {
	values, err := decodeRow(t, u.want.fields, u.want.book, ids, now, u.want.file)
	if err != nil {
		return err
	}
	cols := append(columnsOf(t, u.changed), "updated_at")
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
	}
	list := strings.Join(quoted, ", ")
	payload, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE `+t.name+` SET (`+list+`) = (SELECT `+list+` FROM jsonb_populate_record(NULL::`+t.name+`, $1::jsonb)) WHERE id = $2`,
		string(payload), u.id)
	if err != nil {
		return fmt.Errorf("update %s id %d: %w", t.name, u.id, err)
	}
	return nil
}
