package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// loc1662Profile ports Loc1662Localization::PROFILES: the two 1662 books
// share one implementation and differ only in these slugs and in how
// canticle preference values are spelled.
type loc1662Profile struct {
	slugs map[string]string
	// bareCanticleValues: preference values are stored without the office
	// prefix the slug carries ("magnificat", not "evening_magnificat").
	bareCanticleValues bool
}

var loc1662Profiles = map[string]loc1662Profile{
	"loc_1662": {
		slugs: map[string]string{
			"morning_lords_prayer_repeated":  "morning_lords_prayer",
			"evening_lords_prayer_repeated":  "evening_lords_prayer",
			"evening_first_reading_rubric":   "morning_first_reading_rubric",
			"evening_second_reading_rubric":  "morning_second_reading_rubric",
			"evening_prayer_civil_authority": "morning_prayer_civil_authority_1",
			"evening_prayer_clergy_people":   "morning_prayer_clergy_people",
		},
		bareCanticleValues: true,
	},
	"loc_1662_en": {
		slugs: map[string]string{
			"morning_lords_prayer_repeated":  "morning_lords_prayer_repeated",
			"evening_lords_prayer_repeated":  "evening_lords_prayer_repeated",
			"evening_first_reading_rubric":   "evening_first_reading_rubric",
			"evening_second_reading_rubric":  "evening_second_reading_rubric",
			"evening_prayer_civil_authority": "evening_prayer_civil_authority_1",
			"evening_prayer_clergy_people":   "evening_prayer_clergy_people",
		},
	},
}

func init() {
	for code, profile := range loc1662Profiles {
		profile := profile
		Register(code, func(ctx context.Context, c *Context) Builder {
			if c.OfficeType != "morning" && c.OfficeType != "evening" {
				UnknownOfficeType(c.OfficeType)
			}
			b := &loc1662{profile: profile}
			b.Init(ctx, c)
			return b
		})
	}
}

// loc1662 ports DailyOffice::Builders::Loc1662::{Morning,Evening} and the
// Loc1662En subclasses.
type loc1662 struct {
	Base
	profile loc1662Profile
}

func (b *loc1662) Call() *rb.Map {
	if b.OfficeType == "morning" {
		return b.Render(b.morning())
	}
	return b.Render(b.evening())
}

// --- Loc1662Localization ---------------------------------------------------------------

func (b *loc1662) label(key string) string { return b.catalogueText("label", key) }

func (b *loc1662) catalogueText(kind, key string) string {
	slug := "loc1662_" + kind + "_" + key
	t := b.T(slug)
	if t == nil {
		code := ""
		if pb := b.PrayerBook(); pb != nil {
			code = pb.Code
		}
		panic(&rb.RubyError{Class: "KeyError", Message: fmt.Sprintf("Missing LiturgicalText %q for %s", slug, code)})
	}
	return t.Content
}

func (b *loc1662) slug(key string) string { return b.profile.slugs[key] }

func (b *loc1662) preferenceValue(slug string) string {
	if !b.profile.bareCanticleValues {
		return slug
	}
	return strings.TrimPrefix(slug, "evening_")
}

func (b *loc1662) canticleSection(preference, slug, def, alternative string) *Section {
	chosen := def
	v := b.ResolveOptions(preference, []string{b.preferenceValue(def), b.preferenceValue(alternative)})
	if s, ok := v.(string); ok && s == b.preferenceValue(alternative) {
		chosen = alternative
	}
	c := b.T(chosen)
	if c == nil {
		return nil
	}
	return b.Section(Title(c), slug, []*Line{b.I(c.Content, "text")}, nil)
}

// --- shared shapes ---------------------------------------------------------------------

// tAny ports fetch_liturgical_text with a resolve_preference result: only a
// String key can match the catalogue.
func (b *Base) tAny(v any) *store.LiturgicalText {
	if s, ok := v.(string); ok {
		return b.T(s)
	}
	return nil
}

// contentOrNil ports fetch_liturgical_text(slug)&.content.
func (b *Base) contentOrNil(slug string) any {
	if t := b.T(slug); t != nil {
		return t.Content
	}
	return nil
}

// rubric appends line_item(text.content, type:) when the text exists.
func (b *Base) plain(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.I(t.Content, typ))
	}
	return lines
}

// titled appends a heading with the title and the content when the text exists.
func (b *Base) titled(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.I(Title(t), "heading"), b.I(t.Content, typ))
	}
	return lines
}

func compactLines(lines []*Line) []*Line {
	out := lines[:0:0]
	for _, l := range lines {
		if l != nil {
			out = append(out, l)
		}
	}
	return out
}

// celebrationField ports day_info[:celebration]&.dig(key).
func (b *Base) celebrationField(key string) any {
	c, _ := b.DayInfo.Get("celebration").(*rb.Map)
	if c == nil {
		return nil
	}
	return c.Get(key)
}

func (b *loc1662) openingSentence(prefix string) *Section {
	lines := b.plain(nil, prefix+"_opening_sentence_rubric", "rubric")
	key := Or(b.ResolveRange(prefix+"_opening_sentence", 1, 11), 1)
	if s := b.T(prefix + "_opening_sentence_" + rubyInterp(key)); s != nil {
		lines = append(lines, b.I(s.Content, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return b.Section("", "opening_sentence", lines, nil)
}

func (b *loc1662) exhortation(prefix string) *Section {
	t := b.T(prefix + "_exhortation")
	if t == nil {
		return nil
	}
	return b.Section("", "exhortation", []*Line{b.I(t.Content, "text")}, nil)
}

func (b *loc1662) confession(prefix string) *Section {
	lines := b.plain(nil, prefix+"_confession_rubric", "rubric")
	lines = b.plain(lines, prefix+"_confession", "text")
	return b.Section("", "confession", lines, nil)
}

func (b *loc1662) absolution(prefix string) *Section {
	lines := b.plain(nil, prefix+"_absolution_rubric", "rubric")
	lines = b.plain(lines, prefix+"_absolution", "text")
	lines = b.plain(lines, prefix+"_absolution_response_rubric", "rubric")
	return b.Section("", "absolution", lines, nil)
}

func (b *loc1662) lordsPrayer(prefix string) *Section {
	return b.TextSection("", "lords_prayer", []Entry{
		{Slug: prefix + "_lords_prayer_rubric", Type: "rubric"},
		{Slug: prefix + "_lords_prayer"},
	}, false)
}

func (b *loc1662) versicles(prefix string) *Section {
	lines := b.plain(nil, prefix+"_versicles_rubric", "rubric")
	for _, p := range [][2]string{
		{"_inv_v1", "leader"}, {"_inv_r1", "congregation"}, {"_inv_v2", "leader"}, {"_inv_r2", "congregation"},
		{"_gloria_patri_rubric", "rubric"}, {"_gloria_patri_v", "leader"}, {"_gloria_patri_r", "congregation"},
		{"_inv_v3", "leader"}, {"_inv_r3", "congregation"},
	} {
		lines = append(lines, b.I(b.contentOrNil(prefix+p[0]), p[1]))
	}
	return b.Section("", "versicles", lines, nil)
}

func (b *loc1662) psalms(gloriaPrefix string) *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.I(p.Reference, "heading")}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	lines = b.plain(lines, gloriaPrefix+"_gloria_patri_v", "leader")
	lines = b.plain(lines, gloriaPrefix+"_gloria_patri_r", "congregation")
	return b.Section("", "psalms", lines, nil)
}

func (b *loc1662) reading(typ, rubric string) *Section {
	return b.ReadingModule(typ, "morning_reading_announcement_rubric", Rubrics{Pre: rubric}, typ+"_reading", "", false)
}

func (b *loc1662) canticle(v any, slug string) *Section {
	c := b.tAny(v)
	if c == nil {
		return nil
	}
	return b.Section(Title(c), slug, []*Line{b.I(c.Content, "text")}, nil)
}

func (b *loc1662) prayers(prefix string) *Section {
	lines := b.plain(nil, prefix+"_prayers_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil(prefix+"_salutation_v"), "leader"),
		b.I(b.contentOrNil(prefix+"_salutation_r"), "congregation"),
		b.I(b.contentOrNil(prefix+"_lesser_litany_rubric"), "rubric"),
		b.I(b.contentOrNil("kyrie"), "responsive"))
	return b.Section("", "prayers", lines, nil)
}

func (b *loc1662) lordsPrayerRepeated(prefix string) *Section {
	return b.TextSection("", "lords_prayer_repeated", []Entry{
		{Slug: prefix + "_lords_prayer_repeated_rubric", Type: "rubric"},
		{Slug: b.slug(prefix + "_lords_prayer_repeated")},
	}, false)
}

func (b *loc1662) suffrages(prefix string) *Section {
	lines := b.plain(nil, prefix+"_suffrages_rubric", "rubric")
	for i := 1; i <= 6; i++ {
		lines = append(lines,
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_v%d", prefix, i)), "leader"),
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_r%d", prefix, i)), "congregation"))
	}
	return b.Section("", "suffrages", lines, nil)
}

func (b *loc1662) theGrace() *Section {
	t := b.T("the_grace")
	if t == nil {
		return nil
	}
	return b.Section(Title(t), "the_grace", []*Line{b.I(t.Content, "text")}, nil)
}

func (b *loc1662) creed(prefix string) *Section {
	lines := b.plain(nil, prefix+"_creed_rubric", "rubric")
	lines = b.plain(lines, "apostles_creed", "text")
	return b.Section("", "creed", lines, nil)
}

// --- Morning -----------------------------------------------------------------------------

func (b *loc1662) morning() []*Section {
	const p = "morning"
	return Pipeline(
		One(func() *Section { return b.openingSentence(p) }),
		One(func() *Section { return b.exhortation(p) }),
		One(func() *Section { return b.confession(p) }),
		One(func() *Section { return b.absolution(p) }),
		One(func() *Section { return b.lordsPrayer(p) }),
		One(func() *Section { return b.versicles(p) }),
		One(b.mInvitatoryCanticle),
		One(func() *Section { return b.psalms("morning_psalms") }),
		One(func() *Section { return b.reading("first", "morning_first_reading_rubric") }),
		One(func() *Section {
			return b.canticle(Or(b.ResolveOptions("morning_first_canticle", []string{"morning_te_deum", "morning_benedicite"}), "morning_te_deum"), "first_canticle")
		}),
		One(func() *Section { return b.reading("second", "morning_second_reading_rubric") }),
		One(func() *Section {
			return b.canticle(Or(b.ResolveOptions("morning_second_canticle", []string{"morning_benedictus", "morning_jubilate"}), "morning_benedictus"), "second_canticle")
		}),
		One(b.mCreed),
		One(func() *Section { return b.prayers(p) }),
		One(func() *Section { return b.lordsPrayerRepeated(p) }),
		One(func() *Section { return b.suffrages(p) }),
		One(func() *Section { return b.CollectOfDaySection(b.label("collect_of_the_day"), "morning_collects_rubric") }),
		One(func() *Section {
			return b.TextSection(b.label("fixed_collects"), "fixed_collects", []Entry{
				{Slug: "morning_collect_peace", Heading: true},
				{Slug: "morning_collect_grace", Heading: true},
			}, false)
		}),
		One(b.mLitany),
		One(b.mAdditionalPrayers),
		One(b.theGrace),
	)
}

func (b *loc1662) mInvitatoryCanticle() *Section {
	lines := b.plain(nil, "morning_venite_rubric", "rubric")
	slug := "morning_venite"
	if v, ok := b.ResolveOptions("morning_invitatory_canticle", []string{"venite", "jubilate"}).(string); ok && v == "jubilate" {
		slug = "morning_jubilate"
	}
	c := b.T(slug)
	if c == nil {
		return nil
	}
	_ = lines // the rubric is fetched but, as in Rails, not rendered
	return b.Section(Title(c), "invitatory_canticle", []*Line{b.I(c.Content, "text")}, nil)
}

var athanasianFeasts = map[string]bool{
	"christmas_day": true, "epiphany": true, "saint_matthias": true, "easter_day": true,
	"ascension_day": true, "pentecost_sunday": true, "nativity_of_john_baptist": true,
	"saint_james": true, "saint_bartholomew": true, "saint_matthew": true,
	"saints_simon_and_jude": true, "saint_andrew": true, "trinity_sunday": true,
}

func (b *loc1662) mCreed() *Section {
	if name, ok := b.celebrationField("name").(string); ok && athanasianFeasts[name] {
		lines := b.plain(nil, "athanasian_creed_rubric", "rubric")
		lines = b.plain(lines, "athanasian_creed", "text")
		return b.Section(b.label("athanasian_creed"), "athanasian_creed", lines, nil)
	}
	return b.creed("morning")
}

func (b *loc1662) mAdditionalPrayers() *Section {
	lines := b.plain(nil, "morning_additional_prayers_rubric", "rubric")
	lines = b.titled(lines, "morning_prayer_civil_authority_1", "text")
	lines = b.titled(lines, "morning_prayer_clergy_people", "text")
	lines = b.titled(lines, "prayer_st_chrysostom", "text")
	return b.Section(b.label("additional_prayers"), "additional_prayers", lines, nil)
}

func (b *loc1662) mLitany() *Section {
	wd := b.Date.Weekday()
	if wd != 0 && wd != 3 && wd != 5 {
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
		b.FetchLineItem("loc1662_phrase_let_us_pray", "rubric", nil, "content"),
		b.I(b.contentOrNil("litany_prayer_1"), "leader"),
		b.I(b.contentOrNil("litany_prayer_2"), "leader"),
		b.I(b.contentOrNil("litany_gloria_patri"), "responsive"),
		b.I(b.contentOrNil("litany_final_prayer"), "leader"))
	return b.Section(b.label("litany"), "litany", compactLines(lines), nil)
}

// --- Evening -----------------------------------------------------------------------------

func (b *loc1662) evening() []*Section {
	const p = "evening"
	return Pipeline(
		One(func() *Section { return b.openingSentence(p) }),
		One(func() *Section { return b.exhortation(p) }),
		One(func() *Section { return b.confession(p) }),
		One(func() *Section { return b.absolution(p) }),
		One(func() *Section { return b.lordsPrayer(p) }),
		One(func() *Section { return b.versicles(p) }),
		One(func() *Section { return b.psalms(p) }),
		One(func() *Section { return b.reading("first", b.slug("evening_first_reading_rubric")) }),
		One(func() *Section {
			return b.canticleSection("evening_first_canticle", "first_canticle", "evening_magnificat", "evening_cantate_domino")
		}),
		One(func() *Section { return b.reading("second", b.slug("evening_second_reading_rubric")) }),
		One(func() *Section {
			return b.canticleSection("evening_second_canticle", "second_canticle", "evening_nunc_dimittis", "evening_deus_misereatur")
		}),
		One(func() *Section { return b.creed(p) }),
		One(func() *Section { return b.prayers(p) }),
		One(func() *Section { return b.lordsPrayerRepeated(p) }),
		One(func() *Section { return b.suffrages(p) }),
		One(func() *Section { return b.CollectOfDaySection(b.label("collect_of_the_day"), "evening_collects_rubric") }),
		One(func() *Section {
			lines := b.titled(nil, "evening_collect_peace", "leader")
			lines = b.titled(lines, "evening_collect_perils", "leader")
			return b.Section("", "fixed_collects", lines, nil)
		}),
		One(func() *Section {
			lines := b.titled(nil, b.slug("evening_prayer_civil_authority"), "text")
			lines = b.titled(lines, b.slug("evening_prayer_clergy_people"), "text")
			lines = b.titled(lines, "prayer_st_chrysostom", "text")
			return b.Section("", "additional_prayers", lines, nil)
		}),
		One(b.theGrace),
	)
}
