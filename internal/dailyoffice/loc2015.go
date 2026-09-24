package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
)

func init() { Register("loc_2015", newLoc2015) }

// loc2015 ports DailyOffice::Builders::Loc2015::* (routing by office type).
type loc2015 struct{ Base }

func newLoc2015(ctx context.Context, c *Context) Builder {
	switch c.OfficeType {
	case "morning", "evening", "midday", "compline", "late_evening":
	default:
		UnknownOfficeType(c.OfficeType)
	}
	b := &loc2015{}
	b.Init(ctx, c)
	return b
}

func (b *loc2015) Call() *rb.Map {
	switch b.OfficeType {
	case "morning":
		return b.Render(b.morning())
	case "evening":
		return b.Render(b.evening())
	case "midday":
		return b.Render(b.midday())
	case "compline":
		return b.Render(b.compline())
	}
	return b.Render(b.lateEvening())
}

// textSection is the frequent "fetch text or skip; one line" section.
func (b *Base) textSection(textSlug, name, slug, typ string) *Section {
	t := b.T(textSlug)
	if t == nil {
		return nil
	}
	return b.Section(name, slug, []*Line{b.TextLine(t, typ)}, nil)
}

// rubricAndSpacer appends a rubric line and a spacer when the text exists.
func (b *Base) textWithSpacer(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.TextLine(t, typ), b.Spacer())
	}
	return lines
}

// textOnly appends one line when the text exists.
func (b *Base) textOnly(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.TextLine(t, typ))
	}
	return lines
}

// --- Base (family helpers) ---------------------------------------------------------------

func (b *loc2015) familyPsalmFromPref(key, fixedSlug, fixedName string) *Section {
	if b.PrefS(key) == "daily" {
		if p := b.Readings.Psalm; p != nil {
			var lines []*Line
			ref := p.Reference
			if !rb.BlankString(ref) {
				lines = append(lines, b.I(ref, "heading"))
			}
			lines = append(lines, b.Spacer())
			if p.Content != nil {
				lines = append(lines, b.BibleContent(p.Content)...)
			}
			name := ref
			if rb.BlankString(name) {
				name = "Salmo"
			}
			return b.Section(name, "psalms", lines, nil)
		}
	}
	return b.textSection(fixedSlug, fixedName, "psalms", "congregation")
}

func (b *loc2015) familyReadingFromPref(key, fixedPrefix string) []*Section {
	var result []*Section
	lectionary := true
	switch b.PrefS(key) {
	case "old_testament":
		result = compactSections(b.familyLectionaryModule(b.Readings.FirstReading, ""))
	case "new_testament":
		result = compactSections(b.familyLectionaryModule(b.Readings.SecondReading, ""))
	case "gospel":
		result = compactSections(b.familyLectionaryModule(b.Readings.Gospel, ""))
	case "vary":
		opts := []string{"first_reading", "second_reading", "gospel"}
		k := opts[b.SeededNumber(0, 2, key)]
		result = compactSections(b.familyLectionaryModule(b.Readings.Slot(k), ""))
	case "all":
		result = compactSections(
			b.familyLectionaryModule(b.Readings.FirstReading, "Antigo Testamento"),
			b.familyLectionaryModule(b.Readings.SecondReading, "Novo Testamento"),
			b.familyLectionaryModule(b.Readings.Gospel, "Evangelho"),
		)
	default:
		lectionary = false
	}
	if lectionary && len(result) > 0 {
		return result
	}
	raw := b.ResolveRange(key, 1, 3)
	num := 0
	if l, ok := raw.([]any); ok {
		if len(l) > 0 {
			num = rb.StringToI(rb.ToS(l[0]))
		}
	} else {
		num = rb.ToI(raw)
	}
	if num < 1 || num > 3 {
		num = 1
	}
	t := b.T(fmt.Sprintf("%s_%d", fixedPrefix, num))
	if t == nil {
		return nil
	}
	return []*Section{b.Section("Leituras", "reading", []*Line{
		b.TextLine(t, "leader"),
		b.FetchLineItem("reading_citation", "citation", [][2]string{{"reference", Ref(t)}}, "content"),
	}, nil)}
}

func compactSections(list ...*Section) []*Section {
	var out []*Section
	for _, s := range list {
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (b *loc2015) familyOptionalCollect(key string) *Section {
	if b.PrefS(key) != "yes" || len(b.Collects) == 0 {
		return nil
	}
	var lines []*Line
	for _, c := range b.Collects {
		if v := c.Get("preface"); rb.Present(v) {
			lines = append(lines, b.I(v, "leader"))
		}
		lines = append(lines, b.I(c.Get("text"), "leader"), b.Spacer())
	}
	return b.Section("Coleta", "daily_collect", lines, nil)
}

func (b *loc2015) familyLectionaryModule(r *reading.Passage, label string) *Section {
	if r == nil || rb.BlankString(r.Reference) {
		return nil
	}
	lines := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
	if r.Content != nil {
		lines = append(lines, b.BibleContent(r.Content)...)
	}
	if label == "" {
		label = "Leituras"
	}
	return b.Section(label, "reading", lines, nil)
}

func (b *loc2015) lordsPrayerSimple() *Section {
	return b.textSection("our_father", "Pai Nosso", "lords_prayer", "congregation")
}

// psalmsSection ports the LOC 2015 build_psalms (morning and evening).
func (b *loc2015) psalmsSection() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_gloria_patri", "rubric")
	lines = append(lines, b.I(p.Reference, "heading"), b.Spacer())
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
		lines = append(lines, b.Spacer())
	}
	lines = b.textOnly(lines, "gloria_patri", "all")
	return b.Section("Salmos", "psalms", lines, nil)
}

func (b *loc2015) canticleSections(slugs []any) []*Section {
	var out []*Section
	for _, s := range slugs {
		slug := rb.ToS(s)
		t := b.T(slug)
		if t == nil {
			continue
		}
		out = append(out, b.Section(NameWithRef(t, "Cântico"), slug, []*Line{b.TextLine(t, "congregation")}, nil))
	}
	return out
}

func (b *loc2015) absolution() *Section {
	if b.PrefS("use_priestly_absolution") != "yes" {
		return nil
	}
	a := b.T("absolution")
	if a == nil {
		return nil
	}
	lines := []*Line{b.TextLine(a, "leader"), b.Spacer()}
	lines = b.textOnly(lines, "rubric_post_absolution", "rubric")
	return b.Section("Absolvição", "absolution", lines, nil)
}

func (b *loc2015) firstReading() *Section {
	return b.ReadingModule("first", "first_reading", Rubrics{Pre: "rubric_first_reading", Post: "rubric_post_first_reading",
		Response: "canticle_post_first_reading", End: "rubric_end_first_reading"}, "first_reading", "Leituras da Palavra de Deus", false)
}

func (b *loc2015) secondReading() *Section {
	return b.ReadingModule("second", "second_reading", Rubrics{Pre: "rubric_second_reading", Post: "rubric_post_second_reading",
		Response: "canticle_post_second_reading", End: "rubric_end_second_reading"}, "second_reading", "Segunda Leitura", false)
}

func (b *loc2015) offertory() *Section {
	var lines []*Line
	if t := b.T("rubric_offertory"); t != nil {
		lines = append(lines, b.Spacer(), b.TextLine(t, "rubric"))
	}
	return b.Section("Ofertório", "offertory", lines, nil)
}

func (b *loc2015) collectOfTheDay() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_collect_of_the_day", "rubric")
	lines = append(lines, b.CollectLines(b.Collects)...)
	return b.Section("Coletas", "collect_of_the_day", lines, nil)
}

func (b *loc2015) chrysostom() *Section {
	return b.textSection("chrysostom_prayer", "Oração de São João Crisóstomo", "chrysostom", "leader")
}

func (b *loc2015) generalThanksgivingOpening() []*Line {
	var lines []*Line
	lines = b.textWithSpacer(lines, "opening_general_thanksgiving", "leader")
	return b.textWithSpacer(lines, "rubric_after_opening_general_thanksgiving", "rubric")
}

func (b *loc2015) invitatoryCanticle(slug string) *Section {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	return b.Section(NameWithRef(t, "Cântico"), "invitatory_canticle", []*Line{b.TextLine(t, "congregation")}, nil)
}

func (b *loc2015) endSecondCanticle(mods []*Section) []*Section {
	if t := b.T("rubric_end_second_canticle"); t != nil {
		mods = append(mods, b.Section("", "rubric_end_second_canticle", []*Line{b.TextLine(t, "rubric")}, nil))
	}
	return mods
}

func (b *loc2015) seasonOpeningSentenceSlug(prefix string) string {
	s := SeasonToOpeningSentenceSlug(b.Season(), b.FeastDay())
	if s == "" {
		return ""
	}
	return prefix + s
}

func (b *loc2015) invitatoryCanticleSlug(key string, options []string, fallback string) string {
	if lowerRuby(b.Season()) == "páscoa" {
		return "pascha_nostrum"
	}
	return rb.ToS(Or(b.ResolveOptions(key, options), fallback))
}

// --- Morning -----------------------------------------------------------------------------

var loc2015MorningCollects = []string{"for_peace", "for_grace", "for_all_authorities", "for_clergy", "for_parish_family", "for_all_humanity"}

func (b *loc2015) morning() []*Section {
	if b.PrefS("office_type") == "family" {
		return Pipeline(
			One(func() *Section { return b.textSection("family_morning_opening", "De Manhã", "opening", "leader") }),
			One(func() *Section {
				return b.familyPsalmFromPref("family_morning_psalm", "family_morning_psalm_8", "Domine, Dominus noster (Salmo 8)")
			}),
			Many(func() []*Section { return b.familyReadingFromPref("family_morning_reading", "family_morning_reading") }),
			One(func() *Section { return b.familyOptionalCollect("family_morning_collect") }),
			One(b.lordsPrayerSimple),
			One(func() *Section {
				return b.textSection("family_morning_collect", "Oração Conclusiva", "general_collects", "leader")
			}),
		)
	}
	return Pipeline(
		One(b.mWelcome), One(b.mOpeningSentence), One(b.mConfession), One(b.absolution), One(b.mInvitatory),
		One(b.mInvitatoryCanticle), One(b.psalmsSection), One(b.firstReading), Many(b.mFirstCanticle),
		One(b.secondReading), Many(b.mSecondCanticle), One(b.mCreed), One(b.offertory), One(b.mLordsPrayer),
		One(b.collectOfTheDay), One(b.mGeneralCollects), One(b.mGeneralThanksgiving), One(b.chrysostom),
		One(b.mDismissal),
	)
}

func (b *loc2015) mWelcome() *Section {
	slug := "morning_welcome_traditional"
	if b.PrefS("prayer_style") == "contemporary" {
		slug = "morning_welcome_contemporary"
	}
	return b.textSection(slug, "Acolhida", "welcome", "leader")
}

func (b *loc2015) mOpeningSentence() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "opening_sentence_rubric", "rubric")
	n := Or(b.ResolveRange("morning_opening_sentence", 1, 7), 1)
	if g := b.T("morning_opening_sentence_" + rb.ToS(n)); g != nil {
		lines = append(lines, b.TextLine(g, "leader"))
		if g.Reference != nil {
			lines = append(lines, b.I(*g.Reference, "citation"))
		}
		lines = append(lines, b.Spacer())
	}
	if slug := b.seasonOpeningSentenceSlug("morning_opening_sentence_"); slug != "" {
		if s := b.T(slug); s != nil {
			lines = append(lines, b.TextLine(s, "leader"))
			if s.Reference != nil {
				lines = append(lines, b.I(*s.Reference, "citation"))
			}
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section("Sentenças Iniciais", "opening_sentence", lines, nil)
}

func (b *loc2015) mConfession() *Section {
	var lines []*Line
	lines = b.textOnly(lines, "morning_rubric_confession", "rubric")
	lines = append(lines, b.Spacer())
	inv := b.ResolveRange("morning_confession_type", 1, 2)
	lines = b.textWithSpacer(lines, "morning_opening_confession_"+rubyInterp(inv), "leader")
	lines = b.textWithSpacer(lines, "rubric_post_opening_confession", "rubric")
	n := Or(b.ResolveRange("morning_confession_prayer_type", 1, 3), 1)
	lines = b.textWithSpacer(lines, "morning_confession_"+rubyInterp(n), "congregation")
	lines = b.textWithSpacer(lines, "rubric_post_confession", "rubric")
	if b.PrefS("use_priestly_absolution") != "yes" {
		p := b.ResolveRange("morning_prayer_after_confession", 1, 2)
		lines = b.textOnly(lines, "prayer_after_confession_"+rubyInterp(p), "congregation")
	}
	return b.Section("Confissão de Pecados", "confession", lines, nil)
}

func (b *loc2015) mInvitatory() *Section {
	slug := "morning_invocation"
	if b.IsLent() {
		slug = "morning_invocation_lent"
	}
	inv := b.T(slug)
	if inv == nil {
		rb.RaiseNoMethodOnNil("content")
	}
	lines := []*Line{b.TextLine(inv, "responsive")}
	if s := SeasonToAntiphonSlug(b.Season(), b.FeastDay()); s != "" {
		lines = b.textWithSpacer(lines, "morning_before_invocation_"+s, "leader")
	}
	return b.Section("Invitatório e Salmo", "invitatory", lines, nil)
}

func (b *loc2015) mInvitatoryCanticle() *Section {
	return b.invitatoryCanticle(b.invitatoryCanticleSlug("morning_invitatory_canticle", []string{"venite", "jubilate"}, "venite"))
}

func (b *loc2015) mFirstCanticle() []*Section {
	return b.canticleSections(Arr(b.ResolveOptions("morning_post_first_reading_canticle",
		[]string{"benedictus_es_domine", "cantate_domino", "benedicite_omnia_opera"})))
}

func (b *loc2015) mSecondCanticle() []*Section {
	mods := b.canticleSections(Arr(b.ResolveOptions("morning_post_second_reading_canticle",
		[]string{"te_deum_laudamus", "magna_et_mirabilia", "benedic_anima_mea"})))
	return b.endSecondCanticle(mods)
}

func (b *loc2015) mCreed() *Section {
	paraphrase := b.PrefS("morning_creed_type") == "apostolic_paraphrase"
	slug, name := "apostles_creed", "Credo Apostólico"
	if paraphrase {
		slug, name = "apostles_creed_paraphrase", "Paráfrase do Credo Apostólico"
	}
	c := b.T(slug)
	if c == nil {
		return nil
	}
	return b.Section(name, "creed", []*Line{b.TextLine(c, "congregation")}, nil)
}

func (b *loc2015) mLordsPrayer() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "morning_rubric_prayers", "rubric")
	for _, n := range Arr(Or(b.ResolveRange("morning_invocation", 1, 2), 2)) {
		lines = b.textWithSpacer(lines, "invocation_our_father_"+rubyInterp(n), "responsive")
	}
	lp := b.T("our_father")
	if lp == nil {
		return nil
	}
	lines = append(lines, b.TextLine(lp, "congregation"))
	lines = b.textWithSpacer(lines, "rubric_after_our_father", "rubric")
	p := b.ResolveRange("morning_post_lords_prayer_prayer", 1, 2)
	lines = b.textWithSpacer(lines, "mercy_prayer_"+rubyInterp(p), "responsive")
	return b.Section("Orações", "lords_prayer", lines, nil)
}

func (b *loc2015) mGeneralCollects() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_general_collects", "rubric")
	for _, s := range Arr(Or(b.ResolveOptions("morning_general_collects", loc2015MorningCollects), stringsToAny(loc2015MorningCollects))) {
		lines = b.textWithSpacer(lines, rb.ToS(s), "leader")
	}
	lines = b.textWithSpacer(lines, "morning_rubric_after_prayers", "rubric")
	lines = b.textWithSpacer(lines, "morning_final_prayer", "leader")
	lines = b.textOnly(lines, "morning_rubric_after_final_prayer", "rubric")
	return b.Section("Coletas Gerais", "general_collects", lines, nil)
}

func (b *loc2015) mGeneralThanksgiving() *Section {
	lines := b.generalThanksgivingOpening()
	for _, n := range Arr(Or(b.ResolveRange("morning_general_thanksgivings", 1, 2), 1)) {
		lines = b.textWithSpacer(lines, "general_thanksgiving_"+rubyInterp(n), "congregation")
	}
	return b.Section("Geral Ação de Graças", "thanksgiving", lines, nil)
}

func (b *loc2015) mDismissal() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_concluding_prayers", "rubric")
	lines = b.textWithSpacer(lines, "opening_concluding_prayers", "responsive")
	lines = b.textWithSpacer(lines, "morning_rubric_dismissal", "rubric")
	n := Or(b.ResolveRange("morning_concluding_prayer", 1, 4), 1)
	lines = b.textWithSpacer(lines, "dismissal_"+rubyInterp(n), "leader")
	if t := b.T("rubric_post_dismissal"); t != nil {
		lines = append(lines, b.Spacer(), b.TextLine(t, "rubric"))
	}
	return b.Section("Orações Conclusivas", "dismissal", lines, nil)
}

// --- Evening -----------------------------------------------------------------------------

var loc2015EveningCollects = []string{"for_peace", "for_grace", "for_protection", "for_christ_presence",
	"evening_for_all_authorities", "evening_for_clergy", "evening_for_parish_family"}

func (b *loc2015) evening() []*Section {
	if b.PrefS("office_type") == "family" {
		return Pipeline(
			One(func() *Section { return b.textSection("family_evening_opening", "Ao Entardecer", "opening", "leader") }),
			One(func() *Section {
				return b.familyPsalmFromPref("family_evening_psalm", "family_evening_psalm_29", "Afferte Domino (Salmo 29)")
			}),
			Many(func() []*Section { return b.familyReadingFromPref("family_evening_reading", "family_evening_reading") }),
			One(func() *Section { return b.familyOptionalCollect("family_evening_collect") }),
			One(b.lordsPrayerSimple),
			One(func() *Section {
				return b.textSection("family_evening_collect", "Oração Conclusiva", "general_collects", "leader")
			}),
		)
	}
	return Pipeline(
		One(b.eWelcome), One(b.eOpeningSentence), One(b.eConfession), One(b.absolution), One(b.eInvitatory),
		One(b.eInvitatoryCanticle), One(b.psalmsSection), One(b.firstReading), Many(b.eFirstCanticle),
		One(b.secondReading), Many(b.eSecondCanticle), One(b.eCreed), One(b.offertory), One(b.eLordsPrayer),
		One(b.collectOfTheDay), One(b.eGeneralCollects), One(b.eGeneralThanksgiving), One(b.chrysostom),
		One(b.eDismissal),
	)
}

func (b *loc2015) eWelcome() *Section {
	slug := "evening_welcome_traditional"
	if b.PrefS("prayer_style") == "contemporary" {
		slug = "evening_welcome_contemporary"
	}
	w := b.T(slug)
	if w == nil {
		rb.RaiseNoMethodOnNil("content")
	}
	lines := []*Line{b.TextLine(w, "leader")}
	lines = b.textWithSpacer(lines, "opening_sentence_rubric", "rubric")
	return b.Section("Acolhida", "welcome", lines, nil)
}

func (b *loc2015) eOpeningSentence() *Section {
	var lines []*Line
	n := b.Pref("opening_sentence_general")
	if n == nil || n == false {
		n = b.SeededNumber(1, 8, "evening_opening_sentence")
	}
	lines = b.textWithSpacer(lines, "evening_opening_sentence_"+rubyInterp(n), "leader")
	lines = b.textWithSpacer(lines, "evening_rubric_post_opening_sentence", "rubric")
	if slug := b.seasonOpeningSentenceSlug("evening_opening_sentence_"); slug != "" {
		lines = b.textOnly(lines, slug, "leader")
	}
	lines = b.textWithSpacer(lines, "evening_rubric_after_opening", "rubric")
	if len(lines) == 0 {
		return nil
	}
	return b.Section("Sentenças Iniciais", "opening_sentence", lines, nil)
}

func (b *loc2015) eConfession() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "evening_rubric_before_confession", "rubric")
	slug := "evening_confession_long"
	if b.PrefS("evening_confession_type") == "short" {
		slug = "evening_confession_short"
	}
	lines = b.textWithSpacer(lines, slug, "leader")
	lines = b.textWithSpacer(lines, "rubric_post_opening_confession", "rubric")
	for _, n := range Arr(Or(b.ResolveRange("evening_confession_prayer_type", 1, 3), 1)) {
		lines = b.textWithSpacer(lines, "evening_after_confession_"+rubyInterp(n), "congregation")
	}
	lines = b.textWithSpacer(lines, "rubric_post_confession", "rubric")
	if b.PrefS("use_priestly_absolution") != "yes" {
		p := b.ResolveRange("evening_prayer_after_confession", 1, 2)
		lines = b.textOnly(lines, "prayer_after_confession_"+rubyInterp(p), "congregation")
	}
	return b.Section("Confissão de Pecados", "confession", lines, nil)
}

func (b *loc2015) eInvitatory() *Section {
	inv := b.T("evening_invocation")
	if inv == nil {
		return nil
	}
	lines := []*Line{b.TextLine(inv, "responsive"), b.Spacer()}
	lines = b.textOnly(lines, "evening_rubric_post_invocation", "rubric")
	return b.Section("Invitatório e Salmo", "invitatory", lines, nil)
}

func (b *loc2015) eInvitatoryCanticle() *Section {
	return b.invitatoryCanticle(b.invitatoryCanticleSlug("evening_invitatory_canticle", []string{"phos_hilaron", "ecce_nunc"}, "phos_hilaron"))
}

func (b *loc2015) eFirstCanticle() []*Section {
	return b.canticleSections(Arr(Or(b.ResolveOptions("evening_post_first_reading_canticle",
		[]string{"magnificat", "bonum_est_confiteri", "benedictus"}), "magnificat")))
}

func (b *loc2015) eSecondCanticle() []*Section {
	mods := b.canticleSections(Arr(Or(b.ResolveOptions("evening_post_second_reading_canticle",
		[]string{"nunc_dimittis", "deus_misereatur", "dignus_es"}), "nunc_dimittis")))
	return b.endSecondCanticle(mods)
}

func (b *loc2015) eCreed() *Section {
	if b.PrefS("evening_creed_type") == "faith_affirmation" {
		var lines []*Line
		lines = b.textWithSpacer(lines, "evening_rubric_post_apostles_creed", "rubric")
		a := b.T("evening_affirmation_of_faith")
		if a == nil {
			return nil
		}
		lines = append(lines, b.TextLine(a, "congregation"))
		return b.Section("Afirmação de Fé", "creed", lines, nil)
	}
	c := b.T("apostles_creed")
	if c == nil {
		return nil
	}
	return b.Section("Credo Apostólico", "creed", []*Line{b.TextLine(c, "congregation")}, nil)
}

func (b *loc2015) eLordsPrayer() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_before_prayers", "rubric")
	for _, n := range Arr(Or(b.ResolveRange("evening_invocation", 1, 2), 2)) {
		lines = b.textWithSpacer(lines, "invocation_our_father_"+rubyInterp(n), "responsive")
	}
	lp := b.T("our_father")
	if lp == nil {
		return nil
	}
	lines = append(lines, b.TextLine(lp, "congregation"), b.Spacer())
	lines = b.textWithSpacer(lines, "rubric_after_our_father", "rubric")
	if rb.Truthy(b.Pref("use_evening_closing_prayer")) {
		lines = b.textOnly(lines, "evening_closing_prayer", "responsive")
	} else {
		for _, n := range Arr(Or(b.ResolveRange("evening_post_lords_prayer_prayer", 1, 3), 1)) {
			lines = b.textOnly(lines, "mercy_prayer_"+rubyInterp(n), "responsive")
		}
	}
	return b.Section("Orações", "lords_prayer", lines, nil)
}

func (b *loc2015) eGeneralCollects() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_general_collects", "rubric")
	for _, s := range Arr(Or(b.ResolveOptions("evening_general_collects", loc2015EveningCollects), stringsToAny(loc2015EveningCollects))) {
		lines = b.textWithSpacer(lines, rb.ToS(s), "leader")
	}
	lines = b.textOnly(lines, "evening_rubric_post_prayers", "rubric")
	return b.Section("Coletas Gerais", "general_collects", lines, nil)
}

func (b *loc2015) eGeneralThanksgiving() *Section {
	lines := b.generalThanksgivingOpening()
	for _, n := range Arr(Or(b.ResolveRange("evening_general_thanksgivings", 1, 2), 1)) {
		lines = b.textOnly(lines, "general_thanksgiving_"+rubyInterp(n), "congregation")
	}
	return b.Section("Geral Ação de Graças", "thanksgiving", lines, nil)
}

func (b *loc2015) eDismissal() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_concluding_prayers", "rubric")
	lines = b.textWithSpacer(lines, "opening_concluding_prayers", "responsive")
	lines = b.textWithSpacer(lines, "evening_rubric_post_concluding_prayers", "rubric")
	for _, n := range Arr(Or(b.ResolveRange("evening_concluding_prayer", 1, 4), 1)) {
		lines = b.textOnly(lines, "dismissal_"+rubyInterp(n), "leader")
	}
	if t := b.T("rubric_post_dismissal"); t != nil {
		lines = append(lines, b.Spacer(), b.TextLine(t, "rubric"))
	}
	return b.Section("Orações Conclusivas", "dismissal", lines, nil)
}

// --- Midday -------------------------------------------------------------------------------

func (b *loc2015) midday() []*Section {
	if b.PrefS("office_type") == "family" {
		return Pipeline(
			One(func() *Section { return b.textSection("family_midday_opening", "Ao Meio-Dia", "opening", "leader") }),
			One(func() *Section {
				return b.familyPsalmFromPref("family_midday_psalm", "family_midday_psalm_121", "Levavi Oculus (Salmo 121)")
			}),
			Many(func() []*Section { return b.familyReadingFromPref("family_midday_reading", "family_midday_reading") }),
			One(func() *Section { return b.familyOptionalCollect("family_midday_collect") }),
			One(b.lordsPrayerSimple),
			One(func() *Section { return b.textSection("family_midday_collect", "Oração Conclusiva", "prayer", "leader") }),
		)
	}
	return Pipeline(One(b.mdOpening), One(b.mdInvitation), One(b.mdPsalmsTitle), Many(b.mdPsalms),
		One(b.mdReadings), One(b.mdPrayer), One(b.mdDismissal))
}

func nonEmpty(b *Base, name, slug string, lines []*Line) *Section {
	if len(lines) == 0 {
		return nil
	}
	return b.Section(name, slug, lines, nil)
}

func (b *loc2015) mdOpening() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "midday_rubric_opening", "rubric")
	lines = b.textWithSpacer(lines, "midday_preparation", "leader")
	return nonEmpty(&b.Base, "Acolhida", "opening", lines)
}

func (b *loc2015) mdInvitation() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "midday_rubric_invitation", "rubric")
	slug := "midday_invitation"
	if b.IsLent() {
		slug = "midday_invitation_lent"
	}
	lines = b.textWithSpacer(lines, slug, "responsive")
	return nonEmpty(&b.Base, "Invitatório", "invitation", lines)
}

func (b *loc2015) mdPsalmsTitle() *Section {
	return nonEmpty(&b.Base, "Salmo", "psalms_title", b.textWithSpacer(nil, "midday_rubric_psalm", "rubric"))
}

func (b *loc2015) mdPsalms() []*Section {
	slugs := map[any]string{1: "lucerna_pedibus_meis", 2: "levavi_oculos", 3: "in_convertendo"}
	selected := Or(b.ResolveAny("midday_inviting_canticle", []any{1, 2, 3}), []any{1, 2, 3})
	var out []*Section
	for _, n := range Arr(selected) {
		slug, ok := slugs[n]
		if !ok {
			continue
		}
		t := b.T(slug)
		if t == nil {
			continue
		}
		out = append(out, b.Section(NameWithRef(t, "Salmo"), slug, []*Line{b.TextLine(t, "congregation")}, nil))
	}
	return out
}

func (b *loc2015) mdReadings() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "midday_rubric_before_readings", "rubric")
	for n := 1; n <= 3; n++ {
		lines = b.textWithSpacer(lines, fmt.Sprintf("midday_reading_%d", n), "leader")
	}
	return nonEmpty(&b.Base, "Leituras", "readings", lines)
}

func (b *loc2015) mdPrayer() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "midday_rubric_before_prayers", "rubric")
	lines = b.textWithSpacer(lines, "midday_our_father", "responsive")
	lines = b.textWithSpacer(lines, "rubric_after_our_father", "rubric")
	lines = b.textWithSpacer(lines, "midday_prayer", "responsive")
	lines = b.textWithSpacer(lines, "midday_rubric_after_prayer", "rubric")
	return nonEmpty(&b.Base, "Orações", "prayer", lines)
}

func (b *loc2015) mdDismissal() *Section {
	slug := "midday_dismissal"
	if b.IsLent() {
		slug = "midday_dismissal_lent"
	}
	return nonEmpty(&b.Base, "Despedida", "dismissal", b.textOnly(nil, slug, "responsive"))
}

// --- Late evening (family) -----------------------------------------------------------------

func (b *loc2015) lateEvening() []*Section {
	return Pipeline(
		One(func() *Section { return b.textSection("family_late_evening_opening", "Ao Anoitecer", "welcome", "leader") }),
		One(func() *Section {
			return b.familyPsalmFromPref("family_late_evening_psalm", "family_late_evening_psalm_138", "Confitebor Tibi (Salmo 138)")
		}),
		Many(func() []*Section {
			return b.familyReadingFromPref("family_late_evening_reading", "family_late_evening_reading")
		}),
		One(func() *Section { return b.familyOptionalCollect("family_late_evening_collect") }),
		One(b.lordsPrayerSimple),
		One(func() *Section { return b.textSection("family_late_evening_collect", "Oração Conclusiva", "collect", "leader") }),
	)
}

// --- helpers --------------------------------------------------------------------------------

func lowerRuby(s string) string { return strings.ToLower(s) }

// rubyInterp renders a value as Ruby string interpolation does (nil => "").
func rubyInterp(v any) string {
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

func stringsToAny(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// --- Compline -------------------------------------------------------------------------------

func (b *loc2015) compline() []*Section {
	if b.PrefS("office_type") == "family" {
		return Pipeline(
			One(func() *Section { return b.textSection("family_compline_opening", "No Fim do Dia", "opening", "leader") }),
			One(func() *Section {
				return b.familyPsalmFromPref("family_compline_psalm", "family_compline_psalm_91", "Qui Habitat (Salmo 91)")
			}),
			Many(func() []*Section { return b.familyReadingFromPref("family_compline_reading", "family_compline_reading") }),
			One(func() *Section { return b.familyOptionalCollect("family_compline_daily_collect") }),
			One(b.lordsPrayerSimple),
			One(b.cFamilyFixedCollect),
			One(b.cFamilyDismissal),
		)
	}
	return Pipeline(One(b.cOpening), One(b.cBriefLesson), One(b.cConfession), One(b.cAbsolution), One(b.cPsalmsTitle),
		Many(b.cPsalms), One(b.cReadings), One(b.cResponse), One(b.cKyrie), One(b.cLordsPrayer), One(b.cAntiphon),
		One(b.cNuncDimittis), One(b.cDismissal))
}

func (b *loc2015) cFamilyFixedCollect() *Section {
	num := Or(b.ResolveRange("family_compline_collect", 1, 2), 1)
	if l, ok := num.([]any); ok {
		panic(&rb.RubyError{Class: "NoMethodError", Message: "undefined method `to_i' for " + rb.Inspect(l) + ":Array"})
	}
	if n := rb.ToI(num); n < 1 || n > 2 {
		num = 1
	}
	return b.textSection("family_compline_collect_"+rubyInterp(num), "Oração Conclusiva", "collect", "leader")
}

func (b *loc2015) cFamilyDismissal() *Section {
	t := b.T("family_compline_dismissal")
	if t == nil {
		return nil
	}
	return b.Section("Benção", "dismissal", []*Line{
		b.TextLine(t, "leader"),
		b.FetchLineItem("reading_citation", "citation", [][2]string{{"reference", Ref(t)}}, "content"),
	}, nil)
}

func (b *loc2015) cOpening() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "compline_rubric_opening", "rubric")
	lines = b.textWithSpacer(lines, "compline_preparation", "responsive")
	lines = b.textWithSpacer(lines, "compline_rubric_post_preparation", "rubric")
	return nonEmpty(&b.Base, "Preparação", "opening", lines)
}

func (b *loc2015) cBriefLesson() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "compline_brief_lesson", "leader")
	lines = b.textOnly(lines, "compline_rubric_silence", "rubric")
	return nonEmpty(&b.Base, "Lição Breve", "brief_lesson", lines)
}

func (b *loc2015) cConfession() *Section {
	c := b.T("compline_confession")
	if c == nil {
		return nil
	}
	lines := []*Line{b.TextLine(c, "congregation"), b.Spacer()}
	lines = b.textOnly(lines, "compline_rubric_silence", "rubric")
	lines = b.textOnly(lines, "compline_post_confession", "congregation")
	return b.Section("Confissão", "confession", lines, nil)
}

func (b *loc2015) cAbsolution() *Section {
	a := b.T("compline_absolution")
	if a == nil {
		return nil
	}
	lines := []*Line{b.TextLine(a, "responsive"), b.Spacer()}
	lines = b.textOnly(lines, "compline_rubric_post_absolution", "rubric")
	return b.Section("Súplica de Perdão", "absolution", lines, nil)
}

func (b *loc2015) cPsalmsTitle() *Section {
	return nonEmpty(&b.Base, "Salmodia", "psalms", b.textOnly(nil, "compline_rubric_before_psalms", "rubric"))
}

func (b *loc2015) cPsalms() []*Section {
	mods := b.canticleSections(Arr(Or(b.ResolveOptions("compline_inviting_canticle",
		[]string{"cum_invocarem", "qui_habitat", "ecce_nunc"}), "cum_invocarem")))
	if t := b.T("compline_rubric_silence"); t != nil && len(mods) > 0 {
		mods = append(mods, b.Section("", "rubric_silence", []*Line{b.TextLine(t, "rubric")}, nil))
	}
	return mods
}

func (b *loc2015) cReadings() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "compline_rubric_before_lessons", "rubric")
	n := b.ResolveRange("compline_brief_lesson", 1, 3)
	lines = b.textWithSpacer(lines, "compline_brief_lesson_"+rubyInterp(n), "leader")
	lines = b.textOnly(lines, "compline_rubric_post_lessons", "rubric")
	lines = b.textOnly(lines, "compline_rubric_silence", "rubric")
	return nonEmpty(&b.Base, "Lições Breves", "readings", lines)
}

func (b *loc2015) cResponse() *Section {
	return b.textSection("compline_brief_response", "Responsório Breve", "response", "responsive")
}

func (b *loc2015) cKyrie() *Section {
	slug := "kyrie_translated"
	if b.PrefS("office_type") == "traditional" {
		slug = "kyrie_original"
	}
	return b.textSection(slug, "Kyrie Eleison", "kyrie", "congregation")
}

func (b *loc2015) cLordsPrayer() *Section {
	var lines []*Line
	lines = b.textWithSpacer(lines, "rubric_before_prayers", "rubric")
	lines = b.textWithSpacer(lines, "compline_our_father", "congregation")
	lines = b.textOnly(lines, "compline_starting_prayer", "responsive")
	if len(lines) == 0 {
		return nil
	}
	lines = b.textWithSpacer(lines, "compline_rubric_before_final_prayer", "rubric")
	selected := Arr(b.ResolveRange("compline_final_prayer", 1, 6))
	for i, n := range selected {
		if t := b.T("compline_final_prayer_" + rubyInterp(n)); t != nil {
			lines = append(lines, b.TextLine(t, "leader"))
			if i < len(selected)-1 {
				lines = append(lines, b.Spacer())
			}
		}
	}
	lines = b.textWithSpacer(lines, "compline_rubric_before_antiphon", "rubric")
	return b.Section("Orações", "lords_prayer", lines, nil)
}

func (b *loc2015) cAntiphon() *Section {
	return b.Section("Antífona", "antiphon", b.textOnly(nil, "compline_antiphon", "congregation"), nil)
}

func (b *loc2015) cNuncDimittis() *Section {
	return b.textSection("nunc_dimittis", "Nunc Dimittis", "nunc_dimittis", "congregation")
}

func (b *loc2015) cDismissal() *Section {
	var lines []*Line
	lines = b.textOnly(lines, "compline_antiphon", "congregation")
	lines = b.textWithSpacer(lines, "compline_final_prayer", "responsive")
	lines = b.textOnly(lines, "compline_final_rubric", "rubric")
	return nonEmpty(&b.Base, "Antífona", "dismissal", lines)
}
