package rb

import (
	"fmt"
	"strconv"
	"strings"
)

// JSONGenerate ports the json gem's JSON.generate (no ActiveSupport): only
// quotes, backslashes and C0 controls are escaped in strings (lowercase hex),
// "/", DEL, U+2028 and HTML characters are left as they are.
func JSONGenerate(v any) string {
	var b strings.Builder
	jsonGen(&b, v)
	return b.String()
}

func jsonGen(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case string:
		JSONGenerateString(b, x)
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		b.WriteString(JSONFloat(x))
	case *Map:
		b.WriteByte('{')
		i := 0
		x.Each(func(k string, v any) {
			if i > 0 {
				b.WriteByte(',')
			}
			i++
			JSONGenerateString(b, k)
			b.WriteByte(':')
			jsonGen(b, v)
		})
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			jsonGen(b, e)
		}
		b.WriteByte(']')
	default:
		JSONGenerateString(b, ToS(v))
	}
}

// JSONGenerateString writes s the way JSON.generate does.
func JSONGenerateString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
