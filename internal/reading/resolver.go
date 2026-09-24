package reading

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/books"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// movableRulesWithReadings ports MOVABLE_RULES_WITH_READINGS.
var movableRulesWithReadings = []string{
	"easter_minus_46_days", "easter_minus_6_days", "easter_minus_5_days", "easter_minus_4_days",
	"easter_minus_3_days", "easter_minus_2_days", "easter_minus_1_days",
}

// Options mirrors the keyword arguments of ReadingService.for. Strings use
// "" for nil; Translation must be set explicitly ("nvi" is the Ruby default
// when the caller omits it).
type Options struct {
	PrayerBookCode string
	Calendar       *liturgical.Calendar
	DayContext     *liturgical.DayContext
	Translation    string
	ReadingType    string
	// ReadingTypeRaw carries a preference value that is not a string (a
	// list or a hash from the preferences JSON). Rails passes it through to
	// Reading::Query, whose connection.quote raises TypeError on it.
	ReadingTypeRaw   any
	ServiceType      string
	ServiceVariant   string
	PsalmTable       string
	PsalmTranslation string
	LoadContent      bool
	Cache            *bible.SegmentCache
}

// Outcome ports Reading::PrecedenceResolver::Outcome.
type Outcome struct {
	Rule     string
	Source   string
	Attempts []Attempt
}

type rule struct {
	code, source string
	applicable   func() bool
	resolve      func() *Record
}

type bookKind int

const (
	kindDefault bookKind = iota
	kind2015
	kind2027
	kind1928
	kind1662
	kind1962
	kindWales
)

var registry = map[string]bookKind{
	"loc_1662_en": kind1662, "loc_1962_en": kind1962, "loc_1928_en": kind1928, "loc_1928_pt": kind1928,
	"loc_1984_en": kindWales, "loc_1984_cy": kindWales, "loc_2015": kind2015, "loc_2027": kind2027,
}

// Resolver ports Reading::Resolver and its per-book subclasses.
type Resolver struct {
	ctx              context.Context
	Date             Date
	Code             string
	kind             bookKind
	rules            *liturgical.RuleSet
	cal              *liturgical.Calendar
	dayCtx           *liturgical.DayContext
	Cycle            string
	Translation      string
	ReadingType      string
	readingTypeRaw   any
	ServiceType      string
	ServiceVariant   string
	PsalmTable       string
	PsalmTranslation string
	LoadContent      bool
	cache            *bible.SegmentCache

	pbLoaded bool
	pb       *store.PrayerBook

	outcome            *Outcome
	provenanceFallback bool
	slotOrder          []string
	slots              map[string]SlotProvenance
	referenceCycle     string

	query      *Query
	refBuilder ReferenceBuilder
	selBuilder *SelectionBuilder
}

// For ports ReadingService.for / Reading::Registry.build.
func For(ctx context.Context, date Date, o Options) *Resolver {
	code := o.PrayerBookCode
	if code == "" {
		code = books.DefaultCode
	}
	kind := registry[code]
	r := &Resolver{ctx: ctx, Date: date, Code: code, kind: kind, cache: o.Cache}
	if r.cache == nil {
		r.cache = bible.NewSegmentCache()
	}
	r.rules = liturgical.RulesFor(code)
	if o.Calendar != nil {
		r.cal = o.Calendar.ForDate(date)
	} else {
		r.cal = liturgical.NewCalendar(date.Year(), code)
	}
	r.dayCtx = o.DayContext
	if r.dayCtx == nil {
		r.dayCtx = r.cal.ContextFor(date)
	}
	r.Translation = o.Translation
	readingType := o.ReadingType
	if kind == kind2027 && readingType == "" {
		readingType = "complementary"
	}
	r.ReadingType = r.rules.NormalizeReadingType(readingType)
	if !r.rules.WeekdayTableLectionary() {
		r.readingTypeRaw = o.ReadingTypeRaw
	}
	r.ServiceType = o.ServiceType
	if rb.BlankString(o.ServiceVariant) {
		r.ServiceVariant = r.defaultServiceVariant()
	} else {
		r.ServiceVariant = o.ServiceVariant
	}
	r.PsalmTable = o.PsalmTable
	if rb.BlankString(r.PsalmTable) {
		r.PsalmTable = "appointed"
	}
	r.PsalmTranslation = o.PsalmTranslation
	if rb.BlankString(r.PsalmTranslation) {
		r.PsalmTranslation = ""
	}
	r.LoadContent = o.LoadContent
	r.Cycle = r.determineCycle()
	if kind == kind1928 {
		r.init1928()
	}
	return r
}

func (r *Resolver) prayerBook() *store.PrayerBook {
	if !r.pbLoaded {
		r.pbLoaded = true
		pb, err := store.PrayerBookByCode(r.ctx, r.Code)
		if err != nil {
			panic(err)
		}
		r.pb = pb
	}
	return r.pb
}

func (r *Resolver) prayerBookID() *int64 {
	if pb := r.prayerBook(); pb != nil {
		id := pb.ID
		return &id
	}
	return nil
}

func (r *Resolver) capabilities() *books.Capabilities {
	pb := r.prayerBook()
	return books.For(pb.Code, pb.Features)
}

func (r *Resolver) defaultServiceVariant() string {
	switch r.kind {
	case kind1928:
		if v := r.capabilities().DefaultLectionaryVariant(); v != "" {
			return v
		}
		return map[string]string{"loc_1928_en": "original_1928", "loc_1928_pt": "revised_1945"}[r.Code]
	case kind1662, kind1962:
		first := "original_1662"
		if r.kind == kind1962 {
			first = "canada_1962"
		}
		if r.prayerBook() == nil {
			return first
		}
		if v := r.capabilities().DefaultLectionaryVariant(); v != "" {
			return v
		}
		return first
	}
	return r.rules.DefaultServiceVariant(r.ServiceType)
}

func (r *Resolver) strictServiceVariant() bool {
	return r.kind == kind1928 || r.kind == kind1662 || r.kind == kind1962
}

// --- Loc1928Service ----------------------------------------------------------

func (r *Resolver) init1928() {
	v := r.ServiceVariant
	if strings.HasPrefix(v, "alternative_") || strings.HasPrefix(v, "special_occasion_") || strings.HasPrefix(v, "optional_autumn_ember") {
		v = r.defaultServiceVariant() + ":" + v
	}
	r.ServiceVariant = v
	variant := v
	if v != "original_1928:optional_ember" {
		variant = strings.SplitN(v, ":", 2)[0]
	}
	r.rules = r.rules.ForLectionaryVariant(variant)
	if r.ServiceVariant == "original_1928:optional_ember" && !r.optionalEmberDay() {
		r.ServiceVariant = "original_1928"
		r.rules = r.rules.ForLectionaryVariant(r.ServiceVariant)
	}
}

func (r *Resolver) optionalEmberDay() bool {
	if r.rules.EmberReadingReference(r.Date, r.cal.Easter.Dates) == "" {
		return false
	}
	if r.fixedHolyDay(r.Date) {
		return false
	}
	if r.ServiceType == "evening_prayer" && r.fixedHolyDay(r.Date.Add(1)) {
		return false
	}
	return true
}

func (r *Resolver) fixedHolyDay(d Date) bool {
	return len(r.rules.FixedDateReadingReferences(d.Month(), d.Day())) > 0
}

// --- cycle -------------------------------------------------------------------

func (r *Resolver) determineCycle() string {
	rules := liturgical.RulesFor(r.Code)
	if c := rules.ReadingCycle(); c != "" {
		return c
	}
	office := r.ServiceType == "morning_prayer" || r.ServiceType == "evening_prayer"
	if anchor := rules.AlternatingOfficeSeriesAnchorYear(); anchor != 0 && office {
		seriesAAtMorning := (r.Date.Year()-anchor)%2 == 0
		morning := r.ServiceType == "morning_prayer"
		if seriesAAtMorning == morning {
			return "A"
		}
		return "B"
	}
	d := r.Date
	if rules.BiennialCycleFor(r.ServiceType, &d) {
		if r.liturgicalYear()%2 != 0 {
			return "odd"
		}
		return "even"
	}
	if rules.WeekdayCycleScheme() == "paired_weekday_year" && r.Date.Weekday() != 0 {
		daily := "1"
		if r.liturgicalYear()%2 == 0 {
			daily = "2"
		}
		return r.cal.LiturgicalYearCycle(r.Date) + "-" + daily
	}
	return r.cal.LiturgicalYearCycle(r.Date)
}

func (r *Resolver) liturgicalYear() int {
	advent := r.cal.Easter.Dates["first_sunday_of_advent"]
	if r.Date >= advent {
		return r.Date.Year() + 1
	}
	return r.Date.Year()
}

// --- collaborators -----------------------------------------------------------

func (r *Resolver) Query() *Query {
	if r.query == nil {
		r.query = &Query{
			PrayerBookID: r.prayerBookID(), Cycle: r.Cycle, Date: r.Date, ReadingType: r.ReadingType, ReadingTypeRaw: r.readingTypeRaw,
			ServiceType: r.ServiceType, ServiceVariant: r.ServiceVariant, StrictServiceVariant: r.strictServiceVariant(),
		}
	}
	return r.query
}

func (r *Resolver) builder() ReferenceBuilder {
	if r.refBuilder == nil {
		switch r.kind {
		case kind1928:
			r.refBuilder = newBaseBuilder(r.Date, r.cal, r.ServiceVariant, r.rules)
		case kind1662:
			r.refBuilder = &loc1662Builder{newBaseBuilder(r.Date, r.cal, r.ServiceVariant, nil)}
		case kind1962:
			r.refBuilder = &loc1962Builder{newBaseBuilder(r.Date, r.cal, r.ServiceVariant, nil)}
		default:
			r.refBuilder = newBaseBuilder(r.Date, r.cal, r.ServiceVariant, nil)
		}
	}
	return r.refBuilder
}

// SelectionBuilder returns the typed-selection builder.
func (r *Resolver) SelectionBuilder() *SelectionBuilder {
	if r.selBuilder == nil {
		lang := "pt-BR"
		if pb := r.prayerBook(); pb != nil && pb.Language != "" {
			lang = pb.Language
		}
		r.selBuilder = &SelectionBuilder{
			Translation: r.Translation, PsalmTranslation: r.PsalmTranslation, Language: lang,
			CumulativePsalmLists: r.rules.CumulativePsalmLists(), LoadContent: r.LoadContent, Cache: r.cache,
		}
	}
	return r.selBuilder
}

func (r *Resolver) officeReadingRequested() bool {
	return r.ServiceType == "morning_prayer" || r.ServiceType == "evening_prayer"
}

// --- public API ----------------------------------------------------------------

// Selection ports #selection (Rails caches it; the value is deterministic).
func (r *Resolver) Selection() *Selection {
	return r.findReadingsUncached()
}

// FindReadings ports #find_readings.
func (r *Resolver) FindReadings() any { return SelectionToH(r.Selection()) }

// Resolve ports #resolve.
func (r *Resolver) Resolve() *Resolution {
	r.outcome = nil
	r.provenanceFallback = false
	r.slotOrder = nil
	r.slots = nil
	sel := r.findReadingsUncached()
	o := r.outcome
	p := &Provenance{
		Source: "fallback", Rule: "no_reading", Cycle: r.provenanceCycle(),
		Track: optional(r.ReadingType), ServiceType: optional(r.ServiceType), ServiceVariant: optional(r.ServiceVariant),
		Attempts: []Attempt{}, SlotOrder: r.slotOrder, Slots: r.slots,
	}
	if o != nil {
		p.Source, p.Rule, p.Attempts = o.Source, o.Rule, o.Attempts
		if p.Attempts == nil {
			p.Attempts = []Attempt{}
		}
	}
	if r.officeReadingRequested() {
		p.PsalmTable = optional(r.PsalmTable)
	}
	p.Fallback = r.provenanceFallback || o == nil || o.Source == "fallback"
	return &Resolution{Selection: sel, Provenance: p}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Resolver) provenanceCycle() string {
	if r.kind == kind2027 && r.referenceCycle != "" {
		return r.referenceCycle
	}
	return r.Cycle
}

// DailyCourseSelection ports find_daily_course_readings.
func (r *Resolver) DailyCourseSelection() *Selection {
	var rec *Record
	if r.Date.Weekday() == 0 {
		rec = r.findByProper()
		if rec == nil {
			rec = r.findBySunday()
		}
		if rec == nil {
			rec = r.findByFixedDate()
		}
	} else {
		rec = r.Query().FindWeeklyByReferences(r.ctx, r.weeklyReferencesByPrecedence())
		if rec == nil {
			rec = r.findByFixedDate()
		}
	}
	rec = r.applyFixedHolyDaySubstitution(rec)
	return r.format(r.supplementPsalmFromDailyCycle(rec))
}

func (r *Resolver) format(rec *Record) *Selection {
	return r.SelectionBuilder().Call(r.ctx, rec)
}

// --- resolution ---------------------------------------------------------------

func (r *Resolver) findReadingsUncached() *Selection {
	if r.kind == kindWales {
		return r.walesFindReadings()
	}
	return r.baseFindReadings()
}

func (r *Resolver) baseFindReadings() *Selection {
	rec := r.findVigilReading()
	if rec == nil {
		if r.Date.Weekday() == 0 {
			rec = r.replaceEucharistWithDailyCourse(r.findSundayReadingsByPrecedence())
		} else {
			rec = r.replaceEucharistWithDailyCourse(r.findWeekdayReadingsByPrecedence())
		}
	}
	rec = r.applyFixedHolyDaySubstitution(rec)
	rec = r.supplementPsalmFromDailyCycle(rec)
	if lessonsMissing(rec) && r.officeOnlyDefaultRequest() {
		if fallback := r.officeFallbackReadings(); fallback != nil {
			var attempts []Attempt
			if r.outcome != nil {
				attempts = r.outcome.Attempts
			}
			attempts = append(attempts, Attempt{Rule: "office_fallback", Status: "selected"})
			r.outcome = &Outcome{Rule: "office_fallback", Source: "fallback", Attempts: attempts}
			return fallback
		}
		if rec == nil {
			return nil
		}
	}
	return r.format(rec)
}

func lessonsMissing(rec *Record) bool {
	if rec == nil {
		return true
	}
	return !present(rec.FirstReading) && !present(rec.SecondReading) && !present(rec.Gospel)
}

func (r *Resolver) officeOnlyDefaultRequest() bool {
	return r.ServiceType == "" && r.rules.OfficeLessonsWhenUnappointed()
}

func (r *Resolver) officeFallbackReadings() *Selection {
	for _, office := range []string{"morning_prayer", "evening_prayer"} {
		sel := For(r.ctx, r.Date, Options{
			PrayerBookCode: r.Code, Calendar: r.cal, Translation: r.Translation, PsalmTranslation: r.PsalmTranslation,
			ReadingType: r.ReadingType, ReadingTypeRaw: r.readingTypeRaw, ServiceType: office, ServiceVariant: r.ServiceVariant, LoadContent: true, Cache: r.cache,
		}).Selection()
		if sel == nil {
			continue
		}
		for _, p := range []*Passage{sel.FirstReading, sel.SecondReading, sel.Gospel} {
			if p != nil && !rb.BlankString(p.Reference) {
				return sel
			}
		}
	}
	return nil
}

func (r *Resolver) selectServiceVariant(b *sqlBuilder) *Record {
	byID := func(c *sqlBuilder) *Record {
		c.order = append(c.order, "lectionary_readings.id ASC")
		return c.take(r.ctx)
	}
	if r.strictServiceVariant() {
		c := b.clone()
		c.eq("service_variant", nilIfEmpty(r.ServiceVariant))
		return byID(c)
	}
	if r.ServiceVariant != "" {
		c := b.clone()
		c.eq("service_variant", r.ServiceVariant)
		if rec := byID(c); rec != nil {
			return rec
		}
		c = b.clone()
		c.eq("service_variant", nil)
		return byID(c)
	}
	return byID(b.clone())
}

func (r *Resolver) pbCondition(b *sqlBuilder) {
	if id := r.prayerBookID(); id != nil {
		b.eq("prayer_book_id", *id)
	} else {
		b.eq("prayer_book_id", nil)
	}
}

func (r *Resolver) findVigilReading() *Record {
	if !r.rules.VigilReadings() || r.ServiceType != "evening_prayer" {
		return nil
	}
	for _, ref := range r.rules.VigilReadingReferences(r.Date, r.cal.Easter.Dates) {
		b := &sqlBuilder{}
		r.pbCondition(b)
		b.eq("date_reference", ref)
		b.eq("cycle", "all")
		b.eq("service_type", "vigil")
		// .order(:id) is already present; .first keeps it.
		b.order = append(b.order, "lectionary_readings.id ASC")
		if rec := r.selectServiceVariantOrdered(b); rec != nil {
			r.outcome = &Outcome{Rule: "vigil", Source: "official_reference", Attempts: []Attempt{{Rule: "vigil", Status: "selected"}}}
			return rec
		}
	}
	tomorrow := r.Date.Add(1)
	c := store.FixedCelebrationOn(r.ctx, r.prayerBookID(), tomorrow.Month(), tomorrow.Day(), true)
	if c == nil {
		return nil
	}
	b := &sqlBuilder{}
	r.pbCondition(b)
	b.eq("celebration_id", c.ID)
	b.eq("cycle", "all")
	b.eq("service_type", "vigil")
	rec := r.selectServiceVariant(b)
	if rec == nil {
		return nil
	}
	r.outcome = &Outcome{Rule: "vigil", Source: "celebration", Attempts: []Attempt{{Rule: "vigil", Status: "selected"}}}
	return rec
}

// selectServiceVariantOrdered is select_service_variant on a relation that
// already orders by id (so .first adds nothing).
func (r *Resolver) selectServiceVariantOrdered(b *sqlBuilder) *Record {
	if r.strictServiceVariant() {
		c := b.clone()
		c.eq("service_variant", nilIfEmpty(r.ServiceVariant))
		return c.take(r.ctx)
	}
	if r.ServiceVariant != "" {
		c := b.clone()
		c.eq("service_variant", r.ServiceVariant)
		if rec := c.take(r.ctx); rec != nil {
			return rec
		}
		c = b.clone()
		c.eq("service_variant", nil)
		return c.take(r.ctx)
	}
	return b.take(r.ctx)
}

func (r *Resolver) primaryCelebration() *liturgical.CelebrationAttrs {
	if r.dayCtx.Primary == nil {
		return nil
	}
	return r.dayCtx.Primary.Attrs
}

func (r *Resolver) resolvePrecedence(rules []rule) *Record {
	var attempts []Attempt
	for _, ru := range rules {
		if ru.applicable != nil && !ru.applicable() {
			attempts = append(attempts, Attempt{Rule: ru.code, Status: "not_applicable"})
			continue
		}
		if rec := ru.resolve(); rec != nil {
			attempts = append(attempts, Attempt{Rule: ru.code, Status: "selected"})
			r.outcome = &Outcome{Rule: ru.code, Source: ru.source, Attempts: attempts}
			return rec
		}
		attempts = append(attempts, Attempt{Rule: ru.code, Status: "not_found"})
	}
	r.outcome = &Outcome{Rule: "no_reading", Source: "fallback", Attempts: attempts}
	return nil
}

func (r *Resolver) findWeekdayReadingsByPrecedence() *Record {
	c := r.primaryCelebration()
	byCelebration := func() *Record { return r.findByResolvedCelebration(c) }
	return r.resolvePrecedence([]rule{
		{"principal_celebration", "celebration", func() bool { return principalCelebration(c) }, byCelebration},
		{"movable_celebration", "celebration", func() bool { return r.movableFeastWithReadings(c) }, byCelebration},
		{"festival", "celebration", func() bool { return c != nil && c.Type == "festival" }, byCelebration},
		{"common_worship_christmas_eve", "common_worship_table_2", func() bool { return r.rules.FixedOfficeEvening(r.Date, r.ServiceType) }, r.findWeeklyReading},
		{r.weeklyRuleCode(), r.rules.WeeklySourceCode(), nil, r.findWeeklyReading},
		{"minor_celebration", "celebration", nil, r.findByCelebration},
		{"civil_date", "civil_date", nil, r.findByFixedDate},
	})
}

func (r *Resolver) findSundayReadingsByPrecedence() *Record {
	c := r.primaryCelebration()
	return r.resolvePrecedence([]rule{
		{"principal_celebration", "celebration", func() bool { return principalCelebration(c) }, func() *Record { return r.findByResolvedCelebration(c) }},
		{"common_worship_christmas_eve", "common_worship_table_2", func() bool {
			return r.Date.Weekday() == 0 && r.rules.FixedOfficeEvening(r.Date, r.ServiceType)
		}, r.findByFixedDate},
		{"major_season_proper", "proper", r.sundayInMajorSeason, r.findByProper},
		{"major_season_sunday", "sunday", r.sundayInMajorSeason, r.findBySunday},
		{"sunday_celebration", "celebration", nil, r.findByCelebration},
		{"proper", "proper", nil, r.findByProper},
		{"sunday", "sunday", nil, r.findBySunday},
		{"civil_date", "civil_date", func() bool { return !r.rules.DatedChristmastideCourse(r.Date) }, r.findByFixedDate},
	})
}

func principalCelebration(c *liturgical.CelebrationAttrs) bool {
	return c != nil && (c.Type == "principal_feast" || c.Type == "major_holy_day")
}

func (r *Resolver) movableFeastWithReadings(c *liturgical.CelebrationAttrs) bool {
	if c == nil || c.ID == 0 {
		return false
	}
	var ruleName string
	if c.CalculationRule != nil {
		ruleName = *c.CalculationRule
	} else if cel := store.CelebrationByID(r.ctx, r.prayerBook(), c.ID); cel != nil && cel.CalculationRule != nil {
		ruleName = *cel.CalculationRule
	}
	if rb.BlankString(ruleName) {
		return false
	}
	for _, m := range movableRulesWithReadings {
		if m == ruleName {
			return true
		}
	}
	return false
}

func (r *Resolver) sundayInMajorSeason() bool {
	if r.Date.Weekday() != 0 {
		return false
	}
	for _, s := range liturgical.ReadingPrecedenceSeasons {
		if s == r.dayCtx.Season {
			return true
		}
	}
	return false
}

func (r *Resolver) replaceEucharistWithDailyCourse(rec *Record) *Record {
	if !r.rules.DailyOfficeCourse() || !r.officeReadingRequested() {
		return rec
	}
	if rec != nil && rec.ServiceType != "eucharist" {
		return rec
	}
	daily := r.findDailyCourseRecord()
	if daily == nil {
		return rec
	}
	prev := r.outcome
	var attempts []Attempt
	replaces := ""
	if prev != nil {
		attempts = prev.Attempts
		replaces = prev.Rule
	}
	attempts = append(attempts, Attempt{Rule: "daily_course", Status: "selected", Replaces: replaces})
	r.outcome = &Outcome{Rule: "daily_course", Source: "daily_cycle", Attempts: attempts}
	r.provenanceFallback = true
	return daily
}

func (r *Resolver) dailyCourseReferences() []string {
	refs := []string{r.builder().DailyCourseReference(), fmt.Sprintf("%d-%d", r.Date.Month(), r.Date.Day())}
	return compactUniq(refs)
}

func (r *Resolver) findDailyCourseRecord() *Record {
	refs := r.dailyCourseReferences()
	if len(refs) == 0 {
		return nil
	}
	b := &sqlBuilder{}
	r.pbCondition(b)
	list := make([]*string, len(refs))
	for i := range refs {
		list[i] = &refs[i]
	}
	b.in("date_reference", list)
	b.eq("service_type", nilIfEmpty(r.ServiceType))
	b.eq("cycle", "all")
	records := b.query(r.ctx, 0)
	if r.strictServiceVariant() {
		var kept []*Record
		for _, rec := range records {
			if rec.ServiceVariant == r.ServiceVariant {
				kept = append(kept, rec)
			}
		}
		records = kept
	}
	for _, ref := range refs {
		for _, rec := range records {
			if rec.DateReference == ref {
				return rec
			}
		}
	}
	return nil
}

func (r *Resolver) findByResolvedCelebration(info *liturgical.CelebrationAttrs) *Record {
	if info == nil {
		return nil
	}
	if info.ID == 0 {
		return nil
	}
	c := store.CelebrationByID(r.ctx, r.prayerBook(), info.ID)
	if c == nil {
		return nil
	}
	q := r.Query()
	if rec := q.FindByCelebrationID(r.ctx, c.ID); rec != nil {
		return rec
	}
	ref := rb.ParameterizeSep(c.Name, "_")
	if rec := q.FindByReference(r.ctx, ref, false); rec != nil {
		return rec
	}
	if c.CalculationRule != nil && !rb.BlankString(*c.CalculationRule) {
		if rec := q.FindByReference(r.ctx, *c.CalculationRule, false); rec != nil {
			return rec
		}
	}
	rec := q.FindByReferences(r.ctx, r.builder().CelebrationReferences(c))
	if r.officeReadingRequested() && (rec == nil || rec.CelebrationID == nil) {
		eq := &Query{
			PrayerBookID: r.prayerBookID(), Cycle: r.Cycle, Date: r.Date, ReadingType: r.ReadingType, ReadingTypeRaw: r.readingTypeRaw,
			ServiceVariant: r.ServiceVariant, StrictServiceVariant: r.strictServiceVariant(),
		}
		if e := eq.FindByCelebrationID(r.ctx, c.ID); e != nil {
			return e
		}
		if e := eq.FindByReference(r.ctx, ref, false); e != nil {
			return e
		}
	}
	return rec
}

func (r *Resolver) findByCelebration() *Record {
	c := store.FixedCelebrationOn(r.ctx, r.prayerBookID(), r.Date.Month(), r.Date.Day(), false)
	if c == nil {
		return nil
	}
	if rec := r.Query().FindByCelebrationID(r.ctx, c.ID); rec != nil {
		return rec
	}
	return r.Query().FindByReference(r.ctx, rb.ParameterizeSep(c.Name, "_"), false)
}

func (r *Resolver) findByProper() *Record {
	if r.Date.Weekday() != 0 {
		return nil
	}
	n, ok := r.cal.ProperNumber(r.Date)
	if !ok {
		return nil
	}
	return r.Query().FindByReference(r.ctx, fmt.Sprintf("proper_%d", n), false)
}

func (r *Resolver) findBySunday() *Record {
	if r.Date.Weekday() != 0 {
		return nil
	}
	return r.Query().FindByReferences(r.ctx, r.builder().SundayReferences())
}

func (r *Resolver) findByFixedDate() *Record {
	return r.Query().FindByReferences(r.ctx, r.builder().FixedDateReferences())
}

func (r *Resolver) weeklyReferencesByPrecedence() []string {
	refs := r.builder().WeeklyReferences()
	reversed := make([]string, len(refs))
	for i, s := range refs {
		reversed[len(refs)-1-i] = s
	}
	if !r.rules.DatedChristmastideCourse(r.Date) {
		return reversed
	}
	if len(refs) == 0 || rb.BlankString(refs[0]) {
		return reversed
	}
	civilRef := refs[0]
	out := []string{civilRef}
	for _, s := range reversed {
		if s != civilRef {
			out = append(out, s)
		}
	}
	return out
}

func (r *Resolver) findWeeklyReading() *Record {
	if r.Date.Weekday() == 0 {
		return nil
	}
	switch r.kind {
	case kind2015:
		return r.Query().FindWeeklyByReferences(r.ctx, r.loc2015WeeklyReferences())
	case kind2027:
		return r.weeklyQuery2027().FindWeeklyByReferences(r.ctx, r.loc2027WeeklyReferences())
	}
	return r.Query().FindWeeklyByReferences(r.ctx, r.builder().WeeklyReferences())
}

func (r *Resolver) weeklyRuleCode() string {
	if r.kind == kind2015 || r.kind == kind2027 {
		if r.preparationDay() {
			return "preparation"
		}
		return "weekly_reflection"
	}
	return r.rules.WeeklyRuleCode()
}

// --- psalm supplement -------------------------------------------------------------

func (r *Resolver) recordSlot(slot, source, ruleName string, fallback bool) {
	if r.slots == nil {
		r.slots = map[string]SlotProvenance{}
	}
	if _, ok := r.slots[slot]; !ok {
		r.slotOrder = append(r.slotOrder, slot)
	}
	r.slots[slot] = SlotProvenance{Source: source, Rule: ruleName, Fallback: fallback}
}

func (r *Resolver) commonWorshipPsalmTableReading(rec *Record) bool {
	if !r.rules.WeekdayTableLectionary() || !r.officeReadingRequested() {
		return false
	}
	if rec.ServiceVariant == "weekday_course" || rec.ServiceVariant == "ascension_alternative" {
		return true
	}
	if r.outcome == nil {
		return false
	}
	return r.outcome.Rule == "common_worship_weekday_course" || r.outcome.Rule == "common_worship_christmas_eve"
}

func (r *Resolver) supplementPsalmFromDailyCycle(rec *Record) *Record {
	if rec == nil || r.ServiceType == "" {
		return rec
	}
	rec = rec.dup()
	if r.commonWorshipPsalmTableReading(rec) {
		if ref := CwPsalmReference(r.Date, r.ServiceType, r.PsalmTable); ref != "" {
			rec.Psalm = &ref
			rec.PsalmAlternative = nil
			r.recordSlot("psalm", "common_worship_psalm_table", "common_worship_psalm_table", false)
		}
		return rec
	}
	proper := ""
	if r.officeReadingRequested() {
		proper = r.rules.ProperPsalmReference(r.Date, r.ServiceType, r.cal.Easter.Dates)
	}
	if !rb.BlankString(proper) {
		p := proper
		rec.Psalm = &p
		rec.PsalmAlternative = nil
		r.recordSlot("psalm", "proper_psalms", "proper_psalms", false)
	}
	needsPsalm := !present(rec.Psalm)
	if needsPsalm && r.officeReadingRequested() {
		if ref := r.rules.FixedPsalterReference(r.Date, r.ServiceType); ref != "" {
			rec.Psalm = &ref
			r.recordSlot("psalm", "daily_psalter", "daily_course", true)
		}
		needsPsalm = !present(rec.Psalm)
	}
	useMonthly := r.PsalmTable == "monthly" && rb.BlankString(proper)
	if (needsPsalm || useMonthly) && r.rules.MonthlyPsalter() && r.officeReadingRequested() {
		office := "evening"
		if r.ServiceType == "morning_prayer" {
			office = "morning"
		}
		if c := FindPsalmCycle(r.ctx, r.Date, office, r.Code, "monthly"); c != nil && rb.Present(c.PsalmNumbers) {
			parts := make([]string, len(c.PsalmNumbers))
			for i, n := range c.PsalmNumbers {
				parts[i] = rb.ToS(n)
			}
			joined := strings.Join(parts, ", ")
			rec.Psalm = &joined
			if useMonthly {
				rec.PsalmAlternative = nil
			}
			r.recordSlot("psalm", "monthly_psalter", "monthly_psalter", !useMonthly)
		}
		needsPsalm = !present(rec.Psalm)
	}
	needsLessons := r.rules.DailyOfficeCourse() && r.officeReadingRequested() &&
		(!present(rec.FirstReading) || !present(rec.SecondReading))
	if !needsPsalm && !needsLessons {
		return rec
	}
	daily := r.findDailyCourseRecord()
	if daily == nil || daily.ID == rec.ID {
		return rec
	}
	if needsPsalm && present(daily.Psalm) {
		rec.Psalm = daily.Psalm
		r.recordSlot("psalm", "daily_cycle", "daily_course", true)
	}
	if needsLessons {
		if !present(rec.FirstReading) && present(daily.FirstReading) {
			rec.FirstReading = daily.FirstReading
			r.recordSlot("first_reading", "daily_cycle", "daily_course", true)
		}
		if !present(rec.SecondReading) && present(daily.SecondReading) {
			rec.SecondReading = daily.SecondReading
			r.recordSlot("second_reading", "daily_cycle", "daily_course", true)
		}
	}
	return rec
}

func (r *Resolver) applyFixedHolyDaySubstitution(rec *Record) *Record {
	if rec == nil {
		return nil
	}
	observed := r.Date
	if rec.ServiceType == "vigil" {
		observed = r.Date.Add(1)
	}
	alternative := ""
	if rec.SecondReadingAlternative != nil {
		alternative = *rec.SecondReadingAlternative
	}
	if replacement, ok := r.rules.FixedHolyDaySecondReading(rec.DateReference, rec.ServiceType, observed, r.cal.Easter.Dates, alternative); ok {
		rec = rec.dup()
		rec.SecondReading = &replacement
		r.recordSlot("second_reading", "fixed_holy_day_footnote", "seasonal_substitution", true)
	}
	if r.kind == kind1928 && r.januaryThirteenthLesson(rec) {
		rec = rec.dup()
		rec.FirstReading = rec.FirstReadingAlternative
		r.recordSlot("first_reading", "calendar_footnote", "january_thirteenth", false)
	}
	return rec
}

func (r *Resolver) januaryThirteenthLesson(rec *Record) bool {
	return r.ServiceVariant == "revised_1945" && r.Date.Month() == 1 && r.Date.Day() == 13 && r.Date.Weekday() == 0 &&
		rec != nil && rec.DateReference == "1st_sunday_after_epiphany" && rec.ServiceType == "morning_prayer" &&
		present(rec.FirstReadingAlternative)
}

// FindPsalmCycle ports PsalmCycle.find_for_date_and_office.
func FindPsalmCycle(ctx context.Context, date Date, office, code, cycleType string) *store.PsalmCycle {
	pb, err := store.PrayerBookByCode(ctx, code)
	if err != nil {
		panic(err)
	}
	cache := store.PsalmCyclesFor(ctx, pb)
	switch cycleType {
	case "weekly":
		return cache[store.PsalmCycleKey("weekly", date.Weekday(), office, 0)]
	case "monthly":
		rules := liturgical.RulesFor(code)
		week := rules.MonthlyPsalterWeekNumber(date)
		exact := cache[store.PsalmCycleKey("monthly", date.Day(), office, week)]
		if exact == nil && week != 0 {
			exact = cache[store.PsalmCycleKey("monthly", date.Day(), office, 0)]
		}
		if exact != nil {
			return exact
		}
		day := mod(date.Day()-1, 30) + 1
		if date.Day() > 30 && rules.PsalterRepeatsThirtieth() {
			day = 30
		}
		if c := cache[store.PsalmCycleKey("monthly", day, office, week)]; c != nil {
			return c
		}
		return cache[store.PsalmCycleKey("monthly", day, office, 0)]
	}
	return nil
}
