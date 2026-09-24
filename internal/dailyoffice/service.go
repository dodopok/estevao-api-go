package dailyoffice

import (
	"context"
	"sort"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
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
	code := rb.ToS(s.Prefs.Get("prayer_book_code"))
	build := fetchBuilder(code)
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		panic(err)
	}
	if pb == nil {
		web.RecordNotFound("Couldn't find PrayerBook with code=" + code)
	}
	family := s.Prefs.Get("family_rite") == true
	available := books.For(pb.Code, pb.Features).AvailableOffices(family)
	supported := false
	for _, o := range available {
		if o == s.OfficeType {
			supported = true
		}
	}
	if !supported {
		web.Raise("UnsupportedOfficeType", "O ofício '"+s.OfficeType+"' não está disponível para o Prayer Book "+code+".", "")
	}
	c := NewContext(ctx, s.Date, s.OfficeType, s.Prefs)
	out := build(ctx, c).Call()
	return RemoveAudioData(out).(*rb.Map)
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
