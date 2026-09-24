package dailyoffice

import (
	"context"
	"fmt"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// The 1928 offices subclass the English 1662 offices in Rails
// (Loc1662En -> Loc1928En -> Loc1928Pt, and Loc1928En -> Rec2005En with the
// Rec2005En::Shared module prepended). One type ports the three, switched by
// variant where a subclass overrides a step.
const (
	v1928En = "loc_1928_en"
	v1928Pt = "loc_1928_pt"
	vRec    = "rec_2005_en"
)

func init() {
	for _, code := range []string{v1928En, v1928Pt, vRec} {
		code := code
		Register(code, func(ctx context.Context, c *Context) Builder {
			if c.OfficeType != "morning" && c.OfficeType != "evening" {
				UnknownOfficeType(c.OfficeType)
			}
			b := &loc1928{variant: code}
			b.profile = loc1662Profiles["loc_1662_en"]
			if code == vRec {
				b.H.FetchText = func(slug string) *store.LiturgicalText {
					if b.PrayerBook() == nil {
						return nil
					}
					return b.Texts()[b.recLanguageStyle()+"_"+slug]
				}
				b.H.ReadingServiceVariant = func() string { return b.recLanguageStyle() + "_set_1" }
				b.H.CollectLanguageStyle = b.recLanguageStyle
			}
			b.Init(ctx, c)
			return b
		})
	}
}

type loc1928 struct {
	loc1662
	variant string
}

func (b *loc1928) rec() bool { return b.variant == vRec }

func (b *loc1928) Call() *rb.Map {
	if b.OfficeType == "morning" {
		return b.Render(b.morning())
	}
	return b.Render(b.evening())
}

// --- Rec2005En::Shared -------------------------------------------------------------------

// presentPrefOrDefault ports preferences[key].presence || preference_default(key).
func (b *Base) presentPrefOrDefault(key string) any {
	if v := b.Pref(key); rb.Present(v) {
		return v
	}
	return b.PreferenceDefault(key)
}

func (b *loc1928) recLanguageStyle() string {
	v := rb.ToS(b.presentPrefOrDefault("rec_language_style"))
	if v == "traditional" || v == "modern" {
		return v
	}
	return "traditional"
}

func (b *loc1928) recLordsPrayerForm() string {
	v := rb.ToS(b.presentPrefOrDefault("rec_lords_prayer_form"))
	if v == "traditional" || v == "contemporary" {
		return v
	}
	return "traditional"
}

func (b *loc1928) recContemporary() bool {
	return b.recLanguageStyle() == "modern" && b.recLordsPrayerForm() == "contemporary"
}

func (b *loc1928) recModernVersicles(lines []*Line, prefix string) []*Line {
	if b.recLanguageStyle() != "modern" {
		return lines
	}
	return append(lines,
		b.I(b.contentOrNil(prefix+"_inv_v2"), "leader"),
		b.I(b.contentOrNil(prefix+"_inv_r2"), "congregation"))
}

// --- shared 1928 helpers -----------------------------------------------------------------

// reading1928 ports Loc1928En's build_reading_module call: no rubric, the
// reference as heading (REC 2005 prints no heading).
func (b *loc1928) reading1928(typ, announcement string) *Section {
	return b.ReadingModule(typ, announcement, Rubrics{}, typ+"_reading", "", !b.rec())
}

func (b *loc1928) openingSentence1928(prefix string, hi int) *Section {
	lines := b.plain(nil, prefix+"_opening_sentence_rubric", "rubric")
	key := Or(b.ResolveRange(prefix+"_opening_sentence", 1, hi), 1)
	if s := b.T(prefix + "_opening_sentence_" + rubyInterp(key)); s != nil {
		lines = append(lines, b.I(s.Content, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return b.Section("", "opening_sentence", lines, nil)
}

func (b *loc1928) omitPenitentialOpening() bool {
	v, ok := b.ResolveOptions(b.OfficeType+"_penitential_opening", []string{"full", "omit"}).(string)
	return ok && v == "omit"
}

func (b *loc1928) preferenceEnabled(key string) bool {
	var v any
	if b.Prefs.Has(key) {
		v = b.Prefs.Get(key)
	} else {
		v = b.PreferenceDefault(key)
	}
	s := rb.ToS(v)
	return v == true || s == "true" || s == "include"
}

func (b *loc1928) includeLitany() bool { return b.preferenceEnabled(b.OfficeType + "_include_litany") }

func (b *loc1928) lordsPrayer1928(prefix string) *Section {
	if !b.rec() {
		return b.lordsPrayer(prefix)
	}
	lines := b.plain(nil, prefix+"_lords_prayer_rubric", "rubric")
	slug := prefix + "_lords_prayer"
	if b.recContemporary() {
		slug += "_contemporary"
	}
	lines = b.plain(lines, slug, "text")
	return b.Section("", "lords_prayer", lines, nil)
}

func (b *loc1928) versicles1928(prefix string) *Section {
	lines := b.plain(nil, prefix+"_versicles_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil(prefix+"_inv_v1"), "leader"),
		b.I(b.contentOrNil(prefix+"_inv_r1"), "congregation"))
	if b.rec() {
		lines = b.recModernVersicles(lines, prefix)
	}
	for _, p := range [][2]string{
		{"_gloria_patri_rubric", "rubric"}, {"_gloria_patri_v", "leader"}, {"_gloria_patri_r", "congregation"},
		{"_inv_v3", "leader"}, {"_inv_r3", "congregation"},
	} {
		lines = append(lines, b.I(b.contentOrNil(prefix+p[0]), p[1]))
	}
	return b.Section("", "versicles", lines, nil)
}

func (b *loc1928) prayers1928(prefix string) *Section {
	lines := b.plain(nil, prefix+"_prayers_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil(prefix+"_salutation_v"), "leader"),
		b.I(b.contentOrNil(prefix+"_salutation_r"), "congregation"),
		b.I(b.contentOrNil(prefix+"_lesser_litany_rubric"), "rubric"))
	return b.Section("", "prayers", lines, nil)
}

func (b *loc1928) additionalPrayers(prefix string, slugs []string) *Section {
	if b.includeLitany() {
		return nil
	}
	lines := b.plain(nil, prefix+"_additional_prayers_rubric", "rubric")
	for _, slug := range slugs {
		t := b.T(slug)
		if t == nil {
			continue
		}
		if t.Title != nil {
			lines = append(lines, b.I(*t.Title, "heading"))
		}
		lines = append(lines, b.I(t.Content, "text"))
	}
	return b.Section(b.label("additional_prayers"), "additional_prayers", lines, nil)
}

// litany1928 ports the per-variant build_litany: the Lord's Prayer slug, the
// "let us pray" line, whether the text after the Lord's Prayer is always
// emitted, and the section name.
func (b *loc1928) litany1928() *Section {
	if !b.includeLitany() {
		return nil
	}
	evening := b.OfficeType == "evening"
	lines := b.plain(nil, "litany_rubric", "rubric")
	for i := 1; i <= 5; i++ {
		lines = b.plain(lines, fmt.Sprintf("litany_part_%d", i), "responsive")
	}
	lines = b.plain(lines, "litany_lords_prayer_rubric", "rubric")
	lpSlug := b.OfficeType + "_lords_prayer"
	if b.rec() {
		lpSlug = "litany_lords_prayer"
		if b.recContemporary() {
			lpSlug = "litany_lords_prayer_contemporary"
		}
	}
	lines = b.plain(lines, lpSlug, "congregation")
	// Loc1928En::Evening and Loc1928Pt::Evening emit this line even when the
	// text is missing; the morning offices and REC only when it exists.
	if evening && !b.rec() {
		lines = append(lines, b.I(b.contentOrNil("litany_after_lords_prayer"), "responsive"))
	} else {
		lines = b.plain(lines, "litany_after_lords_prayer", "responsive")
	}
	var letUsPray string
	switch {
	case b.rec():
		letUsPray = "litany_let_us_pray"
	case b.variant == v1928Pt:
		letUsPray = b.OfficeType + "_lesser_litany_rubric"
	case evening:
		letUsPray = "litany_let_us_pray"
	default:
		letUsPray = "loc1662_phrase_let_us_pray"
	}
	lines = append(lines,
		b.FetchLineItem(letUsPray, "rubric", nil, "content"),
		b.I(b.contentOrNil("litany_prayer_1"), "leader"),
		b.I(b.contentOrNil("litany_prayer_2"), "leader"),
		b.I(b.contentOrNil("litany_gloria_patri"), "responsive"),
		b.I(b.contentOrNil("litany_final_prayer"), "leader"))
	name := "The Litany"
	if !evening || b.rec() || b.variant == v1928Pt {
		name = b.label("litany")
	}
	return b.Section(name, "litany", compactLines(lines), nil)
}

func (b *loc1928) creed1928(prefix string) *Section {
	lines := b.plain(nil, prefix+"_creed_rubric", "rubric")
	slug := Or(b.ResolveOptions(prefix+"_creed", []string{"apostles_creed", "nicene_creed"}), "apostles_creed")
	if t := b.tAny(slug); t != nil {
		lines = append(lines, b.I(t.Content, "text"))
	}
	return b.Section("", "creed", lines, nil)
}

// --- Morning -----------------------------------------------------------------------------

func (b *loc1928) morning() []*Section {
	const p = "morning"
	openingHi := 25
	if b.rec() {
		openingHi = 33
	}
	return Pipeline(
		One(func() *Section { return b.openingSentence1928(p, openingHi) }),
		One(func() *Section {
			if b.omitPenitentialOpening() {
				return nil
			}
			return b.exhortation(p)
		}),
		One(func() *Section {
			if b.omitPenitentialOpening() {
				return nil
			}
			return b.confession(p)
		}),
		One(func() *Section {
			if b.omitPenitentialOpening() {
				return nil
			}
			return b.absolution(p)
		}),
		One(func() *Section { return b.lordsPrayer1928(p) }),
		One(func() *Section { return b.versicles1928(p) }),
		One(b.mInvitatory1928),
		One(func() *Section { return b.psalms("morning_psalms") }),
		One(func() *Section { return b.reading1928("first", "morning_first_reading_rubric") }),
		One(b.mFirstCanticle1928),
		One(func() *Section { return b.reading1928("second", "morning_second_reading_rubric") }),
		One(func() *Section {
			return b.canticle(Or(b.ResolveOptions("morning_second_canticle", []string{"morning_benedictus", "morning_jubilate"}), "morning_benedictus"), "second_canticle")
		}),
		One(b.mCreed1928),
		One(func() *Section { return b.prayers1928(p) }),
		One(func() *Section {
			lines := b.plain(nil, "morning_suffrages_rubric", "rubric")
			for i := 1; i <= 2; i++ {
				lines = append(lines,
					b.I(b.contentOrNil(fmt.Sprintf("morning_suff_v%d", i)), "leader"),
					b.I(b.contentOrNil(fmt.Sprintf("morning_suff_r%d", i)), "congregation"))
			}
			return b.Section("", "suffrages", lines, nil)
		}),
		One(func() *Section {
			return b.CollectOfDaySection(b.label("collect_of_the_day"), "morning_collects_rubric")
		}),
		One(func() *Section {
			return b.TextSection(b.label("fixed_collects"), "fixed_collects", []Entry{
				{Slug: "morning_collect_peace", Heading: true},
				{Slug: "morning_collect_grace", Heading: true},
			}, false)
		}),
		One(b.litany1928),
		One(b.mAdditionalPrayers1928),
		One(b.theGrace),
	)
}

func (b *loc1928) mAdditionalPrayers1928() *Section {
	civilAuthority := "morning_prayer_civil_authority_1"
	if b.variant == v1928Pt {
		v := Or(b.ResolveOptions("morning_civil_authority_prayer",
			[]string{"morning_prayer_civil_authority_1", "morning_prayer_civil_authority_2"}), "morning_prayer_civil_authority_1")
		if s, ok := v.(string); ok {
			civilAuthority = s
		} else {
			// An Array slug matches no text and is skipped.
			civilAuthority = ""
		}
	}
	return b.additionalPrayers("morning", []string{
		civilAuthority, "morning_prayer_clergy_people", "morning_prayer_all_conditions",
		"morning_general_thanksgiving", "prayer_st_chrysostom",
	})
}

func (b *loc1928) mInvitatory1928() *Section {
	if !b.rec() && b.easterOctave() {
		var lines []*Line
		lines = b.plain(lines, "morning_easter_venite_rubric", "rubric")
		c := b.T("morning_christ_our_passover")
		if c != nil {
			lines = append(lines, b.I(c.Content, "text"))
		}
		var name any = "Christ our Passover"
		if c != nil && c.Title != nil {
			name = *c.Title
		}
		return b.Section(name, "invitatory_canticle", lines, nil)
	}
	lines := b.plain(nil, "morning_venite_rubric", "rubric")
	if a := b.officialVeniteAntiphon(); a != nil {
		lines = append(lines, b.Item(a.Content, "antiphon", a.Slug, ""))
	}
	choice := Or(b.ResolveOptions("morning_invitatory_canticle", []string{"venite", "psalm_95"}), "venite")
	slug := "morning_venite"
	if s, ok := choice.(string); ok && s == "psalm_95" {
		slug = "morning_psalm_95"
	}
	c := b.T(slug)
	if c == nil {
		return nil
	}
	lines = append(lines, b.I(c.Content, "text"))
	lines = b.plain(lines, "morning_gloria_patri_v", "leader")
	lines = b.plain(lines, "morning_gloria_patri_r", "congregation")
	return b.Section(Title(c), "invitatory_canticle", lines, nil)
}

func (b *loc1928) movable(key string) civil.Date { return b.Calendar().Easter.Dates[key] }

func (b *loc1928) easterOctave() bool {
	e := b.Calendar().Easter.EasterDate
	return b.Date >= e && b.Date <= e.Add(6)
}

func (b *loc1928) adventSeason() bool {
	first := b.movable("first_sunday_of_advent")
	return b.Date.IsSunday() && b.Date >= first && b.Date < civil.MustNew(b.Date.Year(), 12, 25)
}

func (b *loc1928) christmasOctave() bool {
	m, d := b.Date.Month(), b.Date.Day()
	return (m == 12 && d >= 25) || (m == 1 && d <= 5)
}

func (b *loc1928) epiphanyOctave() bool {
	y := b.Date.Year()
	return b.Date >= civil.MustNew(y, 1, 6) && b.Date <= civil.MustNew(y, 1, 13)
}

func (b *loc1928) saintsDayWithPropers() bool {
	c, _ := b.DayInfo.Get("celebration").(*rb.Map)
	if c == nil || c.Len() == 0 || b.Date.IsSunday() {
		return false
	}
	pt := rb.ToS(c.Get("person_type"))
	return pt == "singular" || pt == "plural"
}

func (b *loc1928) officialVeniteAntiphon() *store.LiturgicalText {
	easter := b.Calendar().Easter.EasterDate
	ascension, pentecost := b.movable("ascension"), b.movable("pentecost")
	d := b.Date
	m, day := d.Month(), d.Day()
	var key string
	switch {
	case d.IsSunday() && b.adventSeason():
		key = "advent"
	case b.christmasOctave():
		key = "christmas"
	case b.epiphanyOctave() || (m == 8 && day == 6):
		key = "epiphany"
	case d > easter && d < ascension:
		key = "easter"
	case d >= ascension && d < pentecost:
		key = "ascension"
	case d >= pentecost && d <= pentecost.Add(6):
		key = "whitsunday"
	case d == pentecost.Add(7):
		key = "trinity"
	case (m == 2 && day == 2) || (m == 3 && day == 25):
		key = "incarnation"
	case b.saintsDayWithPropers():
		key = "saints"
	default:
		return nil
	}
	return b.T("morning_venite_antiphon_" + key)
}

func (b *loc1928) mFirstCanticle1928() *Section {
	slug := Or(b.ResolveOptions("morning_first_canticle",
		[]string{"morning_te_deum", "morning_benedictus_es", "morning_benedicite"}), "morning_te_deum")
	if b.rec() && b.recLanguageStyle() == "modern" {
		variant := Or(b.ResolveOptions("rec_modern_morning_canticle",
			[]string{"default", "benedicite_1", "benedicite_2", "magnificate"}), "default")
		if s, ok := variant.(string); ok {
			switch s {
			case "benedicite_1":
				slug = "morning_benedicite_1"
			case "benedicite_2":
				slug = "morning_benedicite_2"
			case "magnificate":
				slug = "morning_magnificate"
			}
		}
	}
	return b.canticle(slug, "first_canticle")
}

var recAthanasianDays = map[string]bool{
	"Christmas Day": true, "Epiphany": true, "Saint Matthias the Apostle": true, "Easter Day": true,
	"Ascension Day": true, "Whitsunday": true, "Nativity of Saint John Baptist": true,
	"Saint James the Apostle": true, "Saint Bartholomew the Apostle": true, "Saint Matthew the Apostle": true,
	"Saints Simon and Jude": true, "Saint Andrew the Apostle": true, "Trinity Sunday": true,
}

func (b *loc1928) mCreed1928() *Section {
	if !b.rec() {
		return b.creed1928("morning")
	}
	var slug any = Or(b.ResolveOptions("morning_creed", []string{"apostles_creed", "nicene_creed", "athanasian_creed"}), "apostles_creed")
	name, _ := b.celebrationField("name").(string)
	if slug == "athanasian_creed" && !recAthanasianDays[name] {
		slug = "apostles_creed"
	}
	rubric := "morning_creed_rubric"
	if slug == "athanasian_creed" {
		rubric = "athanasian_creed_rubric"
	}
	lines := b.plain(nil, rubric, "rubric")
	if t := b.tAny(slug); t != nil {
		lines = append(lines, b.I(t.Content, "text"))
	}
	creedName := ""
	if slug == "athanasian_creed" {
		creedName = b.label("athanasian_creed")
	}
	return b.Section(creedName, "creed", lines, nil)
}

// --- Evening -----------------------------------------------------------------------------

func (b *loc1928) evening() []*Section {
	const p = "evening"
	openingHi := 18
	if b.rec() {
		openingHi = 22
	}
	return Pipeline(
		One(func() *Section { return b.openingSentence1928(p, openingHi) }),
		One(func() *Section {
			if b.omitPenitentialOpening() {
				return nil
			}
			return b.exhortation(p)
		}),
		One(func() *Section {
			if b.omitPenitentialOpening() {
				return nil
			}
			return b.confession(p)
		}),
		One(b.eAbsolution1928),
		One(func() *Section { return b.lordsPrayer1928(p) }),
		One(func() *Section { return b.versicles1928(p) }),
		One(b.ePsalms1928),
		One(func() *Section {
			if b.eveningPsalmConclusion() != "gloria_in_excelsis" {
				return nil
			}
			return b.canticle("evening_gloria_in_excelsis", "gloria_in_excelsis")
		}),
		One(func() *Section {
			if b.eveningLessonSelection() == "second_only" {
				return nil
			}
			return b.reading1928("first", "evening_first_reading_rubric")
		}),
		One(func() *Section {
			if b.eveningLessonSelection() == "second_only" {
				return nil
			}
			return b.canticle(Or(b.ResolveOptions("evening_first_canticle",
				[]string{"evening_magnificat", "evening_cantate_domino", "evening_bonum_est_confiteri"}), "evening_magnificat"), "first_canticle")
		}),
		One(func() *Section {
			if b.eveningLessonSelection() == "first_only" {
				return nil
			}
			return b.reading1928("second", "evening_second_reading_rubric")
		}),
		One(func() *Section {
			if b.eveningLessonSelection() == "first_only" {
				return nil
			}
			return b.canticle(Or(b.ResolveOptions("evening_second_canticle",
				[]string{"evening_nunc_dimittis", "evening_deus_misereatur", "evening_benedic_anima_mea"}), "evening_nunc_dimittis"), "second_canticle")
		}),
		One(func() *Section { return b.creed1928(p) }),
		One(func() *Section { return b.prayers1928(p) }),
		One(func() *Section { return b.suffrages(p) }),
		One(func() *Section {
			return b.CollectOfDaySection(b.label("collect_of_the_day"), "evening_collects_rubric")
		}),
		One(func() *Section {
			lines := b.titled(nil, "evening_collect_peace", "leader")
			lines = b.titled(lines, "evening_collect_perils", "leader")
			return b.Section("", "fixed_collects", lines, nil)
		}),
		One(b.litany1928),
		One(func() *Section {
			return b.additionalPrayers("evening", []string{
				"evening_prayer_civil_authority_1", "evening_prayer_clergy_people", "evening_prayer_all_conditions",
				"evening_general_thanksgiving", "prayer_st_chrysostom_evening",
			})
		}),
		One(b.theGrace),
	)
}

func (b *loc1928) eAbsolution1928() *Section {
	if b.omitPenitentialOpening() {
		return nil
	}
	lines := b.plain(nil, "evening_absolution_rubric", "rubric")
	var slug any = "evening_absolution"
	if !b.rec() {
		slug = Or(b.ResolveOptions("evening_absolution_form",
			[]string{"evening_absolution", "evening_absolution_alternative"}), "evening_absolution")
	}
	if t := b.tAny(slug); t != nil {
		lines = append(lines, b.I(t.Content, "text"))
	}
	return b.Section("", "absolution", lines, nil)
}

func (b *loc1928) eveningPsalmConclusion() any {
	return Or(b.ResolveOptions("evening_psalm_conclusion", []string{"gloria_patri", "gloria_in_excelsis"}), "gloria_patri")
}

func (b *loc1928) eveningLessonSelection() any {
	return Or(b.ResolveOptions("evening_lesson_selection", []string{"both", "first_only", "second_only"}), "both")
}

func (b *loc1928) ePsalms1928() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.I(p.Reference, "heading")}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	if b.eveningPsalmConclusion() == "gloria_patri" {
		lines = b.plain(lines, "evening_gloria_patri_v", "leader")
		lines = b.plain(lines, "evening_gloria_patri_r", "congregation")
	}
	return b.Section("", "psalms", lines, nil)
}
