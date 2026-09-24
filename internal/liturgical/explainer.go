package liturgical

import (
	"regexp"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Port of Liturgical::ColorExplainer: the pastoral meaning code of a colour.

var explainerColorAliases = map[string]string{
	"branco": "white", "white": "white", "blanco": "white",
	"dourado": "gold", "gold": "gold", "dorado": "gold",
	"vermelho": "red", "red": "red", "rojo": "red",
	"roxo": "purple", "purple": "purple", "morado": "purple",
	"violeta": "violet", "violet": "violet",
	"rosa": "rose", "rose": "rose",
	"verde": "green", "green": "green",
	"preto": "black", "black": "black", "negro": "black",
	"azul": "blue", "azul_escuro": "blue", "blue": "blue",
	"pano_cru": "unbleached", "unbleached": "unbleached",
}

var (
	reMartyr      = regexp.MustCompile(`\bmartyr\w*\b|\bmartir\w*\b`)
	reApostle     = regexp.MustCompile(`\bapostol\w*\b|\bevangelist\w*\b`)
	rePalm        = regexp.MustCompile(`palm sunday|domingo de ramos`)
	reGoodFriday  = regexp.MustCompile(`good friday|sexta-feira (da )?paixao|sexta-feira santa`)
	reHolyWeek    = regexp.MustCompile(`holy (monday|tuesday|wednesday)|segunda-feira santa|terca-feira santa|quarta-feira santa`)
	rePentecost   = regexp.MustCompile(`pentecost`)
	reEasterVigil = regexp.MustCompile(`easter vigil|vigilia pascal|vigilia de pascua`)
	reEaster      = regexp.MustCompile(`easter|pascoa|pascua`)
	reChristmas   = regexp.MustCompile(`christmas|natal|natividade`)
	reAscension   = regexp.MustCompile(`ascension|ascensao`)
	reTrinity     = regexp.MustCompile(`trinity|santissima trindade`)
	reAllSaints   = regexp.MustCompile(`all saints|todos os santos|todos los santos`)
)

// ExplainColor returns the meaning code for the effective colour.
func ExplainColor(date Date, season, color, source string, c *CelebrationAttrs, m Movable) string {
	e := &colorExplainer{date: date, season: season, source: source, c: c, m: m}
	e.color = explainerColorAliases[strings.ToLower(strings.TrimSpace(color))]
	if e.color == "" {
		e.color = "unknown"
	}
	if source == "celebration" {
		if v := e.celebrationMeaning(); v != "" {
			return v
		}
		return e.generic()
	}
	if source == "season" {
		if v := e.specialSeasonal(); v != "" {
			return v
		}
	}
	return e.generic()
}

type colorExplainer struct {
	date          Date
	season, color string
	source        string
	c             *CelebrationAttrs
	m             Movable
}

func (e *colorExplainer) is(cs ...string) bool { return contains(cs, e.color) }
func (e *colorExplainer) white() bool          { return e.is("white", "gold") }

func (e *colorExplainer) dateIs(key string) bool {
	d, ok := e.m[key]
	return ok && e.date == d
}

func (e *colorExplainer) roseSunday(kind string) bool {
	if e.color != "rose" {
		return false
	}
	if kind == "gaudete" {
		d, ok := e.m["first_sunday_of_advent"]
		return ok && e.date == d.Add(14)
	}
	d, ok := e.m["first_sunday_in_lent"]
	return ok && e.date == d.Add(21)
}

func (e *colorExplainer) specialSeasonal() string {
	switch {
	case e.dateIs("pentecost") && e.color == "red":
		return "pentecost_red"
	case e.dateIs("palm_sunday") && e.color == "red":
		return "palm_sunday_red"
	case e.dateIs("good_friday") && e.color == "red":
		return "good_friday_red"
	case e.dateIs("good_friday") && e.color == "black":
		return "good_friday_black"
	case e.holyWeekRed():
		return "holy_week_red"
	case e.dateIs("maundy_thursday") && e.white():
		return "maundy_thursday_white"
	case e.dateIs("holy_saturday") && e.white():
		return "easter_vigil_white"
	case e.dateIs("holy_saturday") && e.color == "black":
		return "holy_saturday_black"
	case e.dateIs("easter") && e.white():
		return "easter_white"
	case e.dateIs("ascension") && e.white():
		return "ascension_white"
	case e.dateIs("trinity_sunday") && e.white():
		return "trinity_white"
	case e.roseSunday("gaudete"):
		return "gaudete_rose"
	case e.roseSunday("laetare"):
		return "laetare_rose"
	}
	switch e.season {
	case Advent:
		if e.color == "blue" {
			return "advent_blue"
		}
		if e.is("violet", "purple") {
			return "advent_violet"
		}
	case Christmas:
		if e.white() {
			return "christmas_white"
		}
	case Epiphany:
		if e.color == "green" {
			return "epiphany_green"
		}
	case Lent:
		if e.is("purple", "violet", "blue", "unbleached") {
			return "lent_purple"
		}
	case EasterSeason:
		if e.white() {
			return "easter_white"
		}
	case OrdinaryTime:
		if e.color == "green" {
			return "ordinary_green"
		}
	}
	return ""
}

func (e *colorExplainer) holyWeekRed() bool {
	if e.season != Lent || e.color != "red" {
		return false
	}
	p, ok1 := e.m["palm_sunday"]
	g, ok2 := e.m["good_friday"]
	return ok1 && ok2 && e.date > p && e.date < g
}

func (e *colorExplainer) text() string {
	if e.c == nil {
		return ""
	}
	parts := []string{e.c.Name}
	if e.c.Description != nil {
		parts = append(parts, *e.c.Description)
	}
	return strings.ToLower(rb.Transliterate(strings.Join(parts, " ")))
}

func (e *colorExplainer) special() string {
	rule := ""
	if e.c != nil && e.c.CalculationRule != nil {
		rule = *e.c.CalculationRule
	}
	switch rule {
	case "palm_sunday", "easter_minus_7_days":
		return "palm_sunday"
	case "good_friday", "easter_minus_2_days":
		return "good_friday"
	case "holy_monday", "easter_minus_6_days", "holy_tuesday", "easter_minus_5_days", "holy_wednesday", "easter_minus_4_days":
		return "holy_week"
	case "maundy_thursday", "easter_minus_3_days":
		return "maundy_thursday"
	case "pentecost", "easter_plus_49_days":
		return "pentecost"
	case "easter":
		return "easter"
	case "holy_saturday", "easter_minus_1_day", "easter_minus_1_days":
		return "easter_vigil"
	case "ascension", "easter_plus_39_days":
		return "ascension"
	case "trinity_sunday", "first_sunday_after_pentecost", "easter_plus_56_days":
		return "trinity"
	}
	t := e.text()
	switch {
	case rePalm.MatchString(t):
		return "palm_sunday"
	case reGoodFriday.MatchString(t):
		return "good_friday"
	case reHolyWeek.MatchString(t):
		return "holy_week"
	case rePentecost.MatchString(t):
		return "pentecost"
	case reEasterVigil.MatchString(t):
		return "easter_vigil"
	case reEaster.MatchString(t):
		return "easter"
	case reChristmas.MatchString(t):
		return "christmas"
	case reAscension.MatchString(t):
		return "ascension"
	case reTrinity.MatchString(t):
		return "trinity"
	case reAllSaints.MatchString(t):
		return "all_saints"
	}
	return ""
}

func (e *colorExplainer) celebrationMeaning() string {
	if s := e.special(); s != "" {
		return e.specialCelebrationMeaning(s)
	}
	t := e.text()
	if e.color == "red" && reMartyr.MatchString(t) {
		return "celebration_martyr_red"
	}
	if e.color == "red" && reApostle.MatchString(t) {
		return "celebration_apostle_red"
	}
	switch e.color {
	case "white", "gold":
		return "celebration_white"
	case "red":
		return "celebration_red"
	case "blue":
		return "celebration_blue"
	case "purple", "violet", "unbleached":
		return "celebration_penitential"
	case "rose":
		return "celebration_rose"
	case "green":
		return "celebration_green"
	case "black":
		return "celebration_black"
	}
	return ""
}

func (e *colorExplainer) specialCelebrationMeaning(s string) string {
	switch s {
	case "palm_sunday":
		if e.color == "red" {
			return "palm_sunday_red"
		}
	case "good_friday":
		if e.color == "red" {
			return "good_friday_red"
		}
		if e.color == "black" {
			return "good_friday_black"
		}
	case "holy_week":
		if e.color == "red" {
			return "holy_week_red"
		}
	case "maundy_thursday":
		if e.white() {
			return "maundy_thursday_white"
		}
	case "pentecost":
		if e.color == "red" {
			return "pentecost_red"
		}
	case "easter":
		if e.white() {
			return "easter_white"
		}
	case "easter_vigil":
		if e.white() {
			return "easter_vigil_white"
		}
		if e.color == "black" {
			return "holy_saturday_black"
		}
	case "ascension":
		if e.white() {
			return "ascension_white"
		}
	case "trinity":
		if e.white() {
			return "trinity_white"
		}
	case "christmas":
		if e.white() {
			return "christmas_white"
		}
	case "all_saints":
		if e.white() {
			return "all_saints_white"
		}
	}
	return ""
}

func (e *colorExplainer) generic() string {
	switch e.color {
	case "white", "gold":
		return "generic_white"
	case "red":
		return "generic_red"
	case "blue":
		return "generic_blue"
	case "purple", "violet", "unbleached":
		return "generic_penitential"
	case "rose":
		return "generic_rose"
	case "green":
		return "generic_green"
	case "black":
		return "generic_black"
	}
	return "generic_color"
}
