package prefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Resolved ports Preferences::Resolved (values and cache keys; sources are
// not part of any response).
type Resolved struct {
	Values    *rb.Map
	CacheKeys []string
}

var cacheSystemKeys = []string{
	"prayer_book_code", "language", "bible_version", "preferred_audio_voice", "seed",
	"reading_type", "confession_type", "creed_type", "lords_prayer_version", "family_rite",
	"lectionary_variant", "psalm_translation", "psalm_cycle", "morning_psalm_cycle",
	"evening_psalm_cycle", "morning_prayer_psalm_cycle", "evening_prayer_psalm_cycle",
	"weekday_psalm_table", "ascension_readings", "daily_office_rite",
}

var cacheExcludedKeys = map[string]bool{
	"user_id": true, "provider_uid": true, "api_key": true, "token": true,
	"notifications": true, "notifications_enabled": true,
	"streak_reminder_enabled": true, "streak_display_enabled": true, "prayer_times": true,
	"mode": true,
}

// CacheKey ports Resolved#cache_key: the SHA-256 of the sorted, compacted
// projection of the values that shape shared liturgical content.
func (r *Resolved) CacheKey() string {
	included := map[string]bool{}
	for _, k := range append(append([]string{}, r.CacheKeys...), cacheSystemKeys...) {
		if !cacheExcludedKeys[k] {
			included[k] = true
		}
	}
	var keys []string
	r.Values.Each(func(k string, v any) {
		if included[k] && v != nil {
			keys = append(keys, k)
		}
	})
	sort.Strings(keys)
	projection := rb.NewMap()
	for _, k := range keys {
		projection.Set(k, r.Values.Get(k))
	}
	sum := sha256.Sum256(rb.ToJSON(projection))
	return hex.EncodeToString(sum[:])
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
	res := newResolved(r.Values.Merge(other))
	res.CacheKeys = r.CacheKeys
	return res
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
	res := newResolved(normalized)
	res.CacheKeys = defs.Keys()
	return res, nil
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
