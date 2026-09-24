package prefs

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Resolved ports Preferences::Resolved (values only; sources and cache keys
// are internal to Rails' caching and not part of any response).
type Resolved struct {
	Values *rb.Map
}

func (r *Resolved) Get(key string) any       { return r.Values.Get(key) }
func (r *Resolved) String(key string) string { return rb.ToS(r.Values.Get(key)) }

func newResolved(values *rb.Map) *Resolved {
	if rb.Blank(values.Get("prayer_book_code")) {
		panic(&web.DomainError{Class: "InvalidPreference", Code: "PRAYER_BOOK_CODE_REQUIRED", Message: "prayer_book_code is required"})
	}
	if seed, ok := values.Lookup("seed"); ok && seed != nil {
		n, isInt := seed.(int)
		if !isInt || n < 0 {
			panic(&web.DomainError{Class: "InvalidPreference", Code: "INVALID_SEED", Message: "seed must be a non-negative integer"})
		}
	}
	return &Resolved{Values: values}
}

// Merge ports Resolved#merge.
func (r *Resolved) Merge(other *rb.Map) *Resolved {
	return newResolved(r.Values.Merge(other))
}

// DefaultValues ports Preferences::Resolver#default_values.
func DefaultValues(ctx context.Context, pb *store.PrayerBook, caps *books.Capabilities) (*rb.Map, error) {
	bv := "nvi"
	v, err := store.DefaultBibleVersionFor(ctx, pb.Language)
	if err != nil {
		return nil, err
	}
	if v != nil {
		bv = v.Code
	}
	rt := caps.DefaultReadingType()
	if rt == "" {
		rt = "semicontinuous"
	}
	lp := "traditional"
	if l := books.ArrayOf(caps.Dig("daily_office.available_lords_prayer")); len(l) > 0 {
		lp = l[0]
	}
	ct := "long"
	if l := books.ArrayOf(caps.Dig("daily_office.available_confession_types")); len(l) > 0 {
		ct = l[0]
	}
	return rb.M(
		"prayer_book_code", pb.Code,
		"language", pb.Language,
		"bible_version", bv,
		"reading_type", rt,
		"lords_prayer_version", lp,
		"confession_type", ct,
		"creed_type", "apostles",
	), nil
}

// Resolve ports Preferences::Resolver.call.
func Resolve(ctx context.Context, pb *store.PrayerBook, stored, overrides *rb.Map) (*Resolved, error) {
	if stored == nil {
		stored = rb.NewMap()
	}
	if overrides == nil {
		overrides = rb.NewMap()
	}
	caps := books.For(pb.Code, pb.Features)
	defs, err := For(ctx, pb)
	if err != nil {
		return nil, err
	}
	nStored := defs.Normalize(scoped(defs, stored), true, true)
	nOverrides := defs.Normalize(scoped(defs, overrides), true, false)
	if defs.Has("psalm_cycle") && (rb.Present(nStored.Get("psalm_cycle")) || rb.Present(nOverrides.Get("psalm_cycle"))) {
		nStored = nStored.Except(LegacyPsalterKeys...)
		nOverrides = nOverrides.Except(LegacyPsalterKeys...)
	}
	values, err := DefaultValues(ctx, pb, caps)
	if err != nil {
		return nil, err
	}
	values = values.Merge(defs.Defaults()).Merge(nStored).Merge(nOverrides)
	values.Set("prayer_book_code", pb.Code)
	if !rb.Truthy(values.Get("language")) {
		values.Set("language", pb.Language)
	}
	values.Set("reading_type", caps.NormalizeReadingType(values.Get("reading_type")))
	normalized := defs.Normalize(values, true, false)
	return newResolved(normalized), nil
}

func scoped(defs *DefinitionSet, values *rb.Map) *rb.Map {
	canonical := defs.CanonicalizeLegacyKeys(values)
	allowed := append(append([]string{}, SystemKeys...), defs.Keys()...)
	return canonical.Slice(allowed...)
}

// PsalmCycle ports Preferences::PsalmCycle.for ("" == nil).
func PsalmCycle(values *rb.Map, keyPrefix string, fallbacks ...string) string {
	keys := append([]string{keyPrefix + "_psalm_cycle", "psalm_cycle"}, fallbacks...)
	for _, k := range keys {
		if v := values.Get(k); rb.Present(v) {
			return rb.ToS(v)
		}
	}
	return ""
}
