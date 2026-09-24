package dailyoffice

import (
	"context"
	"sort"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// Hooks are the BaseBuilder methods a book overrides. Nil entries use the
// base behavior.
type Hooks struct {
	LoadReadings          func() *reading.Selection
	CollectLanguageStyle  func() string
	ReadingServiceVariant func() string
	ReadingServiceType    func() (string, bool) // (value, handled)
	ReadingService        func() *reading.Resolver
	FetchText             func(slug string) *store.LiturgicalText
	BuildSection          func(name any, slug string, lines []*Line, meta *rb.Map) *Section
	LineItem              func(text any, typ, slug, reference string) *Line
	ReadingFor            func(readingKey string) *reading.Passage
}

// Base ports BaseBuilder + SharedHelpers + LocBase and the concerns every
// book includes (ReadingFormatter, SeasonMapper, TextSections).
type Base struct {
	Ctx        context.Context
	C          *Context
	Date       civil.Date
	OfficeType string
	Prefs      *rb.Map
	DayContext *liturgical.DayContext
	DayInfo    *rb.Map
	Readings   *reading.Selection
	Collects   []*rb.Map
	H          Hooks

	calendar       *liturgical.Calendar
	pbLoaded       bool
	pb             *store.PrayerBook
	texts          map[string]*store.LiturgicalText
	defs           *prefs.DefinitionSet
	readingService *reading.Resolver
	caps           *books.Capabilities
}

// Init ports BaseBuilder#initialize (hooks must be set first).
func (b *Base) Init(ctx context.Context, c *Context) {
	b.Ctx = ctx
	b.C = c
	b.Date = c.Date
	b.OfficeType = c.OfficeType
	b.Prefs = c.Prefs.Dup()
	b.DayContext = c.DayContext
	if b.DayContext == nil {
		b.DayContext = b.Calendar().ContextFor(b.Date)
		c.DayContext = b.DayContext
	}
	b.DayInfo = b.Calendar().DayInfo(b.Date)
	if b.H.LoadReadings != nil {
		b.Readings = b.H.LoadReadings()
	} else {
		b.Readings = b.LoadReadings()
	}
	style := ""
	if b.H.CollectLanguageStyle != nil {
		style = b.H.CollectLanguageStyle()
	}
	b.Collects = collects.New(ctx, b.Date, collects.Options{
		PrayerBookCode: c.Code, Calendar: b.Calendar(), DayContext: b.DayContext,
		OfficeType: b.OfficeType, LanguageStyle: style, IncludeFixedOfficeCollect: true,
	}).FindCollects()
}

// Pref returns preferences[key].
func (b *Base) Pref(key string) any { return b.Prefs.Get(key) }

// PrefS returns preferences[key].to_s.
func (b *Base) PrefS(key string) string { return rb.ToS(b.Prefs.Get(key)) }

// Calendar ports liturgical_calendar.
func (b *Base) Calendar() *liturgical.Calendar {
	if b.calendar == nil {
		b.calendar = liturgical.NewCalendar(b.Date.Year(), b.PrefS("prayer_book_code"))
	}
	return b.calendar
}

// PrayerBook ports prayer_book (find_by_code of the preference).
func (b *Base) PrayerBook() *store.PrayerBook {
	if !b.pbLoaded {
		b.pbLoaded = true
		pb, err := store.PrayerBookByCode(b.Ctx, b.PrefS("prayer_book_code"))
		if err != nil {
			panic(err)
		}
		b.pb = pb
	}
	return b.pb
}

// Caps ports prayer_book_capabilities.
func (b *Base) Caps() *books.Capabilities {
	if b.caps == nil {
		pb := b.PrayerBook()
		if pb == nil {
			rb.RaiseNoMethodOnNil("features")
		}
		b.caps = books.For(pb.Code, pb.Features)
	}
	return b.caps
}

// --- readings ---------------------------------------------------------------------

// LoadReadings ports load_readings.
func (b *Base) LoadReadings() *reading.Selection {
	sel := b.ReadingService().Selection()
	if sel == nil {
		return &reading.Selection{}
	}
	return sel
}

// ReadingService ports reading_service (memoized).
func (b *Base) ReadingService() *reading.Resolver {
	if b.H.ReadingService != nil {
		return b.H.ReadingService()
	}
	if b.readingService == nil {
		b.readingService = b.NewReadingService(b.ReadingServiceType())
	}
	return b.readingService
}

// NewReadingService builds the reading resolver for a service type.
func (b *Base) NewReadingService(serviceType string) *reading.Resolver {
	translation := "nvi"
	if v := b.Pref("bible_version"); v != nil && v != false {
		translation = rb.ToS(v)
	}
	variant := ""
	if b.H.ReadingServiceVariant != nil {
		variant = b.H.ReadingServiceVariant()
	} else {
		variant = b.ReadingServiceVariant()
	}
	return reading.For(b.Ctx, b.Date, reading.Options{
		PrayerBookCode: b.C.Code, Calendar: b.Calendar(), DayContext: b.DayContext, Translation: translation,
		PsalmTranslation: b.SelectedPsalmTranslation(), ReadingType: b.PrefString("reading_type"),
		ServiceType: serviceType, ServiceVariant: variant, PsalmTable: prefs.PsalmCycle(b.Prefs, b.OfficeType),
		LoadContent: true,
	})
}

// PrefString is a preference as a string, "" for nil.
func (b *Base) PrefString(key string) string {
	v := b.Pref(key)
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

// ReadingServiceVariant ports reading_service_variant.
func (b *Base) ReadingServiceVariant() string {
	if len(b.Caps().AvailableLectionaryVariants()) > 0 {
		return b.Caps().LectionaryServiceVariant(b.Prefs)
	}
	key := b.OfficeType + "_reading_variant"
	v := b.Pref(key)
	if rb.Blank(v) {
		v = b.PreferenceDefault(key)
	}
	if rb.Blank(v) {
		return ""
	}
	return rb.ToS(v)
}

// ReadingServiceType ports reading_service_type.
func (b *Base) ReadingServiceType() string {
	if b.H.ReadingServiceType != nil {
		if v, ok := b.H.ReadingServiceType(); ok {
			return v
		}
	}
	switch b.OfficeType {
	case "morning":
		return "morning_prayer"
	case "evening":
		return "evening_prayer"
	}
	return ""
}

// SelectedPsalmTranslation ports selected_psalm_translation.
func (b *Base) SelectedPsalmTranslation() string {
	v := b.Pref("psalm_translation")
	if rb.Blank(v) {
		v = b.PreferenceDefault("psalm_translation")
	}
	if rb.Blank(v) {
		return "bible_version"
	}
	return rb.ToS(v)
}

// PsalmScriptureTranslation ports psalm_scripture_translation.
func (b *Base) PsalmScriptureTranslation() string {
	if reading.IsPsalter(b.SelectedPsalmTranslation()) {
		return b.SelectedPsalmTranslation()
	}
	if v := b.Pref("bible_version"); v != nil && v != false {
		return rb.ToS(v)
	}
	return "nvi"
}

// SelectedOfficeReadings ports selected_office_readings.
func (b *Base) SelectedOfficeReadings() string {
	key := b.OfficeType + "_readings"
	v := b.Pref(key)
	if rb.Blank(v) {
		v = b.PreferenceDefault(key)
	}
	if rb.Blank(v) {
		return OfficeLessonsDefault(b.OfficeType)
	}
	return rb.ToS(v)
}

// OfficeLessons ports office_lessons.
func (b *Base) OfficeLessons() []*reading.Passage {
	return OfficeLessonsFor(b.Readings, b.SelectedOfficeReadings(), b.OfficeType)
}

// OfficeLessonsIn ports office_lessons_in(testament).
func (b *Base) OfficeLessonsIn(testament string) []*reading.Passage {
	return OfficeLessonsInTestament(b.Readings, b.SelectedOfficeReadings(), b.OfficeType, testament)
}

// --- render -------------------------------------------------------------------------

// Render ports render_office(sections).
func (b *Base) Render(sections []*Section) *rb.Map {
	var pbName any
	if pb := b.PrayerBook(); pb != nil {
		pbName = pb.Name
	}
	modules := make([]any, len(sections))
	for i, s := range sections {
		modules[i] = s.ToH()
	}
	return rb.M(
		"date", b.Date.ISO(),
		"office_type", b.OfficeType,
		"modules", modules,
		"season", b.DayInfo.Get("liturgical_season"),
		"color", b.DayInfo.Get("color"),
		"celebration", b.DayInfo.Get("celebration"),
		"saint", b.DayInfo.Get("saint"),
		"metadata", rb.M(
			"prayer_book_code", b.Pref("prayer_book_code"),
			"prayer_book_name", pbName,
			"bible_version", b.Pref("bible_version"),
			"language", b.Pref("language"),
			"seed", b.Pref("seed"),
		),
	)
}

// Step is one pipeline entry: it returns zero or more sections.
type Step func() []*Section

// One adapts a builder method returning one (possibly nil) section.
func One(f func() *Section) Step {
	return func() []*Section {
		if s := f(); s != nil {
			return []*Section{s}
		}
		return nil
	}
}

// Many adapts a builder method returning several sections.
func Many(f func() []*Section) Step { return f }

// Pipeline ports assemble_with_pipeline.
func Pipeline(steps ...Step) []*Section {
	var out []*Section
	for _, s := range steps {
		for _, sec := range s() {
			if sec != nil {
				out = append(out, sec)
			}
		}
	}
	return out
}

// --- lines and sections --------------------------------------------------------------

// Line ports build_line(text:, type:, **metadata).
func (b *Base) Line(text any, typ string, meta *rb.Map) *Line { return NewLine(text, typ, meta) }

// Section ports build_section (through the book's override, if any).
func (b *Base) Section(name any, slug string, lines []*Line, meta *rb.Map) *Section {
	if b.H.BuildSection != nil {
		return b.H.BuildSection(name, slug, lines, meta)
	}
	return NewSection(name, slug, lines, meta)
}

// Item ports line_item(text, type:, slug:, reference:) ("" slug/reference
// are omitted, as blank ones are in Ruby).
func (b *Base) Item(text any, typ string, slug, reference string) *Line {
	if b.H.LineItem != nil {
		return b.H.LineItem(text, typ, slug, reference)
	}
	return BaseItem(text, typ, slug, reference)
}

// BaseItem is the unhooked line_item.
func BaseItem(text any, typ string, slug, reference string) *Line {
	var meta *rb.Map
	if !rb.BlankString(slug) {
		meta = rb.M("slug", slug)
	}
	if !rb.BlankString(reference) {
		if meta == nil {
			meta = rb.NewMap()
		}
		meta.Set("reference", reference)
	}
	return NewLine(text, typ, meta)
}

// I is Item(text, typ) without slug or reference.
func (b *Base) I(text any, typ string) *Line { return b.Item(text, typ, "", "") }

// Spacer is line_item("", type: "spacer").
func (b *Base) Spacer() *Line { return b.Item("", "spacer", "", "") }

// T ports fetch_liturgical_text.
func (b *Base) T(slug string) *store.LiturgicalText {
	if b.H.FetchText != nil {
		return b.H.FetchText(slug)
	}
	return b.BaseText(slug)
}

// BaseText is the unhooked fetch_liturgical_text.
func (b *Base) BaseText(slug string) *store.LiturgicalText {
	if b.PrayerBook() == nil {
		return nil
	}
	return b.Texts()[slug]
}

// Texts ports liturgical_texts_cache.
func (b *Base) Texts() map[string]*store.LiturgicalText {
	if b.texts == nil {
		b.texts = store.LiturgicalTextsFor(b.Ctx, b.PrayerBook())
	}
	return b.texts
}

// TextLine renders a text as a line with its slug (line_item(t.content, type:, slug: t.slug)).
func (b *Base) TextLine(t *store.LiturgicalText, typ string) *Line {
	return b.Item(t.Content, typ, t.Slug, "")
}

// Ref returns a text's reference ("" for nil).
func Ref(t *store.LiturgicalText) string {
	if t == nil || t.Reference == nil {
		return ""
	}
	return *t.Reference
}

// FetchLineItem ports fetch_line_item(slug, type:, substitutions:, attribute:).
func (b *Base) FetchLineItem(slug, typ string, subs [][2]string, attribute string) *Line {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	var content string
	switch attribute {
	case "", "content":
		content = t.Content
	case "title":
		if t.Title != nil {
			content = *t.Title
		}
	case "reference":
		content = Ref(t)
	}
	for _, s := range subs {
		content = strings.ReplaceAll(content, "{{"+s[0]+"}}", s[1])
	}
	return b.Item(content, typ, t.Slug, Ref(t))
}

// TextsMatching ports liturgical_texts_matching(pattern).
func (b *Base) TextsMatching(pattern string) []*store.LiturgicalText {
	prefix := strings.TrimSuffix(pattern, "%")
	var out []*store.LiturgicalText
	for _, t := range b.Texts() {
		if strings.HasPrefix(t.Slug, prefix) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// CollectLines ports build_collect_lines.
func (b *Base) CollectLines(list []*rb.Map) []*Line {
	var lines []*Line
	for _, c := range list {
		if v := c.Get("module_title"); rb.Present(v) {
			lines = append(lines, b.I(v, "heading"))
		}
		if v := c.Get("title"); rb.Present(v) {
			lines = append(lines, b.I(v, "subtitle"))
		}
		if v := c.Get("subtitle"); rb.Present(v) {
			lines = append(lines, b.I(v, "text"))
		}
		if v := c.Get("text"); rb.Present(v) {
			lines = append(lines, b.Item(v, "text", rb.ToS(c.Get("slug")), ""))
		}
		if v := c.Get("preface"); rb.Present(v) {
			lines = append(lines, b.I(v, "citation"))
		}
		lines = append(lines, b.Spacer())
	}
	return lines
}

// CollectOfDaySection ports LocBase#collect_of_day_section.
func (b *Base) CollectOfDaySection(name, rubricSlug string) *Section {
	var lines []*Line
	if r := b.T(rubricSlug); r != nil {
		lines = append(lines, b.I(r.Content, "rubric"))
	}
	lines = append(lines, b.CollectLines(b.Collects)...)
	return b.Section(name, "collect_of_the_day", lines, nil)
}

// --- preferences ---------------------------------------------------------------------

func (b *Base) definitions() *prefs.DefinitionSet {
	if b.defs == nil {
		set, err := prefs.For(b.Ctx, b.PrayerBook())
		if err != nil {
			panic(err)
		}
		b.defs = set
	}
	return b.defs
}

// PreferenceDefault ports preference_default.
func (b *Base) PreferenceDefault(key string) any {
	if b.PrayerBook() == nil {
		return nil
	}
	d := b.definitions().Get(key)
	if d == nil {
		return nil
	}
	return d.TypedDefaultValue()
}

// SeededNumber ports SharedHelpers#seeded_random(lo..hi, key:).
func (b *Base) SeededNumber(lo, hi int, key string) int {
	return b.C.Randomizer.Number(lo, hi, key)
}

// SeededPick ports RiteSelectable#seeded_random / LocBase#seeded_choice: the
// index picked among n values (-1 when empty).
func (b *Base) SeededPick(n int, key string) int { return b.C.Randomizer.PickIndex(n, key) }

// PV is a resolve_preference result: nil, int, string or []any.
type PV = any

func blankPref(v any) bool { return v == nil || strings.TrimSpace(rb.ToS(v)) == "" }

// ResolveRange ports resolve_preference(key, lo..hi).
func (b *Base) ResolveRange(key string, lo, hi int) PV {
	v := b.Pref(key)
	if blankPref(v) {
		def := b.PreferenceDefault(key)
		if blankPref(def) {
			return nil
		}
		v = def
	}
	if list, ok := v.([]any); ok {
		var avail []string
		for n := lo; n <= hi; n++ {
			avail = append(avail, itoa(n))
		}
		return selectAvailable(list, avail)
	}
	switch rb.ToS(v) {
	case "random":
		return b.SeededNumber(lo, hi, key)
	case "all":
		out := []any{}
		for n := lo; n <= hi; n++ {
			out = append(out, n)
		}
		return out
	}
	return rb.StringToI(rb.ToS(v))
}

// ResolveOptions ports resolve_preference(key, %w[...]) for string options.
func (b *Base) ResolveOptions(key string, options []string) PV {
	list := make([]any, len(options))
	for i, o := range options {
		list[i] = o
	}
	return b.ResolveAny(key, list)
}

// ResolveAny ports resolve_preference(key, array) keeping the option types:
// "random"/"all" return options as given, a specific value its to_s.
func (b *Base) ResolveAny(key string, options []any) PV {
	v := b.Pref(key)
	if blankPref(v) {
		def := b.PreferenceDefault(key)
		if blankPref(def) {
			return nil
		}
		v = def
	}
	if list, ok := v.([]any); ok {
		avail := make([]string, len(options))
		for i, o := range options {
			avail[i] = rb.ToS(o)
		}
		return selectAvailable(list, avail)
	}
	switch rb.ToS(v) {
	case "random":
		if len(options) == 0 {
			return nil
		}
		return options[b.SeededNumber(0, len(options)-1, key)]
	case "all":
		return append([]any(nil), options...)
	}
	return rb.ToS(v)
}

func selectAvailable(list []any, avail []string) []any {
	out := []any{}
	seen := map[string]bool{}
	for _, x := range list {
		s := rb.ToS(x)
		if seen[s] {
			continue
		}
		for _, a := range avail {
			if a == s {
				seen[s] = true
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// Arr ports Array(value).
func Arr(v any) []any {
	switch x := v.(type) {
	case nil:
		return []any{}
	case []any:
		return x
	}
	return []any{v}
}

// Or ports Ruby's a || b for resolve_preference results.
func Or(v any, fallback any) any {
	if v == nil || v == false {
		return fallback
	}
	return v
}

// RiteChoice ports RiteSelectable#resolve_rite_preference.
func (b *Base) RiteChoice(key string, rites []string) string {
	p := b.Pref(key)
	if p == nil {
		return rites[0]
	}
	if s, ok := p.(string); ok {
		if s == "" {
			return rites[0]
		}
		if s == "random" {
			if i := b.SeededPick(len(rites), key); i >= 0 {
				return rites[i]
			}
			return ""
		}
		return s
	}
	switch x := p.(type) {
	case []any:
		if len(x) == 0 {
			return rites[0]
		}
	case *rb.Map:
		if x.Len() == 0 {
			return rites[0]
		}
	case bool, int, float64:
		// pref.empty? on a value without #empty?.
		panic(&rb.RubyError{Class: "NoMethodError", Message: "undefined method `empty?' for " + rb.Inspect(p) + ":" + rubyClassName(p)})
	}
	return rb.ToS(p)
}

// rubyClassName names the Ruby class of a scalar preference value.
func rubyClassName(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "TrueClass"
		}
		return "FalseClass"
	case int:
		return "Integer"
	case float64:
		return "Float"
	}
	return "Object"
}

// Season helpers --------------------------------------------------------------------

// Season is day_info[:liturgical_season].
func (b *Base) Season() string { return rb.ToS(b.DayInfo.Get("liturgical_season")) }

// IsLent ports is_lent?.
func (b *Base) IsLent() bool { return LentSeason(b.Season()) }

// LentSeason ports lent_season?.
func LentSeason(season string) bool {
	s := strings.ToLower(season)
	return s == "quaresma" || s == "semana santa"
}

// SeasonToOpeningSentenceSlug ports season_to_opening_sentence_slug.
func SeasonToOpeningSentenceSlug(season string, feastDay bool) string {
	switch strings.ToLower(season) {
	case "advento":
		return "advent"
	case "natal":
		return "christmas"
	case "epifania":
		return "epiphany"
	case "quaresma":
		return "lent"
	case "semana santa":
		return "holy_week"
	case "sexta-feira santa":
		return "good_friday"
	case "páscoa":
		return "easter"
	case "ascensão":
		return "ascension"
	case "santo nome":
		return "holy_name"
	case "pentecostes":
		return "pentecost"
	case "trindade":
		return "trinity"
	case "todos os santos":
		return "all_saints"
	}
	if feastDay {
		return "common_feast"
	}
	return ""
}

// SeasonToAntiphonSlug ports season_to_antiphon_slug.
func SeasonToAntiphonSlug(season string, feastDay bool) string {
	switch strings.ToLower(season) {
	case "advento":
		return "advent"
	case "natal":
		return "christmas"
	case "epifania":
		return "epiphany"
	case "quaresma":
		return "lent"
	case "páscoa":
		return "easter"
	case "ascensão":
		return "ascension"
	case "pentecostes":
		return "pentecost"
	case "trindade":
		return "trinity"
	case "anunciação":
		return "anunciation"
	}
	if feastDay {
		return "common_feast"
	}
	return ""
}

// FeastDay is day_info[:feast_day], a key the day hash never carries.
func (b *Base) FeastDay() bool { return rb.Truthy(b.DayInfo.Get("feast_day")) }

// --- TextSections ----------------------------------------------------------------------

// Entry is one TextSections entry.
type Entry struct {
	Slug    string
	Type    string
	Heading bool
}

// TextSection ports text_section(name:, slug:, entries:, skip_if_empty:).
func (b *Base) TextSection(name any, slug string, entries []Entry, skipIfEmpty bool) *Section {
	var lines []*Line
	for _, e := range entries {
		lines = append(lines, b.TextSectionLines(e)...)
	}
	if skipIfEmpty && len(lines) == 0 {
		return nil
	}
	return b.Section(name, slug, lines, nil)
}

// TextSectionLines ports text_section_lines.
func (b *Base) TextSectionLines(e Entry) []*Line {
	t := b.T(e.Slug)
	if t == nil {
		return nil
	}
	typ := e.Type
	if typ == "" {
		typ = "text"
	}
	var lines []*Line
	if e.Heading {
		var title any
		if t.Title != nil {
			title = *t.Title
		}
		lines = append(lines, b.I(title, "heading"))
	}
	return append(lines, b.I(t.Content, typ))
}

// Title returns a text's title (nil for none).
func Title(t *store.LiturgicalText) any {
	if t == nil || t.Title == nil {
		return nil
	}
	return *t.Title
}

// TitleS returns a text's title or "".
func TitleS(t *store.LiturgicalText) string {
	if t == nil || t.Title == nil {
		return ""
	}
	return *t.Title
}

// NameWithRef ports [title, reference&.then { "(#{ref})" }].compact.join(" ").presence || fallback.
func NameWithRef(t *store.LiturgicalText, fallback string) string {
	var parts []string
	if t.Title != nil {
		parts = append(parts, *t.Title)
	}
	if t.Reference != nil {
		parts = append(parts, "("+*t.Reference+")")
	}
	s := strings.Join(parts, " ")
	if rb.BlankString(s) {
		return fallback
	}
	return s
}

// ReadingFor ports reading_for(reading_key) (a book may route a slot).
func (b *Base) ReadingFor(key string) *reading.Passage {
	if b.H.ReadingFor != nil {
		return b.H.ReadingFor(key)
	}
	return b.Readings.Slot(key)
}
