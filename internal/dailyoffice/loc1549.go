package dailyoffice

import (
	"context"
	"fmt"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

func init() {
	Register("loc_1549", func(ctx context.Context, c *Context) Builder {
		if c.OfficeType != "morning" && c.OfficeType != "evening" {
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc1549{}
		b.Init(ctx, c)
		return b
	})
}

// loc1549 ports DailyOffice::Builders::Loc1549::{Morning,Evening}.
type loc1549 struct{ Base }

func (b *loc1549) Call() *rb.Map {
	if b.OfficeType == "morning" {
		return b.Render(b.morning())
	}
	return b.Render(b.evening())
}

// easter ports Liturgical::EasterCalculator.new(date.year).easter_date.
func (b *loc1549) easter() civil.Date {
	return liturgical.EasterFor(b.Date.Year(), liturgical.Gregorian).EasterDate
}

// eastertide: the Alleluia is said from Easter to Trinity Sunday.
func (b *loc1549) eastertide() bool {
	e := b.easter()
	return b.Date >= e && b.Date <= e.Add(56)
}

func (b *loc1549) lordsPrayer(p string) *Section {
	return b.TextSection("", "lords_prayer", []Entry{
		{Slug: p + "_lords_prayer_rubric", Type: "rubric"},
		{Slug: p + "_lords_prayer"},
	}, false)
}

func (b *loc1549) versicles(p string) *Section {
	pairs := [][2]string{{"_versicles_rubric", "rubric"}, {"_inv_v1", "leader"}, {"_inv_r1", "congregation"}}
	if p == "morning" {
		pairs = append(pairs, [2]string{"_inv_v2", "leader"}, [2]string{"_inv_r2", "congregation"},
			[2]string{"_gloria_patri_v", "leader"}, [2]string{"_gloria_patri_r", "congregation"}, [2]string{"_inv_v3", "leader"})
	} else {
		pairs = append(pairs, [2]string{"_gloria_patri_v", "leader"}, [2]string{"_gloria_patri_r", "congregation"},
			[2]string{"_inv_v2", "leader"})
	}
	var lines []*Line
	for _, x := range pairs {
		lines = append(lines, b.I(b.contentOrNil(p+x[0]), x[1]))
	}
	if b.eastertide() {
		lines = append(lines, b.FetchLineItem("easter_alleluia", "congregation", nil, "content"))
	}
	return b.Section("", "versicles", compactLines(lines), nil)
}

func (b *loc1549) psalms(p string) *Section {
	ps := b.Readings.Psalm
	if ps == nil {
		return nil
	}
	lines := b.plain(nil, p+"_psalms_rubric", "rubric")
	lines = append(lines, b.I(ps.Reference, "heading"))
	if ps.Content != nil {
		lines = append(lines, b.BibleContent(ps.Content)...)
	}
	lines = append(lines,
		b.I(b.contentOrNil(p+"_gloria_patri_v"), "leader"),
		b.I(b.contentOrNil(p+"_gloria_patri_r"), "congregation"))
	return b.Section("", "psalms", lines, nil)
}

func (b *loc1549) reading(typ, pre string) *Section {
	return b.ReadingModule(typ, "", Rubrics{Pre: pre}, typ+"_reading", "", true)
}

// canticle ports the canticle sections: optional rubric, then the text.
func (b *loc1549) canticle(slug, rubric, sectionSlug string) *Section {
	c := b.T(slug)
	if c == nil {
		return nil
	}
	var lines []*Line
	if rubric != "" {
		lines = b.plain(lines, rubric, "rubric")
	}
	lines = append(lines, b.I(c.Content, "canticle"))
	return b.Section(Title(c), sectionSlug, lines, nil)
}

func (b *loc1549) kyrie() *Section {
	lines := b.plain(nil, "prayers_rubric", "rubric")
	lines = b.plain(lines, "kyrie", "responsive")
	return b.Section("", "prayers", lines, nil)
}

func (b *loc1549) creed(p string) *Section {
	lines := b.plain(nil, "creed_rubric", "rubric")
	lines = b.plain(lines, "apostles_creed", "creed")
	lines = b.plain(lines, p+"_lords_prayer", "congregation")
	return b.Section("", "creed", lines, nil)
}

func (b *loc1549) suffrages() *Section {
	lines := b.plain(nil, "suffrages_rubric", "rubric")
	for i := 1; i <= 8; i++ {
		lines = append(lines,
			b.I(b.contentOrNil(fmt.Sprintf("suff_v%d", i)), "leader"),
			b.I(b.contentOrNil(fmt.Sprintf("suff_r%d", i)), "congregation"))
	}
	return b.Section("", "suffrages", lines, nil)
}

func (b *loc1549) fixedCollects(slugs ...string) *Section {
	var lines []*Line
	for _, s := range slugs {
		if t := b.T(s); t != nil {
			lines = append(lines, b.I(Title(t), "heading"), b.I(t.Content, "prayer"))
		}
	}
	return b.Section("Coletas Fixas", "collects", lines, nil)
}

func (b *loc1549) preferenceDisabled(key string) bool { return b.PrefS(key) == "false" }

// --- Morning -----------------------------------------------------------------------------

func (b *loc1549) morning() []*Section {
	const p = "morning"
	return Pipeline(
		One(b.easterAnthems),
		One(func() *Section { return b.lordsPrayer(p) }),
		One(func() *Section { return b.versicles(p) }),
		One(func() *Section { return b.canticle("morning_venite", "morning_venite_rubric", "invitatory_canticle") }),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section { return b.reading("first", "morning_readings_rubric") }),
		One(b.firstCanticle),
		One(func() *Section { return b.reading("second", "") }),
		One(func() *Section {
			return b.canticle("morning_benedictus", "morning_benedictus_rubric", "second_canticle")
		}),
		One(b.athanasianCreed),
		One(b.kyrie),
		One(func() *Section { return b.creed(p) }),
		One(b.suffrages),
		One(func() *Section { return b.CollectOfDaySection("Coleta do Dia", "morning_collects_rubric") }),
		One(func() *Section { return b.fixedCollects("morning_collect_peace", "morning_collect_grace") }),
		One(b.litany),
	)
}

// easterAnthems: on Easter Day the Anthems are sung before Matins.
func (b *loc1549) easterAnthems() *Section {
	if b.Date != b.easter() {
		return nil
	}
	anthems := b.T("easter_anthems")
	if anthems == nil {
		return nil
	}
	lines := b.plain(nil, "easter_anthems_rubric", "rubric")
	lines = append(lines,
		b.I(anthems.Content, "canticle"),
		b.I(b.contentOrNil("easter_anthems_v"), "leader"),
		b.I(b.contentOrNil("easter_anthems_r"), "congregation"))
	if collect := b.T("easter_anthems_collect"); collect != nil {
		lines = append(lines,
			b.FetchLineItem("prayers_let_us_pray_period", "rubric", nil, "content"),
			b.I(collect.Content, "prayer"))
	}
	return b.Section("Antífonas da Páscoa", "easter_anthems", compactLines(lines), nil)
}

func (b *loc1549) firstCanticle() *Section {
	pref := Or(b.ResolveOptions("morning_first_canticle", []string{"seasonal", "morning_te_deum", "morning_benedicite"}), "seasonal")
	var c = b.tAny(pref)
	if pref == "seasonal" {
		if b.IsLent() {
			c = b.T("morning_benedicite")
		} else {
			c = b.T("morning_te_deum")
		}
	}
	if c == nil {
		return nil
	}
	lines := b.plain(nil, "morning_te_deum_rubric", "rubric")
	lines = append(lines, b.I(c.Content, "canticle"))
	return b.Section(Title(c), "first_canticle", lines, nil)
}

// athanasianCreed: Christmas, Epiphany, Easter, Ascension, Pentecost and Trinity.
func (b *loc1549) athanasianCreed() *Section {
	d, e := b.Date, b.easter()
	day := (d.Month() == 12 && d.Day() == 25) || (d.Month() == 1 && d.Day() == 6) ||
		d == e || d == e.Add(39) || d == e.Add(49) || d == e.Add(56)
	if !day || b.preferenceDisabled("include_athanasian_creed") {
		return nil
	}
	text := b.T("athanasian_creed")
	if text == nil {
		return nil
	}
	lines := b.plain(nil, "athanasian_creed_rubric", "rubric")
	lines = append(lines, b.I(text.Content, "creed"))
	return b.Section("Credo Atanasiano", "athanasian_creed", lines, nil)
}

func (b *loc1549) litany() *Section {
	wd := b.Date.Weekday()
	if (wd != 0 && wd != 3 && wd != 5) || b.preferenceDisabled("include_litany") {
		return nil
	}
	lines := b.plain(nil, "litany_rubric", "rubric")
	for i := 1; i <= 5; i++ {
		lines = b.plain(lines, fmt.Sprintf("litany_part_%d", i), "responsive")
	}
	lines = b.plain(lines, "litany_lords_prayer_rubric", "rubric")
	lines = b.plain(lines, "morning_lords_prayer", "congregation")
	lines = b.plain(lines, "litany_after_lords_prayer", "responsive")
	lines = append(lines,
		b.FetchLineItem("prayers_let_us_pray_period", "rubric", nil, "content"),
		b.I(b.contentOrNil("litany_prayer_1"), "leader"),
		b.I(b.contentOrNil("litany_prayer_2"), "leader"),
		b.I(b.contentOrNil("litany_gloria_patri"), "responsive"),
		b.I(b.contentOrNil("litany_final_prayer"), "leader"))
	if t := b.T("prayer_st_chrysostom"); t != nil {
		lines = append(lines, b.I(Title(t), "heading"), b.I(t.Content, "prayer"))
	}
	return b.Section("A Ladainha", "litany", compactLines(lines), nil)
}

// --- Evening -----------------------------------------------------------------------------

func (b *loc1549) evening() []*Section {
	const p = "evening"
	return Pipeline(
		One(func() *Section { return b.lordsPrayer(p) }),
		One(func() *Section { return b.versicles(p) }),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section { return b.reading("first", "") }),
		One(func() *Section { return b.canticle("evening_magnificat", "", "first_canticle") }),
		One(func() *Section { return b.reading("second", "evening_nunc_dimittis_rubric") }),
		One(func() *Section { return b.canticle("evening_nunc_dimittis", "", "second_canticle") }),
		One(b.kyrie),
		One(func() *Section { return b.creed(p) }),
		One(b.suffrages),
		One(func() *Section { return b.CollectOfDaySection("Coleta do Dia", "evening_collects_rubric") }),
		One(func() *Section { return b.fixedCollects("evening_collect_peace", "evening_collect_aid") }),
	)
}
