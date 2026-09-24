package audio

import (
	"bufio"
	"compress/gzip"
	"os"
	"testing"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// TestLinesMatchRails replays tools/golden/audio_lines.rb output
// (AUDIO_GOLDEN): the context check, normalization, segmentation and clip
// keys of every distinct spoken line of an office corpus.
func TestLinesMatchRails(t *testing.T) {
	path := os.Getenv("AUDIO_GOLDEN")
	if path == "" {
		t.Skip("AUDIO_GOLDEN required")
	}
	for _, env := range []string{"AUDIO_TTS_PROVIDER", "OPENAI_TTS_MODEL", "OPENAI_TTS_VOICE", "OPENAI_TTS_SPEED",
		"OPENAI_TTS_INSTRUCTIONS", "OPENAI_TTS_LANGUAGE", "GOOGLE_TTS_MODEL", "GOOGLE_TTS_VOICE", "GOOGLE_TTS_SPEED",
		"GOOGLE_TTS_INSTRUCTIONS", "GOOGLE_TTS_LANGUAGE", "GOOGLE_TTS_SHORT_INPUT_MAX_CHARACTERS"} {
		if v, ok := os.LookupEnv(env); ok {
			os.Unsetenv(env)
			t.Cleanup(func() { os.Setenv(env, v) })
		}
	}
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
	gens := map[string]*Generator{}
	gen := func(name, lang string) *Generator {
		k := name + "\x00" + lang
		if gens[k] == nil {
			l := lang
			gens[k] = NewGenerator(Build(name, &l))
		}
		return gens[k]
	}
	for sc.Scan() {
		v, err := rb.ParseJSON(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		rec := v.(*rb.Map)
		n++
		text, typ, lang := rb.ToS(rec.Get("text")), rb.ToS(rec.Get("type")), rb.ToS(rec.Get("language"))
		e := Entry{Text: text, Type: typ, Slug: rec.Get("slug"), VerseNumber: rec.Get("verse_number")}
		var problems []string
		if got := ContextRequired(text, lang); got != rec.Get("context_required").(bool) {
			problems = append(problems, "context_required")
		}
		wantNorm := rb.ToS(rec.Get("normalized"))
		gotNorm := Normalize(text, typ, rb.ToS(e.Slug), e.VerseNumber, lang)
		if gotNorm != wantNorm {
			problems = append(problems, "normalized\n   rails: "+wantNorm+"\n   go:    "+gotNorm)
		}
		for _, name := range []string{"openai", "google"} {
			g := gen(name, lang)
			want, _ := rec.Get(name).([]any)
			got := g.SegmentsFor(e)
			ok := len(want) == len(got)
			for i := 0; ok && i < len(got); i++ {
				pair := want[i].([]any)
				ok = got[i] == pair[0].(string) && ClipKey(g.provider, got[i]) == pair[1].(string)
			}
			if !ok {
				problems = append(problems, name+" segments/keys: rails "+rb.Inspect(want)+" go "+rb.Inspect(got))
			}
		}
		if len(problems) > 0 {
			bad++
			if bad <= 25 {
				t.Errorf("[%s %s %s] %q\n  %v", lang, typ, rb.ToS(e.Slug), text, problems)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d lines, %d mismatches", n, bad)
}
