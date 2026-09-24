package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
)

func init() {
	Register("loc_1987", func(ctx context.Context, c *Context) Builder {
		if c.OfficeType != "morning" && c.OfficeType != "evening" {
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc1987{}
		b.Init(ctx, c)
		return b
	})
}

// loc1987 ports DailyOffice::Builders::Loc1987::{Morning,Evening}; the two
// offices share their order and differ where noted.
type loc1987 struct{ Base }

func (b *loc1987) Call() *rb.Map {
	return b.Render(Pipeline(
		One(b.introductorySentences),
		One(b.confession),
		One(b.invitatory),
		One(b.psalms),
		One(b.readingsAndCanticles),
		One(b.creed),
		One(b.offertory),
		One(b.prayers),
		One(b.generalCollects),
		One(b.generalThanksgiving),
		One(b.chrysostom),
		One(b.conclusion),
	))
}

func (b *loc1987) morning() bool { return b.OfficeType == "morning" }

// pref resolves "<office>_<key>" against string options.
func (b *loc1987) pref(key string, options ...string) any {
	return b.ResolveOptions(b.OfficeType+"_"+key, options)
}

// slugged appends line_item(t.content, type:, slug: t.slug) when the text exists.
func (b *loc1987) slugged(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.Item(t.Content, typ, t.Slug, ""))
	}
	return lines
}

func (b *loc1987) fetch(slug, typ string) *Line { return b.FetchLineItem(slug, typ, nil, "content") }

var loc1987SentenceSlugs = map[string]string{
	"advento": "opening_sentence_advent", "natal": "opening_sentence_christmas",
	"epifania": "opening_sentence_epiphany", "quaresma": "opening_sentence_lent",
	"semana santa": "opening_sentence_holy_week", "paixão": "opening_sentence_passion",
	"páscoa": "opening_sentence_easter", "ascensão": "opening_sentence_ascension",
	"pentecostes": "opening_sentence_pentecost",
}

func (b *loc1987) introductorySentences() *Section {
	seasonSlug, ok := loc1987SentenceSlugs[strings.ToLower(b.Season())]
	if !ok {
		seasonSlug = "opening_sentence_general_1"
	}
	minister, people := b.T(seasonSlug+"_minister"), b.T(seasonSlug+"_people")
	if minister == nil {
		minister, people = b.T(b.OfficeType+"_opening_sentence_minister"), b.T(b.OfficeType+"_opening_sentence_people")
	}
	var lines []*Line
	if minister != nil {
		lines = append(lines, b.Item(minister.Content, "leader", minister.Slug, ""))
	}
	if people != nil {
		lines = append(lines, b.Item(people.Content, "congregation", people.Slug, ""))
	}
	return b.Section("Sentenças Introdutórias", "introductory_sentences", lines, nil)
}

func (b *loc1987) confession() *Section {
	lines := b.slugged(nil, "rubric_confession_intro", "rubric")
	invitationSlug := "confession_invitation_1"
	if b.pref("confession_type", "1", "2", "3") == "2" {
		invitationSlug = "confession_invitation_2"
	}
	if t := b.T(invitationSlug); t != nil {
		lines = append(lines, b.Item(t.Content, "leader", t.Slug, ""), b.Spacer())
	}
	if t := b.T("rubric_confession_silence"); t != nil {
		lines = append(lines, b.Item(t.Content, "rubric", t.Slug, ""), b.Spacer())
	}
	opt := Or(b.pref("confession_type", "1", "2", "3"), "1")
	if t := b.T("confession_prayer_" + rubyInterp(opt)); t != nil {
		lines = append(lines, b.Item(t.Content, "congregation", t.Slug, ""), b.Spacer())
	}
	lines = b.slugged(lines, "rubric_absolution_intro", "rubric")
	switch Or(b.pref("abs_type", "1", "2"), "1") {
	case "1":
		if t := b.T("absolution_prayer_deacon"); t != nil {
			lines = append(lines, b.Item(t.Content, "congregation", t.Slug, ""), b.Spacer())
		}
	case "2":
		lines = b.slugged(lines, "absolution_prayer_deacon_2_minister", "leader")
		lines = b.slugged(lines, "absolution_prayer_deacon_2_people", "congregation")
		lines = append(lines, b.Spacer())
	}
	lines = b.slugged(lines, "rubric_absolution_priest", "rubric")
	lines = b.slugged(lines, "absolution_priest", "leader")
	return b.Section("Confissão e Absolvição", "confession", lines, nil)
}

var loc1987AntiphonSlugs = map[string]string{
	"advento": "invitatory_antiphon_advent", "natal": "invitatory_antiphon_christmas",
	"epifania": "invitatory_antiphon_epiphany", "quaresma": "invitatory_antiphon_lent",
	"páscoa": "invitatory_antiphon_easter", "ascensão": "invitatory_antiphon_ascension",
	"pentecostes": "invitatory_antiphon_pentecost",
}

func (b *loc1987) invitatory() *Section {
	lines := b.slugged(nil, "rubric_invitatory_stand", "rubric")
	lines = append(lines,
		b.fetch("invitatory_opening_minister", "leader"), b.fetch("invitatory_opening_people", "congregation"),
		b.fetch("invitatory_glory_minister", "leader"), b.fetch("invitatory_glory_people", "congregation"),
		b.fetch("invitatory_praise_minister", "leader"), b.fetch("invitatory_praise_people", "congregation"),
		b.Spacer())
	if !b.morning() {
		// No Venite at Evening Prayer.
		return b.Section("Invitatório", "invitatory", lines, nil)
	}
	season := strings.ToLower(b.Season())
	lines = b.slugged(lines, "rubric_antiphons", "rubric")
	if slug, ok := loc1987AntiphonSlugs[season]; ok {
		if t := b.T(slug); t != nil {
			lines = append(lines, b.Item(t.Content, "leader", t.Slug, ""), b.Spacer())
		}
	}
	lines = b.slugged(lines, "rubric_venite", "rubric")
	var opt any = "pascha_nostrum"
	if season != "páscoa" {
		opt = Or(b.pref("invitatory_psalm", "venite", "jubilate", "pascha_nostrum"), "venite")
	}
	if t := b.tAny(opt); t != nil {
		lines = append(lines, b.Item(t.Content, "text", t.Slug, ""), b.Spacer())
	}
	lines = b.slugged(lines, "gloria_patri_minister", "leader")
	lines = b.slugged(lines, "gloria_patri_people", "congregation")
	lines = append(lines, b.Spacer())
	return b.Section("Invitatório", "invitatory", lines, nil)
}

func (b *loc1987) psalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := b.slugged(nil, "rubric_psalms", "rubric")
	lines = append(lines, b.I(p.Reference, "heading"), b.Spacer())
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
		lines = append(lines, b.Spacer())
	}
	lines = append(lines,
		b.fetch("psalms_gloria_patri_minister", "leader"),
		b.fetch("psalms_gloria_patri_people", "congregation"))
	return b.Section("Salmos", "psalms", lines, nil)
}

func (b *loc1987) readingsAndCanticles() *Section {
	lines := b.slugged(nil, "rubric_readings_sit", "rubric")
	lessons := b.OfficeLessons()
	var first, second *reading.Passage
	if len(lessons) > 0 {
		first = lessons[0]
	}
	if len(lessons) > 1 {
		second = lessons[1]
	}
	if t := b.T("rubric_readings_announcement"); t != nil {
		lines = append(lines, b.Item(SubstituteReadingPlaceholders(t.Content, first), "rubric", t.Slug, ""))
	}
	rubricEnd := b.T("rubric_readings_end")
	lessonTail := func(l []*Line) []*Line {
		if rubricEnd != nil {
			l = append(l, b.Item(rubricEnd.Content, "rubric", rubricEnd.Slug, ""))
		}
		return append(l,
			b.fetch("reading_word_of_the_lord", "leader"),
			b.fetch("reading_thanks_be_to_god", "congregation"),
			b.Spacer())
	}
	formatLines := func(r *reading.Passage) []*Line {
		l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			l = append(l, b.BibleContent(r.Content)...)
		}
		return lessonTail(l)
	}
	if first != nil {
		lines = append(lines, formatLines(first)...)
	}
	silence := b.T("rubric_readings_silence")
	if silence != nil {
		lines = append(lines, b.Item(silence.Content, "rubric", silence.Slug, ""))
	}
	var canticleSlug any
	if b.morning() {
		canticleSlug = Or(b.pref("canticle", "te_deum", "benedictus_es", "benedictus"), "te_deum")
	} else {
		canticleSlug = Or(b.pref("canticle_1", "magnificat", "cantate_domino", "bonum_est"), "magnificat")
	}
	if t := b.tAny(canticleSlug); t != nil {
		lines = append(lines, b.Item(t.Content, "text", t.Slug, ""), b.Spacer())
	}
	if second != nil {
		lines = append(lines, b.I(second.Reference, "heading"), b.Spacer())
		if second.Content != nil {
			lines = append(lines, b.BibleContent(second.Content)...)
		}
		lines = lessonTail(lines)
	}
	if b.morning() {
		if canticleSlug != "benedictus" {
			lines = b.slugged(lines, "rubric_readings_silence", "rubric")
			lines = b.slugged(lines, "benedictus", "text")
		}
	} else {
		if silence != nil {
			lines = append(lines, b.Item(silence.Content, "rubric", silence.Slug, ""))
		}
		c2 := Or(b.pref("canticle_2", "nunc_dimittis", "deus_misereatur", "benedic_anima_mea"), "nunc_dimittis")
		if t := b.tAny(c2); t != nil {
			lines = append(lines, b.Item(t.Content, "text", t.Slug, ""))
		}
	}
	lines = b.slugged(lines, "rubric_sermon", "rubric")
	return b.SectionWithReadingExtras("Leitura da Palavra de Deus", "readings", lines, "first_reading", nil, formatLines)
}

func (b *loc1987) creed() *Section {
	lines := b.slugged(nil, "rubric_creed_stand", "rubric")
	lines = b.slugged(lines, "apostles_creed", "congregation")
	if b.morning() {
		lines = b.slugged(lines, "rubric_creed_paraphrase", "rubric")
	}
	return b.Section("Credo", "creed", lines, nil)
}

func (b *loc1987) offertory() *Section {
	o := b.T("offertory_sentence")
	if o == nil {
		return nil
	}
	lines := b.slugged(nil, "rubric_offertory", "rubric")
	lines = append(lines, b.Item(o.Content, "leader", o.Slug, ""))
	return b.Section("Ofertório", "offertory", lines, nil)
}

func (b *loc1987) prayers() *Section {
	lines := b.slugged(nil, "rubric_prayers_intro", "rubric")
	lines = append(lines,
		b.fetch("prayers_greeting_minister", "leader"),
		b.fetch("prayers_greeting_people", "congregation"),
		b.fetch("prayers_let_us_pray", "leader"),
		b.Spacer())
	lines = b.slugged(lines, "lords_prayer_traditional", "congregation")
	lines = append(lines, b.Spacer())
	lines = b.slugged(lines, "rubric_suffrages_intro", "rubric")
	set, n := "2", 5
	if Or(b.pref("suffrages", "1", "2"), "1") == "1" {
		set, n = "1", 7
	}
	for i := 1; i <= n; i++ {
		lines = b.plain(lines, fmt.Sprintf("suffrages_%s_v%d", set, i), "leader")
		lines = b.plain(lines, fmt.Sprintf("suffrages_%s_r%d", set, i), "congregation")
	}
	lines = append(lines, b.Spacer())
	lines = b.slugged(lines, "rubric_collects_intro", "rubric")
	lines = append(lines, b.CollectLines(b.Collects)...)
	return b.Section("Orações", "prayers", lines, nil)
}

var (
	loc1987MorningCollects = []string{"collect_peace", "collect_grace", "prayer_authorities",
		"prayer_clergy_people", "prayer_parish_family", "prayer_all_humanity"}
	loc1987EveningCollects = []string{"collect_peace_2", "collect_grace", "prayer_authorities",
		"prayer_clergy_people", "prayer_parish_family", "prayer_all_humanity"}
)

func (b *loc1987) generalCollects() *Section {
	lines := b.slugged(nil, "rubric_prayers_general", "rubric")
	lines = b.slugged(lines, "rubric_communion_bridge", "rubric")
	slugs := loc1987MorningCollects
	if !b.morning() {
		slugs = loc1987EveningCollects
	}
	selected := b.pref("general_collects", slugs...)
	if selected == nil {
		selected = stringsToAny(slugs)
	}
	for _, s := range Arr(selected) {
		t := b.tAny(s)
		if t == nil {
			continue
		}
		lines = append(lines, b.I(Title(t), "heading"), b.Item(t.Content, "text", t.Slug, ""), b.Spacer())
	}
	return b.Section("Coletas Gerais", "general_collects", lines, nil)
}

func (b *loc1987) generalThanksgiving() *Section {
	lines := b.slugged(nil, "rubric_thanksgiving_joint", "rubric")
	if Or(b.pref("general_thanksgiving", "1", "2"), "1") == "1" {
		lines = b.slugged(lines, "general_thanksgiving_1", "congregation")
	} else {
		for i := 1; i <= 9; i++ {
			lines = b.slugged(lines, fmt.Sprintf("general_thanksgiving_2_v%d", i), "leader")
			lines = b.slugged(lines, fmt.Sprintf("general_thanksgiving_2_r%d", i), "congregation")
		}
		lines = b.slugged(lines, "general_thanksgiving_2_all", "congregation")
	}
	return b.Section("Ação de Graças Geral", "thanksgiving", lines, nil)
}

func (b *loc1987) chrysostom() *Section {
	t := b.T("prayer_chrysostom")
	if t == nil {
		return nil
	}
	return b.Section("Oração de São João Crisóstomo", "chrysostom", []*Line{b.Item(t.Content, "leader", t.Slug, "")}, nil)
}

func (b *loc1987) conclusion() *Section {
	var lines []*Line
	switch Or(b.pref("conclusion", "1", "2", "3"), "1") {
	case "1":
		lines = b.slugged(lines, "conclusion_grace", "leader")
	case "2":
		lines = b.slugged(lines, "conclusion_blessing", "leader")
	case "3":
		lines = b.plain(lines, "conclusion_protection_minister", "leader")
		lines = b.plain(lines, "conclusion_protection_people", "congregation")
	}
	lines = b.slugged(lines, "rubric_sermon_end", "rubric")
	return b.Section("Conclusão", "conclusion", lines, nil)
}
