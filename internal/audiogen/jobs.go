package audiogen

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/audioadmin"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func init() {
	for class, perform := range map[string]func(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error){
		"CleanupAudioClipsJob":   performCleanup,
		"IndexAudioCatalogJob":   performIndex,
		"RegenerateAudioClipJob": performRegenerate,
		"GenerateBookAudioJob":   performGenerateBook,
		"PrewarmOfficeAudioJob":  performPrewarm,
	} {
		solidqueue.Register(class, solidqueue.Handler{Queue: "maintenance", Perform: withOperation(perform)})
	}
}

func i64(n int64) *int64 { return &n }

// partialFailure is a job's PartialFailure < StandardError.
func partialFailure(job, message string) error {
	return &rb.RubyError{Class: job + "::PartialFailure", Message: message}
}

// withOperation ports Audio::OperationJob#with_operation: the operation is
// marked running, then completed with the result hash or failed with the
// error, which is raised again.
func withOperation(perform func(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error)) func(context.Context, *solidqueue.Execution) error {
	return func(ctx context.Context, e *solidqueue.Execution) error {
		var op *int64
		if id := e.Kwarg("operation_id"); id != nil {
			if o := audioadmin.FindOperation(ctx, int64(rb.ToI(id))); o != nil {
				op = &o.ID
			}
		}
		if op != nil {
			audioadmin.MarkRunning(ctx, *op)
		}
		var result *rb.Map
		err := catch(func() error {
			var err error
			if result, err = perform(ctx, e, op); err != nil {
				return err
			}
			if op != nil {
				if result == nil {
					result = rb.NewMap()
				}
				return audioadmin.MarkCompleted(ctx, *op, progressOf(result), result)
			}
			return nil
		})
		if err != nil && op != nil {
			audioadmin.MarkFailed(ctx, *op, solidqueue.ClassOf(err), err.Error())
		}
		return err
	}
}

// progressOf ports update_progress!(values): the counters the values name.
func progressOf(m *rb.Map) audioadmin.Progress {
	get := func(k string) *int64 {
		if !m.Has(k) {
			return nil
		}
		return i64(int64(rb.ToI(m.Get(k))))
	}
	return audioadmin.Progress{Processed: get("processed_items"), Total: get("total_items"), Generated: get("generated_clips"),
		Skipped: get("skipped_clips"), Failed: get("failed_items"), Characters: get("generated_characters")}
}

func update(ctx context.Context, op *int64, p audioadmin.Progress) error {
	if op == nil {
		return nil
	}
	return audioadmin.UpdateProgress(ctx, *op, p)
}

// --- CleanupAudioClipsJob -------------------------------------------------------------

func performCleanup(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error) {
	filters := rb.NewMap()
	if pos := e.Positional(); len(pos) > 0 {
		m, ok := pos[0].(*rb.Map)
		if !ok && pos[0] != nil {
			return nil, &rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_h", pos[0])}
		}
		if m != nil {
			filters = m
		}
	}
	query := &audioadmin.ClipQuery{Filters: filters}
	if err := update(ctx, op, audioadmin.Progress{Total: i64(query.Count(ctx))}); err != nil {
		return nil, err
	}
	var processed, skipped, failed int64
	usageFilters := filters.Slice("prayer_book_code", "source_name")
	var last int64
	for {
		batch := query.Batch(ctx, last, 100)
		for _, clip := range batch {
			last = clip.ID
			var removed bool
			err := catch(func() error {
				var err error
				removed, err = CleanupClip(ctx, clip, usageFilters)
				if err != nil {
					return err
				}
				processed++
				if !removed {
					skipped++
				}
				return update(ctx, op, audioadmin.Progress{Processed: i64(processed), Skipped: i64(skipped), Failed: i64(failed),
					Generated: i64(0), Characters: i64(0)})
			})
			if err != nil {
				failed++
				slog.Error("[CleanupAudioClips] clip=" + strconv.FormatInt(clip.ID, 10) + " failed=" + solidqueue.ClassOf(err))
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	if failed > 0 {
		return nil, partialFailure("CleanupAudioClipsJob", strconv.FormatInt(failed, 10)+" audio clips could not be removed")
	}
	return rb.M("processed_items", processed, "skipped_clips", skipped, "failed_items", failed, "generated_clips", 0,
		"generated_characters", 0, "total_items", processed), nil
}

// --- IndexAudioCatalogJob ---------------------------------------------------------------

func performIndex(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error) {
	var list []*store.PrayerBook
	if code := e.Kwarg("prayer_book_code"); rb.Present(code) {
		pb, err := findBook(ctx, rb.ToS(code))
		if err != nil {
			return nil, err
		}
		list = []*store.PrayerBook{pb}
	} else {
		all, err := store.PrayerBooksWhere(ctx, "")
		if err != nil {
			return nil, err
		}
		list = all
	}
	n := int64(len(list))
	if err := update(ctx, op, audioadmin.Progress{Total: i64(n)}); err != nil {
		return nil, err
	}
	for i, pb := range list {
		if err := IndexCatalog(ctx, pb); err != nil {
			return nil, err
		}
		if err := update(ctx, op, audioadmin.Progress{Processed: i64(int64(i + 1))}); err != nil {
			return nil, err
		}
	}
	return rb.M("processed_items", n, "total_items", n), nil
}

// --- RegenerateAudioClipJob --------------------------------------------------------------

func performRegenerate(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error) {
	var raw any
	if pos := e.Positional(); len(pos) > 0 {
		raw = pos[0]
	}
	var clip *audioadmin.Clip
	if id, ok := findID(raw); ok {
		clip = audioadmin.FindClip(ctx, id)
	}
	if clip == nil {
		return nil, &rb.RubyError{Class: "ActiveRecord::RecordNotFound", Message: "Couldn't find AudioClip with 'id'=" + rb.Inspect(raw)}
	}
	if err := update(ctx, op, audioadmin.Progress{Total: i64(1)}); err != nil {
		return nil, err
	}
	var additional *string
	if v := e.Kwarg("additional_instructions"); rb.Present(v) {
		s := rb.ToS(v)
		additional = &s
	}
	id, characters, err := RegenerateCandidate(ctx, clip, additional, op)
	if err != nil {
		return nil, err
	}
	if op != nil {
		audioadmin.SetParameter(ctx, *op, "candidate_id", id)
	}
	chars := int64(characters)
	if err := update(ctx, op, audioadmin.Progress{Processed: i64(1), Generated: i64(1), Characters: i64(chars)}); err != nil {
		return nil, err
	}
	return rb.M("total_items", 1, "processed_items", 1, "generated_clips", 1, "generated_characters", chars), nil
}

// findID ports the integer cast of AudioClip.find(id).
func findID(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	case string:
		s := strings.TrimSpace(x)
		end := 0
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 0 {
			return 0, false
		}
		n, err := strconv.ParseInt(s[:end], 10, 64)
		return n, err == nil
	}
	return 0, false
}

// --- GenerateBookAudioJob -------------------------------------------------------------------

// StringsOf ports `Array(value).map(&:to_s)`.
func StringsOf(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, len(x))
		for i, s := range x {
			out[i] = rb.ToS(s)
		}
		return out
	}
	return []string{rb.ToS(v)}
}

func performGenerateBook(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error) {
	code := rb.ToS(e.KwargOr("prayer_book_code", books.DefaultCode))
	generate := e.KwargOr("generate", true)
	budget := audio.NewBudget(e.Kwarg("character_budget"))
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, web.NewDomainError("UnsupportedPrayerBook", "unknown prayer book: "+code, "")
	}
	corpora := StringsOf(e.Kwarg("translations"))
	if len(corpora) == 0 {
		if corpora, err = TranslationsFor(ctx, pb); err != nil {
			return nil, err
		}
	}
	lang := pb.Language
	catalog := NewCatalog(pb, corpora, audio.NewGenerator(audio.Current(&lang)))
	if op != nil {
		sources, err := catalog.Sources(ctx, nil)
		if err != nil {
			return nil, err
		}
		if err := update(ctx, op, audioadmin.Progress{Total: i64(int64(len(sources))), Processed: i64(0)}); err != nil {
			return nil, err
		}
	}
	var processed int64
	result, err := catalog.Call(ctx, rb.Truthy(generate), budget, nil, func(source string, t Totals) error {
		processed++
		slog.Info("[GenerateBookAudio] " + code + " " + source + " clips=" + strconv.FormatInt(t.Clips, 10) +
			" generated=" + strconv.FormatInt(t.Generated, 10) + " characters=" + strconv.FormatInt(t.Characters, 10))
		return update(ctx, op, audioadmin.Progress{Processed: i64(processed), Generated: i64(t.Generated),
			Skipped: i64(t.Clips - t.Generated), Failed: i64(t.Skipped), Characters: i64(t.Characters)})
	})
	if err != nil {
		return nil, err
	}
	out := result.Map()
	out.Set("generate", generate)
	return out, nil
}

// --- PrewarmOfficeAudioJob ------------------------------------------------------------------

const (
	prewarmDefaultDays = 9
	reporterInterval   = 3 * time.Second
	reporterClips      = 5
)

var fixedSourceNames = []string{"liturgical_texts", "collects", "psalms"}

// progressReporter ports Audio::ProgressReporter.
type progressReporter struct {
	op        int64
	lastAt    time.Time
	lastClips int64
	reported  bool
}

func (r *progressReporter) call(ctx context.Context, g *audio.Generator) {
	now := time.Now()
	if r.reported && g.GeneratedClips-r.lastClips < reporterClips && now.Sub(r.lastAt) < reporterInterval {
		return
	}
	r.persist(ctx, g, audioadmin.Progress{}, now)
}

func (r *progressReporter) sync(ctx context.Context, g *audio.Generator, p audioadmin.Progress) {
	r.persist(ctx, g, p, time.Now())
}

func (r *progressReporter) persist(ctx context.Context, g *audio.Generator, p audioadmin.Progress, now time.Time) {
	p.Generated, p.Characters = i64(g.GeneratedClips), i64(g.CharactersGenerated)
	if err := audioadmin.UpdateProgress(ctx, r.op, p); err != nil {
		slog.Warn("[Audio::ProgressReporter] " + solidqueue.ClassOf(err) + ": " + err.Error())
		return
	}
	r.lastAt, r.lastClips, r.reported = now, g.GeneratedClips, true
}

type prewarmStats struct {
	offices, characters, missing int64
	errors                       []string
}

// toDate ports `value&.to_date || Date.current` for the start_date keyword.
func toDate(v any) (civil.Date, error) {
	today := civil.FromTime(time.Now().In(rb.AppZone))
	if v == nil {
		return today, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, &rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_date", v)}
	}
	if rb.BlankString(s) {
		return today, nil
	}
	ymd, err := rb.DateParse(s, false)
	if err != nil {
		return 0, &rb.RubyError{Class: "Date::Error", Message: "invalid date"}
	}
	d, ok := civil.New(int(ymd.Y), int(ymd.M), int(ymd.D))
	if !ok {
		return 0, &rb.RubyError{Class: "Date::Error", Message: "invalid date"}
	}
	return d, nil
}

// hashArg ports `value.to_h` of a preferences/variant argument.
func hashArg(v any) (*rb.Map, error) {
	switch x := v.(type) {
	case nil:
		return rb.NewMap(), nil
	case *rb.Map:
		return x, nil
	}
	return nil, &rb.RubyError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("to_h", v)}
}

// VariantsArg ports `Array(variants).map(&:to_h)` (nil when blank).
func VariantsArg(v any) ([]*rb.Map, error) {
	if !rb.Present(v) {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	out := make([]*rb.Map, 0, len(list))
	for _, item := range list {
		m, err := hashArg(item)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func performPrewarm(ctx context.Context, e *solidqueue.Execution, op *int64) (*rb.Map, error) {
	from, err := toDate(e.Kwarg("start_date"))
	if err != nil {
		return nil, err
	}
	days := int64(rb.ToI(e.KwargOr("days", prewarmDefaultDays)))
	code := rb.ToS(e.KwargOr("prayer_book_code", books.DefaultCode))
	budget := audio.NewBudget(e.Kwarg("character_budget"))
	generate := rb.Truthy(e.KwargOr("generate", true))
	includeFixed := rb.Truthy(e.KwargOr("include_fixed_sources", true))
	preferences, err := hashArg(e.KwargOr("preferences", rb.NewMap()))
	if err != nil {
		return nil, err
	}
	variantList, err := VariantsArg(e.Kwarg("variants"))
	if err != nil {
		return nil, err
	}
	var reporter *progressReporter
	if op != nil {
		reporter = &progressReporter{op: *op}
	}
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	lang := "pt-BR"
	if pb != nil {
		lang = pb.Language
	}
	generator := audio.NewReusingGenerator(audio.Current(&lang))
	generator.Budget = budget
	if reporter != nil {
		generator.Progress = func(g *audio.Generator) { reporter.call(ctx, g) }
	}
	matrix, err := NewCoverageMatrix(ctx, code, StringsOf(e.Kwarg("offices")), preferences, variantList)
	if err != nil {
		return nil, err
	}
	stats := &prewarmStats{}
	total := int64(0)
	if op != nil {
		n, err := matrix.VariantCount(ctx)
		if err != nil {
			return nil, err
		}
		total = days * int64(n)
	}
	if err := update(ctx, op, audioadmin.Progress{Total: i64(total), Processed: i64(0), Generated: i64(generator.GeneratedClips),
		Characters: i64(generator.CharactersGenerated)}); err != nil {
		return nil, err
	}
	sync := func() {
		if reporter == nil {
			return
		}
		reporter.sync(ctx, generator, audioadmin.Progress{Processed: i64(stats.offices), Failed: i64(int64(len(stats.errors)))})
	}
	if includeFixed && pb != nil {
		err := catch(func() error {
			_, err := NewCatalog(pb, nil, generator).Call(ctx, generate, budget, fixedSourceNames, nil)
			return err
		})
		if err != nil {
			if !rescuable(err) {
				return nil, err
			}
			slog.Error("[PrewarmOfficeAudio] fixed sources failed: "+solidqueue.ClassOf(err), "error", err.Error())
			stats.errors = append(stats.errors, "fixed sources: "+solidqueue.ClassOf(err))
		}
	}
	sync()
	byOffice, err := matrix.ConfigurationsByOffice(ctx)
	if err != nil {
		return nil, err
	}
	for offset := int64(0); offset < days; offset++ {
		date := from.Add(int(offset))
		for _, o := range byOffice {
			for _, configuration := range o.Configurations {
				if err := warmOffice(ctx, generator, date, o.OfficeType, code, configuration, stats, generate); err != nil {
					return nil, err
				}
				sync()
				if budget != nil && stats.characters > budget.Limit {
					return nil, &audio.BudgetExceeded{Message: "stopped after " + strconv.FormatInt(stats.offices, 10) + " offices: " +
						strconv.FormatInt(stats.characters, 10) + " characters exceeds the budget of " + budget.Text}
				}
			}
		}
	}
	variantCount, _ := matrix.VariantCount(ctx)
	slog.Info("[PrewarmOfficeAudio] book=" + code + " variants=" + strconv.Itoa(variantCount) + " offices=" +
		strconv.FormatInt(stats.offices, 10) + " characters=" + strconv.FormatInt(stats.characters, 10) +
		" errors=" + strconv.Itoa(len(stats.errors)))
	if len(stats.errors) > 0 {
		return nil, partialFailure("PrewarmOfficeAudioJob", strings.Join(stats.errors, "; "))
	}
	if generate && stats.missing > 0 {
		return nil, partialFailure("PrewarmOfficeAudioJob", strconv.FormatInt(stats.missing, 10)+" office audio segments remain missing")
	}
	return rb.M("offices", stats.offices, "characters", stats.characters, "missing", stats.missing,
		"generated_clips", generator.GeneratedClips, "generated_characters", generator.CharactersGenerated), nil
}

// warmOffice ports warm_office: the office assembled as a request would,
// then its track built (and bought) with the usage recorded.
func warmOffice(ctx context.Context, g *audio.Generator, date civil.Date, officeType, code string, configuration *rb.Map,
	stats *prewarmStats, generate bool) error {
	err := catch(func() error {
		office, err := BuildOffice(ctx, date, officeType, code, configuration)
		if err != nil {
			return err
		}
		recorder := NewUsageRecorder(code, "office:"+officeType)
		track, err := audio.BuildTrack(ctx, g, office, generate, recorder.Record)
		if err != nil {
			return err
		}
		if err := recorder.Flush(ctx); err != nil {
			return err
		}
		stats.characters += int64(track.Characters)
		stats.missing += int64(track.Missing)
		stats.offices++
		return nil
	})
	if err != nil {
		if !rescuable(err) {
			return err
		}
		slog.Error("[PrewarmOfficeAudio] "+date.ISO()+" "+officeType+" failed: "+solidqueue.ClassOf(err), "error", err.Error())
		stats.errors = append(stats.errors, date.ISO()+" "+officeType+": "+solidqueue.ClassOf(err))
	}
	return nil
}
