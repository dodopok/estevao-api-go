package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rx"
	"github.com/dodopok/estevao-api-go/internal/store"
)

func init() {
	Register("loc_1962_en", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "midday", "evening", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc1962{}
		if c.OfficeType == "midday" || c.OfficeType == "compline" {
			b.H.LoadReadings = func() *reading.Selection { return &reading.Selection{} }
		}
		b.Init(ctx, c)
		return b
	})
}

// loc1962 ports DailyOffice::Builders::Loc1962En::* (Canada, 1962):
// TraditionalOffice for Morning and Evening Prayer, plus Mid-day and Compline.
type loc1962 struct{ Base }

func (b *loc1962) Call() *rb.Map {
	switch b.OfficeType {
	case "morning":
		return b.Render(b.morning())
	case "evening":
		return b.Render(b.evening())
	case "midday":
		return b.Render(b.midday())
	}
	return b.Render(b.compline())
}

func (b *loc1962) isMorning() bool { return b.OfficeType == "morning" }

// movable ports all_movable_dates.fetch(key).
func (b *loc1962) movable(key string) civil.Date {
	d, ok := b.Calendar().Easter.Dates.Get(key)
	if !ok {
		rb.RaiseKeyError(":" + key)
	}
	return d
}

func (b *loc1962) maundyThursday() bool { return b.Date == b.movable("maundy_thursday") }
func (b *loc1962) goodFriday() bool     { return b.Date == b.movable("good_friday") }
func (b *loc1962) holySaturday() bool   { return b.Date == b.movable("holy_saturday") }

// prefEnabled ports preference_enabled?.
func (b *Base) prefEnabled(key string) bool {
	var v any
	if b.Prefs.Has(key) {
		v = b.Prefs.Get(key)
	} else {
		v = b.PreferenceDefault(key)
	}
	s := rb.ToS(v)
	return v == true || s == "true" || s == "include"
}

// prefOmitted ports the "value == false || %w[false omit omitted]" checks.
func (b *Base) prefOmitted(key string) bool {
	var v any
	if b.Prefs.Has(key) {
		v = b.Prefs.Get(key)
	} else {
		v = b.PreferenceDefault(key)
	}
	s := rb.ToS(v)
	return v == false || s == "false" || s == "omit" || s == "omitted"
}

func (b *loc1962) includeLitany() bool { return b.prefEnabled(b.OfficeType + "_include_litany") }

func (b *loc1962) celebrationType() string { return rb.ToS(b.celebrationField("type")) }

// --- TraditionalOffice -------------------------------------------------------------------

func (b *loc1962) tail() []Step {
	if b.includeLitany() {
		return []Step{One(b.litany)}
	}
	return []Step{
		One(b.prayers),
		One(func() *Section {
			return b.TextSection("", "lords_prayer_repeated", []Entry{{Slug: b.OfficeType + "_lords_prayer_repeated"}}, false)
		}),
		One(b.suffrages),
		One(func() *Section { return b.CollectOfDaySection("Collect of the Day", b.OfficeType+"_collects_rubric") }),
		One(b.fixedCollects),
		One(b.additionalPrayers),
		One(b.theGrace),
	}
}

func (b *loc1962) exhortation() *Section {
	p := b.OfficeType
	choice := Or(b.ResolveOptions(p+"_exhortation", []string{"full", "without_second_paragraph", "short"}), "full")
	slug := p + "_exhortation"
	switch rb.ToS(choice) {
	case "without_second_paragraph":
		slug = p + "_exhortation_without_second_paragraph"
	case "short":
		slug = p + "_exhortation_short"
	}
	lines := b.plain(nil, p+"_exhortation_rubric", "rubric")
	t := b.T(slug)
	if t == nil {
		return nil
	}
	lines = append(lines, b.I(t.Content, "text"))
	return b.Section("", "exhortation", lines, nil)
}

func (b *loc1962) confession() *Section {
	lines := b.plain(nil, b.OfficeType+"_confession_rubric", "rubric")
	lines = b.plain(lines, b.OfficeType+"_confession", "text")
	return b.Section("", "confession", lines, nil)
}

func (b *loc1962) absolution() *Section {
	p := b.OfficeType
	lines := b.plain(nil, p+"_absolution_rubric", "rubric")
	lines = b.plain(lines, p+"_absolution", "text")
	lines = b.plain(lines, p+"_absolution_response_rubric", "rubric")
	return b.Section("", "absolution", lines, nil)
}

func (b *loc1962) lordsPrayer() *Section {
	return b.TextSection("", "lords_prayer", []Entry{
		{Slug: b.OfficeType + "_lords_prayer_rubric", Type: "rubric"},
		{Slug: b.OfficeType + "_lords_prayer"},
	}, false)
}

func (b *loc1962) versicles() *Section {
	p := b.OfficeType
	var lines []*Line
	for n := 1; n <= 2; n++ {
		lines = append(lines,
			b.I(b.contentOrNil(fmt.Sprintf("%s_inv_v%d", p, n)), "leader"),
			b.I(b.contentOrNil(fmt.Sprintf("%s_inv_r%d", p, n)), "congregation"))
	}
	lines = append(lines,
		b.I(b.contentOrNil(p+"_gloria_patri_rubric"), "rubric"),
		b.I(b.contentOrNil(p+"_gloria_patri_v"), "leader"),
		b.I(b.contentOrNil(p+"_gloria_patri_r"), "congregation"),
		b.I(b.contentOrNil(p+"_inv_v3"), "leader"),
		b.I(b.contentOrNil(p+"_inv_r3"), "congregation"))
	return b.Section("", "versicles", lines, nil)
}

func (b *loc1962) psalms() *Section {
	ps := b.Readings.Psalm
	if ps == nil {
		return nil
	}
	lines := b.plain(nil, b.OfficeType+"_psalms_rubric", "rubric")
	lines = append(lines, b.I(ps.Reference, "heading"))
	if ps.Content != nil {
		lines = append(lines, b.BibleContent(ps.Content)...)
	}
	if !(b.maundyThursday() || b.goodFriday() || b.holySaturday()) {
		prefix := "evening_gloria_patri"
		if b.isMorning() {
			prefix = "morning_psalms_gloria_patri"
		}
		lines = b.plain(lines, prefix+"_v", "leader")
		lines = b.plain(lines, prefix+"_r", "congregation")
	}
	return b.Section("", "psalms", lines, nil)
}

// reading: the Canadian book names each Lesson by its citation.
func (b *loc1962) reading(typ string) *Section {
	announcement, end := "", ""
	if b.isMorning() {
		announcement, end = "morning_reading_announcement_rubric", "morning_reading_end_rubric"
	}
	return b.ReadingModule(typ, announcement, Rubrics{Pre: b.OfficeType + "_" + typ + "_reading_rubric", End: end},
		typ+"_reading", "", true)
}

func (b *loc1962) firstCanticle() *Section {
	var slug any
	if b.isMorning() {
		if b.goodFriday() {
			slug = "morning_good_friday_salvator"
		} else {
			slug = Or(b.ResolveOptions("morning_first_canticle", []string{"morning_te_deum", "morning_benedicite"}), "morning_te_deum")
		}
	} else {
		slug = Or(b.ResolveOptions("evening_first_canticle", []string{"evening_magnificat", "evening_cantate_domino"}), "evening_magnificat")
	}
	if b.isMorning() && slug == "morning_benedicite" && !b.benediciteSuitable() {
		slug = "morning_te_deum"
	}
	if b.isMorning() && slug == "morning_te_deum" && b.prefOmitted("morning_te_deum_third_section") {
		slug = "morning_te_deum_without_third_section"
	}
	c := b.tAny(slug)
	if c == nil {
		return nil
	}
	rubric := b.OfficeType + "_first_canticle_rubric"
	if b.isMorning() && b.goodFriday() {
		rubric = "morning_good_friday_first_canticle_rubric"
	}
	lines := b.plain(nil, rubric, "rubric")
	lines = append(lines, b.I(b.canticleContent(c), "text"))
	return b.Section(Title(c), "first_canticle", lines, nil)
}

func (b *loc1962) secondCanticle() *Section {
	var slug any
	if b.isMorning() {
		slug = Or(b.ResolveOptions("morning_second_canticle", []string{"morning_benedictus", "morning_jubilate"}), "morning_benedictus")
	} else if b.goodFriday() {
		slug = "evening_nunc_dimittis"
	} else {
		slug = Or(b.ResolveOptions("evening_second_canticle", []string{"evening_nunc_dimittis", "evening_deus_misereatur"}), "evening_nunc_dimittis")
	}
	c := b.tAny(slug)
	if c == nil {
		return nil
	}
	lines := b.plain(nil, b.OfficeType+"_second_canticle_rubric", "rubric")
	lines = append(lines, b.I(b.canticleContent(c), "text"))
	return b.Section(Title(c), "second_canticle", lines, nil)
}

func (b *loc1962) canticleContent(c *store.LiturgicalText) string {
	if !b.goodFriday() {
		return c.Content
	}
	return stripGloriaPatri(c.Content)
}

var (
	gloriaLineRe = rx.MustCompile(`\A\s*(?:GLORY|Glory) be to the Father`)
	asItWasRe    = rx.MustCompile(`\A\s*As it was in the beginning`)
	danielRe     = rx.MustCompile(`\ADaniel 3(?::|\b)`)
)

// rubyLines ports String#lines (each line keeps its "\n").
func rubyLines(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// stripGloriaPatri ports strip_gloria_patri.
func stripGloriaPatri(content string) string {
	var b strings.Builder
	for _, l := range rubyLines(content) {
		if gloriaLineRe.MatchString(l) || asItWasRe.MatchString(l) {
			continue
		}
		b.WriteString(l)
	}
	return b.String()
}

func (b *loc1962) benediciteSuitable() bool {
	d := b.Date
	if d.Between(b.movable("first_sunday_of_advent"), civil.MustNew(d.Year(), 12, 24)) {
		return true
	}
	easter := b.movable("easter")
	if d.Between(b.movable("ash_wednesday"), easter.Add(-1)) {
		return true
	}
	rules := liturgical.RulesFor("loc_1962_en")
	m := b.Calendar().Easter.Dates
	pentecost := b.movable("pentecost")
	for _, e := range rules.EmberDays(d.Year(), m) {
		if e.Between(pentecost, pentecost.Add(6)) {
			continue
		}
		if e == d {
			return true
		}
	}
	for _, r := range rules.RogationDays(m) {
		if r == d {
			return true
		}
	}
	ref := ""
	if r := b.Readings.FirstReading; r != nil {
		ref = r.Reference
	}
	return danielRe.MatchString(ref)
}

func (b *loc1962) creedApostles() *Section {
	lines := b.plain(nil, b.OfficeType+"_creed_rubric", "rubric")
	lines = b.plain(lines, "apostles_creed", "text")
	return b.Section("", "creed", lines, nil)
}

func (b *loc1962) prayers() *Section {
	p := b.OfficeType
	lines := b.plain(nil, p+"_prayers_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil(p+"_salutation_v"), "leader"),
		b.I(b.contentOrNil(p+"_salutation_r"), "congregation"),
		b.I(b.contentOrNil(p+"_lesser_litany_rubric"), "rubric"),
		b.I(b.contentOrNil("kyrie"), "responsive"))
	return b.Section("", "prayers", lines, nil)
}

func (b *loc1962) suffrages() *Section {
	p := b.OfficeType
	lines := b.plain(nil, p+"_suffrages_rubric", "rubric")
	for n := 1; n <= 6; n++ {
		lines = append(lines,
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_v%d", p, n)), "leader"),
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_r%d", p, n)), "congregation"))
	}
	return b.Section("", "suffrages", lines, nil)
}

func (b *loc1962) fixedCollects() *Section {
	if b.isMorning() {
		return b.TextSection("Fixed Collects", "fixed_collects", []Entry{
			{Slug: "morning_collect_peace", Heading: true},
			{Slug: "morning_collect_grace", Heading: true},
		}, false)
	}
	lines := b.titled(nil, "evening_collect_peace", "leader")
	lines = b.titled(lines, "evening_collect_perils", "leader")
	return b.Section("", "fixed_collects", lines, nil)
}

func (b *loc1962) additionalPrayers() *Section {
	p := b.OfficeType
	lines := b.plain(nil, p+"_additional_prayers_rubric", "rubric")
	key := Or(b.ResolveOptions(p+"_authority_prayer", []string{"1", "2", "3"}), "1")
	lines = b.titled(lines, p+"_prayer_civil_authority_"+rubyInterp(key), "text")
	lines = b.titled(lines, p+"_prayer_clergy_people", "text")
	lines = b.titled(lines, "prayer_st_chrysostom", "text")
	return b.Section("Additional Prayers", "additional_prayers", lines, nil)
}

func (b *loc1962) theGrace() *Section {
	t := b.T("the_grace")
	if t == nil {
		return nil
	}
	return b.Section(Title(t), "the_grace", []*Line{b.I(t.Content, "text")}, nil)
}

func (b *loc1962) litany() *Section {
	lines := b.plain(nil, "litany_rubric", "rubric")
	for n := 1; n <= 4; n++ {
		lines = b.plain(lines, fmt.Sprintf("litany_part_%d", n), "responsive")
	}
	lines = b.plain(lines, b.OfficeType+"_lords_prayer", "congregation")
	lines = b.plain(lines, "litany_after_lords_prayer_rubric", "rubric")
	lines = append(lines, b.CollectLines(b.Collects)...)
	if c := b.T("prayer_st_chrysostom"); c != nil {
		if c.Title != nil {
			lines = append(lines, b.I(*c.Title, "heading"))
		}
		lines = append(lines, b.I(c.Content, "prayer"))
	}
	lines = b.plain(lines, "the_grace", "prayer")
	return b.Section("The Litany", "litany", lines, nil)
}

// --- Morning -----------------------------------------------------------------------------

func (b *loc1962) morning() []*Section {
	var steps []Step
	if !(b.maundyThursday() || b.holySaturday()) {
		steps = append(steps,
			One(b.mOpeningSentence),
			One(b.exhortation),
			One(b.confession),
			One(b.absolution),
			One(b.lordsPrayer),
			One(b.versicles))
	}
	steps = append(steps,
		One(b.mInvitatory),
		One(b.psalms),
		One(func() *Section { return b.reading("first") }),
		One(b.firstCanticle),
		One(func() *Section { return b.reading("second") }),
		One(b.secondCanticle),
		One(b.mCreed))
	return Pipeline(append(steps, b.tail()...)...)
}

func (b *loc1962) mOpeningSentence() *Section {
	lines := b.plain(nil, "morning_opening_sentence_rubric", "rubric")
	var keys []int
	for _, v := range Arr(Or(b.ResolveRange("morning_opening_sentence", 1, 26), 1)) {
		k := rubyToI(v)
		if k >= 1 && k <= 26 {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		keys = []int{1}
	}
	for _, k := range keys {
		s := b.T(fmt.Sprintf("morning_opening_sentence_%d", k))
		if s == nil {
			continue
		}
		lines = append(lines, b.I(s.Content, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return b.Section("", "opening_sentence", lines, nil)
}

// rubyToI ports #to_i on an Integer or a String.
func rubyToI(v any) int {
	if n, ok := v.(int); ok {
		return n
	}
	return rb.StringToI(rb.ToS(v))
}

func (b *loc1962) mInvitatory() *Section {
	if key := b.properMorningAnthemKey(); key != "" {
		return b.properMorningAnthem(key)
	}
	if b.maundyThursday() || b.holySaturday() {
		return b.triduumVenite()
	}
	if b.Date.Day() == 19 {
		return nil
	}
	lines := b.plain(nil, "morning_venite_rubric", "rubric")
	var antiphon *store.LiturgicalText
	if v, ok := b.ResolveOptions("morning_invitatory_antiphon", []string{"appointed", "omit"}).(string); !ok || v != "omit" {
		antiphon = b.T("morning_invitatory_antiphon_" + b.invitatoryAntiphonKey())
	}
	if antiphon != nil {
		lines = append(lines, b.I(antiphon.Content, "antiphon"))
	}
	c := b.T(b.veniteSlug())
	if c == nil {
		return nil
	}
	lines = append(lines, b.I(c.Content, "text"))
	if antiphon != nil {
		lines = append(lines, b.I(antiphon.Content, "antiphon"))
	}
	return b.Section(Title(c), "invitatory_canticle", lines, nil)
}

func (b *loc1962) veniteSlug() string {
	if b.prefOmitted("morning_venite_last_four") {
		return "morning_venite_without_last_four"
	}
	return "morning_venite"
}

func (b *loc1962) triduumVenite() *Section {
	rubric := "morning_holy_saturday_venite_rubric"
	if b.maundyThursday() {
		rubric = "morning_maundy_thursday_venite_rubric"
	}
	lines := b.plain(nil, rubric, "rubric")
	var antiphon *store.LiturgicalText
	if b.maundyThursday() {
		antiphon = b.T("morning_invitatory_antiphon_" + b.invitatoryAntiphonKey())
	}
	if antiphon != nil {
		lines = append(lines, b.I(antiphon.Content, "antiphon"))
	}
	c := b.T(b.veniteSlug())
	if c == nil {
		return nil
	}
	lines = append(lines, b.I(stripGloriaPatri(c.Content), "text"))
	if antiphon != nil {
		lines = append(lines, b.I(antiphon.Content, "antiphon"))
	}
	return b.Section(Title(c), "invitatory_canticle", lines, nil)
}

func (b *loc1962) properMorningAnthemKey() string {
	d := b.Date
	switch {
	case d.Month() == 12 && d.Day() == 25:
		return "christmas"
	case d == b.movable("good_friday"):
		return "good_friday"
	case d == b.movable("easter"):
		return "easter"
	case d == b.movable("pentecost"):
		return "pentecost"
	}
	return ""
}

func (b *loc1962) properMorningAnthem(key string) *Section {
	rubricSlug, anthemSlug := "morning_"+key+"_anthem_rubric", "morning_"+key+"_anthem"
	if key == "easter" {
		rubricSlug, anthemSlug = "morning_easter_venite_rubric", "morning_christ_our_passover"
	}
	rubric, anthem := b.T(rubricSlug), b.T(anthemSlug)
	if anthem == nil {
		return nil
	}
	var lines []*Line
	if rubric != nil {
		lines = append(lines, b.I(rubric.Content, "rubric"))
	}
	lines = append(lines, b.I(anthem.Content, "anthem"))
	var name any = rb.Humanize(key)
	if anthem.Title != nil {
		name = *anthem.Title
	}
	return b.Section(name, "invitatory_canticle", lines, nil)
}

func (b *loc1962) invitatoryAntiphonKey() string {
	d := b.Date
	m, day := d.Month(), d.Day()
	md := func(pairs ...[2]int) bool {
		for _, p := range pairs {
			if p[0] == m && p[1] == day {
				return true
			}
		}
		return false
	}
	if md([2]int{1, 6}, [2]int{8, 6}) {
		return "epiphany"
	}
	if md([2]int{2, 2}, [2]int{3, 25}, [2]int{7, 2}, [2]int{8, 15}, [2]int{9, 8}, [2]int{12, 8}) {
		return "blessed_virgin_mary"
	}
	if b.celebrationType() == "major_holy_day" && !b.canadianMovableMajorHolyDay() {
		return "saints"
	}
	if d == b.movable("trinity_sunday") {
		return "trinity"
	}
	if d.Between(b.movable("first_sunday_of_advent"), civil.MustNew(d.Year(), 12, 24)) {
		return "advent"
	}
	christmas := civil.MustNew(d.Year(), 12, 25)
	if m == 1 {
		christmas = civil.MustNew(d.Year()-1, 12, 25)
	}
	if d.Between(christmas, christmas.Add(11)) {
		return "christmastide"
	}
	if d.Between(civil.MustNew(d.Year(), 1, 6), civil.MustNew(d.Year(), 1, 13)) {
		return "epiphany"
	}
	easter := b.movable("easter")
	switch {
	case d.Between(easter.Add(-14), easter.Add(-1)):
		return "passiontide"
	case d.Between(b.movable("ash_wednesday"), easter.Add(-15)):
		return "lent"
	case d.Between(easter.Add(1), b.movable("ascension").Add(-1)):
		return "eastertide"
	case d.Between(b.movable("ascension"), b.movable("pentecost").Add(-1)):
		return "ascensiontide"
	case d.Between(b.movable("pentecost"), b.movable("pentecost").Add(6)):
		return "whitsuntide"
	}
	if d.IsSunday() {
		return "other_sundays"
	}
	return "other_weekdays"
}

func (b *loc1962) canadianMovableMajorHolyDay() bool {
	for _, k := range []string{"ash_wednesday", "palm_sunday", "maundy_thursday", "good_friday", "holy_saturday"} {
		if b.Date == b.movable(k) {
			return true
		}
	}
	return false
}

func (b *loc1962) mCreed() *Section {
	choice := Or(b.ResolveOptions("morning_creed", []string{"apostles_creed", "athanasian_creed"}), "apostles_creed")
	if choice == "athanasian_creed" {
		lines := b.plain(nil, "athanasian_creed_rubric", "rubric")
		lines = b.plain(lines, "athanasian_creed", "text")
		return b.Section("Athanasian Creed", "creed", lines, nil)
	}
	lines := b.plain(nil, "morning_creed_rubric", "rubric")
	lines = b.plain(lines, "apostles_creed", "text")
	return b.Section("", "creed", lines, nil)
}

// --- Evening -----------------------------------------------------------------------------

func (b *loc1962) evening() []*Section {
	steps := []Step{
		One(b.eOpeningSentence),
		One(b.exhortation),
		One(b.confession),
		One(b.absolution),
		One(b.lordsPrayer),
		One(b.versicles),
		One(b.psalms),
		One(func() *Section { return b.reading("first") }),
		One(b.oSapientia),
		One(b.firstCanticle),
		One(func() *Section { return b.reading("second") }),
		One(b.secondCanticle),
		One(b.creedApostles),
	}
	return Pipeline(append(steps, b.tail()...)...)
}

func (b *loc1962) oSapientia() *Section {
	if b.Date.Month() != 12 || b.Date.Day() != 16 {
		return nil
	}
	t := b.T("evening_o_sapientia")
	if t == nil {
		return nil
	}
	var name any = "O Sapientia"
	if t.Title != nil {
		name = *t.Title
	}
	return b.Section(name, "o_sapientia", []*Line{b.Item(t.Content, "anthem", "", Ref(t))}, nil)
}

func (b *loc1962) eOpeningSentence() *Section {
	lines := b.plain(nil, "evening_opening_sentence_rubric", "rubric")
	configured := b.Pref("evening_opening_sentence")
	if rb.Blank(configured) {
		configured = b.PreferenceDefault("evening_opening_sentence")
	}
	var sentence *store.LiturgicalText
	if rb.ToS(configured) == "morning_seasonal" {
		if key := b.seasonalMorningSentenceKey(); key > 0 {
			sentence = b.T(fmt.Sprintf("morning_opening_sentence_%d", key))
		}
	} else {
		choice := Or(b.ResolveRange("evening_opening_sentence", 1, 9), 1)
		sentence = b.T("evening_opening_sentence_" + rubyInterp(choice))
	}
	if sentence != nil {
		lines = append(lines, b.I(sentence.Content, "text"))
		if sentence.Reference != nil {
			lines = append(lines, b.I(*sentence.Reference, "citation"))
		}
	}
	return b.Section("", "opening_sentence", lines, nil)
}

// seasonalMorningSentenceKey ports seasonal_morning_sentence_key (0 = nil).
func (b *loc1962) seasonalMorningSentenceKey() int {
	d := b.Date
	christmas := civil.MustNew(d.Year(), 12, 25)
	if d.Month() <= 1 {
		christmas = civil.MustNew(d.Year()-1, 12, 25)
	}
	easter := b.movable("easter")
	pentecost := b.movable("pentecost")
	switch {
	case d.Between(christmas, christmas.Add(11)):
		return 2
	case d >= b.movable("first_sunday_of_advent") && d <= civil.MustNew(d.Year(), 12, 24):
		return 1
	case d.Month() == 1 && d >= civil.MustNew(d.Year(), 1, 6) && d < b.movable("septuagesima"):
		return 3
	case d == b.movable("good_friday"):
		return 7
	case d == easter:
		return 8
	case d > easter && d < b.movable("ascension"):
		return 9
	case d == b.movable("ascension"):
		return 10
	case d >= pentecost && d <= pentecost.Add(6):
		return 11
	case d == b.movable("trinity_sunday"):
		return 13
	case d.Between(easter.Add(-14), easter.Add(-2)):
		return 6
	case d >= b.movable("ash_wednesday") && d < easter.Add(-14):
		return 4
	case b.celebrationType() == "major_holy_day":
		return 14
	}
	m := b.Calendar().Easter.Dates
	for _, k := range []string{"rogation_monday", "rogation_tuesday", "rogation_wednesday"} {
		if r, ok := m[k]; ok && r == d {
			return 15
		}
	}
	if d.Month() == 1 && d.Day() == 6 {
		return 3
	}
	return 0
}

// --- Mid-day -----------------------------------------------------------------------------

func (b *loc1962) midday() []*Section {
	return Pipeline(
		One(func() *Section {
			lines := b.plain(nil, "midday_heading", "heading")
			lines = b.plain(lines, "midday_lords_prayer", "congregation")
			for n := 1; n <= 3; n++ {
				v, c := b.T(fmt.Sprintf("midday_versicle_%d", n)), b.T(fmt.Sprintf("midday_collect_%d", n))
				if v != nil {
					lines = append(lines, b.I(v.Content, "leader"))
					if v.Reference != nil {
						lines = append(lines, b.I(*v.Reference, "citation"))
					}
				}
				if c != nil {
					lines = append(lines, b.I(c.Content, "congregation"))
				}
			}
			return b.Section("Prayers at Mid-day", "prayers", lines, nil)
		}),
		One(func() *Section {
			t := b.T("midday_additional_rubric")
			if t == nil {
				return nil
			}
			return b.Section("Additional Prayers", "additional_prayers", []*Line{b.I(t.Content, "rubric")}, nil)
		}),
	)
}

// --- Compline ----------------------------------------------------------------------------

var (
	loc1962ComplinePsalms      = []string{"Psalm 4", "Psalm 31:1-6", "Psalm 91", "Psalm 134"}
	loc1962ComplinePsalmLabels = map[string]string{"Psalm 31:1-6": "Psalm 31. 1-6"}
)

func (b *loc1962) compline() []*Section {
	return Pipeline(
		One(b.cOpening),
		One(b.cPsalms),
		One(b.cLesson),
		One(b.cResponsory),
		One(b.cHymn),
		One(func() *Section {
			lines := b.plain(nil, "compline_apple_v", "leader")
			lines = b.plain(lines, "compline_apple_r", "congregation")
			return b.Section("", "apple_of_an_eye", lines, nil)
		}),
		One(b.cNuncDimittis),
		One(func() *Section {
			lines := b.plain(nil, "compline_creed_rubric", "rubric")
			lines = b.plain(lines, "apostles_creed", "text")
			return b.Section("", "creed", lines, nil)
		}),
		One(b.cPrayers),
		One(b.cConfession),
		One(b.cPreces),
		One(b.cCollects),
		One(b.cConclusion),
	)
}

// pairs ports the "(1..n).all? v&&r ? structured : fallback" blocks.
func (b *loc1962) pairs(lines []*Line, prefix string, n int, fallback string) []*Line {
	for i := 1; i <= n; i++ {
		if b.T(fmt.Sprintf("%s_v%d", prefix, i)) == nil || b.T(fmt.Sprintf("%s_r%d", prefix, i)) == nil {
			return b.plain(lines, fallback, "responsive")
		}
	}
	for i := 1; i <= n; i++ {
		lines = append(lines,
			b.I(b.T(fmt.Sprintf("%s_v%d", prefix, i)).Content, "leader"),
			b.I(b.T(fmt.Sprintf("%s_r%d", prefix, i)).Content, "congregation"))
	}
	return lines
}

func (b *loc1962) cOpening() *Section {
	lines := b.plain(nil, "compline_opening_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil("compline_opening"), "leader"),
		b.I(b.contentOrNil("compline_versicle_make_speed"), "leader"),
		b.I(b.contentOrNil("compline_response_make_speed"), "congregation"))
	for _, k := range []string{"compline_gloria", "compline_praise"} {
		v, r := b.T(k+"_v"), b.T(k+"_r")
		if v != nil && r != nil {
			lines = append(lines, b.I(v.Content, "leader"), b.I(r.Content, "congregation"))
		} else {
			lines = b.plain(lines, k, "responsive")
		}
	}
	return b.Section("", "opening", lines, nil)
}

func (b *loc1962) cPsalms() *Section {
	sel := Or(b.ResolveOptions("compline_psalms", loc1962ComplinePsalms), stringsToAny(loc1962ComplinePsalms))
	if sel == "all" {
		sel = stringsToAny(loc1962ComplinePsalms)
	}
	var selected []string
	seen := map[string]bool{}
	for _, x := range Arr(sel) {
		s := rb.ToS(x)
		if seen[s] {
			continue
		}
		for _, p := range loc1962ComplinePsalms {
			if p == s {
				seen[s] = true
				selected = append(selected, s)
			}
		}
	}
	if len(selected) == 0 {
		selected = loc1962ComplinePsalms
	}
	contents := (&reading.ContentLoader{
		Translation: b.PsalmScriptureTranslation(), PsalmTranslation: b.SelectedPsalmTranslation(),
	}).Load(b.Ctx, selected)
	lines := b.plain(nil, "compline_psalms_rubric", "rubric")
	for _, ref := range selected {
		label := ref
		if l, ok := loc1962ComplinePsalmLabels[ref]; ok {
			label = l
		}
		lines = append(lines, b.I(label, "heading"))
		if c := contents[ref]; c != nil && c.Len() > 0 {
			lines = append(lines, b.BibleContent(c)...)
		}
	}
	return b.Section("The Psalms", "psalms", lines, nil)
}

func (b *loc1962) cLesson() *Section {
	number := Or(b.ResolveRange("compline_reading", 1, 3), 1)
	if l, ok := number.([]any); ok {
		panic(&rb.RubyError{Class: "NoMethodError", Message: "undefined method `to_i' for " + rb.Inspect(l) + ":Array"})
	}
	if n := rubyToI(number); n < 1 || n > 3 {
		number = 1
	}
	text := b.T("compline_lesson_" + rubyInterp(number))
	lines := b.plain(nil, "compline_lesson_rubric", "rubric")
	if text != nil {
		lines = append(lines, b.I(text.Content, "text"))
		if text.Reference != nil {
			lines = append(lines, b.I(*text.Reference, "citation"))
		}
	}
	lines = b.plain(lines, "compline_thanks_be_to_god", "congregation")
	return b.Section("The Lesson", "lesson", lines, nil)
}

func (b *loc1962) cResponsory() *Section {
	if !b.prefEnabled("compline_respond") {
		return nil
	}
	lines := b.plain(nil, "compline_responsory_rubric", "rubric")
	lines = b.pairs(lines, "compline_respond", 3, "compline_responsory")
	if len(lines) == 0 {
		return nil
	}
	return b.Section("The Respond", "responsory", lines, nil)
}

func (b *loc1962) cHymn() *Section {
	if !b.prefEnabled("compline_hymn") {
		return nil
	}
	t := b.T("compline_hymn_te_lucis")
	if t == nil {
		return nil
	}
	lines := b.plain(nil, "compline_hymn_rubric", "rubric")
	if t.Title != nil {
		lines = append(lines, b.I(*t.Title, "heading"))
	}
	lines = append(lines, b.I(t.Content, "text"))
	return b.Section("The Office Hymn", "hymn", lines, nil)
}

func (b *loc1962) cNuncDimittis() *Section {
	eastertide := b.Date.Between(b.movable("easter"), b.movable("pentecost").Add(-1))
	anthemSlug, rubricSlug := "compline_anthem", "compline_anthem_rubric"
	if eastertide {
		anthemSlug, rubricSlug = "compline_anthem_easter", "compline_easter_anthem_rubric"
	}
	anthem := b.T(anthemSlug)
	lines := b.plain(nil, rubricSlug, "rubric")
	if anthem != nil {
		lines = append(lines, b.I(anthem.Content, "anthem"))
	}
	lines = b.plain(lines, "compline_nunc_dimittis", "text")
	if anthem != nil {
		lines = append(lines, b.I(anthem.Content, "anthem"))
	}
	return b.Section("Nunc Dimittis", "second_canticle", lines, nil)
}

func (b *loc1962) cPrayers() *Section {
	lines := b.plain(nil, "compline_prayers_rubric", "rubric")
	lines = b.plain(lines, "compline_let_us_pray", "leader")
	lines = b.plain(lines, "compline_kyrie", "responsive")
	lines = b.plain(lines, "compline_lords_prayer", "congregation")
	lines = b.pairs(lines, "compline_suff", 4, "compline_suffrages")
	return b.Section("Prayers", "prayers", lines, nil)
}

func (b *loc1962) cConfession() *Section {
	lines := b.plain(nil, "compline_confession_rubric", "rubric")
	lines = b.plain(lines, "compline_confession", "congregation")
	if b.prefEnabled("compline_priest") {
		lines = b.plain(lines, "compline_absolution_rubric", "rubric")
		lines = b.plain(lines, "compline_absolution", "leader")
	}
	return b.Section("Confession", "confession", lines, nil)
}

func (b *loc1962) cPreces() *Section {
	lines := b.plain(nil, "compline_preces_rubric", "rubric")
	lines = b.pairs(lines, "compline_preces", 4, "compline_preces")
	if len(lines) == 0 {
		return nil
	}
	return b.Section("Preces", "preces", lines, nil)
}

func (b *loc1962) cCollects() *Section {
	lines := b.plain(nil, "compline_collects_rubric", "rubric")
	protection := Or(b.ResolveOptions("compline_protection_collect", []string{"visit", "lighten"}), "visit")
	if t := b.T("compline_collect_" + rubyInterp(protection)); t != nil {
		lines = append(lines, b.FetchLineItem("compline_protection_collect_heading", "heading", nil, "content"), b.I(t.Content, "prayer"))
	}
	lines = append(lines, b.CollectLines(b.Collects)...)
	additional := Or(b.ResolveOptions("compline_additional_collects",
		[]string{"none", "jesus", "look_down", "be_present", "all"}), "none")
	if additional == "all" {
		additional = []any{"jesus", "look_down", "be_present"}
	}
	var keys []string
	for _, x := range Arr(additional) {
		if s := rb.ToS(x); s != "none" {
			keys = append(keys, s)
		}
	}
	if len(keys) > 0 {
		lines = b.plain(lines, "compline_additional_collects_rubric", "rubric")
		for _, k := range keys {
			lines = b.plain(lines, "compline_collect_"+k, "prayer")
		}
	}
	lines = b.plain(lines, "compline_other_prayers_rubric", "rubric")
	return b.Section("Collects", "collects", lines, nil)
}

func (b *loc1962) cConclusion() *Section {
	var lines []*Line
	for _, slug := range []string{"compline_lay_down", "compline_lay_down_response", "compline_salutation_v",
		"compline_salutation_r", "compline_let_us_bless", "compline_thanks_be_to_god", "compline_blessing"} {
		typ := "leader"
		switch slug {
		case "compline_lay_down_response", "compline_salutation_r", "compline_thanks_be_to_god":
			typ = "congregation"
		}
		lines = b.plain(lines, slug, typ)
	}
	return b.Section("The Conclusion", "conclusion", lines, nil)
}
