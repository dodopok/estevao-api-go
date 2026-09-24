package dailyoffice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// loc2019Profile ports Loc2019Localization::PROFILES[..][:behavior].
type loc2019Profile map[string]string

var loc2019Profiles = map[string]loc2019Profile{
	"pt": {
		"family_morning_opening": "traditional", "family_morning_psalm": "generic", "family_morning_reading": "generic",
		"family_morning_collect": "generic", "family_midday_psalm": "generic", "family_midday_reading": "generic",
		"family_midday_collect": "generic", "family_evening_opening": "traditional", "family_evening_psalm": "generic",
		"family_evening_reading": "generic", "family_evening_collect": "generic", "family_compline_psalm_type": "congregation",
		"family_compline_reading": "generic", "family_compline_collect": "generic", "morning_canticle_nil_safe": "false",
		"evening_reading_extras": "true",
	},
	"en": {
		"family_morning_opening": "traditional", "family_morning_psalm": "generic", "family_morning_reading": "generic",
		"family_morning_collect": "generic", "family_midday_psalm": "generic", "family_midday_reading": "generic",
		"family_midday_collect": "generic", "family_evening_opening": "traditional", "family_evening_psalm": "generic",
		"family_evening_reading": "generic", "family_evening_collect": "generic", "family_compline_psalm_type": "congregation",
		"family_compline_reading": "generic", "family_compline_collect": "generic", "morning_canticle_nil_safe": "true",
		"evening_reading_extras": "false",
	},
	"es": {
		"family_morning_opening": "response", "family_morning_psalm": "fixed", "family_morning_reading": "fixed",
		"family_morning_collect": "fixed", "family_midday_psalm": "fixed", "family_midday_reading": "fixed",
		"family_midday_collect": "fixed", "family_evening_opening": "fixed", "family_evening_psalm": "fixed",
		"family_evening_reading": "fixed", "family_evening_collect": "fixed", "family_compline_reading": "fixed",
		"family_compline_collect": "fixed", "family_compline_psalm_type": "text", "morning_canticle_nil_safe": "true",
		"evening_reading_extras": "true",
	},
}

func init() {
	for code, locale := range map[string]string{"loc_2019": "pt", "loc_2019_en": "en", "loc_2019_es": "es"} {
		profile := loc2019Profiles[locale]
		Register(code, func(ctx context.Context, c *Context) Builder {
			switch c.OfficeType {
			case "morning", "evening", "midday", "compline":
			default:
				UnknownOfficeType(c.OfficeType)
			}
			b := &loc2019{profile: profile}
			b.Init(ctx, c)
			return b
		})
	}
}

type loc2019 struct {
	Base
	profile loc2019Profile
}

func (b *loc2019) Call() *rb.Map {
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

func (b *loc2019) behavior(key string) string {
	v, ok := b.profile[key]
	if !ok {
		rb.RaiseKeyError(":" + key)
	}
	return v
}

func (b *loc2019) generic(key string) bool { return b.behavior(key) == "generic" }

// label ports loc2019_label (loc2019_catalogue_text(:label, key)).
func (b *loc2019) label(key string) string {
	slug := "loc2019_label_" + key
	var t = b.Texts()[slug]
	if b.PrayerBook() == nil || t == nil {
		code := ""
		if pb := b.PrayerBook(); pb != nil {
			code = pb.Code
		}
		slog.Error(fmt.Sprintf("Missing LiturgicalText %q for %s", slug, code))
		return rb.Humanize(key)
	}
	return t.Content
}

func (b *loc2019) familyRite() bool { return b.PrefS("office_type") == "family" }

// line helpers: `line_item(t.content, slug: t.slug) if t` with a type.
func (b *loc2019) add(lines []*Line, slug, typ string) []*Line {
	if t := b.T(slug); t != nil {
		lines = append(lines, b.TextLine(t, typ))
	}
	return lines
}

func (b *loc2019) vr(lines []*Line, v, r string) []*Line {
	lines = b.add(lines, v, "leader")
	return b.add(lines, r, "congregation")
}

func (b *loc2019) textAndCitation(lines []*Line, slug string, always bool) []*Line {
	t := b.T(slug)
	if t == nil {
		return lines
	}
	lines = append(lines, b.TextLine(t, "text"))
	if always || t.Reference != nil {
		lines = append(lines, b.I(ptrVal(t.Reference), "citation"))
	}
	return lines
}

func (b *loc2019) lordsPrayerSlug() string {
	if b.PrefS("lords_prayer_version") == "traditional" {
		return "our_father_traditional"
	}
	return "our_father_contemporary"
}

func (b *loc2019) prefOr(key, fallback string) string {
	v := b.Pref(key)
	if v == nil || v == false {
		return fallback
	}
	return rb.ToS(v)
}

func (b *loc2019) seasonDown() string { return strings.ToLower(b.Season()) }

func (b *loc2019) easterToPentecost() bool {
	s := b.seasonDown()
	return s == "páscoa" || s == "pentecostes"
}

// --- Base (family helpers) ------------------------------------------------------------------

func (b *loc2019) familyPsalmFromPref(key, fixedSlug, fixedName string) *Section {
	if b.PrefS(key) == "daily" {
		if p := b.Readings.Psalm; p != nil {
			lines := []*Line{b.I(p.Reference, "heading"), b.Spacer()}
			if p.Content != nil {
				lines = append(lines, b.BibleContent(p.Content)...)
			}
			return b.Section(p.Reference, "psalms", lines, nil)
		}
	}
	t := b.T(fixedSlug)
	if t == nil {
		return nil
	}
	return b.Section(fixedName, "psalms", []*Line{b.TextLine(t, "congregation")}, nil)
}

func (b *loc2019) familyReadingFromPref(key, fixedPrefix string, maxFixed int) []*Section {
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
		result = compactSections(b.familyLectionaryModule(b.Readings.Slot(opts[b.SeededNumber(0, 2, key)]), ""))
	case "all":
		result = compactSections(
			b.familyLectionaryModule(b.Readings.FirstReading, b.label("old_testament")),
			b.familyLectionaryModule(b.Readings.SecondReading, b.label("new_testament")),
			b.familyLectionaryModule(b.Readings.Gospel, b.label("gospel")),
		)
	default:
		lectionary = false
	}
	if lectionary && len(result) > 0 {
		return result
	}
	raw := b.ResolveRange(key, 1, maxFixed)
	num := 0
	if l, ok := raw.([]any); ok {
		if len(l) > 0 {
			num = rb.StringToI(rb.ToS(l[0]))
		}
	} else {
		num = rb.ToI(raw)
	}
	if num < 1 || num > maxFixed {
		num = 1
	}
	t := b.T(fmt.Sprintf("%s_%d", fixedPrefix, num))
	if t == nil {
		return []*Section{}
	}
	return []*Section{b.Section(b.label("reading"), "reading", []*Line{
		b.TextLine(t, "leader"),
		b.FetchLineItem("reading_citation", "citation", [][2]string{{"reference", Ref(t)}}, "content"),
	}, nil)}
}

func (b *loc2019) familyCollectReplacingClosing(key, fixedSlug string) *Section {
	switch b.PrefS(key) {
	case "office", "lectionary":
		if len(b.Collects) == 0 {
			return nil
		}
		var lines []*Line
		for _, c := range b.Collects {
			if v := c.Get("preface"); rb.Present(v) {
				lines = append(lines, b.I(v, "leader"))
			}
			lines = append(lines, b.I(c.Get("text"), "leader"), b.Spacer())
		}
		return b.Section(b.label("collect"), "collects", lines, nil)
	}
	t := b.T(fixedSlug)
	if t == nil {
		return nil
	}
	return b.Section(b.label("collect"), "collects", []*Line{b.TextLine(t, "text")}, nil)
}

func (b *loc2019) familyLectionaryModule(r *reading.Passage, label string) *Section {
	if r == nil || rb.BlankString(r.Reference) {
		return nil
	}
	lines := []*Line{b.I(r.Reference, "heading"), b.Spacer()}
	if r.Content != nil {
		lines = append(lines, b.BibleContent(r.Content)...)
	}
	if label == "" {
		label = b.label("readings")
	}
	return b.Section(label, "reading", lines, nil)
}

func (b *loc2019) familyLordsPrayer() *Section {
	t := b.T(b.lordsPrayerSlug())
	if t == nil {
		return nil
	}
	return b.Section(b.label("lords_prayer"), "lords_prayer", []*Line{b.TextLine(t, "text")}, nil)
}

func (b *loc2019) optionalCreed() *Section {
	t := b.T("apostles_creed")
	if t == nil {
		return nil
	}
	return b.Section(b.label("optional_creed"), "creed", []*Line{
		b.FetchLineItem("loc2019_phrase_optional_creed", "rubric", nil, "content"), b.TextLine(t, "text"),
	}, nil)
}

func (b *loc2019) needText(slug string) *Line {
	t := b.T(slug)
	if t == nil {
		rb.RaiseNoMethodOnNil("content")
	}
	return b.TextLine(t, "text")
}

func (b *loc2019) phrase(slug, typ string) *Line { return b.FetchLineItem(slug, typ, nil, "content") }

// --- Morning ---------------------------------------------------------------------------------

func (b *loc2019) morningIsLent() bool   { return b.seasonDown() == "quaresma" }
func (b *loc2019) morningIsEaster() bool { return b.seasonDown() == "páscoa" }
func (b *loc2019) morningIsAdvent() bool { return b.seasonDown() == "advento" }

func (b *loc2019) sentence(prefix, prefKey string) *store.LiturgicalText {
	num := Or(b.ResolveRange(prefKey, 1, 3), 1)
	var t *store.LiturgicalText
	if s := SeasonToOpeningSentenceSlug(b.Season(), false); s != "" {
		t = b.T(prefix + s)
	}
	if t == nil {
		t = b.T(prefix + rubyInterp(num))
	}
	return t
}

func (b *loc2019) morning() []*Section {
	if b.familyRite() {
		return b.familyMorning()
	}
	return Pipeline(One(b.mOpeningSentence), One(b.mConfession), One(b.mInvitatory), One(b.mInvitatoryCanticle),
		One(b.mPsalms), Many(b.mLessons), One(b.creed), One(b.mPrayers), One(b.mCollects), One(b.mMission),
		One(b.mThanksgiving), One(b.chrysostom), One(b.mDismissal))
}

func (b *loc2019) mOpeningSentence() *Section {
	t := b.sentence("morning_opening_sentence_", "morning_opening_sentence")
	if t == nil {
		return nil
	}
	return b.Section(b.label("opening_sentence"), "opening_sentence", []*Line{b.TextLine(t, "text"), b.I(ptrVal(t.Reference), "citation")}, nil)
}

func (b *loc2019) confessionLines(rubricSlug, typeKey string) []*Line {
	var lines []*Line
	lines = b.add(lines, rubricSlug, "rubric")
	if b.PrefS(typeKey) == "long" {
		lines = b.add(lines, "morning_confession_exhortation", "text")
	} else {
		lines = b.add(lines, "morning_confession_invitation_short", "text")
	}
	lines = append(lines, b.Spacer(), b.phrase("loc2019_phrase_silence", "rubric"), b.Spacer())
	lines = b.add(lines, "morning_confession_body", "text")
	lines = b.add(lines, "prayer_for_pardon_lay_rubric", "rubric")
	return b.add(lines, "prayer_for_pardon_lay", "text")
}

func (b *loc2019) mConfession() *Section {
	return b.Section(b.label("confession"), "confession", b.confessionLines("morning_confession_rubric", "morning_confession_type"), nil)
}

func (b *loc2019) invLines(lines []*Line) []*Line {
	for i := 1; i <= 4; i++ {
		lines = b.vr(lines, fmt.Sprintf("morning_inv_v%d", i), fmt.Sprintf("morning_inv_r%d", i))
	}
	return lines
}

func (b *loc2019) mInvitatory() *Section {
	lines := b.invLines(nil)
	seasonSlug := SeasonToAntiphonSlug(b.Season(), false)
	ordinary := b.Pref("morning_ordinary_antiphon")
	if ordinary == nil {
		ordinary = b.PreferenceDefault("morning_ordinary_antiphon")
	}
	antiphonSlug := seasonSlug
	if antiphonSlug == "" && ordinary == "common" {
		antiphonSlug = "common"
	}
	if antiphonSlug != "" {
		if a := b.T("morning_antiphon_" + antiphonSlug); a != nil {
			lines = append(lines, b.Spacer(), b.TextLine(a, "leader"))
		}
	}
	return b.Section(b.label("invitatory"), "invitatory", lines, nil)
}

func (b *loc2019) mInvitatoryCanticle() *Section {
	slug := "pascha_nostrum"
	if !b.morningIsEaster() {
		slug = b.prefOr("morning_invitatory_canticle", "venite")
	}
	var lines []*Line
	if slug == "venite" {
		lines = b.add(lines, "venite_body", "text")
		full := b.Pref("morning_venite_full")
		if full == nil {
			full = b.PreferenceDefault("morning_venite_full")
		}
		if b.morningIsLent() || full == true {
			lines = b.add(lines, "venite_lent_addition", "text")
		}
	} else {
		lines = b.add(lines, slug, "text")
	}
	cantSlug := slug
	if slug == "venite" {
		cantSlug = "venite_body"
	}
	var name any = b.label("invitatory_canticle")
	if c := b.T(cantSlug); c != nil && c.Title != nil {
		name = *c.Title
	}
	return b.Section(name, "invitatory_canticle", lines, nil)
}

func (b *loc2019) mPsalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.phrase("loc2019_label_psalms_heading", "heading"), b.I(p.Reference, "subtitle"), b.Spacer()}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "gloria_patri", "text")
	return b.Section(b.label("psalms"), "psalms", lines, nil)
}

func (b *loc2019) morningLessonLines(r *reading.Passage) []*Line {
	lines := []*Line{b.FetchLineItem("loc2019_phrase_reading_from", "leader", [][2]string{{"reference", r.Reference}}, "content"), b.Spacer()}
	if r.Content != nil {
		lines = append(lines, b.BibleContent(r.Content)...)
	}
	return append(lines, b.Spacer(), b.phrase("loc2019_phrase_word_of_lord", "leader"), b.phrase("loc2019_phrase_thanks", "congregation"))
}

func (b *loc2019) mLessons() []*Section {
	var mods []*Section
	if r := b.Readings.FirstReading; r != nil {
		mods = append(mods, b.SectionWithReadingExtras(b.label("first_lesson"), "first_lesson", b.morningLessonLines(r),
			"first_reading", rb.M("reference", r.Reference), b.morningLessonLines))
		cSlug := "te_deum_part_1"
		if b.morningIsLent() || b.morningIsAdvent() {
			cSlug = "benedictus_es_domine"
		}
		if c := b.T(cSlug); c != nil {
			lines := []*Line{b.TextLine(c, "text")}
			if cSlug == "te_deum_part_1" {
				lines = b.add(lines, "te_deum_part_2_optional", "text")
			}
			var name any = b.label("canticle")
			if c.Title != nil {
				name = *c.Title
			}
			mods = append(mods, b.Section(name, cSlug, lines, nil))
		}
	}
	if r := b.Readings.SecondReading; r != nil {
		mods = append(mods, b.SectionWithReadingExtras(b.label("second_lesson"), "second_lesson", b.morningLessonLines(r),
			"second_reading", rb.M("reference", r.Reference), b.morningLessonLines))
		if c := b.T("benedictus"); c != nil {
			mods = append(mods, b.Section(Title(c), "benedictus", []*Line{b.TextLine(c, "text")}, nil))
		}
	}
	return mods
}

func (b *loc2019) creed() *Section {
	return b.Section(b.label("creed"), "creed", []*Line{b.phrase("loc2019_phrase_officiant_people_together", "rubric"), b.needText("apostles_creed")}, nil)
}

func (b *loc2019) kyrieLines(lines []*Line) []*Line {
	for _, s := range []string{"v", "r", "v2"} {
		typ := "leader"
		if s == "r" {
			typ = "congregation"
		}
		lines = b.add(lines, "kyrie_"+s, typ)
	}
	return lines
}

func (b *loc2019) suffrages(lines []*Line) []*Line {
	for i := 1; i <= 7; i++ {
		lines = b.vr(lines, fmt.Sprintf("suff_v%d", i), fmt.Sprintf("suff_r%d", i))
	}
	return lines
}

func (b *loc2019) mPrayers() *Section {
	lines := []*Line{b.phrase("loc2019_phrase_lord_be_with_you", "leader"), b.phrase("loc2019_phrase_with_your_spirit", "congregation"),
		b.phrase("loc2019_phrase_let_us_pray", "leader"), b.Spacer()}
	lines = b.kyrieLines(lines)
	lines = append(lines, b.Spacer())
	lines = b.add(lines, b.lordsPrayerSlug(), "text")
	lines = append(lines, b.Spacer())
	return b.Section(b.label("prayers"), "prayers", b.suffrages(lines), nil)
}

func (b *loc2019) mCollects() *Section {
	return b.Section(b.label("collects"), "collects", b.CollectLines(b.Collects), nil)
}

func (b *loc2019) mMission() *Section {
	return b.Section(b.label("mission_prayer"), "mission_prayer", []*Line{b.needText("prayer_for_mission_" + b.prefOr("morning_prayer_for_mission", "1"))}, nil)
}

func (b *loc2019) mThanksgiving() *Section {
	return b.Section(b.label("thanksgiving"), "thanksgiving", []*Line{b.phrase("loc2019_phrase_officiant_people", "rubric"), b.needText("general_thanksgiving")}, nil)
}

func (b *loc2019) chrysostom() *Section {
	return b.Section(b.label("chrysostom"), "chrysostom", []*Line{b.needText("chrysostom_prayer")}, nil)
}

func (b *loc2019) sentenceLines(lines []*Line, slug string) []*Line {
	if s := b.T(slug); s != nil {
		lines = append(lines, b.TextLine(s, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	return lines
}

func (b *loc2019) mDismissal() *Section {
	lines := b.vr(nil, "dismissal_v", "dismissal_r")
	lines = append(lines, b.Spacer())
	lines = b.sentenceLines(lines, "concluding_sentence_"+b.prefOr("morning_concluding_sentence", "1"))
	return b.Section(b.label("dismissal"), "dismissal", lines, nil)
}

func (b *loc2019) familyMorning() []*Section {
	var readings Step
	if b.generic("family_morning_reading") {
		r := b.familyReadingFromPref("family_morning_reading", "family_morning_reading", 3)
		readings = Many(func() []*Section { return r })
	} else {
		readings = One(b.familyMorningReading)
	}
	return Pipeline(One(b.familyMorningOpening), One(b.familyMorningPsalm), readings, One(b.familyLordsPrayer),
		One(b.familyMorningCollect), One(b.optionalCreed))
}

func (b *loc2019) familyMorningOpening() *Section {
	if b.behavior("family_morning_opening") == "traditional" && b.PrefS("family_morning_opening_source") == "traditional" {
		t := b.sentence("morning_opening_sentence_", "morning_opening_sentence")
		if t == nil {
			return nil
		}
		return b.Section(b.label("family_opening_sentence"), "opening", []*Line{b.TextLine(t, "text"), b.I(ptrVal(t.Reference), "citation")}, nil)
	}
	return b.Section(b.label("opening"), "opening", b.vr(nil, "family_morning_opening_v", "family_morning_opening_r"), nil)
}

func (b *loc2019) familyFixedPsalm(slug, behaviorKey string) *Section {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	typ := "text"
	if b.generic(behaviorKey) {
		typ = "congregation"
	}
	lines := []*Line{b.TextLine(t, typ)}
	lines = b.add(lines, "gloria_patri", "text")
	return b.Section(b.label("psalm"), "psalms", lines, nil)
}

func (b *loc2019) familyMorningPsalm() *Section {
	if b.generic("family_morning_psalm") && b.PrefS("family_morning_psalm") == "daily" {
		return b.familyPsalmFromPref("family_morning_psalm", "family_morning_psalm", b.label("psalm"))
	}
	return b.familyFixedPsalm("family_morning_psalm", "family_morning_psalm")
}

func (b *loc2019) familyFixedReading(key, prefix string, max int) *Section {
	num := Or(b.ResolveRange(key, 1, max), 1)
	t := b.T(prefix + "_" + rubyInterp(num))
	if t == nil {
		return nil
	}
	lines := []*Line{b.TextLine(t, "text")}
	if t.Reference != nil {
		lines = append(lines, b.I(*t.Reference, "citation"))
	}
	return b.Section(b.label("reading"), "reading", lines, nil)
}

func firstSection(list []*Section) *Section {
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func (b *loc2019) familyMorningReading() *Section {
	if b.generic("family_morning_reading") {
		return firstSection(b.familyReadingFromPref("family_morning_reading", "family_morning_reading", 3))
	}
	return b.familyFixedReading("family_morning_reading", "family_morning_reading", 3)
}

func (b *loc2019) familyFixedCollect(slug string) *Section {
	t := b.T(slug)
	if t == nil {
		return nil
	}
	return b.Section(b.label("collect"), "collects", []*Line{b.TextLine(t, "text")}, nil)
}

func (b *loc2019) familyMorningCollect() *Section {
	if b.generic("family_morning_collect") {
		return b.familyCollectReplacingClosing("family_morning_collect", "family_morning_collect")
	}
	return b.familyFixedCollect("family_morning_collect")
}

// --- Evening -----------------------------------------------------------------------------------

func (b *loc2019) evening() []*Section {
	if b.familyRite() {
		return b.familyEarlyEvening()
	}
	return Pipeline(One(b.eOpeningSentence), One(b.eConfession), One(b.eInvitatory), One(b.ePsalms), Many(b.eLessons),
		One(b.creed), One(b.ePrayers), One(b.eCollects), One(b.eMission), One(b.eThanksgiving), One(b.chrysostom),
		One(b.eDismissal))
}

func (b *loc2019) eveningSentenceLines() []*Line {
	t := b.sentence("evening_opening_sentence_", "evening_opening_sentence")
	var lines []*Line
	lines = b.add(lines, "evening_opening_rubric", "rubric")
	if t != nil {
		lines = append(lines, b.TextLine(t, "text"))
		if t.Reference != nil {
			lines = append(lines, b.I(*t.Reference, "citation"))
		}
	}
	return lines
}

func (b *loc2019) eOpeningSentence() *Section {
	return b.Section(b.label("opening_sentence"), "opening_sentence", b.eveningSentenceLines(), nil)
}

func (b *loc2019) eConfession() *Section {
	return b.Section(b.label("confession"), "confession", b.confessionLines("evening_confession_rubric", "evening_confession_type"), nil)
}

func (b *loc2019) phosLines(lines []*Line) []*Line {
	if p := b.T("phos_hilaron"); p != nil {
		lines = append(lines, b.I(Title(p), "heading"), b.TextLine(p, "text"))
	}
	return lines
}

func (b *loc2019) eInvitatory() *Section {
	lines := b.invLines([]*Line{b.phrase("loc2019_phrase_all_stand", "rubric")})
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "evening_phos_hilaron_rubric", "rubric")
	return b.Section(b.label("invitatory"), "invitatory", b.phosLines(lines), nil)
}

func (b *loc2019) ePsalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := []*Line{b.phrase("loc2019_label_psalms_heading", "heading"), b.I(p.Reference, "subtitle"), b.Spacer()}
	if p.Content != nil {
		lines = append(lines, b.BibleContent(p.Content)...)
	}
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "midday_gloria_patri_rubric", "rubric")
	lines = b.add(lines, "gloria_patri", "text")
	return b.Section(b.label("psalms"), "psalms", lines, nil)
}

func (b *loc2019) eLessons() []*Section {
	var mods []*Section
	firstRubric, citationRubric := b.T("evening_reading_first_rubric"), b.T("evening_reading_citation_rubric")
	afterRubric, endRubric := b.T("evening_reading_after_rubric"), b.T("evening_reading_end_rubric")
	canticleRubric := b.T("evening_canticle_rubric")
	extras := b.behavior("evening_reading_extras") == "true"
	withExtras := func(name, slug string, lines []*Line, key string, r *reading.Passage, fmtLines func(*reading.Passage) []*Line) *Section {
		meta := rb.M("reference", r.Reference)
		if extras {
			b.ReadingExtras(key, fmtLines).Each(func(k string, v any) { meta.Set(k, v) })
		}
		return b.Section(name, slug, lines, meta)
	}
	if r := b.Readings.FirstReading; r != nil {
		fmtLines := func(r *reading.Passage) []*Line {
			var lines []*Line
			if firstRubric != nil {
				lines = append(lines, b.TextLine(firstRubric, "rubric"))
			}
			lines = append(lines, b.FetchLineItem("loc2019_phrase_reading_from", "leader", [][2]string{{"reference", r.Reference}}, "content"))
			if citationRubric != nil {
				lines = append(lines, b.TextLine(citationRubric, "rubric"))
			}
			lines = append(lines, b.Spacer())
			if r.Content != nil {
				lines = append(lines, b.BibleContent(r.Content)...)
			}
			lines = append(lines, b.Spacer())
			if afterRubric != nil {
				lines = append(lines, b.TextLine(afterRubric, "rubric"))
			}
			lines = append(lines, b.phrase("loc2019_phrase_word_of_lord", "leader"), b.phrase("loc2019_phrase_thanks", "congregation"))
			if endRubric != nil {
				lines = append(lines, b.TextLine(endRubric, "rubric"))
			}
			return append(lines, b.phrase("loc2019_phrase_here_ends_reading", "leader"))
		}
		mods = append(mods, withExtras(b.label("first_lesson"), "first_lesson", fmtLines(r), "first_reading", r, fmtLines))
		var cl []*Line
		if canticleRubric != nil {
			cl = append(cl, b.TextLine(canticleRubric, "rubric"))
		}
		c := b.T("magnificat")
		cl = b.add(cl, "magnificat", "text")
		var name any = "Magnificat"
		if c != nil && c.Title != nil {
			name = *c.Title
		}
		mods = append(mods, b.Section(name, "magnificat", cl, nil))
	}
	if r := b.Readings.SecondReading; r != nil {
		fmtLines := func(r *reading.Passage) []*Line {
			lines := []*Line{b.FetchLineItem("loc2019_phrase_reading_from", "leader", [][2]string{{"reference", r.Reference}}, "content"), b.Spacer()}
			if r.Content != nil {
				lines = append(lines, b.BibleContent(r.Content)...)
			}
			return append(lines, b.Spacer(), b.phrase("loc2019_phrase_word_of_lord", "leader"), b.phrase("loc2019_phrase_thanks", "congregation"),
				b.phrase("loc2019_phrase_here_ends_reading", "leader"))
		}
		mods = append(mods, withExtras(b.label("second_lesson"), "second_lesson", fmtLines(r), "second_reading", r, fmtLines))
		c := b.T("nunc_dimittis")
		var name any = "Nunc Dimittis"
		if c != nil && c.Title != nil {
			name = *c.Title
		}
		if c == nil {
			rb.RaiseNoMethodOnNil("content")
		}
		mods = append(mods, b.Section(name, "nunc_dimittis", []*Line{b.TextLine(c, "text")}, nil))
	}
	return mods
}

func (b *loc2019) ePrayers() *Section {
	lines := []*Line{b.phrase("loc2019_phrase_lord_be_with_you", "leader"), b.phrase("loc2019_phrase_with_your_spirit", "congregation"),
		b.phrase("loc2019_phrase_let_us_pray", "leader"), b.phrase("loc2019_phrase_people_kneel", "rubric"), b.Spacer()}
	lines = b.kyrieLines(lines)
	lines = append(lines, b.Spacer(), b.phrase("loc2019_phrase_officiant_people", "rubric"))
	lines = b.add(lines, b.lordsPrayerSlug(), "text")
	lines = append(lines, b.Spacer())
	if b.prefOr("evening_suffrages_type", "a") == "b" {
		rubric, response := b.T("evening_suff_b_rubric"), b.T("evening_suff_b_response")
		if rubric != nil {
			lines = append(lines, b.TextLine(rubric, "leader"))
		}
		if response != nil {
			lines = append(lines, b.TextLine(response, "congregation"))
		}
		lines = append(lines, b.Spacer())
		for i := 1; i <= 5; i++ {
			lines = b.add(lines, fmt.Sprintf("evening_suff_b_%d", i), "leader")
			if response != nil {
				lines = append(lines, b.I(response.Content, "congregation"))
			}
		}
	} else {
		lines = b.suffrages(lines)
	}
	return b.Section(b.label("prayers"), "prayers", lines, nil)
}

func (b *loc2019) eCollects() *Section {
	lines := b.add(nil, "evening_collects_rubric", "rubric")
	return b.Section(b.label("collects"), "collects", append(lines, b.CollectLines(b.Collects)...), nil)
}

func (b *loc2019) eMission() *Section {
	num := b.prefOr("evening_prayer_for_mission", "1")
	lines := b.add(nil, "evening_mission_rubric", "rubric")
	lines = b.add(lines, "evening_prayer_for_mission_"+num, "text")
	return b.Section(b.label("mission_prayer"), "mission_prayer", lines, nil)
}

func (b *loc2019) eThanksgiving() *Section {
	lines := b.add(nil, "evening_thanksgiving_rubric", "rubric")
	lines = append(lines, b.phrase("loc2019_phrase_officiant_people", "rubric"))
	lines = b.add(lines, "general_thanksgiving", "text")
	return b.Section(b.label("thanksgiving"), "thanksgiving", lines, nil)
}

func (b *loc2019) eDismissal() *Section {
	num := b.prefOr("evening_concluding_sentence", "1")
	lines := b.vr(nil, "dismissal_v", "dismissal_r")
	if b.easterToPentecost() {
		if t := b.T("loc2019_phrase_easter_alleluia"); t != nil {
			lines = append(lines, b.Item(rb.Strip(t.Content), "congregation", t.Slug, ""))
		}
	}
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "evening_conclusion_rubric", "rubric")
	lines = b.sentenceLines(lines, "concluding_sentence_"+num)
	return b.Section(b.label("dismissal"), "dismissal", lines, nil)
}

func (b *loc2019) familyEarlyEvening() []*Section {
	if b.behavior("family_evening_opening") == "fixed" {
		return Pipeline(One(b.familyEarlyEveningOpening), One(b.familyEarlyEveningReading), One(b.familyLordsPrayer), One(b.familyEarlyEveningCollect))
	}
	r := b.familyReadingFromPref("family_early_evening_reading", "family_early_evening_reading", 3)
	return Pipeline(One(b.familyEarlyEveningOpening), One(b.familyEarlyEveningPsalm), Many(func() []*Section { return r }),
		One(b.familyLordsPrayer), One(b.familyEarlyEveningCollect))
}

func (b *loc2019) familyEarlyEveningOpening() *Section {
	if b.behavior("family_evening_opening") == "traditional" && b.PrefS("family_evening_opening_source") == "traditional" {
		return b.Section(b.label("family_opening_sentence"), "opening", b.eveningSentenceLines(), nil)
	}
	lines := b.sentenceLines(nil, "family_early_evening_opening")
	if b.behavior("family_evening_opening") == "fixed" {
		lines = append(lines, b.Spacer())
		lines = b.phosLines(lines)
	}
	return b.Section(b.label("opening"), "opening", lines, nil)
}

func (b *loc2019) familyEarlyEveningPsalm() *Section {
	if b.PrefS("family_early_evening_psalm") == "daily" {
		return b.familyPsalmFromPref("family_early_evening_psalm", "phos_hilaron", b.label("psalm"))
	}
	p := b.T("phos_hilaron")
	if p == nil {
		return nil
	}
	var name any = b.label("psalm")
	if p.Title != nil {
		name = *p.Title
	}
	return b.Section(name, "psalms", []*Line{b.TextLine(p, "text")}, nil)
}

func (b *loc2019) familyEarlyEveningReading() *Section {
	if b.generic("family_evening_reading") {
		return firstSection(b.familyReadingFromPref("family_early_evening_reading", "family_early_evening_reading", 3))
	}
	return b.familyFixedReading("family_early_evening_reading", "family_early_evening_reading", 3)
}

func (b *loc2019) familyEarlyEveningCollect() *Section {
	if b.generic("family_evening_collect") {
		return b.familyCollectReplacingClosing("family_early_evening_collect", "family_early_evening_collect")
	}
	return b.familyFixedCollect("family_early_evening_collect")
}

// --- Midday -------------------------------------------------------------------------------------

func (b *loc2019) midday() []*Section {
	if b.familyRite() {
		if b.generic("family_midday_reading") {
			r := b.familyReadingFromPref("family_midday_reading", "family_midday_reading", 2)
			return Pipeline(One(b.familyMiddayOpening), One(b.familyMiddayPsalm), Many(func() []*Section { return r }),
				One(b.familyLordsPrayer), One(b.familyMiddayCollect))
		}
		return Pipeline(One(b.familyMiddayOpening), One(b.familyMiddayPsalm), One(b.familyMiddayReading),
			One(b.familyLordsPrayer), One(b.familyMiddayCollect))
	}
	return Pipeline(One(b.mdOpening), One(b.mdPsalms), One(b.mdReading), One(b.mdPrayers), One(b.mdCollects), One(b.mdDismissal))
}

func (b *loc2019) alleluiaResponse(lines []*Line, vSlug, rSlug string) []*Line {
	lines = b.add(lines, vSlug, "leader")
	r := b.T(rSlug)
	if r != nil {
		text := r.Content
		if !b.IsLent() {
			if a := b.T("loc2019_phrase_midday_alleluia"); a != nil {
				text += a.Content
			}
		}
		lines = append(lines, b.Item(text, "congregation", r.Slug, ""))
	}
	return b.add(lines, "midday_opening_lent_rubric", "rubric")
}

func (b *loc2019) mdOpening() *Section {
	lines := b.vr(nil, "midday_inv_v1", "midday_inv_r1")
	lines = b.alleluiaResponse(lines, "midday_inv_v2", "midday_inv_r2")
	lines = b.add(lines, "midday_hymn_rubric", "rubric")
	return b.Section(b.label("opening"), "opening", lines, nil)
}

func (b *loc2019) psalmSelection(rubricSlug, key string, all []string) []*Line {
	lines := b.add(nil, rubricSlug, "rubric")
	for _, s := range Arr(b.ResolveOptions(key, all)) {
		p := b.T(rb.ToS(s))
		if p == nil {
			continue
		}
		lines = append(lines, b.I(Title(p), "heading"), b.I(ptrVal(p.Reference), "rubric"), b.TextLine(p, "text"), b.Spacer())
	}
	lines = b.add(lines, "midday_gloria_patri_rubric", "rubric")
	return b.add(lines, "gloria_patri", "text")
}

func (b *loc2019) mdPsalms() *Section {
	return b.Section(b.label("psalms"), "psalms", b.psalmSelection("midday_psalms_rubric", "midday_psalm_selection",
		[]string{"midday_psalm_119", "midday_psalm_121", "midday_psalm_124", "midday_psalm_126"}), nil)
}

func (b *loc2019) mdReading() *Section {
	num := Or(b.ResolveRange("midday_reading", 1, 3), 1)
	read := b.T("midday_reading_" + rubyInterp(num))
	lines := b.add(nil, "midday_reading_rubric", "rubric")
	if read != nil {
		lines = append(lines, b.TextLine(read, "text"))
		if read.Reference != nil {
			lines = append(lines, b.I(*read.Reference, "citation"))
		}
	}
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "midday_reading_end_rubric", "rubric")
	lines = append(lines, b.phrase("loc2019_phrase_word_of_lord", "leader"), b.phrase("loc2019_phrase_thanks", "congregation"), b.Spacer())
	lines = b.add(lines, "midday_meditation_rubric", "rubric")
	return b.Section(b.label("reading"), "reading", lines, nil)
}

func (b *loc2019) middayKyrie(lines []*Line) []*Line {
	prefix := "midday_kyrie_"
	if b.PrefS("midday_kyrie_version") == "short" {
		prefix = "midday_kyrie_short_"
	}
	for _, s := range []string{"v1", "r1", "v2"} {
		typ := "leader"
		if s == "r1" {
			typ = "congregation"
		}
		lines = b.add(lines, prefix+s, typ)
	}
	return lines
}

func (b *loc2019) mdPrayers() *Section {
	lines := b.add(nil, "midday_prayers_rubric", "rubric")
	lines = b.vr(lines, "midday_pre_v", "midday_pre_r")
	lines = append(lines, b.Spacer())
	lines = b.middayKyrie(lines)
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "midday_officiant_people_rubric", "rubric")
	lines = b.add(lines, b.lordsPrayerSlug(), "text")
	lines = append(lines, b.Spacer())
	lines = b.vr(lines, "midday_post_v", "midday_post_r")
	lines = append(lines, b.phrase("loc2019_phrase_let_us_pray", "leader"))
	return b.Section(b.label("prayers"), "prayers", lines, nil)
}

func (b *loc2019) numberedCollects(lines []*Line, prefix string) []*Line {
	for i := 1; i <= 4; i++ {
		if c := b.T(fmt.Sprintf("%s%d", prefix, i)); c != nil {
			lines = append(lines, b.TextLine(c, "text"), b.Spacer())
		}
	}
	return lines
}

func (b *loc2019) mdCollects() *Section {
	lines := b.add(nil, "midday_collects_rubric", "rubric")
	lines = b.numberedCollects(lines, "midday_collect_")
	lines = b.add(lines, "midday_intercessions_rubric", "rubric")
	return b.Section(b.label("collects"), "collects", lines, nil)
}

func (b *loc2019) mdDismissal() *Section {
	lines := []*Line{b.phrase("loc2019_phrase_let_us_bless", "leader")}
	resp := b.T("loc2019_phrase_dismissal_thanks")
	var text any
	slug := ""
	if resp != nil {
		s := resp.Content
		if b.easterToPentecost() {
			if a := b.T("loc2019_phrase_easter_alleluia"); a != nil {
				s += a.Content
			}
		}
		text, slug = s, resp.Slug
	}
	lines = append(lines, b.Item(text, "congregation", slug, ""))
	lines = b.add(lines, "midday_dismissal_alleluia", "rubric")
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "midday_conclusion_rubric", "rubric")
	lines = b.sentenceLines(lines, "concluding_sentence_"+b.prefOr("midday_concluding_sentence", "1"))
	return b.Section(b.label("dismissal"), "dismissal", lines, nil)
}

func (b *loc2019) familyMiddayOpening() *Section {
	if b.T("family_midday_opening") == nil {
		return nil
	}
	return b.Section(b.label("opening"), "opening", b.sentenceLines(nil, "family_midday_opening"), nil)
}

func (b *loc2019) familyMiddayPsalm() *Section {
	if b.generic("family_midday_psalm") && b.PrefS("family_midday_psalm") == "daily" {
		return b.familyPsalmFromPref("family_midday_psalm", "family_midday_psalm", b.label("psalm"))
	}
	return b.familyFixedPsalm("family_midday_psalm", "family_midday_psalm")
}

func (b *loc2019) familyMiddayReading() *Section {
	if b.generic("family_midday_reading") {
		return firstSection(b.familyReadingFromPref("family_midday_reading", "family_midday_reading", 2))
	}
	return b.familyFixedReading("family_midday_reading", "family_midday_reading", 2)
}

func (b *loc2019) familyMiddayCollect() *Section {
	if b.generic("family_midday_collect") {
		return b.familyCollectReplacingClosing("family_midday_collect", "family_midday_collect")
	}
	return b.familyFixedCollect("family_midday_collect")
}

// --- Compline ------------------------------------------------------------------------------------

func (b *loc2019) compline() []*Section {
	if b.familyRite() {
		if b.generic("family_compline_reading") {
			r := b.familyReadingFromPref("family_end_of_day_reading", "family_end_of_day_reading", 2)
			return Pipeline(One(b.familyEndOpening), One(b.familyEndPsalm), Many(func() []*Section { return r }),
				One(b.familyLordsPrayer), One(b.familyEndCollect), One(b.familyNunc), One(b.familyFinalPrayer))
		}
		return Pipeline(One(b.familyEndOpening), One(b.familyEndPsalm), One(b.familyEndReading), One(b.familyLordsPrayer),
			One(b.familyEndCollect), One(b.familyNunc), One(b.familyFinalPrayer))
	}
	return Pipeline(One(b.cOpening), One(b.cConfession), One(b.cInvitatory), One(b.cPsalms), One(b.cReading),
		One(b.cResponsory), One(b.cPrayers), One(b.cCollects), One(b.cMission), One(b.cThanksgiving), One(b.cNunc),
		One(b.cDismissal))
}

func (b *loc2019) cOpening() *Section {
	lines := b.add(nil, "compline_opening_rubric", "rubric")
	lines = b.add(lines, "compline_opening_sentence", "text")
	lines = b.vr(lines, "compline_inv_v1", "compline_inv_r1")
	return b.Section(b.label("opening"), "opening", lines, nil)
}

func (b *loc2019) cConfession() *Section {
	lines := b.add(nil, "compline_confession_rubric", "rubric")
	lines = b.add(lines, "compline_confession_invitation", "text")
	lines = b.add(lines, "compline_silence_rubric", "rubric")
	lines = b.add(lines, "compline_confession_body", "text")
	lines = b.add(lines, "compline_absolution_rubric", "rubric")
	lines = b.add(lines, "compline_absolution", "text")
	return b.Section(b.label("confession"), "confession", lines, nil)
}

func (b *loc2019) cInvitatory() *Section {
	lines := b.vr(nil, "compline_inv_v2", "compline_inv_r2")
	lines = b.alleluiaResponse(lines, "compline_inv_v3", "compline_inv_r3")
	return b.Section(b.label("invitatory"), "invitatory", lines, nil)
}

func (b *loc2019) cPsalms() *Section {
	return b.Section(b.label("psalms"), "psalms", b.psalmSelection("compline_psalms_rubric", "compline_psalm_selection",
		[]string{"compline_psalm_4", "compline_psalm_31", "compline_psalm_91", "compline_psalm_134"}), nil)
}

func (b *loc2019) cReading() *Section {
	num := Or(b.ResolveRange("compline_reading_selection", 1, 4), 1)
	lines := b.add(nil, "compline_reading_rubric", "rubric")
	if r := b.T("compline_reading_" + rubyInterp(num)); r != nil {
		lines = append(lines, b.TextLine(r, "text"), b.I(ptrVal(r.Reference), "citation"))
	}
	lines = append(lines, b.Spacer(), b.phrase("loc2019_phrase_at_end_reading", "rubric"), b.phrase("loc2019_phrase_word_of_lord", "leader"),
		b.phrase("loc2019_phrase_thanks", "congregation"), b.Spacer())
	lines = b.add(lines, "compline_meditation_rubric", "rubric")
	return b.Section(b.label("reading"), "reading", lines, nil)
}

func (b *loc2019) cResponsory() *Section {
	lines := b.vr(nil, "compline_resp_v1", "compline_resp_r1")
	lines = b.vr(lines, "compline_resp_v2", "compline_resp_r2")
	return b.Section(b.label("responsory"), "responsory", lines, nil)
}

func (b *loc2019) cPrayers() *Section {
	lines := b.middayKyrie(nil)
	lines = append(lines, b.Spacer(), b.phrase("loc2019_phrase_officiant_people", "rubric"))
	lines = b.add(lines, b.lordsPrayerSlug(), "text")
	lines = append(lines, b.Spacer())
	lines = b.vr(lines, "midday_post_v", "midday_post_r")
	lines = append(lines, b.phrase("loc2019_phrase_let_us_pray", "leader"))
	return b.Section(b.label("compline_prayers"), "prayers", lines, nil)
}

func (b *loc2019) cCollects() *Section {
	lines := b.add(nil, "compline_collects_rubric", "rubric")
	lines = b.numberedCollects(lines, "compline_collect_")
	if b.Date.Weekday() == 6 {
		if s := b.T("compline_collect_saturday"); s != nil {
			lines = append(lines, b.I(Title(s), "heading"), b.TextLine(s, "text"), b.Spacer())
		}
	}
	return b.Section(b.label("collects"), "collects", lines, nil)
}

func (b *loc2019) cMission() *Section {
	num := Or(b.ResolveRange("compline_mission_selection", 1, 2), 1)
	lines := b.add(nil, "compline_mission_rubric", "rubric")
	lines = b.add(lines, "compline_mission_"+rubyInterp(num), "text")
	lines = append(lines, b.Spacer())
	lines = b.add(lines, "compline_intercessions_rubric", "rubric")
	return b.Section(b.label("mission_prayer"), "mission_prayer", lines, nil)
}

func (b *loc2019) cThanksgiving() *Section {
	lines := []*Line{b.phrase("loc2019_phrase_before_close", "rubric"), b.phrase("loc2019_phrase_thanksgiving_heading", "heading"),
		b.phrase("loc2019_phrase_officiant_people", "rubric")}
	lines = b.add(lines, "general_thanksgiving", "text")
	return b.Section(b.label("compline_thanksgiving"), "thanksgiving", lines, nil)
}

func (b *loc2019) cNunc() *Section {
	easter := b.seasonDown() == "páscoa"
	lines := b.add(nil, "compline_nunc_rubric", "rubric")
	antSlug := "compline_nunc_antiphon"
	if easter {
		antSlug = "compline_nunc_antiphon_easter"
	}
	ant := b.T(antSlug)
	if ant != nil {
		lines = append(lines, b.TextLine(ant, "text"))
	}
	if er := b.T("compline_nunc_easter_rubric"); er != nil && easter {
		lines = append(lines, b.TextLine(er, "rubric"))
	}
	lines = append(lines, b.Spacer())
	if n := b.T("nunc_dimittis"); n != nil {
		lines = append(lines, b.I(Title(n), "heading"), b.TextLine(n, "text"))
	}
	lines = append(lines, b.Spacer())
	if ant != nil {
		lines = append(lines, b.I(ant.Content, "text"))
	}
	return b.Section(b.label("nunc_dimittis"), "nunc_dimittis", lines, nil)
}

func (b *loc2019) cDismissal() *Section {
	lines := []*Line{b.phrase("loc2019_phrase_let_us_bless", "leader"), b.phrase("loc2019_phrase_dismissal_thanks", "congregation"), b.Spacer()}
	lines = b.add(lines, "compline_conclusion_rubric", "rubric")
	lines = b.add(lines, "compline_conclusion", "text")
	return b.Section(b.label("dismissal"), "dismissal", lines, nil)
}

func (b *loc2019) familyEndOpening() *Section {
	if b.T("family_end_of_day_opening") == nil {
		return nil
	}
	return b.Section(b.label("opening"), "opening", b.sentenceLines(nil, "family_end_of_day_opening"), nil)
}

func (b *loc2019) familyEndPsalm() *Section {
	p := b.T("compline_psalm_134")
	if p == nil {
		return nil
	}
	typ := "text"
	if b.behavior("family_compline_psalm_type") == "congregation" {
		typ = "congregation"
	}
	lines := []*Line{b.TextLine(p, typ)}
	lines = b.add(lines, "gloria_patri", "text")
	return b.Section(b.label("psalm"), "psalms", lines, nil)
}

func (b *loc2019) familyEndReading() *Section {
	if b.generic("family_compline_reading") {
		return firstSection(b.familyReadingFromPref("family_end_of_day_reading", "family_end_of_day_reading", 2))
	}
	return b.familyFixedReading("family_end_of_day_reading", "family_end_of_day_reading", 2)
}

func (b *loc2019) familyEndCollect() *Section {
	if b.generic("family_compline_collect") {
		return b.familyCollectReplacingClosing("family_end_of_day_collect", "family_end_of_day_collect")
	}
	return b.familyFixedCollect("family_end_of_day_collect")
}

func (b *loc2019) familyNunc() *Section {
	n := b.T("nunc_dimittis")
	if n == nil {
		return nil
	}
	ant := b.T("compline_nunc_antiphon")
	var lines []*Line
	if ant != nil {
		lines = append(lines, b.TextLine(ant, "text"))
	}
	lines = append(lines, b.Spacer(), b.I(Title(n), "heading"), b.TextLine(n, "text"), b.Spacer())
	if ant != nil {
		lines = append(lines, b.I(ant.Content, "text"))
	}
	return b.Section(b.label("nunc_dimittis"), "nunc_dimittis", lines, nil)
}

func (b *loc2019) familyFinalPrayer() *Section {
	t := b.T("family_final_prayer")
	if t == nil {
		return nil
	}
	return b.Section(b.label("family_final_prayer"), "dismissal", []*Line{b.TextLine(t, "text")}, nil)
}
