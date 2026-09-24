package bible

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

type goldenSeg struct {
	Book              string `json:"book"`
	Chapter           int    `json:"chapter"`
	VerseStart        *int   `json:"verse_start"`
	VerseEnd          *int   `json:"verse_end"`
	FetchAll          bool   `json:"fetch_all"`
	FetchAllFromVerse bool   `json:"fetch_all_from_verse"`
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// TestParserMatchesRailsCorpus replays every reference string of the seeded
// lectionaries through the parser and compares with the Rails output.
func TestParserMatchesRailsCorpus(t *testing.T) {
	f, err := os.Open("../../test/golden/references.jsonl.gz")
	if err != nil {
		t.Skip("golden corpus not present")
	}
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n, bad := 0, 0
	for sc.Scan() {
		var row struct {
			Ref        string      `json:"ref"`
			Normalized string      `json:"normalized"`
			Segments   []goldenSeg `json:"segments"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		n++
		if got := Normalize(row.Ref); got != row.Normalized {
			bad++
			if bad < 15 {
				t.Errorf("normalize %q: got %q want %q", row.Ref, got, row.Normalized)
			}
			continue
		}
		got := ParseAll(row.Ref)
		ok := len(got) == len(row.Segments)
		for i := 0; ok && i < len(got); i++ {
			w := row.Segments[i]
			g := got[i]
			ok = g.Book == w.Book && g.Chapter == w.Chapter && g.VerseStart == deref(w.VerseStart) &&
				g.VerseEnd == deref(w.VerseEnd) && g.FetchAll == w.FetchAll && g.FetchAllFromVerse == w.FetchAllFromVerse
		}
		if !ok {
			bad++
			if bad < 15 {
				t.Errorf("parse %q:\n got  %+v\n want %+v", row.Ref, got, row.Segments)
			}
		}
	}
	t.Logf("%d references, %d mismatches", n, bad)
	if bad > 0 {
		t.Fail()
	}
}
