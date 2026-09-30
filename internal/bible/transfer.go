package bible

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dodopok/estevao-api-go/internal/clock"
)

// A translation file is the one format Bible texts move in, whatever their
// source: gzipped JSON lines, a TransferHeader and then one TransferVerse
// per verse, in the table's order. `estevao bible export` writes it from any
// database; `estevao bible import` loads it. The Rails app had one importer
// per source (verse-table and MyBible SQLite modules, bolls.life JSON);
// converting a new source to this format is the only per-source step left.

// TransferFormat names the file format.
const TransferFormat = "estevao-bible-v1"

// TransferHeader is a file's first line.
type TransferHeader struct {
	Format      string `json:"format"`
	Translation string `json:"translation"`
	Verses      int    `json:"verses"`
}

// TransferVerse is one bible_texts row.
type TransferVerse struct {
	Book       string `json:"book"`
	BookNumber int    `json:"book_number"`
	Chapter    int    `json:"chapter"`
	Verse      int    `json:"verse"`
	Text       string `json:"text"`
}

// ExportTranslation writes a translation's verses to w.
func ExportTranslation(ctx context.Context, pool *pgxpool.Pool, translation string, w io.Writer) (int, error) {
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bible_texts WHERE translation = $1`, translation).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("no verses for translation %q", translation)
	}
	gz := gzip.NewWriter(w)
	bw := bufio.NewWriter(gz)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(TransferHeader{Format: TransferFormat, Translation: translation, Verses: n}); err != nil {
		return 0, err
	}
	rows, err := pool.Query(ctx, `SELECT book, book_number, chapter, verse, text FROM bible_texts WHERE translation = $1 ORDER BY id`, translation)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	written := 0
	for rows.Next() {
		var v TransferVerse
		if err := rows.Scan(&v.Book, &v.BookNumber, &v.Chapter, &v.Verse, &v.Text); err != nil {
			return 0, err
		}
		if err := enc.Encode(v); err != nil {
			return 0, err
		}
		written++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := bw.Flush(); err != nil {
		return 0, err
	}
	return written, gz.Close()
}

// ReadTranslation parses and validates a translation file. Warnings are
// accepted oddities (a book name that is not the catalog's); an error means
// the file must not be imported.
func ReadTranslation(r io.Reader) (TransferHeader, []TransferVerse, []string, error) {
	var h TransferHeader
	gz, err := gzip.NewReader(r)
	if err != nil {
		return h, nil, nil, fmt.Errorf("not a gzipped translation file: %w", err)
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	if !sc.Scan() {
		return h, nil, nil, errors.New("empty file")
	}
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil || h.Format != TransferFormat {
		return h, nil, nil, fmt.Errorf("the first line is not a %s header", TransferFormat)
	}
	if strings.TrimSpace(h.Translation) == "" || h.Translation != strings.ToLower(h.Translation) {
		return h, nil, nil, fmt.Errorf("translation %q must be a lowercase code", h.Translation)
	}
	var verses []TransferVerse
	var warnings []string
	seen := map[[3]int]bool{}
	renamed := map[string]string{}
	for line := 2; sc.Scan(); line++ {
		var v TransferVerse
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return h, nil, nil, fmt.Errorf("line %d: %w", line, err)
		}
		switch {
		case v.BookNumber <= 0 || v.Chapter <= 0 || v.Verse <= 0:
			return h, nil, nil, fmt.Errorf("line %d: book_number, chapter and verse must be positive", line)
		case BookNameByID(v.BookNumber) == "":
			return h, nil, nil, fmt.Errorf("line %d: book_number %d is not in the book catalog", line, v.BookNumber)
		case strings.TrimSpace(v.Book) == "" || strings.TrimSpace(v.Text) == "":
			return h, nil, nil, fmt.Errorf("line %d: empty book or text", line)
		}
		key := [3]int{v.BookNumber, v.Chapter, v.Verse}
		if seen[key] {
			return h, nil, nil, fmt.Errorf("line %d: %s %d:%d appears twice", line, v.Book, v.Chapter, v.Verse)
		}
		seen[key] = true
		if name := BookNameByID(v.BookNumber); name != v.Book {
			renamed[v.Book] = name
		}
		verses = append(verses, v)
	}
	if err := sc.Err(); err != nil {
		return h, nil, nil, err
	}
	if h.Verses != len(verses) {
		return h, nil, nil, fmt.Errorf("the header announces %d verses, the file has %d", h.Verses, len(verses))
	}
	for from, to := range renamed {
		warnings = append(warnings, fmt.Sprintf("book %q is %q in the catalog (kept as written)", from, to))
	}
	return h, verses, warnings, nil
}

// ImportTranslation writes verses as translation in one transaction. A
// translation already present is replaced only when replace is set.
func ImportTranslation(ctx context.Context, pool *pgxpool.Pool, translation string, verses []TransferVerse, replace bool) (replaced int64, err error) {
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var existing int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bible_texts WHERE translation = $1`, translation).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 && !replace {
			return fmt.Errorf("translation %q already has %d verses (use -replace to replace them)", translation, existing)
		}
		if existing > 0 {
			tag, err := tx.Exec(ctx, `DELETE FROM bible_texts WHERE translation = $1`, translation)
			if err != nil {
				return err
			}
			replaced = tag.RowsAffected()
		}
		now := clock.Now().UTC()
		src := pgx.CopyFromSlice(len(verses), func(i int) ([]any, error) {
			v := verses[i]
			return []any{v.Book, v.BookNumber, v.Chapter, v.Verse, v.Text, translation, now, now}, nil
		})
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"bible_texts"},
			[]string{"book", "book_number", "chapter", "verse", "text", "translation", "created_at", "updated_at"}, src)
		return err
	})
	return replaced, err
}

// TranslationStat is one line of Stats.
type TranslationStat struct {
	Translation string
	Verses      int64
	Books       int64
}

// Stats counts verses and books per translation (rake bible:stats).
func Stats(ctx context.Context, pool *pgxpool.Pool) ([]TranslationStat, error) {
	rows, err := pool.Query(ctx, `SELECT translation, count(*), count(DISTINCT book_number) FROM bible_texts GROUP BY translation ORDER BY translation`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (TranslationStat, error) {
		var s TranslationStat
		err := r.Scan(&s.Translation, &s.Verses, &s.Books)
		return s, err
	})
}
