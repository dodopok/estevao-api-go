package web

import (
	"regexp"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

var nestedAttributeKey = regexp.MustCompile(`\A-?[0-9]+\z`)

// PermitHashOrArray ports how Parameters#permit filters a value declared as
// `key: [:field, ...]` (permit_hash_or_array): an Array keeps its hashes, a
// hash with any fields_for-style entry ("0" => {...}) keeps only those
// entries, and any other hash is permitted as one record. Scalars are
// dropped (ok is false).
func PermitHashOrArray(v any, permit func(*rb.Map) *rb.Map) (any, bool) {
	switch x := v.(type) {
	case []any:
		out := []any{}
		for _, el := range x {
			if m, ok := el.(*rb.Map); ok {
				out = append(out, permit(m))
			}
		}
		return out, true
	case *rb.Map:
		nested := false
		x.Each(func(k string, v any) {
			if _, ok := v.(*rb.Map); ok && nestedAttributeKey.MatchString(k) {
				nested = true
			}
		})
		if !nested {
			return permit(x), true
		}
		out := rb.NewMap()
		x.Each(func(k string, v any) {
			if m, ok := v.(*rb.Map); ok && nestedAttributeKey.MatchString(k) {
				out.Set(k, permit(m))
			}
		})
		return out, true
	}
	return nil, false
}

// PermittedScalar reports whether a parsed parameter value passes a scalar
// filter (anything but a hash or an array, as JSON and query parsing
// produce them).
func PermittedScalar(v any) bool {
	switch v.(type) {
	case *rb.Map, []any:
		return false
	}
	return true
}
