package users

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// RecordInvalid is ActiveRecord::RecordInvalid with the record's full
// messages.
type RecordInvalid struct{ Messages []string }

func (e *RecordInvalid) Error() string { return "Validation failed" }

// Equal compares two decoded JSON values the way Ruby's == compares
// Hashes, Arrays and scalars (Hash key order is irrelevant).
func Equal(a, b any) bool {
	switch x := a.(type) {
	case *rb.Map:
		y, ok := b.(*rb.Map)
		if !ok || x.Len() != y.Len() {
			return false
		}
		eq := true
		x.Each(func(k string, v any) {
			w, ok := y.Lookup(k)
			if !ok || !Equal(v, w) {
				eq = false
			}
		})
		return eq
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !Equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case nil:
		return b == nil
	}
	if b == nil {
		return false
	}
	return string(rb.JSON(a)) == string(rb.JSON(b))
}

func strEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Save ports `user.update!(attributes)` for the attributes the API lets a
// user change: before_validation normalizes the country code, the model
// validations run, before_save syncs the Bible version when preferences
// changed, and only changed columns (plus updated_at) are written. It
// returns *RecordInvalid when a validation fails; u is updated in place.
func Save(ctx context.Context, q db.Querier, before, u *User) (bool, error) {
	u.CountryCode = NormalizeCountryCode(u.CountryCode)
	if msgs := u.Errors(); len(msgs) > 0 {
		return false, &RecordInvalid{Messages: msgs}
	}
	cols := map[string]any{}
	if !strEq(before.Name, u.Name) {
		cols["name"] = u.Name
	}
	if !strEq(before.PhotoURL, u.PhotoURL) {
		cols["photo_url"] = u.PhotoURL
	}
	if before.Timezone != u.Timezone {
		cols["timezone"] = u.Timezone
	}
	if !strEq(before.CountryCode, u.CountryCode) {
		cols["country_code"] = u.CountryCode
	}
	if !Equal(before.Preferences, u.Preferences) {
		if err := SyncBibleVersionLanguage(ctx, before.Preferences, u.Preferences); err != nil {
			return false, err
		}
		cols["preferences"] = string(rb.JSON(u.Preferences))
	}
	if len(cols) == 0 {
		return false, nil
	}
	now := Now()
	if err := UpdateColumnsAt(ctx, q, u.ID, cols, now); err != nil {
		return false, err
	}
	u.UpdatedAt = now
	return true, nil
}

// Clone copies the user's mutable attributes (the "before" of an update).
func (u *User) Clone() *User {
	c := *u
	if u.Preferences != nil {
		c.Preferences = u.Preferences.Dup()
	}
	return &c
}
