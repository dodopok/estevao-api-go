package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

func init() {
	Register("loc_1979_en", func(ctx context.Context, c *Context) Builder {
		b := &loc1979{}
		switch c.OfficeType {
		case "morning", "evening":
			// Rite One and Rite Two are distinct liturgies: the preference picks
			// the office class (Loc1979EnBuilder#office_builder_class_name).
			b.rite = 2
			if b.riteFromPreference(c) == "1" {
				b.rite = 1
			}
		case "midday", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		if b.rite == 1 {
			b.H.CollectLanguageStyle = func() string { return "traditional" }
		}
		b.Init(ctx, c)
		return b
	})
	Register("loc_1979_es", func(ctx context.Context, c *Context) Builder {
		// The Spanish book prints one contemporary form per office.
		switch c.OfficeType {
		case "morning", "evening", "midday", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc1979{rite: 2}
		b.H.BuildSection = func(name any, slug string, lines []*Line, meta *rb.Map) *Section {
			n := rb.ToS(name)
			if t, ok := loc1979EsSectionNames[n]; ok {
				n = t
			}
			return NewSection(n, slug, lines, meta)
		}
		b.Init(ctx, c)
		return b
	})
}

var loc1979EsSectionNames = map[string]string{
	"Opening Sentences": "Versículos de apertura", "Confession of Sin": "Confesión de pecado",
	"Invitatory": "Invitatorio y salterio", "Psalms": "Los salmos del día", "The Lessons": "Las lecturas",
	"Lessons": "Las lecturas", "The Creed": "Credo de los apóstoles", "The Prayers": "Las oraciones",
	"Collects": "Colectas", "Prayer for Mission": "Oración por la misión",
	"The General Thanksgiving":   "Acción de Gracias de uso General",
	"A Prayer of St. Chrysostom": "Oración de san Juan Crisóstomo", "Conclusion": "Conclusión",
	"In the Morning": "Devoción de la mañana", "At Noon": "Devoción del mediodía",
	"In the Evening": "Devoción del atardecer", "In the Early Evening": "Devoción del atardecer",
	"At the Close of Day": "Devoción para terminar el día", "Reading": "Lectura", "Devotions": "Devociones",
	"Prayers": "Oraciones", "Collect": "La colecta", "Blessing": "Bendición",
	"Nunc Dimittis": "Cántico de Simeón", "Compline": "Oración de la Noche",
}

// loc1979 ports DailyOffice::Builders::Loc1979En::* (and Loc1979Es, which
// localizes the Rite Two, Midday and Compline classes).
type loc1979 struct {
	Base
	rite int // 1 or 2 for Morning/Evening Prayer
}

// riteFromPreference ports resolve_rite_preference(:daily_office_rite, %w[1 2])
// with rites.fetch(rite, rites.fetch("2")).
func (b *loc1979) riteFromPreference(c *Context) string {
	tmp := &Base{C: c, Prefs: c.Prefs}
	r := tmp.RiteChoice("daily_office_rite", []string{"1", "2"})
	if r == "1" {
		return "1"
	}
	return "2"
}

func (b *loc1979) Call() *rb.Map {
	family := b.PrefS("office_type") == "family"
	switch b.OfficeType {
	case "morning":
		if b.rite == 2 && family {
			return b.Render(b.family("morning"))
		}
		return b.Render(b.morning())
	case "evening":
		if b.rite == 2 && family {
			return b.Render(b.family("evening"))
		}
		return b.Render(b.evening())
	case "midday":
		if family {
			return b.Render(b.family("midday"))
		}
		return b.Render(b.midday())
	}
	if family {
		return b.Render(b.family("compline"))
	}
	return b.Render(b.compline())
}

// --- helpers -----------------------------------------------------------------------------

// sl appends line_item(t.content, type:, slug: t.slug) when the text exists.
func (b *loc1979) sl(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.Item(t.Content, typ, t.Slug, ""))
	}
	return lines
}

func (b *loc1979) fetch(slug, typ string) *Line { return b.FetchLineItem(slug, typ, nil, "content") }

// pair appends both lines only when both texts exist.
func (b *loc1979) pair(lines []*Line, v, r string) []*Line {
	tv, tr := b.T(v), b.T(r)
	if tv != nil && tr != nil {
		lines = append(lines, b.Item(tv.Content, "leader", tv.Slug, ""), b.Item(tr.Content, "congregation", tr.Slug, ""))
	}
	return lines
}

func (b *loc1979) seasonDown() string { return strings.ToLower(b.Season()) }

// presencePref ports preferences[key].presence.
func (b *loc1979) presencePref(key string) any {
	if v := b.Pref(key); rb.Present(v) {
		return v
	}
	return nil
}

// orPref ports preferences[key] || default.
func (b *loc1979) orPref(key string, def any) any {
	v := b.Pref(key)
	if v == nil || v == false {
		return def
	}
	return v
}

func (b *loc1979) seededChoice(values []string, key string) string {
	return values[b.SeededPick(len(values), key)]
}

// rotatingChoice ports loc1979_rotating_choice.
func (b *loc1979) rotatingChoice(key string, values []string) string {
	choice := b.presencePref(key)
	if choice == nil {
		choice = "random"
	}
	if rb.ToS(choice) == "random" {
		return b.seededChoice(values, key)
	}
	return rb.ToS(choice)
}

// collectChoices ports loc1979_collect_choices.
func (b *loc1979) collectChoices(key string, values []string) []string {
	choice := b.presencePref(key)
	if choice == nil {
		choice = "random"
	}
	if l, ok := choice.([]any); ok && len(l) == 1 && (rb.ToS(l[0]) == "random" || rb.ToS(l[0]) == "all") {
		choice = l[0]
	}
	var out []string
	switch rb.ToS(choice) {
	case "random":
		out = []string{b.seededChoice(values, key)}
	case "all":
		out = values
	default:
		for _, x := range Arr(choice) {
			s := rb.ToS(x)
			for _, v := range values {
				if v == s {
					out = append(out, s)
					break
				}
			}
		}
	}
	if len(out) == 0 {
		out = []string{values[0]}
	}
	return out
}

var (
	loc1979MorningCanticles = map[int]map[string][][]string{
		1: {
			"first": {{"canticle_4", "canticle_16"}, {"canticle_9"}, {"canticle_2", "canticle_13"},
				{"canticle_11"}, {"canticle_8"}, {"canticle_10"}, {"canticle_1", "canticle_12"}},
			"second": {{"canticle_7", "canticle_21"}, {"canticle_19"}, {"canticle_18"},
				{"canticle_4", "canticle_16"}, {"canticle_6", "canticle_20"}, {"canticle_18"}, {"canticle_19"}},
		},
		2: {
			"first": {{"canticle_4", "canticle_16"}, {"canticle_9"}, {"canticle_2", "canticle_13"},
				{"canticle_11"}, {"canticle_8"}, {"canticle_10"}, {"canticle_1", "canticle_12"}},
			"second": {{"canticle_7", "canticle_21"}, {"canticle_19"}, {"canticle_18"},
				{"canticle_4", "canticle_16"}, {"canticle_6", "canticle_20"}, {"canticle_18"}, {"canticle_19"}},
		},
	}
	loc1979EveningCanticles = map[int]map[string][][]string{
		1: {
			"first": {{"ep1_magnificat"}, {"canticle_8"}, {"canticle_10"}, {"canticle_1", "canticle_12"},
				{"canticle_11"}, {"canticle_2", "canticle_13"}, {"canticle_9"}},
			"second": {{"ep1_nunc_dimittis"}, {"ep1_nunc_dimittis"}, {"ep1_magnificat"},
				{"ep1_nunc_dimittis"}, {"ep1_magnificat"}, {"ep1_nunc_dimittis"}, {"ep1_magnificat"}},
		},
		2: {
			"first": {{"canticle_15"}, {"canticle_8"}, {"canticle_10"}, {"canticle_1", "canticle_12"},
				{"canticle_11"}, {"canticle_2", "canticle_13"}, {"canticle_9"}},
			"second": {{"canticle_17"}, {"canticle_17"}, {"canticle_15"}, {"canticle_17"},
				{"canticle_15"}, {"canticle_17"}, {"canticle_15"}},
		},
	}
)

func (b *loc1979) majorFeast() bool {
	t := rb.ToS(b.celebrationField("type"))
	return t == "principal_feast" || t == "major_holy_day"
}

func (b *loc1979) morningCanticle(key, slot string) string {
	weekday := b.Date.Weekday()
	if b.majorFeast() {
		weekday = 0
	}
	options := loc1979MorningCanticles[b.rite][slot][weekday]
	if !b.majorFeast() {
		if s := b.morningSeasonalCanticles(slot); s != nil {
			options = s
		}
	}
	return b.tableChoice(key, options)
}

func (b *loc1979) eveningCanticle(key, slot string) string {
	weekday := b.Date.Weekday()
	if b.majorFeast() {
		weekday = 0
	}
	options := loc1979EveningCanticles[b.rite][slot][weekday]
	if slot == "first" && b.Date.Weekday() == 1 && b.seasonDown() == "quaresma" {
		options = []string{"canticle_14"}
	}
	return b.tableChoice(key, options)
}

// tableChoice ports loc1979_table_choice: a paired cell holds the Rite I and
// Rite II forms of one appointment, so "random" keeps the rite's form too.
func (b *loc1979) tableChoice(key string, options []string) string {
	choice := b.presencePref(key)
	if choice == nil {
		choice = "weekday"
	}
	var selected string
	switch rb.ToS(choice) {
	case "weekday", "rotation", "random":
		if b.rite == 1 {
			selected = options[0]
		} else {
			selected = options[len(options)-1]
		}
	default:
		selected = rb.ToS(choice)
	}
	switch selected {
	case "magnificat":
		return "ep1_magnificat"
	case "nunc_dimittis":
		return "ep1_nunc_dimittis"
	}
	return selected
}

func (b *loc1979) morningSeasonalCanticles(slot string) []string {
	season, wd := b.seasonDown(), b.Date.Weekday()
	if slot == "first" && wd == 0 {
		switch season {
		case "advento":
			return []string{"canticle_11"}
		case "quaresma":
			return []string{"canticle_14"}
		case "páscoa":
			return []string{"canticle_8"}
		}
	}
	if slot == "first" && (wd == 3 || wd == 5) && season == "quaresma" {
		return []string{"canticle_14"}
	}
	penitential := season == "advento" || season == "quaresma"
	if slot == "second" && wd == 4 && penitential {
		return []string{"canticle_19"}
	}
	if slot == "second" && wd == 0 && penitential {
		return []string{"canticle_4", "canticle_16"}
	}
	return nil
}

func (b *loc1979) seasonSlug(general string) string {
	switch b.seasonDown() {
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
	}
	return general
}

func (b *loc1979) lordsPrayerSlug() string {
	if b.Pref("lords_prayer_style") == "traditional" {
		return "lords_prayer_traditional"
	}
	return "lords_prayer_contemporary"
}

// --- Morning / Evening Prayer (Rite One and Rite Two) -----------------------------------

func (b *loc1979) names() (office, pref string) {
	if b.OfficeType == "morning" {
		return fmt.Sprintf("mp%d", b.rite), fmt.Sprintf("m%d", b.rite)
	}
	return fmt.Sprintf("ep%d", b.rite), fmt.Sprintf("e%d", b.rite)
}

func (b *loc1979) morning() []*Section {
	steps := []Step{
		One(b.introductorySentences), One(b.confession), One(b.invitatory), One(b.psalms),
		One(b.lessonsAndCanticles), One(b.creed), One(b.prayers),
	}
	if b.rite == 1 {
		steps = append(steps, One(b.riteOneCollects))
	} else {
		steps = append(steps, One(func() *Section {
			c := b.T("mp2_collect_grace")
			if c == nil {
				return nil
			}
			return b.Section("Collects", "general_collects", []*Line{
				b.I(Title(c), "heading"), b.Item(c.Content, "text", c.Slug, ""), b.Spacer(),
			}, nil)
		}))
	}
	return Pipeline(append(steps, One(b.mission), One(b.thanksgiving), One(b.chrysostom), One(b.conclusion))...)
}

func (b *loc1979) evening() []*Section {
	steps := []Step{
		One(b.introductorySentences), One(b.confession), One(b.invitatory), One(b.psalms),
		One(b.lessonsAndCanticles), One(b.creed), One(b.prayers),
	}
	if b.rite == 1 {
		steps = append(steps, One(b.riteOneCollects))
	} else {
		steps = append(steps, One(b.eveningGeneralCollects))
	}
	return Pipeline(append(steps, One(b.mission), One(b.thanksgiving), One(b.chrysostom), One(b.conclusion))...)
}

func (b *loc1979) introductorySentences() *Section {
	op, pref := b.names()
	lines := b.sl(nil, op+"_rubric_sentences", "rubric")
	choice := b.orPref(pref+"_opening_sentence", "seasonal")
	seasonalPrefix := fmt.Sprintf("mp%d", b.rite) // Evening Prayer uses the Morning seasonal sentences
	var pattern string
	if choice == "seasonal" {
		pattern = seasonalPrefix + "_opening_" + b.seasonSlug("general") + "_%"
	} else {
		pattern = op + "_opening_" + rubyInterp(choice) + "%"
	}
	if matching := b.TextsMatching(pattern); len(matching) > 0 {
		s := matching[b.SeededPick(len(matching), pref+"_opening_sentence")]
		lines = append(lines, b.Item(s.Content, "leader", s.Slug, Ref(s)))
	}
	return b.Section("Opening Sentences", "introductory_sentences", lines, nil)
}

func (b *loc1979) confession() *Section {
	op, pref := b.names()
	if rb.Truthy(b.Pref(pref + "_confession_skip")) {
		return nil
	}
	withSpacer := func(lines []*Line, slug, typ string) []*Line {
		if t := b.T(slug); t != nil {
			lines = append(lines, b.Item(t.Content, typ, t.Slug, ""), b.Spacer())
		}
		return lines
	}
	var lines []*Line
	if b.rite == 1 {
		lines = b.sl(lines, op+"_rubric_confession", "rubric")
		lines = withSpacer(lines, op+"_confession_invitation_1", "leader")
		lines = b.sl(lines, op+"_rubric_confession_silence", "rubric")
		lines = withSpacer(lines, op+"_confession_body", "congregation")
		lines = b.sl(lines, op+"_absolution", "leader")
	} else {
		lines = b.sl(lines, op+"_rubric_confession_intro", "rubric")
		lines = withSpacer(lines, "confession_invitation_1", "leader")
		lines = withSpacer(lines, "mp2_rubric_confession_silence", "rubric")
		lines = withSpacer(lines, "confession_prayer_1", "congregation")
		lines = b.sl(lines, "absolution_priest", "leader")
	}
	return b.Section("Confession of Sin", "confession", lines, nil)
}

func (b *loc1979) invitatory() *Section {
	op, pref := b.names()
	lines := []*Line{b.fetch("common_rubric_all_stand", "rubric")}
	lines = b.sl(lines, op+"_inv_v1", "leader")
	lines = b.sl(lines, op+"_inv_r1", "congregation")
	if b.rite == 1 {
		lines = b.pair(lines, "mp1_gloria_patri_v", "mp1_gloria_patri_r")
	} else {
		lines = b.pair(lines, "gloria_patri_minister", "gloria_patri_people")
	}
	if b.seasonDown() != "quaresma" {
		lines = append(lines, b.fetch("common_rubric_alleluia_may_be_added", "rubric"))
	}
	lines = append(lines, b.Spacer())
	if b.OfficeType == "evening" {
		if b.rite == 1 || b.Pref("e2_invitatory_style") != "none" {
			if p := b.T(op + "_phos_hilaron"); p != nil {
				lines = append(lines, b.I(Title(p), "heading"), b.Item(p.Content, "text", p.Slug, ""), b.Spacer())
			}
		}
		return b.Section("Invitatory", "invitatory", lines, nil)
	}
	general := "general_1"
	if b.rite == 1 {
		general = "general_" + rubyInterp(b.orPref("m1_antiphon_general", "1"))
	}
	if a := b.T(op + "_antiphon_" + b.seasonSlug(general)); a != nil {
		lines = append(lines, b.Item(a.Content, "leader", a.Slug, ""), b.Spacer())
	}
	var psalm string
	if b.seasonDown() == "páscoa" {
		psalm = fmt.Sprintf("pascha_nostrum_%d", b.rite)
	} else {
		switch c := b.rotatingChoice(pref+"_invitatory_psalm", []string{"venite", "jubilate"}); c {
		case "venite", "jubilate":
			psalm = fmt.Sprintf("%s_%d", c, b.rite)
		default:
			psalm = c
		}
	}
	if t := b.T(psalm); t != nil {
		lines = append(lines, b.Item(t.Content, "text", t.Slug, ""), b.Spacer())
	}
	return b.Section("Invitatory", "invitatory", lines, nil)
}

func (b *loc1979) psalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.I(p.Reference, "text"), b.Spacer()}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
		lines = append(lines, b.Spacer())
	}
	lines = append(lines, b.fetch("common_rubric_psalms_appointed", "rubric"))
	if b.rite == 1 {
		lines = append(lines, b.fetch("mp1_psalms_gloria_v", "leader"), b.fetch("mp1_psalms_gloria_r", "congregation"))
	} else {
		lines = b.pair(lines, "gloria_patri_minister", "gloria_patri_people")
	}
	return b.Section("Psalms", "psalms", lines, nil)
}

func (b *loc1979) lessonsAndCanticles() *Section {
	_, pref := b.names()
	lines := []*Line{b.fetch("common_rubric_lessons_intro", "rubric")}
	lesson := func(key string) {
		r := b.Readings.Slot(key)
		if r == nil {
			return
		}
		lines = append(lines,
			b.FetchLineItem("reading_announcement", "leader", [][2]string{{"reference", r.Reference}}, "content"),
			b.Spacer())
		if r.Content != nil {
			lines = append(lines, b.BibleContent(r.Content)...)
		}
		lines = append(lines,
			b.fetch("common_word_of_the_lord", "leader"),
			b.fetch("common_thanks_be_to_god", "congregation"),
			b.Spacer())
	}
	canticle := func(slot string) {
		key := pref + "_canticle_1"
		if slot == "second" {
			key = pref + "_canticle_2"
		}
		var slug string
		if b.OfficeType == "morning" {
			slug = b.morningCanticle(key, slot)
		} else {
			slug = b.eveningCanticle(key, slot)
		}
		if c := b.T(slug); c != nil {
			lines = append(lines, b.I(Title(c), "heading"), b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
		}
	}
	lesson("first_reading")
	canticle("first")
	lesson("second_reading")
	canticle("second")
	return b.Section("The Lessons", "readings", lines, nil)
}

func (b *loc1979) creed() *Section {
	op, _ := b.names()
	lines := []*Line{
		b.FetchLineItem("apostles_creed", "heading", nil, "title"),
		b.fetch("common_rubric_officiant_people_all_standing", "rubric"),
	}
	slug := "apostles_creed"
	if b.rite == 1 {
		slug = op + "_apostles_creed"
	}
	lines = b.sl(lines, slug, "congregation")
	return b.Section("The Creed", "creed", lines, nil)
}

func (b *loc1979) prayers() *Section {
	op, _ := b.names()
	greeting := "common_greeting_r1"
	if b.rite == 1 {
		greeting = "common_greeting_r1_traditional"
	}
	lines := []*Line{
		b.fetch("common_prayers_heading", "heading"),
		b.fetch("common_rubric_kneel_stand", "rubric"),
		b.fetch("common_greeting_v1", "leader"),
		b.fetch(greeting, "congregation"),
		b.fetch("common_let_us_pray", "leader"),
		b.Spacer(),
	}
	if b.rite == 2 && b.OfficeType == "evening" {
		kv1, kr1, kv2 := b.T("common_kyrie_v1"), b.T("common_kyrie_r1"), b.T("common_kyrie_v2")
		if kv1 != nil && kr1 != nil && kv2 != nil {
			lines = append(lines,
				b.Item(kv1.Content, "leader", kv1.Slug, ""),
				b.Item(kr1.Content, "congregation", kr1.Slug, ""),
				b.Item(kv2.Content, "leader", kv2.Slug, ""),
				b.Spacer())
		}
	}
	lpSlug := b.lordsPrayerSlug()
	if b.rite == 1 {
		lpSlug = op + "_lords_prayer"
	}
	if lp := b.T(lpSlug); lp != nil {
		lines = append(lines, b.fetch("common_rubric_officiant_people", "rubric"), b.Item(lp.Content, "text", lp.Slug, ""), b.Spacer())
	}
	lines = append(lines, b.suffrages()...)
	if b.rite == 2 {
		lines = append(lines, b.CollectLines(b.Collects)...)
	}
	return b.Section("The Prayers", "prayers", lines, nil)
}

func (b *loc1979) suffrages() []*Line {
	op, pref := b.names()
	var lines []*Line
	if b.rotatingChoice(pref+"_suffrages", []string{"a", "b"}) == "b" {
		if b.OfficeType == "morning" {
			for i := 1; i <= 5; i++ {
				lines = b.sl(lines, fmt.Sprintf("%s_suffrages_b_v%d", op, i), "leader")
				lines = b.sl(lines, fmt.Sprintf("%s_suffrages_b_r%d", op, i), "congregation")
			}
		} else {
			for i := 1; i <= 6; i++ {
				if t := b.T(fmt.Sprintf("%s_suffrages_b_%d", op, i)); t != nil {
					lines = append(lines, b.Item(t.Content, "leader", t.Slug, ""))
					lines = b.sl(lines, op+"_suffrages_b_response", "congregation")
				}
			}
		}
	} else {
		prefix := "suffrages_a"
		if b.rite == 1 {
			prefix = op + "_suffrages_a"
		}
		for i := 1; i <= 7; i++ {
			lines = b.sl(lines, fmt.Sprintf("%s_v%d", prefix, i), "leader")
			lines = b.sl(lines, fmt.Sprintf("%s_r%d", prefix, i), "congregation")
		}
	}
	return append(lines, b.Spacer())
}

// riteOneCollects ports Rite One's build_collects: the Traditional text of
// each collect when the book has one.
func (b *loc1979) riteOneCollects() *Section {
	lines := []*Line{b.fetch("common_rubric_collects_intro", "rubric")}
	list := make([]*rb.Map, len(b.Collects))
	for i, c := range b.Collects {
		list[i] = c
		slug := collects.TraditionalCollectSlug(nonNilS(c.Get("sunday_reference")), nonNilS(c.Get("celebration_name")))
		if slug == "" {
			continue
		}
		if t := b.T(slug); t != nil {
			m := c.Dup()
			m.Set("text", t.Content)
			list[i] = m
		}
	}
	lines = append(lines, b.CollectLines(list)...)
	return b.Section("Collects", "collects", lines, nil)
}

func nonNilS(v any) string {
	if v == nil {
		return ""
	}
	return rb.ToS(v)
}

func (b *loc1979) eveningGeneralCollects() *Section {
	var lines []*Line
	add := func(slug string) {
		if c := b.T(slug); c != nil {
			lines = append(lines, b.I(Title(c), "heading"), b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
		}
	}
	switch b.Date.Weekday() {
	case 0:
		add("ep2_collect_sunday")
	case 5:
		add("ep2_collect_friday")
	case 6:
		add("ep2_collect_saturday")
	}
	for _, k := range b.collectChoices("e2_collects", []string{"peace", "aid", "protection", "presence"}) {
		add("ep2_collect_" + k)
	}
	return b.Section("Collects", "general_collects", lines, nil)
}

func (b *loc1979) mission() *Section {
	op, pref := b.names()
	t := b.T(op + "_mission_prayer_" + b.rotatingChoice(pref+"_mission_prayer", []string{"1", "2", "3"}))
	if t == nil {
		return nil
	}
	return b.Section("Prayer for Mission", "mission", []*Line{b.Item(t.Content, "leader", t.Slug, "")}, nil)
}

func (b *loc1979) thanksgiving() *Section {
	op, pref := b.names()
	slug := op + "_general_thanksgiving"
	if b.rite == 2 {
		if b.Pref(pref+"_show_thanksgiving") == false {
			return nil
		}
		slug = "mp2_general_thanksgiving"
	} else if b.OfficeType == "morning" && b.Pref("m1_show_thanksgiving") == false {
		return nil
	}
	t := b.T(slug)
	if t == nil {
		return nil
	}
	return b.Section("The General Thanksgiving", "thanksgiving", []*Line{
		b.fetch("common_rubric_officiant_people", "rubric"), b.Item(t.Content, "text", t.Slug, ""),
	}, nil)
}

func (b *loc1979) chrysostom() *Section {
	op, pref := b.names()
	slug := op + "_chrysostom"
	if b.rite == 2 {
		if b.Pref(pref+"_show_chrysostom") == false {
			return nil
		}
		slug = "mp2_chrysostom"
	} else if b.OfficeType == "morning" && b.Pref("m1_show_chrysostom") == false {
		return nil
	}
	t := b.T(slug)
	if t == nil {
		return nil
	}
	return b.Section("A Prayer of St. Chrysostom", "chrysostom", []*Line{b.Item(t.Content, "leader", t.Slug, "")}, nil)
}

func (b *loc1979) conclusion() *Section {
	op, pref := b.names()
	var lines []*Line
	v, r := b.T("common_bless_the_lord"), b.T("common_thanks_be_to_god")
	if v != nil && r != nil {
		if s := b.seasonDown(); s == "páscoa" || s == "pentecostes" {
			if e := b.T("common_bless_the_lord_easter"); e != nil {
				v = e
			}
			if e := b.T("common_thanks_be_to_god_easter"); e != nil {
				r = e
			}
		}
		lines = append(lines, b.Item(v.Content, "leader", v.Slug, ""), b.Item(r.Content, "congregation", r.Slug, ""))
	}
	lines = append(lines, b.Spacer())
	lines = b.sl(lines, op+"_grace_"+b.rotatingChoice(pref+"_closing_sentence", []string{"1", "2", "3"}), "leader")
	return b.Section("Conclusion", "conclusion", lines, nil)
}

// --- Daily Devotions for Individuals and Families ----------------------------------------

func (b *loc1979) family(office string) []*Section {
	one := func(slug, name, sectionSlug, typ string, withRef bool) Step {
		return One(func() *Section {
			t := b.T(slug)
			if t == nil {
				return nil
			}
			ref := ""
			if withRef {
				ref = Ref(t)
			}
			return b.Section(name, sectionSlug, []*Line{b.Item(t.Content, typ, t.Slug, ref)}, nil)
		})
	}
	prayers := One(func() *Section {
		lp := b.T(b.lordsPrayerSlug())
		if lp == nil {
			return nil
		}
		return b.Section("Prayers", "prayers", []*Line{b.Item(lp.Content, "congregation", lp.Slug, "")}, nil)
	})
	switch office {
	case "morning":
		return Pipeline(
			one("devotion_morning_opening", "In the Morning", "opening", "text", false),
			one("devotion_morning_reading", "Reading", "reading", "text", true),
			one("devotion_morning_rubric_1", "Devotions", "devotions", "rubric", false),
			prayers,
			one("devotion_morning_collect", "Collect", "collect", "text", false))
	case "evening":
		return Pipeline(
			one("devotion_evening_opening_rubric", "In the Evening", "rubric", "rubric", false),
			one("devotion_evening_opening", "In the Early Evening", "opening", "text", false),
			one("devotion_evening_reading", "Reading", "reading", "text", true),
			one("devotion_evening_rubric_1", "Devotions", "devotions", "rubric", false),
			prayers,
			one("devotion_evening_collect", "Collect", "collect", "text", false))
	case "midday":
		return Pipeline(
			one("devotion_noon_opening", "At Noon", "opening", "text", false),
			one("devotion_noon_reading", "Reading", "reading", "text", true),
			one("devotion_noon_rubric_1", "Devotions", "devotions", "rubric", false),
			prayers,
			One(func() *Section {
				choice := b.orPref("noonday_collect", "1")
				if choice == "random" || choice == "day" {
					choice = "1"
				}
				t := b.T("devotion_noon_collect_" + rubyInterp(choice))
				if t == nil {
					t = b.T("devotion_noon_collect_1")
				}
				if t == nil {
					return nil
				}
				return b.Section("Collect", "collect", []*Line{b.Item(t.Content, "text", t.Slug, "")}, nil)
			}))
	}
	return Pipeline(
		one("devotion_close_opening", "At the Close of Day", "opening", "text", false),
		one("devotion_close_reading", "Reading", "reading", "text", true),
		One(func() *Section {
			t := b.T("devotion_close_optional_nunc")
			if t == nil {
				return nil
			}
			return b.Section("Nunc Dimittis", "nunc_dimittis", []*Line{
				b.fetch("compline_following_may_be_said", "rubric"), b.Item(t.Content, "text", t.Slug, ""),
			}, nil)
		}),
		one("devotion_close_rubric_1", "Devotions", "devotions", "rubric", false),
		prayers,
		one("devotion_close_collect", "Collect", "collect", "text", false),
		one("devotion_close_blessing", "Blessing", "blessing", "leader", false))
}

// --- An Order of Service for Noonday -----------------------------------------------------

func (b *loc1979) midday() []*Section {
	return Pipeline(
		One(func() *Section {
			lines := b.sl(nil, "noonday_inv_v1", "leader")
			lines = b.sl(lines, "noonday_inv_r1", "congregation")
			lines = b.pair(lines, "gloria_patri_minister", "gloria_patri_people")
			if b.seasonDown() != "quaresma" {
				lines = append(lines, b.fetch("common_rubric_alleluia_may_be_added", "rubric"))
			}
			return b.Section("Invitatory", "invitatory", append(lines, b.Spacer()), nil)
		}),
		One(func() *Section {
			choice := b.orPref("noonday_psalm", "121")
			slugs := []string{"noonday_psalm_" + rubyInterp(choice)}
			if choice == "all" {
				slugs = []string{"noonday_psalm_119_105", "noonday_psalm_121", "noonday_psalm_126"}
			}
			return b.fixedPsalms(slugs)
		}),
		One(func() *Section {
			choice := b.orPref("noonday_lesson", "random")
			if choice == "random" {
				choice = b.seededChoice([]string{"1", "2", "3"}, "noonday_lesson")
			}
			return b.fixedLesson("noonday_lesson_" + rubyInterp(choice))
		}),
		One(b.middayPrayers),
		One(func() *Section {
			var lines []*Line
			v, r := b.T("noonday_conclusion_v1"), b.T("noonday_conclusion_r1")
			if v != nil && r != nil {
				if s := b.seasonDown(); s == "páscoa" || s == "pentecostes" {
					if e := b.T("noonday_conclusion_v1_easter"); e != nil {
						v = e
					}
					if e := b.T("noonday_conclusion_r1_easter"); e != nil {
						r = e
					}
				}
				lines = append(lines, b.Item(v.Content, "leader", v.Slug, ""), b.Item(r.Content, "congregation", r.Slug, ""))
			}
			return b.Section("Conclusion", "conclusion", lines, nil)
		}),
	)
}

func (b *loc1979) fixedPsalms(slugs []string) *Section {
	var lines []*Line
	for _, slug := range slugs {
		p := b.T(slug)
		if p == nil {
			continue
		}
		lines = append(lines, b.I(Title(p), "heading"), b.Item(p.Content, "text", p.Slug, Ref(p)), b.Spacer())
	}
	lines = append(lines,
		b.fetch("gloria_patri_minister", "leader"),
		b.fetch("gloria_patri_people", "congregation"),
		b.Spacer())
	return b.Section("Psalms", "psalms", lines, nil)
}

func (b *loc1979) fixedLesson(slug string) *Section {
	var lines []*Line
	if l := b.T(slug); l != nil {
		lines = append(lines,
			b.Item(l.Content, "text", l.Slug, Ref(l)), b.Spacer(),
			b.fetch("common_thanks_be_to_god", "congregation"), b.Spacer())
	}
	return b.Section("Lessons", "readings", lines, nil)
}

func (b *loc1979) lordsPrayerLines() []*Line {
	lp := b.T(b.lordsPrayerSlug())
	if lp == nil {
		return nil
	}
	return []*Line{b.fetch("common_rubric_officiant_people", "rubric"), b.Item(lp.Content, "congregation", lp.Slug, ""), b.Spacer()}
}

func (b *loc1979) middayPrayers() *Section {
	lines := b.sl(nil, "noonday_kyrie_v1", "leader")
	lines = b.sl(lines, "noonday_kyrie_r1", "congregation")
	lines = b.sl(lines, "noonday_kyrie_v2", "leader")
	lines = append(lines, b.Spacer())
	lines = append(lines, b.lordsPrayerLines()...)
	lines = b.sl(lines, "noonday_versicle_v1", "leader")
	lines = b.sl(lines, "noonday_versicle_r1", "congregation")
	lines = b.sl(lines, "common_let_us_pray", "leader")
	lines = append(lines, b.Spacer())
	choice := b.orPref("noonday_collect", "random")
	if choice == "day" {
		lines = append(lines, b.CollectLines(b.Collects)...)
	} else {
		if choice == "random" {
			choice = b.seededChoice([]string{"1", "2", "3", "4"}, "noonday_collect")
		}
		if c := b.T("noonday_collect_" + rubyInterp(choice)); c != nil {
			lines = append(lines, b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
		}
	}
	return b.Section("Prayers", "prayers", lines, nil)
}

// --- An Order for Compline ---------------------------------------------------------------

func (b *loc1979) compline() []*Section {
	return Pipeline(
		One(func() *Section {
			lines := []*Line{b.fetch("compline_opening_1", "leader"), b.Spacer()}
			lines = b.sl(lines, "compline_inv_v1", "leader")
			lines = b.sl(lines, "compline_inv_r1", "congregation")
			return b.Section("Compline", "opening", lines, nil)
		}),
		One(func() *Section {
			lines := b.sl(nil, "compline_confession_invitation", "leader")
			lines = append(lines, b.fetch("common_rubric_silence", "rubric"))
			if body := b.T("compline_confession_body"); body != nil {
				lines = append(lines, b.fetch("common_rubric_officiant_people", "rubric"),
					b.Item(body.Content, "congregation", body.Slug, ""), b.Spacer())
			}
			lines = b.sl(lines, "compline_absolution", "leader")
			return b.Section("Confession of Sin", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.sl(nil, "compline_inv_v2", "leader")
			lines = b.sl(lines, "compline_inv_r2", "congregation")
			lines = b.pair(lines, "gloria_patri_minister", "gloria_patri_people")
			if b.seasonDown() != "quaresma" {
				lines = append(lines, b.fetch("common_rubric_alleluia_may_be_added", "rubric"))
			}
			return b.Section("Invitatory", "invitatory", append(lines, b.Spacer()), nil)
		}),
		One(func() *Section {
			choice := b.orPref("compline_psalm", "all")
			slugs := []string{"compline_psalm_" + rubyInterp(choice)}
			if choice == "all" {
				slugs = []string{"compline_psalm_4", "compline_psalm_31", "compline_psalm_91", "compline_psalm_134"}
			}
			return b.fixedPsalms(slugs)
		}),
		One(func() *Section {
			return b.fixedLesson("compline_lesson_" + b.rotatingChoice("compline_lesson", []string{"1", "2", "3", "4"}))
		}),
		One(b.complinePrayers),
		One(func() *Section {
			antSlug := "compline_antiphon"
			if b.seasonDown() == "páscoa" {
				antSlug = "compline_antiphon_easter"
			}
			ant := b.T(antSlug)
			var lines []*Line
			if ant != nil {
				lines = append(lines, b.Item(ant.Content, "leader", ant.Slug, ""))
			}
			lines = b.sl(lines, "canticle_17", "congregation")
			if ant != nil {
				lines = append(lines, b.Item(ant.Content, "leader", ant.Slug, ""))
			}
			lines = append(lines, b.Spacer(),
				b.fetch("common_bless_the_lord", "leader"), b.fetch("common_thanks_be_to_god", "congregation"), b.Spacer())
			lines = b.sl(lines, "compline_blessing", "leader")
			return b.Section("Conclusion", "conclusion", lines, nil)
		}),
	)
}

func (b *loc1979) complinePrayers() *Section {
	var lines []*Line
	for _, p := range [][2]string{{"compline_versicle_v1", "leader"}, {"compline_versicle_r1", "congregation"},
		{"compline_versicle_v2", "leader"}, {"compline_versicle_r2", "congregation"}} {
		lines = b.sl(lines, p[0], p[1])
	}
	lines = append(lines, b.Spacer())
	for _, p := range [][2]string{{"compline_kyrie_v1", "leader"}, {"compline_kyrie_r1", "congregation"}, {"compline_kyrie_v2", "leader"}} {
		lines = b.sl(lines, p[0], p[1])
	}
	lines = append(lines, b.Spacer())
	lines = append(lines, b.lordsPrayerLines()...)
	lines = b.sl(lines, "compline_versicle_v3", "leader")
	lines = b.sl(lines, "compline_versicle_r3", "congregation")
	lines = b.sl(lines, "common_let_us_pray", "leader")
	lines = append(lines, b.Spacer())
	collect := func(slug string) {
		if c := b.T(slug); c != nil {
			lines = append(lines, b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
		}
	}
	if b.Date.Weekday() == 6 {
		collect("compline_collect_saturday")
	} else {
		choice := b.orPref("compline_collect", "random")
		if choice == "random" {
			choice = b.seededChoice([]string{"1", "2", "3", "4"}, "compline_collect")
		}
		collect("compline_collect_" + rubyInterp(choice))
	}
	if add := b.orPref("compline_additional_prayer", "1"); add != "none" {
		collect("compline_additional_" + rubyInterp(add))
	}
	return b.Section("Prayers", "prayers", lines, nil)
}
