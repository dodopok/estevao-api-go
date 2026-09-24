// Package web reproduces the parts of the Rails/Rack HTTP stack that are
// observable by clients: route recognition order and constraints, the
// optional format suffix, parameter parsing, default headers, ETags and
// conditional GET, request ids and the exceptions app.
package web

import (
	"net/url"
	"regexp"
	"strings"
)

// RouteSpec is one Rails route (see routes_table.go).
type RouteSpec struct {
	Verb        string
	Spec        string
	Endpoint    string
	Constraints map[string]string
}

type compiledRoute struct {
	spec  RouteSpec
	verbs map[string]bool
	re    *regexp.Regexp
	names []string
}

// Router recognizes paths in Rails route order.
type Router struct {
	routes []*compiledRoute
}

// NewRouter compiles the route specs.
func NewRouter(specs []RouteSpec) *Router {
	r := &Router{}
	for _, s := range specs {
		re, names := compileSpec(s.Spec, s.Constraints)
		verbs := map[string]bool{}
		for _, v := range strings.Split(s.Verb, "|") {
			verbs[v] = true
		}
		r.routes = append(r.routes, &compiledRoute{spec: s, verbs: verbs, re: re, names: names})
	}
	return r
}

// compileSpec turns a Journey path spec into an anchored regexp.
func compileSpec(spec string, constraints map[string]string) (*regexp.Regexp, []string) {
	var b strings.Builder
	var names []string
	b.WriteString("^")
	for i := 0; i < len(spec); {
		c := spec[i]
		switch {
		case c == '(':
			b.WriteString("(?:")
			i++
		case c == ')':
			b.WriteString(")?")
			i++
		case c == ':' || c == '*':
			j := i + 1
			for j < len(spec) && (isWord(spec[j])) {
				j++
			}
			name := spec[i+1 : j]
			names = append(names, name)
			pat := `[^/.?]+`
			if c == '*' {
				pat = `.+?`
			}
			if cons, ok := constraints[name]; ok {
				pat = convertRubyRegex(cons)
			}
			b.WriteString("(?P<" + name + ">" + pat + ")")
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()), names
}

func isWord(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// convertRubyRegex adapts the small subset of Ruby regex syntax used in the
// route constraints to RE2.
func convertRubyRegex(src string) string {
	return "(?:" + src + ")"
}

// Match is a recognized route with its (unescaped) path parameters.
type Match struct {
	Endpoint string
	Params   map[string]string
	Format   string
}

// NormalizePath ports Journey::Router::Utils.normalize_path.
func NormalizePath(path string) string {
	p := "/" + path
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
		p = strings.TrimSuffix(p, "%2F")
	}
	return percentUpper.ReplaceAllStringFunc(p, strings.ToUpper)
}

var percentUpper = regexp.MustCompile(`%[a-f0-9]{2}`)

// Recognize finds the first route for method and raw (escaped) path.
// HEAD requests are recognized by GET routes, like Rails.
func (r *Router) Recognize(method, rawPath string) (*Match, bool) {
	path := NormalizePath(rawPath)
	for _, rt := range r.routes {
		if !rt.verbs[method] && !(method == "HEAD" && rt.verbs["GET"]) {
			continue
		}
		m := rt.re.FindStringSubmatch(path)
		if m == nil {
			continue
		}
		params := map[string]string{}
		for i, name := range rt.re.SubexpNames() {
			if name == "" || i >= len(m) {
				continue
			}
			if m[i] == "" && name == "format" {
				continue
			}
			if m[i] == "" {
				continue
			}
			params[name] = unescapeURI(m[i])
		}
		format := params["format"]
		return &Match{Endpoint: rt.spec.Endpoint, Params: params, Format: format}, true
	}
	return nil, false
}

// unescapeURI decodes %XX sequences only ('+' stays), like
// Journey::Router::Utils.unescape_uri.
func unescapeURI(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	out, err := url.PathUnescape(strings.ReplaceAll(s, "+", "%2B"))
	if err != nil {
		return s
	}
	return out
}
