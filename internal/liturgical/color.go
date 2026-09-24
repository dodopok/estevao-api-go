package liturgical

// ColorFor ports ColorDeterminator#color_for. celebration may be nil.
func ColorFor(sd *SeasonDeterminator, easter *Easter, rules *RuleSet, date Date, c *CelebrationAttrs) string {
	if principalCelebration(c) {
		return celebrationColor(rules, c)
	}
	if date.IsSunday() {
		return colorForSeason(sd, easter, rules, date)
	}
	if c.ColorPresent() {
		return celebrationColor(rules, c)
	}
	return colorForSeason(sd, easter, rules, date)
}

func celebrationColor(rules *RuleSet, c *CelebrationAttrs) string {
	color := c.ColorString()
	if !rules.LocalizedCelebrationColors() {
		return color
	}
	return TranslateColor(color, "pt-BR")
}

func principalCelebration(c *CelebrationAttrs) bool {
	if !c.ColorPresent() {
		return false
	}
	return c.Type == "principal_feast" || c.Type == "major_holy_day"
}

func colorForSeason(sd *SeasonDeterminator, easter *Easter, rules *RuleSet, date Date) string {
	m := easter.Dates
	switch sd.SeasonFor(date) {
	case Advent:
		if date == m["first_sunday_of_advent"].Add(14) {
			return Colors["rose"]
		}
		return seasonalBase(easter, rules, date, Colors["violet"])
	case Christmas, EasterSeason:
		return seasonalBase(easter, rules, date, Colors["white"])
	case Lent:
		if date >= m["palm_sunday"] && date <= m["good_friday"] {
			return Colors["red"]
		}
		if date == m["first_sunday_in_lent"].Add(21) {
			return Colors["rose"]
		}
		return seasonalBase(easter, rules, date, Colors["purple"])
	default:
		return seasonalBase(easter, rules, date, Colors["green"])
	}
}

func seasonalBase(easter *Easter, rules *RuleSet, date Date, standard string) string {
	if c := rules.OctaveColorFor(date); c != "" {
		return c
	}
	if rules.BookSeasons() {
		if c := rules.BookSeasonColor(NewBookSeasonResolver(easter.Dates).Resolve(date)); c != "" {
			return c
		}
	}
	if c := rules.EmberDayColorFor(date, easter.Dates); c != "" {
		if containsDate(rules.EmberDays(date.Year(), easter.Dates), date) {
			return c
		}
	}
	return standard
}
