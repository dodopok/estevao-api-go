package rosary

import (
	"context"
	"github.com/dodopok/estevao-api-go/internal/ar"
	"regexp"

	"strings"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func (s *Step) changedForAutosave() bool { return s.NewRecord || s.HasChanges() || s.Marked }

func (b *Block) changedForAutosave() bool {
	if b.NewRecord || b.HasChanges() || b.Marked {
		return true
	}
	for _, s := range b.Steps {
		if s.changedForAutosave() {
			return true
		}
	}
	return false
}

func (b *Block) liveSteps() []*Step {
	var out []*Step
	for _, s := range b.Steps {
		if !s.Marked {
			out = append(out, s)
		}
	}
	return out
}

func (p *Prayer) liveBlocks() []*Block {
	var out []*Block
	for _, b := range p.Blocks {
		if !b.Marked {
			out = append(out, b)
		}
	}
	return out
}

func (s *Step) errors() *ar.Errors {
	e := &ar.Errors{}
	ar.TooLong(e, "client_id", s.Get("client_id"), 120)
	ar.Numeric(e, s.Record, "position", rb.GT(0))
	if !ar.Included(s.Get("step_type"), StepTypes) {
		ar.NotIncluded(e, "step_type", s.Get("step_type"))
	}
	if ar.Blank(s.Get("title")) {
		e.Add("title", "can't be blank")
	}
	ar.TooLong(e, "title", s.Get("title"), 80)
	if ar.Blank(s.Get("text")) {
		e.Add("text", "can't be blank")
	}
	ar.TooLong(e, "text", s.Get("text"), 4000)
	ar.Numeric(e, s.Record, "repeat_count", rb.GTE(1), rb.LTE(20))
	return e
}

func duplicatePositions(records []*ar.Record) bool {
	seen := map[int64]bool{}
	for _, r := range records {
		n, ok := r.Get("position").(int64)
		if !ok {
			continue
		}
		if seen[n] {
			return true
		}
		seen[n] = true
	}
	return false
}

func (b *Block) errors() *ar.Errors {
	e := &ar.Errors{}
	for _, s := range b.Steps {
		if s.Marked || !(b.NewRecord || s.changedForAutosave()) {
			continue
		}
		if inner := s.errors(); inner.Any() {
			e.ImportFrom("steps", inner)
		}
	}
	ar.TooLong(e, "client_id", b.Get("client_id"), 120)
	ar.Numeric(e, b.Record, "position", rb.GT(0))
	ar.TooLong(e, "name", b.Get("name"), 80)
	ar.Numeric(e, b.Record, "repeat_count", rb.GTE(1), rb.LTE(20))
	if _, ok := b.Get("in_cycle").(bool); !ok {
		ar.NotIncluded(e, "in_cycle", b.Get("in_cycle"))
	}
	live := b.liveSteps()
	if len(live) > MaxStepsPerBlock {
		e.Add("steps", "cannot exceed "+ar.Itoa(MaxStepsPerBlock)+" steps")
	}
	recs := make([]*ar.Record, len(live))
	for i, s := range live {
		recs[i] = s.Record
	}
	if duplicatePositions(recs) {
		e.Add("steps", "must have unique positions")
	}
	e.Uniq()
	return e
}

func toI(v any) int64 {
	n, _ := v.(int64)
	return n
}

// ExpandedStepCount ports CustomRosaryPrayer#expanded_step_count.
func (p *Prayer) ExpandedStepCount() int64 {
	var total int64
	for _, b := range p.liveBlocks() {
		var perBlock int64
		for _, s := range b.liveSteps() {
			perBlock += toI(s.Get("repeat_count"))
		}
		count := perBlock * toI(b.Get("repeat_count"))
		mult := int64(1)
		if b.Get("in_cycle") == true {
			mult = toI(p.Get("cycle_repeat"))
		}
		total += count * mult
	}
	return total
}

// Validate ports CustomRosaryPrayer#valid? (create selects the on: :create
// validation).
func (p *Prayer) Validate(ctx context.Context, q db.Querier, create bool) *ar.Errors {
	e := &ar.Errors{}
	for _, b := range p.Blocks {
		if b.Marked || !(p.NewRecord || b.changedForAutosave()) {
			continue
		}
		if inner := b.errors(); inner.Any() {
			e.ImportFrom("blocks", inner)
		}
	}
	clientID := p.Get("client_id")
	if ar.Blank(clientID) {
		e.Add("client_id", "can't be blank")
	}
	ar.TooLong(e, "client_id", clientID, 120)
	if clientID != nil {
		var taken bool
		sql := `SELECT EXISTS(SELECT 1 FROM custom_rosary_prayers WHERE client_id = $1 AND user_id = $2`
		args := []any{clientID, p.Get("user_id")}
		if !p.NewRecord {
			sql += ` AND id <> $3`
			args = append(args, p.ID)
		}
		must(q.QueryRow(ctx, sql+`)`, args...).Scan(&taken))
		if taken {
			e.Add("client_id", "has already been taken")
		}
	}
	if ar.Blank(p.Get("title")) {
		e.Add("title", "can't be blank")
	}
	ar.TooLong(e, "title", p.Get("title"), 120)
	ar.TooLong(e, "description", p.Get("description"), 500)
	if !ar.Included(p.Get("locale"), Locales) {
		ar.NotIncluded(e, "locale", p.Get("locale"))
	}
	ar.Numeric(e, p.Record, "cycle_repeat", rb.GTE(1), rb.LTE(12))
	if !ar.Included(p.Get("share_status"), ShareStatuses) {
		ar.NotIncluded(e, "share_status", p.Get("share_status"))
	}
	if d := p.Get("moderation_decision"); d != nil && !ar.Included(d, ModerationDecisions) {
		ar.NotIncluded(e, "moderation_decision", d)
	}
	if !ar.Included(p.Get("publication_status"), PublicationStatuses) {
		ar.NotIncluded(e, "publication_status", p.Get("publication_status"))
	}
	if l := p.Get("publication_locale"); l != nil && !ar.Included(l, Locales) {
		ar.NotIncluded(e, "publication_locale", l)
	}
	if c := p.Get("publication_category"); !ar.Blank(c) {
		if _, err := CategorySelection(c); err != nil {
			e.Add("publication_category", err.Error())
		}
	}
	live := p.liveBlocks()
	if len(live) > MaxBlocks {
		e.Add("blocks", "cannot exceed "+ar.Itoa(MaxBlocks)+" blocks")
	}
	recs := make([]*ar.Record, len(live))
	for i, b := range live {
		recs[i] = b.Record
	}
	if duplicatePositions(recs) {
		e.Add("blocks", "must have unique positions")
	}
	if p.ExpandedStepCount() > MaxExpandedSteps {
		e.Add("blocks", "expand to more than "+ar.Itoa(MaxExpandedSteps)+" steps")
	}
	if create && p.Get("user_id") != nil {
		var n int
		must(q.QueryRow(ctx, `SELECT COUNT(*) FROM custom_rosary_prayers WHERE (moderation_decision != 'approved' OR moderation_decision IS NULL)
			AND user_id = $1`, p.Get("user_id")).Scan(&n))
		if n >= MaxPerUser {
			e.Add("base", "cannot have more than "+ar.Itoa(MaxPerUser)+" custom rosary prayers")
		}
	}
	e.Uniq()
	return e
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// --- CategorySelection ------------------------------------------------------

// InvalidCategory is CategorySelection::Invalid; Field and Problem form its
// errors hash ({field: [problem]}).
type InvalidCategory struct{ Message, Field, Problem string }

func invalidCategory(msg, field, problem string) *InvalidCategory {
	return &InvalidCategory{Message: msg, Field: field, Problem: problem}
}

// ErrorsHash ports Invalid#errors.
func (e *InvalidCategory) ErrorsHash() *rb.Map { return rb.M(e.Field, []string{e.Problem}) }

func (e *InvalidCategory) Error() string { return e.Message }

var whitespace = regexp.MustCompile(`[ \t\n\v\f\r]`)

func rubyStrip(s string) string { return strings.Trim(s, " \t\n\v\f\r\x00") }

func lastPart(field string) string { return field[strings.LastIndex(field, ".")+1:] }

func optionalString(v any, field string, max int) (*string, error) {
	if v == nil || v == "" {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, invalidCategory(field+" must be a string", lastPart(field), "must be a string")
	}
	s = rubyStrip(s)
	if s == "" {
		return nil, nil
	}
	if len([]rune(s)) > max {
		return nil, invalidCategory(field+" is too long", lastPart(field), "is too long")
	}
	return &s, nil
}

func requiredString(v any, field string, max int) (string, error) {
	s, err := optionalString(v, field, max)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", invalidCategory(field+" is required", lastPart(field), "is required")
	}
	return *s, nil
}

// CategorySelection ports CustomRosaryPrayers::CategorySelection.call.
func CategorySelection(v any) (*rb.Map, error) {
	var data *rb.Map
	switch x := v.(type) {
	case *rb.Map:
		data = x
	case nil:
		data = rb.NewMap()
	case []any:
		data = rb.NewMap()
		for i, el := range x {
			pair, ok := el.([]any)
			if !ok {
				panic(&web.StandardError{Class: "TypeError", Message: "wrong element type " + rb.ClassName(el) + " at " + ar.Itoa(i) + " (expected array)"})
			}
			if len(pair) != 2 {
				panic(&web.StandardError{Class: "ArgumentError", Message: "wrong array length at " + ar.Itoa(i) + " (expected 2, was " + ar.Itoa(len(pair)) + ")"})
			}
			data.Set(rb.ToS(pair[0]), pair[1])
		}
	default:
		return nil, invalidCategory("category is required", "category", "is required")
	}
	mode, err := requiredString(data.Get("mode"), "category.mode", 120)
	if err != nil {
		return nil, err
	}
	if mode != "existing" && mode != "new" {
		return nil, invalidCategory("category.mode must be existing or new", "mode", "is invalid")
	}
	slug, err := requiredString(data.Get("slug"), "category.slug", 120)
	if err != nil {
		return nil, err
	}
	if whitespace.MatchString(slug) {
		return nil, invalidCategory("category.slug must not contain whitespace", "slug", "must not contain whitespace")
	}
	if mode == "existing" {
		out := rb.M("mode", "existing", "slug", slug)
		doc, err := optionalString(data.Get("documentId"), "category.documentId", 255)
		if err != nil {
			return nil, err
		}
		if doc != nil {
			out.Set("documentId", *doc)
		}
		return out, nil
	}
	name, err := requiredString(data.Get("name"), "category.name", 120)
	if err != nil {
		return nil, err
	}
	desc, err := optionalString(data.Get("description"), "category.description", 500)
	if err != nil {
		return nil, err
	}
	icon, err := optionalString(data.Get("icon"), "category.icon", 120)
	if err != nil {
		return nil, err
	}
	return rb.M("mode", "new", "slug", slug, "name", name, "description", rb.Deref(desc), "icon", rb.Deref(icon)), nil
}
