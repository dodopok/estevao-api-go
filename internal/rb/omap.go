// Package rb reproduces the Ruby/ActiveSupport value semantics the Rails
// application exposes through its HTTP contract: insertion-ordered hashes,
// blank?/present?, to_s/to_i coercions and ActiveSupport's JSON encoding.
//
// Dynamic values use: nil, bool, int (Ruby Integer), float64 (Float), string,
// []any (Array), *Map (Hash), Raw (pre-encoded JSON), time.Time (encoded
// like ActiveSupport::TimeWithZone), civil.Date and Decimal.
package rb

// Map is an insertion-ordered string-keyed hash, like a Ruby Hash with
// symbol or string keys (the distinction is erased when rendered to JSON).
type Map struct {
	keys []string
	vals map[string]any
}

// KV is a key/value pair used to build maps literally.
type KV struct {
	K string
	V any
}

// NewMap builds a map from pairs, in order. Later duplicates overwrite the
// value but keep the first position, like Ruby.
func NewMap(pairs ...KV) *Map {
	m := &Map{vals: make(map[string]any, len(pairs))}
	for _, p := range pairs {
		m.Set(p.K, p.V)
	}
	return m
}

// M is shorthand for building a map from alternating keys and values.
func M(kv ...any) *Map {
	m := &Map{vals: make(map[string]any, len(kv)/2)}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1])
	}
	return m
}

func (m *Map) init() {
	if m.vals == nil {
		m.vals = map[string]any{}
	}
}

// Set assigns key, appending it when new.
func (m *Map) Set(k string, v any) *Map {
	m.init()
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
	return m
}

// Get returns the value for k (nil when absent).
func (m *Map) Get(k string) any {
	if m == nil {
		return nil
	}
	return m.vals[k]
}

// Lookup returns the value and whether the key exists.
func (m *Map) Lookup(k string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[k]
	return v, ok
}

// Has reports whether k is a key.
func (m *Map) Has(k string) bool {
	if m == nil {
		return false
	}
	_, ok := m.vals[k]
	return ok
}

// Delete removes k.
func (m *Map) Delete(k string) {
	if m == nil {
		return
	}
	if _, ok := m.vals[k]; !ok {
		return
	}
	delete(m.vals, k)
	for i, key := range m.keys {
		if key == k {
			m.keys = append(m.keys[:i:i], m.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the keys in order.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Len is the number of keys.
func (m *Map) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// Each iterates in order.
func (m *Map) Each(f func(k string, v any)) {
	if m == nil {
		return
	}
	for _, k := range m.keys {
		f(k, m.vals[k])
	}
}

// Dup is a shallow copy.
func (m *Map) Dup() *Map {
	out := &Map{vals: map[string]any{}}
	if m == nil {
		return out
	}
	out.keys = append([]string(nil), m.keys...)
	for k, v := range m.vals {
		out.vals[k] = v
	}
	return out
}

// Merge returns a copy with other's pairs applied (Ruby Hash#merge).
func (m *Map) Merge(other *Map) *Map {
	out := m.Dup()
	other.Each(func(k string, v any) { out.Set(k, v) })
	return out
}

// Slice returns a new map with only keys present in m, in the order of keys
// given (Ruby Hash#slice follows the argument order).
func (m *Map) Slice(keys ...string) *Map {
	out := &Map{vals: map[string]any{}}
	for _, k := range keys {
		if v, ok := m.Lookup(k); ok {
			out.Set(k, v)
		}
	}
	return out
}

// Except returns a copy without keys.
func (m *Map) Except(keys ...string) *Map {
	out := m.Dup()
	for _, k := range keys {
		out.Delete(k)
	}
	return out
}

// Compact drops nil values.
func (m *Map) Compact() *Map {
	out := &Map{vals: map[string]any{}}
	m.Each(func(k string, v any) {
		if v != nil {
			out.Set(k, v)
		}
	})
	return out
}

// String returns the value at k as a string (Ruby value.to_s).
func (m *Map) String(k string) string { return ToS(m.Get(k)) }

// Map returns the nested map at k or nil.
func (m *Map) Map(k string) *Map {
	v, _ := m.Get(k).(*Map)
	return v
}

// Dig follows keys through nested maps.
func (m *Map) Dig(keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(*Map)
		if !ok || mm == nil {
			return nil
		}
		cur = mm.Get(k)
	}
	return cur
}
