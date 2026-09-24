package rb

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// testdata/dateparse.jsonl.gz holds Date.parse(s, comp) from the date gem
// 3.5.1 over generated inputs, run on 2026-09-24 with TZ=UTC (inputs
// without a year or date are completed from that day).
func TestDateParseMatchesRuby(t *testing.T) {
	f, err := os.Open("testdata/dateparse.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	sc := bufio.NewScanner(gz)
	n, bad := 0, 0
	for sc.Scan() {
		var c struct {
			S    string `json:"s"`
			Comp bool   `json:"comp"`
			R    string `json:"r"`
		}
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		n++
		got := "ERR"
		if d, err := dateParseAt(c.S, c.Comp, now); err == nil {
			got = d.ISO()
		}
		if got != c.R {
			bad++
			if bad <= 40 {
				t.Errorf("Date.parse(%q, %v): ruby %s go %s", c.S, c.Comp, c.R, got)
			}
		}
	}
	t.Logf("%d cases, %d mismatches", n, bad)
}
