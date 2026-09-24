package dailyoffice

import (
	"context"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rx"
)

func init() {
	for _, code := range []string{"awrv_2025_en", "awrv_2025_pt"} {
		pt := code == "awrv_2025_pt"
		Register(code, func(ctx context.Context, c *Context) Builder {
			switch c.OfficeType {
			case "morning", "prime", "terce", "midday", "none", "evening", "compline":
			default:
				UnknownOfficeType(c.OfficeType)
			}
			b := &awrv{pt: pt}
			if pt {
				b.H.BuildSection = func(name any, slug string, lines []*Line, meta *rb.Map) *Section {
					n := rb.ToS(name)
					if t, ok := awrvPtSectionNames[n]; ok {
						n = t
					}
					return NewSection(n, slug, lines, meta)
				}
				b.H.LineItem = func(text any, typ, slug, reference string) *Line {
					if s, ok := text.(string); ok {
						text = awrvPsalmWordRe.Sub(s, "Salmo")
					}
					return BaseItem(text, typ, slug, reference)
				}
			}
			b.Init(ctx, c)
			return b
		})
	}
}

// awrv ports DailyOffice::Builders::Awrv2025En::* (the 2025 Proposed Book
// of Common Prayer) and the Awrv2025Pt localization of the same offices.
type awrv struct {
	Base
	pt           bool
	bookSeason   string
	dividedSeas  string
	rankLoaded   bool
	rank         string
	psalmContent map[string]*rb.Map
}

func (b *awrv) Call() *rb.Map {
	switch b.OfficeType {
	case "morning":
		return b.Render(b.morning())
	case "evening":
		return b.Render(b.evening())
	case "compline":
		return b.Render(b.compline())
	}
	return b.Render(b.littleHour())
}

var (
	awrvPtSectionNames = map[string]string{
		"The Fore-Office": "O Ofício Preparatório", "The Lord's Prayer & Angelic Salutation": "A Oração do Senhor e a Saudação Angélica",
		"The Psalms": "Os Salmos", "The First Lesson": "A Primeira Leitura", "The Second Lesson": "A Segunda Leitura",
		"The Creed": "O Credo", "The Collect of the Day": "A Coleta do Dia", "The Conclusion": "A Conclusão",
		"After the Third Collect": "Depois da Terceira Coleta", "The Grace": "A Graça",
		"Venite, exultemus Domino": "Vinde, exultemos ao Senhor", "The Lord's Prayer": "A Oração do Senhor",
		"Preces": "Preces", "The Chapter": "O Capítulo", "Short Respond": "Breve Responsório",
		"The Office Hymn": "Hino do Ofício", "Nunc Dimittis": "Nunc dimittis", "Suffrages": "Sufrágios",
		"Confession": "Confissão", "The Collect": "A Coleta", "The Marian Anthem": "A Antífona Mariana",
		"The Collects": "As Coletas", "Pretiosa": "Pretiosa",
	}
	awrvPsalmWordRe = rx.MustCompile(`\APsalms?\b`)
	awrvFeastRank   = rx.MustCompile(`(?i)\b(?:semi)?double\b`)
	awrvOurLordEn   = rx.MustCompile(`Our Lord|Blessed Virgin Mary|Our Lady`)
	awrvOurLordPt   = rx.MustCompile(`Nosso Senhor|Nossa Senhora|Virgem Maria`)
)

// --- Base --------------------------------------------------------------------------------

func (b *awrv) movable() liturgical.Movable { return b.Calendar().Easter.Dates }

func (b *awrv) seasons() *liturgical.BookSeasonResolver {
	return liturgical.NewBookSeasonResolver(b.movable())
}

// season ports book_season: the Triduum keeps saying Passiontide's texts.
func (b *awrv) season() string {
	if b.bookSeason == "" {
		s := b.seasons().Resolve(b.Date)
		if s == "triduum" {
			s = "passiontide"
		}
		b.bookSeason = s
	}
	return b.bookSeason
}

func (b *awrv) dividedSeason() string {
	if b.dividedSeas == "" {
		b.dividedSeas = b.seasons().TextPeriod(b.Date)
	}
	return b.dividedSeas
}

func (b *awrv) epiphanyOctave() bool {
	return b.Date.Month() == 1 && b.Date.Day() >= 6 && b.Date.Day() <= 13
}

func (b *awrv) celebrationRank() string {
	if b.rankLoaded {
		return b.rank
	}
	b.rankLoaded = true
	id := b.celebrationField("id")
	if id == nil {
		return ""
	}
	var latin *string
	err := db.Q().QueryRow(b.Ctx, `SELECT "celebrations"."latin_name" FROM "celebrations" WHERE "celebrations"."id" = $1 LIMIT 1`, id).Scan(&latin)
	if err != nil && !db.NoRows(err) {
		panic(err)
	}
	if latin != nil {
		b.rank = *latin
	}
	return b.rank
}

// ferial: the book's Ranking of Days (Double and Semidouble ranks are Feasts).
func (b *awrv) ferial() bool {
	if b.Date.IsSunday() {
		return false
	}
	return !awrvFeastRank.MatchString(b.celebrationRank())
}

func (b *awrv) festal() bool { return !b.ferial() }

// --- GloriaPatri -------------------------------------------------------------------------

func (b *awrv) sacredTriduum() bool {
	e := b.movable()["easter"]
	return b.Date.Between(e.Add(-3), e.Add(-1))
}

func (b *awrv) gloriaSilenced(passiontideGloria bool) bool {
	return b.sacredTriduum() || (!passiontideGloria && b.season() == "passiontide")
}

func (b *awrv) gloriaLines(slug string, passiontideGloria bool) []*Line {
	if b.gloriaSilenced(passiontideGloria) {
		return nil
	}
	return b.TextSectionLines(Entry{Slug: slug, Type: "responsive"})
}

func (b *awrv) linesWithInnerGloria(slug string, passiontideGloria bool) []*Line {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	content := t.Content
	if b.gloriaSilenced(passiontideGloria) {
		content = withoutGloriaVerse(content)
	}
	return []*Line{b.I(content, "responsive")}
}

// withoutGloriaVerse drops the Gloria verse and the response that answers it.
func withoutGloriaVerse(content string) string {
	rows := rubyLines(content)
	idx := -1
	for i, r := range rows {
		if strings.HasPrefix(r, "℣. Glory be to the Father") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return content
	}
	rows = append(rows[:idx], rows[idx+1:]...)
	if idx < len(rows) && strings.HasPrefix(rows[idx], "℟.") {
		rows = append(rows[:idx], rows[idx+1:]...)
	}
	return strings.Join(rows, "")
}

// --- Fore-Office, psalms, lessons, collects ----------------------------------------------

func (b *awrv) openingVersicles(slug string) *Section {
	lines := b.linesWithInnerGloria(slug, true)
	if len(lines) == 0 {
		return nil
	}
	return b.Section("", "versicles", lines, nil)
}

func (b *awrv) foreOffice() *Section {
	lines := b.TextSectionLines(Entry{Slug: "fore_office_rubric", Type: "rubric"})
	sentences := b.TextsMatching("opening_sentence_" + b.openingSentenceGroup() + "_")
	if len(sentences) > 0 {
		s := sentences[b.SeededPick(len(sentences), "opening_sentence")]
		lines = append(lines, b.I(s.Content, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return b.Section("The Fore-Office", "opening_sentence", lines, nil)
}

var awrvSentenceGroups = []string{"seasonal", "general", "morning", "evening", "advent", "christmastide", "epiphanytide",
	"septuagesimatide", "lent", "passiontide", "eastertide", "ascensiontide", "whitsuntide", "trinity_sunday"}

func (b *awrv) openingSentenceGroup() string {
	choice := Or(b.ResolveOptions("opening_sentence_group", awrvSentenceGroups), "seasonal")
	if choice != "seasonal" {
		return rubyInterp(choice)
	}
	if len(b.TextsMatching("opening_sentence_"+b.dividedSeason()+"_")) > 0 {
		return b.dividedSeason()
	}
	if b.Date == b.movable()["trinity_sunday"] {
		return "trinity_sunday"
	}
	return "general"
}

func (b *awrv) penitentialRite() *Section {
	if !rb.Truthy(b.DayInfo.Get("fast_day")) {
		if v, ok := b.ResolveOptions("penitential_opening", []string{"full", "omit"}).(string); ok && v == "omit" {
			return nil
		}
	}
	var entries []Entry
	switch b.ResolveOptions("exhortation_form", []string{"long", "short", "bidding"}) {
	case "short":
		entries = []Entry{{Slug: "exhortation_to_penitence_short"}}
	case "bidding":
		entries = []Entry{{Slug: "exhortation_omitted_bidding", Type: "rubric"}}
	default:
		entries = []Entry{{Slug: "exhortation_to_penitence_long"}}
	}
	absolution := "absolution_priest"
	switch b.ResolveOptions("absolution_form", []string{"priest", "priest_alternative", "minister"}) {
	case "priest_alternative":
		absolution = "absolution_priest_alternative"
	case "minister":
		absolution = "absolution_minister"
	}
	entries = append(entries,
		Entry{Slug: "general_confession_rubric", Type: "rubric"},
		Entry{Slug: "general_confession", Type: "congregation"},
		Entry{Slug: absolution, Type: "leader", Heading: true})
	return b.TextSection("", "confession", entries, true)
}

func (b *awrv) foreOfficePrayers() *Section {
	return b.TextSection("The Lord's Prayer & Angelic Salutation", "lords_prayer", []Entry{
		{Slug: "lords_prayer_with_doxology", Type: "congregation"},
		{Slug: "angelic_salutation", Type: "congregation"},
	}, true)
}

func (b *awrv) psalms(gloriaSlug string) *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.I(p.Reference, "heading")}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	lines = append(lines, b.gloriaLines(gloriaSlug, true)...)
	return b.Section("The Psalms", "psalms", lines, nil)
}

func (b *awrv) lesson(typ, name string) *Section {
	return b.ReadingModule(typ, "", Rubrics{}, typ+"_reading", name, true)
}

func (b *awrv) creed() *Section {
	slug := Or(b.ResolveOptions("creed", []string{"apostles_creed", "nicene_creed"}), "apostles_creed")
	return b.TextSection("The Creed", "creed", []Entry{{Slug: rubyInterp(slug), Type: "congregation"}}, true)
}

func (b *awrv) collectOfTheDay() *Section {
	lines := b.CollectLines(b.Collects)
	if len(lines) == 0 {
		return nil
	}
	return b.Section("The Collect of the Day", "collect_of_the_day", lines, nil)
}

func (b *awrv) conclusion(slug string) *Section {
	return b.TextSection("The Conclusion", "conclusion", []Entry{{Slug: slug, Type: "responsive"}}, true)
}

func (b *awrv) afterTheThirdCollect(key string) *Section {
	if v, ok := b.ResolveOptions(key, []string{"full", "grace_only"}).(string); ok && v == "grace_only" {
		return b.TextSection("The Grace", "additional_prayers", []Entry{{Slug: "the_grace"}}, true)
	}
	return b.TextSection("After the Third Collect", "additional_prayers", []Entry{
		{Slug: "after_the_third_collect_rubric", Type: "rubric"},
		{Slug: "prayer_for_clergy_and_people", Heading: true},
		{Slug: "prayer_for_all_conditions", Heading: true},
		{Slug: "general_thanksgiving", Heading: true},
		{Slug: "prayer_of_saint_chrysostom", Heading: true},
		{Slug: "the_grace", Heading: true},
	}, true)
}

func (b *awrv) officePrayers() []Step {
	return []Step{
		One(func() *Section {
			return b.TextSection("", "prayers", []Entry{
				{Slug: "salutation", Type: "responsive"}, {Slug: "kyrie", Type: "responsive"},
			}, true)
		}),
		One(func() *Section {
			return b.TextSection("The Lord's Prayer", "lords_prayer_office", []Entry{{Slug: "lords_prayer_traditional", Type: "congregation"}}, true)
		}),
		One(func() *Section {
			return b.TextSection("Preces", "suffrages", []Entry{{Slug: "preces", Type: "responsive"}}, true)
		}),
		One(b.collectOfTheDay),
	}
}

// --- Mattins -----------------------------------------------------------------------------

func (b *awrv) morning() []*Section {
	steps := []Step{
		One(b.foreOffice), One(b.penitentialRite), One(b.foreOfficePrayers),
		One(func() *Section { return b.openingVersicles("opening_versicles") }),
		One(b.invitatory),
		One(func() *Section { return b.psalms("gloria_patri") }),
		One(func() *Section { return b.lesson("first", "The First Lesson") }),
		One(b.mCanticle),
		One(func() *Section { return b.lesson("second", "The Second Lesson") }),
		One(func() *Section {
			lines := b.TextSectionLines(Entry{Slug: "benedictus"})
			if len(lines) == 0 {
				return nil
			}
			lines = append(lines, b.gloriaLines("gloria_patri", true)...)
			return b.Section("Benedictus", "second_canticle", lines, nil)
		}),
		One(b.creed),
	}
	steps = append(steps, b.officePrayers()...)
	steps = append(steps,
		One(func() *Section {
			return b.TextSection("", "fixed_collects", []Entry{
				{Slug: "morning_collect_for_peace", Heading: true}, {Slug: "morning_collect_for_grace", Heading: true},
			}, true)
		}),
		One(func() *Section { return b.conclusion("office_conclusion") }),
		One(func() *Section { return b.afterTheThirdCollect("morning_after_third_collect") }),
	)
	return Pipeline(steps...)
}

var awrvAntiphonSeasons = map[string]bool{"advent": true, "christmastide": true, "epiphanytide": true, "septuagesimatide": true,
	"lent": true, "passiontide": true, "eastertide": true, "ascensiontide": true, "trinitytide": true}

func (b *awrv) invitatory() *Section {
	lines := b.TextSectionLines(Entry{Slug: "invitatory_rubric", Type: "rubric"})
	antiphon := b.invitatoryAntiphon()
	if antiphon != "" {
		lines = append(lines, b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})...)
	}
	lines = append(lines, b.TextSectionLines(Entry{Slug: "venite"})...)
	if antiphon != "" {
		lines = append(lines, b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})...)
	}
	lines = append(lines, b.gloriaLines("gloria_patri", false)...)
	return b.Section("Venite, exultemus Domino", "invitatory_canticle", lines, nil)
}

func (b *awrv) invitatoryAntiphon() string {
	if v, ok := b.ResolveOptions("morning_invitatory_antiphon", []string{"appointed", "omit"}).(string); ok && v == "omit" {
		return ""
	}
	if s := b.dividedSeason(); awrvAntiphonSeasons[s] {
		return "invitatory_antiphon_" + s
	}
	name := rb.ToS(b.celebrationField("name"))
	re := awrvOurLordEn
	if b.pt {
		re = awrvOurLordPt
	}
	if re.MatchString(name) {
		return "invitatory_antiphon_feasts_of_our_lord_and_our_lady"
	}
	if b.festal() && !b.Date.IsSunday() {
		return "invitatory_antiphon_other_feasts"
	}
	return ""
}

func penitentialSeason(s string) bool {
	return s == "advent" || s == "septuagesimatide" || s == "lent" || s == "passiontide"
}

func (b *awrv) mCanticle() *Section {
	var slug string
	switch b.ResolveOptions("morning_canticle", []string{"appointed", "te_deum", "benedicite", "old_testament_canticle"}) {
	case "te_deum":
		slug = "te_deum"
	case "benedicite", "old_testament_canticle":
		slug = "benedicite"
	default:
		slug = "benedicite"
		if b.festal() && !penitentialSeason(b.season()) {
			slug = "te_deum"
		}
	}
	rubric := "benedicite_rubric"
	if slug == "te_deum" {
		rubric = "te_deum_rubric"
	}
	t := b.T(slug)
	if t == nil {
		return nil
	}
	lines := b.TextSectionLines(Entry{Slug: rubric, Type: "rubric"})
	lines = append(lines, b.I(t.Content, "text"))
	lines = append(lines, b.gloriaLines("gloria_patri", true)...)
	return b.Section(Title(t), "first_canticle", lines, nil)
}

// --- Evensong ----------------------------------------------------------------------------

func (b *awrv) evening() []*Section {
	canticle := func(slug, sectionSlug string) *Section {
		t := b.T(slug)
		if t == nil {
			return nil
		}
		lines := append([]*Line{b.I(t.Content, "text")}, b.gloriaLines("gloria_patri", true)...)
		return b.Section(Title(t), sectionSlug, lines, nil)
	}
	steps := []Step{
		One(b.foreOffice), One(b.penitentialRite), One(b.foreOfficePrayers),
		One(func() *Section { return b.openingVersicles("opening_versicles") }),
		One(func() *Section {
			gloria := "gloria_patri"
			if v, ok := b.ResolveOptions("evening_final_gloria", []string{"gloria_patri", "gloria_in_excelsis"}).(string); ok &&
				v == "gloria_in_excelsis" && !penitentialSeason(b.season()) {
				gloria = "gloria_in_excelsis"
			}
			return b.psalms(gloria)
		}),
		One(func() *Section { return b.lesson("first", "The First Lesson") }),
		One(func() *Section { return canticle("magnificat", "first_canticle") }),
		One(func() *Section { return b.lesson("second", "The Second Lesson") }),
		One(func() *Section { return canticle("nunc_dimittis", "second_canticle") }),
		One(b.creed),
	}
	steps = append(steps, b.officePrayers()...)
	steps = append(steps,
		One(func() *Section {
			return b.TextSection("", "fixed_collects", []Entry{
				{Slug: "evening_collect_for_peace", Heading: true}, {Slug: "evening_collect_for_aid", Heading: true},
			}, true)
		}),
		One(func() *Section { return b.conclusion("office_conclusion") }),
		One(func() *Section {
			v := b.Pref("evening_marian_anthem")
			if !(v == true || (v == nil && b.PreferenceDefault("evening_marian_anthem") == true)) {
				return nil
			}
			return b.TextSection("The Marian Anthem", "marian_anthem", []Entry{{Slug: "marian_anthem_rubric", Type: "rubric"}}, true)
		}),
		One(func() *Section { return b.afterTheThirdCollect("evening_after_third_collect") }),
	)
	return Pipeline(steps...)
}

// --- The Little Hours --------------------------------------------------------------------

var awrvHourPsalms = map[string][]string{
	"prime": {"Psalm 54", "Psalm 119:1-8", "Psalm 119:9-16", "Psalm 119:17-24", "Psalm 119:25-32"},
	"terce": {"Psalm 119:33-40", "Psalm 119:41-48", "Psalm 119:49-56", "Psalm 119:57-64", "Psalm 119:65-72", "Psalm 119:73-80"},
	"sext":  {"Psalm 119:81-88", "Psalm 119:89-96", "Psalm 119:97-104", "Psalm 119:105-112", "Psalm 119:113-120", "Psalm 119:121-128"},
	"none":  {"Psalm 119:129-136", "Psalm 119:137-144", "Psalm 119:145-152", "Psalm 119:153-160", "Psalm 119:161-168", "Psalm 119:169-176"},
}

func (b *awrv) hour() string {
	if b.OfficeType == "midday" {
		return "sext"
	}
	return b.OfficeType
}

func (b *awrv) littleHour() []*Section {
	h := b.hour()
	prime := h == "prime"
	var steps []Step
	if prime {
		steps = append(steps, One(func() *Section {
			return b.TextSection("", "introduction", []Entry{{Slug: "prime_rubric", Type: "rubric"}}, true)
		}))
	}
	steps = append(steps,
		One(func() *Section { return b.openingVersicles("hour_opening_versicles") }),
		One(func() *Section {
			t := b.T(h + "_hymn")
			if t == nil {
				return nil
			}
			return b.Section(Title(t), "hymn", []*Line{b.I(t.Content, "text")}, nil)
		}),
		One(b.hourPsalmody))
	if prime {
		steps = append(steps, One(func() *Section {
			return b.TextSection("", "creed", []Entry{{Slug: "prime_athanasian_rubric", Type: "rubric"}}, true)
		}))
	}
	steps = append(steps,
		One(func() *Section {
			chapter := h + "_chapter_ferias"
			switch {
			case b.season() == "eastertide":
				chapter = h + "_chapter_eastertide"
			case b.festal():
				chapter = h + "_chapter_festal"
			}
			return b.TextSection("The Chapter", "chapter", []Entry{{Slug: chapter}, {Slug: "chapter_response", Type: "responsive"}}, true)
		}),
		One(func() *Section {
			slug := h + "_short_respond_ferias"
			switch {
			case b.T(h+"_short_respond") != nil:
				slug = h + "_short_respond"
			case b.festal() || b.season() == "eastertide":
				slug = h + "_short_respond_festal"
			}
			lines := b.linesWithInnerGloria(slug, false)
			if len(lines) == 0 {
				return nil
			}
			return b.Section("Short Respond", "short_respond", lines, nil)
		}),
		One(b.hourSuffrages),
		One(func() *Section {
			if prime {
				lines := b.TextSectionLines(Entry{Slug: "prime_collect"})
				lines = append(lines, b.CollectLines(b.Collects)...)
				return b.Section("The Collect", "collect_of_the_day", lines, nil)
			}
			lines := b.TextSectionLines(Entry{Slug: "salutation", Type: "responsive"})
			lines = append(lines, b.CollectLines(b.Collects)...)
			lines = append(lines, b.TextSectionLines(Entry{Slug: h + "_memorial_collect", Heading: true})...)
			return b.Section("The Collects", "collect_of_the_day", lines, nil)
		}),
		One(func() *Section { return b.conclusion(h + "_conclusion") }))
	if prime {
		steps = append(steps, One(func() *Section {
			v := b.PreferenceDefault("prime_pretiosa")
			if b.Prefs.Has("prime_pretiosa") {
				v = b.Pref("prime_pretiosa")
			}
			if !(v == true || rb.ToS(v) == "true") {
				return nil
			}
			return b.TextSection("Pretiosa", "pretiosa", []Entry{
				{Slug: "pretiosa_rubric", Type: "rubric"}, {Slug: "pretiosa", Type: "responsive"},
			}, true)
		}))
	}
	return Pipeline(steps...)
}

func (b *awrv) hourAntiphonSeason() string {
	if b.epiphanyOctave() {
		return "epiphany_octave"
	}
	s := b.season()
	switch s {
	case "advent", "christmastide", "septuagesimatide", "lent", "eastertide":
		return s
	case "passiontide":
		if b.hour() != "prime" {
			return s
		}
	}
	if b.festal() {
		return "festivals"
	}
	return "ferias"
}

func (b *awrv) fixedPsalmContents(refs []string) map[string]*rb.Map {
	if b.psalmContent == nil {
		b.psalmContent = (&reading.ContentLoader{
			Translation: b.PsalmScriptureTranslation(), PsalmTranslation: b.SelectedPsalmTranslation(),
		}).Load(b.Ctx, refs)
	}
	return b.psalmContent
}

func (b *awrv) fixedPsalmLines(ref string, contents map[string]*rb.Map) []*Line {
	lines := []*Line{b.I(ref, "heading")}
	if c := contents[ref]; c != nil && c.Len() > 0 {
		lines = append(lines, b.BibleContent(c)...)
	}
	return append(lines, b.gloriaLines("gloria_patri", true)...)
}

func (b *awrv) hourPsalmody() *Section {
	h := b.hour()
	antiphon := h + "_psalm_antiphon_" + b.hourAntiphonSeason()
	lines := b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})
	lines = append(lines, b.TextSectionLines(Entry{Slug: "little_hours_gloria_rubric", Type: "rubric"})...)
	contents := b.fixedPsalmContents(awrvHourPsalms[h])
	for _, ref := range awrvHourPsalms[h] {
		lines = append(lines, b.fixedPsalmLines(ref, contents)...)
	}
	lines = append(lines, b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})...)
	return b.Section("The Psalms", "psalms", lines, nil)
}

func (b *awrv) hourSuffrages() *Section {
	switch b.ResolveOptions("little_hours_suffrages", []string{"appointed", "always", "omit"}) {
	case "always":
	case "omit":
		return nil
	default:
		if !(b.ferial() && b.season() != "eastertide") {
			return nil
		}
	}
	h := b.hour()
	entries := []Entry{
		{Slug: "little_hours_suffrages_rubric", Type: "rubric"},
		{Slug: "little_hours_kyrie", Type: "responsive"},
		{Slug: "lords_prayer_traditional", Type: "congregation"},
		{Slug: h + "_suffrages_versicles", Type: "responsive"},
	}
	if h == "prime" {
		entries = append(entries,
			Entry{Slug: "compline_confession", Type: "congregation"},
			Entry{Slug: "compline_absolution_minister", Type: "leader"},
			Entry{Slug: "compline_absolution_priest", Type: "leader"},
			Entry{Slug: "prime_preces", Type: "responsive"})
	}
	return b.TextSection("Suffrages", "suffrages", entries, true)
}

// --- Compline ----------------------------------------------------------------------------

var awrvComplinePsalms = []string{"Psalm 4", "Psalm 31:1-6", "Psalm 91", "Psalm 134"}

func (b *awrv) compline() []*Section {
	return Pipeline(
		One(func() *Section {
			return b.TextSection("", "opening_sentence", []Entry{{Slug: "compline_opening", Type: "responsive"}}, true)
		}),
		One(func() *Section { return b.openingVersicles("compline_introduction") }),
		One(func() *Section {
			season := "throughout_the_year"
			switch s := b.season(); {
			case b.epiphanyOctave():
				season = "epiphany_octave"
			case s == "christmastide" || s == "lent" || s == "eastertide":
				season = s
			case s == "passiontide":
				season = "lent"
			}
			antiphon := "compline_psalm_antiphon_" + season
			lines := b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})
			contents := b.fixedPsalmContents(awrvComplinePsalms)
			for _, ref := range awrvComplinePsalms {
				lines = append(lines, b.fixedPsalmLines(ref, contents)...)
			}
			lines = append(lines, b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})...)
			return b.Section("The Psalms", "psalms", lines, nil)
		}),
		One(func() *Section {
			return b.TextSection("The Chapter", "chapter", []Entry{{Slug: "compline_chapter"}}, true)
		}),
		One(func() *Section {
			lines := b.linesWithInnerGloria("compline_short_respond", true)
			if len(lines) == 0 {
				return nil
			}
			return b.Section("Short Respond", "short_respond", lines, nil)
		}),
		One(func() *Section {
			var hymn string
			switch b.ResolveOptions("compline_hymn", []string{"appointed", "ferial", "festal"}) {
			case "ferial":
				hymn = "compline_hymn_ferial"
			case "festal":
				hymn = "compline_hymn_festal"
			default:
				switch s := b.season(); {
				case s == "eastertide":
					hymn = "compline_hymn_eastertide"
				case s == "lent" || s == "passiontide":
					hymn = "compline_hymn_lent"
				case b.festal():
					hymn = "compline_hymn_festal"
				default:
					hymn = "compline_hymn_ferial"
				}
			}
			return b.TextSection("The Office Hymn", "hymn", []Entry{
				{Slug: hymn, Heading: true}, {Slug: "compline_hymn_versicle", Type: "responsive"},
			}, true)
		}),
		One(func() *Section {
			season := "throughout_the_year"
			switch s := b.season(); {
			case b.epiphanyOctave():
				season = "epiphany_octave"
			case s == "advent" || s == "christmastide" || s == "lent" || s == "passiontide" || s == "eastertide":
				season = s
			}
			antiphon := "compline_nunc_antiphon_" + season
			lines := b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})
			lines = append(lines, b.TextSectionLines(Entry{Slug: "nunc_dimittis"})...)
			lines = append(lines, b.gloriaLines("gloria_patri", true)...)
			lines = append(lines, b.TextSectionLines(Entry{Slug: antiphon, Type: "antiphon"})...)
			return b.Section("Nunc Dimittis", "second_canticle", lines, nil)
		}),
		One(func() *Section {
			return b.TextSection("Suffrages", "suffrages", []Entry{
				{Slug: "compline_suffrages", Type: "responsive"}, {Slug: "lords_prayer_traditional", Type: "congregation"},
				{Slug: "apostles_creed", Type: "congregation"}, {Slug: "compline_versicles", Type: "responsive"},
			}, true)
		}),
		One(func() *Section {
			return b.TextSection("Confession", "confession", []Entry{
				{Slug: "compline_confession", Type: "congregation"},
				{Slug: "compline_absolution_minister", Type: "leader"},
				{Slug: "compline_absolution_priest", Type: "leader"},
			}, true)
		}),
		One(func() *Section {
			return b.TextSection("", "preces", []Entry{{Slug: "compline_preces", Type: "responsive"}}, true)
		}),
		One(func() *Section {
			slug := "compline_collect"
			if v, ok := b.ResolveOptions("compline_collect", []string{"visit", "lord_jesu_christ"}).(string); ok && v == "lord_jesu_christ" {
				slug = "compline_collect_alternative"
			}
			return b.TextSection("The Collect", "collect_of_the_day", []Entry{{Slug: slug}}, true)
		}),
		One(func() *Section { return b.conclusion("compline_conclusion") }),
		One(func() *Section {
			v := b.PreferenceDefault("compline_marian_anthem")
			if b.Prefs.Has("compline_marian_anthem") {
				v = b.Pref("compline_marian_anthem")
			}
			if !(v == true || rb.ToS(v) == "true") {
				return nil
			}
			return b.TextSection("The Marian Anthem", "marian_anthem", []Entry{{Slug: "compline_marian_anthem_rubric", Type: "rubric"}}, true)
		}),
	)
}
