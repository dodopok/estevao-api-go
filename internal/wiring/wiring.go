// Package wiring installs the database-backed hooks that domain packages
// declare as function variables (so they stay free of SQL). The server and
// DB-backed tests share it.
package wiring

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/bible"
	"github.com/dodopok/estevao-api-go/internal/liturgical"
	_ "github.com/dodopok/estevao-api-go/internal/reading" // psalter hooks
	"github.com/dodopok/estevao-api-go/internal/store"
)

// Install sets every hook.
func Install() {
	liturgical.LoadCelebrations = func(code string) *liturgical.BookCelebrations {
		ctx := context.Background()
		pb, err := store.PrayerBookByCode(ctx, code)
		if err != nil {
			panic(err)
		}
		bc, err := store.CelebrationsForBook(ctx, pb)
		if err != nil {
			panic(err)
		}
		return bc
	}
	bible.VersionLookup = func(ctx context.Context, code string) (*bible.VersionInfo, error) {
		v, err := store.BibleVersionByCode(ctx, code)
		if err != nil || v == nil {
			return nil, err
		}
		return &bible.VersionInfo{Language: v.Language, VersificationSystem: v.VersificationSystem}, nil
	}
}
