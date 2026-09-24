package activestorage

import "strconv"

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

// unmarshalRuby decodes the subset of Ruby's Marshal format that legacy
// signed ids carry: nil, booleans, Integers, Strings (with their encoding
// ivars), Symbols, Arrays and Hashes. Hash keys become strings.
func unmarshalRuby(b []byte) (any, bool) {
	if len(b) < 2 || b[0] != 4 || b[1] != 8 {
		return nil, false
	}
	d := &marshalDecoder{b: b, pos: 2}
	v, ok := d.value()
	return v, ok && d.pos == len(b)
}

type marshalDecoder struct {
	b       []byte
	pos     int
	symbols []string
	objects []any
}

func (d *marshalDecoder) byte() (byte, bool) {
	if d.pos >= len(d.b) {
		return 0, false
	}
	c := d.b[d.pos]
	d.pos++
	return c, true
}

// long reads Marshal's packed integer.
func (d *marshalDecoder) long() (int64, bool) {
	c, ok := d.byte()
	if !ok {
		return 0, false
	}
	n := int64(int8(c))
	switch {
	case n == 0:
		return 0, true
	case n >= 5:
		return n - 5, true
	case n <= -5:
		return n + 5, true
	case n > 0:
		var x int64
		for i := int64(0); i < n; i++ {
			c, ok := d.byte()
			if !ok {
				return 0, false
			}
			x |= int64(c) << (8 * i)
		}
		return x, true
	default:
		x := int64(-1)
		for i := int64(0); i < -n; i++ {
			c, ok := d.byte()
			if !ok {
				return 0, false
			}
			x &= ^(int64(0xff) << (8 * i))
			x |= int64(c) << (8 * i)
		}
		return x, true
	}
}

func (d *marshalDecoder) bytes() (string, bool) {
	n, ok := d.long()
	if !ok || n < 0 || d.pos+int(n) > len(d.b) {
		return "", false
	}
	s := string(d.b[d.pos : d.pos+int(n)])
	d.pos += int(n)
	return s, true
}

func (d *marshalDecoder) value() (any, bool) {
	c, ok := d.byte()
	if !ok {
		return nil, false
	}
	switch c {
	case '0':
		return nil, true
	case 'T':
		return true, true
	case 'F':
		return false, true
	case 'i':
		n, ok := d.long()
		return n, ok
	case ':':
		s, ok := d.bytes()
		d.symbols = append(d.symbols, s)
		return s, ok
	case ';':
		n, ok := d.long()
		if !ok || n < 0 || int(n) >= len(d.symbols) {
			return nil, false
		}
		return d.symbols[n], true
	case '@':
		n, ok := d.long()
		if !ok || n < 0 || int(n) >= len(d.objects) {
			return nil, false
		}
		return d.objects[n], true
	case '"':
		s, ok := d.bytes()
		d.objects = append(d.objects, s)
		return s, ok
	case 'I':
		v, ok := d.value()
		if !ok {
			return nil, false
		}
		n, ok := d.long()
		if !ok {
			return nil, false
		}
		for i := int64(0); i < n; i++ {
			if _, ok := d.value(); !ok {
				return nil, false
			}
			if _, ok := d.value(); !ok {
				return nil, false
			}
		}
		return v, true
	case '[':
		n, ok := d.long()
		if !ok || n < 0 {
			return nil, false
		}
		arr := make([]any, 0, n)
		d.objects = append(d.objects, arr)
		for i := int64(0); i < n; i++ {
			v, ok := d.value()
			if !ok {
				return nil, false
			}
			arr = append(arr, v)
		}
		return arr, true
	case '{':
		n, ok := d.long()
		if !ok || n < 0 {
			return nil, false
		}
		m := map[string]any{}
		d.objects = append(d.objects, m)
		for i := int64(0); i < n; i++ {
			k, ok := d.value()
			if !ok {
				return nil, false
			}
			v, ok := d.value()
			if !ok {
				return nil, false
			}
			ks, isStr := k.(string)
			if !isStr {
				return nil, false
			}
			m[ks] = v
		}
		return m, true
	}
	return nil, false
}
