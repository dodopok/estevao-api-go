package liturgical

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/langs"
)

// Port of Liturgical::Translator.

var seasonsEN = map[string]string{Advent: "Advent", Christmas: "Christmas", Epiphany: "Epiphany", Lent: "Lent", EasterSeason: "Easter", OrdinaryTime: "Ordinary Time"}
var seasonsES = map[string]string{Advent: "Adviento", Christmas: "Navidad", Epiphany: "Epifanía", Lent: "Cuaresma", EasterSeason: "Pascua", OrdinaryTime: "Tiempo Ordinario"}
var seasonsCY = map[string]string{Advent: "Yr Adfent", Christmas: "Y Nadolig", Epiphany: "Yr Ystwyll", Lent: "Y Grawys", EasterSeason: "Y Pasg", OrdinaryTime: "Amser Cyffredin"}

var bookSeasonsEN = map[string]string{"advent": "Advent", "christmastide": "Christmastide", "epiphanytide": "Epiphanytide",
	"septuagesimatide": "Septuagesimatide", "lent": "Lent", "passiontide": "Passiontide", "triduum": "Sacred Triduum",
	"eastertide": "Eastertide", "ascensiontide": "Ascensiontide", "whitsuntide": "Whitsuntide", "trinitytide": "Trinitytide"}
var bookSeasonsPT = map[string]string{"advent": "Advento", "christmastide": "Tempo do Natal", "epiphanytide": "Tempo da Epifania",
	"septuagesimatide": "Septuagésima", "lent": "Quaresma", "passiontide": "Tempo da Paixão", "triduum": "Tríduo Sacro",
	"eastertide": "Tempo Pascal", "ascensiontide": "Tempo da Ascensão", "whitsuntide": "Tempo de Pentecostes", "trinitytide": "Tempo depois de Trindade"}
var bookSeasonsES = map[string]string{"advent": "Adviento", "christmastide": "Tiempo de Navidad", "epiphanytide": "Tiempo de Epifanía",
	"septuagesimatide": "Septuagésima", "lent": "Cuaresma", "passiontide": "Tiempo de Pasión", "triduum": "Triduo Sacro",
	"eastertide": "Tiempo Pascual", "ascensiontide": "Tiempo de la Ascensión", "whitsuntide": "Tiempo de Pentecostés", "trinitytide": "Tiempo después de Trinidad"}
var bookSeasons = map[string]map[string]string{"en": bookSeasonsEN, "es": bookSeasonsES, "pt-BR": bookSeasonsPT, "pt-PT": bookSeasonsPT}

var colorKeys = map[string]string{
	"branco": "white", "white": "white", "blanco": "white",
	"dourado": "gold", "gold": "gold", "dorado": "gold",
	"vermelho": "red", "red": "red", "rojo": "red",
	"roxo": "purple", "purple": "purple", "morado": "purple",
	"violeta": "violet", "violet": "violet",
	"rosa": "rose", "rose": "rose",
	"verde": "green", "green": "green",
	"preto": "black", "black": "black", "negro": "black",
	"azul": "blue", "blue": "blue",
}

var colorNames = map[string]map[string]string{
	"white":  {"pt": "branco", "en": "white", "es": "blanco", "cy": "gwyn"},
	"gold":   {"pt": "dourado", "en": "gold", "es": "dorado", "cy": "aur"},
	"red":    {"pt": "vermelho", "en": "red", "es": "rojo", "cy": "coch"},
	"purple": {"pt": "roxo", "en": "purple", "es": "morado", "cy": "porffor"},
	"violet": {"pt": "violeta", "en": "violet", "es": "violeta", "cy": "fioled"},
	"rose":   {"pt": "rosa", "en": "rose", "es": "rosa", "cy": "pinc"},
	"green":  {"pt": "verde", "en": "green", "es": "verde", "cy": "gwyrdd"},
	"black":  {"pt": "preto", "en": "black", "es": "negro", "cy": "du"},
	"blue":   {"pt": "azul", "en": "blue", "es": "azul", "cy": "glas"},
}

var whiteGoldColorValues = []string{"branco", "dourado", "white", "gold", "blanco", "dorado", "branco/dourado", "white/gold", "blanco/dorado"}

var DayNamesEN = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
var DayNamesPT = []string{"Domingo", "Segunda-feira", "Terça-feira", "Quarta-feira", "Quinta-feira", "Sexta-feira", "Sábado"}
var DayNamesES = []string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}
var DayNamesCY = []string{"Dydd Sul", "Dydd Llun", "Dydd Mawrth", "Dydd Mercher", "Dydd Iau", "Dydd Gwener", "Dydd Sadwrn"}

var sundayNamesCY = map[string]string{
	"Domingo da Páscoa": "Sul y Pasg", "Pentecostes": "Y Pentecost", "Santíssima Trindade": "Sul y Drindod",
	"Domingo de Ramos": "Sul y Blodau", "Domingo após a Ascensão": "Y Sul ar ôl y Dyrchafael",
	"1º Domingo do Advento": "Sul Cyntaf yr Adfent", "2º Domingo do Advento": "Ail Sul yr Adfent",
	"3º Domingo do Advento": "Trydydd Sul yr Adfent", "4º Domingo do Advento": "Pedwerydd Sul yr Adfent",
	"Septuagésima": "Septuagesima", "Sexagésima": "Sexagesima", "Quinquagésima": "Quinquagesima",
}

var sundayNamesEN = map[string]string{
	"Domingo da Páscoa": "Easter Sunday", "Pentecostes": "Pentecost", "Santíssima Trindade": "Trinity Sunday",
	"Domingo de Ramos": "Palm Sunday", "Domingo após a Ascensão": "Sunday after Ascension",
	"Cristo Rei do Universo": "Christ the King", "1º Domingo do Advento": "1st Sunday of Advent",
	"2º Domingo do Advento": "2nd Sunday of Advent", "3º Domingo do Advento": "3rd Sunday of Advent",
	"4º Domingo do Advento": "4th Sunday of Advent", "Batismo de nosso Senhor Jesus Cristo": "Baptism of our Lord",
	"1º Domingo após Natal": "1st Sunday after Christmas", "2º Domingo após Natal": "2nd Sunday after Christmas",
	"Septuagésima": "Septuagesima", "Sexagésima": "Sexagesima", "Quinquagésima": "Quinquagesima",
}

var sundayNamesES = map[string]string{
	"Domingo da Páscoa": "Domingo de Pascua", "Pentecostes": "Pentecostés", "Santíssima Trindade": "Domingo de la Santísima Trinidad",
	"Domingo de Ramos": "Domingo de Ramos", "Domingo após a Ascensão": "Domingo después de la Ascensión",
	"Cristo Rei do Universo": "Cristo Rey del Universo", "1º Domingo do Advento": "1.er Domingo de Adviento",
	"2º Domingo do Advento": "2.º Domingo de Adviento", "3º Domingo do Advento": "3.er Domingo de Adviento",
	"4º Domingo do Advento": "4.º Domingo de Adviento", "Batismo de nosso Senhor Jesus Cristo": "El Bautismo de Nuestro Señor",
	"1º Domingo após Natal": "1.er Domingo después de Navidad", "2º Domingo após Natal": "2.º Domingo después de Navidad",
}

var descriptionsEN = map[string]string{
	"Domingo de Ramos": "Palm Sunday", "Segunda-feira Santa": "Holy Monday", "Terça-feira Santa": "Holy Tuesday",
	"Quarta-feira Santa": "Holy Wednesday", "Quinta-feira Santa": "Maundy Thursday", "Sexta-feira da Paixão": "Good Friday",
	"Sábado Santo": "Holy Saturday", "Quarta-feira de Cinzas": "Ash Wednesday", "Domingo da Páscoa": "Easter Sunday",
	"Pentecostes": "Pentecost", "Santíssima Trindade": "Trinity Sunday", "Ascensão": "Ascension Day",
	"Cristo Rei do Universo": "Christ the King", "Batismo de nosso Senhor Jesus Cristo": "Baptism of our Lord",
	"Oitava do Natal": "Christmas Octave", "Semana após o Natal": "Week after Christmas",
	"Semana após a Quarta-feira de Cinzas": "Week after Ash Wednesday", "Semana após a Trindade": "Week after Trinity Sunday",
	"Oitava da Páscoa": "Easter Octave", "Oitava de Pentecostes": "Pentecost Octave",
	"Semana da Septuagésima": "Septuagesima Week", "Semana da Sexagésima": "Sexagesima Week", "Semana da Quinquagésima": "Quinquagesima Week",
}

var descriptionsES = map[string]string{
	"Domingo de Ramos": "Domingo de Ramos", "Segunda-feira Santa": "Lunes Santo", "Terça-feira Santa": "Martes Santo",
	"Quarta-feira Santa": "Miércoles Santo", "Quinta-feira Santa": "Jueves Santo", "Sexta-feira da Paixão": "Viernes Santo",
	"Sábado Santo": "Sábado Santo", "Quarta-feira de Cinzas": "Miércoles de Ceniza", "Domingo da Páscoa": "Domingo de Pascua",
	"Pentecostes": "Pentecostés", "Santíssima Trindade": "Domingo de la Santísima Trinidad", "Ascensão": "Ascensión del Señor",
	"Cristo Rei do Universo": "Cristo Rey del Universo", "Batismo de nosso Senhor Jesus Cristo": "El Bautismo de Nuestro Señor",
	"Oitava do Natal": "Octava de Navidad", "Semana após o Natal": "Semana después de Navidad",
	"Semana após a Quarta-feira de Cinzas": "Semana después del Miércoles de Ceniza", "Semana após a Trindade": "Semana después de la Trinidad",
	"Oitava da Páscoa": "Octava de Pascua", "Oitava de Pentecostes": "Octava de Pentecostés",
	"Semana da Septuagésima": "Semana de Septuagésima", "Semana da Sexagésima": "Semana de Sexagésima", "Semana da Quinquagésima": "Semana de Quincuagésima",
}

var descriptionsCY = map[string]string{
	"Domingo de Ramos": "Sul y Blodau", "Segunda-feira Santa": "Dydd Llun yr Wythnos Fawr", "Terça-feira Santa": "Dydd Mawrth yr Wythnos Fawr",
	"Quarta-feira Santa": "Dydd Mercher yr Wythnos Fawr", "Quinta-feira Santa": "Dydd Iau Cablyd", "Sexta-feira da Paixão": "Dydd Gwener y Groglith",
	"Sábado Santo": "Dydd Sadwrn Sanctaidd", "Quarta-feira de Cinzas": "Dydd Mercher y Lludw", "Domingo da Páscoa": "Sul y Pasg",
	"Pentecostes": "Y Pentecost", "Santíssima Trindade": "Sul y Drindod", "Ascensão": "Dydd Iau Dyrchafael",
	"Oitava do Natal": "Wythfed Dydd y Nadolig", "Oitava da Páscoa": "Wythfed Dydd y Pasg", "Oitava de Pentecostes": "Wythfed Dydd y Pentecost",
	"Semana da Septuagésima": "Wythnos Septuagesima", "Semana da Sexagésima": "Wythnos Sexagesima", "Semana da Quinquagésima": "Wythnos Quinquagesima",
}

var numberedDescriptions = []struct {
	re     *regexp.Regexp
	en, es string
}{
	{regexp.MustCompile(`^(\d+)ª Semana do Advento$`), "%s Week of Advent", "%s Semana de Adviento"},
	{regexp.MustCompile(`^(\d+)ª Semana após a Epifania$`), "%s Week after Epiphany", "%s Semana después de Epifanía"},
	{regexp.MustCompile(`^(\d+)ª Semana da Quaresma$`), "%s Week of Lent", "%s Semana de Cuaresma"},
	{regexp.MustCompile(`^(\d+)ª Semana da Páscoa$`), "%s Week of Easter", "%s Semana de Pascua"},
	{regexp.MustCompile(`^(\d+)ª Semana do Tempo Comum$`), "%s Week of Ordinary Time", "%s Semana del Tiempo Ordinario"},
	{regexp.MustCompile(`^(\d+)ª Semana após Pentecostes$`), "%s Week after Pentecost", "%s Semana después de Pentecostés"},
	{regexp.MustCompile(`^(\d+)ª Semana após a Trindade$`), "%s Week after Trinity", "%s Semana después de la Trinidad"},
}

var seasonArticles = map[string]string{"a Páscoa": "Easter", "a Trindade": "Trinity", "a Epifania": "Epiphany", "a Quaresma": "Lent", "o Advento": "Advent", "o Natal": "Christmas"}

type profile struct {
	seasons, sundayNames, descriptions map[string]string
	books                              map[int]string
	dayNames                           []string
	translated                         bool
}

var profilesTr = map[string]profile{
	"en":    {seasonsEN, sundayNamesEN, descriptionsEN, booksEN, DayNamesEN, true},
	"es":    {seasonsES, sundayNamesES, descriptionsES, booksES, DayNamesES, true},
	"cy":    {seasonsCY, sundayNamesCY, descriptionsCY, booksCY, DayNamesCY, true},
	"pt-BR": {dayNames: DayNamesPT},
	"pt-PT": {dayNames: DayNamesPT},
}

var properPattern = regexp.MustCompile(`^Próprio (\d+)$`)

func translationLanguage(language string) string {
	if l := langs.TranslatorLanguageFor(language); l != "" {
		return l
	}
	return language
}

func profileFor(language string) (profile, bool) {
	p, ok := profilesTr[translationLanguage(language)]
	return p, ok
}

// TranslatedLanguage reports whether the language has translation tables.
func TranslatedLanguage(language string) bool {
	p, ok := profileFor(language)
	return ok && p.translated
}

// TranslateSeason returns the season name in language (identity for pt).
func TranslateSeason(seasonPT, language string) string {
	if p, ok := profileFor(language); ok && p.seasons != nil {
		if v, ok := p.seasons[seasonPT]; ok {
			return v
		}
	}
	return seasonPT
}

// TranslateBookSeason names a book-specific season.
func TranslateBookSeason(key, language string) string {
	if Blank(key) {
		return ""
	}
	table, ok := bookSeasons[translationLanguage(language)]
	if !ok {
		table = bookSeasonsEN
	}
	if v, ok := table[key]; ok {
		return v
	}
	if v, ok := bookSeasonsEN[key]; ok {
		return v
	}
	return key
}

func colorKey(value string) (string, bool) {
	k, ok := colorKeys[strings.ToLower(rubyStrip(value))]
	return k, ok
}

// TranslateColor renders a stored colour in language; unknown values pass through.
func TranslateColor(value, language string) string {
	key, ok := colorKey(value)
	if !ok {
		return value
	}
	if v, ok := colorNames[key][langs.ColorLanguageFor(language)]; ok {
		return v
	}
	return value
}

// ColorCode returns the language-independent colour key or "".
func ColorCode(value string) string {
	k, _ := colorKey(value)
	return k
}

// ColorWithAlternative shows white/gold as interchangeable.
func ColorWithAlternative(value, language string) string {
	if Blank(value) {
		return value
	}
	normalized := strings.Join(strings.Fields(strings.ToLower(value)), "")
	if !contains(whiteGoldColorValues, normalized) {
		return TranslateColor(value, language)
	}
	switch langs.ColorLanguageFor(language) {
	case "es":
		return "blanco/dorado"
	case "cy":
		return "gwyn/aur"
	case "pt":
		return "branco/dourado"
	}
	return "white/gold"
}

var sundayNamePattern = regexp.MustCompile(`(\d+)º Domingo (do|no|da|na|após) (.+)`)

// SundayNameParts ports Liturgical::SundayName.parse.
type SundayNameParts struct {
	Number      int
	Preposition string
	Season      string
}

func ParseSundayName(name string) (SundayNameParts, bool) {
	m := sundayNamePattern.FindStringSubmatch(name)
	if m == nil {
		return SundayNameParts{}, false
	}
	n, _ := strconv.Atoi(m[1])
	return SundayNameParts{n, m[2], m[3]}, true
}

// TranslateSundayName translates a rendered Sunday name.
func TranslateSundayName(namePT *string, language string) *string {
	if namePT == nil {
		return nil
	}
	tl := translationLanguage(language)
	p, ok := profilesTr[tl]
	if !ok || !p.translated {
		return namePT
	}
	if v, ok := p.sundayNames[*namePT]; ok {
		return &v
	}
	parts, ok := ParseSundayName(*namePT)
	if !ok {
		return namePT
	}
	s := numberedSundayName(parts, tl)
	return &s
}

func numberedSundayName(p SundayNameParts, language string) string {
	switch language {
	case "cy":
		connector := "yn"
		if p.Preposition == "após" {
			connector = "ar ôl"
		}
		season := p.Season
		if v, ok := seasonsCY[season]; ok {
			season = v
		}
		return fmt.Sprintf("%dfed Sul %s %s", p.Number, connector, season)
	case "es":
		connector := "de"
		if p.Preposition == "após" {
			connector = "después de"
		}
		season := p.Season
		if v, ok := seasonsES[season]; ok {
			season = v
		}
		return fmt.Sprintf("%s Domingo %s %s", ordinalES(p.Number), connector, season)
	}
	connector := "of"
	switch p.Preposition {
	case "no", "na":
		connector = "in"
	case "após":
		connector = "after"
	}
	season := p.Season
	if p.Preposition == "após" {
		if v, ok := seasonArticles[season]; ok {
			season = v
		} else if v, ok := seasonsEN[season]; ok {
			season = v
		}
	} else if v, ok := seasonsEN[season]; ok {
		season = v
	}
	return fmt.Sprintf("%s Sunday %s %s", Ordinalize(p.Number), connector, season)
}

// DayName returns the weekday name in the book language (pt default).
func DayName(date Date, language string) string {
	if p, ok := profileFor(language); ok && p.dayNames != nil {
		return p.dayNames[date.Weekday()]
	}
	return DayNamesPT[date.Weekday()]
}

// TranslateBookName translates a Portuguese Bible book name.
func TranslateBookName(ptName, language string) string {
	p, ok := profileFor(language)
	if !ok || p.books == nil {
		return ptName
	}
	id, ok := bible.ExactBookID(ptName)
	if !ok {
		return ptName
	}
	if v, ok := p.books[id]; ok {
		return v
	}
	return ptName
}

// TranslateDescription translates one description line.
func TranslateDescription(text, language string) string {
	tl := translationLanguage(language)
	p, ok := profilesTr[tl]
	if !ok || !p.translated {
		return text
	}
	if strings.HasSuffix(text, " (movido)") {
		base := TranslateDescription(strings.TrimSuffix(text, " (movido)"), tl)
		suffix := "(transferred)"
		switch tl {
		case "es":
			suffix = "(trasladado)"
		case "cy":
			suffix = "(wedi’i drosglwyddo)"
		}
		return base + " " + suffix
	}
	if v, ok := p.descriptions[text]; ok {
		return v
	}
	for _, nd := range numberedDescriptions {
		m := nd.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		switch tl {
		case "cy":
			period := regexp.MustCompile(`^\d+ª Semana `).ReplaceAllString(text, "")
			key := regexp.MustCompile(`^(?:do|da) `).ReplaceAllString(period, "")
			if v, ok := seasonsCY[key]; ok {
				return fmt.Sprintf("%dfed Wythnos %s", n, v)
			}
			return fmt.Sprintf("%dfed Wythnos %s", n, period)
		case "es":
			return fmt.Sprintf(nd.es, fmt.Sprintf("%d.ª", n))
		}
		return fmt.Sprintf(nd.en, Ordinalize(n))
	}
	if m := properPattern.FindStringSubmatch(text); m != nil {
		switch tl {
		case "cy":
			return "Priod " + m[1]
		case "es":
			return "Propio " + m[1]
		}
		return "Proper " + m[1]
	}
	return text
}

// TranslateDescriptions maps TranslateDescription over a list.
func TranslateDescriptions(list []string, language string) []string {
	if !TranslatedLanguage(language) {
		return list
	}
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = TranslateDescription(s, language)
	}
	return out
}

func ordinalES(n int) string {
	switch n {
	case 1:
		return "1.er"
	case 2:
		return "2.º"
	case 3:
		return "3.er"
	}
	return fmt.Sprintf("%d.º", n)
}

func rubyStrip(s string) string { return strings.Trim(s, " \t\n\v\f\r\x00") }
