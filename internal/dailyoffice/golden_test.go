package dailyoffice_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/dailyoffice"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
	"github.com/dodopok/estevao-api-go/internal/wiring"
)

// TestOfficesMatchRails replays tools/golden/daily_office.rb output
// (OFFICE_GOLDEN) against the same database; OFFICE_BOOKS limits the books.
func TestOfficesMatchRails(t *testing.T) {
	path, dsn := os.Getenv("OFFICE_GOLDEN"), os.Getenv("DATABASE_URL")
	if path == "" || dsn == "" {
		t.Skip("OFFICE_GOLDEN and DATABASE_URL required")
	}
	only := map[string]bool{}
	for _, c := range strings.Split(os.Getenv("OFFICE_BOOKS"), ",") {
		if c != "" {
			only[c] = true
		}
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
	sc.Buffer(make([]byte, 256<<20), 256<<20)
	n, bad := 0, 0
	perBook := map[string][2]int{}
	for sc.Scan() {
		v, err := rb.ParseJSON(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		m := v.(*rb.Map)
		p := m.Get("prefs").(*rb.Map)
		code := rb.ToS(p.Get("prayer_book_code"))
		if len(only) > 0 && !only[code] {
			continue
		}
		n++
		want := expected(m)
		got := actual(ctx, m)
		c := perBook[code]
		c[0]++
		if got != want {
			bad++
			c[1]++
			if bad <= 8 {
				t.Errorf("%s %s %s\n%s", code, m.Get("date"), m.Get("office"), firstDiff(want, got))
			}
		}
		perBook[code] = c
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for k, c := range perBook {
		t.Logf("%-14s %6d cases %6d mismatches", k, c[0], c[1])
	}
	t.Logf("%d cases, %d mismatches", n, bad)
}

func firstDiff(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := i - 300
	if lo < 0 {
		lo = 0
	}
	cut := func(s string) string {
		hi := i + 300
		if hi > len(s) {
			hi = len(s)
		}
		if lo > len(s) {
			return ""
		}
		return s[lo:hi]
	}
	return fmt.Sprintf("  rails: …%s…\n  go:    …%s…", cut(a), cut(b))
}

func expected(m *rb.Map) string {
	if m.Has("error") {
		return fmt.Sprintf("error %v: %v", m.Get("error"), m.Get("message"))
	}
	return string(rb.JSON(m.Get("result")))
}

func actual(ctx context.Context, m *rb.Map) (out string) {
	defer func() {
		if rec := recover(); rec != nil {
			switch e := rec.(type) {
			case *web.DomainError:
				out = fmt.Sprintf("error %s: %s", e.Class, e.Message)
			case *web.StandardError:
				out = fmt.Sprintf("error %s: %s", e.Class, e.Message)
			case *rb.RubyError:
				out = fmt.Sprintf("error %s: %s", e.Class, e.Message)
			default:
				out = fmt.Sprintf("panic %v", rec)
			}
		}
	}()
	date, _ := civil.ParseISO(rb.ToS(m.Get("date")))
	svc := dailyoffice.NewService(ctx, date, rb.ToS(m.Get("office")), m.Get("prefs").(*rb.Map).Dup())
	return string(rb.JSON(svc.Call(ctx)))
}
