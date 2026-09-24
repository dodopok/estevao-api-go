package rb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

// AppZone is Rails' config.time_zone; AR timestamps render in it.
var AppZone = mustZone("America/Sao_Paulo")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Marshaler lets a Go type render itself as a Ruby-compatible value tree.
type Marshaler interface {
	AsJSON() any
}

// JSON encodes v exactly as `render json:` does in the Rails application
// (ActiveSupport JSON over the json 2.21 generator, HTML entities escaped,
// JS separators left alone).
func JSON(v any) []byte {
	var b bytes.Buffer
	encode(&b, v)
	return b.Bytes()
}

func encode(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		encodeString(b, x)
	case Symbol:
		encodeString(b, string(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case float64:
		b.WriteString(JSONFloat(x))
	case float32:
		b.WriteString(JSONFloat(float64(x)))
	case *Map:
		if x == nil {
			b.WriteString("null")
			return
		}
		b.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encodeString(b, k)
			b.WriteByte(':')
			encode(b, x.vals[k])
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			encode(b, e)
		}
		b.WriteByte(']')
	case []string:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			encodeString(b, e)
		}
		b.WriteByte(']')
	case []*Map:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			encode(b, e)
		}
		b.WriteByte(']')
	case []int:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(e))
		}
		b.WriteByte(']')
	case Raw:
		b.WriteString(string(x))
	case time.Time:
		encodeString(b, FormatTime(x))
	case *time.Time:
		if x == nil {
			b.WriteString("null")
		} else {
			encodeString(b, FormatTime(*x))
		}
	case civil.Date:
		encodeString(b, x.ISO())
	case *civil.Date:
		if x == nil {
			b.WriteString("null")
		} else {
			encodeString(b, x.ISO())
		}
	case *string:
		if x == nil {
			b.WriteString("null")
		} else {
			encodeString(b, *x)
		}
	case *int:
		if x == nil {
			b.WriteString("null")
		} else {
			b.WriteString(strconv.Itoa(*x))
		}
	case *int64:
		if x == nil {
			b.WriteString("null")
		} else {
			b.WriteString(strconv.FormatInt(*x, 10))
		}
	case *bool:
		if x == nil {
			b.WriteString("null")
		} else {
			encode(b, *x)
		}
	case *float64:
		if x == nil {
			b.WriteString("null")
		} else {
			b.WriteString(JSONFloat(*x))
		}
	case Decimal:
		encodeString(b, x.String())
	case Marshaler:
		encode(b, x.AsJSON())
	case json.Number:
		b.WriteString(string(x))
	default:
		panic(fmt.Sprintf("rb.JSON: unsupported type %T", v))
	}
}

// FormatTime renders a timestamp like ActiveSupport::TimeWithZone#as_json.
func FormatTime(t time.Time) string {
	return t.In(AppZone).Format("2006-01-02T15:04:05.000-07:00")
}

const hexDigits = "0123456789abcdef"

func encodeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '"':
				b.WriteString(`\"`)
			case c == '\\':
				b.WriteString(`\\`)
			case c == '\b':
				b.WriteString(`\b`)
			case c == '\f':
				b.WriteString(`\f`)
			case c == '\n':
				b.WriteString(`\n`)
			case c == '\r':
				b.WriteString(`\r`)
			case c == '\t':
				b.WriteString(`\t`)
			case c < 0x20:
				b.WriteString(`\u00`)
				b.WriteByte(hexDigits[c>>4])
				b.WriteByte(hexDigits[c&0xf])
			case c == '<':
				b.WriteString(`<`)
			case c == '>':
				b.WriteString(`>`)
			case c == '&':
				b.WriteString(`&`)
			default:
				b.WriteByte(c)
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString("�")
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
}

// JSONFloat renders a float like the json gem's fpconv_dtoa.
func JSONFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return FloatToS(f)
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	neg := f < 0
	s := strconv.FormatFloat(math.Abs(f), 'e', -1, 64)
	epos := strings.IndexByte(s, 'e')
	digits := strings.Replace(s[:epos], ".", "", 1)
	e, _ := strconv.Atoi(s[epos+1:])
	nd := len(digits)
	K := e - (nd - 1)
	exp := K + nd - 1
	if exp < 0 {
		exp = -exp
	}
	var out strings.Builder
	if neg {
		out.WriteByte('-')
	}
	if K >= 0 && exp < 15 {
		out.WriteString(digits)
		out.WriteString(strings.Repeat("0", K))
		out.WriteString(".0")
		return out.String()
	}
	if K < 0 && (K > -7 || exp < 10) {
		offset := nd - (-K)
		if offset <= 0 {
			out.WriteString("0.")
			out.WriteString(strings.Repeat("0", -offset))
			out.WriteString(digits)
		} else {
			out.WriteString(digits[:offset])
			out.WriteByte('.')
			out.WriteString(digits[offset:])
		}
		return out.String()
	}
	maxd := 18
	if neg {
		maxd = 17
	}
	if nd > maxd {
		nd = maxd
	}
	out.WriteByte(digits[0])
	if nd > 1 {
		out.WriteByte('.')
		out.WriteString(digits[1:nd])
	}
	out.WriteByte('e')
	if K+nd-1 < 0 {
		out.WriteByte('-')
	} else {
		out.WriteByte('+')
	}
	out.WriteString(strconv.Itoa(exp))
	return out.String()
}

// String renders a Decimal like BigDecimal#to_s ("12.5", "0.0").
func (d Decimal) String() string {
	if d.R == nil {
		return "0.0"
	}
	s := d.R.FloatString(20)
	s = strings.TrimRight(s, "0")
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}
