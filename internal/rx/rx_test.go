package rx

import (
	"github.com/dlclark/regexp2"
	"reflect"
	"testing"
)

func TestRubySemantics(t *testing.T) {
	re := MustCompile(`-(\d+),(\d+[a-z]?)(?=\s*(?:,|;|\z))`, "i")
	if got := re.Gsub("Amós 3:12-4,5", `-\1:\2`); got != "Amós 3:12-4:5" {
		t.Errorf("gsub lookahead: %q", got)
	}
	if !MustCompile(`\A\d?\s*\p{L}[\p{L}\s.]*\z`).MatchString("Judas") {
		t.Error("unicode letter class")
	}
	if MustCompile(`\d`).MatchString("٣") {
		t.Error(`\d must be ASCII like Ruby`)
	}
	if !MustCompile(`^b`).MatchString("a\nb") {
		t.Error("^ must be a line anchor like Ruby")
	}
	if !MustCompile(`[[:alpha:]]`).MatchString("é") {
		t.Error("posix alpha")
	}
	m := MustCompile(`\A(?<book>\d*\s*[^\d(]+?)\s*\((?<optional>[^)]+)\)`).Find("Matthew (26:36-75) 27:1")
	if m == nil || m.N("book") != "Matthew" || m.N("optional") != "26:36-75" {
		t.Errorf("named groups: %+v", m)
	}
	if got := MustCompile(`[,;]`).Split("a, b;c", 0); !reflect.DeepEqual(got, []string{"a", " b", "c"}) {
		t.Errorf("split: %#v", got)
	}
	if got := MustCompile(`\s+(?:or|ou)\s+`, "i").Split("A 1 or B 2", 0); !reflect.DeepEqual(got, []string{"A 1", "B 2"}) {
		t.Errorf("split alt: %#v", got)
	}
	if got := MustCompile(`(\d+)`).Sub("a1b2", `<\1>`); got != "a<1>b2" {
		t.Errorf("sub: %s", got)
	}
}

// Ruby's \b treats accented letters as word characters (\w does not).
func TestWordBoundaryIsUnicode(t *testing.T) {
	if MustCompile(`x\b`).MatchString("xé") {
		t.Error(`x\b matched "xé"`)
	}
	if got := MustCompile(`\bà\(ao\)`, "i").Index(" à(ao)"); got != 1 {
		t.Errorf("index = %d, want 1", got)
	}
	if MustCompile(`\bà\(ao\)`, "i").MatchString("dà(ao)") {
		t.Error("matched inside a word")
	}
	if MustCompile(`\w`).MatchString("é") {
		t.Error(`\w matched "é"`)
	}
}

// A non-positive MatchTimeout is an expired deadline in regexp2; matches
// then fail at random under load. Patterns must never time out.
func TestNoMatchDeadline(t *testing.T) {
	if got := MustCompile(`a+b`).re.MatchTimeout; got != regexp2.DefaultMatchTimeout {
		t.Fatalf("MatchTimeout = %v", got)
	}
}
