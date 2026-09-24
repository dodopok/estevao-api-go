// Package rx runs Ruby (Onigmo) regular expressions with Ruby semantics on
// top of a backtracking engine: lookaround, named groups and \p{..} work as
// written, "^"/"$" are line anchors, and \d \w \s \b \h are ASCII-only, as
// they are in Ruby. Ported code keeps its patterns verbatim.
package rx

import (
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

// Regexp is a compiled Ruby regular expression.
type Regexp struct {
	re  *regexp2.Regexp
	src string
}

var cache sync.Map

// MustCompile compiles a Ruby pattern. flags may contain i, m (Ruby's
// dot-matches-newline) and x.
func MustCompile(src string, flags ...string) *Regexp {
	f := strings.Join(flags, "")
	key := f + "\x00" + src
	if v, ok := cache.Load(key); ok {
		return v.(*Regexp)
	}
	opts := regexp2.RegexOptions(regexp2.Multiline)
	if strings.Contains(f, "i") {
		opts |= regexp2.IgnoreCase
	}
	if strings.Contains(f, "m") {
		opts |= regexp2.Singleline
	}
	if strings.Contains(f, "x") {
		opts |= regexp2.IgnorePatternWhitespace
	}
	re := regexp2.MustCompile(translate(src), opts)
	re.MatchTimeout = -1
	r := &Regexp{re: re, src: src}
	cache.Store(key, r)
	return r
}

// Quote mirrors Regexp.escape.
func Quote(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '.', '*', '?', '+', '^', '$', '|', '(', ')', '[', ']', '{', '}', '\\', '/', '-':
			b.WriteByte('\\')
			b.WriteRune(r)
		case ' ':
			b.WriteString("\\ ")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '#':
			b.WriteString("\\#")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

const (
	asciiWord  = `a-zA-Z0-9_`
	asciiSpace = ` \t\n\x0B\f\r`
)

var posixClasses = map[string]string{
	"alpha": `\p{L}\p{M}\p{Nl}`, "digit": `0-9`, "alnum": `\p{L}\p{M}\p{Nl}\p{Nd}`,
	"upper": `\p{Lu}`, "lower": `\p{Ll}`, "space": `\s`, "punct": `\p{P}`, "word": `\p{L}\p{M}\p{Nd}\p{Pc}`,
	"xdigit": `0-9a-fA-F`, "blank": ` \t`, "cntrl": `\p{Cc}`,
}

// translate rewrites Ruby-specific escapes into .NET equivalents.
func translate(src string) string {
	var b strings.Builder
	inClass := 0
	rs := []rune(src)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c == '[' {
			// POSIX bracket expression inside a class: [[:alpha:]]
			if inClass > 0 && i+1 < len(rs) && rs[i+1] == ':' {
				end := strings.Index(string(rs[i:]), ":]")
				if end > 0 {
					name := string(rs[i+2 : i+end])
					neg := strings.HasPrefix(name, "^")
					name = strings.TrimPrefix(name, "^")
					if rep, ok := posixClasses[name]; ok && !neg {
						b.WriteString(rep)
						i += len([]rune(string(rs[i:i+end]))) + 1
						continue
					}
				}
			}
			inClass++
			b.WriteRune(c)
			continue
		}
		if c == ']' && inClass > 0 {
			inClass--
			b.WriteRune(c)
			continue
		}
		if c == '\\' && i+1 < len(rs) {
			n := rs[i+1]
			i++
			if inClass > 0 {
				switch n {
				case 'd':
					b.WriteString("0-9")
				case 'w':
					b.WriteString(asciiWord)
				case 's':
					b.WriteString(` \t\n\x0B\f\r`)
				case 'h':
					b.WriteString("0-9a-fA-F")
				default:
					b.WriteRune('\\')
					b.WriteRune(n)
				}
				continue
			}
			switch n {
			case 'd':
				b.WriteString("[0-9]")
			case 'D':
				b.WriteString("[^0-9]")
			case 'w':
				b.WriteString("[" + asciiWord + "]")
			case 'W':
				b.WriteString("[^" + asciiWord + "]")
			case 's':
				b.WriteString("[" + asciiSpace + "]")
			case 'S':
				b.WriteString("[^" + asciiSpace + "]")
			case 'h':
				b.WriteString("[0-9a-fA-F]")
			case 'H':
				b.WriteString("[^0-9a-fA-F]")
			case 'b':
				b.WriteString(`(?:(?<=[a-zA-Z0-9_])(?![a-zA-Z0-9_])|(?<![a-zA-Z0-9_])(?=[a-zA-Z0-9_]))`)
			case 'B':
				b.WriteString(`(?:(?<=[a-zA-Z0-9_])(?=[a-zA-Z0-9_])|(?<![a-zA-Z0-9_])(?![a-zA-Z0-9_]))`)
			default:
				b.WriteRune('\\')
				b.WriteRune(n)
			}
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// Match is one regex match with Ruby MatchData-like accessors.
type Match struct {
	m     *regexp2.Match
	input []rune
}

// Group returns group i and whether it participated.
func (m *Match) Group(i int) (string, bool) {
	g := m.m.GroupByNumber(i)
	if g == nil || len(g.Captures) == 0 {
		return "", false
	}
	return g.String(), true
}

// G returns group i ("" when it did not participate).
func (m *Match) G(i int) string { s, _ := m.Group(i); return s }

// Named returns a named group and whether it participated.
func (m *Match) Named(name string) (string, bool) {
	g := m.m.GroupByName(name)
	if g == nil || len(g.Captures) == 0 {
		return "", false
	}
	return g.String(), true
}

// N returns a named group ("" when absent).
func (m *Match) N(name string) string { s, _ := m.Named(name); return s }

// String is the whole match.
func (m *Match) String() string { return m.m.String() }

// Begin/End are rune offsets of the whole match.
func (m *Match) Begin() int { return m.m.Index }
func (m *Match) End() int   { return m.m.Index + m.m.Length }

// PreMatch/PostMatch mirror Ruby's.
func (m *Match) PreMatch() string  { return string(m.input[:m.Begin()]) }
func (m *Match) PostMatch() string { return string(m.input[m.End():]) }

// GroupCount is the number of capture groups.
func (m *Match) GroupCount() int { return m.m.GroupCount() - 1 }

// Find returns the first match (Ruby =~ / match).
func (r *Regexp) Find(s string) *Match { return r.FindFrom(s, 0) }

// FindFrom matches starting at rune offset pos.
func (r *Regexp) FindFrom(s string, pos int) *Match {
	runes := []rune(s)
	m, err := r.re.FindRunesMatchStartingAt(runes, pos)
	if err != nil || m == nil {
		return nil
	}
	return &Match{m: m, input: runes}
}

// MatchString mirrors String#match?.
func (r *Regexp) MatchString(s string) bool {
	ok, err := r.re.MatchString(s)
	return err == nil && ok
}

// FindAll returns all non-overlapping matches (String#scan).
func (r *Regexp) FindAll(s string) []*Match {
	runes := []rune(s)
	var out []*Match
	m, _ := r.re.FindRunesMatch(runes)
	for m != nil {
		out = append(out, &Match{m: m, input: runes})
		m, _ = r.re.FindNextMatch(m)
	}
	return out
}

// Scan mirrors String#scan: whole matches when the pattern has no groups,
// otherwise the first group of each match.
func (r *Regexp) Scan(s string) [][]string {
	var out [][]string
	for _, m := range r.FindAll(s) {
		n := m.GroupCount()
		if n == 0 {
			out = append(out, []string{m.String()})
			continue
		}
		row := make([]string, n)
		for i := 1; i <= n; i++ {
			row[i-1] = m.G(i)
		}
		out = append(out, row)
	}
	return out
}

// Gsub replaces every match using a Ruby replacement string (\1, \k<n>, \0).
func (r *Regexp) Gsub(s, repl string) string {
	return r.GsubFunc(s, func(m *Match) string { return expand(m, repl) })
}

// Sub replaces the first match.
func (r *Regexp) Sub(s, repl string) string {
	return r.SubFunc(s, func(m *Match) string { return expand(m, repl) })
}

// GsubFunc replaces every match with f's result (gsub with a block).
func (r *Regexp) GsubFunc(s string, f func(m *Match) string) string {
	matches := r.FindAll(s)
	if len(matches) == 0 {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(string(runes[last:m.Begin()]))
		b.WriteString(f(m))
		last = m.End()
	}
	b.WriteString(string(runes[last:]))
	return b.String()
}

// SubFunc replaces the first match with f's result.
func (r *Regexp) SubFunc(s string, f func(m *Match) string) string {
	m := r.Find(s)
	if m == nil {
		return s
	}
	runes := []rune(s)
	return string(runes[:m.Begin()]) + f(m) + string(runes[m.End():])
}

func expand(m *Match, repl string) string {
	if !strings.Contains(repl, `\`) {
		return repl
	}
	var b strings.Builder
	rs := []rune(repl)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' || i+1 >= len(rs) {
			b.WriteRune(rs[i])
			continue
		}
		n := rs[i+1]
		switch {
		case n >= '0' && n <= '9':
			b.WriteString(m.G(int(n - '0')))
			i++
		case n == '&':
			b.WriteString(m.String())
			i++
		case n == '\\':
			b.WriteRune('\\')
			i++
		case n == '`':
			b.WriteString(m.PreMatch())
			i++
		case n == '\'':
			b.WriteString(m.PostMatch())
			i++
		case n == 'k' && i+2 < len(rs) && rs[i+2] == '<':
			end := strings.IndexRune(string(rs[i+3:]), '>')
			if end < 0 {
				b.WriteRune('\\')
				continue
			}
			name := string(rs[i+3 : i+3+end])
			if num, err := strconv.Atoi(name); err == nil {
				b.WriteString(m.G(num))
			} else {
				b.WriteString(m.N(name))
			}
			i += 3 + end
		default:
			b.WriteRune('\\')
		}
	}
	return b.String()
}

// Split mirrors String#split(regexp, limit): captured groups are included,
// and with limit 0 trailing empty strings are removed. A leading empty
// field is kept unless the match is empty.
func (r *Regexp) Split(s string, limit int) []string {
	if s == "" {
		return []string{}
	}
	runes := []rune(s)
	var out []string
	start := 0
	pos := 0
	for {
		if limit > 0 && len(out) == limit-1 {
			break
		}
		m, err := r.re.FindRunesMatchStartingAt(runes, pos)
		if err != nil || m == nil {
			break
		}
		mb, me := m.Index, m.Index+m.Length
		if me == mb {
			if mb >= len(runes) {
				break
			}
			if mb == start {
				pos = mb + 1
				if mb == 0 {
					continue
				}
				// empty match right after previous split point
				out = append(out, string(runes[start:mb+1]))
				start = mb + 1
				continue
			}
		}
		out = append(out, string(runes[start:mb]))
		mm := &Match{m: m, input: runes}
		for i := 1; i <= mm.GroupCount(); i++ {
			if g, ok := mm.Group(i); ok {
				out = append(out, g)
			}
		}
		start = me
		pos = me
		if me == mb {
			pos = me + 1
		}
	}
	out = append(out, string(runes[start:]))
	if limit == 0 {
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
	}
	return out
}

// Index returns the rune index of the first match or -1 (String#index).
func (r *Regexp) Index(s string) int {
	m := r.Find(s)
	if m == nil {
		return -1
	}
	return m.Begin()
}
