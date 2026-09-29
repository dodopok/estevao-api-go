package dailyoffice

import (
	"context"
	"sort"
	"time"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Context ports DailyOffice::Context.
type Context struct {
	Date       civil.Date
	OfficeType string
	Code       string
	Prefs      *rb.Map
	DayContext *liturgical.DayContext
	Seed       int64
	Randomizer Randomizer
}

// NormalizePreferences ports Context.normalize_preferences.
func NormalizePreferences(ctx context.Context, input *rb.Map) *rb.Map {
	n := rb.M("prayer_book_code", books.DefaultCode, "bible_version", "nvi")
	if input != nil {
		input.Each(func(k string, v any) { n.Set(k, v) })
	}
	pb, err := store.PrayerBookByCode(ctx, rb.ToS(n.Get("prayer_book_code")))
	if err != nil {
		panic(err)
	}
	if pb != nil {
		set, err := prefs.For(ctx, pb)
		if err != nil {
			panic(err)
		}
		n = set.CanonicalizeLegacyKeys(n)
	}
	family := n.Get("family_rite") == true || rb.ToS(n.Get("office_type")) == "family"
	n.Set("family_rite", family)
	if family {
		n.Set("office_type", "family")
	} else {
		n.Set("office_type", n.Get("office_type"))
	}
	return n
}

// NewContext ports Context.from_legacy(resolve_day_context: false) + new.
func NewContext(ctx context.Context, date civil.Date, officeType string, preferences *rb.Map) *Context {
	code := books.DefaultCode
	if v := preferences.Get("prayer_book_code"); rb.Present(v) {
		code = rb.ToS(v)
	}
	seed := DeterministicSeed(date.ISO(), officeType)
	if v := preferences.Get("seed"); v != nil {
		seed = int64(rb.ToI(v))
	}
	values := NormalizePreferences(ctx, preferences)
	values.Set("prayer_book_code", code)
	values.Set("seed", seed)
	return &Context{Date: date, OfficeType: officeType, Code: code, Prefs: values, Seed: seed, Randomizer: Randomizer{Seed: seed}}
}

// Builder is a book's office builder.
type Builder interface{ Call() *rb.Map }

// BookBuilder routes an office type to its builder (StandardBuilder).
type BookBuilder func(ctx context.Context, c *Context) Builder

var registry = map[string]BookBuilder{}

// Register adds a book to DailyOffice::Registry.
func Register(code string, b BookBuilder) { registry[code] = b }

// Codes lists the registered books.
func Codes() []string {
	var out []string
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fetchBuilder(code string) BookBuilder {
	b, ok := registry[code]
	if !ok {
		web.Raise("UnsupportedPrayerBook", "Daily Office não suportado para o Prayer Book '"+code+"'.", "")
	}
	return b
}

// UnknownOfficeType is StandardBuilder's ArgumentError.
func UnknownOfficeType(officeType string) {
	panic(&rb.RubyError{Class: "ArgumentError", Message: "Unknown office type: " + officeType})
}

// Service ports DailyOfficeService.
type Service struct {
	Date       civil.Date
	OfficeType string
	Prefs      *rb.Map
}

// NewService ports DailyOfficeService.new (preferences normalized once).
func NewService(ctx context.Context, date civil.Date, officeType string, preferences *rb.Map) *Service {
	in := rb.M("prayer_book_code", books.DefaultCode, "bible_version", "nvi", "family_rite", false)
	if preferences != nil {
		preferences.Each(func(k string, v any) { in.Set(k, v) })
	}
	return &Service{Date: date, OfficeType: officeType, Prefs: NormalizePreferences(ctx, in)}
}

// Call ports #call for a caller without audio access.
func (s *Service) Call(ctx context.Context) *rb.Map {
	return RemoveAudioData(s.Base(ctx)).(*rb.Map)
}

// Base ports fetch_base_office: the office before personalization, cached
// under DailyOfficeService.base_cache_key (Cacheable::TTL::DAILY_OFFICE) and
// versioned by the Prayer Book's updated_at, as Rails caches it. Office
// support is validated only when the office is built (a cache miss), as in
// Rails; unsupported offices are never cached.
func (s *Service) Base(ctx context.Context) *rb.Map {
	code := rb.ToS(s.Prefs.Get("prayer_book_code"))
	build := fetchBuilder(code)
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		panic(err)
	}
	if pb == nil {
		web.RecordNotFound("Couldn't find PrayerBook with code=" + code)
	}
	key := "daily_office/base/v3/" + s.Date.ISO() + "/" + s.OfficeType + "/" + s.preferencesHash(ctx, pb) +
		"/pb_" + rediscache.TimestampVersion(&pb.UpdatedAt)
	raw := rediscache.FetchJSON(ctx, key, 24*time.Hour, func() []byte {
		s.validateOfficeSupport(pb)
		return rb.JSON(build(ctx, NewContext(ctx, s.Date, s.OfficeType, s.Prefs)).Call())
	})
	office, err := rb.ParseJSON(raw)
	if err != nil {
		panic(err)
	}
	return office.(*rb.Map)
}

func (s *Service) validateOfficeSupport(pb *store.PrayerBook) {
	family := s.Prefs.Get("family_rite") == true
	for _, o := range books.For(pb.Code, pb.Features).AvailableOffices(family) {
		if o == s.OfficeType {
			return
		}
	}
	web.Raise("UnsupportedOfficeType", "O ofício '"+s.OfficeType+"' não está disponível para o Prayer Book "+pb.Code+".", "")
}

// preferencesHash ports DailyOfficeService.build_preferences_hash: the
// defaults merged under the preferences, projected on the book's declared
// preference keys (all keys when it declares none).
func (s *Service) preferencesHash(ctx context.Context, pb *store.PrayerBook) string {
	full := rb.M("prayer_book_code", books.DefaultCode, "bible_version", "nvi", "family_rite", false)
	s.Prefs.Each(func(k string, v any) { full.Set(k, v) })
	var keys []string
	if defs, err := prefs.For(ctx, pb); err == nil {
		keys = defs.Keys()
	} else {
		panic(err)
	}
	if len(keys) == 0 {
		keys = full.Keys()
	}
	compact := rb.NewMap()
	full.Each(func(k string, v any) {
		if v != nil {
			compact.Set(k, v)
		}
	})
	return (&prefs.Resolved{Values: compact, CacheKeys: keys}).CacheKey()
}

// RemoveAudioData ports remove_audio_data!.
func RemoveAudioData(node any) any {
	switch x := node.(type) {
	case *rb.Map:
		x.Delete("audio_url")
		x.Delete("audio_track")
		x.Each(func(_ string, v any) { RemoveAudioData(v) })
	case []any:
		for _, v := range x {
			RemoveAudioData(v)
		}
	}
	return node
}
