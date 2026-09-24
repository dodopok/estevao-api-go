package liturgical

import (
	"sync"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

type Date = civil.Date

// Paschalion values.
const (
	Gregorian = "gregorian"
	Orthodox  = "orthodox"
)

// Movable holds every movable date of a civil year, keyed by the names the
// Rails EasterCalculator#all_movable_dates uses. Rule tables refer to these
// names, so they are kept as strings.
type Movable map[string]Date

// Get returns the date for key and whether it exists.
func (m Movable) Get(key string) (Date, bool) {
	d, ok := m[key]
	return d, ok
}

// Easter computes the movable feasts for one year under a Paschalion.
type Easter struct {
	Year       int
	Paschalion string
	Dates      Movable
	EasterDate Date
}

var easterCache sync.Map // key: easterKey -> *Easter

type easterKey struct {
	year       int
	paschalion string
}

// EasterFor returns the (memoized) calculator for year and paschalion.
func EasterFor(year int, paschalion string) *Easter {
	if paschalion != Orthodox {
		paschalion = Gregorian
	}
	k := easterKey{year, paschalion}
	if v, ok := easterCache.Load(k); ok {
		return v.(*Easter)
	}
	e := newEaster(year, paschalion)
	v, _ := easterCache.LoadOrStore(k, e)
	return v.(*Easter)
}

func newEaster(year int, paschalion string) *Easter {
	e := &Easter{Year: year, Paschalion: paschalion}
	if paschalion == Orthodox {
		e.EasterDate = orthodoxEaster(year)
	} else {
		e.EasterDate = gregorianEaster(year)
	}
	easter := e.EasterDate
	ash := easter.Add(-46)
	pentecost := easter.Add(49)
	trinity := pentecost.Add(7)
	ascension := easter.Add(39)
	advent1 := firstSundayOfAdvent(year)
	epiphany := civil.MustNew(year, 1, 6)

	var baptism Date
	if epiphany.IsSunday() {
		baptism = epiphany.Add(7)
	} else {
		baptism = epiphany.Add(7 - epiphany.Weekday())
	}
	lastAfterEpiphany := ash
	if ash.Weekday() == 0 {
		lastAfterEpiphany = ash.Add(-7)
	} else {
		lastAfterEpiphany = ash.Add(-ash.Weekday())
	}
	oct31 := civil.MustNew(year, 10, 31)
	nov1 := civil.MustNew(year, 11, 1)
	firstThu := nov1.Add(mod(4-nov1.Weekday(), 7))

	e.Dates = Movable{
		"easter":                      easter,
		"ash_wednesday":               ash,
		"first_sunday_in_lent":        easter.Add(-42),
		"palm_sunday":                 easter.Add(-7),
		"holy_monday":                 easter.Add(-6),
		"holy_tuesday":                easter.Add(-5),
		"holy_wednesday":              easter.Add(-4),
		"maundy_thursday":             easter.Add(-3),
		"good_friday":                 easter.Add(-2),
		"holy_saturday":               easter.Add(-1),
		"second_sunday_of_easter":     easter.Add(7),
		"ascension":                   ascension,
		"ascension_eve":               ascension.Add(-1),
		"ascension_friday":            ascension.Add(1),
		"ascension_saturday":          ascension.Add(2),
		"sunday_after_ascension":      easter.Add(42),
		"pentecost":                   pentecost,
		"pentecost_eve":               pentecost.Add(-1),
		"week_of_pentecost_wednesday": pentecost.Add(3),
		"week_of_pentecost_thursday":  pentecost.Add(4),
		"week_of_pentecost_friday":    pentecost.Add(5),
		"week_of_pentecost_saturday":  pentecost.Add(6),
		"easter_monday":               easter.Add(1),
		"easter_tuesday":              easter.Add(2),
		"whitsun_monday":              pentecost.Add(1),
		"whitsun_tuesday":             pentecost.Add(2),
		"trinity_sunday":              trinity,
		"corpus_christi":              trinity.Add(4),
		"sacred_heart":                trinity.Add(12),
		"immaculate_heart":            trinity.Add(13),
		"septuagesima":                easter.Add(-63),
		"sexagesima":                  easter.Add(-56),
		"quinquagesima":               easter.Add(-49),
		"rogation_sunday":             easter.Add(35),
		"first_sunday_of_advent":      advent1,
		"christ_the_king":             advent1.Add(-7),
		"last_sunday_of_october":      oct31.Add(-oct31.Weekday()),
		"patronage_of_saint_joseph":   easter.Add(17),
		"baptism_of_the_lord":         baptism,
		"last_sunday_after_epiphany":  lastAfterEpiphany,
		"thanksgiving":                firstThu.Add(21),
		"thanksgiving_first_thursday": firstThu,
	}
	return e
}

// MovableKeysOrdered lists the keys in the order Rails builds the hash; used
// where the order is observable (for example when the dates are serialized).
var MovableKeysOrdered = []string{
	"easter", "ash_wednesday", "first_sunday_in_lent", "palm_sunday", "holy_monday", "holy_tuesday",
	"holy_wednesday", "maundy_thursday", "good_friday", "holy_saturday", "second_sunday_of_easter",
	"ascension", "ascension_eve", "ascension_friday", "ascension_saturday", "sunday_after_ascension",
	"pentecost", "pentecost_eve", "week_of_pentecost_wednesday", "week_of_pentecost_thursday",
	"week_of_pentecost_friday", "week_of_pentecost_saturday", "easter_monday", "easter_tuesday",
	"whitsun_monday", "whitsun_tuesday", "trinity_sunday", "corpus_christi", "sacred_heart",
	"immaculate_heart", "septuagesima", "sexagesima", "quinquagesima", "rogation_sunday",
	"first_sunday_of_advent", "christ_the_king", "last_sunday_of_october", "patronage_of_saint_joseph",
	"baptism_of_the_lord", "last_sunday_after_epiphany", "thanksgiving", "thanksgiving_first_thursday",
}

// RogationDays are the Monday-Wednesday before the Ascension.
func (e *Easter) RogationDays() []Date {
	a := e.Dates["ascension"]
	return []Date{a.Add(-3), a.Add(-2), a.Add(-1)}
}

func mod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// floorDiv is Ruby's Integer#/ (floors toward negative infinity).
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func gregorianEaster(year int) Date {
	a := mod(year, 19)
	b := floorDiv(year, 100)
	c := mod(year, 100)
	d := floorDiv(b, 4)
	e := mod(b, 4)
	f := floorDiv(b+8, 25)
	g := floorDiv(b-f+1, 3)
	h := mod(19*a+b-d-g+15, 30)
	i := floorDiv(c, 4)
	k := mod(c, 4)
	l := mod(32+2*e+2*i-h-k, 7)
	m := floorDiv(a+11*h+22*l, 451)
	month := floorDiv(h+l-7*m+114, 31)
	day := mod(h+l-7*m+114, 31) + 1
	return civil.MustNew(year, month, day)
}

func orthodoxEaster(year int) Date {
	a := mod(year, 4)
	b := mod(year, 7)
	c := mod(year, 19)
	d := mod(19*c+15, 30)
	e := mod(2*a+4*b-d+34, 7)
	month := floorDiv(d+e+114, 31)
	day := mod(d+e+114, 31) + 1
	offset := floorDiv(year, 100) - floorDiv(year, 400) - 2
	return civil.MustNew(year, month, day).Add(offset)
}

func firstSundayOfAdvent(year int) Date {
	christmas := civil.MustNew(year, 12, 25)
	days := christmas.Weekday()
	if days == 0 {
		days = 7
	}
	return christmas.Add(-days).Add(-21)
}
