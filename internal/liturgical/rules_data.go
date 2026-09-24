package liturgical

// Port of the constant tables in Liturgical::PrayerBookRules.

// Colors mirrors ColorDeterminator::COLORS.
var Colors = map[string]string{
	"white": "branco", "red": "vermelho", "purple": "roxo", "violet": "violeta",
	"rose": "rosa", "green": "verde", "black": "preto", "blue": "azul",
}

var loc1928FixedDateReferences = map[MD][]string{
	{1, 1}:   {"circumcision_of_christ", "circumcision"},
	{1, 6}:   {"epiphany"},
	{1, 25}:  {"conversion_of_saint_paul", "conversion_of_paul"},
	{2, 2}:   {"purification", "presentation_of_the_lord", "presentation"},
	{2, 24}:  {"saint_matthias", "saint_matthias_the_apostle"},
	{3, 25}:  {"annunciation"},
	{4, 25}:  {"saint_mark", "saint_mark_the_evangelist"},
	{5, 1}:   {"saints_philip_and_james", "saints_philip_and_james_apostles"},
	{6, 11}:  {"saint_barnabas", "saint_barnabas_the_apostle"},
	{6, 24}:  {"nativity_of_saint_john_baptist", "nativity_of_john_baptist"},
	{6, 29}:  {"saint_peter", "saint_peter_the_apostle"},
	{7, 4}:   {"independence_day"},
	{7, 25}:  {"saint_james", "saint_james_the_apostle"},
	{8, 6}:   {"transfiguration"},
	{8, 24}:  {"saint_bartholomew", "saint_bartholomew_the_apostle"},
	{9, 21}:  {"saint_matthew", "saint_matthew_the_apostle"},
	{9, 29}:  {"saint_michael_and_all_angels"},
	{11, 1}:  {"all_saints"},
	{10, 18}: {"saint_luke", "saint_luke_the_evangelist"},
	{10, 28}: {"saints_simon_and_jude", "saints_simon_and_jude_apostles"},
	{11, 30}: {"saint_andrew", "saint_andrew_the_apostle"},
	{12, 21}: {"saint_thomas", "saint_thomas_the_apostle"},
	{12, 25}: {"christmas_day", "christmas"},
	{12, 26}: {"saint_stephen", "saint_stephen_the_martyr"},
	{12, 27}: {"saint_john", "saint_john_the_evangelist"},
	{12, 28}: {"holy_innocents"},
}

var loc1962FixedDateReferences = map[MD][]string{
	{1, 1}:   {"octave_day_of_christmas", "circumcision"},
	{1, 6}:   {"epiphany"},
	{1, 25}:  {"conversion_of_saint_paul", "conversion_of_paul"},
	{2, 2}:   {"purification", "presentation"},
	{2, 24}:  {"saint_matthias"},
	{3, 25}:  {"annunciation"},
	{4, 25}:  {"saint_mark"},
	{5, 1}:   {"saints_philip_and_james"},
	{6, 11}:  {"saint_barnabas"},
	{6, 24}:  {"nativity_of_saint_john_baptist"},
	{6, 29}:  {"saint_peter_and_paul", "saint_peter"},
	{7, 22}:  {"saint_mary_magdalene"},
	{7, 25}:  {"saint_james"},
	{8, 6}:   {"transfiguration"},
	{8, 24}:  {"saint_bartholomew"},
	{9, 21}:  {"saint_matthew"},
	{9, 29}:  {"saint_michael_and_all_angels"},
	{10, 18}: {"saint_luke"},
	{10, 28}: {"saints_simon_and_jude"},
	{11, 1}:  {"all_saints"},
	{11, 30}: {"saint_andrew"},
	{12, 21}: {"saint_thomas"},
	{12, 24}: {"christmas_eve"},
	{12, 25}: {"christmas_day", "christmas"},
	{12, 26}: {"saint_stephen"},
	{12, 27}: {"saint_john_evangelist"},
	{12, 28}: {"holy_innocents"},
}

var loc1962EmberDaySets = []EmberSet{
	{Anchor: "third_sunday_of_advent", AnchorType: "sunday", ReadingPrefix: "3rd_sunday_of_advent"},
	{Anchor: "first_sunday_in_lent", AnchorType: "sunday", ReadingPrefix: "1st_sunday_in_lent"},
	{Anchor: "pentecost", AnchorType: "sunday", ReadingPrefix: "pentecost_sunday"},
	{Anchor: "holy_cross_day", AnchorType: "fixed", Month: 9, Day: 14},
}

var loc1962VigilReadingReferences = map[MD][]string{
	{1, 24}:  {"conversion_of_saint_paul_eve"},
	{2, 1}:   {"purification_eve"},
	{2, 23}:  {"saint_matthias_eve"},
	{3, 24}:  {"annunciation_eve"},
	{4, 24}:  {"saint_mark_eve"},
	{4, 30}:  {"saints_philip_and_james_eve"},
	{6, 10}:  {"saint_barnabas_eve"},
	{6, 23}:  {"nativity_of_saint_john_baptist_eve"},
	{6, 28}:  {"saint_peter_and_paul_eve"},
	{7, 21}:  {"saint_mary_magdalene_eve"},
	{7, 24}:  {"saint_james_eve"},
	{8, 5}:   {"transfiguration_eve"},
	{8, 23}:  {"saint_bartholomew_eve"},
	{9, 20}:  {"saint_matthew_eve"},
	{9, 28}:  {"saint_michael_and_all_angels_eve"},
	{10, 17}: {"saint_luke_eve"},
	{10, 27}: {"saints_simon_and_jude_eve"},
	{10, 31}: {"all_saints_eve"},
	{11, 29}: {"saint_andrew_eve"},
	{12, 20}: {"saint_thomas_eve"},
}

func withoutKey(src map[MD][]string, key MD) map[MD][]string {
	out := make(map[MD][]string, len(src))
	for k, v := range src {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func merged(src map[MD][]string, extra map[MD][]string) map[MD][]string {
	out := make(map[MD][]string, len(src)+len(extra))
	for k, v := range src {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

var recFixedDateReferences = withoutKey(loc1928FixedDateReferences, MD{7, 4})

var ciw1984FixedDateReferences = merged(withoutKey(loc1928FixedDateReferences, MD{7, 4}), map[MD][]string{
	{1, 1}:   {"naming_of_jesus", "circumcision_of_christ", "circumcision"},
	{3, 1}:   {"saint_david"},
	{4, 23}:  {"saint_george"},
	{4, 25}:  {"saint_mark", "saint_mark_the_evangelist"},
	{5, 1}:   {"saints_philip_and_james", "saints_philip_and_james_apostles"},
	{7, 2}:   {"the_visitation_of_the_blessed_virgin_mary", "visitation_of_the_blessed_virgin_mary"},
	{10, 11}: {"saint_daniel"},
	{10, 18}: {"saint_luke", "saint_luke_the_evangelist"},
})

var awrv2025FixedDateReferences = map[MD][]string{
	{1, 1}: {"circumcision"}, {1, 2}: {"holy_name"}, {1, 3}: {"january_3"}, {1, 4}: {"january_4"},
	{1, 5}: {"january_5"}, {1, 6}: {"epiphany"}, {1, 25}: {"conversion_of_saint_paul"},
	{2, 2}: {"purification"}, {2, 22}: {"chair_of_saint_peter_at_antioch"}, {2, 24}: {"saint_matthias"},
	{3, 12}: {"saint_gregory_the_great"}, {3, 19}: {"saint_joseph"}, {3, 21}: {"saint_benedict"},
	{3, 25}: {"annunciation"}, {4, 7}: {"saint_tikhon_of_moscow"}, {4, 23}: {"saint_george"},
	{4, 25}: {"saint_mark"}, {5, 1}: {"saints_philip_and_james"}, {5, 3}: {"invention_of_the_holy_cross"},
	{6, 11}: {"saint_barnabas"}, {6, 24}: {"nativity_of_saint_john_baptist"}, {6, 29}: {"saint_peter"},
	{7, 1}: {"precious_blood"}, {7, 2}: {"visitation"}, {7, 22}: {"saint_mary_magdalene"},
	{7, 25}: {"saint_james"}, {7, 26}: {"saint_anne"}, {8, 6}: {"transfiguration"},
	{8, 10}: {"saint_lawrence"}, {8, 15}: {"assumption"}, {8, 16}: {"saint_joachim"},
	{8, 24}: {"saint_bartholomew"}, {9, 8}: {"nativity_of_the_blessed_virgin_mary"},
	{9, 15}: {"seven_sorrows"}, {9, 21}: {"saint_matthew"}, {9, 29}: {"saint_michael_and_all_angels"},
	{10, 11}: {"motherhood_of_the_blessed_virgin_mary"}, {10, 15}: {"our_lady_of_walsingham"},
	{10, 18}: {"saint_luke"}, {10, 28}: {"saints_simon_and_jude"}, {11, 1}: {"all_saints"},
	{11, 30}: {"saint_andrew"}, {12, 8}: {"conception_of_the_blessed_virgin_mary"},
	{12, 21}: {"saint_thomas"}, {12, 25}: {"christmas_day", "christmas"}, {12, 26}: {"saint_stephen"},
	{12, 27}: {"saint_john"}, {12, 28}: {"holy_innocents"}, {12, 29}: {"december_29"},
	{12, 30}: {"december_30"}, {12, 31}: {"december_31"},
}

var awrv2025VigilReferences = map[MD][]string{
	{1, 1}: {"holy_name_eve"}, {1, 5}: {"epiphany_eve"}, {1, 24}: {"conversion_of_saint_paul_eve"},
	{2, 1}: {"purification_eve"}, {2, 21}: {"chair_of_saint_peter_at_antioch_eve"},
	{2, 23}: {"saint_matthias_eve"}, {3, 11}: {"saint_gregory_the_great_eve"},
	{3, 18}: {"saint_joseph_eve"}, {3, 20}: {"saint_benedict_eve"}, {3, 23}: {"march_24_eve"},
	{3, 24}: {"annunciation_eve"}, {4, 6}: {"saint_tikhon_of_moscow_eve"}, {4, 22}: {"saint_george_eve"},
	{4, 24}: {"saint_mark_eve"}, {4, 30}: {"saints_philip_and_james_eve"},
	{5, 2}: {"invention_of_the_holy_cross_eve"}, {5, 5}: {"may_6_eve"}, {6, 10}: {"saint_barnabas_eve"},
	{6, 23}: {"nativity_of_saint_john_baptist_eve"}, {6, 28}: {"saint_peter_eve"},
	{6, 30}: {"precious_blood_eve"}, {7, 1}: {"visitation_eve"}, {7, 21}: {"saint_mary_magdalene_eve"},
	{7, 24}: {"saint_james_eve"}, {7, 25}: {"saint_anne_eve"}, {7, 31}: {"august_1_eve"},
	{8, 5}: {"transfiguration_eve"}, {8, 9}: {"saint_lawrence_eve"}, {8, 14}: {"assumption_eve"},
	{8, 15}: {"saint_joachim_eve"}, {8, 23}: {"saint_bartholomew_eve"}, {8, 28}: {"august_29_eve"},
	{9, 7}: {"nativity_of_the_blessed_virgin_mary_eve"}, {9, 13}: {"september_14_eve"},
	{9, 14}: {"seven_sorrows_eve"}, {9, 20}: {"saint_matthew_eve"},
	{9, 28}: {"saint_michael_and_all_angels_eve"}, {10, 1}: {"october_2_eve"}, {10, 6}: {"october_7_eve"},
	{10, 10}: {"motherhood_of_the_blessed_virgin_mary_eve"}, {10, 14}: {"our_lady_of_walsingham_eve"},
	{10, 17}: {"saint_luke_eve"}, {10, 23}: {"october_24_eve"}, {10, 27}: {"saints_simon_and_jude_eve"},
	{10, 31}: {"all_saints_eve"}, {11, 29}: {"saint_andrew_eve"},
	{12, 7}: {"conception_of_the_blessed_virgin_mary_eve"}, {12, 20}: {"saint_thomas_eve"},
	{12, 24}: {"christmas_day_eve"}, {12, 31}: {"circumcision_eve"},
}

var cwFixedDateReferences = map[MD][]string{
	{1, 1}: {"naming_of_jesus"}, {1, 6}: {"epiphany"},
	{1, 25}: {"conversion_of_paul"}, {2, 2}: {"presentation_of_christ"},
	{3, 19}: {"joseph_of_nazareth"}, {3, 25}: {"annunciation"},
	{4, 23}: {"george"}, {4, 25}: {"mark_evangelist"},
	{5, 1}: {"philip_and_james"}, {5, 14}: {"matthias"},
	{5, 31}: {"visitation_of_bvm"}, {6, 11}: {"barnabas"},
	{6, 24}: {"birth_of_john_baptist"}, {6, 29}: {"peter_and_paul"},
	{7, 3}: {"thomas_apostle"}, {7, 22}: {"mary_magdalene"},
	{7, 25}: {"james_apostle"}, {8, 6}: {"transfiguration"},
	{8, 15}: {"blessed_virgin_mary"}, {8, 24}: {"bartholomew"},
	{9, 14}: {"holy_cross_day"}, {9, 21}: {"matthew_apostle"},
	{9, 29}: {"michael_and_all_angels"}, {10, 18}: {"luke_evangelist"},
	{10, 28}: {"simon_and_jude"}, {11, 1}: {"all_saints"},
	{11, 30}: {"andrew"}, {12, 26}: {"stephen"},
	{12, 27}: {"john_apostle"}, {12, 28}: {"holy_innocents"},
}

var feastsOfOurLord = []string{
	"celebration-holy-name-circumcision", "celebration-presentation", "celebration-annunciation",
	"celebration-visitation", "celebration-nativity-of-john-the-baptist", "celebration-transfiguration",
	"celebration-holy-cross",
}

var bcp1662VigilDates = []MD{
	{12, 25}, {2, 2}, {3, 25},
	{2, 24}, {6, 24}, {6, 29}, {7, 25}, {8, 24},
	{9, 21}, {10, 28}, {11, 30}, {12, 21}, {11, 1},
}

var fastBCP1662 = []FastRule{
	{On: "listed_vigil", FixedDates: bcp1662VigilDates, MovableDates: []string{"easter", "ascension", "pentecost"},
		MoveMondayToSaturday: true, Kind: "fasting_or_abstinence", Status: "appointed", Reason: "vigil"},
	{On: "ember_day", Kind: "fasting_or_abstinence", Status: "appointed"},
	{On: "rogation_day", Kind: "fasting_or_abstinence", Status: "appointed"},
	{On: "lenten_weekday", Kind: "fasting_or_abstinence", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"christmas_day"}}, Kind: "fasting_or_abstinence", Status: "appointed"},
}

var fastBCP1962Canada = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "appointed"},
	{On: "good_friday", Kind: "fast", Status: "appointed"},
	{On: "ember_day", EmberDaySets: loc1962EmberDaySets, Kind: "special_devotion", Status: "appointed"},
	{On: "rogation_day", Kind: "special_devotion", Status: "appointed"},
	{On: "lenten_weekday", Kind: "abstinence", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"christmas_day", "epiphany"}}, Kind: "abstinence", Status: "appointed"},
}

var fastBCP1928 = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "appointed"},
	{On: "good_friday", Kind: "fast", Status: "appointed"},
	{On: "ember_day", Kind: "fasting_with_abstinence", Status: "appointed"},
	{On: "lenten_weekday", Kind: "fasting_with_abstinence", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"christmas_to_epiphany"}}, Kind: "fasting_with_abstinence", Status: "appointed"},
}

var fastAWRV2025 = []FastRule{
	{On: "good_friday", Kind: "fast_and_total_abstinence", Status: "appointed"},
	{On: "ash_wednesday", Kind: "fast_and_abstinence", Status: "appointed"},
	{On: "lenten_weekday", Kind: "fast_and_abstinence", Status: "appointed"},
	{On: "ember_day", Kind: "fast_and_abstinence", Status: "appointed"},
	{On: "wednesday", Except: &FastExcept{DateRanges: []string{"week_after_easter", "christmas_to_epiphany"}}, Kind: "abstinence", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"week_after_easter", "christmas_to_epiphany"}}, Kind: "abstinence", Status: "appointed"},
}

var fastTEC1979 = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "appointed"},
	{On: "good_friday", Kind: "fast", Status: "appointed"},
	{On: "lenten_weekday", Except: &FastExcept{CelebrationKeys: []string{"celebration-annunciation"}}, Kind: "special_devotion", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"twelve_days_of_christmas", "fifty_days_of_easter"}, CelebrationKeys: feastsOfOurLord},
		Kind: "special_devotion", Status: "appointed"},
}

var fastACNA2019 = []FastRule{
	{On: "ash_wednesday", Kind: "special_devotion_and_total_abstinence", Status: "traditional"},
	{On: "good_friday", Kind: "special_devotion_and_total_abstinence", Status: "traditional"},
	{On: "ember_day", Kind: "fast", Status: "optional"},
	{On: "rogation_day", Kind: "fast", Status: "optional"},
	{On: "lenten_weekday", Kind: "fast", Status: "encouraged"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"twelve_days_of_christmas", "fifty_days_of_easter"}}, Kind: "fast", Status: "encouraged"},
}

var fastIEAB2015 = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "traditional"},
	{On: "good_friday", Kind: "fast", Status: "traditional"},
	{On: "lenten_weekday", Kind: "discipline_and_conversion", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"fifty_days_of_easter"}, CelebrationTypesOutsideLent: []string{"principal_feast", "festival"}},
		Kind: "discipline_and_conversion", Status: "appointed"},
	{On: "principal_feast_vigil", CelebrationTypes: []string{"principal_feast"}, Kind: "discipline_and_conversion", Status: "appointed", Reason: "principal_feast_vigil"},
}

var fastCommonWorship = []FastRule{
	{On: "lenten_weekday", Kind: "discipline_and_self_denial", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"fifty_days_of_easter"}, CelebrationTypesOutsideLent: []string{"principal_feast", "festival"}},
		Kind: "discipline_and_self_denial", Status: "appointed"},
	{On: "principal_feast_vigil", CelebrationTypes: []string{"principal_feast"}, Kind: "discipline_and_self_denial", Status: "optional", Reason: "principal_feast_vigil"},
}

var fastCIW1984 = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "appointed"},
	{On: "good_friday", Kind: "fast", Status: "appointed"},
	{On: "ember_day", Kind: "discipline_and_self_denial", Status: "appointed"},
	{On: "rogation_day", Kind: "discipline_and_self_denial", Status: "appointed"},
	{On: "lenten_weekday", Kind: "discipline_and_self_denial", Status: "appointed"},
	{On: "friday", Except: &FastExcept{CelebrationTypes: []string{"principal_feast", "major_holy_day"}}, Kind: "discipline_and_self_denial", Status: "appointed"},
}

var fastLusitanian1991 = []FastRule{
	{On: "ash_wednesday", Kind: "fast", Status: "appointed"},
	{On: "good_friday", Kind: "fast", Status: "appointed"},
	{On: "lenten_weekday", Kind: "abstinence", Status: "appointed"},
	{On: "friday", Except: &FastExcept{DateRanges: []string{"week_after_christmas", "week_after_easter", "week_after_ascension"}}, Kind: "abstinence", Status: "appointed"},
}

var fastDWDO2021 = []FastRule{
	{On: "ash_wednesday", Kind: "fast_and_abstinence", Status: "obligatory"},
	{On: "good_friday", Kind: "fast_and_abstinence", Status: "obligatory"},
}

var fastREB2027 = []FastRule{
	{On: "ash_wednesday", Kind: "fast_and_abstinence", Status: "appointed"},
	{On: "good_friday", Kind: "fast_and_total_abstinence", Status: "appointed"},
}

var revised1945Substitutions = []Substitution{
	{DateReferences: []string{"conversion_of_saint_paul_eve"}, ServiceTypes: []string{"vigil"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "septuagesima"}},
	{DateReferences: []string{"purification"}, ServiceTypes: []string{"morning_prayer"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "septuagesima"}},
	{DateReferences: []string{"annunciation"}, ServiceTypes: []string{"morning_prayer"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "after", Anchor: "easter"}},
	{DateReferences: []string{"saint_barnabas"}, ServiceTypes: []string{"morning_prayer"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "between_inclusive", Anchor: "trinity_sunday", Offset: 7, Finish: 13}},
	{DateReferences: []string{"saint_barnabas"}, ServiceTypes: []string{"evening_prayer"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "trinity_sunday", Offset: 21}},
	{DateReferences: []string{"nativity_of_saint_john_baptist_eve"}, ServiceTypes: []string{"vigil"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "between_inclusive", Anchor: "trinity_sunday", Offset: 1, Finish: 6}},
	{DateReferences: []string{"saint_peter"}, ServiceTypes: []string{"morning_prayer"}, Alternative: true, Condition: &SubstitutionCondition{Operator: "before", Anchor: "trinity_sunday", Offset: 21}},
}

var recSubstitutions = []Substitution{
	{DateReferences: []string{"conversion_of_saint_paul_eve"}, ServiceTypes: []string{"vigil"}, SecondReading: "Acts 22:1-21", Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "septuagesima"}},
	{DateReferences: []string{"purification"}, ServiceTypes: []string{"morning_prayer"}, SecondReading: "Romans 8:14-21", Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "septuagesima"}},
	{DateReferences: []string{"annunciation"}, ServiceTypes: []string{"morning_prayer"}, SecondReading: "1 John 4:7-14", Condition: &SubstitutionCondition{Operator: "after", Anchor: "easter"}},
	{DateReferences: []string{"saint_barnabas"}, ServiceTypes: []string{"morning_prayer"}, SecondReading: "Luke 14:25-35", Condition: &SubstitutionCondition{Operator: "between_inclusive", Anchor: "trinity_sunday", Offset: 7, Finish: 13}},
	{DateReferences: []string{"saint_barnabas"}, ServiceTypes: []string{"evening_prayer"}, SecondReading: "Romans 10:1-15", Condition: &SubstitutionCondition{Operator: "on_or_after", Anchor: "trinity_sunday", Offset: 21}},
	{DateReferences: []string{"nativity_of_saint_john_baptist_eve"}, ServiceTypes: []string{"vigil"}, SecondReading: "Matthew 21:23-27", Condition: &SubstitutionCondition{Operator: "between_inclusive", Anchor: "trinity_sunday", Offset: 1, Finish: 6}},
	{DateReferences: []string{"saint_peter"}, ServiceTypes: []string{"morning_prayer"}, SecondReading: "Acts 3", Condition: &SubstitutionCondition{Operator: "before", Anchor: "trinity_sunday", Offset: 21}},
}

func md(m, d int) *MD { v := MD{m, d}; return &v }

var standardRepeatedCollectQueries = map[string]CollectQuery{
	"christmas_day": {FixedDate: md(12, 25)},
	"epiphany":      {FixedDate: md(1, 6)},
	"easter_day":    {CalculationRule: "easter"},
	"ascension_day": {CalculationRule: "ascension"},
	"whitsunday":    {CalculationRule: "pentecost"},
	"all_saints":    {FixedAllSaints: true},
}

var awrvMovableVigils = []MovableVigil{
	{"2nd_sunday_of_advent_eve", "first_sunday_of_advent", 6},
	{"3rd_sunday_of_advent_eve", "first_sunday_of_advent", 13},
	{"easter_day_eve", "easter", -1},
	{"patronage_of_saint_joseph_eve", "patronage_of_saint_joseph", -1},
	{"ascension_day_eve", "ascension", -1},
	{"whitsunday_eve", "pentecost", -1},
	{"corpus_christi_eve", "corpus_christi", -1},
	{"compassion_of_our_lord_eve", "sacred_heart", -1},
	{"christ_the_king_eve", "last_sunday_of_october", -1},
}

func aliases(pairs ...string) map[string][]string {
	out := map[string][]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = []string{pairs[i+1]}
	}
	return out
}

var lentOfToIn = aliases(
	"1st_sunday_of_lent", "1st_sunday_in_lent",
	"2nd_sunday_of_lent", "2nd_sunday_in_lent",
	"3rd_sunday_of_lent", "3rd_sunday_in_lent",
	"4th_sunday_of_lent", "4th_sunday_in_lent",
	"5th_sunday_of_lent", "5th_sunday_in_lent",
)

func historic() RuleConfig {
	return RuleConfig{TraditionalCalendar: bp(true), TrinityCalendar: bp(true)}
}

func common1928() RuleConfig {
	return RuleConfig{
		EpiphanyWeeksFromNextSunday: true,
		LectionaryVariants: map[string]func(*RuleConfig){
			"original_1928": func(c *RuleConfig) {
				c.EmberReadingReferences = false
				c.FixedHolyDaySubstitutions = []Substitution{}
			},
			"original_1928:optional_ember": func(c *RuleConfig) {
				c.EmberReadingReferences = true
				c.FixedHolyDaySubstitutions = []Substitution{}
				c.EmberReadingPrefixes = []string{"lent_ember", "", "autumn_ember", "winter_ember"}
			},
			"revised_1945": func(c *RuleConfig) {
				c.EmberReadingReferences = true
				c.FixedHolyDaySubstitutions = revised1945Substitutions
				c.FixedDateReadingReferences = merged(loc1928FixedDateReferences, map[MD][]string{{1, 5}: {"epiphany_eve"}})
			},
		},
		TraditionalDefaults:               true,
		TraditionalCalendar:               bp(true),
		TrinityCalendar:                   bp(true),
		TraditionalLectionary:             bp(true),
		OfficeOnly:                        true,
		ObservesChristTheKing:             bp(false),
		SundayBeforeAdventReference:       true,
		EmberDays:                         true,
		ReadingCycle:                      "all",
		SpecialWeekCollects:               true,
		RepeatedCollects:                  bp(true),
		MonthlyPsalter:                    true,
		FastObservances:                   fastBCP1928,
		VigilReadings:                     true,
		SuppressAscensionOn:               []MD{{5, 1}},
		FixedDateReadingReferences:        loc1928FixedDateReferences,
		RepeatedCollectCelebrationQueries: standardRepeatedCollectQueries,
	}
}

func commonCIW1984() RuleConfig {
	return RuleConfig{
		TraditionalCalendar:               bp(true),
		TrinityCalendar:                   bp(true),
		TraditionalLectionary:             bp(true),
		OfficeOnly:                        true,
		ObservesChristTheKing:             bp(false),
		SundayBeforeAdventReference:       true,
		AlternatingOfficeSeriesAnchorYear: 1984,
		SpecialWeekCollects:               true,
		RepeatedCollects:                  bp(true),
		FastObservances:                   fastCIW1984,
		FixedDateReadingReferences:        ciw1984FixedDateReferences,
		EpiphanyStart:                     "epiphany",
		ObservesBaptism:                   bp(false),
		AllSaintsDate:                     md(11, 1),
		AllSaintsTransfer:                 "none",
	}
}

func awrv2025() RuleConfig {
	return RuleConfig{
		Paschalion:              Orthodox,
		PsalterRepeatsThirtieth: true,
		BookSeasons:             true,
		BookSeasonColors: map[string]string{
			"advent":           Colors["blue"],
			"septuagesimatide": Colors["purple"],
		},
		LocalizedCelebrationColors:        true,
		EmberDayColor:                     Colors["purple"],
		TraditionalCalendar:               bp(true),
		TrinityCalendar:                   bp(true),
		TraditionalLectionary:             bp(true),
		OfficeOnly:                        true,
		MonthlyPsalter:                    true,
		VigilReadings:                     true,
		ReadingCycle:                      "all",
		EpiphanyStart:                     "epiphany",
		ObservesBaptism:                   bp(false),
		ObservesRogationSunday:            true,
		ObservesChristTheKing:             bp(false),
		SundayBeforeAdventReference:       true,
		AllSaintsDate:                     md(11, 1),
		AllSaintsTransfer:                 "none",
		AllSaintsOctave:                   true,
		EmberDays:                         true,
		SpecialWeekCollects:               true,
		RepeatedCollects:                  bp(true),
		ThanksgivingFirstThursday:         bp(false),
		FastObservances:                   fastAWRV2025,
		FixedDateReadingReferences:        awrv2025FixedDateReferences,
		VigilReadingReferences:            awrv2025VigilReferences,
		MovableVigilReadingReferences:     awrvMovableVigils,
		WeeklyReferenceAliases:            lentOfToIn,
		RepeatedCollectCelebrationQueries: standardRepeatedCollectQueries,
	}
}

var ruleSets = map[string]*RuleSet{}

// DefaultRules is the rule set of any book without its own entry.
var DefaultRules = newRuleSet("default", RuleConfig{})

func init() {
	h := historic()
	h.ObservesChristTheKing = bp(false)
	h.DailyOfficeCourse = true
	h.CivilDateReferenceStyle = "numeric"
	h.FastObservances = fastBCP1662
	ruleSets["loc_1662"] = newRuleSet("loc_1662", h)

	h = historic()
	h.ObservesChristTheKing = bp(false)
	h.VigilReadings = true
	h.OfficeOnly = true
	h.DailyOfficeCourse = true
	h.CivilDateReferenceStyle = "numeric"
	h.MonthlyPsalter = true
	h.ProperPsalms = "bcp_1662"
	h.FastObservances = fastBCP1662
	ruleSets["loc_1662_en"] = newRuleSet("loc_1662_en", h)

	h = historic()
	h.ObservesChristTheKing = bp(false)
	h.VigilReadings = true
	h.OfficeOnly = true
	h.DailyOfficeCourse = true
	h.CycleScheme = "biennial"
	h.TraditionalLectionary = bp(true)
	h.EmberDays = true
	h.EmberDaySets = loc1962EmberDaySets
	h.EmberDayColors = map[string]string{
		"third_sunday_of_advent": Colors["violet"],
		"first_sunday_in_lent":   Colors["violet"],
		"pentecost":              Colors["red"],
		"holy_cross_day":         Colors["violet"],
	}
	h.EmberReadingReferences = true
	h.SpecialWeekCollects = true
	h.RepeatedCollects = bp(true)
	h.RepeatedCollectCelebrationQueries = map[string]CollectQuery{
		"christmas_day": {FixedDate: md(12, 25)},
		"epiphany":      {FixedDate: md(1, 6)},
		"easter_day":    {CalculationRule: "easter"},
		"ascension_day": {CalculationRule: "ascension"},
		"whitsunday":    {CalculationRule: "pentecost"},
		"all_saints":    {FixedDate: md(11, 1)},
	}
	h.RepeatedCollectExclusions = map[string][]string{"palm_sunday": {"good_friday"}}
	h.SuppressSeasonalCollectFallbackDates = []string{"maundy_thursday", "good_friday", "holy_saturday"}
	h.MonthlyPsalter = true
	h.MonthlyPsalterParity = true
	h.PsalterRepeatsThirtieth = true
	h.ProperPsalms = "bcp_1962_canada"
	h.FastObservances = fastBCP1962Canada
	h.TransferCalendar = "bcp_1962_canada"
	h.DisplaceTransferredNominalDate = true
	h.SundayBeforeAdventReference = true
	h.AllSaintsOctave = true
	h.AllSaintsOctaveColor = Colors["white"]
	h.SuppressSeasonalCollectFallbackPeriods = []string{"all_saints_octave"}
	h.FixedDateReadingReferences = loc1962FixedDateReferences
	h.VigilReadingReferences = loc1962VigilReadingReferences
	ruleSets["loc_1962_en"] = newRuleSet("loc_1962_en", h)

	h = historic()
	h.ObservesChristTheKing = bp(false)
	h.DailyOfficeCourse = true
	h.CivilDateReferenceStyle = "historic_leap_numeric"
	h.FastObservances = []FastRule{}
	ruleSets["loc_1549"] = newRuleSet("loc_1549", h)

	h = historic()
	h.ObservesChristTheKing = bp(true)
	h.OfficeOnly = true
	h.CycleScheme = "biennial"
	h.FixedPsalter = "dwdo"
	h.FastObservances = fastDWDO2021
	ruleSets["dwdo_2021"] = newRuleSet("dwdo_2021", h)

	ruleSets["loc_2019"] = newRuleSet("loc_2019", RuleConfig{DailyOfficeCourse: true, MonthlyPsalter: true,
		CivilDateReferenceStyle: "portuguese_month", FastObservances: fastACNA2019})
	ruleSets["loc_2019_en"] = newRuleSet("loc_2019_en", RuleConfig{DailyOfficeCourse: true, MonthlyPsalter: true, FastObservances: fastACNA2019})
	ruleSets["loc_2019_es"] = newRuleSet("loc_2019_es", RuleConfig{DailyOfficeCourse: true, MonthlyPsalter: true, FastObservances: fastACNA2019})
	ruleSets["loc_2015"] = newRuleSet("loc_2015", RuleConfig{MonthlyPsalter: true, FastObservances: fastIEAB2015})
	ruleSets["locb_2008"] = newRuleSet("locb_2008", RuleConfig{FastObservances: fastTEC1979, CumulativePsalmLists: true})
	ruleSets["loc_1987"] = newRuleSet("loc_1987", RuleConfig{FastObservances: fastTEC1979, MonthlyPsalter: true, CumulativePsalmLists: true})
	ruleSets["loc_2021"] = newRuleSet("loc_2021", RuleConfig{FastObservances: fastTEC1979})
	ruleSets["loc_1979_en"] = newRuleSet("loc_1979_en", RuleConfig{
		MonthlyPsalter: true, CommonLesserFeastCollect: true, CollectLanguageFollowsRite: true,
		TraditionalCollectsInLiturgicalTexts: true, FastObservances: fastTEC1979,
	})
	ruleSets["loc_1979_es"] = newRuleSet("loc_1979_es", RuleConfig{MonthlyPsalter: true, CommonLesserFeastCollect: true, FastObservances: fastTEC1979})
	ruleSets["cw_2005_en"] = newRuleSet("cw_2005_en", RuleConfig{
		TrinityCalendar:              bp(true),
		DatedSundayReferences:        true,
		OfficeLessonsWhenUnappointed: true,
		WeeklyReferenceAliases: aliases(
			"1st_sunday_after_christmas", "1st_sunday_of_christmas",
			"2nd_sunday_after_christmas", "2nd_sunday_of_christmas",
			"1st_sunday_in_lent", "1st_sunday_of_lent",
			"2nd_sunday_in_lent", "2nd_sunday_of_lent",
			"3rd_sunday_in_lent", "3rd_sunday_of_lent",
			"4th_sunday_in_lent", "4th_sunday_of_lent",
			"5th_sunday_in_lent", "5th_sunday_of_lent",
		),
		WeekdayTableLectionary:            true,
		WeekdayCycleScheme:                "paired_weekday_year",
		ExplanationProfile:                "common_worship",
		ExplanationCacheServiceDimensions: true,
		CommonCollectForCommemoration:     bp(false),
		FastObservances:                   fastCommonWorship,
		FixedDateReadingReferences:        cwFixedDateReferences,
	})
	ruleSets["loc_1991_pt"] = newRuleSet("loc_1991_pt", RuleConfig{OfficeCycleScheme: "biennial", DatedChristmastideCourse: true, FastObservances: fastLusitanian1991})
	ruleSets["loc_2027"] = newRuleSet("loc_2027", RuleConfig{CivilDateReferenceStyle: "numeric", FastObservances: fastREB2027})
	ruleSets["awrv_2025_en"] = newRuleSet("awrv_2025_en", awrv2025())
	ruleSets["awrv_2025_pt"] = newRuleSet("awrv_2025_pt", awrv2025())

	c := common1928()
	c.EpiphanyStart = "epiphany"
	c.ObservesBaptism = bp(false)
	c.ObservesRogationSunday = true
	c.AllSaintsDate = md(11, 1)
	c.AllSaintsTransfer = "none"
	c.AllSaintsOctave = true
	c.EpiphanyCollectOctave = true
	ruleSets["loc_1928_en"] = newRuleSet("loc_1928_en", c)

	ruleSets["loc_1984_en"] = newRuleSet("loc_1984_en", commonCIW1984())
	ruleSets["loc_1984_cy"] = newRuleSet("loc_1984_cy", commonCIW1984())

	c = common1928()
	c.EpiphanyStart = "epiphany"
	c.ObservesBaptism = bp(false)
	c.FixedDateReadingReferences = loc1928FixedDateReferences
	c.AllSaintsDate = md(11, 1)
	c.AllSaintsTransfer = "none"
	c.AllSaintsOctave = true
	c.ObservesRogationSunday = true
	c.EmberReadingReferences = true
	ruleSets["loc_1928_pt"] = newRuleSet("loc_1928_pt", c)

	ruleSets["rec_2005_en"] = newRuleSet("rec_2005_en", RuleConfig{
		TraditionalCalendar:         bp(true),
		TrinityCalendar:             bp(true),
		TraditionalLectionary:       bp(true),
		OfficeOnly:                  true,
		MonthlyPsalter:              true,
		VigilReadings:               true,
		ReadingCycle:                "all",
		EpiphanyStart:               "epiphany",
		ObservesBaptism:             bp(false),
		ObservesRogationSunday:      true,
		ObservesChristTheKing:       bp(false),
		SundayBeforeAdventReference: true,
		AllSaintsDate:               md(11, 1),
		AllSaintsTransfer:           "none",
		AllSaintsOctave:             true,
		EmberDays:                   true,
		FastObservances:             fastBCP1928,
		EmberReadingReferences:      true,
		EmberReadingPrefixes:        []string{"lent_ember", "pentecost_ember", "autumn_ember", "winter_ember"},
		SpecialWeekCollects:         true,
		RepeatedCollects:            bp(true),
		ThanksgivingFirstThursday:   bp(false),
		FixedDateReadingReferences:  recFixedDateReferences,
		FixedHolyDaySubstitutions:   recSubstitutions,
		WeeklyReferenceAliases: aliases(
			"1st_sunday_of_advent", "1st_sunday_in_advent",
			"2nd_sunday_of_advent", "2nd_sunday_in_advent",
			"3rd_sunday_of_advent", "3rd_sunday_in_advent",
			"4th_sunday_of_advent", "4th_sunday_in_advent",
			"1st_sunday_of_lent", "1st_sunday_in_lent",
			"2nd_sunday_of_lent", "2nd_sunday_in_lent",
			"3rd_sunday_of_lent", "3rd_sunday_in_lent",
			"4th_sunday_of_lent", "4th_sunday_in_lent",
			"5th_sunday_of_lent", "5th_sunday_in_lent",
		),
		EasterVigil: true,
		VigilReadingReferences: map[MD][]string{
			{1, 5}: {"epiphany_eve"}, {1, 24}: {"conversion_of_saint_paul_eve"}, {2, 1}: {"purification_eve"},
			{2, 23}: {"saint_matthias_eve"}, {3, 24}: {"annunciation_eve"}, {4, 24}: {"saint_mark_eve"},
			{4, 30}: {"saints_philip_and_james_eve"}, {6, 10}: {"saint_barnabas_eve"},
			{6, 23}: {"nativity_of_saint_john_baptist_eve"}, {6, 28}: {"saint_peter_eve"},
			{7, 24}: {"saint_james_eve"}, {8, 5}: {"transfiguration_eve"}, {8, 23}: {"saint_bartholomew_eve"},
			{9, 20}: {"saint_matthew_eve"}, {9, 28}: {"saint_michael_and_all_angels_eve"},
			{10, 17}: {"saint_luke_eve"}, {10, 27}: {"saints_simon_and_jude_eve"}, {10, 31}: {"all_saints_eve"},
			{11, 29}: {"saint_andrew_eve"}, {12, 20}: {"saint_thomas_eve"}, {12, 24}: {"christmas_eve"},
		},
		RepeatedCollectCelebrationQueries: standardRepeatedCollectQueries,
	})
}

// RulesFor returns the rule set of a prayer book (DefaultRules if none).
func RulesFor(code string) *RuleSet {
	if r, ok := ruleSets[code]; ok {
		return r
	}
	return DefaultRules
}

// RuleCodes lists the codes with their own rule set.
func RuleCodes() []string {
	out := make([]string, 0, len(ruleSets))
	for k := range ruleSets {
		out = append(out, k)
	}
	return out
}

// Loc1962FixedDateReferences exposes LOC_1962_FIXED_DATE_REFERENCES.fetch([m, d], []).
func Loc1962FixedDateReferences(month, day int) []string {
	return append([]string(nil), loc1962FixedDateReferences[MD{month, day}]...)
}
