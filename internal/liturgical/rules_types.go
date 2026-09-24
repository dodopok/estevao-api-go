package liturgical

// MD is a (month, day) pair used by the rule tables.
type MD [2]int

// EmberSet declares one set of Ember Days.
type EmberSet struct {
	Anchor        string // movable key, "third_sunday_of_advent" or a label for fixed sets
	AnchorType    string // "sunday" | "fixed"
	Month, Day    int
	ReadingPrefix string // "" == nil
}

// FastExcept holds the exclusions of a fast rule.
type FastExcept struct {
	DateRanges                  []string
	CelebrationKeys             []string
	CelebrationTypes            []string
	CelebrationTypesOutsideLent []string
}

// FastRule declares one fasting/penitential observance of a book.
type FastRule struct {
	On                   string
	Kind                 string
	Status               string
	Reason               string // "" == use On
	Except               *FastExcept
	EmberDaySets         []EmberSet
	FixedDates           []MD
	MovableDates         []string
	MoveMondayToSaturday bool
	CelebrationTypes     []string // for principal_feast_vigil; nil == ["principal_feast"]
}

// SubstitutionCondition is the date guard of a fixed holy day substitution.
type SubstitutionCondition struct {
	Operator string
	Anchor   string
	Offset   int
	Finish   int
}

// Substitution replaces the second lesson of a fixed holy day.
type Substitution struct {
	DateReferences []string
	ServiceTypes   []string
	SecondReading  string // "" plus Alternative=true means :alternative
	Alternative    bool
	Condition      *SubstitutionCondition
}

// CollectQuery locates a celebration whose collect is repeated.
type CollectQuery struct {
	FixedDate       *MD
	FixedAllSaints  bool
	CalculationRule string
}

// MovableVigil is a vigil declared as an offset from a movable date.
type MovableVigil struct {
	Reference string
	Anchor    string
	Offset    int
}

// RuleConfig is the declarative configuration of one book's calendar rules.
// Pointer fields distinguish "not configured" from false where the Rails
// default depends on another value.
type RuleConfig struct {
	TraditionalDefaults                    bool
	Paschalion                             string
	BookSeasons                            bool
	BookSeasonColors                       map[string]string
	EmberDayColor                          string
	EmberDayColors                         map[string]string
	EmberDaySets                           []EmberSet
	LocalizedCelebrationColors             bool
	TraditionalCalendar                    *bool
	TrinityCalendar                        *bool
	TraditionalLectionary                  *bool
	OfficeOnly                             bool
	OfficeLessonsWhenUnappointed           bool
	MonthlyPsalter                         bool
	MonthlyPsalterParity                   bool
	PsalterRepeatsThirtieth                bool
	CumulativePsalmLists                   bool
	CommonLesserFeastCollect               bool
	FastObservances                        []FastRule
	CollectLanguageFollowsRite             bool
	TraditionalCollectsInLiturgicalTexts   bool
	DailyOfficeCourse                      bool
	VigilReadings                          bool
	ReadingCycle                           string
	CycleScheme                            string
	OfficeCycleScheme                      string
	WeekdayCycleScheme                     string
	WeekdayTableLectionary                 bool
	ExplanationProfile                     string
	ExplanationCacheServiceDimensions      bool
	CommonCollectForCommemoration          *bool
	FixedDateReadingReferences             map[MD][]string
	DatedChristmastideCourse               bool
	CivilDateReferenceStyle                string
	FixedPsalter                           string
	ProperPsalms                           string
	AlternatingOfficeSeriesAnchorYear      int
	ObservesBaptism                        *bool
	ObservesRogationSunday                 bool
	ObservesChristTheKing                  *bool
	SundayBeforeAdventReference            bool
	EpiphanyStart                          string
	EpiphanyWeeksFromNextSunday            bool
	AllSaintsDate                          *MD
	AllSaintsTransfer                      string
	AllSaintsOctave                        bool
	AllSaintsOctaveColor                   string
	TransferCalendar                       string
	SuppressAscensionOn                    []MD
	DisplaceTransferredNominalDate         bool
	FixedHolyDaySubstitutions              []Substitution
	WeeklyReferenceAliases                 map[string][]string
	DatedSundayReferences                  bool
	VigilReadingReferences                 map[MD][]string
	EasterVigil                            bool
	MovableVigilReadingReferences          []MovableVigil
	SpecialWeekCollects                    bool
	EmberDays                              bool
	RepeatedCollects                       *bool
	EpiphanyCollectOctave                  bool
	RepeatedCollectExclusions              map[string][]string
	SuppressSeasonalCollectFallbackDates   []string
	SuppressSeasonalCollectFallbackPeriods []string
	EmberReadingReferences                 bool
	EmberReadingPrefixes                   []string
	RepeatedCollectCelebrationQueries      map[string]CollectQuery
	ThanksgivingFirstThursday              *bool
	LectionaryVariants                     map[string]func(*RuleConfig)
}

func bp(b bool) *bool { return &b }
