// Package bible ports the Bible book catalogue, reference parsing and text
// lookup of the Rails application.
package bible

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

type bookPair struct {
	name string
	id   int
}

var (
	books           = map[string]int{}
	normalizedBooks = map[string]int{}
	compactBooks    = map[string]int{}
)

func init() {
	for _, p := range bookPairs {
		books[p.name] = p.id
	}
	// Iterate the hash (last duplicate wins, first position kept) like Ruby.
	seen := map[string]bool{}
	var ordered []bookPair
	for _, p := range bookPairs {
		if !seen[p.name] {
			seen[p.name] = true
			ordered = append(ordered, bookPair{p.name, books[p.name]})
		}
	}
	for _, p := range ordered {
		n := strings.TrimSpace(strings.ToLower(rb.Transliterate(p.name)))
		normalizedBooks[n] = p.id
		c := strings.ReplaceAll(strings.ToLower(rb.Transliterate(p.name)), " ", "")
		if _, ok := compactBooks[c]; !ok {
			compactBooks[c] = p.id
		}
	}
}

// BookID returns the canonical id of a book name (0 when unknown).
func BookID(name string) int {
	s := rubyStrip(name)
	if id, ok := books[s]; ok {
		return id
	}
	t := strings.ToLower(rb.Transliterate(s))
	if id, ok := normalizedBooks[t]; ok {
		return id
	}
	return compactBooks[strings.ReplaceAll(t, " ", "")]
}

// ExactBookID looks a name up in BOOKS only.
func ExactBookID(name string) (int, bool) {
	id, ok := books[name]
	return id, ok
}

// BookNameByID returns the Portuguese canonical name.
func BookNameByID(id int) string { return booksByID[id] }

// CanonicalBookName maps any known name to its Portuguese canonical name.
func CanonicalBookName(name string) string {
	id := BookID(name)
	if id == 0 {
		return name
	}
	return booksByID[id]
}

// IsDeuterocanonical ports deuterocanonical_book?.
func IsDeuterocanonical(name string) bool {
	id, ok := books[CanonicalBookName(name)]
	return ok && id >= 67 && id <= 80
}

// rubyStrip mirrors String#strip (ASCII whitespace and NUL).
func rubyStrip(s string) string {
	return strings.Trim(s, " \t\n\v\f\r\x00")
}
