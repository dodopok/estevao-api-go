package dailyoffice

import (
	"context"
	"fmt"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/collects"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/reading"
	"github.com/dodopok/estevao-api-go/internal/rx"
	"github.com/dodopok/estevao-api-go/internal/store"
)

func init() {
	Register("dwdo_2021", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "midday", "evening", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &dwdo{}
		b.Init(ctx, c)
		b.observedDate = b.Date
		if b.OfficeType == "evening" {
			b.applyFirstEvensong()
		}
		return b
	})
}

// dwdo ports DailyOffice::Builders::Dwdo2021::* (Divine Worship: Daily
// Office, 2021) and its SharedRubrics.
type dwdo struct {
	Base
	firstEvensong bool
	observedDate  civil.Date
	observedCal   *liturgical.Calendar
	eveDate       civil.Date
	eveSet        bool
	eve           *reading.Selection
	titles        []string
	psalmBible    *bible.TextService
}

func (b *dwdo) Call() *rb.Map {
	var sections []*Section
	switch b.OfficeType {
	case "morning":
		sections = b.morning()
	case "evening":
		sections = b.evening()
	case "midday":
		sections = b.midday()
	default:
		sections = b.compline()
	}
	result := b.Render(sections)
	// Ordinary Sundays stay the primary observance in DWDO responses.
	if b.observedDate.IsSunday() {
		if c, ok := result.Get("celebration").(*rb.Map); ok {
			switch rb.ToS(c.Get("type")) {
			case "major_holy_day", "festival", "lesser_feast", "commemoration":
				result.Set("celebration", nil)
			}
		}
	}
	if b.firstEvensong {
		result.Get("metadata").(*rb.Map).Set("first_evensong", true)
	}
	return result
}

// --- First Evensong ----------------------------------------------------------------------

func (b *dwdo) calendarFor(d civil.Date) *liturgical.Calendar {
	if d.Year() == b.Date.Year() {
		return b.Calendar()
	}
	return liturgical.NewCalendar(d.Year(), b.PrefS("prayer_book_code"))
}

func (b *dwdo) observedCalendar() *liturgical.Calendar {
	if b.observedCal == nil {
		b.observedCal = b.calendarFor(b.observedDate)
	}
	return b.observedCal
}

func (b *dwdo) translation() string {
	if v := b.Pref("bible_version"); v != nil && v != false {
		return rb.ToS(v)
	}
	return "nvi"
}

func (b *dwdo) service(d civil.Date, cal *liturgical.Calendar, serviceType string) *reading.Resolver {
	return reading.For(b.Ctx, d, reading.Options{
		PrayerBookCode: b.PrefS("prayer_book_code"), Calendar: cal, Translation: b.translation(),
		PsalmTranslation: b.PrefString("psalm_translation"), ReadingType: b.PrefString("reading_type"),
		ServiceType: serviceType, LoadContent: true,
	})
}

// applyFirstEvensong: Sundays and feasts with an explicit Eve row begin with
// First Evensong; at equal precedence the current day keeps its Second Evensong.
func (b *dwdo) applyFirstEvensong() {
	tomorrow := b.Date.Add(1)
	cal := b.calendarFor(tomorrow)
	info := cal.DayInfo(tomorrow)
	if !(tomorrow.IsSunday() || b.explicitEve(tomorrow, cal) != nil) {
		return
	}
	if dwdoPrecedence(info, tomorrow) >= dwdoPrecedence(b.DayInfo, b.Date) {
		return
	}
	b.firstEvensong = true
	b.observedDate = tomorrow
	b.observedCal = cal
	b.DayInfo = info
	b.Readings = b.firstEvensongReadings(tomorrow, cal)
	b.Collects = collects.New(b.Ctx, tomorrow, collects.Options{
		PrayerBookCode: b.PrefS("prayer_book_code"), Calendar: cal, OfficeType: "evening",
		IncludeFixedOfficeCollect: true,
	}).FindCollects()
}

func (b *dwdo) explicitEve(d civil.Date, cal *liturgical.Calendar) *reading.Selection {
	if b.eveSet && b.eveDate == d {
		return b.eve
	}
	b.eveSet, b.eveDate = true, d
	b.eve = b.service(d, cal, "vigil").Selection()
	return b.eve
}

func (b *dwdo) firstEvensongReadings(tomorrow civil.Date, cal *liturgical.Calendar) *reading.Selection {
	course := b.service(b.Date, b.Calendar(), "evening_prayer").DailyCourseSelection()
	if course == nil {
		course = b.Readings
	}
	eve := b.explicitEve(tomorrow, cal)
	if eve == nil || (eve.FirstReading == nil && eve.SecondReading == nil) {
		return course
	}
	first, second := eve.FirstReading, eve.SecondReading
	if first == nil {
		first = course.FirstReading
	}
	if second == nil {
		second = course.SecondReading
	}
	return &reading.Selection{
		FirstReading: first, Psalm: course.Psalm, PsalmAlternative: course.PsalmAlternative,
		SecondReading: second, Gospel: course.Gospel, Notes: course.Notes,
	}
}

func dwdoPrecedence(info *rb.Map, d civil.Date) int {
	typ := ""
	if c, ok := info.Get("celebration").(*rb.Map); ok {
		typ = rb.ToS(c.Get("type"))
	}
	season := rb.ToS(info.Get("liturgical_season"))
	if d.IsSunday() && info.Get("liturgical_season") != nil {
		for _, s := range liturgical.ReadingPrecedenceSeasons {
			if s == season {
				return 0
			}
		}
	}
	switch {
	case typ == "principal_feast":
		return 0
	case d.IsSunday():
		return 10
	case typ == "major_holy_day":
		return 20
	case typ == "festival":
		return 30
	}
	return 100
}

// --- SharedRubrics: titles -----------------------------------------------------------------

var dwdoOrdinals = [][2]string{
	{"1ST", "FIRST"}, {"2ND", "SECOND"}, {"3RD", "THIRD"}, {"4TH", "FOURTH"},
	{"5TH", "FIFTH"}, {"6TH", "SIXTH"}, {"7TH", "SEVENTH"}, {"8TH", "EIGHTH"},
	{"9TH", "NINTH"}, {"10TH", "TENTH"}, {"11TH", "ELEVENTH"}, {"12TH", "TWELFTH"},
	{"13TH", "THIRTEENTH"}, {"14TH", "FOURTEENTH"}, {"15TH", "FIFTEENTH"},
	{"16TH", "SIXTEENTH"}, {"17TH", "SEVENTEENTH"}, {"18TH", "EIGHTEENTH"},
	{"19TH", "NINETEENTH"}, {"20TH", "TWENTIETH"}, {"21ST", "TWENTY FIRST"},
	{"22ND", "TWENTY SECOND"}, {"23RD", "TWENTY THIRD"}, {"24TH", "TWENTY FOURTH"},
	{"25TH", "TWENTY FIFTH"}, {"26TH", "TWENTY SIXTH"}, {"27TH", "TWENTY SEVENTH"},
}

var dwdoOrdinalRes = func() []*rx.Regexp {
	out := make([]*rx.Regexp, len(dwdoOrdinals))
	for i, o := range dwdoOrdinals {
		out[i] = rx.MustCompile(`\b` + o[0] + `\b`)
	}
	return out
}()

var dwdoSaintRe = rx.MustCompile(`\bST\b`)

// normalizeLiturgicalTitle ports normalize_liturgical_title.
func normalizeLiturgicalTitle(title string) string {
	up := strings.ToUpper(strings.ReplaceAll(title, "ß", "SS"))
	up = strings.ReplaceAll(up, "_", " ")
	var sb strings.Builder
	for _, r := range up {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte(' ')
		}
	}
	s := strings.TrimSpace(squeezeSpaces(sb.String()))
	for i, o := range dwdoOrdinals {
		s = dwdoOrdinalRes[i].Gsub(s, o[1])
	}
	s = dwdoSaintRe.Gsub(s, "SAINT")
	var words []string
	for _, w := range strings.Fields(s) {
		if w != "THE" && w != "OF" && w != "IN" {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

func squeezeSpaces(s string) string {
	var sb strings.Builder
	prev := false
	for _, r := range s {
		if r == ' ' {
			if prev {
				continue
			}
			prev = true
		} else {
			prev = false
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func (b *dwdo) lookupTitles() []string {
	if b.titles != nil {
		return b.titles
	}
	var raw []string
	for _, k := range []string{"celebration", "saint"} {
		if m, ok := b.DayInfo.Get(k).(*rb.Map); ok {
			if n := m.Get("name"); n != nil {
				raw = append(raw, rb.ToS(n))
			}
		}
	}
	raw = append(raw, reading.MapSundayWithAliases(b.observedDate, b.observedCalendar())...)
	t := b.observedDate.Time()
	raw = append(raw, fmt.Sprintf("%s %d", t.Month().String(), t.Day()))
	seen := map[string]bool{}
	var normalized []string
	for _, r := range raw {
		n := normalizeLiturgicalTitle(r)
		if !seen[n] {
			seen[n] = true
			normalized = append(normalized, n)
		}
	}
	if b.firstEvensong {
		seen = map[string]bool{}
		var out []string
		for _, n := range normalized {
			for _, x := range []string{n, "EVE " + n} {
				if !seen[x] {
					seen[x] = true
					out = append(out, x)
				}
			}
		}
		normalized = out
	}
	if normalized == nil {
		normalized = []string{}
	}
	b.titles = normalized
	return normalized
}

func (b *dwdo) titleMatches(pattern string) bool {
	wanted := strings.Fields(normalizeLiturgicalTitle(pattern))
	for _, title := range b.lookupTitles() {
		words := map[string]bool{}
		for _, w := range strings.Fields(title) {
			words[w] = true
		}
		all := true
		for _, w := range wanted {
			if !words[w] {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// bestMatch ports select(prefix, reference present, matching).max_by(word count)
// over the texts in catalogue order (the first maximum wins).
func (b *dwdo) bestMatch(keep func(*store.LiturgicalText) bool) *store.LiturgicalText {
	var best *store.LiturgicalText
	bestN := -1
	for _, t := range store.LiturgicalTextsOrdered(b.Ctx, b.PrayerBook()) {
		if !keep(t) || t.Reference == nil || rb.BlankString(*t.Reference) || !b.titleMatches(*t.Reference) {
			continue
		}
		if n := len(strings.Fields(normalizeLiturgicalTitle(*t.Reference))); n > bestN {
			best, bestN = t, n
		}
	}
	return best
}

// --- SharedRubrics: office hymn ----------------------------------------------------------

type hymnRule struct {
	slug     string
	patterns []string
}

var dwdoMorningHymns = []hymnRule{
	{"morning_office_hymn_verbum_supernum", []string{"FIRST SUNDAY OF ADVENT", "SECOND SUNDAY OF ADVENT"}},
	{"morning_office_hymn_vox_clara", []string{"THIRD SUNDAY OF ADVENT", "FOURTH SUNDAY OF ADVENT", "DECEMBER 24"}},
	{"morning_office_hymn_a_solis_ortus", []string{"CHRISTMAS DAY", "NATIVITY OF THE LORD"}},
	{"morning_office_hymn_sancte_dei", []string{"SAINT STEPHEN"}},
	{"morning_office_hymn_o_gente_felix", []string{"HOLY FAMILY"}},
	{"morning_office_hymn_audit_tyrannus", []string{"HOLY INNOCENTS"}},
	{"morning_office_hymn_ave_radix", []string{"MARY THE HOLY MOTHER OF GOD", "MARY MOTHER OF GOD"}},
	{"morning_office_hymn_a_patre", []string{"EPIPHANY", "BAPTISM OF THE LORD"}},
	{"morning_office_hymn_aeterne_rerum", []string{"SUNDAY AFTER EPIPHANY", "SEPTUAGESIMA", "SEXAGESIMA", "QUINQUAGESIMA"}},
	{"morning_office_hymn_o_gloriosa", []string{"CANDLEMAS", "PRESENTATION OF THE LORD", "ANNUNCIATION"}},
	{"morning_office_hymn_jesu_quadragenarie", []string{"THIRD SUNDAY LENT", "FOURTH SUNDAY LENT"}},
}

var dwdoEveningHymns = []hymnRule{
	{"evening_office_hymn_conditor_alme", []string{"SUNDAY ADVENT"}},
	{"evening_office_hymn_veni_redemptor", []string{"EVE NATIVITY OF THE LORD", "EVE CHRISTMAS DAY"}},
	{"evening_office_hymn_christe_redemptor", []string{"NATIVITY OF THE LORD", "CHRISTMAS DAY"}},
	{"evening_office_hymn_deus_tuorum", []string{"SAINT STEPHEN"}},
	{"evening_office_hymn_o_lux_beata", []string{"EVE HOLY FAMILY"}},
	{"evening_office_hymn_sacrae_iam", []string{"HOLY FAMILY"}},
	{"evening_office_hymn_salvete_flores", []string{"HOLY INNOCENTS"}},
	{"evening_office_hymn_corde_natus", []string{"MARY THE HOLY MOTHER OF GOD", "MARY MOTHER OF GOD"}},
	{"evening_office_hymn_hostis_herodes", []string{"EPIPHANY", "BAPTISM OF THE LORD"}},
	{"evening_office_hymn_dulce_carmen", []string{"EVE SEPTUAGESIMA"}},
	{"evening_office_hymn_deus_creator", []string{"EVE SUNDAY AFTER EPIPHANY", "EVE SEXAGESIMA", "EVE QUINQUAGESIMA"}},
	{"evening_office_hymn_lucis_creator", []string{"SUNDAY AFTER EPIPHANY", "SEPTUAGESIMA", "SEXAGESIMA", "QUINQUAGESIMA"}},
	{"evening_office_hymn_quod_chorus", []string{"EVE CANDLEMAS", "EVE PRESENTATION OF THE LORD"}},
	{"evening_office_hymn_hail_to_the_lord", []string{"CANDLEMAS", "PRESENTATION OF THE LORD"}},
	{"evening_office_hymn_ecce_tempus", []string{"THIRD SUNDAY LENT", "FOURTH SUNDAY LENT"}},
	{"evening_office_hymn_ave_maris_stella", []string{"EVE ANNUNCIATION"}},
	{"evening_office_hymn_quem_terra", []string{"ANNUNCIATION"}},
}

func (b *dwdo) properHymnSlug(morning bool) string {
	rules := dwdoEveningHymns
	if morning {
		rules = dwdoMorningHymns
	}
	for _, r := range rules {
		for _, p := range r.patterns {
			if b.titleMatches(p) {
				return r.slug
			}
		}
	}
	return ""
}

func (b *dwdo) currentSeasonSlug() string {
	return strings.ReplaceAll(rb.Parameterize(rb.ToS(b.DayInfo.Get("liturgical_season"))), "-", "_")
}

func (b *dwdo) officeHymn(morning bool) *Section {
	prefix, segment := "evening", "evening"
	if morning {
		prefix, segment = "morning", "morning"
	} else if b.firstEvensong {
		segment = "first_evensong"
	}
	hymnPrefix := "office_hymn_" + segment + "_"
	proper := b.bestMatch(func(t *store.LiturgicalText) bool {
		return strings.HasPrefix(t.Slug, hymnPrefix) && !strings.HasSuffix(t.Slug, "_vandr")
	})
	var vandr *store.LiturgicalText
	hymn := proper
	if proper != nil {
		vandr = b.T(proper.Slug + "_vandr")
	}
	if hymn == nil {
		if s := b.properHymnSlug(morning); s != "" {
			hymn = b.T(s)
		}
	}
	if hymn == nil {
		hymn = b.T(prefix + "_office_hymn_" + b.currentSeasonSlug())
	}
	if hymn == nil {
		hymn = b.T(prefix + "_office_hymn")
	}
	if hymn == nil {
		return nil
	}
	lines := b.plain(nil, prefix+"_office_hymn_rubric", "rubric")
	lines = append(lines, b.I(Title(hymn), "heading"), b.I(hymn.Content, "text"))
	if proper != nil && proper == hymn && vandr != nil {
		// String#split("\n", 2): an empty content has no fields.
		var versicle, response any
		if vandr.Content != "" {
			parts := strings.SplitN(vandr.Content, "\n", 2)
			versicle = parts[0]
			if len(parts) > 1 {
				response = parts[1]
			}
		}
		lines = append(lines, b.I(versicle, "leader"), b.I(response, "congregation"))
	}
	return b.Section("", "office_hymn", lines, nil)
}

// --- SharedRubrics: readings, canticles, psalms ------------------------------------------

// readingModule ports dwdo_reading_module: the lesson title goes after the
// announcement rubric, before the Scripture reference.
func (b *dwdo) readingModule(typ, pre, title string) *Section {
	mod := b.ReadingModule(typ, "reading_announcement_rubric", Rubrics{Pre: pre}, typ+"_reading", "", true)
	at := len(mod.Lines)
	for i, l := range mod.Lines {
		if l.Type == "heading" {
			at = i
			break
		}
	}
	lines := make([]*Line, 0, len(mod.Lines)+1)
	lines = append(lines, mod.Lines[:at]...)
	lines = append(lines, b.I(title, "heading"))
	lines = append(lines, mod.Lines[at:]...)
	return b.Section(mod.Name, mod.Slug, lines, mod.Meta)
}

func (b *dwdo) antiphonedCanticleLines(c *store.LiturgicalText, kind string) []*Line {
	a := b.properCanticleAntiphon(kind)
	if a == nil {
		return b.canticleLines(c, true)
	}
	lines := []*Line{b.I(a.Content, "antiphon")}
	lines = append(lines, b.canticleLines(c, true)...)
	return append(lines, b.I(a.Content, "antiphon"))
}

func (b *dwdo) properCanticleAntiphon(kind string) *store.LiturgicalText {
	if kind == "magnificat" && b.Date.Month() == 12 && b.Date.Day() >= 17 && b.Date.Day() <= 23 {
		if g := b.T(fmt.Sprintf("evening_magnificat_antiphon_great_o_dec%d", b.Date.Day())); g != nil {
			return g
		}
	}
	prefix := "evening_magnificat_antiphon_"
	if kind == "benedictus" {
		prefix = "morning_benedictus_antiphon_"
	}
	return b.bestMatch(func(t *store.LiturgicalText) bool { return strings.HasPrefix(t.Slug, prefix) })
}

func (b *dwdo) canticleLines(c *store.LiturgicalText, gloria bool) []*Line {
	lines := []*Line{b.I(c.Content, "text")}
	if gloria {
		if g := b.gloriaPatriLine(); g != nil {
			lines = append(lines, g)
		}
	}
	return lines
}

func (b *dwdo) easter(key string) civil.Date {
	e := liturgical.EasterFor(b.Date.Year(), liturgical.Gregorian)
	if key == "easter" {
		return e.EasterDate
	}
	return e.Dates[key]
}

func (b *dwdo) triduum() bool {
	return b.Date >= b.easter("maundy_thursday") && b.Date < b.easter("easter")
}

func (b *dwdo) passiontide() bool {
	e := b.easter("easter")
	return b.Date >= e.Add(-14) && b.Date < e
}

func (b *dwdo) gloriaPatriLine() *Line {
	if b.triduum() {
		return nil
	}
	if t := b.T("psalms_gloria_patri"); t != nil {
		return b.I(t.Content, "congregation")
	}
	return nil
}

func (b *dwdo) appointedPsalms() *Section {
	p := b.Readings.Psalm
	if p == nil {
		return nil
	}
	lines := b.plain(nil, "psalms_appointed_rubric", "rubric")
	lines = append(lines, b.psalmLines(p, nil)...)
	return b.Section("", "psalms", lines, nil)
}

func (b *dwdo) fixedPsalms(refs []string) *Section {
	lines := b.plain(nil, "psalms_fixed_rubric", "rubric")
	lines = append(lines, b.psalmLines(nil, refs)...)
	return b.Section("", "psalms", lines, nil)
}

var (
	dwdoPsalmPrefixRe = rx.MustCompile(`(?i)\A\s*Psalms?\s*`)
	dwdoCommaRe       = rx.MustCompile(`\s*,\s*`)
	dwdoCoverdaleRe   = rx.MustCompile(`(?i)\APsalm\s+(\d+)(?::(\d+)(?:-(\d+|end))?)?\z`)
	dwdoSpaceRe       = rx.MustCompile(`\s+`)
)

// splitPsalmReferences: "Psalms 24, 25, 26" => ["Psalm 24", "Psalm 25", "Psalm 26"].
func splitPsalmReferences(ref string) []string {
	if rb.BlankString(ref) {
		return nil
	}
	var out []string
	for _, part := range dwdoCommaRe.Split(dwdoPsalmPrefixRe.Sub(ref, ""), 0) {
		if !rb.BlankString(part) {
			out = append(out, "Psalm "+part)
		}
	}
	return out
}

func (b *dwdo) psalmLines(p *reading.Passage, fixed []string) []*Line {
	refs := fixed
	if refs == nil && p != nil {
		refs = splitPsalmReferences(p.Reference)
	}
	if len(refs) == 0 {
		return nil
	}
	var single *rb.Map
	if len(refs) == 1 && p != nil {
		single = p.Content
	}
	var lines []*Line
	for _, ref := range refs {
		var content *rb.Map
		if b.SelectedPsalmTranslation() == "prayer_book" {
			content = b.coverdale(ref)
		}
		if content == nil {
			content = single
		}
		if content == nil {
			content = b.fallbackPsalm(ref)
		}
		lines = append(lines, b.I(ref, "heading"))
		if content != nil {
			lines = append(lines, b.BibleContent(content)...)
		}
		if g := b.gloriaPatriLine(); g != nil {
			lines = append(lines, g)
		}
	}
	return lines
}

func (b *dwdo) coverdale(ref string) *rb.Map {
	match := dwdoCoverdaleRe.Find(ref)
	if match == nil {
		return nil
	}
	m := []string{match.String(), match.G(1), match.G(2), match.G(3)}
	psalm := store.PsalmsFor(b.Ctx, b.PrayerBook())[rb.StringToI(m[1])]
	if psalm == nil {
		return nil
	}
	all := psalm.FormattedVerses()
	if len(all) == 0 {
		return nil
	}
	hasFirst := m[2] != ""
	first, last := rb.StringToI(m[2]), 0
	switch {
	case strings.EqualFold(m[3], "end"):
		last = rubyToI(all[len(all)-1].Get("number"))
	case m[3] != "":
		last = rb.StringToI(m[3])
	default:
		last = rb.StringToI(m[2])
	}
	var verses []any
	for _, v := range all {
		n := rubyToI(v.Get("number"))
		if !hasFirst || (n >= first && n <= last) {
			verses = append(verses, v)
		}
	}
	if len(verses) == 0 {
		return nil
	}
	return rb.M("verses", verses)
}

func (b *dwdo) fallbackPsalm(ref string) *rb.Map {
	if b.psalmBible == nil {
		b.psalmBible = bible.NewTextService(b.PsalmScriptureTranslation(), bible.NewSegmentCache())
	}
	m, err := b.psalmBible.FetchPassageStructured(b.Ctx, ref)
	if err != nil {
		panic(err)
	}
	return m
}

// --- SharedRubrics: collects and seasons -------------------------------------------------

func (b *dwdo) advent() bool { return rb.ToS(b.DayInfo.Get("liturgical_season")) == "Advento" }

func (b *dwdo) lent() bool {
	return b.Date >= b.easter("ash_wednesday") && b.Date < b.easter("easter")
}

func (b *dwdo) preLent() bool {
	return b.Date >= b.easter("septuagesima") && b.Date < b.easter("ash_wednesday")
}

func (b *dwdo) seasonalAdditionalCollects() []*rb.Map {
	var records []*store.Collect
	switch {
	case b.advent():
		records = store.CollectsFor(b.Ctx, b.PrayerBook()).BySunday["1st_sunday_of_advent"]
	case b.lent():
		records = b.ashWednesdayCollects()
	}
	// include? compares nil and strings by value: key nil apart from "".
	key := func(v any) string {
		if v == nil {
			return "\x00nil"
		}
		return rb.ToS(v)
	}
	existing := map[string]bool{}
	for _, c := range b.Collects {
		existing[key(c.Get("text"))] = true
	}
	title := "Ash Wednesday"
	if b.advent() {
		title = "The First Sunday of Advent"
	}
	var out []*rb.Map
	for _, r := range records {
		if existing[key(rb.Deref(r.Text))] {
			continue
		}
		out = append(out, rb.M("text", r.Text, "module_title", "Seasonal Collect", "title", title))
	}
	return out
}

func (b *dwdo) ashWednesdayCollects() []*store.Collect {
	bc, err := store.CelebrationsForBook(b.Ctx, b.PrayerBook())
	if err != nil {
		panic(err)
	}
	for _, c := range bc.All {
		if c.CalculationRule != nil && (*c.CalculationRule == "ash_wednesday" || *c.CalculationRule == "easter_minus_46_days") {
			return store.CollectsFor(b.Ctx, b.PrayerBook()).ByCelebration[c.ID]
		}
	}
	return nil
}

func (b *dwdo) teDeumAppointed() bool {
	switch rb.ToS(b.celebrationField("type")) {
	case "principal_feast", "major_holy_day", "festival":
		return true
	}
	pentecost := b.easter("pentecost")
	eastertide := b.Date >= b.easter("easter") && b.Date <= pentecost
	christmasOctave := (b.Date.Month() == 12 && b.Date.Day() >= 25) || (b.Date.Month() == 1 && b.Date.Day() == 1)
	pentecostOctave := b.Date >= pentecost && b.Date < pentecost.Add(7)
	if eastertide || christmasOctave || pentecostOctave {
		return true
	}
	if !b.Date.IsSunday() {
		return false
	}
	return !b.advent() && !b.preLent() && !b.lent()
}

func withoutModuleTitle(list []*rb.Map) []*rb.Map {
	out := make([]*rb.Map, len(list))
	for i, m := range list {
		c := m.Dup()
		c.Delete("module_title")
		out[i] = c
	}
	return out
}

func (b *dwdo) collectOfTheDay(prefix string) *Section {
	lines := b.plain(nil, prefix+"_collects_rubric", "rubric")
	lines = append(lines, b.CollectLines(withoutModuleTitle(b.Collects))...)
	lines = append(lines, b.CollectLines(b.seasonalAdditionalCollects())...)
	return b.Section("Collect of the Day", "collect_of_the_day", lines, nil)
}

func (b *dwdo) fixedCollects(slugs ...string) *Section {
	var lines []*Line
	for _, s := range slugs {
		if t := b.T(s); t != nil {
			lines = append(lines, b.I(Title(t), "heading"), b.I(t.Content, "text"))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section("Fixed Collects", "fixed_collects", lines, nil)
}

func (b *dwdo) conclusion() *Section {
	lines := b.plain(nil, "conclusion_rubric", "rubric")
	for _, s := range []string{"prayer_kings_majesty", "prayer_royal_family", "prayer_clergy_people", "prayer_st_chrysostom", "the_grace"} {
		t := b.T(s)
		if t == nil {
			continue
		}
		if t.Title != nil {
			lines = append(lines, b.I(*t.Title, "heading"))
		}
		lines = append(lines, b.I(t.Content, "text"))
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section("The Conclusion", "conclusion", lines, nil)
}

func (b *dwdo) layOfficiant() bool {
	v, ok := b.ResolveOptions("officiant", []string{"priest", "lay"}).(string)
	return ok && v == "lay"
}

func (b *dwdo) lesserLitanyLines() []*Line {
	form := "priest"
	if b.layOfficiant() {
		form = "lay"
	}
	return []*Line{
		b.I(b.contentOrNil("lesser_litany_"+form+"_v"), "leader"),
		b.I(b.contentOrNil("lesser_litany_"+form+"_r"), "congregation"),
		b.I(b.contentOrNil("lesser_litany_kyrie_v1"), "leader"),
		b.I(b.contentOrNil("lesser_litany_kyrie_r1"), "congregation"),
		b.I(b.contentOrNil("lesser_litany_kyrie_v2"), "leader"),
	}
}

func (b *dwdo) introduction(prefix string) *Section {
	if !(b.observedDate.IsSunday() || rb.ToS(b.celebrationField("type")) == "principal_feast") {
		return nil
	}
	lines := b.plain(nil, "intro_rubric", "rubric")
	if s := b.T(fmt.Sprintf("intro_sentence_%d", b.Date.YearDay()%11+1)); s != nil {
		lines = append(lines, b.I(s.Content, "text"))
		if s.Reference != nil {
			lines = append(lines, b.I(*s.Reference, "citation"))
		}
	}
	if e := b.tAny(Or(b.ResolveOptions("exhortation", []string{"intro_exhortation", "intro_exhortation_alt"}), "intro_exhortation")); e != nil {
		lines = append(lines, b.I(Title(e), "heading"), b.I(e.Content, "text"))
	}
	lines = b.plain(lines, "intro_confession_rubric", "rubric")
	lines = b.plain(lines, "intro_confession", "text")
	absolution := "intro_absolution"
	if b.layOfficiant() {
		absolution = "intro_absolution_lay"
	}
	if a := b.T(absolution); a != nil {
		lines = append(lines, b.I(Title(a), "heading"), b.I(a.Content, "leader"))
	}
	lines = b.plain(lines, "intro_lords_prayer_rubric", "rubric")
	lines = b.plain(lines, prefix+"_lords_prayer", "congregation")
	return b.Section("The Introduction", "introduction", lines, nil)
}

func (b *dwdo) openingVersicles(prefix string) *Section {
	lines := b.plain(nil, prefix+"_opening_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil(prefix+"_opening_v1"), "leader"),
		b.I(b.contentOrNil(prefix+"_opening_r1"), "congregation"),
		b.I(b.contentOrNil(prefix+"_opening_v2"), "leader"),
		b.I(b.contentOrNil(prefix+"_opening_r2"), "congregation"))
	if !b.triduum() {
		lines = append(lines, b.I(b.contentOrNil(prefix+"_gloria_patri"), "congregation"))
	}
	lines = append(lines, b.I(b.contentOrNil(prefix+"_alleluia_rubric"), "rubric"))
	return b.Section("", "opening_versicles", lines, nil)
}

func (b *dwdo) preces(prefix string) *Section {
	lines := b.plain(nil, prefix+"_preces_rubric", "rubric")
	lines = append(lines, b.lesserLitanyLines()...)
	lines = append(lines, b.I(b.contentOrNil(prefix+"_lords_prayer"), "congregation"))
	for i := 1; i <= 6; i++ {
		lines = append(lines,
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_v%d", prefix, i)), "leader"),
			b.I(b.contentOrNil(fmt.Sprintf("%s_suff_r%d", prefix, i)), "congregation"))
	}
	return b.Section("", "preces", lines, nil)
}

// --- Mattins -----------------------------------------------------------------------------

func (b *dwdo) morning() []*Section {
	return Pipeline(
		One(func() *Section { return b.introduction("morning") }),
		One(func() *Section { return b.openingVersicles("morning") }),
		One(b.mInvitatory),
		One(b.appointedPsalms),
		One(func() *Section { return b.readingModule("first", "morning_first_reading_rubric", "The First Lesson") }),
		One(b.mFirstCanticle),
		One(func() *Section {
			return b.readingModule("second", "morning_second_reading_rubric", "The Second Lesson")
		}),
		One(func() *Section { return b.officeHymn(true) }),
		One(func() *Section {
			c := b.T("morning_benedictus")
			if c == nil {
				return nil
			}
			return b.Section(Title(c), "second_canticle", b.antiphonedCanticleLines(c, "benedictus"), nil)
		}),
		One(b.mCreed),
		One(func() *Section { return b.preces("morning") }),
		One(func() *Section { return b.collectOfTheDay("morning") }),
		One(func() *Section { return b.fixedCollects("morning_collect_peace", "morning_collect_grace") }),
		One(b.conclusion),
	)
}

func (b *dwdo) mInvitatory() *Section {
	if b.triduum() {
		return nil
	}
	lines := b.plain(nil, "morning_invitatory_rubric", "rubric")
	slug := b.invitatorySlug()
	c := b.tAny(slug)
	if c == nil {
		return nil
	}
	lines = append(lines, b.I(c.Content, "text"))
	if slug != "morning_easter_anthems" {
		if b.passiontide() {
			lines = b.plain(lines, "morning_gloria_omitted_rubric", "rubric")
		} else {
			lines = b.plain(lines, "morning_gloria_patri", "congregation")
		}
	}
	return b.Section(Title(c), "invitatory", lines, nil)
}

func (b *dwdo) invitatorySlug() any {
	e := b.easter("easter")
	if b.Date >= e && b.Date < e.Add(7) {
		return "morning_easter_anthems"
	}
	if b.Date.Day() == 19 && b.monthlyPsalmody() {
		return "morning_jubilate"
	}
	return Or(b.ResolveOptions("morning_invitatory", []string{"morning_venite", "morning_jubilate"}), "morning_venite")
}

func (b *dwdo) monthlyPsalmody() bool {
	if b.Date.IsSunday() || b.properFeastPsalmody() {
		return false
	}
	normalize := func(ref string) string {
		return dwdoSpaceRe.Gsub(dwdoPsalmPrefixRe.Sub(ref, ""), "")
	}
	appointed := ""
	if p := b.Readings.Psalm; p != nil {
		appointed = p.Reference
	}
	return normalize(appointed) == normalize(reading.DwdoPsalterReference(b.Date, "morning_prayer"))
}

func (b *dwdo) properFeastPsalmody() bool {
	c, ok := b.DayInfo.Get("celebration").(*rb.Map)
	if !ok {
		return false
	}
	switch rb.ToS(c.Get("type")) {
	case "principal_feast", "major_holy_day", "festival":
	default:
		return false
	}
	t := b.Date.Time()
	candidates := []any{strings.ToLower(fmt.Sprintf("%s_%d", t.Month().String(), t.Day()))}
	if r := c.Get("calculation_rule"); r != nil {
		candidates = append(candidates, rb.ToS(r))
	}
	if n := c.Get("name"); n != nil {
		candidates = append(candidates, rb.ParameterizeSep(rb.ToS(n), "_"))
	}
	svc := reading.For(b.Ctx, b.Date, reading.Options{
		PrayerBookCode: b.PrefS("prayer_book_code"), Calendar: b.Calendar(), Translation: "nvi",
		ServiceType: "morning_prayer", PsalmTranslation: b.PrefString("psalm_translation"),
	})
	var one int
	err := db.Q().QueryRow(b.Ctx, `SELECT 1 AS one FROM "lectionary_readings"
WHERE "lectionary_readings"."prayer_book_id" = $1 AND "lectionary_readings"."date_reference" = ANY($2)
AND "lectionary_readings"."cycle" IN ($3, 'all') AND "lectionary_readings"."service_type" = 'morning_prayer'
AND NOT (("lectionary_readings"."psalm" = '' OR "lectionary_readings"."psalm" IS NULL)) LIMIT 1`,
		b.PrayerBook().ID, candidates, svc.Cycle).Scan(&one)
	if db.NoRows(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}

var dwdoFirstCanticleChoices = []string{"automatic", "morning_te_deum", "morning_benedicite", "morning_canticle_three_children",
	"morning_canticle_isaiah", "morning_canticle_hezekiah", "morning_canticle_hannah",
	"morning_canticle_moses", "morning_canticle_habakkuk"}

func (b *dwdo) mFirstCanticle() *Section {
	selected, _ := b.ResolveOptions("morning_first_canticle", dwdoFirstCanticleChoices).(string)
	known := false
	for _, c := range dwdoFirstCanticleChoices {
		if c == selected {
			known = true
		}
	}
	if !known {
		selected = "automatic"
	}
	slug := selected
	if selected == "automatic" {
		slug = "morning_benedicite"
		if b.teDeumAppointed() {
			slug = "morning_te_deum"
		}
	}
	c := b.T(slug)
	if c == nil {
		return nil
	}
	return b.Section(Title(c), "first_canticle", b.canticleLines(c, slug != "morning_te_deum"), nil)
}

var dwdoAthanasianFeasts = map[string]bool{
	"The Nativity of the Lord (Christmas)": true, "The Epiphany of the Lord": true, "Easter Day": true,
	"The Ascension of the Lord": true, "Pentecost (Whitsunday)": true, "Trinity Sunday": true,
	"St Matthias, Apostle": true, "The Nativity of St John the Baptist": true, "St James, Apostle": true,
	"St Bartholomew, Apostle": true, "St Matthew, Apostle and Evangelist": true,
	"Sts Simon and Jude, Apostles": true, "St Andrew, Apostle": true,
}

func (b *dwdo) mCreed() *Section {
	name, _ := b.celebrationField("name").(string)
	athanasian := dwdoAthanasianFeasts[name]
	var lines []*Line
	if !athanasian {
		lines = b.plain(lines, "morning_creed_rubric", "rubric")
	}
	slug := "apostles_creed"
	if athanasian {
		slug = "athanasian_creed"
	}
	if c := b.T(slug); c != nil {
		if athanasian {
			lines = append(lines, b.I(Title(c), "heading"))
		}
		lines = append(lines, b.I(c.Content, "text"))
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section("", "creed", lines, nil)
}

// --- Evensong ----------------------------------------------------------------------------

func (b *dwdo) evening() []*Section {
	return Pipeline(
		One(func() *Section { return b.introduction("evening") }),
		One(func() *Section { return b.openingVersicles("evening") }),
		One(b.appointedPsalms),
		One(func() *Section { return b.readingModule("first", "evening_first_reading_rubric", "The First Lesson") }),
		One(func() *Section { return b.officeHymn(false) }),
		One(func() *Section {
			c := b.T("evening_magnificat")
			if c == nil {
				return nil
			}
			return b.Section(Title(c), "first_canticle", b.antiphonedCanticleLines(c, "magnificat"), nil)
		}),
		One(func() *Section {
			return b.readingModule("second", "evening_second_reading_rubric", "The Second Lesson")
		}),
		One(func() *Section {
			c := b.T("evening_nunc_dimittis")
			if c == nil {
				return nil
			}
			return b.Section(Title(c), "second_canticle", b.canticleLines(c, true), nil)
		}),
		One(func() *Section {
			lines := b.plain(nil, "evening_creed_rubric", "rubric")
			lines = b.plain(lines, "apostles_creed", "text")
			if len(lines) == 0 {
				return nil
			}
			return b.Section("", "creed", lines, nil)
		}),
		One(func() *Section { return b.preces("evening") }),
		One(func() *Section { return b.collectOfTheDay("evening") }),
		One(func() *Section { return b.fixedCollects("evening_collect_peace", "evening_collect_aid_against_perils") }),
		One(b.conclusion),
	)
}

// --- Sext --------------------------------------------------------------------------------

func (b *dwdo) midday() []*Section {
	wd := weekdayName(b.Date)
	steps := []Step{
		One(func() *Section {
			lines := b.plain(nil, "midday_opening_rubric", "rubric")
			lines = append(lines,
				b.I(b.contentOrNil("midday_opening_v1"), "leader"),
				b.I(b.contentOrNil("midday_opening_r1"), "congregation"))
			if !b.passiontide() {
				lines = append(lines, b.I(b.contentOrNil("midday_gloria_patri"), "congregation"))
			}
			if !(b.preLent() || b.lent()) {
				lines = append(lines, b.I(b.contentOrNil("midday_alleluia"), "congregation"))
			}
			return b.Section("", "opening_versicles", lines, nil)
		}),
	}
	if !b.triduum() {
		steps = append(steps, One(func() *Section {
			h := b.T("midday_office_hymn")
			if h == nil {
				return nil
			}
			return b.Section("", "office_hymn", []*Line{b.I(Title(h), "heading"), b.I(h.Content, "text")}, nil)
		}))
	}
	steps = append(steps,
		One(func() *Section { return b.fixedPsalms([]string{"Psalm 123", "Psalm 124", "Psalm 125"}) }),
		One(func() *Section { return b.shortChapter(wd) }),
		One(func() *Section {
			return b.Section("", "versicle", []*Line{
				b.I(b.contentOrNil("midday_versicle_"+wd+"_v"), "leader"),
				b.I(b.contentOrNil("midday_versicle_"+wd+"_r"), "congregation"),
			}, nil)
		}),
		One(func() *Section {
			form := "priest"
			if b.layOfficiant() {
				form = "lay"
			}
			return b.Section("", "salutation", []*Line{
				b.I(b.contentOrNil("lesser_litany_"+form+"_v"), "leader"),
				b.I(b.contentOrNil("lesser_litany_"+form+"_r"), "congregation"),
				b.I(b.contentOrNil("midday_let_us_pray"), "leader"),
			}, nil)
		}),
		One(func() *Section {
			lines := b.plain(nil, "midday_collects_rubric", "rubric")
			choice := Or(b.ResolveOptions("midday_collect", []string{"collect_of_day", "midday_collect_cross", "midday_collect_saviour"}), "collect_of_day")
			if choice == "collect_of_day" {
				lines = append(lines, b.CollectLines(b.Collects)...)
			} else if f := b.tAny(choice); f != nil {
				lines = append(lines, b.I(f.Content, "text"))
			}
			if len(lines) == 0 {
				return nil
			}
			return b.Section("Collect", "collect_of_the_day", lines, nil)
		}),
		One(func() *Section {
			return b.Section("", "conclusion", []*Line{
				b.I(b.contentOrNil("conclusion_v"), "leader"),
				b.I(b.contentOrNil("conclusion_r"), "congregation"),
				b.FetchLineItem("midday_faithful_departed_rubric", "rubric", nil, "content"),
				b.FetchLineItem("midday_faithful_departed", "leader", nil, "content"),
			}, nil)
		}),
	)
	return Pipeline(steps...)
}

func (b *dwdo) shortChapter(wd string) *Section {
	ch := b.T("midday_short_chapter_" + wd)
	if ch == nil {
		return nil
	}
	var lines []*Line
	scripture, err := bible.NewTextService(b.translation(), bible.NewSegmentCache()).FetchPassageStructured(b.Ctx, Ref(ch))
	if err != nil {
		panic(err)
	}
	if scripture != nil {
		lines = append(lines, b.BibleContent(scripture)...)
	} else {
		lines = append(lines, b.I(ch.Content, "text"))
	}
	if ch.Reference != nil {
		lines = append(lines, b.I(*ch.Reference, "citation"))
	}
	return b.Section("", "short_chapter", lines, nil)
}

// --- Compline ----------------------------------------------------------------------------

func (b *dwdo) compline() []*Section {
	return Pipeline(
		One(b.cOpening),
		One(func() *Section { return b.fixedPsalms([]string{"Psalm 4", "Psalm 31:1-6", "Psalm 91", "Psalm 134"}) }),
		One(b.cShortChapter),
		One(func() *Section {
			h := b.T("compline_office_hymn")
			if h == nil {
				return nil
			}
			return b.Section("", "office_hymn", []*Line{
				b.I(Title(h), "heading"), b.I(h.Content, "text"),
				b.I(b.contentOrNil("compline_c2_v"), "leader"),
				b.I(b.contentOrNil("compline_c2_r"), "congregation"),
			}, nil)
		}),
		One(func() *Section {
			c := b.T("apostles_creed")
			if c == nil {
				return nil
			}
			lines := []*Line{b.I(c.Content, "text")}
			lines = b.plain(lines, "compline_c4", "responsive")
			return b.Section(Title(c), "creed", lines, nil)
		}),
		One(func() *Section {
			lines := b.plain(nil, "compline_lords_prayer", "congregation")
			lines = b.plain(lines, "compline_c5", "responsive")
			lines = b.plain(lines, "compline_blessing", "leader")
			lines = append(lines, b.I(b.contentOrNil("compline_blessing_r"), "congregation"))
			return b.Section("", "prayers", lines, nil)
		}),
		One(b.cNuncDimittis),
		One(func() *Section {
			lines := b.plain(nil, "compline_confession_rubric", "rubric")
			lines = b.plain(lines, "compline_confession", "text")
			lines = b.plain(lines, "compline_absolution", "leader")
			for i := 1; i <= 4; i++ {
				lines = append(lines,
					b.I(b.contentOrNil(fmt.Sprintf("compline_suff_v%d", i)), "leader"),
					b.I(b.contentOrNil(fmt.Sprintf("compline_suff_r%d", i)), "congregation"))
			}
			return b.Section("", "confession", lines, nil)
		}),
		One(func() *Section {
			lines := b.plain(nil, "compline_collects_rubric", "rubric")
			for _, s := range []string{"compline_collect_visit", "compline_collect_jesus_christ", "compline_collect_look_down",
				"compline_collect_be_present", "compline_collect"} {
				lines = b.plain(lines, s, "text")
			}
			if len(lines) == 0 {
				return nil
			}
			return b.Section("Collect", "collect_of_the_day", lines, nil)
		}),
		One(func() *Section {
			lines := []*Line{
				b.I(b.contentOrNil("compline_dismissal_v1"), "leader"),
				b.I(b.contentOrNil("compline_dismissal_r1"), "congregation"),
				b.I(b.contentOrNil("compline_dismissal_sal_v"), "leader"),
				b.I(b.contentOrNil("compline_dismissal_sal_r"), "congregation"),
				b.I(b.contentOrNil("compline_dismissal_v2"), "leader"),
				b.I(b.contentOrNil("compline_dismissal_r2"), "congregation"),
			}
			lines = b.plain(lines, "compline_dismissal_faithful", "responsive")
			return b.Section("", "dismissal", lines, nil)
		}),
		One(b.cMarianAnthem),
	)
}

func (b *dwdo) cOpening() *Section {
	lines := b.plain(nil, "compline_opening_rubric", "rubric")
	lines = append(lines,
		b.I(b.contentOrNil("compline_c1_v1"), "leader"),
		b.I(b.contentOrNil("compline_c1_r1"), "congregation"))
	if r := b.T("compline_opening_reading"); r != nil {
		lines = append(lines, b.I(r.Content, "text"))
		if r.Reference != nil {
			lines = append(lines, b.I(*r.Reference, "citation"))
		}
	}
	lines = append(lines,
		b.I(b.contentOrNil("compline_c1_v2"), "leader"),
		b.I(b.contentOrNil("compline_c1_r2"), "congregation"),
		b.I(b.contentOrNil("compline_c1_v3"), "leader"),
		b.I(b.contentOrNil("compline_c1_r3"), "congregation"))
	if !b.triduum() {
		lines = append(lines, b.I(b.contentOrNil("compline_gloria_patri"), "congregation"))
	}
	lines = append(lines,
		b.I(b.contentOrNil("compline_c1_v4"), "leader"),
		b.I(b.contentOrNil("compline_c1_r4"), "congregation"))
	return b.Section("", "opening", lines, nil)
}

func (b *dwdo) cShortChapter() *Section {
	var lines []*Line
	if ch := b.T("compline_short_chapter"); ch != nil {
		lines = append(lines, b.I(ch.Content, "text"))
		if ch.Reference != nil {
			lines = append(lines, b.I(*ch.Reference, "citation"))
		}
	}
	var responsory *store.LiturgicalText
	if b.passiontide() {
		responsory = b.T("compline_c3_passiontide")
	}
	if responsory == nil {
		responsory = b.T("compline_c3")
	}
	if responsory != nil {
		lines = append(lines, b.I(responsory.Content, "responsive"))
	}
	if len(lines) == 0 {
		return nil
	}
	return b.Section("", "short_chapter", lines, nil)
}

func (b *dwdo) cNuncDimittis() *Section {
	if v, ok := b.ResolveOptions("compline_nunc_dimittis", []string{"include", "omit"}).(string); ok && v == "omit" {
		return nil
	}
	c := b.T("compline_nunc_dimittis")
	if c == nil {
		return nil
	}
	a := b.T("compline_nunc_dimittis_antiphon")
	var lines []*Line
	if a != nil {
		lines = append(lines, b.I(a.Content, "congregation"))
	}
	lines = append(lines, b.I(c.Content, "text"), b.gloriaPatriLine())
	if a != nil {
		lines = append(lines, b.I(a.Content, "congregation"))
	}
	return b.Section(Title(c), "nunc_dimittis", lines, nil)
}

func (b *dwdo) cMarianAnthem() *Section {
	suffix := ""
	if v, ok := b.ResolveOptions("marian_anthem_language", []string{"english", "latin"}).(string); ok && v == "latin" {
		suffix = "_latin"
	}
	slug := b.marianAnthemSlug()
	if slug == "" {
		return nil
	}
	a := b.T(slug + suffix)
	if a == nil {
		a = b.T(slug)
	}
	if a == nil {
		return nil
	}
	lines := b.plain(nil, "marian_anthem_rubric", "rubric")
	lines = append(lines, b.I(a.Content, "text"),
		b.I(b.contentOrNil("compline_final_versicle_v"), "leader"),
		b.I(b.contentOrNil("compline_final_versicle_r"), "congregation"))
	return b.Section(Title(a), "marian_anthem", lines, nil)
}

func (b *dwdo) marianAnthemSlug() string {
	d := b.Date
	switch {
	case d <= civil.MustNew(d.Year(), 2, 2) || d >= b.easter("first_sunday_of_advent").Add(-1):
		return "marian_anthem_advento"
	case d < b.easter("maundy_thursday"):
		return "marian_anthem_epifania"
	case d < b.easter("easter"):
		return ""
	case d < b.easter("trinity_sunday").Add(-1):
		return "marian_anthem_pascoa"
	}
	return "marian_anthem"
}
