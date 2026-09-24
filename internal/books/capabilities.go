// Package books ports PrayerBooks::Capabilities and PrayerBooks::OfficeNames.
package books

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// DefaultOffices are the four standard offices.
var DefaultOffices = []string{"morning", "midday", "evening", "compline"}

// DefaultCode is PrayerBooks::Defaults::CODE.
const DefaultCode = "loc_2015"

// Capabilities is the typed boundary over prayer_books.features.
type Capabilities struct {
	Code     string
	Features *rb.Map
}

// For builds (and validates) the capabilities of a book.
func For(code string, features *rb.Map) *Capabilities {
	if features == nil {
		features = rb.NewMap()
	}
	c := &Capabilities{Code: code, Features: features}
	c.validate()
	return c
}

func invalid(msg string) {
	panic(&web.DomainError{Class: "InvalidPreference", Code: "INVALID_CAPABILITIES", Message: msg})
}

func (c *Capabilities) validate() {
	lect, lok := c.Features.Lookup("lectionary")
	if lok && lect != nil {
		if _, ok := lect.(*rb.Map); !ok {
			invalid("lectionary capabilities must be an object")
		}
	}
	do, dok := c.Features.Lookup("daily_office")
	if dok && do != nil {
		if _, ok := do.(*rb.Map); !ok {
			invalid("daily_office capabilities must be an object")
		}
	}
	lm, _ := lect.(*rb.Map)
	dm, _ := do.(*rb.Map)
	validateArray(lm, "reading_types")
	validateArray(lm, "variants")
	validateArray(dm, "available_offices")
	validateArray(dm, "family_rite_offices")
	if lm != nil {
		def := lm.Get("default_variant")
		if rb.Present(def) {
			variants := toStrings(lm.Get("variants"))
			if !containsStr(variants, rb.ToS(def)) {
				invalid("default_variant must be listed in variants")
			}
		}
	}
}

func validateArray(parent *rb.Map, key string) {
	if parent == nil || !parent.Has(key) {
		return
	}
	if _, ok := parent.Get(key).([]any); !ok {
		invalid(key + " capabilities must be an array")
	}
}

// Dig ports Capabilities#[] with a dotted path.
func (c *Capabilities) Dig(path string) any {
	var cur any = c.Features
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(*rb.Map)
		if !ok {
			return nil
		}
		cur = m.Get(k)
	}
	return cur
}

// Supports ports supports?.
func (c *Capabilities) Supports(path string) bool {
	v := c.Dig(path)
	return v == true || rb.Present(v)
}

func (c *Capabilities) Lectionary() *rb.Map {
	if m, ok := c.Features.Get("lectionary").(*rb.Map); ok {
		return m
	}
	return rb.NewMap()
}

func (c *Capabilities) DailyOffice() *rb.Map {
	if m, ok := c.Features.Get("daily_office").(*rb.Map); ok {
		return m
	}
	return rb.NewMap()
}

// ArrayOf mirrors Ruby Array(value).map(&:to_s).
func ArrayOf(v any) []string { return toStrings(v) }

func toStrings(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = rb.ToS(e)
		}
		return out
	case *rb.Map:
		var out []string
		x.Each(func(k string, e any) { out = append(out, rb.Inspect([]any{k, e})) })
		return out
	}
	return []string{rb.ToS(v)}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// AvailableOffices ports available_offices.
func (c *Capabilities) AvailableOffices(familyRite bool) []string {
	if familyRite && !c.SupportsFamilyRite() {
		return []string{}
	}
	do := c.DailyOffice()
	var config any
	if familyRite && do.Has("family_rite_offices") {
		config = do.Get("family_rite_offices")
	} else {
		config = do.Get("available_offices")
	}
	if config == nil {
		return append([]string{}, DefaultOffices...)
	}
	return toStrings(config)
}

func (c *Capabilities) SupportsFamilyRite() bool {
	return c.DailyOffice().Get("supports_family_rite") == true
}
func (c *Capabilities) SupportsVigil() bool { return c.Lectionary().Get("supports_vigil") == true }

func (c *Capabilities) AvailableReadingTypes() []string {
	return toStrings(c.Lectionary().Get("reading_types"))
}

// DefaultReadingType returns "" when absent.
func (c *Capabilities) DefaultReadingType() string {
	v := c.Lectionary().Get("default_reading_type")
	if rb.Present(v) {
		return rb.ToS(v)
	}
	return ""
}

// NormalizeReadingType ports normalize_reading_type.
func (c *Capabilities) NormalizeReadingType(value any) string {
	def := c.DefaultReadingType()
	selected := ""
	if rb.Present(value) {
		selected = rb.ToS(value)
	} else if def != "" {
		selected = def
	} else {
		selected = "semicontinuous"
	}
	available := c.AvailableReadingTypes()
	if len(available) == 0 {
		if def != "" {
			available = []string{def}
		} else {
			available = []string{"semicontinuous"}
		}
	}
	if containsStr(available, selected) {
		return selected
	}
	if def != "" && containsStr(available, def) {
		return def
	}
	return available[0]
}

func (c *Capabilities) AvailableLectionaryVariants() []string {
	return toStrings(c.Lectionary().Get("variants"))
}

func (c *Capabilities) DefaultLectionaryVariant() string {
	v := c.Lectionary().Get("default_variant")
	if rb.Present(v) {
		return rb.ToS(v)
	}
	return ""
}

// NormalizeLectionaryVariant ports normalize_lectionary_variant ("" == nil).
func (c *Capabilities) NormalizeLectionaryVariant(value any) string {
	if len(c.AvailableLectionaryVariants()) == 0 {
		return ""
	}
	if !rb.Present(value) {
		return c.DefaultLectionaryVariant()
	}
	selected := rb.ToS(value)
	if containsStr(c.AvailableLectionaryVariants(), selected) {
		return selected
	}
	panic(&web.DomainError{Class: "InvalidPreference", Code: "INVALID_LECTIONARY_VARIANT",
		Message: "Lectionary variant '" + rb.ToS(value) + "' is not supported by " + c.Code + "."})
}

// LectionaryServiceVariant ports lectionary_service_variant.
func (c *Capabilities) LectionaryServiceVariant(prefs *rb.Map) string {
	variant := c.NormalizeLectionaryVariant(prefs.Get("lectionary_variant"))
	pv, _ := c.Lectionary().Get("preference_variants").(*rb.Map)
	pv.Each(func(key string, choices any) {
		cm, ok := choices.(*rb.Map)
		if !ok {
			return
		}
		inner, ok := cm.Get(rb.ToS(prefs.Get(key))).(*rb.Map)
		if !ok {
			return
		}
		if v := inner.Get(variant); v != nil && v != false {
			variant = rb.ToS(v)
		}
	})
	return variant
}

// NormalizeLectionaryServiceVariant ports normalize_lectionary_service_variant.
func (c *Capabilities) NormalizeLectionaryServiceVariant(value any) string {
	pv, _ := c.Lectionary().Get("preference_variants").(*rb.Map)
	var qualified []string
	pv.Each(func(_ string, choices any) {
		cm, ok := choices.(*rb.Map)
		if !ok {
			return
		}
		cm.Each(func(_ string, inner any) {
			if im, ok := inner.(*rb.Map); ok {
				im.Each(func(_ string, v any) { qualified = append(qualified, rb.ToS(v)) })
			}
		})
	})
	if s, ok := value.(string); ok && containsStr(qualified, s) {
		return s
	}
	return c.NormalizeLectionaryVariant(value)
}

// PrayerRequestsPlacement ports prayer_requests_placement.
func (c *Capabilities) PrayerRequestsPlacement() *rb.Map {
	if d := c.DailyOffice().Get("prayer_requests_placement"); rb.Present(d) {
		if m, ok := d.(*rb.Map); ok {
			return m
		}
	}
	return rb.M("morning", "collects", "evening", "collects", "midday", "collects", "compline", "collects")
}
