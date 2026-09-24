package dailyoffice

import (
	"context"
	"strconv"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

func init() {
	Register("loc_1991_pt", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "evening", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc1991{}
		b.Init(ctx, c)
		return b
	})
}

// loc1991 ports DailyOffice::Builders::Loc1991Pt::* (Lusitanian Church).
type loc1991 struct{ Base }

func (b *loc1991) Call() *rb.Map {
	switch b.OfficeType {
	case "morning":
		return b.Render(b.morning())
	case "evening":
		return b.Render(b.evening())
	}
	return b.Render(b.compline())
}

// --- Base --------------------------------------------------------------------------------

func (b *loc1991) easterDay() bool { return b.Date == b.Calendar().Easter.EasterDate }

func (b *loc1991) slugLine(t *store.LiturgicalText, typ string) *Line {
	return b.Item(t.Content, typ, t.Slug, "")
}

// textLines ports text_lines: the text's line, or nothing.
func (b *loc1991) textLines(slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		return []*Line{b.slugLine(t, typ)}
	}
	return nil
}

func (b *loc1991) wordOfTheLordLines() []*Line {
	rSlug := "word_of_the_lord_r"
	if b.OfficeType == "morning" {
		rSlug = "word_of_the_lord_morning_r"
	}
	return append(b.textLines("word_of_the_lord_v", "leader"), b.textLines(rSlug, "congregation")...)
}

func (b *loc1991) gloriaLine() *Line {
	if g := b.T("gloria_patri"); g != nil {
		return b.slugLine(g, "text")
	}
	return nil
}

func (b *loc1991) canticleModule(slug, moduleSlug string) *Section {
	c := b.T(slug)
	if c == nil {
		return nil
	}
	var name any = "Cântico"
	if c.Title != nil {
		name = *c.Title
	}
	return b.Section(name, moduleSlug, []*Line{b.slugLine(c, "text")}, nil)
}

func (b *loc1991) lessonModule(key, name, slug string) *Section {
	r := b.Readings.Slot(key)
	if r == nil {
		return nil
	}
	lines := []*Line{
		b.FetchLineItem("reading_announcement", "leader", [][2]string{{"reference", r.Reference}}, "content"),
		b.Spacer(),
	}
	if r.Content != nil {
		lines = append(lines, b.BibleContent(r.Content)...)
	}
	lines = append(lines, b.Spacer())
	lines = append(lines, b.wordOfTheLordLines()...)
	return b.Section(name, slug, lines, rb.M("reference", r.Reference))
}

// pickOption ports pick_option: nil or empty falls back to the default, an
// Array yields its first element.
func (b *loc1991) pickOption(key string, options []string, def string) string {
	v := b.ResolveOptions(key, options)
	switch x := v.(type) {
	case nil:
		return def
	case string:
		if x == "" {
			return def
		}
		return x
	case []any:
		if len(x) == 0 {
			return def
		}
		return rb.ToS(x[0])
	}
	return rb.ToS(v)
}

// selectedList ports Array(resolve_preference(key, options)) defaulting to ["1"].
func (b *loc1991) selectedList(key string, options []string) []string {
	var out []string
	for _, x := range Arr(b.ResolveOptions(key, options)) {
		out = append(out, rb.ToS(x))
	}
	if len(out) == 0 {
		out = []string{"1"}
	}
	return out
}

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strconv.Itoa(i + 1)
	}
	return out
}

func (b *loc1991) versiclePair(v, r string) []*Line {
	return append(b.textLines(v, "leader"), b.textLines(r, "congregation")...)
}

func (b *loc1991) introduction() *Section {
	intro := b.T("office_introduction")
	if intro == nil {
		return nil
	}
	return b.Section("Introdução", "introduction", []*Line{
		b.FetchLineItem("rubric_minister", "rubric", nil, "content"),
		b.slugLine(intro, "text"),
	}, nil)
}

func (b *loc1991) penitence() *Section {
	var lines []*Line
	lines = append(lines, b.textLines("penitential_admonition", "text")...)
	lines = append(lines, b.textLines("confession_invitation", "leader")...)
	lines = append(lines, b.FetchLineItem("rubric_de_joelhos", "rubric", nil, "content"))
	lines = append(lines, b.textLines("confession", "text")...)
	pref := "evening_use_priestly_absolution"
	if b.OfficeType == "morning" {
		pref = "morning_use_priestly_absolution"
	}
	if b.PrefS(pref) == "yes" {
		lines = append(lines, b.FetchLineItem("rubric_priest", "rubric", nil, "content"))
		lines = append(lines, b.textLines("absolution", "text")...)
	}
	return b.Section("Confissão", "confession", lines, nil)
}

func (b *loc1991) openingVersicles() *Section {
	lines := b.versiclePair("opening_versicle_1_v", "opening_versicle_1_r")
	lines = append(lines, b.versiclePair("opening_versicle_2_v", "opening_versicle_2_r")...)
	lines = append(lines, b.gloriaLine())
	return b.Section("Preces", "opening_versicles", lines, nil)
}

func (b *loc1991) psalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{
		b.FetchLineItem("psalms_heading", "heading", nil, "content"),
		b.I(p.Reference, "subtitle"),
		b.Spacer(),
	}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	lines = append(lines, b.Spacer(), b.gloriaLine())
	return b.Section("Salmos", "psalms", lines, nil)
}

func (b *loc1991) creed() *Section {
	c := b.T("apostles_creed")
	if c == nil {
		return nil
	}
	return b.Section("Credo dos Apóstolos", "creed", []*Line{b.slugLine(c, "text")}, nil)
}

func (b *loc1991) prayers() *Section {
	lines := []*Line{b.FetchLineItem("rubric_de_joelhos", "rubric", nil, "content")}
	lines = append(lines, b.textLines("kyrie", "text")...)
	lines = append(lines, b.textLines("lords_prayer", "text")...)
	return b.Section("Orações", "prayers", lines, nil)
}

func (b *loc1991) suffrages() *Section {
	s := b.T("suffrages")
	if s == nil {
		return nil
	}
	return b.Section("Responsório", "suffrages", []*Line{
		b.FetchLineItem("rubric_say_responsory", "rubric", nil, "content"),
		b.slugLine(s, "text"),
	}, nil)
}

func (b *loc1991) conclusion(blessingSlug string) *Section {
	lines := b.versiclePair("conclusion_versicle_1_v", "conclusion_versicle_1_r")
	lines = append(lines, b.versiclePair("conclusion_versicle_2_v", "conclusion_versicle_2_r")...)
	lines = append(lines, b.Spacer())
	lines = append(lines, b.textLines(blessingSlug, "text")...)
	return b.Section("Conclusão", "conclusion", lines, nil)
}

// concludingCollects appends the numbered collects after the collect of the day.
func (b *loc1991) concludingCollects(prefix string, nums []string) *Section {
	lines := b.CollectLines(b.Collects)
	for _, n := range nums {
		t := b.T(prefix + n)
		if t == nil {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, b.Spacer())
		}
		lines = append(lines, b.slugLine(t, "text"))
	}
	return b.Section("Oração do Dia", "collects", lines, nil)
}

// --- Morning / Evening -------------------------------------------------------------------

func (b *loc1991) morning() []*Section {
	return Pipeline(
		One(b.introduction),
		One(b.penitence),
		One(b.openingVersicles),
		One(func() *Section {
			slug := "antifonas_pascais"
			if !b.easterDay() {
				slug = b.pickOption("morning_invitatory_canticle", []string{"venite", "jubilate"}, "venite")
			}
			return b.canticleModule(slug, "invitatory_canticle")
		}),
		One(b.psalms),
		One(func() *Section {
			return b.lessonModule("first_reading", "Primeira Leitura (Antigo Testamento)", "first_lesson")
		}),
		One(func() *Section {
			slug := b.pickOption("morning_canticle_ot", []string{"benedictus", "benedicite", "magna_et_mirabilia"}, "benedictus")
			return b.canticleModule(slug, "canticle_after_first")
		}),
		One(func() *Section {
			return b.lessonModule("second_reading", "Segunda Leitura (Novo Testamento)", "second_lesson")
		}),
		One(b.mCanticleAfterSecond),
		One(b.creed),
		One(b.prayers),
		One(b.suffrages),
		One(func() *Section {
			return b.concludingCollects("morning_collect_",
				[]string{b.pickOption("morning_concluding_collect", numbered(3), "1")})
		}),
		One(func() *Section { return b.conclusion("blessing") }),
	)
}

func (b *loc1991) mCanticleAfterSecond() *Section {
	slug := "salvador_do_mundo"
	if !LentSeason(b.Season()) {
		slug = b.pickOption("morning_canticle_nt", []string{"te_deum", "gloria_in_excelsis", "salvador_do_mundo"}, "te_deum")
	}
	s := b.canticleModule(slug, "canticle_after_second")
	if s == nil || slug != "te_deum" || b.PrefS("morning_extended_te_deum") != "true" {
		return s
	}
	if extra := b.T("te_deum_extra"); extra != nil {
		s.Lines = append(s.Lines, b.slugLine(extra, "text"))
	}
	return s
}

func (b *loc1991) evening() []*Section {
	return Pipeline(
		One(b.introduction),
		One(b.penitence),
		One(b.openingVersicles),
		One(func() *Section {
			slug := "antifonas_pascais"
			if !b.easterDay() {
				slug = b.pickOption("evening_invitatory_canticle", []string{"phos_hilaron", "salmo_134"}, "phos_hilaron")
			}
			return b.canticleModule(slug, "invitatory_canticle")
		}),
		One(b.psalms),
		One(func() *Section {
			return b.lessonModule("first_reading", "Primeira Leitura (Antigo Testamento)", "first_lesson")
		}),
		One(func() *Section {
			return b.canticleModule(b.pickOption("evening_canticle_ot", []string{"magnificat", "benedictus_es"}, "magnificat"), "canticle_after_first")
		}),
		One(func() *Section {
			return b.lessonModule("second_reading", "Segunda Leitura (Novo Testamento)", "second_lesson")
		}),
		One(func() *Section {
			return b.canticleModule(b.pickOption("evening_canticle_nt",
				[]string{"nunc_dimittis", "gloria_de_cristo", "gloria_e_honra"}, "nunc_dimittis"), "canticle_after_second")
		}),
		One(b.creed),
		One(b.prayers),
		One(b.suffrages),
		One(func() *Section {
			return b.concludingCollects("evening_collect_", b.selectedList("evening_concluding_collect", numbered(3)))
		}),
		One(func() *Section { return b.conclusion("blessing") }),
	)
}

// --- Compline ----------------------------------------------------------------------------

var (
	loc1991PsalmKeys = numbered(7)
	loc1991Psalms    = map[string]string{
		"1": "Salmo 4", "2": "Salmo 16:7-11", "3": "Salmo 17:1b-8", "4": "Salmo 31:2-6",
		"5": "Salmo 91", "6": "Salmo 134", "7": "Salmo 139:1b-11,17-18",
	}
	loc1991ReadingKeys = numbered(10)
	loc1991Readings    = map[string]string{
		"1": "Isaías 26:3-5,7-9", "2": "Isaías 35:8-10", "3": "Jeremias 31:33-34", "4": "Habacuc 3:17-19",
		"5": "Deuteronómio 6:4-7", "6": "João 3:19-21", "7": "1 Coríntios 1:26-31",
		"8": "1 Coríntios 2:10b-13", "9": "Efésios 4:26-27", "10": "Filipenses 4:6-9",
	}
)

func (b *loc1991) compline() []*Section {
	return Pipeline(
		One(b.cOpening),
		One(b.cPenitence),
		One(func() *Section {
			h := b.T("te_lucis")
			if h == nil {
				return nil
			}
			var name any = "Hino"
			if h.Title != nil {
				name = *h.Title
			}
			return b.Section(name, "hymn", []*Line{b.FetchLineItem("rubric_hymn", "rubric", nil, "content"), b.slugLine(h, "text")}, nil)
		}),
		One(b.cPsalms),
		One(b.cReading),
		One(b.cNuncDimittis),
		One(func() *Section {
			lp := b.T("lords_prayer")
			if lp == nil {
				return nil
			}
			return b.Section("Pai Nosso", "lords_prayer", []*Line{b.slugLine(lp, "text")}, nil)
		}),
		One(func() *Section {
			r := b.T("compline_responsory")
			if r == nil {
				return nil
			}
			var name any = "Responsório"
			if r.Title != nil {
				name = *r.Title
			}
			return b.Section(name, "responsory", []*Line{b.FetchLineItem("rubric_responsory", "rubric", nil, "content"), b.slugLine(r, "text")}, nil)
		}),
		One(b.cCollects),
		One(func() *Section { return b.conclusion("compline_blessing") }),
	)
}

func (b *loc1991) bibleText() *bible.TextService {
	tr := "bpt"
	if v := b.Pref("bible_version"); v != nil && v != false {
		tr = rb.ToS(v)
	}
	return bible.NewTextService(tr, bible.NewSegmentCache())
}

func (b *loc1991) passage(reference string) *rb.Map {
	m, err := b.bibleText().FetchPassageStructured(b.Ctx, reference)
	if err != nil {
		panic(err)
	}
	return m
}

func (b *loc1991) cOpening() *Section {
	lines := []*Line{b.FetchLineItem("rubric_minister_label", "rubric", nil, "content")}
	lines = append(lines, b.textLines("compline_opening", "text")...)
	for _, i := range b.selectedList("compline_opening_sentences", []string{"1", "2", "3"}) {
		s := b.T("compline_sentence_" + i)
		if s == nil {
			continue
		}
		lines = append(lines, b.slugLine(s, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return b.Section("Abertura", "opening", lines, nil)
}

func (b *loc1991) cPenitence() *Section {
	var lines []*Line
	lines = append(lines, b.textLines("compline_meditation", "rubric")...)
	lines = append(lines, b.textLines("compline_confession_invitation", "leader")...)
	lines = append(lines, b.textLines("compline_confession", "text")...)
	lines = append(lines, b.FetchLineItem("rubric_minister", "rubric", nil, "content"))
	lines = append(lines, b.textLines("compline_absolution", "text")...)
	return b.Section("Confissão", "confession", lines, nil)
}

func (b *loc1991) cPsalms() *Section {
	var lines []*Line
	for _, key := range b.selectedList("compline_psalms", loc1991PsalmKeys) {
		ref := loc1991Psalms[key]
		content := b.passage(ref)
		lines = append(lines, b.I(ref, "subtitle"))
		if content != nil {
			lines = append(lines, b.BibleContent(content)...)
		}
	}
	lines = append(lines, b.gloriaLine())
	return b.Section("Salmos", "psalms", lines, nil)
}

func (b *loc1991) cReading() *Section {
	selected := "1"
	if s, ok := b.ResolveOptions("compline_reading", loc1991ReadingKeys).(string); ok {
		if _, known := loc1991Readings[s]; known {
			selected = s
		}
	}
	ref := loc1991Readings[selected]
	content := b.passage(ref)
	lines := []*Line{
		b.FetchLineItem("reading_announcement", "leader", [][2]string{{"reference", ref}}, "content"),
		b.Spacer(),
	}
	lines = append(lines, b.BibleContent(content)...)
	lines = append(lines, b.Spacer())
	lines = append(lines, b.wordOfTheLordLines()...)
	return b.Section("Leitura", "reading", lines, nil)
}

func (b *loc1991) cNuncDimittis() *Section {
	antiphon, canticle := b.T("nunc_dimittis_antiphon"), b.T("nunc_dimittis")
	if canticle == nil {
		return nil
	}
	var lines []*Line
	if antiphon != nil {
		lines = append(lines, b.slugLine(antiphon, "antiphon"))
	}
	lines = append(lines, b.slugLine(canticle, "text"), b.gloriaLine())
	if antiphon != nil {
		lines = append(lines, b.slugLine(antiphon, "antiphon"))
	}
	lines = append(lines, b.textLines("kyrie", "text")...)
	var name any = "Nunc Dimittis"
	if canticle.Title != nil {
		name = *canticle.Title
	}
	return b.Section(name, "nunc_dimittis", lines, nil)
}

func (b *loc1991) cCollects() *Section {
	var lines []*Line
	for _, n := range b.selectedList("compline_collect", numbered(6)) {
		lines = append(lines, b.textLines("compline_collect_"+n, "text")...)
	}
	easterSeason := lowerRuby(b.Season()) == "páscoa"
	if b.Date.IsSunday() || easterSeason {
		if e := b.T("compline_collect_easter"); e != nil {
			lines = append(lines, b.FetchLineItem("rubric_easter_compline", "rubric", nil, "content"), b.slugLine(e, "text"))
		}
	}
	return b.Section("Orações", "collects", lines, nil)
}
