package rosary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/users"
)

const (
	staleAfterPublish     = 15 * time.Minute
	stalePublicationError = "publication discarded because prayer approval changed"
)

// --- Expansion and StepNumbering --------------------------------------------

// ExpandedStep is CustomRosaryPrayers::Expansion::Step.
type ExpandedStep struct {
	Position     int
	StepType     string
	Title        string
	DisplayTitle string
	Text         string
}

// Expand ports CustomRosaryPrayers::Expansion.call.
func (p *Prayer) Expand() []ExpandedStep {
	blocks := p.Blocks
	emit := func(bs []*Block) []*Step {
		var out []*Step
		for _, b := range bs {
			var beads []*Step
			for _, s := range b.Steps {
				for i := int64(0); i < toI(s.Get("repeat_count")); i++ {
					beads = append(beads, s)
				}
			}
			for i := int64(0); i < toI(b.Get("repeat_count")); i++ {
				out = append(out, beads...)
			}
		}
		return out
	}
	var seq []*Step
	for i := 0; i < len(blocks); {
		if blocks[i].Get("in_cycle") == true {
			end := i
			for end < len(blocks) && blocks[end].Get("in_cycle") == true {
				end++
			}
			for n := int64(0); n < toI(p.Get("cycle_repeat")); n++ {
				seq = append(seq, emit(blocks[i:end])...)
			}
			i = end
		} else {
			seq = append(seq, emit(blocks[i:i+1])...)
			i++
		}
	}
	if len(seq) > MaxExpandedSteps {
		seq = seq[:MaxExpandedSteps]
	}
	numbers := stepNumbering(seq)
	out := make([]ExpandedStep, len(seq))
	for i, s := range seq {
		title := s.Str("title")
		display := title
		if numbers[i] != 0 {
			display = title + " " + strconv.Itoa(numbers[i])
		}
		out[i] = ExpandedStep{Position: i + 1, StepType: s.Str("step_type"), Title: title, DisplayTitle: display, Text: s.Str("text")}
	}
	return out
}

// stepNumbering ports CustomRosaryPrayers::StepNumbering (0 = no number).
func stepNumbering(steps []*Step) []int {
	totals := map[string]int{}
	laps := 0
	previous := ""
	for i, s := range steps {
		t := s.Str("step_type")
		totals[t]++
		if t == "week" && (i == 0 || previous != "week") {
			laps++
		}
		previous = t
	}
	out := make([]int, len(steps))
	bead, cruciforms := 0, 0
	for i, s := range steps {
		switch s.Str("step_type") {
		case "week":
			bead++
			if totals["week"] > 1 {
				out[i] = bead
			}
		case "cruciform":
			bead = 0
			cruciforms++
			if totals["cruciform"] > 1 {
				if laps == 0 {
					out[i] = cruciforms
				} else {
					out[i] = (cruciforms-1)%laps + 1
				}
			}
		default:
			bead = 0
		}
	}
	return out
}

// --- PublicationPayload -----------------------------------------------------

// Payload ports CustomRosaryPrayers::PublicationPayload.call.
func (p *Prayer) Payload() (*rb.Map, error) {
	revision := p.Get("source_revision")
	if ar.Blank(revision) {
		revision = p.Get("updated_at").(time.Time).UTC().Format("2006-01-02T15:04:05.000000Z07:00")
	}
	slug := p.Get("strapi_slug")
	if ar.Blank(slug) {
		s := rb.Parameterize(rb.ToS(p.Get("title")))
		if s == "" {
			s = "prayer"
		}
		slug = s + "-" + strconv.FormatInt(p.ID, 10)
	}
	category, err := CategorySelection(p.Get("publication_category"))
	if err != nil {
		return nil, err
	}
	steps := []any{}
	for _, s := range p.Expand() {
		steps = append(steps, rb.M("order", s.Position, "type", s.StepType, "text", s.Text))
	}
	return rb.M("approved", true, "source_locale", p.Get("locale"), "prayer", rb.M(
		"source_api_prayer_id", p.ID, "source_client_id", p.Get("client_id"), "source_revision", revision,
		"source_type", "user_submission", "slug", slug, "name", p.Get("title"), "description", p.Get("description"),
		"category", category, "steps", steps)), nil
}

func sortValue(v any) any {
	switch x := v.(type) {
	case *rb.Map:
		keys := x.Keys()
		sort.Strings(keys)
		out := rb.NewMap()
		for _, k := range keys {
			out.Set(k, sortValue(x.Get(k)))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = sortValue(e)
		}
		return out
	}
	return v
}

// PayloadHash ports PublicationPayload.hash.
func PayloadHash(payload *rb.Map) string {
	sum := sha256.Sum256([]byte(rb.JSONGenerate(sortValue(payload))))
	return hex.EncodeToString(sum[:])
}

// --- Publish ------------------------------------------------------------------

// Publication failures, as in CustomRosaryPrayers::Publish / Unpublish.
type (
	InvalidState     struct{ Message string }
	PermanentFailure struct{ Message string }
	Failed           struct{ Message string }
)

func (e *InvalidState) Error() string     { return e.Message }
func (e *PermanentFailure) Error() string { return e.Message }
func (e *Failed) Error() string           { return e.Message }

// truncate ports String#truncate(1000).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

func loadForUpdate(ctx context.Context, tx pgx.Tx, id int64) (*Prayer, error) {
	list, err := LoadWithStructure(ctx, tx, "custom_rosary_prayers.ID = $1", "", id)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM custom_rosary_prayers WHERE id = $1 FOR UPDATE`, id); err != nil {
		return nil, err
	}
	return LoadAgain(ctx, tx, id)
}

// LoadAgain reloads a prayer (with structure) inside q.
func LoadAgain(ctx context.Context, q db.Querier, id int64) (*Prayer, error) {
	list, err := LoadWithStructure(ctx, q, "custom_rosary_prayers.ID = $1", "", id)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

func withLock(ctx context.Context, id int64, f func(tx pgx.Tx, p *Prayer) error) error {
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := loadForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if p == nil {
			return errRecordNotFound
		}
		return f(tx, p)
	})
}

var errRecordNotFound = errors.New("record not found")

func permanentHTTP(err *StrapiError) bool {
	return err.Status >= 400 && err.Status <= 499 && err.Status != 408 && err.Status != 429
}

// publish ports CustomRosaryPrayers::Publish.call.
func publish(ctx context.Context, id int64) error {
	client := newStrapiClient()
	var sourceHash string
	markFailed := func(cause error, allowUnhashed, retryable bool) {
		if sourceHash == "" && !allowUnhashed {
			return
		}
		_ = withLock(ctx, id, func(tx pgx.Tx, p *Prayer) error {
			if !p.approved() {
				return nil
			}
			if !allowUnhashed && !(p.Get("publication_status") == "publishing" && p.Get("source_hash") == sourceHash) {
				return nil
			}
			var retryAt any
			if retryable && toI(p.Get("publication_attempts")) < MaxAutomaticPublicationAttempts {
				retryAt = users.Now().Add(staleAfterPublish)
			}
			return p.UpdateColumns(ctx, tx, map[string]any{"publication_status": "failed",
				"publication_error": truncate(cause.Error(), 1000), "publication_started_at": nil, "publication_retry_at": retryAt})
		})
	}
	failPermanently := func(cause error) error {
		markFailed(cause, true, false)
		return &PermanentFailure{cause.Error()}
	}
	err := func() error {
		p, err := LoadAgain(ctx, db.Conn(ctx), id)
		if err != nil {
			return err
		}
		if p == nil {
			return errRecordNotFound
		}
		if !p.approved() {
			return &InvalidState{"only approved prayers can be published"}
		}
		payload, err := p.Payload()
		if err != nil {
			return err
		}
		sourceHash = PayloadHash(payload)
		claimed := false
		err = withLock(ctx, id, func(tx pgx.Tx, p *Prayer) error {
			if !p.approved() {
				return &InvalidState{"only approved prayers can be published"}
			}
			same := p.Get("source_hash") == sourceHash
			if p.Get("publication_status") == "published" && same && !ar.Blank(p.Get("strapi_document_id")) {
				return nil
			}
			if p.Get("publication_status") == "publishing" && same && p.Get("publication_started_at") != nil {
				if started := p.Get("publication_started_at").(time.Time); !started.Before(time.Now().Add(-staleAfterPublish)) {
					return nil
				}
			}
			if same && toI(p.Get("publication_attempts")) >= MaxAutomaticPublicationAttempts {
				return &PermanentFailure{"publication retry limit reached"}
			}
			attempts := int64(1)
			if same {
				attempts = toI(p.Get("publication_attempts")) + 1
			}
			claimed = true
			return p.UpdateColumns(ctx, tx, map[string]any{"publication_status": "publishing", "publication_error": nil,
				"source_hash": sourceHash, "publication_started_at": users.Now(), "publication_retry_at": nil,
				"publication_attempts": attempts})
		})
		if err != nil || !claimed {
			return err
		}
		response, err := client.publish(ctx, payload)
		if err != nil {
			return err
		}
		documentID := documentIDFrom(response)
		if ar.Blank(documentID) {
			return &Failed{"Strapi response did not include documentId"}
		}
		return finishPublication(ctx, id, payload, sourceHash, documentID)
	}()
	var invalid *InvalidState
	var permanent *PermanentFailure
	var category *InvalidCategory
	var strapi *StrapiError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &invalid):
		return err
	case errors.As(err, &permanent), errors.As(err, &category):
		return failPermanently(err)
	case errors.As(err, &strapi):
		if permanentHTTP(strapi) {
			return failPermanently(err)
		}
		markFailed(err, false, true)
		return &Failed{err.Error()}
	}
	markFailed(err, false, true)
	return &Failed{err.Error()}
}

func documentIDFrom(response any) any {
	m, ok := response.(*rb.Map)
	if !ok {
		return ar.CastString(response)
	}
	if v := m.Get("documentId"); ar.Truthy(v) {
		return v
	}
	if data, ok := m.Get("data").(*rb.Map); ok {
		return data.Get("documentId")
	}
	return nil
}

func finishPublication(ctx context.Context, id int64, payload *rb.Map, sourceHash string, documentID any) error {
	var cleanup string
	err := withLock(ctx, id, func(tx pgx.Tx, p *Prayer) error {
		current, err := p.Payload()
		if err != nil {
			return err
		}
		currentSource := p.Get("publication_status") == "publishing" && p.Get("source_hash") == sourceHash
		prayer := payload.Get("prayer").(*rb.Map)
		switch {
		case p.approved() && currentSource && PayloadHash(current) == sourceHash:
			slug := p.Get("strapi_slug")
			if ar.Blank(slug) {
				slug = prayer.Get("slug")
			}
			return p.UpdateColumns(ctx, tx, map[string]any{"publication_status": "published",
				"strapi_document_id": ar.CastString(documentID), "strapi_slug": slug, "source_revision": prayer.Get("source_revision"),
				"publication_error": nil, "publication_locale": p.Get("locale"), "published_at": users.Now(),
				"publication_started_at": nil, "publication_retry_at": nil})
		case !p.approved() || p.Get("is_public") != true:
			cleanup = rb.ToS(ar.CastString(documentID))
			doc := p.Get("strapi_document_id")
			if ar.Blank(doc) {
				doc = ar.CastString(documentID)
			}
			return p.UpdateColumns(ctx, tx, map[string]any{"publication_status": "unpublishing", "strapi_document_id": doc,
				"publication_error": stalePublicationError, "publication_started_at": nil, "publication_retry_at": users.Now()})
		}
		return nil
	})
	if errors.Is(err, errRecordNotFound) {
		enqueueStaleUnpublication(ctx, id, ar.CastString(documentID))
		return nil
	}
	if err != nil {
		return err
	}
	if cleanup != "" {
		enqueueStaleUnpublication(ctx, id, cleanup)
	}
	return nil
}

// enqueueStaleUnpublication ports Publish#enqueue_stale_unpublication: a
// failed enqueue leaves the unpublishing row for the reconciler.
func enqueueStaleUnpublication(ctx context.Context, id int64, documentID any) {
	if err := EnqueueUnpublish(ctx, id, documentID); err != nil {
		_, _ = db.Conn(ctx).Exec(ctx, `UPDATE custom_rosary_prayers SET publication_retry_at = $2
			WHERE id = $1 AND publication_status = 'unpublishing'`, id, users.Now())
		slog.Error("[CustomRosaryPrayer] stale publication cleanup enqueue failed", "error", err)
	}
}

// --- Unpublish ------------------------------------------------------------------

func unpublishDocument(ctx context.Context, documentID string, sourceID int64) error {
	if rb.BlankString(documentID) {
		return &InvalidState{"Strapi document is required"}
	}
	_, err := newStrapiClient().unpublish(ctx, documentID, &sourceID)
	var strapi *StrapiError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &strapi) && strapi.Status == 404:
		return nil
	case errors.As(err, &strapi) && permanentHTTP(strapi):
		return &PermanentFailure{err.Error()}
	}
	return &Failed{err.Error()}
}

// unpublishJob ports CustomRosaryPrayers::UnpublishJob#perform.
func unpublishJob(ctx context.Context, id int64, documentID string) error {
	p, err := LoadAgain(ctx, db.Conn(ctx), id)
	if err != nil {
		return err
	}
	if documentID == "" && p != nil {
		documentID = p.Str("strapi_document_id")
	}
	if rb.BlankString(documentID) {
		return nil
	}
	if p != nil && p.Get("publication_status") != "unpublishing" {
		return nil
	}
	if p != nil && p.Get("strapi_document_id") == documentID {
		must(p.UpdateColumns(ctx, db.Conn(ctx), map[string]any{"publication_started_at": users.Now(), "publication_retry_at": nil,
			"publication_error": nil}))
	}
	if err := unpublishDocument(ctx, documentID, id); err != nil {
		var permanent *PermanentFailure
		retryable := !errors.As(err, &permanent)
		if q, _ := LoadAgain(ctx, db.Conn(ctx), id); q != nil && q.Get("strapi_document_id") == documentID &&
			q.Get("publication_status") == "unpublishing" {
			var retryAt any
			if retryable {
				retryAt = users.Now().Add(staleAfterPublish)
			}
			must(q.UpdateColumns(ctx, db.Conn(ctx), map[string]any{"publication_started_at": nil, "publication_retry_at": retryAt,
				"publication_error": truncate(err.Error(), 1000)}))
		}
		return err
	}
	if p == nil {
		return nil
	}
	q, err := LoadAgain(ctx, db.Conn(ctx), id)
	if err != nil || q == nil || q.Get("strapi_document_id") != documentID {
		return err
	}
	next := "unpublished"
	if q.approved() && q.Get("is_public") == true {
		next = "pending"
	}
	must(q.UpdateColumns(ctx, db.Conn(ctx), map[string]any{"publication_status": next, "publication_started_at": nil,
		"publication_retry_at": nil, "publication_error": nil}))
	if next == "pending" {
		if err := EnqueuePublish(ctx, id); err != nil {
			must(q.UpdateColumns(ctx, db.Conn(ctx), map[string]any{"publication_retry_at": users.Now(),
				"publication_error": truncate(err.Error(), 1000)}))
		}
	}
	return nil
}

// --- jobs -----------------------------------------------------------------------

const (
	publishJob    = "CustomRosaryPrayers::PublishJob"
	unpublishJob_ = "CustomRosaryPrayers::UnpublishJob"
)

// rubyError names a Go failure with the Ruby class the job declarations
// (and a failed execution) know it by.
type rubyError struct {
	class string
	err   error
}

func (e *rubyError) Error() string     { return e.err.Error() }
func (e *rubyError) Unwrap() error     { return e.err }
func (e *rubyError) RubyClass() string { return e.class }

// asRuby maps the publication failures of scope ("CustomRosaryPrayers::Publish"
// or "...::Unpublish") to their classes.
func asRuby(scope string, err error) error {
	var invalid *InvalidState
	var permanent *PermanentFailure
	var failed *Failed
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errRecordNotFound):
		return &rubyError{"ActiveRecord::RecordNotFound", err}
	case errors.As(err, &invalid):
		return &rubyError{scope + "::InvalidState", err}
	case errors.As(err, &permanent):
		return &rubyError{scope + "::PermanentFailure", err}
	case errors.As(err, &failed):
		return &rubyError{scope + "::Failed", err}
	}
	return err
}

func init() {
	solidqueue.Register(publishJob, solidqueue.Handler{
		Queue: "default",
		Rescue: []solidqueue.Rescue{
			{Key: "[CustomRosaryPrayers::Publish::PermanentFailure]", Match: solidqueue.Classes("CustomRosaryPrayers::Publish::PermanentFailure")},
			{Key: "[CustomRosaryPrayers::Publish::Failed]", Match: solidqueue.Classes("CustomRosaryPrayers::Publish::Failed"), Attempts: 3},
			{Key: "[ActiveJob::DeserializationError]", Match: solidqueue.Classes("ActiveJob::DeserializationError")},
			{Key: "[CustomRosaryPrayers::Publish::InvalidState]", Match: solidqueue.Classes("CustomRosaryPrayers::Publish::InvalidState")},
		},
		Perform: func(ctx context.Context, e *solidqueue.Execution) error {
			return asRuby("CustomRosaryPrayers::Publish", publish(ctx, int64(rb.ToI(e.Arg(0)))))
		},
	})
	solidqueue.Register(unpublishJob_, solidqueue.Handler{
		Queue: "default",
		Rescue: []solidqueue.Rescue{
			{Key: "[CustomRosaryPrayers::Unpublish::Failed]", Match: solidqueue.Classes("CustomRosaryPrayers::Unpublish::Failed"), Attempts: 3},
			{Key: "[CustomRosaryPrayers::Unpublish::PermanentFailure]", Match: solidqueue.Classes("CustomRosaryPrayers::Unpublish::PermanentFailure")},
			{Key: "[ActiveJob::DeserializationError]", Match: solidqueue.Classes("ActiveJob::DeserializationError")},
		},
		Perform: func(ctx context.Context, e *solidqueue.Execution) error {
			return asRuby("CustomRosaryPrayers::Unpublish", unpublishJob(ctx, int64(rb.ToI(e.Arg(0))), rb.ToS(e.Arg(1))))
		},
	})
}

// EnqueuePublish ports CustomRosaryPrayers::PublishJob.perform_later(id).
func EnqueuePublish(ctx context.Context, id int64) error {
	_, err := solidqueue.PerformLater(ctx, publishJob, id)
	return err
}

// EnqueueUnpublish ports CustomRosaryPrayers::UnpublishJob.perform_later(id,
// document_id); documentID is nil or a string.
func EnqueueUnpublish(ctx context.Context, id int64, documentID any) error {
	_, err := solidqueue.PerformLater(ctx, unpublishJob_, id, documentID)
	return err
}

// --- ReconcilePublicationJobsJob ------------------------------------------------

const reconcileBatch = 100

func init() {
	solidqueue.Register("CustomRosaryPrayers::ReconcilePublicationJobsJob", solidqueue.Handler{Queue: "maintenance", Perform: reconcile})
}

func idPairs(ctx context.Context, sql string, args ...any) ([][2]any, error) {
	rows, err := db.Q().Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]any
	for rows.Next() {
		var id int64
		var doc *string
		if len(rows.FieldDescriptions()) == 2 {
			if err := rows.Scan(&id, &doc); err != nil {
				return nil, err
			}
		} else if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		var d any
		if doc != nil {
			d = *doc
		}
		out = append(out, [2]any{id, d})
	}
	return out, rows.Err()
}

// reconcile ports CustomRosaryPrayers::ReconcilePublicationJobsJob#perform.
func reconcile(ctx context.Context, _ *solidqueue.Execution) error {
	now := users.Now()
	staleBefore := now.Add(-staleAfterPublish)
	if _, err := db.Q().Exec(ctx, `UPDATE custom_rosary_prayers SET publication_status = 'pending', publication_started_at = NULL,
		publication_retry_at = $2, publication_error = 'Recovered after a worker interruption'
		WHERE publication_status = 'publishing' AND publication_started_at IS NOT NULL AND publication_started_at < $1`, staleBefore, now); err != nil {
		return err
	}
	if _, err := db.Q().Exec(ctx, `UPDATE custom_rosary_prayers SET publication_started_at = NULL,
		publication_retry_at = $2, publication_error = 'Recovered after a worker interruption'
		WHERE publication_status = 'unpublishing' AND publication_started_at IS NOT NULL AND publication_started_at < $1`, staleBefore, now); err != nil {
		return err
	}
	for _, q := range []string{
		`SELECT id FROM custom_rosary_prayers WHERE share_status = 'approved' AND publication_status = 'pending'
			AND (publication_retry_at IS NULL OR publication_retry_at <= $1) AND (publication_attempts < $2) ORDER BY id ASC LIMIT $3`,
		`SELECT id FROM custom_rosary_prayers WHERE share_status = 'approved' AND publication_status = 'failed'
			AND (publication_retry_at IS NOT NULL AND publication_retry_at <= $1) AND (publication_attempts < $2) ORDER BY id ASC LIMIT $3`,
	} {
		ids, err := idPairs(ctx, q, now, MaxAutomaticPublicationAttempts, reconcileBatch)
		if err != nil {
			return err
		}
		for _, p := range ids {
			if err := EnqueuePublish(ctx, p[0].(int64)); err != nil {
				return err
			}
		}
	}
	pairs, err := idPairs(ctx, `SELECT id, strapi_document_id FROM custom_rosary_prayers WHERE publication_status = 'unpublishing'
		AND NOT ((strapi_document_id IS NULL OR strapi_document_id = '')) AND (publication_started_at IS NULL)
		AND (publication_retry_at IS NOT NULL AND publication_retry_at <= $1) ORDER BY id ASC LIMIT $2`, now, reconcileBatch)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		if err := EnqueueUnpublish(ctx, p[0].(int64), p[1]); err != nil {
			return err
		}
	}
	return nil
}
