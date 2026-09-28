// Package seed holds the reference data the Rails seeds (db/seeds.rb) build,
// as one canonical dataset: a JSON file per table, split per Prayer Book
// where the table belongs to one, with every foreign key written as the
// natural key of the row it points to. Export writes the dataset from a
// database; Load rebuilds the same rows, in the same order, in an empty one.
package seed

// ref is a foreign key column written as the referenced row's natural key.
type ref struct {
	column string // e.g. "celebration_id"
	field  string // the dataset field, e.g. "celebration"
	table  string
	keys   []string // natural key columns of the referenced table
	// scope is a key column shared with the row's own Prayer Book: when the
	// referenced row belongs to the same book, only the other keys are
	// written.
	scope string
}

// table is one seeded table.
type table struct {
	name string
	// keys is the table's natural key (what other tables reference).
	keys []string
	refs []ref
	// book is the column (or ref field) that places a row in a Prayer Book
	// directory; empty for global tables.
	book string
	// relative are date columns the seeds compute from Date.today: they are
	// written as "@today" or "@today-N" (from the row's created_at) and
	// resolved against the loading day.
	relative []string
	// enums are Rails enum columns, written by name (the integer is what the
	// column stores).
	enums map[string]map[string]int
}

// celebrationEnums are Celebration's enums (app/models/celebration.rb).
var celebrationEnums = map[string]map[string]int{
	"celebration_type": {"principal_feast": 1, "major_holy_day": 2, "festival": 3, "lesser_feast": 4, "commemoration": 5},
	"person_type":      {"event": 0, "singular": 1, "plural": 2},
	"gender":           {"neutral": 0, "masculine": 1, "feminine": 2, "mixed": 3},
}

var bookRef = ref{column: "prayer_book_id", field: "prayer_book", table: "prayer_books", keys: []string{"code"}}

// tables lists the seeded tables in load order (parents first).
var tables = []table{
	{name: "liturgical_colors", keys: []string{"name"}},
	{name: "liturgical_seasons", keys: []string{"name"}},
	{name: "bible_versions", keys: []string{"code"}},
	{name: "bible_texts"},
	{name: "prayer_books", keys: []string{"code"}},
	{name: "celebrations", keys: []string{"prayer_book_id", "name"}, refs: []ref{bookRef}, book: "prayer_book_id", enums: celebrationEnums},
	{name: "preference_categories", keys: []string{"prayer_book_id", "key"}, refs: []ref{bookRef}, book: "prayer_book_id"},
	{name: "preference_definitions", refs: []ref{{column: "preference_category_id", field: "category", table: "preference_categories",
		keys: []string{"prayer_book_id", "key"}, scope: "prayer_book_id"}}, book: "preference_category_id"},
	{name: "liturgical_texts", refs: []ref{bookRef}, book: "prayer_book_id"},
	{name: "collects", refs: []ref{bookRef,
		{column: "celebration_id", field: "celebration", table: "celebrations", keys: []string{"prayer_book_id", "name"}, scope: "prayer_book_id"},
		{column: "season_id", field: "season", table: "liturgical_seasons", keys: []string{"name"}}}, book: "prayer_book_id"},
	{name: "lectionary_readings", refs: []ref{bookRef,
		{column: "celebration_id", field: "celebration", table: "celebrations", keys: []string{"prayer_book_id", "name"}, scope: "prayer_book_id"}},
		book: "prayer_book_id"},
	{name: "psalms", refs: []ref{bookRef}, book: "prayer_book_id"},
	{name: "psalm_cycles", refs: []ref{bookRef}, book: "prayer_book_id"},
	{name: "users", keys: []string{"email"}},
	{name: "life_rules", keys: []string{"user_id"}, refs: []ref{
		{column: "user_id", field: "user", table: "users", keys: []string{"email"}},
		{column: "original_life_rule_id", field: "original_life_rule", table: "life_rules", keys: []string{"user_id"}}}},
	{name: "life_rule_steps", refs: []ref{{column: "life_rule_id", field: "life_rule", table: "life_rules", keys: []string{"user_id"}}}},
	{name: "journals", refs: []ref{{column: "user_id", field: "user", table: "users", keys: []string{"email"}}}, relative: []string{"date_reference"}},
	{name: "background_categories", keys: []string{"slug"}},
	{name: "background_tracks", keys: []string{"slug"}},
	{name: "background_track_categories", refs: []ref{
		{column: "track_id", field: "track", table: "background_tracks", keys: []string{"slug"}},
		{column: "category_id", field: "category", table: "background_categories", keys: []string{"slug"}}}},
	{name: "background_track_placements", refs: []ref{{column: "track_id", field: "track", table: "background_tracks", keys: []string{"slug"}}}},
	{name: "background_track_assets", refs: []ref{{column: "track_id", field: "track", table: "background_tracks", keys: []string{"slug"}}}},
	{name: "feature_flags", refs: []ref{{column: "user_id", field: "user", table: "users", keys: []string{"email"}}}},
}

// skipped columns are set by the loader (the sequence and the clock).
var skipped = map[string]bool{"id": true, "created_at": true, "updated_at": true}

func tableByName(name string) *table {
	for i := range tables {
		if tables[i].name == name {
			return &tables[i]
		}
	}
	return nil
}
