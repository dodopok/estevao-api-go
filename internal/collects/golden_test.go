package collects_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
	"github.com/dodopok/estevao-api-go/internal/wiring"
)

// TestCollectsMatchRails replays tools/golden/collects.rb output
// (COLLECTS_GOLDEN) against the same database (DATABASE_URL).
func TestCollectsMatchRails(t *testing.T) {
	path, dsn := os.Getenv("COLLECTS_GOLDEN"), os.Getenv("DATABASE_URL")
	if path == "" || dsn == "" {
		t.Skip("COLLECTS_GOLDEN and DATABASE_URL required")
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
		v, err := rb.ParseJSON(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		m := v.(*rb.Map)
		n++
		want := expected(m)
		got := actual(ctx, m)
		if got != want {
			bad++
			if bad <= 20 {
				t.Errorf("%v %s\n  rails: %.1500s\n  go:    %.1500s", m.Get("date"), rb.JSON(m.Get("opts")), want, got)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases, %d mismatches", n, bad)
}

func expected(m *rb.Map) string {
	if m.Has("error") {
		return fmt.Sprintf("error %v: %v", m.Get("error"), m.Get("message"))
	}
	return string(rb.JSON(m.Get("result")))
}

func actual(ctx context.Context, m *rb.Map) (out string) {
	opts := m.Get("opts").(*rb.Map)
	date, _ := civil.ParseISO(rb.ToS(m.Get("date")))
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
	style := ""
	if v := opts.Get("language_style"); v != nil {
		style = rb.ToS(v)
	}
	s := collects.New(ctx, date, collects.Options{
		PrayerBookCode:            rb.ToS(opts.Get("prayer_book_code")),
		OfficeType:                rb.ToS(opts.Get("office_type")),
		LanguageStyle:             style,
		IncludeFixedOfficeCollect: opts.Get("include_fixed_office_collect") == true,
	})
	list := s.FindCollectsUncached()
	arr := make([]any, len(list))
	for i, c := range list {
		arr[i] = c
	}
	return string(rb.JSON(arr))
}
