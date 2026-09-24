package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// FixedCelebrationOn ports
// Celebration.fixed.for_prayer_book_id(id).for_date(m, d)[.order(:rank)].first.
// byRank selects ORDER BY rank (an explicit order); otherwise .first orders
// by the primary key.
func FixedCelebrationOn(ctx context.Context, prayerBookID *int64, month, day int, byRank bool) *liturgical.Celebration {
	order := "celebrations.id ASC"
	if byRank {
		order = "celebrations.rank ASC"
	}
	var pb any
	cond := "celebrations.prayer_book_id IS NULL"
	args := []any{month, day}
	if prayerBookID != nil {
		pb = *prayerBookID
		cond = "celebrations.prayer_book_id = $3"
		args = append(args, pb)
	}
	sql := `SELECT ` + celebrationColumns + ` FROM celebrations WHERE celebrations.movable = FALSE AND ` + cond +
		` AND celebrations.fixed_month = $1 AND celebrations.fixed_day = $2 ORDER BY ` + order + ` LIMIT 1`
	rows, err := db.Q().Query(ctx, sql, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			panic(err)
		}
		return nil
	}
	c, err := ScanCelebration(rows)
	if err != nil {
		panic(err)
	}
	return c
}

// CelebrationByID ports Celebration.find_cached(id, prayer_book_code:).
func CelebrationByID(ctx context.Context, pb *PrayerBook, id int64) *liturgical.Celebration {
	if pb == nil {
		return nil
	}
	bc, err := CelebrationsForBook(ctx, pb)
	if err != nil {
		panic(err)
	}
	var found *liturgical.Celebration
	for _, c := range bc.All {
		if c.ID == id {
			// index_by: a later duplicate would win; ids are unique.
			found = c
		}
	}
	return found
}

// PsalmCycle is a cached psalm_cycles row.
type PsalmCycle struct {
	ID           int64
	CycleType    string
	WeekNumber   *int
	DayOfWeek    int
	OfficeType   string
	PsalmNumbers []any
	PrayerBookID int64
}

type psalmCyclesEntry struct {
	version time.Time
	value   map[string]*PsalmCycle
}

var (
	psalmCyclesMu    sync.Mutex
	psalmCyclesCache = map[int64]psalmCyclesEntry{}
)

// PsalmCycleKey ports PsalmCycle.psalm_cycle_lookup_key (week 0 == nil).
func PsalmCycleKey(cycleType string, dayOfWeek int, officeType string, week int) string {
	w := ""
	if week != 0 {
		w = fmt.Sprint(week)
	}
	return fmt.Sprintf("%s/%d/%s/%s", cycleType, dayOfWeek, officeType, w)
}

// PsalmCyclesFor ports PsalmCycle.psalm_cycles_cache_for.
func PsalmCyclesFor(ctx context.Context, pb *PrayerBook) map[string]*PsalmCycle {
	if pb == nil {
		return map[string]*PsalmCycle{}
	}
	psalmCyclesMu.Lock()
	if e, ok := psalmCyclesCache[pb.ID]; ok && e.version.Equal(pb.UpdatedAt) {
		psalmCyclesMu.Unlock()
		return e.value
	}
	psalmCyclesMu.Unlock()
	rows, err := db.Q().Query(ctx, `SELECT id, cycle_type, week_number, day_of_week, office_type, psalm_numbers, prayer_book_id
FROM psalm_cycles WHERE psalm_cycles.prayer_book_id = $1`, pb.ID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := map[string]*PsalmCycle{}
	for rows.Next() {
		var c PsalmCycle
		var raw []byte
		if err := rows.Scan(&c.ID, &c.CycleType, &c.WeekNumber, &c.DayOfWeek, &c.OfficeType, &raw, &c.PrayerBookID); err != nil {
			panic(err)
		}
		c.PsalmNumbers = decodeJSONArray(raw)
		week := 0
		if c.WeekNumber != nil {
			week = *c.WeekNumber
		}
		out[PsalmCycleKey(c.CycleType, c.DayOfWeek, c.OfficeType, week)] = &c
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	psalmCyclesMu.Lock()
	psalmCyclesCache[pb.ID] = psalmCyclesEntry{pb.UpdatedAt, out}
	psalmCyclesMu.Unlock()
	return out
}

func decodeJSONArray(raw []byte) []any {
	v, err := rb.ParseJSON(raw)
	if err != nil {
		return nil
	}
	list, _ := v.([]any)
	return list
}
