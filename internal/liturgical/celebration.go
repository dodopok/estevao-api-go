package liturgical

import (
	"sort"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

// Celebration is a row of the celebrations table with enums decoded.
type Celebration struct {
	ID               int64
	Name             string
	LatinName        *string
	CelebrationType  string // principal_feast | major_holy_day | festival | lesser_feast | commemoration
	Rank             int
	LiturgicalColor  *string
	Description      *string
	DescriptionYear  *string
	FixedMonth       *int
	FixedDay         *int
	Movable          bool
	CanBeTransferred bool
	CalculationRule  *string
	PostSlug         string // "" when NULL; see PostSlugPtr
	PostSlugNull     bool
	PersonType       string // event | singular | plural
	Gender           string // neutral | masculine | feminine | mixed
	PrayerBookID     int64
}

// CelebrationTypes maps the integer enum to its name.
var CelebrationTypes = map[int]string{1: "principal_feast", 2: "major_holy_day", 3: "festival", 4: "lesser_feast", 5: "commemoration"}

// CelebrationTypeValues maps names back to the integer enum.
var CelebrationTypeValues = map[string]int{"principal_feast": 1, "major_holy_day": 2, "festival": 3, "lesser_feast": 4, "commemoration": 5}

// CelebrationTypeOrder is the enum declaration order.
var CelebrationTypeOrder = []string{"principal_feast", "major_holy_day", "festival", "lesser_feast", "commemoration"}

var PersonTypes = map[int]string{0: "event", 1: "singular", 2: "plural"}
var Genders = map[int]string{0: "neutral", 1: "masculine", 2: "feminine", 3: "mixed"}

func (c *Celebration) IsPrincipalFeast() bool { return c.CelebrationType == "principal_feast" }
func (c *Celebration) IsMajorHolyDay() bool   { return c.CelebrationType == "major_holy_day" }
func (c *Celebration) IsFestival() bool       { return c.CelebrationType == "festival" }
func (c *Celebration) SupersedesSunday() bool { return c.IsPrincipalFeast() || c.IsMajorHolyDay() }
func (c *Celebration) IsEvent() bool          { return c.PersonType == "event" }
func (c *Celebration) IsPlural() bool         { return c.PersonType == "plural" }

func (c *Celebration) PostSlugPtr() *string {
	if c.PostSlugNull {
		return nil
	}
	s := c.PostSlug
	return &s
}

func (c *Celebration) Rule() string {
	if c.CalculationRule == nil {
		return ""
	}
	return *c.CalculationRule
}

func (c *Celebration) Color() string {
	if c.LiturgicalColor == nil {
		return ""
	}
	return *c.LiturgicalColor
}

// HasFixedDate reports whether both fixed_month and fixed_day are present.
func (c *Celebration) HasFixedDate() bool { return c.FixedMonth != nil && c.FixedDay != nil }

// FixedDateIn returns the nominal date in year. ok=false when the day does
// not exist in that year (Ruby raises Date::Error there).
func (c *Celebration) FixedDateIn(year int) (Date, bool) {
	if !c.HasFixedDate() {
		return 0, false
	}
	return civil.New(year, *c.FixedMonth, *c.FixedDay)
}

// PossessivePronoun / ServantForm port the Portuguese grammar helpers.
func (c *Celebration) PossessivePronoun() string {
	if c.IsEvent() {
		return ""
	}
	switch {
	case c.PersonType == "singular" && c.Gender == "masculine":
		return "teu"
	case c.PersonType == "singular" && c.Gender == "feminine":
		return "tua"
	case c.PersonType == "plural" && (c.Gender == "masculine" || c.Gender == "mixed"):
		return "teus"
	case c.PersonType == "plural" && c.Gender == "feminine":
		return "tuas"
	}
	return "teu"
}

func (c *Celebration) ServantForm() string {
	if c.IsEvent() {
		return ""
	}
	switch {
	case c.PersonType == "singular" && c.Gender == "masculine":
		return "servo"
	case c.PersonType == "singular" && c.Gender == "feminine":
		return "serva"
	case c.PersonType == "plural" && (c.Gender == "masculine" || c.Gender == "mixed"):
		return "servos"
	case c.PersonType == "plural" && c.Gender == "feminine":
		return "servas"
	}
	return "servo"
}

// ServantPhrase returns "teu servo N." or "" for events.
func (c *Celebration) ServantPhrase(name string) string {
	if c.IsEvent() {
		return ""
	}
	return c.PossessivePronoun() + " " + c.ServantForm() + " " + name
}

func (c *Celebration) ServantFormEN() string {
	if c.IsEvent() {
		return ""
	}
	if c.IsPlural() {
		return "servants"
	}
	return "servant"
}

func (c *Celebration) ServantPhraseEN(name string) string {
	if c.IsEvent() {
		return ""
	}
	return "your " + c.ServantFormEN() + " " + name
}

// CelebrationAttrs is the occurrence hash DayResolver builds for a
// celebration observed on a date.
type CelebrationAttrs struct {
	ID              int64
	Name            string
	Type            string
	Rank            int
	Color           *string
	Description     *string
	DescriptionYear *string
	Transferred     bool
	PostSlug        *string
	PersonType      string
	Gender          string
	CalculationRule *string
}

func (a *CelebrationAttrs) ColorPresent() bool {
	return a != nil && a.Color != nil && !blank(*a.Color)
}

func (a *CelebrationAttrs) ColorString() string {
	if a == nil || a.Color == nil {
		return ""
	}
	return *a.Color
}

func blank(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\v', ' ', '　':
			continue
		}
		return false
	}
	return true
}

// Blank mirrors ActiveSupport's String#blank? for strings.
func Blank(s string) bool { return blank(s) }

// sortByRank mirrors Ruby's sort_by(&:rank). Ruby's sort is not stable, but
// for the small arrays seen here MRI's merge of equal keys preserves input
// order in practice; a stable sort is the closest faithful rendering.
func sortByRank(cs []*Celebration) []*Celebration {
	out := append([]*Celebration(nil), cs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}
