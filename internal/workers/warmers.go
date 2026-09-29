package workers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/dailyoffice"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// The warmers run the same domain steps as Rails' CacheWarmerJob and
// CalendarWarmerJob, so a book whose data no longer computes still fails the
// job the same way. The Daily Office base they build lands in the Go
// stack's own cache (same key and version as the Rails entry, JSON under
// "go/"); the rest only fills this process' in-memory caches (see
// docs/EQUIVALENCE.md).
func init() {
	solidqueue.Register("CacheWarmerJob", solidqueue.Handler{Queue: "maintenance", Perform: cacheWarmer})
	solidqueue.Register("CalendarWarmerJob", solidqueue.Handler{Queue: "maintenance", Perform: calendarWarmer})
}

type warmStats struct {
	errors []string
	counts map[string]int
}

func (s *warmStats) step(context, counter string, f func() error) {
	err := func() (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("%v", rec)
			}
		}()
		return f()
	}()
	if err != nil {
		msg := context + ": " + solidqueue.ClassOf(err) + " " + err.Error()
		s.errors = append(s.errors, msg)
		slog.Error("[CacheWarmer] " + msg)
		return
	}
	s.counts[counter]++
}

func codesArg(v any) []string {
	var out []string
	add := func(s string) {
		for _, c := range strings.Split(s, ",") {
			if c = strings.TrimSpace(c); c != "" {
				out = append(out, c)
			}
		}
	}
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			add(rb.ToS(e))
		}
	case nil:
	default:
		add(rb.ToS(x))
	}
	return out
}

func booksByID(ctx context.Context, codes []string) ([]*store.PrayerBook, error) {
	all, err := store.PrayerBooksOrdered(ctx, "", "")
	if err != nil {
		return nil, err
	}
	byCode := map[string]*store.PrayerBook{}
	for _, pb := range all {
		byCode[pb.Code] = pb
	}
	var out []*store.PrayerBook
	if len(codes) == 0 {
		out = all
	} else {
		var missing []string
		seen := map[string]bool{}
		for _, c := range codes {
			if seen[c] {
				continue
			}
			seen[c] = true
			if pb, ok := byCode[c]; ok {
				out = append(out, pb)
			} else {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			return nil, &solidqueue.Error{Class: "ArgumentError", Message: "Prayer Book(s) not found or inactive: " + strings.Join(missing, ", ")}
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ID < out[j-1].ID; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// cacheWarmer ports CacheWarmerJob#perform(date:, prayer_book_codes:,
// warm_family_rites:).
func cacheWarmer(ctx context.Context, e *solidqueue.Execution) error {
	date := civil.FromTime(time.Now().UTC())
	if v := e.Kwarg("date"); rb.Present(v) {
		d, ok := civil.ParseISO(rb.ToS(v))
		if !ok {
			return &solidqueue.Error{Class: "ArgumentError", Message: "date must use YYYY-MM-DD"}
		}
		date = d
	}
	family := e.Kwarg("warm_family_rites") != false
	pbs, err := booksByID(ctx, codesArg(e.Kwarg("prayer_book_codes")))
	if err != nil {
		return err
	}
	s := &warmStats{counts: map[string]int{}}
	for _, pb := range pbs {
		code := pb.Code
		s.step(code+"/prayer_book", "static", func() error { _, err := store.PrayerBookByCode(ctx, code); return err })
		s.step(code+"/preferences", "static", func() error { _, err := prefs.For(ctx, pb); return err })
		s.step(code+"/liturgical_texts", "static", func() error { store.LiturgicalTextsFor(ctx, pb); return nil })
		s.step(code+"/psalms", "static", func() error { store.PsalmsFor(ctx, pb); return nil })
		s.step(code+"/psalm_cycles", "static", func() error { store.PsalmCyclesFor(ctx, pb); return nil })
		s.step(code+"/collects", "static", func() error { store.CollectsFor(ctx, pb); return nil })
		s.step(code+"/celebrations", "static", func() error { _, err := store.CelebrationsForBook(ctx, pb); return err })
		var resolved *prefs.Resolved
		s.step(code+"/preferences", "preferences", func() error {
			var err error
			resolved, err = prefs.Resolve(ctx, pb, nil, nil)
			return err
		})
		var cal *liturgical.Calendar
		s.step(code+"/calendar", "calendars", func() error {
			bc, err := store.CelebrationsForBook(ctx, pb)
			if err != nil {
				return err
			}
			cal = liturgical.NewCalendarWith(date.Year(), code, bc)
			cal.DayInfo(date)
			return nil
		})
		if resolved == nil {
			continue
		}
		values := resolved.Values
		variant := books.For(pb.Code, pb.Features).LectionaryServiceVariant(values)
		for _, st := range []string{"", "eucharist", "morning_prayer", "evening_prayer"} {
			name := st
			if name == "" {
				name = "default"
			}
			s.step(code+"/readings/"+name, "readings", func() error {
				reading.For(ctx, date, reading.Options{PrayerBookCode: code, Calendar: cal, Translation: rb.ToS(values.Get("bible_version")),
					ReadingType: rb.ToS(values.Get("reading_type")), ServiceVariant: variant, ServiceType: st}).Selection()
				return nil
			})
		}
		caps := books.For(pb.Code, pb.Features)
		sets := []bool{false}
		if family && caps.SupportsFamilyRite() {
			sets = append(sets, true)
		}
		for _, fr := range sets {
			office := values.Dup()
			prefix, counter := "", "offices"
			if fr {
				office.Set("family_rite", true)
				prefix, counter = "family/", "family_offices"
			}
			for _, o := range caps.AvailableOffices(fr) {
				s.step(code+"/"+prefix+o, counter, func() error {
					dailyoffice.NewService(ctx, date, o, office).Call(ctx)
					return nil
				})
			}
		}
	}
	slog.Info(fmt.Sprintf("[CacheWarmer] Completed date=%s offices=%d family_offices=%d readings=%d errors=%d",
		date.ISO(), s.counts["offices"], s.counts["family_offices"], s.counts["readings"], len(s.errors)))
	if len(s.errors) > 0 {
		first := s.errors
		if len(first) > 3 {
			first = first[:3]
		}
		return &solidqueue.Error{Class: "CacheWarmerJob::PartialFailure",
			Message: fmt.Sprintf("%d cache warming step(s) failed: %s", len(s.errors), strings.Join(first, "; "))}
	}
	return nil
}

// calendarWarmer ports CalendarWarmerJob#perform(months_back:,
// months_ahead:, prayer_book_codes:): every month of the window is computed
// once per book; one book failing never stops the others.
func calendarWarmer(ctx context.Context, e *solidqueue.Execution) error {
	back, ahead := 3, 12
	if v := e.Kwarg("months_back"); v != nil {
		back = rb.ToI(v)
	}
	if v := e.Kwarg("months_ahead"); v != nil {
		ahead = rb.ToI(v)
	}
	pbs, err := store.PrayerBooksOrdered(ctx, "", "")
	if err != nil {
		return err
	}
	if codes := codesArg(e.Kwarg("prayer_book_codes")); len(codes) > 0 {
		want := map[string]bool{}
		for _, c := range codes {
			want[c] = true
		}
		var kept []*store.PrayerBook
		for _, pb := range pbs {
			if want[pb.Code] {
				kept = append(kept, pb)
			}
		}
		pbs = kept
	}
	today := civil.FromTime(time.Now().In(rb.AppZone))
	first := civil.MustNew(today.Year(), today.Month(), 1)
	warmed, failures := 0, 0
	for _, pb := range pbs {
		err := func() (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("%v", rec)
				}
			}()
			bc, err := store.CelebrationsForBook(ctx, pb)
			if err != nil {
				return err
			}
			cals := map[int]*liturgical.Calendar{}
			for off := -back; off <= ahead; off++ {
				m := first.Month() - 1 + off
				y := first.Year() + floorDiv(m, 12)
				m = m - floorDiv(m, 12)*12 + 1
				cal, ok := cals[y]
				if !ok {
					cal = liturgical.NewCalendarWith(y, pb.Code, bc)
					cals[y] = cal
				}
				for d := 1; d <= civil.DaysInMonth(y, m); d++ {
					date := civil.MustNew(y, m, d)
					cal.DayInfo(date)
					cal.ContextFor(date)
				}
				warmed++
			}
			return nil
		}()
		if err != nil {
			failures++
			slog.Error("[CalendarWarmer] " + pb.Code + ": " + err.Error())
		}
	}
	slog.Info(fmt.Sprintf("[CalendarWarmer] prayer_books=%d warmed=%d errors=%d", len(pbs), warmed, failures))
	return nil
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
