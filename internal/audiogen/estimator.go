package audiogen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/audio"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/dailyoffice"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// catch turns a Ruby-style raise (a panic carrying an error) into an error.
func catch(f func() error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if e, ok := rec.(error); ok {
				err = e
				return
			}
			panic(rec)
		}
	}()
	return f()
}

// BuildOffice ports DailyOfficeService.new(date:, office_type:,
// preferences: configuration.merge(prayer_book_code:)).call.
func BuildOffice(ctx context.Context, date civil.Date, officeType, code string, configuration *rb.Map) (office *rb.Map, err error) {
	err = catch(func() error {
		p := configuration.Dup()
		p.Set("prayer_book_code", code)
		office = dailyoffice.NewService(ctx, date, officeType, p).Call(ctx)
		return nil
	})
	return office, err
}

// spokenEntries ports each_line(office).select { SPOKEN && text.present? }.
func spokenEntries(office *rb.Map) []audio.Entry {
	var out []audio.Entry
	for _, e := range audio.EachLine(office) {
		if audio.Spoken(e.Type) && !rb.BlankString(e.Text) {
			out = append(out, e)
		}
	}
	return out
}

// EstimateArguments are OfficeEstimator.new's keywords.
type EstimateArguments struct {
	PrayerBookCode string
	StartDate      civil.Date
	Days           int64
	Offices        []string
	Preferences    *rb.Map
	Variants       []*rb.Map
}

// Estimate ports Audio::OfficeEstimator#call: what a date-based run over
// the window would find ready and what it would have to buy.
func Estimate(ctx context.Context, a EstimateArguments) (*rb.Map, error) {
	pb, err := store.PrayerBookByCode(ctx, a.PrayerBookCode)
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, web.NewDomainError("UnsupportedPrayerBook", "unknown prayer book: "+a.PrayerBookCode, "")
	}
	if a.Days < 1 || a.Days > 31 {
		return nil, &rb.RubyError{Class: "ArgumentError", Message: "days must be between 1 and 31"}
	}
	lang := pb.Language
	provider := audio.Current(&lang)
	generator := audio.NewReusingGenerator(provider)
	matrix, err := NewCoverageMatrix(ctx, a.PrayerBookCode, a.Offices, a.Preferences, a.Variants)
	if err != nil {
		return nil, err
	}
	offices, err := matrix.OfficeTypes()
	if err != nil {
		return nil, err
	}
	byOffice, err := matrix.ConfigurationsByOffice(ctx)
	if err != nil {
		return nil, err
	}
	rows := []any{}
	var ready, missing, missingChars, variantCount int64
	for _, o := range byOffice {
		variantCount += int64(len(o.Configurations))
	}
	for offset := 0; offset < int(a.Days); offset++ {
		date := a.StartDate.Add(offset)
		for _, o := range byOffice {
			for _, configuration := range o.Configurations {
				row, err := estimateOffice(ctx, generator, date, o.OfficeType, a.PrayerBookCode, configuration)
				if err != nil {
					return nil, err
				}
				ready += row.Get("ready_clips").(int64)
				missing += row.Get("missing_clips").(int64)
				missingChars += row.Get("missing_characters").(int64)
				rows = append(rows, row)
			}
		}
	}
	officeList := make([]any, len(offices))
	for i, o := range offices {
		officeList[i] = o
	}
	return rb.M("prayer_book_code", a.PrayerBookCode, "start_date", a.StartDate.ISO(), "days", a.Days, "offices", officeList,
		"variants", variantCount, "provider", provider.ProfileMap(), "ready_clips", ready, "missing_clips", missing,
		"missing_characters", missingChars, "estimated_cost", estimatedCost(missingChars, provider.Name), "rows", rows), nil
}

func estimateOffice(ctx context.Context, g *audio.Generator, date civil.Date, officeType, code string, configuration *rb.Map) (*rb.Map, error) {
	office, err := BuildOffice(ctx, date, officeType, code, configuration)
	if err != nil {
		return nil, err
	}
	entries := spokenEntries(office)
	if err := g.Preload(ctx, entries); err != nil {
		return nil, err
	}
	var ready, missing, missingChars int64
	for _, e := range entries {
		segments, err := g.CallSegments(ctx, e, false)
		if err != nil {
			return nil, err
		}
		for _, s := range segments {
			if s.Clip != nil {
				ready++
			} else {
				missing++
				missingChars += int64(len([]rune(s.Text)))
			}
		}
	}
	sum := sha256.Sum256(rb.JSON(configuration))
	return rb.M("date", date.ISO(), "office_type", officeType, "ready_clips", ready, "missing_clips", missing,
		"missing_characters", missingChars, "variant", hex.EncodeToString(sum[:])[:12]), nil
}

// estimatedCost ports estimated_cost(characters, provider_name).
func estimatedCost(characters int64, provider string) any {
	key := map[string]string{
		"openai":     "OPENAI_TTS_ESTIMATED_USD_PER_1K_CHARACTERS",
		"google":     "GOOGLE_TTS_ESTIMATED_USD_PER_1K_CHARACTERS",
		"elevenlabs": "ELEVENLABS_TTS_ESTIMATED_USD_PER_1K_CHARACTERS",
	}[provider]
	if key == "" {
		return nil
	}
	raw, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	rate, ok := rb.KernelFloat(raw)
	if !ok || rate <= 0 {
		return nil
	}
	return rb.RoundFloat(float64(characters)/1000.0*rate, 2)
}

// rescuable ports `rescue DomainError, InfrastructureError,
// ActiveRecord::ActiveRecordError`.
func rescuable(err error) bool {
	var de *web.DomainError
	var ie *web.InfraError
	if errors.As(err, &de) || errors.As(err, &ie) {
		return true
	}
	var re *rb.RubyError
	if errors.As(err, &re) && strings.HasPrefix(re.Class, "ActiveRecord::") {
		return true
	}
	var se *web.StandardError
	if errors.As(err, &se) && strings.HasPrefix(se.Class, "ActiveRecord::") {
		return true
	}
	_, _, ok := db.RubyError(err)
	return ok
}
