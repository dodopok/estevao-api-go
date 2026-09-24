package web

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// ErrBadParams is returned for query strings Rack refuses to parse
// (invalid percent-encoding, type conflicts, invalid UTF-8); Rails answers
// those with 400 Bad Request.
var ErrBadParams = errors.New("invalid parameters")

const paramDepthLimit = 32

// ParseNestedQuery ports Rack::QueryParser#parse_nested_query.
func ParseNestedQuery(qs string) (*rb.Map, error) {
	params := rb.NewMap()
	if qs == "" {
		return params, nil
	}
	for _, pair := range splitPairs(qs) {
		if pair == "" {
			continue
		}
		var k string
		var v any
		if i := strings.IndexByte(pair, '='); i >= 0 {
			kk, err := decodeComponent(pair[:i])
			if err != nil {
				return nil, err
			}
			vv, err := decodeComponent(pair[i+1:])
			if err != nil {
				return nil, err
			}
			k, v = kk, vv
		} else {
			kk, err := decodeComponent(pair)
			if err != nil {
				return nil, err
			}
			k, v = kk, nil
		}
		if _, err := normalizeParams(params, &k, v, 0); err != nil {
			return nil, err
		}
	}
	return params, nil
}

// splitPairs splits on "&" followed by optional spaces (DEFAULT_SEP = /& */).
func splitPairs(qs string) []string {
	var out []string
	start := 0
	for i := 0; i < len(qs); i++ {
		if qs[i] == '&' {
			out = append(out, qs[start:i])
			j := i + 1
			for j < len(qs) && qs[j] == ' ' {
				j++
			}
			start = j
			i = j - 1
		}
	}
	return append(out, qs[start:])
}

// decodeComponent is URI.decode_www_form_component: '+' is a space and a
// malformed %-escape is an error.
func decodeComponent(s string) (string, error) {
	if !strings.ContainsAny(s, "%+") {
		if !utf8.ValidString(s) {
			return "", ErrBadParams
		}
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '+':
			b.WriteByte(' ')
		case c == '%':
			if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
				return "", ErrBadParams
			}
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
		default:
			b.WriteByte(c)
		}
	}
	out := b.String()
	if !utf8.ValidString(out) {
		return "", ErrBadParams
	}
	return out, nil
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// normalizeParams ports Rack's _normalize_params. name==nil means no key.
func normalizeParams(params *rb.Map, name *string, v any, depth int) (any, error) {
	if depth >= paramDepthLimit {
		return nil, ErrBadParams
	}
	var k, after string
	switch {
	case name == nil:
		k, after = "", ""
	case depth == 0:
		n := *name
		if start := indexFrom(n, '[', 1); start >= 0 {
			k, after = n[:start], n[start:]
		} else {
			k, after = n, ""
		}
	case strings.HasPrefix(*name, "[]"):
		k, after = "[]", (*name)[2:]
	case strings.HasPrefix(*name, "[") && indexFrom(*name, ']', 1) >= 0:
		start := indexFrom(*name, ']', 1)
		k, after = (*name)[1:start], (*name)[start+1:]
	default:
		k, after = *name, ""
	}
	if k == "" {
		return nil, nil
	}
	switch {
	case after == "":
		if k == "[]" && depth != 0 {
			return []any{v}, nil
		}
		params.Set(k, v)
	case after == "[":
		params.Set(*name, v)
	case after == "[]":
		cur, ok := params.Lookup(k)
		if !ok || cur == nil {
			cur = []any{}
		}
		arr, isArr := cur.([]any)
		if !isArr {
			return nil, ErrBadParams
		}
		params.Set(k, append(arr, v))
	case strings.HasPrefix(after, "[]"):
		var childKey string
		if len(after) > 3 && after[2] == '[' && strings.HasSuffix(after, "]") {
			ck := after[3 : len(after)-1]
			if ck != "" && !strings.ContainsAny(ck, "[]") {
				childKey = ck
			}
		}
		if childKey == "" {
			childKey = after[2:]
		}
		cur, ok := params.Lookup(k)
		if !ok || cur == nil {
			cur = []any{}
		}
		arr, isArr := cur.([]any)
		if !isArr {
			return nil, ErrBadParams
		}
		var last *rb.Map
		if len(arr) > 0 {
			last, _ = arr[len(arr)-1].(*rb.Map)
		}
		if last != nil && !paramsHashHasKey(last, childKey) {
			if _, err := normalizeParams(last, &childKey, v, depth+1); err != nil {
				return nil, err
			}
		} else {
			child := rb.NewMap()
			res, err := normalizeParams(child, &childKey, v, depth+1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, res)
		}
		params.Set(k, arr)
	default:
		cur, ok := params.Lookup(k)
		if !ok || cur == nil {
			cur = rb.NewMap()
		}
		m, isMap := cur.(*rb.Map)
		if !isMap {
			return nil, ErrBadParams
		}
		res, err := normalizeParams(m, &after, v, depth+1)
		if err != nil {
			return nil, err
		}
		params.Set(k, res)
	}
	return params, nil
}

func indexFrom(s string, c byte, from int) int {
	if from >= len(s) {
		return -1
	}
	if i := strings.IndexByte(s[from:], c); i >= 0 {
		return i + from
	}
	return -1
}

func paramsHashHasKey(h *rb.Map, key string) bool {
	if strings.Contains(key, "[]") {
		return false
	}
	var cur any = h
	for _, part := range splitBrackets(key) {
		if part == "" {
			continue
		}
		m, ok := cur.(*rb.Map)
		if !ok || !m.Has(part) {
			return false
		}
		cur = m.Get(part)
	}
	return true
}

func splitBrackets(key string) []string {
	return strings.FieldsFunc(key, func(r rune) bool { return r == '[' || r == ']' })
}
