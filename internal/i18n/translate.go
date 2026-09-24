package i18n

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// MissingTranslation mirrors I18n::MissingTranslationData raised by I18n.t!.
func MissingTranslation(locale, key string) *rb.RubyError {
	return &rb.RubyError{Class: "I18n::MissingTranslationData", Message: "Translation missing: " + locale + "." + key}
}

// Opts are interpolation values (and "count" for pluralization). A key
// present with a nil value interpolates as "".
type Opts map[string]any

// TBang ports I18n.t!(key, locale:, **opts): fallbacks, count
// pluralization and %{} interpolation; a missing key panics.
func TBang(locale, key string, opts Opts) string {
	once.Do(load)
	for _, l := range Fallbacks(locale) {
		v, ok := lookup(l, key)
		if !ok || v == nil {
			continue
		}
		if m, isMap := v.(map[string]any); isMap {
			if count, has := opts["count"]; has && count != nil {
				k := "other"
				n := rb.ToI(count)
				if n == 0 {
					if _, z := m["zero"]; z {
						k = "zero"
					}
				}
				if k != "zero" && n == 1 {
					k = "one"
				}
				entry, ok := m[k]
				if !ok {
					panic(&rb.RubyError{Class: "I18n::InvalidPluralizationData", Message: fmt.Sprintf("translation data %s can not be used with :count => %v. key '%s' is missing.", rb.Inspect(toRB(m)), count, k)})
				}
				return Interpolate(rb.ToS(entry), opts)
			}
			return rb.Inspect(toRB(m))
		}
		return Interpolate(rb.ToS(v), opts)
	}
	panic(MissingTranslation(locale, key))
}

func toRB(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := rb.NewMap()
		for k, e := range x {
			m.Set(k, toRB(e))
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = toRB(e)
		}
		return out
	}
	return v
}

var interpolationRe = regexp.MustCompile(`%%|%\{([^}]+)\}|%<([^>]+)>([^\d]*?\d*\.?\d*[bBdiouxXeEfgGcps])`)

// Interpolate ports I18n.interpolate for %{name} and %<name>fmt.
func Interpolate(s string, opts Opts) string {
	if !strings.Contains(s, "%") {
		return s
	}
	return interpolationRe.ReplaceAllStringFunc(s, func(m string) string {
		if m == "%%" {
			return "%"
		}
		sub := interpolationRe.FindStringSubmatch(m)
		name := sub[1]
		if name == "" {
			name = sub[2]
		}
		v, ok := opts[name]
		if !ok {
			panic(&rb.RubyError{Class: "I18n::MissingInterpolationArgument", Message: fmt.Sprintf("missing interpolation argument :%s in %q (%s given)", name, s, inspectOpts(opts))})
		}
		if sub[3] != "" {
			return fmt.Sprintf("%"+sub[3], v)
		}
		return rb.ToS(v)
	})
}

func inspectOpts(opts Opts) string {
	var parts []string
	for k, v := range opts {
		parts = append(parts, k+": "+rb.Inspect(v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// LocalizeDate ports I18n.l(date, format: :name, locale:).
func LocalizeDate(locale string, t time.Time, format string) string {
	f := TBang(locale, "date.formats."+format, nil)
	names := func(key string, i int) string {
		v, ok := Lookup(locale, "date."+key)
		if !ok {
			panic(MissingTranslation(locale, "date."+key))
		}
		list, _ := v.([]any)
		if i < len(list) && list[i] != nil {
			return rb.ToS(list[i])
		}
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		c := f[i]
		pad := true
		if c == '-' && i+1 < len(f) {
			pad = false
			i++
			c = f[i]
		}
		switch c {
		case 'd':
			if pad {
				fmt.Fprintf(&b, "%02d", t.Day())
			} else {
				fmt.Fprintf(&b, "%d", t.Day())
			}
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'm':
			if pad {
				fmt.Fprintf(&b, "%02d", int(t.Month()))
			} else {
				fmt.Fprintf(&b, "%d", int(t.Month()))
			}
		case 'Y':
			fmt.Fprintf(&b, "%d", t.Year())
		case 'B':
			b.WriteString(names("month_names", int(t.Month())))
		case 'b':
			b.WriteString(names("abbr_month_names", int(t.Month())))
		case 'A':
			b.WriteString(names("day_names", int(t.Weekday())))
		case 'a':
			b.WriteString(names("abbr_day_names", int(t.Weekday())))
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(c)
		}
	}
	return b.String()
}
