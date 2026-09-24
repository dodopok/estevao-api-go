package liturgical

import (
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
)

type boundary struct{ season, anchor string }

var rubricalBoundaries = []boundary{
	{"epiphanytide", "septuagesima"}, {"septuagesimatide", "ash_wednesday"}, {"lent", "passion_sunday"},
	{"passiontide", "maundy_thursday"}, {"triduum", "easter"}, {"eastertide", "trinity_sunday"},
}

var textBoundaries = []boundary{
	{"epiphanytide", "septuagesima"}, {"septuagesimatide", "ash_wednesday"}, {"lent", "passion_sunday"},
	{"passiontide", "easter"}, {"eastertide", "ascension"}, {"ascensiontide", "pentecost"},
	{"whitsuntide", "trinity_sunday"},
}

const finalBookSeason = "trinitytide"

// BookSeasonsAll is what a reader may be told they are in.
var BookSeasonsAll = func() []string {
	out := []string{"advent", "christmastide"}
	for _, b := range rubricalBoundaries {
		out = append(out, b.season)
	}
	return append(out, finalBookSeason)
}()

// BookSeasonsAllPeriods includes the finer text periods.
var BookSeasonsAllPeriods = func() []string {
	out := append([]string{}, BookSeasonsAll...)
	for _, b := range textBoundaries {
		out = append(out, b.season)
	}
	return uniqStrings(out)
}()

// BookSeasonSlug returns "season-<name>".
func BookSeasonSlug(season string) string {
	return "season-" + strings.ReplaceAll(season, "_", "-")
}

// BookSeasonResolver ports Liturgical::BookSeasonResolver.
type BookSeasonResolver struct{ m Movable }

func NewBookSeasonResolver(m Movable) *BookSeasonResolver { return &BookSeasonResolver{m} }

func (b *BookSeasonResolver) Resolve(date Date) string    { return b.seasonFor(date, rubricalBoundaries) }
func (b *BookSeasonResolver) TextPeriod(date Date) string { return b.seasonFor(date, textBoundaries) }

func (b *BookSeasonResolver) seasonFor(date Date, bs []boundary) string {
	if b.advent(date) {
		return "advent"
	}
	if christmastide(date) {
		return "christmastide"
	}
	for _, x := range bs {
		if date < b.boundary(x.anchor) {
			return x.season
		}
	}
	return finalBookSeason
}

// RangeFor returns the inclusive span of the book season containing date.
func (b *BookSeasonResolver) RangeFor(date Date) SeasonRange {
	season := b.Resolve(date)
	type start struct {
		name string
		day  Date
	}
	var starts []start
	advent := b.m["first_sunday_of_advent"]
	if date >= advent {
		starts = []start{{"advent", advent}, {"christmastide", civil.MustNew(date.Year(), 12, 25)}, {"", civil.MustNew(date.Year()+1, 1, 6)}}
	} else {
		starts = []start{{"christmastide", civil.MustNew(date.Year()-1, 12, 25)}, {"epiphanytide", civil.MustNew(date.Year(), 1, 6)}}
		names := []string{}
		for _, x := range rubricalBoundaries {
			names = append(names, x.season)
		}
		names = append(names, finalBookSeason)
		for i, x := range rubricalBoundaries {
			starts = append(starts, start{names[i+1], b.boundary(x.anchor)})
		}
		starts = append(starts, start{"", advent})
	}
	for i, s := range starts {
		if s.name == season {
			return SeasonRange{Name: season, Start: s.day, End: starts[i+1].day.Add(-1)}
		}
	}
	return SeasonRange{Name: season}
}

func (b *BookSeasonResolver) advent(date Date) bool {
	return date >= b.m["first_sunday_of_advent"] && date < civil.MustNew(date.Year(), 12, 25)
}

func christmastide(date Date) bool {
	return (date.Month() == 12 && date.Day() >= 25) || (date.Month() == 1 && date.Day() <= 5)
}

func (b *BookSeasonResolver) boundary(anchor string) Date {
	switch anchor {
	case "passion_sunday":
		return b.m["easter"].Add(-14)
	case "maundy_thursday":
		return b.m["easter"].Add(-3)
	}
	return b.m[anchor]
}
