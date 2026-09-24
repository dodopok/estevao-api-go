package users

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/prefs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// InvalidAudioVoice is the INVALID_AUDIO_VOICE InvalidPreference the
// controller renders itself.
type InvalidAudioVoice struct{}

func (InvalidAudioVoice) Error() string { return "Invalid preferred_audio_voice" }

func invalidPrayerBook(code any) {
	panic(&web.DomainError{Class: "InvalidPreference", Code: "INVALID_PRAYER_BOOK", Message: "Invalid prayer_book_code",
		Context: map[string]any{"prayer_book_code": code}})
}

// UpdatePreferences ports Users::UpdatePreferences.call. attributes is the
// permitted preferences hash. It returns the user's in-memory preferences
// after the update (Ruby insertion order), *RecordInvalid,
// InvalidAudioVoice, or raises the INVALID_PRAYER_BOOK domain error.
func UpdatePreferences(ctx context.Context, u *User, attributes *rb.Map) (*rb.Map, error) {
	current := u.Preferences
	if current == nil {
		current = rb.NewMap()
	}
	code := attributes.Get("prayer_book_code")
	if rb.Blank(code) {
		code = current.Get("prayer_book_code")
	}
	var pb *store.PrayerBook
	if !rb.Blank(code) {
		var err error
		if pb, err = findPrayerBook(ctx, code); err != nil {
			return nil, err
		}
		if pb == nil {
			invalidPrayerBook(code)
		}
		if pb.ExternalOnly {
			invalidPrayerBook(pb.Code)
		}
	}
	normalized := attributes
	var defs *prefs.DefinitionSet
	if pb != nil {
		var err error
		if defs, err = prefs.For(ctx, pb); err != nil {
			return nil, err
		}
		normalized = defs.Normalize(attributes, true, true)
	}
	if v := normalized.Get("preferred_audio_voice"); !rb.Blank(v) && !ValidVoice(v) {
		return nil, InvalidAudioVoice{}
	}
	base := current.Dup()
	if defs != nil {
		base = defs.Normalize(current, true, true)
	}
	updated := base.Merge(normalized)
	var out *rb.Map
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		before := u.Clone()
		after := u.Clone()
		after.Preferences = updated
		if _, err := Save(ctx, tx, before, after); err != nil {
			return err
		}
		if err := syncOnboarding(ctx, tx, after); err != nil {
			return err
		}
		out = after.Preferences
		*u = *after
		return nil
	})
	return out, err
}

// findPrayerBook ports PrayerBook.find_by_code (the column is a string, so
// any value is compared as its text).
func findPrayerBook(ctx context.Context, code any) (*store.PrayerBook, error) {
	switch code.(type) {
	case *rb.Map, []any:
		return nil, nil
	}
	return store.PrayerBookByCode(ctx, rb.ToS(code))
}

// syncOnboarding ports UpdatePreferences#sync_onboarding!.
func syncOnboarding(ctx context.Context, q db.Querier, u *User) error {
	o := u.Onboarding
	if o == nil {
		return nil
	}
	p := u.Preferences
	code := p.Get("prayer_book_code")
	if rb.Blank(code) {
		return nil
	}
	pb, err := findPrayerBook(ctx, code)
	if err != nil || pb == nil {
		return err
	}
	before := o.Clone()
	after := o.Clone()
	changed := false
	if o.PrayerBookID != pb.ID {
		after.PrayerBookID = pb.ID
		after.Preferences = rb.NewMap()
		changed = true
	} else {
		defs, err := prefs.For(ctx, pb)
		if err != nil {
			return err
		}
		liturgical := rb.NewMap()
		for _, k := range defs.Keys() {
			if v, ok := p.Lookup(k); ok {
				liturgical.Set(k, v)
			}
		}
		if liturgical.Len() > 0 {
			after.Preferences = o.Preferences.Merge(liturgical)
			changed = true
		}
	}
	if bvCode := p.Get("bible_version"); !rb.Blank(bvCode) {
		bv, err := store.BibleVersionByCode(ctx, rb.ToS(bvCode))
		if err != nil {
			return err
		}
		if bv != nil && o.BibleVersionID != bv.ID {
			after.BibleVersionID = bv.ID
			changed = true
		}
	}
	if mode := p.Get("mode"); !rb.Blank(mode) && rb.ToS(mode) != o.Mode {
		after.Mode = rb.ToS(mode)
		changed = true
	}
	if !changed {
		return nil
	}
	book, err := store.PrayerBookByID(ctx, after.PrayerBookID)
	if err != nil {
		return err
	}
	if err := SaveOnboarding(ctx, q, before, after, book); err != nil {
		return err
	}
	u.Onboarding = after
	return nil
}
