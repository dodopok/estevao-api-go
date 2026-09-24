package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/store"
)

func init() {
	Register("locb_2008", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "evening", "midday", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &locb{family: c.Prefs.Get("family_rite") == true}
		if !b.family {
			tmp := &Base{C: c, Prefs: c.Prefs}
			switch c.OfficeType {
			case "morning":
				b.rite = locbRite(tmp.RiteChoice("morning_prayer_rite", []string{"1", "2", "3", "4"}))
			case "evening":
				b.rite = locbRite(tmp.RiteChoice("evening_prayer_rite", []string{"1", "2", "3", "4"}))
			case "compline":
				b.rite = locbRite(tmp.RiteChoice("compline_prayer_rite", []string{"1", "2"}))
			}
		}
		// Locb2008::Base#reading_for: the Testament sections read the reader's
		// chosen lesson of that Testament.
		b.H.ReadingFor = func(key string) *reading.Passage {
			switch key {
			case "first_reading":
				return firstPassage(b.OfficeLessonsIn("old"))
			case "second_reading":
				return firstPassage(b.OfficeLessonsIn("new"))
			}
			return b.Readings.Slot(key)
		}
		b.Init(ctx, c)
		return b
	})
}

// locbRite maps RITE_NAMES.fetch(rite, RITE_NAMES.fetch(rites.first)).
func locbRite(r string) int {
	switch r {
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	}
	return 1
}

func firstPassage(list []*reading.Passage) *reading.Passage {
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// locb ports DailyOffice::Builders::Locb2008::* (Livro de Oração Comum
// Brasileiro 2008): four rites of Morning and Evening Prayer, two of
// Compline, Midday and the family offices.
type locb struct {
	Base
	family bool
	rite   int
}

func (b *locb) Call() *rb.Map {
	if b.family {
		return b.Render(b.familyOffice())
	}
	switch b.OfficeType {
	case "morning":
		switch b.rite {
		case 2:
			return b.Render(b.morning2())
		case 3:
			return b.Render(b.morning3())
		case 4:
			return b.Render(b.morning4())
		}
		return b.Render(b.morning1())
	case "evening":
		switch b.rite {
		case 2:
			return b.Render(b.evening2())
		case 3:
			return b.Render(b.evening3())
		case 4:
			return b.Render(b.evening4())
		}
		return b.Render(b.evening1())
	case "midday":
		return b.Render(b.midday())
	}
	if b.rite == 2 {
		return b.Render(b.compline2())
	}
	return b.Render(b.compline1())
}

// --- line helpers --------------------------------------------------------------------------

// t appends line_item(text.content, type:, slug: text.slug) when the text exists.
func (b *locb) t(lines []*Line, slug, typ string) []*Line {
	if x := b.T(slug); x != nil {
		lines = append(lines, b.Item(x.Content, typ, x.Slug, ""))
	}
	return lines
}

// ts is t followed by a spacer, both only when the text exists.
func (b *locb) ts(lines []*Line, slug, typ string) []*Line {
	if x := b.T(slug); x != nil {
		lines = append(lines, b.Item(x.Content, typ, x.Slug, ""), b.Spacer())
	}
	return lines
}

// pairs appends the "<prefix>_minister" (leader) / "<prefix>_all"
// (congregation) texts.
func (b *locb) pairs(lines []*Line, prefixes ...string) []*Line {
	for _, p := range prefixes {
		lines = b.t(lines, p+"_minister", "leader")
		lines = b.t(lines, p+"_all", "congregation")
	}
	return lines
}

func (b *locb) empty(name, slug string) *Section { return b.Section(name, slug, []*Line{}, nil) }

func (b *locb) mainPart(name, slug string, lines []*Line) *Section {
	if lines == nil {
		lines = []*Line{}
	}
	return b.Section(name, slug, lines, rb.M("type", "main_part"))
}

func (b *locb) opt(key string, lo, hi int) string {
	return rubyInterp(Or(b.ResolveRange(key, lo, hi), 1))
}

func (b *locb) seededRandom(n int, key string) int { return b.SeededNumber(0, n-1, key) }

// psalms ports Base#rite_psalms_section.
func (b *locb) psalms(prefix string) *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := b.ts(nil, prefix+"_psalms_rubric", "rubric")
	lines = append(lines, b.I(p.Reference, "heading"), b.Spacer())
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
		lines = append(lines, b.Spacer())
	}
	lines = b.pairs(lines, prefix+"_psalms_gloria")
	return b.Section("Salmodia", "psalms", lines, nil)
}

// collectOfTheDay ports RiteOneAndTwo#build_collect_of_the_day.
func (b *locb) collectOfTheDay() *Section {
	return b.Section("", "collect_of_the_day", b.CollectLines(b.Collects), nil)
}

func (b *locb) graceConclusion(lines []*Line, prefix string) []*Line {
	return b.pairs(lines, prefix+"_conclusion_grace")
}

// lessonLines is the format_lines lambda of the readings sections: the
// reference as heading, the text, and the optional response pair.
func (b *locb) lessonLines(readerSlug, allSlug string) func(*reading.Passage) []*Line {
	return b.lessonLinesSp(readerSlug, allSlug, true)
}

// lessonLinesSp: trailingSpacer says whether a spacer follows the response.
func (b *locb) lessonLinesSp(readerSlug, allSlug string, trailingSpacer bool) func(*reading.Passage) []*Line {
	reader, all := b.T(readerSlug), b.T(allSlug)
	return func(r *reading.Passage) []*Line {
		l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			l = append(l, b.BibleContent(r.Content)...)
			l = append(l, b.Spacer())
		}
		if reader != nil {
			l = append(l, b.Item(reader.Content, "reader", reader.Slug, ""))
		}
		if all != nil {
			l = append(l, b.Item(all.Content, "congregation", all.Slug, ""))
			if trailingSpacer {
				l = append(l, b.Spacer())
			}
		}
		return l
	}
}

// testamentReading ports the Old/New Testament reading sections of rites
// II-IV: nothing when the reader keeps no lesson of that Testament.
func (b *locb) testamentReading(testament, name, slug, key string, format func(*reading.Passage) []*Line, pre []*Line) *Section {
	lessons := b.OfficeLessonsIn(testament)
	if len(lessons) == 0 {
		return nil
	}
	lines := pre
	for _, l := range lessons {
		lines = append(lines, format(l)...)
	}
	return b.SectionWithReadingExtras(name, slug, lines, key, nil, format)
}

func (b *locb) seasonDown() string { return strings.ToLower(b.Season()) }

func (b *locb) celebrationNameDown() string {
	if n, ok := b.celebrationField("name").(string); ok {
		return strings.ToLower(n)
	}
	return ""
}

// --- Morning Prayer, Rite One (pp. 25-33) --------------------------------------------------

func (b *locb) morning1() []*Section {
	const p = "morning_1"
	return Pipeline(
		One(func() *Section {
			o := b.opt("morning_1_preparation", 1, 2)
			lines := b.t(nil, p+"_preparation_"+o+"_greeting_minister", "leader")
			lines = b.ts(lines, p+"_preparation_"+o+"_greeting_all", "congregation")
			lines = b.t(lines, p+"_preparation_"+o+"_opening_minister", "leader")
			lines = b.t(lines, p+"_preparation_"+o+"_opening_all", "congregation")
			return b.mainPart("Preparação", "preparation", lines)
		}),
		One(func() *Section {
			w := b.T(p + "_welcome_minister")
			if w == nil {
				return nil
			}
			return b.Section("Acolhida", "welcome", []*Line{b.Item(w.Content, "leader", w.Slug, "")}, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_silence", "rubric")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			lines = b.pairs(lines, p+"_confession_absolution", p+"_thanksgiving_blessed")
			lines = b.t(lines, p+"_thanksgiving_hearts_minister", "leader")
			lines = b.ts(lines, p+"_thanksgiving_hearts_all", "congregation")
			return b.Section("Confissão de Pecados", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_thanksgiving_rubric", "rubric")
			lines = b.pairs(lines, p+"_thanksgiving_prayer")
			return b.Section("Oração de Ação de Graças", "thanksgiving", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_reflection_minister", "leader")
			lines = b.ts(lines, p+"_reflection_silence", "rubric")
			lines = b.pairs(lines, p+"_reflection_prayer")
			return b.Section("Breve Reflexão", "reflection", lines, nil)
		}),
		One(func() *Section { return b.mainPart("A Palavra de Deus", "word_of_god", nil) }),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section {
			var slug string
			switch b.seasonDown() {
			case "advento":
				slug = p + "_ot_canticle_advent"
			case "natal":
				slug = p + "_ot_canticle_christmas"
			case "epifania":
				slug = p + "_ot_canticle_epiphany"
			case "quaresma", "semana santa":
				slug = p + "_ot_canticle_lent"
			case "páscoa":
				slug = p + "_ot_canticle_easter"
			case "pentecostes":
				slug = p + "_ot_canticle_pentecost"
			default:
				slug = p + "_ot_canticle_ordinary"
			}
			c := b.T(slug)
			if c == nil {
				return nil
			}
			lines := []*Line{b.Item(c.Content, "text", c.Slug, ""), b.Spacer()}
			lines = b.pairs(lines, p+"_psalms_gloria")
			return b.Section("Cântico do Antigo Testamento", "ot_canticle", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_readings_rubric", "rubric")
			format := b.lessonLines(p+"_readings_response_reader", p+"_readings_response_all")
			for _, l := range b.OfficeLessons() {
				lines = append(lines, format(l)...)
			}
			return b.SectionWithReadingExtras("Leituras das Escrituras", "readings", lines, "first_reading", nil, format)
		}),
		One(func() *Section { return b.empty("Hino", "hymn") }),
		One(func() *Section {
			lines := b.pairs(nil, p+"_response_awake", p+"_response_dead", p+"_response_above", p+"_response_manifest")
			return b.Section("Responso", "response", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_benedictus_title", "rubric")
			lines = b.ts(lines, p+"_benedictus", "text")
			lines = b.t(lines, p+"_benedictus_gloria", "rubric")
			lines = b.pairs(lines, p+"_psalms_gloria")
			return b.Section("Cântico Evangélico", "benedictus", lines, nil)
		}),
		One(func() *Section { return b.empty("Sermão", "sermon") }),
		One(func() *Section {
			lines := b.ts(nil, p+"_creed_rubric", "rubric")
			c := b.T(p + "_creed_all")
			if c == nil {
				return nil
			}
			lines = append(lines, b.Item(c.Content, "congregation", c.Slug, ""))
			return b.Section("O Credo", "creed", lines, nil)
		}),
		One(func() *Section { return b.empty("Orações", "prayers") }),
		One(func() *Section { return b.empty("Intercessões", "intercessions") }),
		One(b.collectOfTheDay),
		One(func() *Section {
			return b.Section("A Oração do Senhor", "lords_prayer", b.t(nil, "lords_prayer_all", "congregation"), nil)
		}),
		One(func() *Section {
			var lines []*Line
			switch rb.ToS(b.Pref("morning_1_conclusion_type")) {
			case "grace":
				lines = b.graceConclusion(lines, p)
			case "peace":
				lines = b.rePeace(p, true)
			case "dismissal":
				lines = b.t(lines, p+"_conclusion_dismissal_minister", "leader")
				lines = b.t(lines, p+"_conclusion_dismissal_people", "congregation")
				lines = b.t(lines, p+"_conclusion_dismissal_all", "congregation")
			default:
				lines = b.blessingConclusion(p)
			}
			return b.Section("Conclusão", "conclusion", lines, nil)
		}),
	)
}

func (b *locb) blessingConclusion(p string) []*Line {
	lines := b.t(nil, p+"_conclusion_blessing_minister", "leader")
	lines = b.ts(lines, p+"_conclusion_blessing_all", "congregation")
	return b.pairs(lines, p+"_conclusion_blessing_praise")
}

// rePeace ports the peace conclusions; Morning Rite One ends with a second
// exchange, Evening Rite One with the optional final words.
func (b *locb) rePeace(p string, morning bool) []*Line {
	lines := b.t(nil, p+"_conclusion_peace_minister", "leader")
	lines = b.ts(lines, p+"_conclusion_peace_all", "congregation")
	lines = b.t(lines, p+"_conclusion_peace_exchange_minister", "leader")
	if morning {
		lines = b.ts(lines, p+"_conclusion_peace_exchange_all", "congregation")
		return b.t(lines, p+"_conclusion_peace_exchange_minister_2", "leader")
	}
	lines = b.t(lines, p+"_conclusion_peace_exchange_all", "congregation")
	if r := b.T(p + "_conclusion_final_rubric"); r != nil {
		lines = append(lines, b.Spacer(), b.Item(r.Content, "rubric", r.Slug, ""))
	}
	return b.t(lines, p+"_conclusion_final_minister", "leader")
}

// --- Morning Prayer, Rite Two ------------------------------------------------------------

func (b *locb) morning2() []*Section {
	const p = "morning_2"
	format := func(r *reading.Passage) []*Line {
		l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			l = append(l, b.BibleContent(r.Content)...)
			l = append(l, b.Spacer())
		}
		return l
	}
	return Pipeline(
		One(func() *Section { return b.empty("Acolhida", "welcome") }),
		One(func() *Section {
			lines := b.ts(nil, p+"_invitation_rubric", "rubric")
			lines = b.ts(lines, b.morning2InvitationSlug(), "leader")
			return b.Section("Convite à Adoração", "invitation", lines, nil)
		}),
		One(func() *Section {
			o := b.opt("morning_2_confession_invitation", 1, 2)
			lines := b.ts(nil, p+"_confession_invitation_"+o+"_minister", "leader")
			lines = b.ts(lines, p+"_confession_silence", "rubric")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			return b.Section("Confissão de Pecados", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.t(nil, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_absolution_all", "congregation")
			lines = b.ts(lines, p+"_absolution_local_rubric", "rubric")
			lines = b.pairs(lines, p+"_absolution_local")
			return b.Section("Declaração de Perdão", "absolution", lines, nil)
		}),
		One(func() *Section {
			return b.mainPart("Oração Matutina", "morning_prayer_section", b.t(nil, p+"_morning_prayer_rubric", "rubric"))
		}),
		One(func() *Section {
			lines := b.pairs(nil, p+"_response_1", p+"_response_2", p+"_response_3", p+"_response_4")
			return b.Section("Responso", "response", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if x := b.T(p + "_venite_title"); x != nil {
				lines = append(lines, b.Item(Title(x), "text", x.Slug, ""), b.Spacer())
			}
			lines = b.ts(lines, p+"_venite", "text")
			lines = b.pairs(lines, p+"_venite_gloria")
			return b.Section("Venite", "venite", lines, nil)
		}),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section {
			return b.testamentReading("old", "Leitura do Antigo Testamento", "ot_reading", "first_reading", format, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_te_deum_rubric", "rubric")
			lines = b.t(lines, p+"_te_deum", "text")
			return b.Section("Te Deum Laudamus", "te_deum", lines, nil)
		}),
		One(func() *Section {
			return b.testamentReading("new", "Leitura do Novo Testamento", "nt_reading", "second_reading", format, nil)
		}),
		One(func() *Section { return b.empty("Sermão", "sermon") }),
		One(func() *Section {
			benedictus := func(lines []*Line) []*Line {
				lines = b.ts(lines, p+"_benedictus_rubric", "rubric")
				lines = b.ts(lines, p+"_benedictus", "text")
				return b.pairs(lines, p+"_benedictus_gloria")
			}
			jubilate := func(lines []*Line) []*Line {
				lines = b.ts(lines, p+"_jubilate_rubric", "rubric")
				lines = b.ts(lines, p+"_jubilate", "text")
				return b.pairs(lines, p+"_psalms_gloria")
			}
			var lines []*Line
			v := b.ResolveOptions("morning_2_canticle_post_reading", []string{"benedictus", "jubilate_deo"})
			switch x := v.(type) {
			case []any:
				lines = jubilate(append(benedictus(lines), b.Spacer()))
			case string:
				if x == "jubilate_deo" {
					lines = jubilate(lines)
				} else {
					lines = benedictus(lines)
				}
			default:
				lines = benedictus(lines)
			}
			return b.Section("Cântico Evangélico", "canticle_post_reading", lines, nil)
		}),
		One(func() *Section {
			c := b.T(p + "_creed_all")
			if c == nil {
				return nil
			}
			return b.Section("O Credo dos Apóstolos", "creed", []*Line{b.Item(c.Content, "congregation", c.Slug, "")}, nil)
		}),
		One(func() *Section {
			lines := b.t(nil, p+"_prayers_1_minister", "leader")
			lines = b.ts(lines, p+"_prayers_1_all", "congregation")
			lines = b.pairs(lines, p+"_prayers_2")
			lines = b.ts(lines, p+"_prayers_3_minister", "leader")
			lines = b.ts(lines, p+"_lords_prayer_all", "congregation")
			lines = b.pairs(lines, p+"_prayers_4")
			for i := 1; i <= 5; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_suffrage_%d", p, i))
			}
			return b.Section("Orações", "prayers", lines, nil)
		}),
		One(b.collectOfTheDay),
		One(func() *Section {
			return b.Section("A Coleta pela Paz", "collect_peace", b.t(nil, p+"_collect_peace", "leader"), nil)
		}),
		One(func() *Section {
			return b.Section("A Coleta pela Graça", "collect_grace", b.t(nil, p+"_collect_grace", "leader"), nil)
		}),
		One(func() *Section { return b.empty("Hino", "hymn") }),
		One(func() *Section {
			return b.Section("Outras Orações", "other_prayers", b.t(nil, p+"_other_prayers_rubric", "rubric"), nil)
		}),
		One(func() *Section {
			var lines []*Line
			if v, ok := b.ResolveOptions("morning_2_conclusion", []string{"grace", "dismissal"}).(string); ok && v == "dismissal" {
				lines = b.t(lines, p+"_conclusion_dismissal_minister", "leader")
				lines = b.t(lines, p+"_conclusion_dismissal_people", "congregation")
				lines = b.t(lines, p+"_conclusion_dismissal_all", "congregation")
			} else {
				lines = b.graceConclusion(lines, p)
			}
			return b.Section("Conclusão", "conclusion", lines, nil)
		}),
	)
}

func (b *locb) morning2InvitationSlug() string {
	name := b.celebrationNameDown()
	switch {
	case strings.Contains(name, "sexta-feira santa"):
		return "morning_2_invitation_good_friday"
	case strings.Contains(name, "vigília pascal"):
		return "morning_2_invitation_easter_vigil"
	case strings.Contains(name, "ascensão"):
		return "morning_2_invitation_ascension"
	case strings.Contains(name, "trindade"):
		return "morning_2_invitation_trinity"
	}
	switch b.seasonDown() {
	case "advento":
		return "morning_2_invitation_advent"
	case "natal":
		return "morning_2_invitation_christmas"
	case "epifania":
		return "morning_2_invitation_epiphany"
	case "quaresma":
		return fmt.Sprintf("morning_2_invitation_penitential_%d", b.seededRandom(10, "morning_2_invitation_penitential")+1)
	case "semana santa":
		return "morning_2_invitation_holy_week"
	case "páscoa":
		return "morning_2_invitation_easter"
	case "pentecostes":
		return "morning_2_invitation_pentecost"
	}
	return fmt.Sprintf("morning_2_invitation_general_%d", b.seededRandom(2, "morning_2_invitation_general")+1)
}

// --- Evening Prayer, Rite One (pp. 68-74) --------------------------------------------------

func (b *locb) evening1() []*Section {
	const p = "evening_1"
	return Pipeline(
		One(func() *Section {
			o := b.opt("evening_1_preparation", 1, 2)
			lines := b.t(nil, p+"_preparation_"+o+"_greeting_minister", "leader")
			lines = b.ts(lines, p+"_preparation_"+o+"_greeting_all", "congregation")
			lines = b.t(lines, p+"_preparation_"+o+"_opening_minister", "leader")
			lines = b.t(lines, p+"_preparation_"+o+"_opening_all", "congregation")
			lines = b.t(lines, p+"_welcome_rubric", "congregation")
			lines = b.t(lines, p+"_welcome_minister", "congregation")
			lines = b.ts(lines, p+"_confession_rubric", "rubric")
			lines = b.ts(lines, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_silence", "rubric")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			lines = b.pairs(lines, p+"_confession_absolution")
			lines = b.ts(lines, p+"_thanksgiving_rubric", "rubric")
			lines = b.pairs(lines, p+"_thanksgiving_prayer")
			return b.mainPart("Preparação", "preparation", lines)
		}),
		One(func() *Section { return b.empty("Hino", "hymn_opening") }),
		One(func() *Section {
			slug := p + "_canticle_psalm_104"
			switch rb.ToS(b.Pref("evening_1_canticle_post_reading")) {
			case "psalm_141":
				slug = p + "_canticle_psalm_141"
			case "random":
				if b.SeededPick(2, "evening_1_canticle_post_reading") == 1 {
					slug = p + "_canticle_psalm_141"
				}
			}
			var lines []*Line
			if c := b.T(slug); c != nil {
				if c.Title != nil && !rb.BlankString(*c.Title) {
					lines = append(lines, b.I(*c.Title, "heading"))
				}
				lines = append(lines, b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
			}
			lines = b.pairs(lines, p+"_canticle_gloria")
			lines = b.ts(lines, p+"_reflection_minister", "leader")
			lines = b.ts(lines, p+"_reflection_silence", "rubric")
			lines = b.pairs(lines, p+"_reflection_prayer")
			return b.Section("Cântico de Abertura", "opening_canticle", lines, nil)
		}),
		One(func() *Section { return b.mainPart("A Palavra de Deus", "word_of_god", nil) }),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section { return b.empty("Hino ou Cântico", "hymn") }),
		One(func() *Section {
			lines := b.ts(nil, p+"_readings_rubric", "rubric")
			lessons := b.OfficeLessons()
			key := "first_reading"
			if b.Readings.SecondReading != nil {
				key = "second_reading"
			}
			format := b.lessonLines(p+"_readings_response_reader", p+"_readings_response_all")
			for _, l := range lessons {
				lines = append(lines, format(l)...)
			}
			lines = b.ts(lines, p+"_response_rubric", "rubric")
			lines = b.pairs(lines, p+"_response_light", p+"_response_darkness", p+"_response_gloria")
			return b.SectionWithReadingExtras("Leituras Bíblicas", "readings", lines, key, nil, format)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_magnificat_title", "rubric")
			lines = b.ts(lines, p+"_magnificat", "text")
			lines = b.pairs(lines, p+"_magnificat_gloria")
			return b.Section("Cântico Evangélico", "magnificat", lines, nil)
		}),
		One(func() *Section {
			return b.Section("Sermão", "sermon", b.t([]*Line{}, p+"_sermon_rubric", "rubric"), nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_creed_rubric", "rubric")
			c := b.T(p + "_creed_all")
			if c == nil {
				return nil
			}
			lines = append(lines, b.Item(c.Content, "congregation", c.Slug, ""))
			return b.Section("Afirmação de Fé", "creed", lines, nil)
		}),
		One(func() *Section {
			return b.Section("Orações", "prayers", b.t(nil, p+"_prayers_rubric", "rubric"), nil)
		}),
		One(b.collectOfTheDay),
		One(func() *Section {
			o := b.opt("evening_1_lords_prayer_opening", 1, 2)
			lines := b.ts(nil, p+"_lords_prayer_opening_"+o+"_minister", "leader")
			lines = b.t(lines, "lords_prayer_all", "congregation")
			return b.Section("A Oração do Senhor", "lords_prayer", lines, nil)
		}),
		One(func() *Section {
			kind := "blessing"
			switch v := rb.ToS(b.Pref("evening_1_conclusion")); v {
			case "grace", "peace", "blessing":
				kind = v
			case "random":
				kind = []string{"blessing", "grace", "peace"}[b.SeededPick(3, "evening_1_conclusion")]
			}
			var lines []*Line
			name := "A Bênção"
			switch kind {
			case "grace":
				lines, name = b.graceConclusion(nil, p), "A Graça"
			case "peace":
				lines, name = b.rePeace(p, false), "A Paz"
			default:
				lines = b.blessingConclusion(p)
			}
			return b.Section(name, "conclusion", lines, nil)
		}),
	)
}

// choice ports "result = resolve_preference(key, [a, b]); :all if Array;
// result&.to_sym || a": "all", a specific value, or the default.
func (b *locb) choice(key string, options ...string) string {
	switch x := b.ResolveOptions(key, options).(type) {
	case []any:
		return "all"
	case string:
		return x
	}
	return options[0]
}

// canticleBlock appends "<slug>_rubric" (+spacer), the text (+spacer) and
// the Gloria pair under gloriaPrefix.
func (b *locb) canticleBlock(lines []*Line, slug, gloriaPrefix string) []*Line {
	lines = b.ts(lines, slug+"_rubric", "rubric")
	lines = b.ts(lines, slug, "text")
	return b.pairs(lines, gloriaPrefix)
}

// --- Evening Prayer, Rite Two ------------------------------------------------------------

func (b *locb) evening2() []*Section {
	const p = "evening_2"
	twoCanticles := func(key, first, second, slug string) *Section {
		var lines []*Line
		switch b.choice(key, first, second) {
		case second:
			lines = b.canticleBlock(lines, p+"_"+second, p+"_"+second+"_gloria")
		case "all":
			lines = b.canticleBlock(lines, p+"_"+first, p+"_"+first+"_gloria")
			lines = append(lines, b.Spacer())
			lines = b.canticleBlock(lines, p+"_"+second, p+"_"+second+"_gloria")
		default:
			lines = b.canticleBlock(lines, p+"_"+first, p+"_"+first+"_gloria")
		}
		return b.Section("Cântico", slug, lines, nil)
	}
	return Pipeline(
		One(func() *Section {
			lines := b.ts(nil, p+"_introduction_rubric", "rubric")
			lines = b.ts(lines, p+"_introduction_rubric_2", "rubric")
			o := b.opt("evening_2_preparation", 1, 2)
			lines = b.ts(lines, p+"_confession_invitation_"+o+"_minister", "leader")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			lines = b.ts(lines, p+"_absolution_rubric", "rubric")
			lines = b.t(lines, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_absolution_all", "congregation")
			lines = b.ts(lines, p+"_absolution_local_rubric", "rubric")
			lines = b.pairs(lines, p+"_absolution_local")
			return b.mainPart("Introdução", "introduction", lines)
		}),
		One(func() *Section {
			return b.mainPart("Oração Vespertina", "evening_prayer_section", b.t(nil, p+"_response_rubric", "rubric"))
		}),
		One(func() *Section {
			lines := b.pairs(nil, p+"_response_1", p+"_response_2", p+"_response_3", p+"_response_4")
			return b.Section("Responso", "response", lines, nil)
		}),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section {
			f := b.lessonLinesSp(p+"_ot_reading_response_reader", p+"_ot_reading_response_all", false)
			return b.testamentReading("old", "Leitura do Antigo Testamento", "ot_reading", "first_reading", f, nil)
		}),
		One(func() *Section {
			return twoCanticles("evening_2_canticle_post_first_reading", "magnificat", "cantate_domino", "canticle_post_first_reading")
		}),
		One(func() *Section {
			f := b.lessonLinesSp(p+"_nt_reading_response_reader", p+"_nt_reading_response_all", false)
			return b.testamentReading("new", "Leitura do Novo Testamento", "nt_reading", "second_reading", f, nil)
		}),
		One(func() *Section {
			return twoCanticles("evening_2_canticle_post_second_reading", "nunc_dimittis", "deus_misereatur", "canticle_post_second_reading")
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_creed_rubric", "rubric")
			creed := p + "_creed_apostolic"
			switch rb.ToS(b.Pref("evening_2_creed_type")) {
			case "nicene":
				creed = p + "_creed_nicene"
			case "random":
				if b.SeededPick(2, "evening_2_creed_type") == 1 {
					creed = p + "_creed_nicene"
				}
			}
			lines = b.t(lines, creed, "congregation")
			return b.Section("Afirmação de Fé", "creed", lines, nil)
		}),
		One(func() *Section {
			lines := b.t(nil, p+"_prayers_1_minister", "leader")
			lines = b.ts(lines, p+"_prayers_1_all", "congregation")
			lines = b.pairs(lines, p+"_prayers_2")
			lines = b.ts(lines, p+"_prayers_3_minister", "leader")
			lines = b.ts(lines, p+"_lords_prayer_all", "congregation")
			lines = b.pairs(lines, p+"_prayers_4")
			for i := 1; i <= 5; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_suffrage_%d", p, i))
			}
			return b.Section("Orações", "prayers", lines, nil)
		}),
		One(b.collectOfTheDay),
		One(func() *Section {
			return b.Section("A Coleta pela Paz", "collect_peace", b.t(nil, p+"_collect_peace", "leader"), nil)
		}),
		One(func() *Section {
			return b.Section("A Coleta Por Ajuda Contra Todos os Perigos", "collect_dangers", b.t(nil, p+"_collect_dangers", "leader"), nil)
		}),
		One(func() *Section { return b.Section("Hino", "hymn", b.t(nil, p+"_hymn_rubric", "rubric"), nil) }),
		One(func() *Section { return b.Section("Sermão", "sermon", b.t(nil, p+"_sermon_rubric", "rubric"), nil) }),
		One(func() *Section {
			var lines []*Line
			if b.choice("evening_2_conclusion", "grace", "dismissal") == "dismissal" {
				lines = b.ts(lines, p+"_conclusion_dismissal_rubric", "rubric")
				lines = b.t(lines, p+"_conclusion_dismissal_minister", "leader")
				lines = b.t(lines, p+"_conclusion_dismissal_people", "congregation")
				lines = b.t(lines, p+"_conclusion_dismissal_all", "congregation")
			} else {
				lines = b.graceConclusion(lines, p)
			}
			return b.Section("Outras Orações", "other_prayers", lines, nil)
		}),
	)
}

// --- Rite Three (Morning and Evening) ------------------------------------------------------

// canticleName ports format_canticle_name for a cached text (which has no
// subtitle): its title, or "Cântico".
func canticleName(t *store.LiturgicalText) string {
	if t == nil || t.Title == nil {
		return "Cântico"
	}
	return *t.Title
}

// spacedCanticle appends a spacer and the canticle, returning its name.
func (b *locb) spacedCanticle(lines []*Line, slug string) ([]*Line, string) {
	c := b.T(slug)
	if c == nil {
		return lines, "Cântico"
	}
	return append(lines, b.Spacer(), b.Item(c.Content, "text", c.Slug, "")), canticleName(c)
}

// gloriaCanticle appends the canticle (+spacer) and its Gloria pair.
func (b *locb) gloriaCanticle(lines []*Line, slug string) ([]*Line, string) {
	c := b.T(slug)
	if c != nil {
		lines = append(lines, b.Item(c.Content, "text", c.Slug, ""), b.Spacer())
	}
	return b.pairs(lines, slug+"_gloria"), canticleName(c)
}

// titledText appends line_item(t.title), a spacer and the text as typ.
func (b *locb) titledText(lines []*Line, slug, typ string) []*Line {
	if x := b.T(slug); x != nil {
		lines = append(lines, b.I(Title(x), "text"), b.Spacer(), b.Item(x.Content, typ, x.Slug, ""))
	}
	return lines
}

// rite3Reading ports the Rite Three Testament readings: the rubric's title
// as heading, the lessons, and the silence rubric.
func (b *locb) rite3Reading(p, testament, which, readerType, name, slug, key string) *Section {
	var lines []*Line
	if r := b.T(p + "_" + which + "_reading_rubric"); r != nil {
		lines = append(lines, b.I(Title(r), "heading"), b.Spacer())
	}
	reader, all := b.T(p+"_"+which+"_reading_response_reader"), b.T(p+"_"+which+"_reading_response_all")
	format := func(r *reading.Passage) []*Line {
		l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			l = append(l, b.BibleContent(r.Content)...)
			l = append(l, b.Spacer())
		}
		if reader != nil {
			l = append(l, b.Item(reader.Content, readerType, reader.Slug, ""))
		}
		if all != nil {
			l = append(l, b.Item(all.Content, "congregation", all.Slug, ""), b.Spacer())
		}
		return l
	}
	lessons := b.OfficeLessonsIn(testament)
	if len(lessons) == 0 {
		return nil
	}
	for _, l := range lessons {
		lines = append(lines, format(l)...)
	}
	lines = b.t(lines, p+"_silence_rubric", "rubric")
	return b.SectionWithReadingExtras(name, slug, lines, key, nil, format)
}

func (b *locb) rite3Conclusion(p string) *Section {
	o := b.opt(p+"_conclusion", 1, 3)
	lines := b.t(nil, p+"_final_prayer_"+o, "leader")
	lines = b.ts(lines, p+"_other_prayers_rubric", "rubric")
	lines = b.t(lines, p+"_conclusion_dismissal_minister", "leader")
	lines = b.t(lines, p+"_conclusion_dismissal_people", "congregation")
	lines = b.t(lines, p+"_conclusion_dismissal_all", "congregation")
	return b.Section("Conclusão/Despedida", "conclusion", lines, nil)
}

func (b *locb) rite3Collect(p string) *Section {
	lines := b.ts(nil, p+"_collect_rubric", "rubric")
	lines = append(lines, b.CollectLines(b.Collects)...)
	return b.Section("Coleta do Dia", "collect_of_the_day", lines, nil)
}

func (b *locb) morning3() []*Section {
	const p = "morning_3"
	return Pipeline(
		One(func() *Section {
			return b.Section("Acolhida", "welcome", b.t(nil, p+"_welcome_minister", "leader"), nil)
		}),
		One(func() *Section {
			return b.Section("Frase Bíblica", "scripture_sentence", b.t(nil, p+"_scripture_sentence_rubric", "rubric"), nil)
		}),
		One(func() *Section { return b.Section("Hino", "hymn", b.t(nil, p+"_hymn_rubric", "rubric"), nil) }),
		One(func() *Section {
			lines := b.ts(nil, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_alternative_rubric", "rubric")
			lines = b.t(lines, p+"_confession_minister", "leader")
			lines = b.t(lines, p+"_confession_prayer_all", "congregation")
			return b.Section("Convite à Confissão", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.t(nil, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_absolution_all", "congregation")
			lines = b.t(lines, p+"_penitential_omit_rubric", "rubric")
			lines = b.pairs(lines, p+"_response_1", p+"_response_2", p+"_response_3")
			return b.Section("Declaração de Perdão", "absolution", lines, nil)
		}),
		One(func() *Section {
			name := "Cântico"
			lines := b.ts(nil, p+"_canticle_before_reading_rubric", "rubric")
			if b.seasonDown() == "páscoa" {
				if r := b.T(p + "_easter_antiphons_rubric"); r != nil {
					lines = append(lines, b.I(Title(r), "heading"), b.Item(r.Content, "rubric", r.Slug, ""), b.Spacer())
				}
				for i := 1; i <= 3; i++ {
					lines = b.ts(lines, fmt.Sprintf("%s_easter_antiphon_%d", p, i), "text")
				}
				name = "Antífonas da Páscoa"
			} else if b.choice("morning_3_canticle_before_reading", "venite", "jubilate_deo") == "jubilate_deo" {
				lines, name = b.spacedCanticle(lines, p+"_jubilate")
			} else {
				lines, name = b.spacedCanticle(lines, p+"_venite")
			}
			return b.Section(name, "canticle_before_reading", lines, nil)
		}),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section {
			return b.rite3Reading(p, "old", "ot", "leader", "Leitura do Antigo Testamento", "ot_reading", "first_reading")
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_canticle_after_reading_rubric", "rubric")
			var name string
			switch b.choice("morning_3_canticle_after_reading", "benedictus", "benedic_anima_mea", "magna_et_mirabilia") {
			case "benedic_anima_mea":
				lines, name = b.spacedCanticle(lines, p+"_benedic_anima_mea")
			case "magna_et_mirabilia":
				lines, name = b.spacedCanticle(lines, p+"_magna_et_mirabilia")
			default:
				lines, name = b.spacedCanticle(lines, p+"_benedictus")
			}
			return b.Section(name, "canticle_after_first_reading", lines, nil)
		}),
		One(func() *Section {
			return b.rite3Reading(p, "new", "nt", "leader", "Leitura do Novo Testamento", "nt_reading", "second_reading")
		}),
		One(func() *Section { return b.Section("Sermão", "sermon", b.t(nil, p+"_sermon_rubric", "rubric"), nil) }),
		One(func() *Section {
			lines := b.ts(nil, p+"_canticle_after_second_reading_rubric", "rubric")
			var name string
			switch {
			case b.seasonDown() == "quaresma":
				lines = b.ts(lines, p+"_savior_of_the_world_rubric", "rubric")
				lines, name = b.spacedCanticle(lines, p+"_savior_of_the_world")
			case b.choice("morning_3_canticle_after_second_reading", "te_deum", "gloria_in_excelsis") == "gloria_in_excelsis":
				lines, name = b.spacedCanticle(lines, p+"_gloria_in_excelsis")
			default:
				lines, name = b.spacedCanticle(lines, p+"_te_deum")
			}
			return b.Section(name, "canticle_after_second_reading", lines, nil)
		}),
		One(func() *Section {
			lines := b.titledText(nil, p+"_creed_all", "congregation")
			lines = b.ts(lines, p+"_prayers_rubric", "rubric")
			lines = b.ts(lines, p+"_prayers_invitation", "leader")
			lines = b.pairs(lines, p+"_kyrie_1")
			lines = b.t(lines, p+"_kyrie_2_minister", "leader")
			lines = b.titledText(lines, p+"_lords_prayer_all", "congregation")
			if o := b.ResolveRange("morning_3_prayer_conclusion", 1, 2); o == 2 {
				lines = b.ts(lines, p+"_responsory_2_rubric", "rubric")
				lines = b.t(lines, p+"_responsory_2_all", "congregation")
			} else {
				lines = b.ts(lines, p+"_responsory_rubric", "rubric")
				for i := 1; i <= 7; i++ {
					lines = b.pairs(lines, fmt.Sprintf("%s_responsory_1_%d", p, i))
				}
			}
			return b.Section("Credo dos Apóstolos", "creed", lines, nil)
		}),
		One(func() *Section { return b.rite3Collect(p) }),
		One(func() *Section { return b.rite3Conclusion(p) }),
	)
}

func (b *locb) evening3() []*Section {
	const p = "evening_3"
	return Pipeline(
		One(func() *Section {
			lines := b.t(nil, p+"_welcome_minister", "leader")
			lines = b.t(lines, p+"_scripture_sentence_rubric", "rubric")
			lines = b.ts(lines, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_alternative_rubric", "rubric")
			lines = b.t(lines, p+"_confession_minister", "leader")
			lines = b.t(lines, p+"_confession_prayer_all", "congregation")
			lines = b.t(lines, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_absolution_all", "congregation")
			lines = b.t(lines, p+"_penitential_omit_rubric", "rubric")
			lines = b.pairs(lines, p+"_response_1", p+"_response_2", p+"_response_3")
			return b.Section("Acolhida", "welcome", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_canticle_invitatory_rubric", "rubric")
			var name string
			if v, ok := Or(b.ResolveOptions("evening_3_invitating_canticle", []string{"1", "2"}), "1").(string); ok && v == "2" {
				lines, name = b.gloriaCanticle(lines, p+"_phos_hilaron")
			} else {
				lines, name = b.gloriaCanticle(lines, p+"_psalm_134")
			}
			return b.Section(name, "canticle_invitatory", lines, nil)
		}),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section {
			return b.rite3Reading(p, "old", "ot", "reader", "Leitura do Antigo Testamento", "ot_reading", "first_reading")
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_canticle_after_reading_rubric", "rubric")
			var name string
			switch b.choice("evening_3_canticle_post_first_reading", "magnificat", "benedic_anima_mea") {
			case "benedic_anima_mea":
				lines, name = b.gloriaCanticle(lines, p+"_benedic_anima_mea")
			case "all":
				lines, name = b.gloriaCanticle(lines, p+"_magnificat")
				lines = append(lines, b.Spacer())
				lines, _ = b.gloriaCanticle(lines, p+"_benedic_anima_mea")
			default:
				lines, name = b.gloriaCanticle(lines, p+"_magnificat")
			}
			return b.Section(name, "canticle_after_first_reading", lines, nil)
		}),
		One(func() *Section {
			return b.rite3Reading(p, "new", "nt", "leader", "Leitura do Novo Testamento", "nt_reading", "second_reading")
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_sermon_rubric", "rubric")
			var name string
			switch b.choice("evening_3_canticle_post_second_reading", "nunc_dimittis", "canticle_of_christ_glory", "gloria_et_honor") {
			case "canticle_of_christ_glory":
				lines, name = b.gloriaCanticle(lines, p+"_canticle_christ_glory")
			case "gloria_et_honor":
				lines, name = b.gloriaCanticle(lines, p+"_gloria_et_honor")
			case "all":
				lines, name = b.gloriaCanticle(lines, p+"_nunc_dimittis")
				lines = append(lines, b.Spacer())
				lines, _ = b.gloriaCanticle(lines, p+"_canticle_christ_glory")
				lines = append(lines, b.Spacer())
				lines, _ = b.gloriaCanticle(lines, p+"_gloria_et_honor")
			default:
				lines, name = b.gloriaCanticle(lines, p+"_nunc_dimittis")
			}
			return b.Section(name, "canticle_after_second_reading", lines, nil)
		}),
		One(func() *Section {
			lines := b.titledText(nil, p+"_creed_all", "congregation")
			lines = b.ts(lines, p+"_prayers_rubric", "rubric")
			lines = b.pairs(lines, p+"_kyrie_1")
			lines = b.t(lines, p+"_kyrie_2_minister", "leader")
			lines = b.ts(lines, p+"_prayers_invitation", "leader")
			lines = b.t(lines, p+"_lords_prayer_all", "congregation")
			lines = b.ts(lines, p+"_responsory_rubric", "rubric")
			for i := 1; i <= 7; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_responsory_%d", p, i))
			}
			return b.Section("Credo dos Apóstolos", "creed", lines, nil)
		}),
		One(func() *Section { return b.rite3Collect(p) }),
		One(func() *Section { return b.rite3Conclusion(p) }),
	)
}

// --- Rite Four -----------------------------------------------------------------------------

// titledT appends the text's title as heading (when present) and the text.
func (b *locb) titledT(lines []*Line, slug, typ string) []*Line {
	if x := b.T(slug); x != nil {
		if x.Title != nil && !rb.BlankString(*x.Title) {
			lines = append(lines, b.I(*x.Title, "heading"))
		}
		lines = append(lines, b.Item(x.Content, typ, x.Slug, ""))
	}
	return lines
}

// contentPlusReference ports "text.content + ' ' + text.reference" (a nil
// reference raises TypeError, as String#+ does).
func contentPlusReference(t *store.LiturgicalText) string {
	if t.Reference == nil {
		panic(&rb.RubyError{Class: "TypeError", Message: "no implicit conversion of nil into String"})
	}
	return t.Content + " " + *t.Reference
}

func (b *locb) pick(key string, options ...string) string {
	return options[b.seededRandom(len(options), key)]
}

func (b *locb) morning4() []*Section {
	const p = "morning_4"
	gloria := func(lines []*Line) []*Line { return b.pairs(lines, p+"_gloria_patri") }
	// referencedCanticle: reference line, the text and a spacer, then the Gloria.
	referencedCanticle := func(lines []*Line, slug string) []*Line {
		if x := b.T(slug); x != nil {
			if x.Reference != nil && !rb.BlankString(*x.Reference) {
				lines = append(lines, b.I(*x.Reference, "reference"))
			}
			lines = append(lines, b.Item(x.Content, "text", x.Slug, ""), b.Spacer())
		}
		return gloria(lines)
	}
	return Pipeline(
		One(func() *Section {
			lines := b.ts(nil, p+"_opening_rubric", "rubric")
			if s := b.T(b.morning4SentenceSlug()); s != nil {
				lines = append(lines, b.Item(contentPlusReference(s), "leader", s.Slug, ""))
			}
			return b.Section("Sentença de Abertura", "opening_sentence", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_exhortation_rubric", "rubric")
			switch v := b.ResolveOptions("morning_4_confession_type", []string{"long", "short"}).(type) {
			case nil:
				lines = b.t(lines, p+"_exhortation_long", "leader")
			case string:
				if v == "long" || v == "short" {
					lines = b.t(lines, p+"_exhortation_"+v, "leader")
				}
			}
			return b.Section("Exortação à Confissão", "exhortation", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if r := b.T(p + "_confession_rubric"); r != nil {
				if r.Title != nil && !rb.BlankString(*r.Title) {
					lines = append(lines, b.I(*r.Title, "heading"))
				}
				lines = append(lines, b.Item(r.Content, "rubric", r.Slug, ""), b.Spacer())
			}
			lines = b.t(lines, p+"_confession_prayer_all", "congregation")
			return b.Section("Confissão Geral", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_absolution_rubric", "rubric")
			lines = b.t(lines, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_lords_prayer_rubric", "rubric")
			lines = b.t(lines, p+"_lords_prayer_all", "congregation")
			lines = b.ts(lines, p+"_versicles_rubric", "rubric")
			lines = b.t(lines, p+"_versicle_1_minister", "leader")
			lines = b.ts(lines, p+"_versicle_1_all", "congregation")
			lines = b.ts(lines, p+"_versicles_rubric_2", "rubric")
			lines = b.pairs(lines, p+"_versicle_2", p+"_versicle_3")
			lines = b.ts(lines, p+"_venite_rubric", "rubric")
			lines = b.ts(lines, p+"_venite_antiphon_rubric", "rubric")
			lines = b.ts(lines, b.morning4AntiphonSlug(), "antiphon")
			return b.Section("Declaração de Absolvição ou Remissão de Pecados", "absolution", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if v := b.T(p + "_venite"); v != nil {
				if v.Title != nil && !rb.BlankString(*v.Title) {
					lines = append(lines, b.I(*v.Title, "heading"))
				}
				lines = append(lines, b.Item(v.Content, "text", v.Slug, ""), b.Spacer())
			}
			return b.Section("Venite", "venite", gloria(lines), nil)
		}),
		One(func() *Section {
			ps := b.Readings.Psalm
			if ps == nil {
				return nil
			}
			lines := b.ts(nil, p+"_venite_gloria_rubric", "rubric")
			lines = append(lines, b.I(ps.Reference, "heading"), b.Spacer())
			if ps.Content != nil {
				lines = append(lines, b.BibleContent(ps.Content)...)
				lines = append(lines, b.Spacer())
			}
			return b.Section("Salmodia", "psalms", gloria(lines), nil)
		}),
		One(func() *Section {
			if len(b.OfficeLessonsIn("old")) == 0 {
				return nil
			}
			return b.ReadingModule("first", p+"_reading_introduction",
				Rubrics{Pre: p + "_first_reading_rubric", End: p + "_canticle_after_first_reading_rubric"},
				"first_reading", "Primeira Leitura", false)
		}),
		One(func() *Section {
			simple := func(lines []*Line, slug, name string) ([]*Line, string) {
				return b.t(lines, slug, "text"), name
			}
			var lines []*Line
			var name string
			switch b.choice("morning_4_canticle_after_first_reading", "te_deum", "benedictus_es_domine", "benedicite_omnia_opera_domini") {
			case "benedictus_es_domine":
				lines, name = simple(lines, p+"_benedictus_es", "Benedictus es, Domine")
			case "benedicite_omnia_opera_domini":
				lines, name = simple(lines, p+"_benedicite", "Benedicite, omnia opera Domini")
			case "all":
				lines, name = simple(lines, p+"_te_deum", "Te Deum Laudamus")
				lines, _ = simple(append(lines, b.Spacer()), p+"_benedictus_es", "")
				lines, _ = simple(append(lines, b.Spacer()), p+"_benedicite", "")
			default:
				lines, name = simple(lines, p+"_te_deum", "Te Deum Laudamus")
			}
			return b.Section(name, "canticle_after_first_reading", lines, nil)
		}),
		One(func() *Section {
			format := func(r *reading.Passage) []*Line {
				l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
				if r.Content != nil {
					l = append(l, b.BibleContent(r.Content)...)
					l = append(l, b.Spacer())
				}
				return l
			}
			return b.testamentReading("new", "Segunda Leitura", "second_reading", "second_reading", format,
				b.ts(nil, p+"_second_reading_rubric", "rubric"))
		}),
		One(func() *Section {
			var lines []*Line
			name := "Benedictus"
			switch b.choice("morning_4_canticle_after_second_reading", "benedictur", "jubilate_deo") {
			case "jubilate_deo":
				lines, name = referencedCanticle(lines, p+"_jubilate"), "Jubilate Deo"
			case "all":
				lines = referencedCanticle(lines, p+"_benedictus")
				lines = referencedCanticle(append(lines, b.Spacer()), p+"_jubilate")
			default:
				lines = referencedCanticle(lines, p+"_benedictus")
			}
			lines = b.ts(lines, p+"_creed_rubric", "rubric")
			switch b.choice("morning_4_creed_type", "apostolic", "nicene") {
			case "nicene":
				lines = b.ts(lines, p+"_nicene_creed_rubric", "rubric")
				lines = b.titledT(lines, p+"_nicene_creed_all", "congregation")
			case "all":
				lines = b.ts(lines, p+"_creed_all", "congregation")
				lines = b.ts(lines, p+"_nicene_creed_rubric", "rubric")
				lines = b.t(lines, p+"_nicene_creed_all", "congregation")
			default:
				lines = b.t(lines, p+"_creed_all", "congregation")
			}
			lines = b.ts(lines, p+"_suffrages_rubric", "rubric")
			lines = b.pairs(lines, p+"_suffrage_1")
			lines = b.ts(lines, p+"_suffrage_2_minister", "leader")
			lines = b.ts(lines, p+"_lords_prayer_suffrage_rubric", "rubric")
			lines = b.pairs(lines, p+"_suffrage_3", p+"_suffrage_4")
			return b.Section(name, "canticle_after_second_reading", lines, nil)
		}),
		One(func() *Section { return b.rite3Collect(p) }),
		One(func() *Section {
			lines := b.ts(nil, p+"_optional_prayers_rubric", "rubric")
			for _, k := range b.morning4GeneralCollects() {
				switch k {
				case "president_alt":
					lines = b.t(lines, p+"_prayer_president_alt", "leader")
				case "peace", "grace":
					lines = b.titledT(lines, p+"_collect_"+k, "leader")
				case "thanksgiving":
					lines = b.titledT(lines, p+"_general_thanksgiving", "leader")
				default:
					lines = b.titledT(lines, p+"_prayer_"+k, "leader")
				}
				lines = append(lines, b.Spacer())
			}
			lines = b.titledT(lines, p+"_prayer_chrysostom", "leader")
			return b.Section("Orações", "optional_prayers", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if g := b.T(p + "_grace"); g != nil {
				lines = b.titledT(lines, p+"_grace", "leader")
				if g.Reference != nil && !rb.BlankString(*g.Reference) {
					lines = append(lines, b.I(*g.Reference, "reference"))
				}
			}
			return b.Section("Graça", "conclusion", lines, nil)
		}),
	)
}

var locbGeneralCollects = []string{"peace", "grace", "president", "president_alt", "clergy", "humanity", "thanksgiving"}

// morning4GeneralCollects ports resolve_general_collects.
func (b *locb) morning4GeneralCollects() []string {
	switch rb.ToS(b.Pref("morning_4_general_collects")) {
	case "for_peace":
		return []string{"peace"}
	case "for_grace":
		return []string{"grace"}
	case "for_authorities_1":
		return []string{"president"}
	case "for_authorities_2":
		return []string{"president_alt"}
	case "for_clergy":
		return []string{"clergy"}
	case "for_all_humanity":
		return []string{"humanity"}
	case "general_thanksgiving":
		return []string{"thanksgiving"}
	case "all":
		return locbGeneralCollects
	case "random":
		return []string{b.pick("morning_4_general_collect", locbGeneralCollects...)}
	}
	return []string{"peace", "grace"}
}

func (b *locb) morning4SentenceSlug() string {
	if v := b.Pref("morning_4_invitatory"); rb.Present(v) && v != "random" {
		return "morning_4_sentence_general_" + rubyInterp(v)
	}
	switch b.seasonDown() {
	case "advento":
		return "morning_4_sentence_advent_1"
	case "natal":
		return "morning_4_sentence_christmas"
	case "epifania":
		return "morning_4_sentence_epiphany_1"
	case "quaresma", "semana santa":
		return fmt.Sprintf("morning_4_sentence_penitential_%d", b.seededRandom(6, "morning_4_sentence_penitential")+1)
	case "páscoa":
		return b.pick("morning_4_sentence_easter", "morning_4_sentence_easter", "morning_4_sentence_easter_2")
	case "pentecostes":
		return b.pick("morning_4_sentence_pentecost", "morning_4_sentence_pentecost_1", "morning_4_sentence_pentecost_2")
	}
	return fmt.Sprintf("morning_4_sentence_general_%d", b.seededRandom(7, "morning_4_sentence_general")+1)
}

func (b *locb) morning4AntiphonSlug() string {
	name := b.celebrationNameDown()
	if strings.Contains(name, "purificação") || strings.Contains(name, "anunciação") {
		return "morning_4_venite_antiphon_purification"
	}
	if strings.Contains(name, "transfiguração") {
		return "morning_4_venite_antiphon_epiphany"
	}
	switch b.seasonDown() {
	case "advento":
		return "morning_4_venite_antiphon_advent"
	case "natal":
		return "morning_4_venite_antiphon_christmas"
	case "epifania":
		return "morning_4_venite_antiphon_epiphany"
	case "páscoa":
		return "morning_4_venite_antiphon_easter"
	case "ascensão":
		return "morning_4_venite_antiphon_ascension"
	case "pentecostes":
		if strings.Contains(name, "trindade") {
			return "morning_4_venite_antiphon_trinity"
		}
		return "morning_4_venite_antiphon_pentecost"
	}
	return "morning_4_venite_antiphon_feasts"
}

func (b *locb) evening4() []*Section {
	const p = "evening_4"
	// headedCanticle: title heading, reference line and the text.
	headedCanticle := func(lines []*Line, slug, name string) ([]*Line, string) {
		if x := b.T(slug); x != nil {
			if x.Title != nil && !rb.BlankString(*x.Title) {
				lines = append(lines, b.I(*x.Title, "heading"))
			}
			if x.Reference != nil && !rb.BlankString(*x.Reference) {
				lines = append(lines, b.I(*x.Reference, "reference"))
			}
			lines = append(lines, b.Item(x.Content, "text", x.Slug, ""))
		}
		return lines, name
	}
	headedRubric := func(lines []*Line, slug string) []*Line {
		if r := b.T(slug); r != nil {
			if r.Title != nil && !rb.BlankString(*r.Title) {
				lines = append(lines, b.I(*r.Title, "heading"))
			}
			lines = append(lines, b.Item(r.Content, "rubric", r.Slug, ""), b.Spacer())
		}
		return lines
	}
	spacedRubric := func(lines []*Line, slug string) []*Line {
		if r := b.T(slug); r != nil {
			lines = append(lines, b.Spacer(), b.Item(r.Content, "rubric", r.Slug, ""), b.Spacer())
		}
		return lines
	}
	format := func(r *reading.Passage) []*Line {
		l := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			l = append(l, b.BibleContent(r.Content)...)
			l = append(l, b.Spacer())
		}
		return l
	}
	return Pipeline(
		One(func() *Section {
			lines := b.ts(nil, p+"_opening_rubric", "rubric")
			if s := b.T(b.evening4SentenceSlug()); s != nil {
				lines = append(lines, b.Item(s.Content+" "+Ref(s), "leader", s.Slug, ""))
			}
			lines = b.ts(lines, p+"_exhortation_rubric", "rubric")
			switch v := b.ResolveOptions("evening_4_confession_type", []string{"long", "short"}).(type) {
			case nil:
				lines = b.t(lines, p+"_exhortation_short", "leader")
			case string:
				if v == "long" || v == "short" {
					lines = b.t(lines, p+"_exhortation_"+v, "leader")
				}
			}
			return b.Section("Sentença de Abertura", "opening_sentence", lines, nil)
		}),
		One(func() *Section {
			lines := headedRubric(nil, p+"_confession_rubric")
			lines = b.t(lines, p+"_confession_prayer_all", "congregation")
			return b.Section("Confissão Geral", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_absolution_rubric", "rubric")
			lines = b.t(lines, p+"_absolution_minister", "leader")
			lines = b.ts(lines, p+"_absolution_all", "congregation")
			lines = b.ts(lines, p+"_lords_prayer_rubric", "rubric")
			lines = b.t(lines, p+"_lords_prayer_invitation", "leader")
			lines = b.ts(lines, p+"_lords_prayer_all", "congregation")
			lines = b.ts(lines, p+"_versicles_rubric", "rubric")
			lines = b.t(lines, p+"_versicle_1_minister", "leader")
			lines = b.ts(lines, p+"_versicle_1_all", "congregation")
			lines = b.ts(lines, p+"_versicles_rubric_2", "rubric")
			lines = b.pairs(lines, p+"_versicle_2", p+"_versicle_3")
			return b.Section("Declaração de Absolvição ou Remissão de Pecados", "absolution", lines, nil)
		}),
		One(func() *Section {
			ps := b.Readings.Psalm
			if ps == nil {
				return nil
			}
			lines := headedRubric(nil, p+"_psalmody_rubric")
			lines = append(lines, b.I(ps.Reference, "heading"), b.Spacer())
			if ps.Content != nil {
				lines = append(lines, b.BibleContent(ps.Content)...)
				lines = append(lines, b.Spacer())
			}
			if g := b.T(p + "_gloria_in_excelsis_title"); g != nil && g.Title != nil && !rb.BlankString(*g.Title) {
				lines = append(lines, b.I(*g.Title, "heading"))
			}
			lines = b.pairs(lines, p+"_gloria_in_excelsis")
			return b.Section("Salmodia", "psalms", lines, nil)
		}),
		One(func() *Section {
			return b.testamentReading("old", "Primeira Leitura", "first_reading", "first_reading", format,
				headedRubric(nil, p+"_first_reading_rubric"))
		}),
		One(func() *Section {
			var lines []*Line
			var name string
			switch b.choice("evening_4_canticle_after_first_reading", "magnificat", "cantate_domino", "bonum_est_confiteri") {
			case "cantate_domino":
				lines, name = headedCanticle(lines, p+"_cantate_domino", "Cantate Domino")
			case "bonum_est_confiteri":
				lines, name = headedCanticle(lines, p+"_bonum_est_confiteri", "Bonum est confiteri")
			case "all":
				lines, name = headedCanticle(lines, p+"_magnificat", "Magnificat")
				lines, _ = headedCanticle(append(lines, b.Spacer()), p+"_cantate_domino", "")
				lines, _ = headedCanticle(append(lines, b.Spacer()), p+"_bonum_est_confiteri", "")
			default:
				lines, name = headedCanticle(lines, p+"_magnificat", "Magnificat")
			}
			return b.Section(name, "canticle_after_first_reading", lines, nil)
		}),
		One(func() *Section {
			return b.testamentReading("new", "Segunda Leitura", "second_reading", "second_reading", format,
				headedRubric(nil, p+"_second_reading_rubric"))
		}),
		One(func() *Section {
			var lines []*Line
			var name string
			switch b.choice("evening_4_canticle_after_second_reading", "nunc_dimittis", "deus_misereatur", "benedic_anima_mea") {
			case "deus_misereatur":
				lines, name = headedCanticle(lines, p+"_deus_misereatur", "Deus misereatur")
			case "benedic_anima_mea":
				lines, name = headedCanticle(lines, p+"_benedic_anima_mea", "Benedic, anima mea")
			case "all":
				lines, name = headedCanticle(lines, p+"_nunc_dimittis", "Nunc dimittis")
				lines, _ = headedCanticle(append(lines, b.Spacer()), p+"_deus_misereatur", "")
				lines, _ = headedCanticle(append(lines, b.Spacer()), p+"_benedic_anima_mea", "")
			default:
				lines, name = headedCanticle(lines, p+"_nunc_dimittis", "Nunc dimittis")
			}
			lines = spacedRubric(lines, p+"_creed_rubric")
			switch v := b.ResolveOptions("evening_4_creed_type", []string{"apostolic", "nicene"}).(type) {
			case nil:
				lines = b.t(lines, p+"_apostles_creed", "congregation")
			case []any:
				lines = b.ts(lines, p+"_apostles_creed", "congregation")
				lines = b.ts(lines, p+"_nicene_creed_rubric", "rubric")
				lines = b.titledT(lines, p+"_nicene_creed", "congregation")
			case string:
				switch v {
				case "apostolic":
					lines = b.t(lines, p+"_apostles_creed", "congregation")
				case "nicene":
					lines = b.ts(lines, p+"_nicene_creed_rubric", "rubric")
					lines = b.titledT(lines, p+"_nicene_creed", "congregation")
				default:
					lines = b.titledT(lines, p+"_apostles_creed", "congregation")
				}
			}
			lines = spacedRubric(lines, p+"_suffrages_rubric")
			lines = b.pairs(lines, p+"_suffrage_1")
			lines = b.ts(lines, p+"_suffrage_2_minister", "leader")
			lines = b.ts(lines, p+"_suffrages_lords_prayer_rubric", "rubric")
			lines = b.ts(lines, p+"_lords_prayer_all", "congregation")
			for i := 3; i <= 8; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_suffrage_%d", p, i))
			}
			return b.Section(name, "canticle_after_second_reading", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_collect_day_rubric", "rubric")
			lines = append(lines, b.CollectLines(b.Collects)...)
			if x := b.T(p + "_collect_peace"); x != nil {
				lines = b.titledT(lines, p+"_collect_peace", "leader")
				lines = append(lines, b.Spacer())
			}
			lines = b.titledT(lines, p+"_collect_night_dangers", "leader")
			return b.Section("Coleta do Dia", "collect_of_the_day", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			for _, k := range b.evening4GeneralCollects() {
				slug := map[string]string{
					"peace": p + "_collect_peace", "night_dangers": p + "_collect_night_dangers",
					"president": p + "_prayer_president", "clergy": p + "_prayer_clergy_people",
					"humanity": p + "_prayer_humanity", "thanksgiving": p + "_thanksgiving",
				}[k]
				lines = b.titledT(lines, slug, "leader")
				lines = append(lines, b.Spacer())
			}
			lines = b.ts(lines, p+"_antiphon_rubric", "rubric")
			lines = b.titledT(lines, p+"_chrysostom_prayer", "leader")
			return b.Section("Orações", "optional_prayers", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if g := b.T(p + "_grace"); g != nil {
				lines = b.titledT(lines, p+"_grace", "leader")
				if g.Reference != nil && !rb.BlankString(*g.Reference) {
					lines = append(lines, b.I(*g.Reference, "reference"))
				}
			}
			lines = b.t(lines, p+"_grace_all", "congregation")
			return b.Section("Graça", "conclusion", lines, nil)
		}),
	)
}

// evening4GeneralCollects ports Evening Rite Four's resolve_general_collects.
func (b *locb) evening4GeneralCollects() []string {
	v := b.Pref("evening_4_general_collects")
	if v == nil || v == false {
		v = b.Pref("morning_4_general_collects")
	}
	all := []string{"president", "clergy", "humanity", "thanksgiving"}
	switch rb.ToS(v) {
	case "for_peace":
		return []string{"peace"}
	case "against_dangers_of_night":
		return []string{"night_dangers"}
	case "for_authorities":
		return []string{"president"}
	case "for_clergy":
		return []string{"clergy"}
	case "for_all_humanity":
		return []string{"humanity"}
	case "general_thanksgiving":
		return []string{"thanksgiving"}
	case "all":
		return all
	case "random":
		return []string{b.pick("evening_4_general_collect", all...)}
	}
	return nil
}

func (b *locb) evening4SentenceSlug() string {
	if v := b.Pref("evening_4_invitatory"); rb.Present(v) && v != "random" {
		return "evening_4_sentence_general_" + rubyInterp(v)
	}
	switch b.seasonDown() {
	case "advento":
		return "evening_4_sentence_advent"
	case "natal":
		return "evening_4_sentence_christmas"
	case "epifania":
		return "evening_4_sentence_epiphany"
	case "quaresma":
		return b.pick("evening_4_sentence_lent", "evening_4_sentence_lent", "evening_4_sentence_lent_2", "evening_4_sentence_lent_3")
	case "semana santa":
		return "evening_4_sentence_good_friday"
	case "páscoa":
		return b.pick("evening_4_sentence_easter", "evening_4_sentence_easter", "evening_4_sentence_easter_2")
	case "ascensão":
		return "evening_4_sentence_ascension"
	case "pentecostes":
		// day_info[:celebration]&.downcase: Hash has no #downcase.
		if c, ok := b.DayInfo.Get("celebration").(*rb.Map); ok && c != nil {
			panic(&rb.RubyError{Class: "NoMethodError", Message: "undefined method `downcase' for " + rubyHashRef(c) + ":Hash"})
		}
		return b.pick("evening_4_sentence_pentecost", "evening_4_sentence_pentecost_1", "evening_4_sentence_pentecost_2")
	}
	return fmt.Sprintf("evening_4_sentence_general_%d", b.seededRandom(5, "evening_4_sentence_general")+1)
}

// rubyHashRef approximates the receiver in a Ruby 3.2 NoMethodError message:
// a celebration hash inspects past 65 characters, so Ruby prints
// #<Hash:0x...> with the object's (volatile) address instead.
func rubyHashRef(*rb.Map) string { return "#<Hash:0x0000000000000000>" }

// --- Midday -------------------------------------------------------------------------------

// contentPlusTitle ports "text.content + ' ' + text.title".
func contentPlusTitle(t *store.LiturgicalText) string {
	if t.Title == nil {
		panic(&rb.RubyError{Class: "TypeError", Message: "no implicit conversion of nil into String"})
	}
	return t.Content + " " + *t.Title
}

// psalmSections ports the build_psalms of Midday and Compline: one section
// per chosen psalm (Array of the preference), with its own Gloria.
func (b *locb) psalmSections(key string, mapping [][2]string, rubricSlug string) []*Section {
	lookup := map[string]string{}
	keys := make([]string, len(mapping))
	for i, m := range mapping {
		lookup[m[0]], keys[i] = m[1], m[0]
	}
	var rubric *store.LiturgicalText
	if rubricSlug != "" {
		rubric = b.T(rubricSlug)
	}
	var sections []*Section
	for index, k := range Arr(b.ResolveOptions(key, keys)) {
		slug, ok := lookup[rb.ToS(k)]
		if !ok {
			continue
		}
		ps := b.T(slug)
		if ps == nil {
			continue
		}
		var lines []*Line
		if index == 0 && rubric != nil {
			lines = append(lines, b.Item(rubric.Content, "rubric", rubric.Slug, ""), b.Spacer())
		}
		lines = append(lines, b.Item(ps.Content, "responsive", ps.Slug, ""), b.Spacer())
		lines = b.pairs(lines, slug+"_gloria")
		var name any = "Salmo"
		if ps.Title != nil {
			name = *ps.Title
		}
		sections = append(sections, b.Section(name, slug, lines, rb.M("type", "canticle", "reference", ptrVal(ps.Reference))))
	}
	return sections
}

func (b *locb) numbered(key string, hi int, prefix string, lines []*Line, each func([]*Line, *store.LiturgicalText) []*Line) []*Line {
	for _, n := range Arr(Or(b.ResolveRange(key, 1, hi), 1)) {
		if t := b.T(prefix + rubyInterp(n)); t != nil {
			lines = each(lines, t)
		}
	}
	return lines
}

func (b *locb) midday() []*Section {
	return Pipeline(
		One(func() *Section {
			lines := b.t(nil, "midday_opening_minister", "leader")
			lines = b.ts(lines, "midday_opening_all", "congregation")
			lines = b.pairs(lines, "midday_gloria")
			if s := b.seasonDown(); s != "advento" && s != "quaresma" {
				lines = b.t(lines, "midday_alleluia", "congregation")
			}
			lines = append(lines, b.Spacer())
			lines = b.t(lines, "midday_rubric_alleluia", "rubric")
			return b.Section("Abertura", "opening", lines, nil)
		}),
		Many(func() []*Section {
			return b.psalmSections("midday_psalm", [][2]string{
				{"psalm_119", "midday_psalm_119"}, {"psalm_121", "midday_psalm_121"}, {"psalm_126", "midday_psalm_126"},
			}, "")
		}),
		One(func() *Section {
			lines := b.ts(nil, "midday_readings_rubric", "rubric")
			lines = b.numbered("midday_reading", 3, "midday_reading_", lines, func(l []*Line, t *store.LiturgicalText) []*Line {
				l = append(l, b.Item(contentPlusReference(t), "leader", t.Slug, ""))
				l = b.t(l, t.Slug+"_response_all", "congregation")
				return append(l, b.Spacer())
			})
			lines = b.t(lines, "midday_meditation_rubric", "rubric")
			return b.Section("Leitura das Escrituras", "reading", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, "midday_prayers_rubric", "rubric")
			lines = b.t(lines, "midday_kyrie_minister", "leader")
			lines = b.t(lines, "midday_kyrie_all", "congregation")
			lines = b.t(lines, "midday_kyrie_minister_2", "leader")
			lines = b.ts(lines, "midday_lords_prayer_all", "congregation")
			lines = b.t(lines, "midday_versicle_minister", "leader")
			lines = b.t(lines, "midday_versicle_all", "congregation")
			if o := b.T("midday_oremos"); o != nil {
				lines = append(lines, b.Spacer(), b.Item(o.Content, "leader", o.Slug, ""))
			}
			return b.Section("Orações", "prayers", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, "midday_collects_rubric", "rubric")
			lines = b.numbered("midday_collect", 4, "midday_collect_", lines, func(l []*Line, t *store.LiturgicalText) []*Line {
				return append(l, b.Item(t.Content, "leader", t.Slug, ""), b.Spacer())
			})
			lines = append(lines, b.plainCollects()...)
			return b.Section("Coleta do Dia", "collect_of_the_day", lines, nil)
		}),
		One(func() *Section {
			lines := b.t(nil, "midday_intercessions_rubric_1", "rubric")
			lines = b.t(lines, "midday_intercessions_rubric_2", "rubric")
			lines = b.pairs(lines, "midday_dismissal")
			return b.Section("Intercessões e Conclusão", "intercessions", lines, nil)
		}),
	)
}

// plainCollects: each collect's preface and text as leader lines, then a spacer.
func (b *locb) plainCollects() []*Line {
	var lines []*Line
	for _, c := range b.Collects {
		if v := c.Get("preface"); rb.Present(v) {
			lines = append(lines, b.I(v, "leader"))
		}
		lines = append(lines, b.I(c.Get("text"), "leader"), b.Spacer())
	}
	return lines
}

// --- Compline ------------------------------------------------------------------------------

// titleHeadingSpacer ports "if rubric: heading (title present) + spacer".
func (b *locb) titleHeadingSpacer(lines []*Line, slug string) []*Line {
	if r := b.T(slug); r != nil {
		if r.Title != nil && !rb.BlankString(*r.Title) {
			lines = append(lines, b.I(*r.Title, "heading"))
		}
		lines = append(lines, b.Spacer())
	}
	return lines
}

// headedRubricSp ports "if rubric: heading (title present), rubric, spacer".
func (b *locb) headedRubricSp(lines []*Line, slug string) []*Line {
	if r := b.T(slug); r != nil {
		if r.Title != nil && !rb.BlankString(*r.Title) {
			lines = append(lines, b.I(*r.Title, "heading"))
		}
		lines = append(lines, b.Item(r.Content, "rubric", r.Slug, ""), b.Spacer())
	}
	return lines
}

func (b *locb) easterSeason() bool {
	s := b.seasonDown()
	return s == "páscoa" || s == "pascoa" || s == "easter"
}

func (b *locb) compline1() []*Section {
	const p = "compline_1"
	return Pipeline(
		One(func() *Section {
			lines := b.t(nil, p+"_opening_minister", "leader")
			lines = b.ts(lines, p+"_opening_all", "congregation")
			lines = b.ts(lines, p+"_brief_lesson_rubric", "rubric")
			lines = b.numbered("compline_1_brief_lesson", 3, p+"_brief_lesson_", lines, func(l []*Line, t *store.LiturgicalText) []*Line {
				return append(l, b.Item(contentPlusTitle(t), "leader", t.Slug, ""), b.Spacer())
			})
			lines = b.ts(lines, p+"_confession_rubric", "rubric")
			lines = b.ts(lines, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			lines = b.pairs(lines, p+"_absolution")
			return b.Section("Abertura", "opening", lines, nil)
		}),
		One(func() *Section {
			lines := b.ts(nil, p+"_hymn_rubric", "rubric")
			h := b.T(p + "_hymn_te_lucis")
			if h != nil {
				lines = append(lines, b.Item(h.Content, "text", h.Slug, ""))
			}
			need(h, "title")
			var name any = "Hino"
			if h.Title != nil {
				name = *h.Title
			}
			return b.Section(name, "hymn", lines, rb.M("type", "canticle"))
		}),
		Many(func() []*Section {
			return b.psalmSections("compline_1_psalm", [][2]string{
				{"psalm_4", p + "_psalm_4"}, {"psalm_16", p + "_psalm_16"}, {"psalm_17", p + "_psalm_17"},
				{"psalm_31", p + "_psalm_31"}, {"psalm_91", p + "_psalm_91"}, {"psalm_134", p + "_psalm_134"},
				{"psalm_139", p + "_psalm_139"},
			}, p+"_psalmody_rubric")
		}),
		One(func() *Section {
			lines := b.headedRubricSp(nil, p+"_word_of_god_rubric")
			lines = b.ts(lines, p+"_word_of_god_references", "leader")
			lines = b.t(lines, p+"_word_of_god_response_reader", "reader")
			lines = b.t(lines, p+"_word_of_god_response_all", "congregation")
			lines = b.ts(lines, p+"_nunc_dimittis_rubric", "rubric")
			lines = b.ts(lines, p+"_nunc_dimittis_antiphon_all", "congregation")
			lines = b.ts(lines, p+"_nunc_dimittis", "text")
			lines = b.t(lines, p+"_nunc_dimittis_gloria_minister", "leader")
			lines = b.ts(lines, p+"_nunc_dimittis_gloria_all", "congregation")
			lines = b.t(lines, p+"_nunc_dimittis_antiphon_repeat_all", "congregation")
			lines = b.t(lines, p+"_kyrie_minister", "leader")
			lines = b.t(lines, p+"_kyrie_all", "congregation")
			lines = b.ts(lines, p+"_kyrie_minister_2", "leader")
			lines = b.t(lines, p+"_lords_prayer_all", "congregation")
			lines = b.ts(lines, p+"_responsory_rubric", "rubric")
			for i := 1; i <= 4; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_responsory_%d", p, i))
			}
			lines = b.ts(lines, p+"_final_prayers_rubric", "rubric")
			lines = b.numbered("compline_1_final_prayer", 6, p+"_final_prayer_", lines, func(l []*Line, t *store.LiturgicalText) []*Line {
				return append(l, b.Item(t.Content, "leader", t.Slug, ""), b.Spacer())
			})
			if b.easterSeason() {
				lines = b.ts(lines, p+"_easter_prayer_rubric", "rubric")
				lines = b.t(lines, p+"_easter_prayer", "leader")
			}
			return b.Section("A Palavra de Deus", "word_of_god", lines, nil)
		}),
		One(func() *Section {
			var lines []*Line
			if o := Or(b.ResolveRange("compline_1_conclusion", 1, 2), 1); o == 2 {
				lines = b.t(lines, p+"_conclusion_2_minister", "leader")
				lines = b.t(lines, p+"_conclusion_2_people", "congregation")
				lines = b.t(lines, p+"_conclusion_2_all", "congregation")
			} else {
				lines = b.pairs(lines, p+"_conclusion_1")
				lines = b.t(lines, p+"_conclusion_1_minister_2", "leader")
				lines = b.ts(lines, p+"_conclusion_1_all_2", "congregation")
				if b.easterSeason() {
					lines = b.ts(lines, p+"_conclusion_easter_rubric", "rubric")
				}
				lines = b.pairs(lines, p+"_conclusion_blessing")
			}
			return b.Section("Conclusão", "conclusion", lines, nil)
		}),
	)
}

func (b *locb) compline2() []*Section {
	const p = "compline_2"
	return Pipeline(
		One(func() *Section {
			lines := b.ts(nil, p+"_opening_rubric", "rubric")
			lines = b.t(lines, p+"_opening_minister", "leader")
			lines = b.ts(lines, p+"_opening_all", "congregation")
			lines = b.titleHeadingSpacer(lines, p+"_brief_lesson_rubric")
			lines = b.t(lines, p+"_brief_lesson_minister", "leader")
			lines = b.t(lines, p+"_brief_lesson_all", "congregation")
			lines = b.t(lines, p+"_brief_lesson_minister_2", "leader")
			lines = b.ts(lines, p+"_brief_lesson_all_2", "congregation")
			lines = b.titleHeadingSpacer(lines, p+"_confession_title")
			lines = b.t(lines, p+"_confession_invitation_minister", "leader")
			lines = b.ts(lines, p+"_confession_prayer_all", "congregation")
			lines = b.titleHeadingSpacer(lines, p+"_absolution_rubric")
			lines = b.pairs(lines, p+"_absolution")
			lines = b.t(lines, p+"_absolution_minister_2", "leader")
			lines = b.t(lines, p+"_absolution_all_2", "congregation")
			lines = b.t(lines, p+"_absolution_minister_3", "leader")
			lines = b.t(lines, p+"_absolution_all_3", "congregation")
			lines = b.pairs(lines, p+"_gloria")
			return b.Section("Abertura", "opening", lines, nil)
		}),
		Many(func() []*Section {
			return b.psalmSections("compline_2_psalm", [][2]string{
				{"psalm_4", p + "_psalm_4"}, {"psalm_91", p + "_psalm_91"}, {"psalm_134", p + "_psalm_134"},
			}, "")
		}),
		One(func() *Section {
			lines := b.headedRubricSp(nil, p+"_hymn_rubric")
			h := b.T(p + "_hymn_te_lucis")
			var name any = "Hino"
			if h != nil {
				lines = append(lines, b.Item(h.Content, "text", h.Slug, ""))
				if h.Title != nil {
					name = *h.Title
				}
			}
			return b.Section(name, "hymn", lines, rb.M("type", "canticle"))
		}),
		One(func() *Section {
			lines := b.headedRubricSp(nil, p+"_reading_rubric")
			lines = b.ts(lines, p+"_reading_rubric_response", "rubric")
			lines = b.t(lines, p+"_reading_response_reader", "reader")
			lines = b.ts(lines, p+"_reading_response_all", "congregation")
			lines = b.titleHeadingSpacer(lines, p+"_responsory_rubric")
			for i := 1; i <= 4; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_responsory_%d", p, i))
			}
			lines = append(lines, b.Spacer())
			lines = b.titleHeadingSpacer(lines, p+"_kyrie_rubric")
			lines = b.t(lines, p+"_kyrie_minister", "leader")
			lines = b.t(lines, p+"_kyrie_all", "congregation")
			lines = b.ts(lines, p+"_kyrie_minister_2", "leader")
			lines = b.ts(lines, p+"_lords_prayer_all", "congregation")
			for i := 1; i <= 4; i++ {
				lines = b.pairs(lines, fmt.Sprintf("%s_versicle_%d", p, i))
			}
			lines = append(lines, b.Spacer())
			lines = b.ts(lines, p+"_final_prayers_rubric", "rubric")
			lines = b.numbered("compline_2_final_prayer", 5, p+"_final_prayer_", lines, func(l []*Line, t *store.LiturgicalText) []*Line {
				return append(l, b.Item(t.Content, "leader", t.Slug, ""), b.Spacer())
			})
			lines = b.ts(lines, p+"_final_prayers_response_all", "congregation")
			lines = b.titleHeadingSpacer(lines, p+"_nunc_dimittis_rubric")
			lines = b.ts(lines, p+"_nunc_dimittis", "text")
			lines = b.t(lines, p+"_nunc_dimittis_gloria_minister", "leader")
			lines = b.ts(lines, p+"_nunc_dimittis_gloria_all", "congregation")
			lines = b.t(lines, p+"_nunc_dimittis_antiphon_repeat_all", "congregation")
			lines = b.t(lines, p+"_conclusion_versicle_minister", "leader")
			lines = b.ts(lines, p+"_conclusion_versicle_all", "congregation")
			lines = b.pairs(lines, p+"_conclusion_1")
			lines = b.t(lines, p+"_conclusion_2_minister", "leader")
			lines = b.ts(lines, p+"_conclusion_2_all", "congregation")
			lines = b.pairs(lines, p+"_blessing", p+"_final_blessing")
			return b.Section("A Palavra de Deus", "word_of_god", lines, nil)
		}),
	)
}

// --- Family offices -----------------------------------------------------------------------

func (b *locb) familyOffice() []*Section {
	var steps []Step
	o := b.OfficeType
	psalm := map[string][2]string{
		"morning": {"family_locb_morning_psalm_51", "Salmo 51"}, "midday": {"family_locb_midday_psalm_113", "Salmo 113"},
		"evening": {"family_locb_evening_opening", "Luz animadora"}, "compline": {"family_locb_compline_psalm_134", "Salmo 134"},
	}[o]
	numFixed := map[string]int{"morning": 1, "midday": 2, "evening": 1, "compline": 2}[o]
	if o == "evening" {
		steps = append(steps, One(func() *Section {
			r := b.T("family_locb_evening_rubric")
			if r == nil {
				return nil
			}
			return b.Section("Ao Entardecer", "opening", []*Line{b.Item(r.Content, "rubric", r.Slug, "")}, nil)
		}))
	}
	readingSlug := "reading"
	if o == "compline" {
		readingSlug = "word_of_god"
	}
	steps = append(steps,
		One(func() *Section { return b.familyPsalm("family_locb_"+o+"_psalm", psalm[0], psalm[1]) }),
		Many(func() []*Section {
			return b.familyReading("family_locb_"+o+"_reading", "family_locb_"+o+"_reading_fixed", readingSlug)
		}))
	if o == "compline" {
		steps = append(steps, One(func() *Section {
			rubric, canticle := b.T("family_locb_compline_nunc_dimittis_rubric"), b.T("family_locb_compline_nunc_dimittis")
			if canticle == nil {
				return nil
			}
			var lines []*Line
			if rubric != nil {
				lines = append(lines, b.Item(rubric.Content, "rubric", rubric.Slug, ""), b.Spacer())
			}
			lines = append(lines, b.Item(canticle.Content, "responsive", canticle.Slug, ""))
			return b.Section("Nunc Dimittis", "canticle", lines, nil)
		}))
	}
	steps = append(steps,
		One(func() *Section {
			t := b.T("lords_prayer_traditional_all")
			if t == nil {
				t = b.T("lords_prayer_all")
			}
			if t == nil {
				t = b.T("our_father")
			}
			if t == nil {
				return nil
			}
			return b.Section("Pai Nosso", "lords_prayer", []*Line{b.Item(t.Content, "congregation", t.Slug, "")}, nil)
		}),
		One(func() *Section {
			return b.familyCollect("family_locb_"+o+"_collect", "family_locb_"+o+"_collect_fixed", numFixed)
		}))
	return Pipeline(steps...)
}

func (b *locb) familyPsalm(key, fixedSlug, fixedName string) *Section {
	if b.PrefS(key) == "daily" && b.Readings.Psalm != nil {
		p := b.Readings.Psalm
		lines := []*Line{b.I(p.Reference, "heading"), b.Spacer()}
		if p.Content != nil {
			lines = append(lines, b.BibleContent(p.Content)...)
		}
		return b.Section(p.Reference, "psalms", lines, nil)
	}
	t := b.T(fixedSlug)
	if t == nil {
		return nil
	}
	var name any = fixedName
	if t.Title != nil {
		name = *t.Title
	}
	return b.Section(name, "psalms", []*Line{b.Item(t.Content, "responsive", t.Slug, "")}, nil)
}

func (b *locb) familyReading(key, fixedSlug, sectionSlug string) []*Section {
	var rubric []*Line
	if r := b.T("family_locb_reading_rubric"); r != nil {
		rubric = []*Line{b.Spacer(), b.Item(r.Content, "rubric", r.Slug, "")}
	}
	module := func(r *reading.Passage, rubricLines []*Line, label string) *Section {
		if r == nil || rb.BlankString(r.Reference) {
			return nil
		}
		lines := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
		if r.Content != nil {
			lines = append(lines, b.BibleContent(r.Content)...)
		}
		lines = append(lines, rubricLines...)
		if label == "" {
			label = "Leitura"
		}
		return b.Section(label, sectionSlug, lines, nil)
	}
	var result []*Section
	lectionary := true
	switch b.PrefS(key) {
	case "old_testament":
		result = compactSections(module(b.Readings.FirstReading, rubric, ""))
	case "new_testament":
		result = compactSections(module(b.Readings.SecondReading, rubric, ""))
	case "gospel":
		result = compactSections(module(b.Readings.Gospel, rubric, ""))
	case "vary":
		k := []string{"first_reading", "second_reading", "gospel"}[b.SeededNumber(0, 2, key)]
		result = compactSections(module(b.Readings.Slot(k), rubric, ""))
	case "all":
		result = compactSections(
			module(b.Readings.FirstReading, rubric, "Antigo Testamento"),
			module(b.Readings.SecondReading, nil, "Novo Testamento"),
			module(b.Readings.Gospel, nil, "Evangelho"))
	default:
		lectionary = false
	}
	if lectionary && len(result) > 0 {
		return result
	}
	t := b.T(fixedSlug)
	if t == nil {
		return nil
	}
	lines := []*Line{b.Item(t.Content, "leader", t.Slug, "")}
	if t.Reference != nil && !rb.BlankString(*t.Reference) {
		lines = append(lines, b.FetchLineItem("reading_citation", "citation", [][2]string{{"reference", *t.Reference}}, "content"))
	}
	lines = append(lines, rubric...)
	return []*Section{b.Section("Leitura", sectionSlug, lines, nil)}
}

func (b *locb) familyCollect(key, base string, numFixed int) *Section {
	var lines []*Line
	switch b.PrefS(key) {
	case "daily":
		if len(b.Collects) == 0 {
			return nil
		}
		lines = b.plainCollects()
	case "fixed_2":
		t := b.T(base + "_2")
		if t == nil {
			return nil
		}
		lines = []*Line{b.Item(t.Content, "leader", t.Slug, "")}
	case "vary":
		t := b.T(fmt.Sprintf("%s_%d", base, b.SeededNumber(1, numFixed, key)))
		if t == nil {
			return nil
		}
		lines = []*Line{b.Item(t.Content, "leader", t.Slug, "")}
	default:
		slug := base
		if numFixed > 1 {
			slug = base + "_1"
		}
		t := b.T(slug)
		if t == nil {
			return nil
		}
		lines = []*Line{b.Item(t.Content, "leader", t.Slug, "")}
	}
	return b.Section("Coleta", "collect_of_the_day", lines, nil)
}
