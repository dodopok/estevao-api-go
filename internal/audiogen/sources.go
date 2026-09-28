// Package audiogen ports the generation half of the Rails narration pipeline
// (app/services/audio and the audio jobs): the text sources of a prayer
// book, the catalogue and office runs that buy clips from the speech
// provider, the usage index, per-clip regeneration and cleanup, and the
// preference matrix those runs visit.
package audiogen

import (
	"context"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/dailyoffice"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// Text is one [text, line_type, metadata] a source yields.
type Text struct {
	Text        string
	LineType    string
	Slug        any
	VerseNumber any
	// hasContext is false when the source yields no metadata hash.
	hasContext bool
}

func (t Text) entry() audio.Entry {
	return audio.Entry{Type: t.LineType, Text: t.Text, Slug: t.Slug, VerseNumber: t.VerseNumber}
}

// Source ports one Audio::Sources::* class.
type Source interface {
	Name() string
	Each(ctx context.Context, yield func(Text) error) error
}

const findEachBatch = 1000

// findEach ports find_each: batches of 1000 in primary key order.
func findEach(ctx context.Context, query string, args []any, scan func(row interface{ Scan(...any) error }) (int64, error)) error {
	var last int64
	for {
		rows, err := db.Q().Query(ctx, query+` AND id > $`+itoa(len(args)+1)+` ORDER BY id ASC LIMIT `+itoa(findEachBatch),
			append(append([]any{}, args...), last)...)
		if err != nil {
			return err
		}
		n := 0
		for rows.Next() {
			id, err := scan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			last = id
			n++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if n < findEachBatch {
			return nil
		}
	}
}

// eachCollected runs find_each to completion before yielding, so a yield
// that queries the database never interleaves with an open result set.
func eachCollected(ctx context.Context, query string, args []any, scan func(row interface{ Scan(...any) error }) (int64, Text, bool, error),
	yield func(Text) error) error {
	var last int64
	for {
		rows, err := db.Q().Query(ctx, query+` AND id > $`+itoa(len(args)+1)+` ORDER BY id ASC LIMIT `+itoa(findEachBatch),
			append(append([]any{}, args...), last)...)
		if err != nil {
			return err
		}
		var batch []Text
		n := 0
		for rows.Next() {
			id, t, ok, err := scan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			last = id
			n++
			if ok {
				batch = append(batch, t)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, t := range batch {
			if err := yield(t); err != nil {
				return err
			}
		}
		if n < findEachBatch {
			return nil
		}
	}
}

// silentCategories is Audio::SpeakableLine::SILENT.
var silentCategories = []string{"citation", "heading", "reference", "rubric", "spacer", "subtitle"}

type liturgicalTexts struct{ pb *store.PrayerBook }

func (s liturgicalTexts) Name() string { return "liturgical_texts" }

// Each ports LiturgicalTexts#each_text: every fixed text said out loud.
func (s liturgicalTexts) Each(ctx context.Context, yield func(Text) error) error {
	return eachCollected(ctx, `SELECT id, content, category, slug FROM liturgical_texts
		WHERE prayer_book_id = $1 AND category NOT IN ('citation', 'heading', 'reference', 'rubric', 'spacer', 'subtitle')`,
		[]any{s.pb.ID}, func(row interface{ Scan(...any) error }) (int64, Text, bool, error) {
			var id int64
			var content, category, slug *string
			if err := row.Scan(&id, &content, &category, &slug); err != nil {
				return 0, Text{}, false, err
			}
			text := rb.Deref(content)
			if audio.ContextRequired(rb.ToS(text), s.pb.Language) {
				return id, Text{}, false, nil
			}
			return id, Text{Text: rb.ToS(text), LineType: rb.ToS(rb.Deref(category)), Slug: rb.Deref(slug), hasContext: true}, true, nil
		}, yield)
}

type collects struct{ pb *store.PrayerBook }

func (s collects) Name() string { return "collects" }

// Each ports Collects#each_text.
func (s collects) Each(ctx context.Context, yield func(Text) error) error {
	return eachCollected(ctx, `SELECT id, text FROM collects WHERE prayer_book_id = $1`, []any{s.pb.ID},
		func(row interface{ Scan(...any) error }) (int64, Text, bool, error) {
			var id int64
			var text *string
			if err := row.Scan(&id, &text); err != nil {
				return 0, Text{}, false, err
			}
			t := rb.ToS(rb.Deref(text))
			if audio.ContextRequired(t, s.pb.Language) {
				return id, Text{}, false, nil
			}
			return id, Text{Text: t, LineType: "prayer"}, true, nil
		}, yield)
}

type psalms struct{ pb *store.PrayerBook }

func (s psalms) Name() string { return "psalms:" + s.pb.Code }

// Each ports Psalms#each_text: the prayer-book Psalter verse by verse.
func (s psalms) Each(ctx context.Context, yield func(Text) error) error {
	var verses [][]Text
	err := findEach(ctx, `SELECT id, verses FROM psalms WHERE prayer_book_id = $1`, []any{s.pb.ID},
		func(row interface{ Scan(...any) error }) (int64, error) {
			var id int64
			var raw []byte
			if err := row.Scan(&id, &raw); err != nil {
				return 0, err
			}
			var list []Text
			if parsed, err := rb.ParseJSON(raw); err == nil && raw != nil {
				if arr, ok := parsed.([]any); ok {
					for _, v := range arr {
						m, _ := v.(*rb.Map)
						var number, text any
						if m != nil {
							number, text = m.Get("number"), m.Get("text")
						}
						list = append(list, Text{Text: dailyoffice.SanitizeBibleText(rb.ToS(text)), LineType: "reading_text",
							VerseNumber: number, hasContext: true})
					}
				}
			}
			verses = append(verses, list)
			return id, nil
		})
	if err != nil {
		return err
	}
	for _, list := range verses {
		for _, t := range list {
			if err := yield(t); err != nil {
				return err
			}
		}
	}
	return nil
}

type scripture struct{ translation string }

func (s scripture) Name() string { return "scripture:" + s.translation }

// Each ports Scripture#each_text: every verse of the corpus (find_each
// ignores the relation's order and walks the primary key).
func (s scripture) Each(ctx context.Context, yield func(Text) error) error {
	return eachCollected(ctx, `SELECT id, text, verse FROM bible_texts WHERE translation = $1`, []any{s.translation},
		func(row interface{ Scan(...any) error }) (int64, Text, bool, error) {
			var id int64
			var text *string
			var verse *int64
			if err := row.Scan(&id, &text, &verse); err != nil {
				return 0, Text{}, false, err
			}
			var number any
			if verse != nil {
				number = *verse
			}
			return id, Text{Text: dailyoffice.SanitizeBibleText(rb.ToS(rb.Deref(text))), LineType: "reading_text",
				VerseNumber: number, hasContext: true}, true, nil
		}, yield)
}

// SourcesFor ports Audio::Sources.for(prayer_book, translations:).
func SourcesFor(ctx context.Context, pb *store.PrayerBook, translations []string) ([]Source, error) {
	out := []Source{liturgicalTexts{pb}, collects{pb}}
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return nil, err
	}
	if d := defs.Get("psalm_translation"); d != nil && containsStr(d.ValidOptionValues(), "prayer_book") {
		var exists bool
		if err := db.Q().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM psalms WHERE prayer_book_id = $1)`, pb.ID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			out = append(out, psalms{pb})
		}
	}
	for _, t := range translations {
		out = append(out, scripture{t})
	}
	return out, nil
}

// TranslationsFor ports GenerateBookAudioJob.translations_for: the Bible
// versions and psalters a book can read from that exist as corpora.
func TranslationsFor(ctx context.Context, pb *store.PrayerBook) ([]string, error) {
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return nil, err
	}
	var values []string
	for _, key := range []string{"bible_version", "psalm_translation"} {
		if d := defs.Get(key); d != nil {
			values = append(values, d.ValidOptionValues()...)
		}
	}
	available, err := availableBibleVersions(ctx, pb.Language)
	if err != nil {
		return nil, err
	}
	values = append(values, available...)
	var out []string
	seen := map[string]bool{}
	for _, v := range values {
		if v == "bible_version" || seen[v] {
			continue
		}
		seen[v] = true
		ok, err := corpus(ctx, v)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// availableBibleVersions ports available_bible_versions:
// BibleVersion.for_language(language).pluck(:code) plus the default code.
func availableBibleVersions(ctx context.Context, language string) ([]string, error) {
	codes, err := bibleVersionCodesFor(ctx, language)
	if err != nil {
		return nil, err
	}
	def, err := store.DefaultBibleVersionFor(ctx, language)
	if err != nil {
		return nil, err
	}
	if def != nil {
		codes = append(codes, def.Code)
	}
	return codes, nil
}

// bibleVersionCodesFor ports BibleVersion.for_language(language).pluck(:code)
// (no order: the rows come back as PostgreSQL returns them).
func bibleVersionCodesFor(ctx context.Context, language string) ([]string, error) {
	candidates := langs.BibleLanguageCandidatesFor(language)
	if len(candidates) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(candidates))
	args := make([]any, len(candidates))
	for i, c := range candidates {
		placeholders[i] = "$" + itoa(i+1)
		args[i] = c
	}
	cond := `"bible_versions"."language" = $1`
	if len(candidates) > 1 {
		cond = `"bible_versions"."language" IN (` + strings.Join(placeholders, ", ") + `)`
	}
	rows, err := db.Q().Query(ctx, `SELECT "bible_versions"."code" FROM "bible_versions" WHERE "bible_versions"."is_active" = TRUE AND `+cond, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

// corpus ports GenerateBookAudioJob.corpus?.
func corpus(ctx context.Context, translation string) (bool, error) {
	var ok bool
	err := db.Q().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bible_texts WHERE translation = $1)`, translation).Scan(&ok)
	return ok, err
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func itoa(n int) string { return rb.ToS(int64(n)) }
