package dailyoffice

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

func init() {
	Register("loc_2021", func(ctx context.Context, c *Context) Builder {
		switch c.OfficeType {
		case "morning", "evening", "midday", "compline":
		default:
			UnknownOfficeType(c.OfficeType)
		}
		b := &loc2021{}
		b.Init(ctx, c)
		return b
	})
}

// loc2021 ports DailyOffice::Builders::Loc2021::Office: the book prints one
// office, used for every hour it appoints.
type loc2021 struct{ Base }

func (b *loc2021) Call() *rb.Map {
	if b.PrefS("office_type") == "alternative" {
		return b.Render(b.alternative())
	}
	return b.Render(b.traditional())
}

// slugItem ports line_item(t&.content, type:, slug: t&.slug).
func (b *Base) slugItem(t *store.LiturgicalText, typ string) *Line {
	if t == nil {
		return b.I(nil, typ)
	}
	return b.Item(t.Content, typ, t.Slug, "")
}

// need ports calling #content on a text that may be nil.
func need(t *store.LiturgicalText, method string) *store.LiturgicalText {
	if t == nil {
		rb.RaiseNoMethodOnNil(method)
	}
	return t
}

func (b *loc2021) traditional() []*Section {
	return Pipeline(
		One(b.welcome),
		One(b.confession),
		One(func() *Section {
			return b.pair("absolution_m", "absolution_r", "DECLARAÇÃO DE PERDÃO", "absolution")
		}),
		One(func() *Section {
			return b.pair("thanksgiving_m", "thanksgiving_r", "ORAÇÃO DE AÇÃO DE GRAÇAS", "thanksgiving")
		}),
		Many(b.lessons),
		One(func() *Section { return b.Section("SERMÃO", "sermon", []*Line{}, nil) }),
		One(b.creed),
		One(b.lordsPrayer),
		One(func() *Section { return b.pair("blessing_m", "blessing_r", "A BÊNÇÃO", "blessing") }),
	)
}

func (b *loc2021) alternative() []*Section {
	mGrace, rGrace := b.T("grace_m"), b.T("grace_r")
	return Pipeline(
		One(b.altWelcome),
		One(b.altConfession),
		One(b.lordsPrayer),
		Many(b.lessons),
		One(b.creed),
		One(func() *Section {
			return b.Section("ORAÇÃO DO DIA", "collect_of_the_day", b.CollectLines(b.Collects), nil)
		}),
		One(func() *Section {
			rubric := need(b.T("other_prayers_rubric"), "content")
			return b.Section("ORAÇÃO PASTORAL", "other_prayers", []*Line{b.slugItem(rubric, "rubric")}, nil)
		}),
		One(func() *Section {
			return b.Section("A GRAÇA", "grace", []*Line{b.slugItem(mGrace, "leader"), b.slugItem(rGrace, "congregation")}, nil)
		}),
	)
}

func (b *loc2021) welcome() *Section {
	m := b.T("opening_welcome_m")
	if m == nil {
		return nil
	}
	return b.Section("ACOLHIDA", "welcome", []*Line{b.slugItem(m, "leader")}, nil)
}

func (b *loc2021) confession() *Section {
	var lines []*Line
	if m := b.T("confession_invitation_m1"); m != nil {
		lines = append(lines, b.slugItem(m, "leader"))
	}
	if m := b.T("confession_invitation_m2"); m != nil {
		lines = append(lines, b.slugItem(m, "leader"))
	}
	lines = append(lines, b.Spacer(), b.Spacer())
	if c := b.T("confession_body"); c != nil {
		lines = append(lines, b.slugItem(c, "congregation"))
	}
	return b.Section("CONVITE À CONFISSÃO", "confession", lines, nil)
}

// pair ports the leader/congregation sections (absolution, thanksgiving, blessing).
func (b *loc2021) pair(mSlug, rSlug, name, slug string) *Section {
	m, r := b.T(mSlug), b.T(rSlug)
	if m == nil {
		return nil
	}
	return b.Section(name, slug, []*Line{b.slugItem(m, "leader"), b.slugItem(r, "congregation")}, nil)
}

func (b *loc2021) lessons() []*Section {
	var modules []*Section
	for _, key := range []string{"first_reading", "second_reading", "gospel"} {
		r := b.Readings.Slot(key)
		if r == nil {
			continue
		}
		lines := []*Line{
			b.FetchLineItem("reading_announcement", "leader", [][2]string{{"reference", r.Reference}}, "content"),
			b.Spacer(),
		}
		if r.Content != nil {
			lines = append(lines, b.BibleContent(r.Content)...)
		}
		lines = append(lines, b.Spacer())
		if t := b.T("reading_introduction"); t != nil {
			lines = append(lines, b.slugItem(t, "rubric"))
		}
		if t := b.T("reading_v"); t != nil {
			lines = append(lines, b.slugItem(t, "leader"))
		}
		if t := b.T("reading_r"); t != nil {
			lines = append(lines, b.slugItem(t, "congregation"))
		}
		modules = append(modules, b.Section("LEITURA DAS ESCRITURAS", key, lines, rb.M("reference", r.Reference)))
	}
	return modules
}

// creed ports build_creed. Rails reads preferences["<office>_creed_type"]
// with a String key from a Symbol-keyed hash, which never matches, so the
// Apostles' Creed is always chosen.
func (b *loc2021) creed() *Section {
	c := b.T("credo_apostolico")
	if c == nil {
		return nil
	}
	return b.Section("O CREDO", "creed", []*Line{b.slugItem(c, "congregation")}, nil)
}

func (b *loc2021) lordsPrayer() *Section {
	lp := b.T("lords_prayer_contemporary")
	if lp == nil {
		return nil
	}
	return b.Section("A ORAÇÃO DO SENHOR", "lords_prayer", []*Line{b.slugItem(lp, "congregation")}, nil)
}

func (b *loc2021) altWelcome() *Section {
	m, intro := b.T("alt_office_welcome_m"), b.T("alt_office_intro")
	if m == nil {
		return nil
	}
	return b.Section("ACOLHIDA", "welcome", []*Line{b.slugItem(intro, "rubric"), b.slugItem(m, "leader")}, nil)
}

func (b *loc2021) altConfession() *Section {
	rubric, text := b.T("alt_confession_rubric"), b.T("alt_confession_body")
	m, r := b.T("alt_absolution_m"), b.T("alt_absolution_r")
	return b.Section("CONFISSÃO", "confession", []*Line{
		b.slugItem(need(text, "content"), "congregation"),
		b.slugItem(need(rubric, "content"), "rubric"),
		b.slugItem(need(m, "content"), "leader"),
		b.slugItem(r, "congregation"),
	}, nil)
}
