// Package reading ports the Rails lectionary resolution (Reading::* and
// ReadingService): which record a date gets, and how it is shaped into the
// typed selection the API publishes.
package reading

import (
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Passage ports Reading::Passage. Nil pointers are Ruby nils.
type Passage struct {
	Reference    string
	Translation  *string
	BookName     *string
	Chapter      *int
	VerseStart   *int
	VerseEnd     *int
	Content      *rb.Map
	Title        *string
	Alternative  *Passage
	Alternatives []*Passage
}

// newReference ports Bible::Reference.new: squished, never blank.
func newReference(value string) string {
	normalized := rb.Squish(value)
	if normalized == "" {
		web.Raise("InvalidBibleReference", "Bible reference cannot be blank", "INVALID_BIBLE_REFERENCE")
	}
	return normalized
}

func strPtr(s string) *string { return &s }

// translationPtr maps the "" convention for a missing translation to nil.
func translationPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ToH ports Passage#to_h.
func (p *Passage) ToH() *rb.Map {
	m := rb.NewMap()
	m.Set("reference", p.Reference)
	if p.Translation != nil {
		m.Set("translation", *p.Translation)
	}
	if p.BookName != nil {
		m.Set("book_name", *p.BookName)
	}
	if p.Chapter != nil {
		m.Set("chapter", *p.Chapter)
	}
	if p.VerseStart != nil {
		m.Set("verse_start", *p.VerseStart)
	}
	if p.VerseEnd != nil {
		m.Set("verse_end", *p.VerseEnd)
	}
	if p.Content != nil {
		m.Set("content", p.Content)
	}
	if p.Title != nil {
		m.Set("title", *p.Title)
	}
	if p.Alternative != nil {
		m.Set("alternative", p.Alternative.ToH())
	}
	if len(p.Alternatives) > 0 {
		list := make([]any, len(p.Alternatives))
		for i, a := range p.Alternatives {
			list[i] = a.ToH()
		}
		m.Set("alternatives", list)
	}
	return m
}

func (p *Passage) dup() *Passage {
	c := *p
	return &c
}

// Selection ports Reading::Selection.
type Selection struct {
	FirstReading     *Passage
	Psalm            *Passage
	PsalmAlternative *Passage
	SecondReading    *Passage
	Gospel           *Passage
	Notes            any // nil when absent
}

// Slot returns a passage by member name (Selection#[]).
func (s *Selection) Slot(name string) *Passage {
	switch name {
	case "first_reading":
		return s.FirstReading
	case "psalm":
		return s.Psalm
	case "psalm_alternative":
		return s.PsalmAlternative
	case "second_reading":
		return s.SecondReading
	case "gospel":
		return s.Gospel
	}
	return nil
}

// ToH ports Selection#to_h.
func (s *Selection) ToH() *rb.Map {
	m := rb.NewMap()
	for _, pair := range []struct {
		k string
		p *Passage
	}{{"first_reading", s.FirstReading}, {"psalm", s.Psalm}, {"psalm_alternative", s.PsalmAlternative}, {"second_reading", s.SecondReading}, {"gospel", s.Gospel}} {
		if pair.p != nil {
			m.Set(pair.k, pair.p.ToH())
		}
	}
	if s.Notes != nil {
		m.Set("notes", s.Notes)
	}
	return m
}

// SelectionToH is ToH that tolerates a nil selection (Ruby's &.to_h).
func SelectionToH(s *Selection) any {
	if s == nil {
		return nil
	}
	return s.ToH()
}

// Attempt is one entry of the precedence trail.
type Attempt struct {
	Rule     string
	Status   string
	Replaces string // "" when absent
}

func (a Attempt) toH() *rb.Map {
	m := rb.M("rule", rb.Symbol(a.Rule), "status", rb.Symbol(a.Status))
	if a.Replaces != "" {
		m.Set("replaces", rb.Symbol(a.Replaces))
	}
	return m
}

// SlotProvenance is the per-slot decision record.
type SlotProvenance struct {
	Source   string
	Rule     string
	Fallback bool
}

// Provenance ports Reading::Provenance.
type Provenance struct {
	Source         string
	Rule           string
	Cycle          string
	Track          *string
	ServiceType    *string
	ServiceVariant *string
	PsalmTable     *string
	Fallback       bool
	Attempts       []Attempt
	SlotOrder      []string
	Slots          map[string]SlotProvenance
}

// ExpectedFallbackRules are explicit liturgical substitutions.
var ExpectedFallbackRules = []string{"daily_course", "office_fallback"}

// UnexpectedFallback ports unexpected_fallback?.
func (p *Provenance) UnexpectedFallback() bool {
	if !p.Fallback {
		return false
	}
	for _, r := range ExpectedFallbackRules {
		if r == p.Rule {
			return false
		}
	}
	return true
}

// ForSlot ports for_slot.
func (p *Provenance) ForSlot(slot string) SlotProvenance {
	if s, ok := p.Slots[slot]; ok {
		return s
	}
	return SlotProvenance{Source: p.Source, Rule: p.Rule, Fallback: p.Fallback}
}

// ToH ports Provenance#to_h.
func (p *Provenance) ToH() *rb.Map {
	m := rb.M("source", rb.Symbol(p.Source), "rule", rb.Symbol(p.Rule), "cycle", p.Cycle)
	if p.Track != nil {
		m.Set("track", *p.Track)
	}
	if p.ServiceType != nil {
		m.Set("service_type", *p.ServiceType)
	}
	if p.ServiceVariant != nil {
		m.Set("service_variant", *p.ServiceVariant)
	}
	if p.PsalmTable != nil {
		m.Set("psalm_table", *p.PsalmTable)
	}
	m.Set("fallback", p.Fallback)
	attempts := make([]any, len(p.Attempts))
	for i, a := range p.Attempts {
		attempts[i] = a.toH()
	}
	m.Set("attempts", attempts)
	if len(p.SlotOrder) > 0 {
		slots := rb.NewMap()
		for _, k := range p.SlotOrder {
			s := p.Slots[k]
			slots.Set(k, rb.M("source", rb.Symbol(s.Source), "rule", rb.Symbol(s.Rule), "fallback", s.Fallback))
		}
		m.Set("slot_provenance", slots)
	}
	return m
}

// Resolution ports Reading::Resolution.
type Resolution struct {
	Selection  *Selection
	Provenance *Provenance
}

// ToH ports Resolution#to_h.
func (r *Resolution) ToH() *rb.Map {
	m := rb.NewMap()
	if r.Selection != nil {
		m.Set("readings", r.Selection.ToH())
	}
	if r.Provenance != nil {
		m.Set("provenance", r.Provenance.ToH())
	}
	return m
}
