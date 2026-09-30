package bible

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func translationFile(lines ...string) *bytes.Reader {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, _ = gz.Write([]byte(strings.Join(lines, "\n") + "\n"))
	_ = gz.Close()
	return bytes.NewReader(b.Bytes())
}

const header2 = `{"format":"estevao-bible-v1","translation":"tst","verses":2}`

func TestReadTranslation(t *testing.T) {
	h, verses, warnings, err := ReadTranslation(translationFile(header2,
		`{"book":"Gênesis","book_number":1,"chapter":1,"verse":1,"text":"No princípio"}`,
		`{"book":"Genesis","book_number":1,"chapter":1,"verse":2,"text":"A terra"}`))
	if err != nil {
		t.Fatal(err)
	}
	if h.Translation != "tst" || len(verses) != 2 || len(warnings) != 1 || !strings.Contains(warnings[0], "Gênesis") {
		t.Fatalf("header %+v, %d verses, warnings %v", h, len(verses), warnings)
	}
}

func TestReadTranslationRejects(t *testing.T) {
	ok := `{"book":"Gênesis","book_number":1,"chapter":1,"verse":1,"text":"x"}`
	cases := map[string][]string{
		"not a header":    {`{"translation":"tst"}`, ok},
		"uppercase code":  {`{"format":"estevao-bible-v1","translation":"NVI","verses":1}`, ok},
		"count mismatch":  {header2, ok},
		"duplicate verse": {header2, ok, ok},
		"unknown book":    {`{"format":"estevao-bible-v1","translation":"tst","verses":1}`, `{"book":"X","book_number":999,"chapter":1,"verse":1,"text":"x"}`},
		"zero chapter":    {`{"format":"estevao-bible-v1","translation":"tst","verses":1}`, `{"book":"Gênesis","book_number":1,"chapter":0,"verse":1,"text":"x"}`},
		"empty text":      {`{"format":"estevao-bible-v1","translation":"tst","verses":1}`, `{"book":"Gênesis","book_number":1,"chapter":1,"verse":1,"text":" "}`},
		"malformed line":  {`{"format":"estevao-bible-v1","translation":"tst","verses":1}`, `{"book":`},
	}
	for name, lines := range cases {
		if _, _, _, err := ReadTranslation(translationFile(lines...)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, _, _, err := ReadTranslation(bytes.NewReader([]byte("plain text"))); err == nil {
		t.Error("an uncompressed file was accepted")
	}
}
