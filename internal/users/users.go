// Package users ports the User and UserOnboarding models: persistence,
// default preferences and the Bible/Prayer Book language sync callback.
package users

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/store"
)

// DefaultTimezone is User::DEFAULT_TIMEZONE.
const DefaultTimezone = "America/Sao_Paulo"

// User is a users row plus its onboarding.
type User struct {
	ID                    int64
	Admin                 bool
	CountryCode           *string
	CreatedAt             time.Time
	CurrentStreak         *int
	Email                 string
	LastCompletedOfficeAt *time.Time
	LongestStreak         *int
	Name                  *string
	PhotoURL              *string
	Preferences           *rb.Map
	PremiumExpiresAt      *time.Time
	ProviderUID           *string
	RevenueCatUserID      *string
	Timezone              string
	UpdatedAt             time.Time

	Onboarding *Onboarding
}

// Onboarding is a user_onboardings row.
type Onboarding struct {
	ID                  int64
	BibleVersionID      int64
	CompletedAt         *time.Time
	CreatedAt           time.Time
	Mode                string
	OnboardingCompleted *bool
	PrayerBookID        int64
	Preferences         *rb.Map
	UpdatedAt           time.Time
	UserID              int64
}

const userColumns = `u.id, u.admin, u.country_code, u.created_at, u.current_streak, u.email,
u.last_completed_office_at, u.longest_streak, u.name, u.photo_url, u.preferences, u.premium_expires_at,
u.provider_uid, u.revenue_cat_user_id, u.timezone, u.updated_at`

const onboardingColumns = `o.id, o.bible_version_id, o.completed_at, o.created_at, o.mode, o.onboarding_completed,
o.prayer_book_id, o.preferences, o.updated_at, o.user_id`

func decodeMap(b []byte) *rb.Map {
	if len(b) == 0 {
		return rb.NewMap()
	}
	v, err := rb.ParseJSON(b)
	if err != nil {
		return rb.NewMap()
	}
	if m, ok := v.(*rb.Map); ok {
		return m
	}
	return rb.NewMap()
}

// load runs a query selecting user and onboarding columns (LEFT JOIN).
func load(ctx context.Context, q db.Querier, where string, args ...any) (*User, error) {
	sql := `SELECT ` + userColumns + `, ` + onboardingColumns + ` FROM users u
		LEFT OUTER JOIN user_onboardings o ON o.user_id = u.id WHERE ` + where + ` LIMIT 1`
	row := q.QueryRow(ctx, sql, args...)
	var u User
	var prefs, oPrefs []byte
	var oID, oBV, oPB, oUser *int64
	var oCompletedAt *time.Time
	var oCreated, oUpdated *time.Time
	var oMode *string
	var oDone *bool
	err := row.Scan(&u.ID, &u.Admin, &u.CountryCode, &u.CreatedAt, &u.CurrentStreak, &u.Email,
		&u.LastCompletedOfficeAt, &u.LongestStreak, &u.Name, &u.PhotoURL, &prefs, &u.PremiumExpiresAt,
		&u.ProviderUID, &u.RevenueCatUserID, &u.Timezone, &u.UpdatedAt,
		&oID, &oBV, &oCompletedAt, &oCreated, &oMode, &oDone, &oPB, &oPrefs, &oUpdated, &oUser)
	if db.NoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Preferences = decodeMap(prefs)
	if oID != nil {
		u.Onboarding = &Onboarding{ID: *oID, BibleVersionID: *oBV, CompletedAt: oCompletedAt, CreatedAt: *oCreated,
			Mode: *oMode, OnboardingCompleted: oDone, PrayerBookID: *oPB, Preferences: decodeMap(oPrefs),
			UpdatedAt: *oUpdated, UserID: *oUser}
	}
	return &u, nil
}

// ByProviderUID finds a user by Firebase uid.
func ByProviderUID(ctx context.Context, uid string) (*User, error) {
	return load(ctx, db.Q(), "u.provider_uid = $1", uid)
}

// ByEmail finds a user by email.
func ByEmail(ctx context.Context, email string) (*User, error) {
	return load(ctx, db.Q(), "u.email = $1", email)
}

// ByID finds a user by id.
func ByID(ctx context.Context, id int64) (*User, error) {
	return load(ctx, db.Q(), "u.id = $1", id)
}

// Reload re-reads the user.
func (u *User) Reload(ctx context.Context) (*User, error) { return ByID(ctx, u.ID) }

// OnboardingCompleted ports onboarding_completed?.
func (u *User) OnboardingCompleted() bool {
	return u.Onboarding != nil && u.Onboarding.OnboardingCompleted != nil && *u.Onboarding.OnboardingCompleted
}

// Premium ports premium? (production: MOCK_PREMIUM ignored).
func (u *User) Premium() bool {
	return u.PremiumExpiresAt != nil && u.PremiumExpiresAt.After(time.Now())
}

// PreferredAudioVoice ports preferred_audio_voice.
func (u *User) PreferredAudioVoice() string {
	if v := u.Preferences.Get("preferred_audio_voice"); rb.Truthy(v) {
		return rb.ToS(v)
	}
	return "male_1"
}

// Now returns the current time truncated to Postgres precision.
func Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// DefaultPreferences is User::DEFAULT_PREFERENCES.
func DefaultPreferences() *rb.Map {
	return rb.M(
		"notifications", true,
		"notifications_enabled", true,
		"streak_reminder_enabled", true,
		"streak_display_enabled", true,
		"prayer_times", []any{},
		"version", "loc_2015",
		"prayer_book_code", "loc_2015",
		"language", "pt-BR",
		"bible_version", "nvi",
		"preferred_audio_voice", "male_1",
	)
}

// SyncBibleVersionLanguage ports the before_save sync_bible_version_language
// callback. oldPrefs is the value before the change.
func SyncBibleVersionLanguage(ctx context.Context, oldPrefs, newPrefs *rb.Map) error {
	if oldPrefs == nil {
		oldPrefs = rb.NewMap()
	}
	changed := !sameValue(oldPrefs.Get("prayer_book_code"), newPrefs.Get("prayer_book_code")) ||
		!sameValue(oldPrefs.Get("language"), newPrefs.Get("language")) ||
		rb.Blank(newPrefs.Get("bible_version"))
	if !changed {
		return nil
	}
	code := newPrefs.Get("prayer_book_code")
	if rb.Blank(code) {
		return nil
	}
	pb, err := store.PrayerBookByCode(ctx, rb.ToS(code))
	if err != nil || pb == nil {
		return err
	}
	current, err := store.BibleVersionByCode(ctx, rb.ToS(newPrefs.Get("bible_version")))
	if err != nil {
		return err
	}
	newPrefs.Set("language", pb.Language)
	if current == nil || !langs.CompatibleBibleLanguages(current.LanguageS(), pb.Language) {
		nb, err := store.DefaultBibleVersionFor(ctx, pb.Language)
		if err != nil {
			return err
		}
		if nb != nil {
			newPrefs.Set("bible_version", nb.Code)
		}
	}
	return nil
}

// AdvisoryLockID ports the lock id FirebaseAuthService derives for a uid.
func AdvisoryLockID(providerUID string) int64 {
	sum := sha256.Sum256([]byte("firebase-user:" + providerUID))
	n, _ := strconv.ParseInt(hex.EncodeToString(sum[:])[:15], 16, 64)
	return n
}

// Create inserts a user the way User.create! does for a Firebase sign-in.
func Create(ctx context.Context, q db.Querier, providerUID, email string, name, photoURL *string) (*User, error) {
	prefs := DefaultPreferences()
	if err := SyncBibleVersionLanguage(ctx, nil, prefs); err != nil {
		return nil, err
	}
	now := Now()
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO users (email, provider_uid, name, photo_url, preferences, timezone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7) RETURNING id`,
		email, providerUID, name, photoURL, string(rb.JSON(prefs)), DefaultTimezone, now).Scan(&id)
	if err != nil {
		return nil, err
	}
	return load(ctx, q, "u.id = $1", id)
}

// UpdateColumns runs an UPDATE of the given columns plus updated_at.
func UpdateColumns(ctx context.Context, q db.Querier, id int64, cols map[string]any) error {
	if len(cols) == 0 {
		return nil
	}
	sql := "UPDATE users SET "
	args := []any{}
	i := 1
	for k, v := range cols {
		sql += pgx.Identifier{k}.Sanitize() + " = $" + strconv.Itoa(i) + ", "
		args = append(args, v)
		i++
	}
	sql += "updated_at = $" + strconv.Itoa(i) + " WHERE id = $" + strconv.Itoa(i+1)
	args = append(args, Now(), id)
	_, err := q.Exec(ctx, sql, args...)
	return err
}

// sameValue compares two JSON-like scalars the way Ruby's == does.
func sameValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return string(rb.JSON(a)) == string(rb.JSON(b))
}

// EffectivePreferences ports User#effective_preferences ({} without an
// onboarding). definitions returns the book's definition keys and typed
// defaults, in order.
func (u *User) EffectivePreferences(ctx context.Context, definitions func(pb *store.PrayerBook) ([]string, *rb.Map, error)) (*rb.Map, error) {
	if u.Onboarding == nil {
		return rb.NewMap(), nil
	}
	o := u.Onboarding
	pbs, err := store.PrayerBooksWhere(ctx, "id = $1", o.PrayerBookID)
	if err != nil {
		return nil, err
	}
	if len(pbs) == 0 {
		return nil, errNotFound("PrayerBook")
	}
	keys, defaults, err := definitions(pbs[0])
	if err != nil {
		return nil, err
	}
	prefs := rb.NewMap()
	for _, k := range keys {
		if v, ok := o.Preferences.Lookup(k); ok {
			prefs.Set(k, v)
		} else {
			prefs.Set(k, defaults.Get(k))
		}
	}
	bvs, err := store.BibleVersionsWhere(ctx, "WHERE id = $1", o.BibleVersionID)
	if err != nil {
		return nil, err
	}
	if len(bvs) == 0 {
		return nil, errNotFound("BibleVersion")
	}
	prefs.Set("prayer_book_code", pbs[0].Code)
	prefs.Set("bible_version", bvs[0].Code)
	prefs.Set("mode", o.Mode)
	return prefs, nil
}

type notFound string

func (n notFound) Error() string { return "undefined method for nil (" + string(n) + ")" }

func errNotFound(model string) error { return notFound(model) }
