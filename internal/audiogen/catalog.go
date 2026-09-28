package audiogen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// --- Audio::UsageRecorder ------------------------------------------------------------

const usageBatchSize = 500

type usageRecord struct{ clipKey, sourceKey string }

// UsageRecorder ports Audio::UsageRecorder: clip references buffered and
// written in batches.
type UsageRecorder struct {
	prayerBookCode, sourceName string
	records                    []usageRecord
}

// NewUsageRecorder ports UsageRecorder.new(prayer_book_code:, source_name:).
func NewUsageRecorder(prayerBookCode, sourceName string) *UsageRecorder {
	return &UsageRecorder{prayerBookCode: prayerBookCode, sourceName: sourceName}
}

// Record ports record(clip_key, source_key:).
func (u *UsageRecorder) Record(clipKey, sourceKey string) {
	if rb.BlankString(clipKey) {
		return
	}
	if rb.BlankString(sourceKey) {
		sourceKey = ""
	}
	u.records = append(u.records, usageRecord{clipKey, sourceKey})
}

// Flush ports flush!.
func (u *UsageRecorder) Flush(ctx context.Context) error {
	seen := map[usageRecord]bool{}
	var unique []usageRecord
	for _, r := range u.records {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	u.records = nil
	for start := 0; start < len(unique); start += usageBatchSize {
		if err := u.writeBatch(ctx, unique[start:min(start+usageBatchSize, len(unique))]); err != nil {
			return err
		}
	}
	return nil
}

func (u *UsageRecorder) writeBatch(ctx context.Context, batch []usageRecord) error {
	keys := make([]string, len(batch))
	for i, r := range batch {
		keys[i] = r.clipKey
	}
	rows, err := db.Q().Query(ctx, `SELECT key, id FROM audio_clips WHERE key = ANY($1)`, keys)
	if err != nil {
		return err
	}
	ids := map[string]int64{}
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			rows.Close()
			return err
		}
		ids[k] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var values []string
	var args []any
	for _, r := range batch {
		id, ok := ids[r.clipKey]
		if !ok {
			continue
		}
		n := len(args)
		values = append(values, "($"+strconv.Itoa(n+1)+", $"+strconv.Itoa(n+2)+", $"+strconv.Itoa(n+3)+", $"+strconv.Itoa(n+4)+
			", $"+strconv.Itoa(n+5)+", $"+strconv.Itoa(n+5)+")")
		args = append(args, id, u.prayerBookCode, u.sourceName, r.sourceKey, now)
	}
	if len(values) == 0 {
		return nil
	}
	_, err = db.Q().Exec(ctx, `INSERT INTO audio_clip_usages (audio_clip_id, prayer_book_code, source_name, source_key, created_at, updated_at)
		VALUES `+strings.Join(values, ", ")+`
		ON CONFLICT (audio_clip_id, prayer_book_code, source_name, source_key) DO NOTHING`, args...)
	return err
}

// sourceKeyFor ports source_key_for(context, text, segment_index).
func sourceKeyFor(t Text, segmentIndex int) string {
	var identity string
	switch {
	case rb.Truthy(t.Slug):
		identity = rb.ToS(t.Slug)
	case rb.Truthy(t.VerseNumber):
		identity = rb.ToS(t.VerseNumber)
	default:
		sum := sha256.Sum256([]byte(t.Text))
		identity = hex.EncodeToString(sum[:])[:16]
	}
	return identity + ":" + strconv.Itoa(segmentIndex)
}

// --- Audio::CatalogGenerator -----------------------------------------------------------

const sourceBatchSize = 500

// Totals are the catalogue counters.
type Totals struct {
	Clips, Generated, Characters, Skipped, MissingCharacters int64
}

// CatalogResult ports CatalogGenerator::Result#to_h.
type CatalogResult struct {
	Totals
	Sources []string
}

// Map renders the result in to_h order.
func (r CatalogResult) Map() *rb.Map {
	sources := make([]any, len(r.Sources))
	for i, s := range r.Sources {
		sources[i] = s
	}
	return rb.M("clips", r.Clips, "generated", r.Generated, "characters", r.Characters, "skipped", r.Skipped,
		"missing_characters", r.MissingCharacters, "sources", sources)
}

// Catalog ports Audio::CatalogGenerator.
type Catalog struct {
	pb           *store.PrayerBook
	translations []string
	generator    *audio.Generator
}

// NewCatalog ports CatalogGenerator.new(prayer_book, translations:, generator:, provider:).
func NewCatalog(pb *store.PrayerBook, translations []string, generator *audio.Generator) *Catalog {
	var uniq []string
	for _, t := range translations {
		if !containsStr(uniq, t) {
			uniq = append(uniq, t)
		}
	}
	return &Catalog{pb: pb, translations: uniq, generator: generator}
}

// Sources ports sources(source_names:).
func (c *Catalog) Sources(ctx context.Context, names []string) ([]Source, error) {
	available, err := SourcesFor(ctx, c.pb, c.translations)
	if err != nil || len(names) == 0 {
		return available, err
	}
	var out []Source
	for _, s := range available {
		for _, n := range names {
			if s.Name() == n || strings.HasPrefix(s.Name(), n+":") {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

// Call ports call(generate:, character_budget:, source_names:, &progress).
func (c *Catalog) Call(ctx context.Context, generate bool, budget *audio.Budget, names []string,
	progress func(source string, totals Totals) error) (*CatalogResult, error) {
	if budget != nil {
		c.generator.Budget = budget
	}
	result := &CatalogResult{Sources: []string{}}
	sources, err := c.Sources(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		result.Sources = append(result.Sources, s.Name())
		if err := c.runSource(ctx, s, &result.Totals, generate, budget); err != nil {
			return nil, err
		}
		if progress != nil {
			if err := progress(s.Name(), result.Totals); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (c *Catalog) runSource(ctx context.Context, s Source, totals *Totals, generate bool, budget *audio.Budget) error {
	recorder := NewUsageRecorder(c.pb.Code, s.Name())
	var batch []Text
	err := s.Each(ctx, func(t Text) error {
		batch = append(batch, t)
		if len(batch) < sourceBatchSize {
			return nil
		}
		err := c.processBatch(ctx, batch, recorder, totals, generate, budget)
		batch = nil
		return err
	})
	if err != nil {
		return err
	}
	if len(batch) > 0 {
		if err := c.processBatch(ctx, batch, recorder, totals, generate, budget); err != nil {
			return err
		}
	}
	return recorder.Flush(ctx)
}

func (c *Catalog) processBatch(ctx context.Context, batch []Text, recorder *UsageRecorder, totals *Totals, generate bool, budget *audio.Budget) error {
	entries := make([]audio.Entry, len(batch))
	for i, t := range batch {
		entries[i] = t.entry()
	}
	var err error
	if generate {
		err = c.generator.PreloadForGeneration(ctx, entries)
	} else {
		err = c.generator.Preload(ctx, entries)
	}
	if err != nil {
		return err
	}
	for _, t := range batch {
		segments, err := c.generator.CallSegments(ctx, t.entry(), generate)
		if err != nil {
			return err
		}
		for i, seg := range segments {
			if seg.Clip == nil {
				totals.Skipped++
				totals.MissingCharacters += int64(len([]rune(seg.Text)))
				continue
			}
			recorder.Record(seg.Clip.Key, sourceKeyFor(t, i))
			totals.Clips++
			if !seg.Clip.Generated {
				continue
			}
			totals.Generated++
			totals.Characters += int64(seg.Clip.CharacterCount)
			if budget != nil && totals.Characters > budget.Limit {
				return &audio.BudgetExceeded{Message: "stopped after " + strconv.FormatInt(totals.Generated, 10) + " clips: " +
					strconv.FormatInt(totals.Characters, 10) + " characters exceeds the budget of " + budget.Text}
			}
		}
	}
	return nil
}

// --- Audio::CatalogUsageIndexer ----------------------------------------------------------

// IndexCatalog ports CatalogUsageIndexer.new(prayer_book).call: the usage
// index rebuilt from the sources without calling the provider.
func IndexCatalog(ctx context.Context, pb *store.PrayerBook) error {
	lang := pb.Language
	provider := audio.Current(&lang)
	translations, err := TranslationsFor(ctx, pb)
	if err != nil {
		return err
	}
	generator := audio.NewGenerator(provider)
	sources, err := SourcesFor(ctx, pb, translations)
	if err != nil {
		return err
	}
	type pending struct{ key, source, sourceKey string }
	var buffer []pending
	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		var order []string
		bySource := map[string]*UsageRecorder{}
		for _, p := range buffer {
			r, ok := bySource[p.source]
			if !ok {
				r = NewUsageRecorder(pb.Code, p.source)
				bySource[p.source] = r
				order = append(order, p.source)
			}
			r.Record(p.key, p.sourceKey)
		}
		for _, name := range order {
			if err := bySource[name].Flush(ctx); err != nil {
				return err
			}
		}
		buffer = nil
		return nil
	}
	for _, s := range sources {
		err := s.Each(ctx, func(t Text) error {
			for i, normalized := range generator.SegmentsFor(t.entry()) {
				buffer = append(buffer, pending{audio.ClipKey(provider, normalized), s.Name(), sourceKeyFor(t, i)})
				if len(buffer) >= usageBatchSize {
					if err := flush(); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return flush()
}
