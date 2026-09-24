package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
)

func init() {
	Register("cw_2005_en", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "evening", "midday", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &cw2005{}
		b.H.ReadingServiceType = func() (string, bool) {
			switch b.OfficeType {
			case "morning", "midday":
				return "morning_prayer", true
			}
			return "evening_prayer", true
		}
		b.H.ReadingService = func() *reading.Resolver {
			if b.rs == nil {
				variant := ""
				if b.PrefS("ascension_readings") == "alternative" {
					variant = "ascension_alternative"
				}
				translation := "nvi"
				if v := b.Pref("bible_version"); v != nil && v != false {
					translation = rb.ToS(v)
				}
				b.rs = reading.For(b.Ctx, b.Date, reading.Options{
					PrayerBookCode: b.C.Code, Calendar: b.Calendar(), DayContext: b.DayContext, Translation: translation,
					PsalmTranslation: b.SelectedPsalmTranslation(), ReadingType: b.PrefString("reading_type"),
					ServiceType: b.ReadingServiceType(), ServiceVariant: variant, PsalmTable: b.PrefString("weekday_psalm_table"),
					LoadContent: true,
				})
			}
			return b.rs
		}
		b.Init(ctx, c)
		return b
	})
}

// cw2005 ports DailyOffice::Builders::Cw2005En::* (Common Worship: Daily Prayer).
type cw2005 struct {
	Base
	rs *reading.Resolver
}

func (b *cw2005) Call() *rb.Map {
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

// part is one {slug:, type:} entry of build_cw_section.
type part struct{ slug, typ string }

func (b *cw2005) section(name, slug string, parts []part) *Section {
	var lines []*Line
	for _, p := range parts {
		if l := b.FetchLineItem(p.slug, p.typ, nil, "content"); l != nil {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section(name, slug, lines, nil)
}

var cwPenitenceParts = map[string][]string{
	"1": {"cw_penitence_form_1_leader_1", "cw_penitence_form_1_people_1", "cw_penitence_form_1_leader_2",
		"cw_penitence_form_1_people_2", "cw_penitence_form_1_leader_3", "cw_penitence_form_1_people_3",
		"cw_penitence_form_1_citation_1", "cw_penitence_form_1_rubric_1", "cw_penitence_form_1_people_4",
		"cw_penitence_form_1_citation_2", "cw_penitence_form_1_leader_4", "cw_penitence_form_1_leader_5",
		"cw_penitence_form_1_people_5"},
	"2": {"cw_penitence_form_2_rubric_1", "cw_penitence_form_2_leader_1", "cw_penitence_form_2_people_1",
		"cw_penitence_form_2_leader_2", "cw_penitence_form_2_people_2", "cw_penitence_form_2_rubric_2",
		"cw_penitence_form_2_leader_3", "cw_penitence_form_2_people_3", "cw_penitence_form_2_citation_1",
		"cw_penitence_form_2_leader_4", "cw_penitence_form_2_leader_5", "cw_penitence_form_2_people_4"},
	"3": {"cw_penitence_form_3_leader_1", "cw_penitence_form_3_people_1", "cw_penitence_form_3_leader_2",
		"cw_penitence_form_3_people_2", "cw_penitence_form_3_leader_3", "cw_penitence_form_3_people_3",
		"cw_penitence_form_3_rubric_1", "cw_penitence_form_3_people_4", "cw_penitence_form_3_leader_4",
		"cw_penitence_form_3_people_5", "cw_penitence_form_3_leader_5", "cw_penitence_form_3_people_6",
		"cw_penitence_form_3_leader_6", "cw_penitence_form_3_people_7", "cw_penitence_form_3_people_8",
		"cw_penitence_form_3_citation_1"},
	"4": {"cw_penitence_form_4_leader_1", "cw_penitence_form_4_people_1", "cw_penitence_form_4_leader_2",
		"cw_penitence_form_4_people_2", "cw_penitence_form_4_rubric_1", "cw_penitence_form_4_leader_3",
		"cw_penitence_form_4_people_3", "cw_penitence_form_4_leader_4", "cw_penitence_form_4_people_4",
		"cw_penitence_form_4_leader_5", "cw_penitence_form_4_people_5", "cw_penitence_form_4_leader_6",
		"cw_penitence_form_4_people_6", "cw_penitence_form_4_leader_7", "cw_penitence_form_4_people_7",
		"cw_penitence_form_4_leader_8", "cw_penitence_form_4_people_8", "cw_penitence_form_4_citation_1"},
}

func (b *cw2005) penitenceParts() []part {
	form := b.PrefS("confession_form")
	if rb.BlankString(form) || form == "none" {
		return nil
	}
	var out []part
	for _, slug := range cwPenitenceParts[form] {
		typ := "leader"
		switch {
		case strings.Contains(slug, "_people_"):
			typ = "congregation"
		case strings.Contains(slug, "_rubric_"):
			typ = "rubric"
		case strings.Contains(slug, "_citation_"):
			typ = "citation"
		}
		out = append(out, part{slug, typ})
	}
	return out
}

func refrainCanticle(prefix string, verses int) []part {
	ps := []part{{prefix + "_refrain_rubric", "rubric"}, {prefix + "_refrain", "congregation"}}
	for i := 1; i <= verses; i++ {
		ps = append(ps, part{fmt.Sprintf("%s_verse_%d", prefix, i), "leader"})
	}
	return append(ps, part{prefix + "_citation", "citation"}, part{prefix + "_gloria", "congregation"},
		part{prefix + "_refrain_end", "congregation"})
}

var cwCanticleParts = map[string][]part{
	"cw_song_of_david":    refrainCanticle("cw_song_of_david", 6),
	"cw_song_of_the_lamb": refrainCanticle("cw_song_of_the_lamb", 5),
	"cw_benedictus":       refrainCanticle("cw_benedictus", 10),
	"cw_magnificat":       refrainCanticle("cw_magnificat", 8),
	"cw_nunc_dimittis": {
		{"cw_nunc_dimittis_opening", "congregation"}, {"cw_nunc_dimittis_verse_1", "leader"},
		{"cw_nunc_dimittis_verse_2", "leader"}, {"cw_nunc_dimittis_verse_3", "leader"},
		{"cw_nunc_dimittis_citation", "citation"}, {"cw_nunc_dimittis_gloria", "congregation"},
		{"cw_nunc_dimittis_closing", "congregation"},
	},
	"cw_easter_anthems": {
		{"cw_easter_anthems_stanza_1", "leader"}, {"cw_easter_anthems_stanza_2", "leader"},
		{"cw_easter_anthems_stanza_3", "leader"},
	},
}

func (b *cw2005) canticle(slug, name, sectionSlug string) *Section {
	return b.section(name, sectionSlug, cwCanticleParts[slug])
}

func (b *cw2005) psalmody(rubric string) *Section {
	lines := []*Line{b.FetchLineItem(rubric, "rubric", nil, "content")}
	if p := b.Readings.Psalm; p != nil {
		if !rb.BlankString(p.Reference) {
			lines = append(lines, b.I(p.Reference, "heading"))
		}
		if p.Content != nil {
			lines = append(lines, b.BibleContent(p.Content)...)
		}
	}
	lines = append(lines,
		b.FetchLineItem("cw_each_psalm_rubric", "rubric", nil, "content"),
		b.FetchLineItem("cw_gloria_patri", "congregation", nil, "content"))
	return b.Section("Psalmody", "psalmody", lines, nil)
}

func (b *cw2005) readings() *Section {
	lines := []*Line{b.FetchLineItem("cw_reading_intro", "rubric", nil, "content")}
	for _, r := range []*reading.Passage{b.Readings.FirstReading, b.Readings.SecondReading} {
		if r == nil {
			continue
		}
		lines = append(lines, b.I(r.Reference, "heading"))
		if r.Content != nil {
			lines = append(lines, b.BibleContent(r.Content)...)
		}
	}
	lines = append(lines, b.FetchLineItem("cw_reading_silence_rubric", "rubric", nil, "content"))
	return b.Section("The Word of God", "readings", lines, nil)
}

func (b *cw2005) lordsPrayerSlug() string {
	if b.PrefS("lords_prayer_style") == "traditional" {
		return "cw_lords_prayer_traditional"
	}
	return "cw_lords_prayer_contemporary"
}

func (b *cw2005) prayers() *Section {
	var lines []*Line
	for _, p := range []part{
		{"cw_intercessions_rubric", "rubric"}, {"cw_intercessions_response_rubric", "rubric"},
		{"cw_intercession_response_1_leader", "leader"}, {"cw_intercession_response_1_people", "congregation"},
		{"cw_or_rubric", "rubric"},
		{"cw_intercession_response_2_leader", "leader"}, {"cw_intercession_response_2_people", "congregation"},
		{"cw_silence_rubric", "rubric"}, {"cw_collect_rubric", "rubric"},
	} {
		lines = append(lines, b.FetchLineItem(p.slug, p.typ, nil, "content"))
	}
	lines = append(lines, b.collectLines()...)
	lord := b.lordsPrayerSlug()
	lines = append(lines,
		b.FetchLineItem("cw_lords_prayer_rubric", "rubric", nil, "content"),
		b.FetchLineItem(lord+"_invitation", "leader", nil, "content"),
		b.FetchLineItem(lord+"_people", "congregation", nil, "content"))
	return b.Section("Prayers", "prayers", lines, nil)
}

// collectLines ports build_cw_collect_lines(@collects.first(1)).
func (b *cw2005) collectLines() []*Line {
	if len(b.Collects) == 0 {
		return nil
	}
	c := b.Collects[0]
	if !rb.Present(c.Get("text")) {
		return nil
	}
	var lines []*Line
	if v := c.Get("module_title"); rb.Present(v) {
		lines = append(lines, b.I(v, "heading"))
	}
	if v := c.Get("title"); rb.Present(v) {
		lines = append(lines, b.I(v, "subtitle"))
	}
	if v := c.Get("subtitle"); rb.Present(v) {
		lines = append(lines, b.I(v, "text"))
	}
	lines = append(lines, b.Item(c.Get("text"), "prayer", rb.ToS(c.Get("slug")), ""))
	if l := b.FetchLineItem("cw_amen", "congregation", nil, "content"); l != nil {
		lines = append(lines, l)
	}
	return lines
}

func (b *cw2005) fixedReading(slug, name string) *Section {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	return b.Section(name, "reading", []*Line{b.Item(t.Content, "text", t.Slug, Ref(t))}, nil)
}

// weekday ports date.strftime("%A").downcase.
func weekdayName(d civil.Date) string {
	return strings.ToLower(d.Time().Weekday().String())
}

// --- Morning / Evening -------------------------------------------------------------------

func (b *cw2005) morning() []*Section {
	return Pipeline(
		One(func() *Section {
			if b.PrefS("morning_preparation") == "dawn_acclamation" {
				return b.section("Preparation", "preparation", []part{
					{"cw_morning_dawn_acclamation_replacement_rubric", "rubric"},
					{"cw_morning_dawn_acclamation_opening_leader", "leader"},
					{"cw_morning_dawn_acclamation_opening_people", "congregation"},
					{"cw_morning_dawn_acclamation_refrain_rubric", "rubric"},
					{"cw_morning_dawn_acclamation_refrain_daystar_people", "congregation"},
					{"cw_morning_dawn_acclamation_alternative_rubric_1", "rubric"},
					{"cw_morning_dawn_acclamation_refrain_true_light_people", "congregation"},
					{"cw_morning_dawn_acclamation_alternative_rubric_2", "rubric"},
					{"cw_morning_dawn_acclamation_refrain_salvation_people", "congregation"},
					{"cw_morning_dawn_acclamation_venite_leader_1", "leader"},
					{"cw_morning_dawn_acclamation_venite_people_1", "congregation"},
					{"cw_morning_dawn_acclamation_venite_leader_2", "leader"},
					{"cw_morning_dawn_acclamation_venite_leader_3", "leader"},
					{"cw_morning_dawn_acclamation_venite_leader_4", "leader"},
					{"cw_morning_dawn_acclamation_venite_people_2", "congregation"},
					{"cw_morning_dawn_acclamation_venite_leader_5", "leader"},
					{"cw_morning_dawn_acclamation_venite_leader_6", "leader"},
					{"cw_morning_dawn_acclamation_venite_leader_7", "leader"},
					{"cw_morning_dawn_acclamation_venite_people_3", "congregation"},
					{"cw_morning_dawn_acclamation_gloria", "congregation"},
					{"cw_morning_dawn_acclamation_repeat_rubric", "rubric"},
					{"cw_morning_dawn_acclamation_thanksgiving_rubric", "rubric"},
					{"cw_morning_dawn_acclamation_thanksgiving", "leader"},
					{"cw_morning_dawn_acclamation_thanksgiving_response", "congregation"},
					{"cw_morning_dawn_acclamation_continuation_rubric", "rubric"},
				})
			}
			return b.section("Preparation", "preparation", append(b.openingParts("morning"), b.penitenceParts()...))
		}),
		One(func() *Section { return b.psalmody("cw_psalmody_rubric") }),
		One(func() *Section { return b.canticle("cw_song_of_david", "A Song of David", "canticle") }),
		One(b.readings),
		One(func() *Section {
			return b.section("Responsory", "responsory", []part{
				{"cw_responsory_rubric", "rubric"},
				{"cw_morning_responsory_leader_1", "leader"}, {"cw_morning_responsory_people_1", "congregation"},
				{"cw_morning_responsory_leader_2", "leader"}, {"cw_morning_responsory_people_2", "congregation"},
				{"cw_morning_responsory_leader_3", "leader"}, {"cw_morning_responsory_people_3", "congregation"},
				{"cw_morning_responsory_leader_4", "leader"}, {"cw_morning_responsory_people_4", "congregation"},
				{"cw_morning_responsory_citation", "citation"},
			})
		}),
		One(func() *Section {
			if b.PrefS("morning_gospel_canticle") == "easter_anthems" {
				return b.canticle("cw_easter_anthems", "The Easter Anthems", "gospel_canticle")
			}
			return b.canticle("cw_benedictus", "Benedictus", "gospel_canticle")
		}),
		One(b.prayers),
		One(func() *Section {
			return b.section("The Conclusion", "conclusion", []part{
				{"cw_morning_conclusion_leader", "leader"}, {"cw_morning_conclusion_people", "congregation"},
				{"cw_morning_conclusion_leader_dismissal", "leader"}, {"cw_morning_conclusion_people_dismissal", "congregation"},
			})
		}),
	)
}

func (b *cw2005) openingParts(p string) []part {
	return []part{
		{"cw_" + p + "_opening_leader", "leader"},
		{"cw_" + p + "_opening_people", "congregation"},
		{"cw_" + p + "_opening_prayer_invitation", "rubric"},
		{"cw_" + p + "_opening_prayer_introduction", "leader"},
		{"cw_" + p + "_opening_prayer_silence", "rubric"},
		{"cw_" + p + "_opening_prayer_prayer", "leader"},
		{"cw_" + p + "_opening_prayer_amen", "congregation"},
	}
}

func (b *cw2005) evening() []*Section {
	return Pipeline(
		One(func() *Section {
			if b.PrefS("evening_preparation") == "blessing_of_light" {
				return b.section("Preparation", "preparation", []part{
					{"cw_evening_blessing_of_light_replacement_rubric", "rubric"},
					{"cw_evening_blessing_of_light_candle_rubric", "rubric"},
					{"cw_evening_blessing_of_light_light_leader", "leader"},
					{"cw_evening_blessing_of_light_peace_leader", "leader"},
					{"cw_evening_blessing_of_light_peace_people", "congregation"},
					{"cw_evening_blessing_of_light_prayer_rubric", "rubric"},
					{"cw_evening_blessing_of_light_prayer", "leader"},
					{"cw_evening_blessing_of_light_response", "congregation"},
					{"cw_evening_blessing_of_light_continuation_rubric", "rubric"},
				})
			}
			return b.section("Preparation", "preparation", append(b.openingParts("evening"), b.penitenceParts()...))
		}),
		One(func() *Section { return b.psalmody("cw_psalmody_rubric") }),
		One(func() *Section { return b.canticle("cw_song_of_the_lamb", "A Song of the Lamb", "canticle") }),
		One(b.readings),
		One(func() *Section {
			return b.section("Responsory", "responsory", []part{
				{"cw_responsory_rubric", "rubric"},
				{"cw_evening_responsory_leader_1", "leader"}, {"cw_evening_responsory_people_1", "congregation"},
				{"cw_evening_responsory_leader_2", "leader"}, {"cw_evening_responsory_people_2", "congregation"},
				{"cw_evening_responsory_leader_3", "leader"}, {"cw_evening_responsory_people_3", "congregation"},
				{"cw_evening_responsory_citation", "citation"},
			})
		}),
		One(func() *Section {
			if b.PrefS("evening_gospel_canticle") == "nunc_dimittis" {
				return b.canticle("cw_nunc_dimittis", "Nunc dimittis", "gospel_canticle")
			}
			return b.canticle("cw_magnificat", "Magnificat", "gospel_canticle")
		}),
		One(b.prayers),
		One(func() *Section {
			return b.section("The Conclusion", "conclusion", []part{
				{"cw_evening_conclusion_people", "congregation"}, {"cw_evening_conclusion_people_amen", "congregation"},
				{"cw_evening_conclusion_leader_dismissal", "leader"}, {"cw_evening_conclusion_people_dismissal", "congregation"},
			})
		}),
	)
}

// --- Prayer During the Day ---------------------------------------------------------------

func (b *cw2005) midday() []*Section {
	wd := weekdayName(b.Date)
	return Pipeline(
		One(func() *Section {
			return b.section("Preparation", "preparation", []part{
				{"cw_midday_opening_leader", "leader"}, {"cw_midday_opening_people", "congregation"},
				{"cw_midday_preparation_" + wd + "_leader", "leader"},
				{"cw_midday_preparation_" + wd + "_people", "congregation"},
				{"cw_midday_preparation_" + wd + "_citation", "citation"},
			})
		}),
		One(func() *Section {
			return b.Section("Praise", "praise", []*Line{b.FetchLineItem("cw_midday_praise_rubric", "rubric", nil, "content")}, nil)
		}),
		One(b.middayPsalmody),
		One(func() *Section {
			if b.PrefS("midday_reading_source") == "lectionary" {
				return b.readings()
			}
			return b.fixedReading("cw_midday_short_reading_"+wd, "Short Reading")
		}),
		One(func() *Section {
			return b.section("Response", "response", []part{
				{"cw_midday_response_" + wd + "_leader", "leader"},
				{"cw_midday_response_" + wd + "_people", "congregation"},
				{"cw_midday_response_" + wd + "_citation", "citation"},
			})
		}),
		One(b.prayers),
		One(func() *Section {
			return b.section("The Conclusion", "conclusion", []part{
				{"cw_midday_conclusion_" + wd + "_leader", "leader"},
				{"cw_midday_conclusion_" + wd + "_people", "congregation"},
			})
		}),
	)
}

func (b *cw2005) middayPsalmody() *Section {
	cycle := b.PrefS("midday_psalm_cycle")
	if cycle == "weekday_lectionary" {
		return b.psalmody("cw_midday_psalmody_rubric")
	}
	switch cycle {
	case "daily", "four_week", "weekly", "fortnightly", "monthly":
	default:
		cycle = "daily"
	}
	week, hasWeek := 0, false
	if b.DayContext != nil && b.DayContext.HasWeek {
		week, hasWeek = b.DayContext.Week, true
	}
	ref := CwPrayerDuringDayReference(b.Date, cycle, week, hasWeek)
	if rb.BlankString(ref) {
		return b.psalmody("cw_midday_psalmody_rubric")
	}
	refs := (&reading.AlternativeParser{PsalmPrefix: "Psalm"}).Call(ref)
	content, err := bible.NewTextService(b.PsalmScriptureTranslation(), bible.NewSegmentCache()).FetchPassagesBatch(b.Ctx, refs)
	if err != nil {
		panic(err)
	}
	lines := []*Line{
		b.FetchLineItem("cw_midday_psalmody_rubric", "rubric", nil, "content"),
		b.Item(ref, "heading", "", ref),
	}
	for _, r := range refs {
		lines = append(lines, b.BibleContent(content[r])...)
	}
	lines = append(lines,
		b.FetchLineItem("cw_each_psalm_rubric", "rubric", nil, "content"),
		b.FetchLineItem("cw_gloria_patri", "congregation", nil, "content"))
	return b.Section("Psalmody", "psalmody", lines, nil)
}

var (
	cwDaily = [7]string{"Psalm 19", "Psalm 126", "Psalm 17.1-8", "Psalm 48", "Psalm 133", "Psalm 23", "Psalm 63.1-8"}
	cwFour  = [7][4]string{
		{"Psalm 20", "Psalm 34", "Psalm 115.1-13", "Psalm 116"},
		{"Psalm 49", "Psalm 65", "Psalm 104.26-end", "Psalm 50.1-15"},
		{"Psalm 16", "Psalm 25", "Psalm 36", "Psalm 39"},
		{"Psalm 2", "Psalm 44", "Psalm 45", "Psalm 72.1-8"},
		{"Psalm 81", "Psalm 90", "Psalm 101", "Psalm 107.1-16"},
		{"Psalm 112", "Psalm 120", "Psalm 123", "Psalm 124"},
		{"Psalm 130", "Psalm 131", "Psalm 138", "Psalm 139"},
	}
	cwWeekly = [7]string{"Psalm 119.1-32", "Psalm 119.33-56", "Psalm 119.57-80", "Psalm 119.81-104",
		"Psalm 119.105-128", "Psalm 119.129-152", "Psalm 119.153-end"}
	cwFortnight = [7][2]string{
		{"Psalm 119.1-32", "Psalm 121, 122"}, {"Psalm 119.33-56", "Psalm 123, 124"},
		{"Psalm 119.57-80", "Psalm 125, 126"}, {"Psalm 119.81-104", "Psalm 127"},
		{"Psalm 119.105-128", "Psalm 128"}, {"Psalm 119.129-152", "Psalm 129, 130"},
		{"Psalm 119.153-end", "Psalm 131, 133"},
	}
	cwMonthly = [32]string{"",
		"Psalm 119.1-8", "Psalm 119.9-16", "Psalm 119.17-24", "Psalm 119.25-32", "Psalm 119.33-40",
		"Psalm 119.41-48", "Psalm 119.49-56", "Psalm 119.57-64", "Psalm 119.65-72", "Psalm 119.73-80",
		"Psalm 119.81-88", "Psalm 119.89-96", "Psalm 119.97-104", "Psalm 119.105-112", "Psalm 119.113-120",
		"Psalm 119.121-128", "Psalm 119.129-136", "Psalm 119.137-144", "Psalm 119.145-152", "Psalm 119.153-160",
		"Psalm 119.161-168", "Psalm 119.169-end", "Psalm 121, 122", "Psalm 123, 124", "Psalm 125, 126",
		"Psalm 127", "Psalm 128", "Psalm 129", "Psalm 130", "Psalm 131", "Psalm 133"}
)

func floorMod(a, n int) int { return ((a % n) + n) % n }

// cwCycleWeek ports Cw2005EnPrayerDuringDayPsalmTable.cycle_week.
func cwCycleWeek(d civil.Date, week int, hasWeek bool) int {
	if hasWeek {
		return floorMod(week, 4) + 1
	}
	_, cweek := d.Time().ISOWeek()
	return floorMod(cweek+2, 4) + 1
}

// CwPrayerDuringDayReference ports Reading::Cw2005EnPrayerDuringDayPsalmTable.reference_for.
func CwPrayerDuringDayReference(d civil.Date, pattern string, week int, hasWeek bool) string {
	wd := d.Weekday()
	switch pattern {
	case "daily":
		return cwDaily[wd]
	case "four_week":
		return cwFour[wd][floorMod(cwCycleWeek(d, week, hasWeek)-1, 4)]
	case "weekly":
		return cwWeekly[wd]
	case "fortnightly":
		return cwFortnight[wd][floorMod(cwCycleWeek(d, week, hasWeek)-1, 2)]
	case "monthly":
		return cwMonthly[d.Day()]
	}
	return ""
}

// --- Compline ----------------------------------------------------------------------------

var cwComplinePsalms = map[string][]string{
	"sunday": {"cw_compline_psalm_sunday_1", "cw_compline_psalm_sunday_2"}, "monday": {"cw_compline_psalm_monday_1"},
	"tuesday": {"cw_compline_psalm_tuesday_1"}, "wednesday": {"cw_compline_psalm_wednesday_1"},
	"thursday": {"cw_compline_psalm_thursday_1"}, "friday": {"cw_compline_psalm_friday_1"},
	"saturday": {"cw_compline_psalm_saturday_1"},
}

func (b *cw2005) compline() []*Section {
	wd := weekdayName(b.Date)
	return Pipeline(
		One(func() *Section {
			penitence := b.penitenceParts()
			if len(penitence) == 0 {
				penitence = []part{{"cw_compline_preparation_penitence", "congregation"}, {"cw_compline_preparation_penitence_amen", "congregation"}}
			}
			parts := []part{
				{"cw_compline_opening_leader_1", "leader"}, {"cw_compline_opening_people_1", "congregation"},
				{"cw_compline_opening_leader_2", "leader"}, {"cw_compline_opening_people_2", "congregation"},
				{"cw_compline_preparation_silence_rubric", "rubric"}, {"cw_compline_preparation_penitence_rubric", "rubric"},
			}
			parts = append(parts, penitence...)
			parts = append(parts,
				part{"cw_compline_preparation_hymn_rubric", "rubric"}, part{"cw_compline_preparation_hymn", "text"},
				part{"cw_compline_invocation_leader", "leader"}, part{"cw_compline_invocation_people", "congregation"},
				part{"cw_compline_invocation_gloria", "congregation"}, part{"cw_compline_invocation_alleluia", "congregation"})
			return b.section("Preparation", "preparation", parts)
		}),
		One(func() *Section { return b.complinePsalmody(wd) }),
		One(func() *Section { return b.fixedReading("cw_compline_short_reading_"+wd, "Scripture Reading") }),
		One(func() *Section {
			return b.section("Responsory", "responsory", []part{
				{"cw_compline_responsory_leader_1", "leader"}, {"cw_compline_responsory_people_1", "congregation"},
				{"cw_compline_responsory_leader_2", "leader"}, {"cw_compline_responsory_people_2", "congregation"},
				{"cw_compline_responsory_leader_3", "leader"}, {"cw_compline_responsory_people_3", "congregation"},
				{"cw_compline_responsory_easter_rubric", "rubric"},
				{"cw_compline_responsory_apple_leader", "leader"}, {"cw_compline_responsory_apple_people", "congregation"},
			})
		}),
		One(func() *Section {
			return b.section("Nunc dimittis", "gospel_canticle",
				append([]part{{"cw_compline_gospel_canticle_rubric", "rubric"}}, cwCanticleParts["cw_nunc_dimittis"]...))
		}),
		One(func() *Section {
			lord := b.lordsPrayerSlug()
			return b.section("Prayers", "prayers", []part{
				{"cw_compline_intercessions_rubric", "rubric"}, {"cw_compline_collect_heading", "heading"},
				{"cw_compline_collect_rubric", "rubric"},
				{"cw_compline_collect_" + wd + "_prayer", "leader"}, {"cw_compline_collect_" + wd + "_amen", "congregation"},
				{"cw_compline_lords_prayer_rubric", "rubric"},
				{lord + "_invitation", "leader"}, {lord + "_people", "congregation"},
			})
		}),
		One(func() *Section {
			return b.section("The Conclusion", "conclusion", []part{
				{"cw_compline_conclusion_leader_1", "leader"}, {"cw_compline_conclusion_people_1", "congregation"},
				{"cw_compline_conclusion_leader_2", "leader"}, {"cw_compline_conclusion_people_2", "congregation"},
				{"cw_compline_conclusion_leader_3", "leader"}, {"cw_compline_conclusion_people_3", "congregation"},
				{"cw_compline_conclusion_optional_leader", "leader"}, {"cw_compline_conclusion_optional_people", "congregation"},
				{"cw_compline_conclusion_blessing", "leader"}, {"cw_compline_conclusion_amen", "congregation"},
			})
		}),
	)
}

func (b *cw2005) complinePsalmody(wd string) *Section {
	var refs []string
	for _, slug := range cwComplinePsalms[wd] {
		if t := b.T(slug); t != nil {
			refs = append(refs, t.Content)
		}
	}
	content, err := bible.NewTextService(b.PsalmScriptureTranslation(), bible.NewSegmentCache()).FetchPassagesBatch(b.Ctx, refs)
	if err != nil {
		panic(err)
	}
	lines := []*Line{b.FetchLineItem("cw_compline_psalmody_rubric", "rubric", nil, "content")}
	for _, slug := range cwComplinePsalms[wd] {
		t := b.T(slug)
		if t == nil || rb.BlankString(t.Content) {
			continue
		}
		ref := t.Content
		lines = append(lines, b.Item(ref, "heading", slug, ref))
		if c := content[ref]; c != nil {
			lines = append(lines, b.BibleContent(c)...)
		}
	}
	lines = append(lines,
		b.FetchLineItem("cw_compline_psalmody_end_rubric", "rubric", nil, "content"),
		b.FetchLineItem("cw_gloria_patri", "congregation", nil, "content"))
	return b.Section("Psalmody", "psalmody", lines, nil)
}
