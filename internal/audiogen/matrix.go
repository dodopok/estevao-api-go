package audiogen

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// familySourcePatterns is PreferenceVariants::FAMILY_SOURCE_PATTERNS.
var familySourcePatterns = map[string]*regexp.Regexp{
	"morning":      regexp.MustCompile(`^family_(?:locb_)?morning_(?:psalm|reading)$`),
	"midday":       regexp.MustCompile(`^family_(?:locb_)?midday_(?:psalm|reading)$`),
	"evening":      regexp.MustCompile(`^family_(?:(?:locb_)?evening|early_evening)_(?:psalm|reading)$`),
	"late_evening": regexp.MustCompile(`^family_late_evening_(?:psalm|reading)$`),
	"compline":     regexp.MustCompile(`^family_(?:(?:locb_)?compline|end_of_day)_(?:psalm|reading)$`),
}

// sourceKeyPatterns is PreferenceVariants::SOURCE_KEY_PATTERNS.
var sourceKeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^bible_version$`),
	regexp.MustCompile(`^reading_type$`),
	regexp.MustCompile(`^lectionary_variant$`),
	regexp.MustCompile(`^(?:.+_)?reading_variant$`),
	regexp.MustCompile(`^ember_readings$`),
	regexp.MustCompile(`^psalm_translation$`),
	regexp.MustCompile(`^(?:.+_)?psalm_cycle$`),
	regexp.MustCompile(`^weekday_psalm_table$`),
}

func sourcePreferenceKey(key string) bool {
	for _, re := range sourceKeyPatterns {
		if re.MatchString(key) {
			return true
		}
	}
	return false
}

// findBook ports PrayerBook.find_by_code!(code).
func findBook(ctx context.Context, code string) (*store.PrayerBook, error) {
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, &rb.RubyError{Class: "ActiveRecord::RecordNotFound", Message: "Couldn't find PrayerBook with code=" + code}
	}
	return pb, nil
}

type dimension struct {
	key          string
	alternatives []string
}

// variants ports Audio::PreferenceVariants.
type variants struct {
	pb            *store.PrayerBook
	base          *rb.Map
	defs          *prefs.DefinitionSet
	caps          *books.Capabilities
	officeType    string
	inferredBible bool
	defaulted     []string
}

// PreferenceVariants ports PreferenceVariants.for(prayer_book_code:,
// preferences:, office_type:): the preference configurations that change
// the day's spoken Bible or psalter content.
func PreferenceVariants(ctx context.Context, code string, preferences *rb.Map, officeType string) ([]*rb.Map, error) {
	pb, err := findBook(ctx, code)
	if err != nil {
		return nil, err
	}
	defs, err := prefs.For(ctx, pb)
	if err != nil {
		return nil, err
	}
	v := &variants{pb: pb, base: preferences.Dup(), defs: defs, caps: books.For(pb.Code, pb.Features), officeType: officeType}
	if err := v.withBibleDefault(ctx); err != nil {
		return nil, err
	}
	v.withSourceDefaults()
	return v.call(ctx)
}

func (v *variants) call(ctx context.Context) ([]*rb.Map, error) {
	configurations := []*rb.Map{v.base}
	dims, err := v.dimensions(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range dims {
		if v.base.Has(d.key) && !v.defaultedSource(d.key) {
			continue
		}
		var next []*rb.Map
		for _, c := range configurations {
			next = append(next, c)
			for _, alt := range d.alternatives {
				m := c.Dup()
				m.Set(d.key, alt)
				next = append(next, m)
			}
		}
		configurations = next
	}
	return uniqMaps(v.addFamilyRiteVariants(configurations)), nil
}

func (v *variants) dimensions(ctx context.Context) ([]dimension, error) {
	var out []dimension
	for _, d := range v.defs.Definitions {
		if dim := v.dimensionFor(d, true); dim != nil {
			out = append(out, *dim)
		}
	}
	if !v.defs.Has("bible_version") {
		dim, err := v.bibleVersionDimension(ctx)
		if err != nil {
			return nil, err
		}
		if dim != nil {
			out = append(out, *dim)
		}
	}
	if !v.defs.Has("reading_type") {
		if dim := v.readingTypeDimension(); dim != nil {
			out = append(out, *dim)
		}
	}
	if v.familyRite() {
		if pattern := familySourcePatterns[v.officeType]; pattern != nil {
			for _, d := range v.defs.Definitions {
				if pattern.MatchString(d.Key) {
					if dim := v.dimensionFor(d, false); dim != nil {
						out = append(out, *dim)
					}
				}
			}
		}
	}
	return out, nil
}

func uniqStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if !containsStr(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// dimensionFor ports dimension_for_definition (include_all_bible_versions
// is false for every caller here).
func (v *variants) dimensionFor(d *prefs.Definition, sourceOnly bool) *dimension {
	if sourceOnly && !sourcePreferenceKey(d.Key) {
		return nil
	}
	if d.Key == "bible_version" || d.PrefType != "select_one" {
		return nil
	}
	def := d.TypedDefaultValue()
	var alternatives []string
	for _, value := range uniqStrings(d.ValidOptionValues()) {
		if rb.Present(def) && value == rb.ToS(def) {
			continue
		}
		alternatives = append(alternatives, value)
	}
	if len(alternatives) == 0 {
		return nil
	}
	return &dimension{d.Key, alternatives}
}

func (v *variants) defaultReadingType(values []string) string {
	if def := v.caps.DefaultReadingType(); def != "" {
		return def
	}
	if len(values) > 0 {
		return values[0]
	}
	return "semicontinuous"
}

func (v *variants) readingTypeDimension() *dimension {
	values := uniqStrings(v.caps.AvailableReadingTypes())
	def := v.defaultReadingType(values)
	var alternatives []string
	for _, value := range values {
		if value != def {
			alternatives = append(alternatives, value)
		}
	}
	if len(alternatives) == 0 {
		return nil
	}
	return &dimension{"reading_type", alternatives}
}

func (v *variants) bibleVersionDimension(ctx context.Context) (*dimension, error) {
	recommended, err := defaultBibleCode(ctx, v.pb.Language)
	if err != nil || recommended == "" || recommended == "bible_version" {
		return nil, err
	}
	ok, err := corpus(ctx, recommended)
	if err != nil || !ok {
		return nil, err
	}
	if rb.ToS(v.base.Get("bible_version")) == recommended {
		return nil, nil
	}
	return &dimension{"bible_version", []string{recommended}}, nil
}

func defaultBibleCode(ctx context.Context, language string) (string, error) {
	bv, err := store.DefaultBibleVersionFor(ctx, language)
	if err != nil || bv == nil {
		return "", err
	}
	return bv.Code, nil
}

// withBibleDefault ports with_bible_default.
func (v *variants) withBibleDefault(ctx context.Context) error {
	if v.base.Has("bible_version") || v.defs.Has("bible_version") {
		return nil
	}
	def, err := defaultBibleCode(ctx, v.pb.Language)
	if err != nil || rb.BlankString(def) || def == "bible_version" {
		return err
	}
	v.inferredBible = true
	v.base.Set("bible_version", def)
	return nil
}

// withSourceDefaults ports with_source_defaults: the source defaults
// materialized ahead of the given values, remembering which were synthesized.
func (v *variants) withSourceDefaults() {
	defaults := rb.NewMap()
	family := v.familyRite()
	pattern := familySourcePatterns[v.officeType]
	for _, d := range v.defs.Definitions {
		if !sourcePreferenceKey(d.Key) && !(family && pattern != nil && pattern.MatchString(d.Key)) {
			continue
		}
		if d.PrefType != "select_one" {
			continue
		}
		if def := d.TypedDefaultValue(); rb.Present(def) {
			defaults.Set(d.Key, def)
		}
	}
	if !v.defs.Has("reading_type") {
		defaults.Set("reading_type", v.defaultReadingType(v.caps.AvailableReadingTypes()))
	}
	defaults.Each(func(k string, _ any) {
		if !v.base.Has(k) {
			v.defaulted = append(v.defaulted, k)
		}
	})
	v.base.Each(func(k string, val any) { defaults.Set(k, val) })
	v.base = defaults
}

func (v *variants) defaultedSource(key string) bool {
	return containsStr(v.defaulted, key) || v.inferredBible && key == "bible_version"
}

func (v *variants) addFamilyRiteVariants(configurations []*rb.Map) []*rb.Map {
	if !v.caps.SupportsFamilyRite() || v.base.Has("family_rite") || rb.ToS(v.base.Get("office_type")) == "family" {
		return configurations
	}
	out := append([]*rb.Map{}, configurations...)
	for _, c := range configurations {
		m := c.Dup()
		m.Set("family_rite", true)
		out = append(out, m)
	}
	return out
}

func (v *variants) familyRite() bool {
	return v.officeType != "" && (v.base.Get("family_rite") == true || rb.ToS(v.base.Get("office_type")) == "family")
}

// hashKey is Hash#hash for dedup: key order does not matter, value types do.
func hashKey(m *rb.Map) string {
	keys := m.Keys()
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(rb.Inspect(k))
		b.WriteString("=>")
		v := m.Get(k)
		if sub, ok := v.(*rb.Map); ok {
			b.WriteString("{" + hashKey(sub) + "}")
		} else {
			b.WriteString(rb.Inspect(v))
		}
		b.WriteString(",")
	}
	return b.String()
}

// uniqMaps ports Array#uniq over hashes.
func uniqMaps(in []*rb.Map) []*rb.Map {
	seen := map[string]bool{}
	var out []*rb.Map
	for _, m := range in {
		k := hashKey(m)
		if !seen[k] {
			seen[k] = true
			out = append(out, m)
		}
	}
	return out
}

// --- Audio::CoverageMatrix ------------------------------------------------------------------

// OfficeConfigurations is one office type and the configurations to visit.
type OfficeConfigurations struct {
	OfficeType     string
	Configurations []*rb.Map
}

// CoverageMatrix ports Audio::CoverageMatrix.
type CoverageMatrix struct {
	pb          *store.PrayerBook
	caps        *books.Capabilities
	requested   []string
	preferences *rb.Map
	variants    []*rb.Map
	offices     []string
	byOffice    []OfficeConfigurations
}

// NewCoverageMatrix ports CoverageMatrix.new(prayer_book_code:, offices:,
// preferences:, variants:). variants is nil when none were given.
func NewCoverageMatrix(ctx context.Context, code string, offices []string, preferences *rb.Map, variantList []*rb.Map) (*CoverageMatrix, error) {
	pb, err := findBook(ctx, code)
	if err != nil {
		return nil, err
	}
	if preferences == nil {
		preferences = rb.NewMap()
	}
	return &CoverageMatrix{pb: pb, caps: books.For(pb.Code, pb.Features), requested: uniqStrings(offices),
		preferences: preferences, variants: variantList}, nil
}

func familyRiteExplicit(m *rb.Map) bool {
	return m.Get("family_rite") == true || rb.ToS(m.Get("office_type")) == "family"
}

func (m *CoverageMatrix) familyOffices() []string {
	if !m.caps.SupportsFamilyRite() {
		return nil
	}
	return m.caps.AvailableOffices(true)
}

// OfficeTypes ports office_types.
func (m *CoverageMatrix) OfficeTypes() ([]string, error) {
	if m.offices != nil {
		return m.offices, nil
	}
	var available []string
	if familyRiteExplicit(m.preferences) {
		available = m.familyOffices()
	} else {
		available = uniqStrings(append(append([]string{}, m.caps.AvailableOffices(false)...), m.familyOffices()...))
	}
	selected := available
	if len(m.requested) > 0 {
		selected = m.requested
	}
	var invalid []string
	for _, o := range selected {
		if !containsStr(available, o) {
			invalid = append(invalid, o)
		}
	}
	if len(invalid) > 0 {
		return nil, &rb.RubyError{Class: "ArgumentError", Message: "unsupported offices: " + strings.Join(invalid, ", ")}
	}
	if selected == nil {
		selected = []string{}
	}
	m.offices = selected
	return selected, nil
}

// ConfigurationsByOffice ports configurations_by_office.
func (m *CoverageMatrix) ConfigurationsByOffice(ctx context.Context) ([]OfficeConfigurations, error) {
	if m.byOffice != nil {
		return m.byOffice, nil
	}
	offices, err := m.OfficeTypes()
	if err != nil {
		return nil, err
	}
	out := []OfficeConfigurations{}
	for _, o := range offices {
		configs, err := m.configurationsFor(ctx, o)
		if err != nil {
			return nil, err
		}
		out = append(out, OfficeConfigurations{o, configs})
	}
	m.byOffice = out
	return out, nil
}

// VariantCount ports variant_count.
func (m *CoverageMatrix) VariantCount(ctx context.Context) (int, error) {
	by, err := m.ConfigurationsByOffice(ctx)
	n := 0
	for _, o := range by {
		n += len(o.Configurations)
	}
	return n, err
}

func (m *CoverageMatrix) configurationsFor(ctx context.Context, officeType string) ([]*rb.Map, error) {
	var configurations []*rb.Map
	switch {
	case len(m.variants) > 0:
		for _, v := range m.variants {
			c := m.preferences.Dup()
			v.Each(func(k string, val any) { c.Set(k, val) })
			configurations = append(configurations, c)
		}
		configurations = uniqMaps(configurations)
	case familyRiteExplicit(m.preferences):
		list, err := PreferenceVariants(ctx, m.pb.Code, m.preferences, officeType)
		if err != nil {
			return nil, err
		}
		configurations = list
	default:
		standard := m.preferences.Dup()
		standard.Set("family_rite", false)
		list, err := PreferenceVariants(ctx, m.pb.Code, standard, officeType)
		if err != nil {
			return nil, err
		}
		configurations = list
		if m.caps.SupportsFamilyRite() {
			family := m.preferences.Dup()
			family.Set("family_rite", true)
			more, err := PreferenceVariants(ctx, m.pb.Code, family, officeType)
			if err != nil {
				return nil, err
			}
			configurations = append(configurations, more...)
		}
	}
	out := []*rb.Map{}
	for _, c := range configurations {
		if containsStr(m.caps.AvailableOffices(familyRiteExplicit(c)), officeType) {
			out = append(out, c)
		}
	}
	return out, nil
}
