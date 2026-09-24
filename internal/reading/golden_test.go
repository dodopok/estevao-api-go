package reading_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/web"
	"github.com/dodopok/estevao-api-go/internal/wiring"
)

// TestResolverMatchesRails replays the corpus written by
// tools/golden/readings.rb (READINGS_GOLDEN) against the same database
// (DATABASE_URL) and compares every resolution byte for byte.
func TestResolverMatchesRails(t *testing.T) {
	path := os.Getenv("READINGS_GOLDEN")
	dsn := os.Getenv("DATABASE_URL")
	if path == "" || dsn == "" {
		t.Skip("READINGS_GOLDEN and DATABASE_URL required")
	}
	ctx := context.Background()
	if err := db.Open(ctx, dsn, 8); err != nil {
		t.Fatal(err)
	}
	wiring.Install()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 64<<20), 64<<20)
	n, bad := 0, 0
	for sc.Scan() {
		row, err := rb.ParseJSON(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		m := row.(*rb.Map)
		n++
		want := expected(m)
		got := actual(ctx, m)
		if got != want {
			bad++
			if bad <= 20 {
				t.Errorf("%s %s\n  rails: %s\n  go:    %s", m.Get("date"), string(rb.JSON(m.Get("opts"))), trunc(want), trunc(got))
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases, %d mismatches", n, bad)
}

func trunc(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "..."
	}
	return s
}

func expected(m *rb.Map) string {
	if m.Has("error") {
		return fmt.Sprintf("error %v: %v", m.Get("error"), m.Get("message"))
	}
	return fmt.Sprintf("cycle=%v %s", m.Get("cycle"), rb.JSON(m.Get("result")))
}

func str(m *rb.Map, k string) string {
	v := m.Get(k)
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

func actual(ctx context.Context, m *rb.Map) (out string) {
	opts := m.Get("opts").(*rb.Map)
	date, ok := civil.ParseISO(str(m, "date"))
	if !ok {
		return "bad date"
	}
	defer func() {
		if rec := recover(); rec != nil {
			switch e := rec.(type) {
			case *web.DomainError:
				out = fmt.Sprintf("error %s: %s", e.Class, e.Message)
			case *rb.RubyError:
				out = fmt.Sprintf("error %s: %s", e.Class, e.Message)
			default:
				out = fmt.Sprintf("panic %v", rec)
			}
		}
	}()
	o := reading.Options{
		PrayerBookCode:   str(opts, "prayer_book_code"),
		Translation:      str(opts, "translation"),
		ReadingType:      str(opts, "reading_type"),
		ServiceType:      str(opts, "service_type"),
		ServiceVariant:   str(opts, "service_variant"),
		PsalmTable:       str(opts, "psalm_table"),
		PsalmTranslation: str(opts, "psalm_translation"),
		LoadContent:      opts.Get("load_content") == true,
	}
	r := reading.For(ctx, date, o)
	res := r.Resolve()
	return fmt.Sprintf("cycle=%s %s", r.Cycle, rb.JSON(res.ToH()))
}
