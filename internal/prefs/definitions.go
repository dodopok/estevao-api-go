// Package prefs ports Preferences::* and PreferenceDefinition: the
// per-Prayer-Book preference catalogue and the resolution of defaults,
// stored values and request overrides.
package prefs

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Definition is a preference_definitions row.
type Definition struct {
	ID                   int64
	CreatedAt, UpdatedAt time.Time
	DefaultValue         *string
	DependsOn            any
	Description          *string
	Key                  string
	Name                 string
	Options              any // decoded jsonb (usually []any of *rb.Map)
	Position             int
	PrefType             string
	PreferenceCategoryID int64
	Required             *bool
	SimpleValue          *string
	ValidationRules      any
}

func (d *Definition) optionList() []*rb.Map {
	arr, _ := d.Options.([]any)
	var out []*rb.Map
	for _, o := range arr {
		if m, ok := o.(*rb.Map); ok {
			out = append(out, m)
		} else {
			out = append(out, nil)
		}
	}
	return out
}

func (d *Definition) isSelect() bool {
	return d.PrefType == "select_one" || d.PrefType == "select_multiple"
}

// CoerceValue ports PreferenceDefinition#coerce_value.
func (d *Definition) CoerceValue(v any) any {
	switch d.PrefType {
	case "select_multiple":
		if arr, ok := v.([]any); ok {
			return arr
		}
		parsed, err := rb.ParseJSON([]byte(rb.ToS(v)))
		if err != nil {
			return []any{}
		}
		if arr, ok := parsed.([]any); ok {
			return arr
		}
		return []any{}
	case "toggle":
		return rb.BoolCast(v)
	case "number":
		return rb.ToI(v)
	}
	return v
}

func strPtrValue(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// TypedDefaultValue ports typed_default_value.
func (d *Definition) TypedDefaultValue() any { return d.CoerceValue(strPtrValue(d.DefaultValue)) }

// TypedSimpleValue ports typed_simple_value.
func (d *Definition) TypedSimpleValue() any {
	if d.SimpleValue != nil && !rb.BlankString(*d.SimpleValue) {
		return d.CoerceValue(*d.SimpleValue)
	}
	return d.TypedDefaultValue()
}

// ValidOptionValues ports valid_option_values.
func (d *Definition) ValidOptionValues() []string {
	if !d.isSelect() {
		return nil
	}
	var out []string
	for _, o := range d.optionList() {
		out = append(out, rb.ToS(o.Get("value")))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// CanonicalValue ports canonical_value (legacy aliases).
func (d *Definition) CanonicalValue(v any) any {
	if d.Key == "office_type" && rb.ToS(v) == "standard" {
		vals := d.ValidOptionValues()
		if contains(vals, "traditional") && !contains(vals, "standard") {
			v = "traditional"
		}
	}
	if !d.isSelect() {
		return v
	}
	for _, o := range d.optionList() {
		if o == nil {
			continue
		}
		var aliases []string
		switch a := o.Get("aliases").(type) {
		case []any:
			for _, x := range a {
				aliases = append(aliases, rb.ToS(x))
			}
		case nil:
		default:
			aliases = append(aliases, rb.ToS(a))
		}
		if contains(aliases, rb.ToS(v)) {
			return o.Get("value")
		}
	}
	return v
}

var (
	timeRe   = regexp.MustCompile(`(?m)^([01]\d|2[0-3]):([0-5]\d)$`)
	digitsRe = regexp.MustCompile(`(?m)^\d+$`)
)

// ValidValue ports valid_value?.
func (d *Definition) ValidValue(v any) bool {
	v = d.CanonicalValue(v)
	switch d.PrefType {
	case "select_one":
		return contains(d.ValidOptionValues(), rb.ToS(v))
	case "select_multiple":
		arr, ok := v.([]any)
		if !ok || len(arr) == 0 {
			return false
		}
		seen := map[string]bool{}
		valid := d.ValidOptionValues()
		for _, item := range arr {
			s := rb.ToS(item)
			if seen[s] || !contains(valid, s) {
				return false
			}
			seen[s] = true
		}
		return true
	case "toggle":
		switch x := v.(type) {
		case bool:
			return true
		case string:
			return x == "true" || x == "false"
		}
		return false
	case "time":
		return v == nil || timeRe.MatchString(rb.ToS(v))
	case "number":
		return v == nil || digitsRe.MatchString(rb.ToS(v))
	}
	return true
}

// AsJSONForAPI ports as_json_for_api.
func (d *Definition) AsJSONForAPI() *rb.Map {
	var required any
	if d.Required != nil {
		required = *d.Required
	}
	m := rb.M(
		"id", d.Key, "key", d.Key, "name", d.Name, "description", d.Description,
		"type", d.PrefType, "required", required, "default_value", d.TypedDefaultValue(),
	)
	if d.isSelect() {
		var defaults []string
		switch dv := d.TypedDefaultValue().(type) {
		case []any:
			for _, x := range dv {
				defaults = append(defaults, rb.ToS(x))
			}
		case nil:
		default:
			defaults = []string{rb.ToS(dv)}
		}
		opts := []any{}
		for _, o := range d.optionList() {
			opts = append(opts, rb.M(
				"value", o.Get("value"), "label", o.Get("label"), "description", o.Get("description"),
				"is_default", contains(defaults, rb.ToS(o.Get("value"))),
			))
		}
		m.Set("options", opts)
	}
	m.Set("simple_value", d.TypedSimpleValue())
	if rb.Present(d.DependsOn) {
		m.Set("depends_on", d.DependsOn)
	}
	return m
}

// SystemKeys ports Preferences::Keys::SYSTEM.
var SystemKeys = []string{
	"notifications", "notifications_enabled", "streak_reminder_enabled",
	"streak_display_enabled", "prayer_times", "version", "prayer_book_code", "language",
	"bible_version", "preferred_audio_voice", "mode", "seed", "reading_type", "confession_type",
	"creed_type", "lords_prayer_version", "family_rite",
}

// LegacyPsalterKeys are the per-office psalter keys psalm_cycle replaced.
var LegacyPsalterKeys = []string{"morning_psalm_cycle", "evening_psalm_cycle"}

var legacyRiteKeys = []string{"morning_prayer_rite", "evening_prayer_rite"}

// DefinitionSet ports Preferences::DefinitionSet.
type DefinitionSet struct {
	Definitions []*Definition
	byKey       map[string]*Definition
	defaults    *rb.Map
}

func newDefinitionSet(defs []*Definition) *DefinitionSet {
	s := &DefinitionSet{Definitions: defs, byKey: map[string]*Definition{}, defaults: rb.NewMap()}
	for _, d := range defs {
		s.defaults.Set(d.Key, d.TypedDefaultValue())
		s.byKey[d.Key] = d
	}
	return s
}

func (s *DefinitionSet) Get(key string) *Definition { return s.byKey[key] }
func (s *DefinitionSet) Has(key string) bool        { _, ok := s.byKey[key]; return ok }
func (s *DefinitionSet) Defaults() *rb.Map          { return s.defaults }

// Keys returns the definition keys in order.
func (s *DefinitionSet) Keys() []string {
	out := make([]string, len(s.Definitions))
	for i, d := range s.Definitions {
		out[i] = d.Key
	}
	return out
}

type setEntry struct {
	version time.Time
	set     *DefinitionSet
}

var (
	setMu    sync.Mutex
	setCache = map[string]setEntry{}
)

// For loads the book's definitions ordered by category and definition position.
func For(ctx context.Context, pb *store.PrayerBook) (*DefinitionSet, error) {
	setMu.Lock()
	if e, ok := setCache[pb.Code]; ok && e.version.Equal(pb.UpdatedAt) {
		setMu.Unlock()
		return e.set, nil
	}
	setMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT pd.id, pd.created_at, pd.default_value, pd.depends_on, pd.description,
		pd.key, pd.name, pd.options, pd.position, pd.pref_type, pd.preference_category_id, pd.required,
		pd.simple_value, pd.updated_at, pd.validation_rules
		FROM preference_definitions pd
		INNER JOIN preference_categories ON preference_categories.id = pd.preference_category_id
		WHERE preference_categories.prayer_book_id = $1
		ORDER BY preference_categories.position ASC, pd.position ASC`, pb.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var defs []*Definition
	for rows.Next() {
		var d Definition
		var dependsOn, options, rules []byte
		if err := rows.Scan(&d.ID, &d.CreatedAt, &d.DefaultValue, &dependsOn, &d.Description, &d.Key, &d.Name,
			&options, &d.Position, &d.PrefType, &d.PreferenceCategoryID, &d.Required, &d.SimpleValue,
			&d.UpdatedAt, &rules); err != nil {
			return nil, err
		}
		d.DependsOn = decodeJSON(dependsOn)
		d.Options = decodeJSON(options)
		d.ValidationRules = decodeJSON(rules)
		defs = append(defs, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	set := newDefinitionSet(defs)
	setMu.Lock()
	setCache[pb.Code] = setEntry{pb.UpdatedAt, set}
	setMu.Unlock()
	return set, nil
}

func decodeJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	v, err := rb.ParseJSON(b)
	if err != nil {
		return nil
	}
	return v
}

// CanonicalizeLegacyKeys ports canonicalize_legacy_keys.
func (s *DefinitionSet) CanonicalizeLegacyKeys(values *rb.Map) *rb.Map {
	n := values.Dup()
	if s.Has("daily_office_rite") {
		rite := firstPresent(n, "daily_office_rite", "morning_prayer_rite", "evening_prayer_rite")
		n = n.Except(legacyRiteKeys...)
		if rb.Present(rite) {
			n.Set("daily_office_rite", rite)
		}
	}
	if s.Has("psalm_cycle") {
		cycle := firstPresent(n, "psalm_cycle", "morning_psalm_cycle", "evening_psalm_cycle")
		n = n.Except(LegacyPsalterKeys...)
		if rb.Present(cycle) {
			n.Set("psalm_cycle", cycle)
		}
	}
	if !s.Has("psalm_translation") {
		n = n.Except("psalm_translation")
	}
	return n
}

func firstPresent(m *rb.Map, keys ...string) any {
	for _, k := range keys {
		if v := m.Get(k); rb.Present(v) {
			return v
		}
	}
	return nil
}

// Normalize ports DefinitionSet#normalize. It raises InvalidPreference
// (via panic) unless ignoreInvalid.
func (s *DefinitionSet) Normalize(values *rb.Map, allowUnknown, ignoreInvalid bool) *rb.Map {
	values = s.CanonicalizeLegacyKeys(values)
	out := rb.NewMap()
	values.Each(func(key string, value any) {
		d := s.Get(key)
		if d == nil {
			if !allowUnknown {
				raiseInvalid(key, "unknown preference")
			}
			out.Set(key, value)
			return
		}
		n := coerce(d, value)
		n = d.CanonicalValue(n)
		if !d.ValidValue(n) {
			if ignoreInvalid {
				return
			}
			raiseInvalid(key, "invalid value")
		}
		out.Set(key, n)
	})
	return out
}

func raiseInvalid(key, reason string) {
	panic(&web.DomainError{Class: "InvalidPreference", Code: "INVALID_PREFERENCE_VALUE", Message: key + ": " + reason,
		Context: map[string]any{"preference_key": key}})
}

func coerce(d *Definition, v any) any {
	switch d.PrefType {
	case "toggle":
		return rb.BoolCast(v)
	case "number":
		if rb.Blank(v) {
			return nil
		}
		if n, ok := rubyInteger(v); ok {
			return n
		}
		return v
	}
	return v
}

// rubyInteger ports Kernel#Integer for the values preferences carry.
func rubyInteger(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	case string:
		s := strings.TrimSpace(x)
		s = strings.ReplaceAll(s, "_", "")
		neg := false
		if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
			neg = s[0] == '-'
			s = s[1:]
		}
		base := 10
		switch {
		case strings.HasPrefix(strings.ToLower(s), "0x"):
			base, s = 16, s[2:]
		case strings.HasPrefix(strings.ToLower(s), "0b"):
			base, s = 2, s[2:]
		case strings.HasPrefix(strings.ToLower(s), "0o"):
			base, s = 8, s[2:]
		case len(s) > 1 && s[0] == '0':
			base, s = 8, s[1:]
		}
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil || s == "" {
			return 0, false
		}
		if neg {
			n = -n
		}
		return int(n), true
	}
	return 0, false
}

// ensure json import is used for future helpers.
var _ = json.Valid
