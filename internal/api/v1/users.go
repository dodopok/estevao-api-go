package v1

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/activestorage"
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// withUser runs authenticate_user! (and preload_avatar, a fresh read of the
// user) before the action.
func withUser(action func(c *web.Context, u *users.User)) web.HandlerFunc {
	return func(c *web.Context) {
		auth.AuthenticateRequired(c)
		u, err := auth.CurrentUser(c).Reload(c.Ctx)
		must(err)
		auth.SetCurrentUser(c, u)
		action(c, u)
	}
}

// profilePhotoURL ports User#profile_photo_url.
func profilePhotoURL(ctx context.Context, u *users.User) (any, bool) {
	b, err := activestorage.Attached(ctx, "User", u.ID, "avatar")
	must(err)
	if b == nil {
		return rb.Deref(u.PhotoURL), false
	}
	host := config.Fetch("APP_HOST", "http://localhost:3000")
	if b.ServiceName == "production" {
		return host + "/storage/" + substr(b.Key, 0, 2) + "/" + substr(b.Key, 2, 4) + "/" + b.Key, true
	}
	return activestorage.BlobURL(b, host), true
}

func substr(s string, from, to int) string {
	if from >= len(s) {
		return ""
	}
	return s[from:min(to, len(s))]
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return rb.FormatTime(*t)
}

func intOrNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func userProfile(ctx context.Context, u *users.User) *rb.Map {
	photo, attached := profilePhotoURL(ctx, u)
	return rb.M("id", u.ID, "email", u.Email, "name", rb.Deref(u.Name), "photo_url", photo, "has_custom_avatar", attached,
		"preferences", u.Preferences, "timezone", u.Timezone, "country_code", rb.Deref(u.CountryCode),
		"current_streak", intOrNil(u.CurrentStreak), "longest_streak", intOrNil(u.LongestStreak),
		"last_completed_office_at", timeOrNil(u.LastCompletedOfficeAt))
}

// UsersShow ports UsersController#show.
var UsersShow = withUser(func(c *web.Context, u *users.User) { c.JSON(200, userProfile(c.Ctx, u)) })

func renderRecordInvalid(c *web.Context, err error) bool {
	var ri *users.RecordInvalid
	if errors.As(err, &ri) {
		msgs := make([]any, len(ri.Messages))
		for i, m := range ri.Messages {
			msgs[i] = m
		}
		c.JSON(422, rb.M("error", msgs))
		return true
	}
	return false
}

// castString ports ActiveModel::Type::String#cast for a permitted scalar.
func castString(v any) *string {
	var s string
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		s = x
	case bool:
		s = "f"
		if x {
			s = "t"
		}
	default:
		s = rb.ToS(x)
	}
	return &s
}

func permittedScalar(v any) bool {
	switch v.(type) {
	case *rb.Map, []any, *web.UploadedFile:
		return false
	}
	return true
}

// UsersUpdateProfile ports UsersController#update_profile.
var UsersUpdateProfile = withUser(func(c *web.Context, u *users.User) {
	before, after := u.Clone(), u.Clone()
	for _, k := range []string{"name", "photo_url"} {
		v, ok := c.Params().Lookup(k)
		if !ok || !permittedScalar(v) {
			continue
		}
		if k == "name" {
			after.Name = castString(v)
		} else {
			after.PhotoURL = castString(v)
		}
	}
	if _, err := users.Save(c.Ctx, db.Q(), before, after); err != nil {
		if renderRecordInvalid(c, err) {
			return
		}
		panic(err)
	}
	out := rb.M("message", "Profile updated successfully")
	userProfile(c.Ctx, after).Each(func(k string, v any) { out.Set(k, v) })
	c.JSON(200, out)
})

var allowedAvatarTypes = []any{"image/jpeg", "image/png", "image/webp"}

const maxAvatarSize = 5 << 20

// UsersUploadAvatar ports UsersController#upload_avatar (Users::UpdateAvatar).
var UsersUploadAvatar = withUser(func(c *web.Context, u *users.User) {
	invalid := func(msg string, extra ...any) {
		out := rb.M("error", msg)
		for i := 0; i+1 < len(extra); i += 2 {
			out.Set(extra[i].(string), extra[i+1])
		}
		c.JSON(422, out)
	}
	v := c.Param("avatar")
	if rb.Blank(v) {
		invalid("Avatar file is required")
		return
	}
	f, ok := v.(*web.UploadedFile)
	if !ok {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("content_type", v)})
	}
	ct := rb.ToS(rb.Deref(f.ContentType))
	allowed := false
	for _, t := range allowedAvatarTypes {
		allowed = allowed || t == ct
	}
	if f.ContentType == nil || !allowed {
		invalid("Invalid file type. Allowed: JPEG, PNG, WebP", "available_types", allowedAvatarTypes)
		return
	}
	if f.Size() > maxAvatarSize {
		invalid("File too large. Maximum size: 5MB", "max_size_bytes", maxAvatarSize)
		return
	}
	if len(u.Errors()) > 0 {
		// record.save fails, the attachment stays unsaved and
		// profile_photo_url asks the unsaved blob for its signed id.
		panic(&web.StandardError{Class: "ArgumentError", Message: "Cannot get a signed_id for a new record"})
	}
	_, err := activestorage.Attach(c.Ctx, "User", "users", u.ID, "avatar",
		activestorage.Upload{Filename: f.OriginalFilename, ContentType: f.ContentType, Data: f.Data}, time.Now())
	must(err)
	photo, _ := profilePhotoURL(c.Ctx, u)
	c.JSON(200, rb.M("message", "Avatar uploaded successfully", "photo_url", photo, "has_custom_avatar", true))
})

// UsersDeleteAvatar ports UsersController#delete_avatar (Users::RemoveAvatar).
var UsersDeleteAvatar = withUser(func(c *web.Context, u *users.User) {
	b, err := activestorage.Attached(c.Ctx, "User", u.ID, "avatar")
	must(err)
	if b == nil {
		c.JSON(404, rb.M("error", "No custom avatar to remove"))
		return
	}
	must(activestorage.Purge(c.Ctx, "User", "users", u.ID, "avatar", time.Now()))
	c.JSON(200, rb.M("message", "Avatar removed successfully", "photo_url", rb.Deref(u.PhotoURL), "has_custom_avatar", false))
})

var preferenceBaseKeys = []string{"version", "prayer_book_code", "language", "bible_version", "lords_prayer_version",
	"creed_type", "confession_type", "notifications", "notifications_enabled", "streak_reminder_enabled",
	"streak_display_enabled", "preferred_audio_voice", "mode"}

var prayerTimeKeys = []string{"office_id", "office_name", "hour", "minute", "enabled"}

var bookPreferenceKey = regexp.MustCompile(`^[a-z0-9_]+$`)

var digitsOnly = regexp.MustCompile(`^\d+$`)

// permitHash ports permit(*keys) on one hash: listed keys with scalar values,
// in the order of keys.
func permitHash(m *rb.Map, keys []string) *rb.Map {
	out := rb.NewMap()
	for _, k := range keys {
		if v, ok := m.Lookup(k); ok && permittedScalar(v) {
			out.Set(k, v)
		}
	}
	return out
}

// preferencesParams ports UsersController#preferences_params.
func preferencesParams(c *web.Context) *rb.Map {
	raw := c.Param("preferences")
	if rb.Blank(raw) {
		panic(&web.StandardError{Class: "ActionController::ParameterMissing", Message: "param is missing or the value is empty or invalid: preferences"})
	}
	p, ok := raw.(*rb.Map)
	if !ok {
		panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("permit", raw)})
	}
	out := permitHash(p, preferenceBaseKeys)
	if v, ok := p.Lookup("prayer_times"); ok {
		switch x := v.(type) {
		case []any:
			list := []any{}
			for _, e := range x {
				if m, ok := e.(*rb.Map); ok {
					list = append(list, permitHash(m, prayerTimeKeys))
				}
			}
			out.Set("prayer_times", list)
		case *rb.Map:
			fieldsFor := x.Len() > 0
			x.Each(func(k string, e any) {
				if _, isMap := e.(*rb.Map); !digitsOnly.MatchString(k) || !isMap {
					fieldsFor = false
				}
			})
			if fieldsFor {
				h := rb.NewMap()
				x.Each(func(k string, e any) { h.Set(k, permitHash(e.(*rb.Map), prayerTimeKeys)) })
				out.Set("prayer_times", h)
			} else {
				out.Set("prayer_times", permitHash(x, prayerTimeKeys))
			}
		}
	}
	skip := map[string]bool{"prayer_times": true}
	for _, k := range preferenceBaseKeys {
		skip[k] = true
	}
	p.Each(func(k string, v any) {
		if skip[k] || !bookPreferenceKey.MatchString(k) {
			return
		}
		ok := false
		switch x := v.(type) {
		case string, int, int64, bool:
			ok = true
		case []any:
			ok = true
			for _, e := range x {
				switch e.(type) {
				case string, int, int64:
				default:
					ok = false
				}
			}
		}
		if ok {
			out.Set(k, v)
		}
	})
	return out
}

// UsersUpdatePreferences ports UsersController#update_preferences.
var UsersUpdatePreferences = withUser(func(c *web.Context, u *users.User) {
	attrs := preferencesParams(c)
	prefs, err := users.UpdatePreferences(c.Ctx, u, attrs)
	if err != nil {
		if errors.As(err, new(users.InvalidAudioVoice)) {
			voices := make([]any, len(users.AvailableVoices))
			for i, v := range users.AvailableVoices {
				voices[i] = v
			}
			c.JSON(422, rb.M("error", "Invalid preferred_audio_voice. Must be one of: "+strings.Join(users.AvailableVoices, ", "),
				"available_voices", voices))
			return
		}
		if renderRecordInvalid(c, err) {
			return
		}
		panic(err)
	}
	c.JSON(200, rb.M("message", "Preferences updated successfully", "preferences", prefs))
})

// UsersCompletions ports UsersController#completions.
var UsersCompletions = withUser(func(c *web.Context, u *users.User) {
	limit := int64(30)
	if v := c.Param("limit"); v != nil {
		n, ok := rubyIntegerArg(v)
		if !ok {
			panic(&web.StandardError{Class: "ArgumentError", Message: "invalid value for Integer(): " + rb.Inspect(v)})
		}
		limit = n
	}
	cols, err := db.OrderedAttributes(c.Ctx, "completions", []string{"id", "date_reference", "office_type", "duration_seconds", "prayer_book_id", "created_at"})
	must(err)
	rows, err := db.Q().Query(c.Ctx, `SELECT id, date_reference, office_type, duration_seconds, prayer_book_id, created_at
		FROM completions WHERE user_id = $1 ORDER BY date_reference DESC, created_at DESC LIMIT $2`, u.ID, limit)
	pgMust(err)
	list := []any{}
	for rows.Next() {
		var id int64
		var date time.Time
		var office string
		var duration *int
		var pbID *int64
		var created time.Time
		must(rows.Scan(&id, &date, &office, &duration, &pbID, &created))
		vals := map[string]any{"id": id, "date_reference": date.Format("2006-01-02"), "office_type": office,
			"duration_seconds": intOrNil(duration), "prayer_book_id": rb.Deref(pbID), "created_at": rb.FormatTime(created)}
		m := rb.NewMap()
		for _, col := range cols {
			m.Set(col, vals[col])
		}
		list = append(list, m)
	}
	rows.Close()
	pgMust(rows.Err())
	var total int64
	must(db.Q().QueryRow(c.Ctx, `SELECT COUNT(*) FROM completions WHERE user_id = $1`, u.ID).Scan(&total))
	c.JSON(200, rb.M("completions", list, "total_completions", total,
		"current_streak", intOrNil(u.CurrentStreak), "longest_streak", intOrNil(u.LongestStreak)))
})

// rubyIntegerArg ports ActiveRecord's sanitize_limit (Integer(value)).
func rubyIntegerArg(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case string:
		s := strings.TrimSpace(x)
		n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
		if err != nil || strings.HasPrefix(s, "_") || strings.HasSuffix(s, "_") || strings.Contains(s, "__") {
			return 0, false
		}
		if len(s) > 1 && (s[0] == '0' || (len(s) > 2 && (s[0] == '-' || s[0] == '+') && s[1] == '0')) {
			// Integer() reads a leading 0 as octal.
			o, err := strconv.ParseInt(strings.TrimLeft(strings.TrimLeft(s, "+-"), "0"), 8, 64)
			if err != nil && strings.TrimLeft(strings.TrimLeft(s, "+-"), "0") != "" {
				return 0, false
			}
			if strings.HasPrefix(s, "-") {
				o = -o
			}
			return o, true
		}
		return n, true
	}
	return 0, false
}

// UsersSaveFCMToken ports UsersController#save_fcm_token.
var UsersSaveFCMToken = withUser(func(c *web.Context, u *users.User) {
	token, _ := c.Params().Lookup("fcm_token")
	if !permittedScalar(token) {
		token = nil
	}
	if rb.Blank(token) {
		c.JSON(422, rb.M("error", "FCM token is required"))
		return
	}
	platform := c.Param("platform")
	if !permittedScalar(platform) || platform == nil {
		platform = "android"
	}
	platformS := castString(platform)
	if platformS != nil && *platformS != "android" && *platformS != "ios" && *platformS != "web" {
		c.JSON(422, rb.M("error", []any{"Platform is not included in the list"}))
		return
	}
	tokenS := *castString(token)
	now := users.Now()
	var id int64
	var existing *string
	err := db.Q().QueryRow(c.Ctx, `SELECT id, platform FROM fcm_tokens WHERE user_id = $1 AND token = $2 LIMIT 1`, u.ID, tokenS).Scan(&id, &existing)
	switch {
	case db.NoRows(err):
		_, err = db.Q().Exec(c.Ctx, `INSERT INTO fcm_tokens (user_id, token, platform, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
			u.ID, tokenS, platformS, now)
		must(err)
	case err != nil:
		panic(err)
	default:
		// save (platform) then touch: updated_at moves either way.
		_, err = db.Q().Exec(c.Ctx, `UPDATE fcm_tokens SET platform = $1, updated_at = $2 WHERE id = $3`, platformS, now, id)
		must(err)
	}
	c.JSON(200, rb.M("message", "Token FCM salvo com sucesso", "fcm_token", token))
})

// UsersDeleteFCMToken ports UsersController#delete_fcm_token.
var UsersDeleteFCMToken = withUser(func(c *web.Context, u *users.User) {
	token := c.Param("fcm_token")
	if rb.Blank(token) {
		c.JSON(422, rb.M("error", "FCM token is required"))
		return
	}
	switch token.(type) {
	case *rb.Map, []any:
		// where(token: array) is an IN list; a hash is a nested condition.
	}
	if list, ok := token.([]any); ok {
		strs := make([]string, 0, len(list))
		for _, e := range list {
			strs = append(strs, rb.ToS(e))
		}
		_, err := db.Q().Exec(c.Ctx, `DELETE FROM fcm_tokens WHERE user_id = $1 AND token = ANY($2)`, u.ID, strs)
		must(err)
	} else if _, isMap := token.(*rb.Map); !isMap {
		_, err := db.Q().Exec(c.Ctx, `DELETE FROM fcm_tokens WHERE user_id = $1 AND token = $2`, u.ID, rb.ToS(token))
		must(err)
	} else {
		panic(&web.StandardError{Class: "ActiveRecord::StatementInvalid", Message: "unsupported fcm_token condition"})
	}
	c.JSON(200, rb.M("message", "Token FCM removido com sucesso"))
})

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)
var countryCodeSep = regexp.MustCompile(`[-_]`)

// UsersUpdateTimezone ports UsersController#update_timezone (Users::UpdateTimezone).
var UsersUpdateTimezone = withUser(func(c *web.Context, u *users.User) {
	tz := rb.ToS(c.Param("timezone"))
	if rb.BlankString(tz) {
		c.JSON(422, rb.M("error", "Timezone is required"))
		return
	}
	if rb.RailsZone(tz) == nil {
		c.JSON(422, rb.M("error", "Invalid timezone. Use IANA timezone format (e.g., 'America/Sao_Paulo', 'Europe/London')"))
		return
	}
	before, after := u.Clone(), u.Clone()
	after.Timezone = tz
	if raw, provided := c.Params().Lookup("country_code"); provided {
		if rb.Blank(raw) {
			after.CountryCode = nil
		} else {
			parts := countryCodeSep.Split(rb.ToS(raw), -1)
			normalized := strings.ToUpper(parts[len(parts)-1])
			if !countryCodeRe.MatchString(normalized) {
				c.JSON(422, rb.M("error", "Invalid country_code. Use a two-letter country code (e.g., 'BR')"))
				return
			}
			after.CountryCode = &normalized
		}
	}
	if _, err := users.Save(c.Ctx, db.Q(), before, after); err != nil {
		if renderRecordInvalid(c, err) {
			return
		}
		panic(err)
	}
	c.JSON(200, rb.M("message", "Timezone updated successfully", "timezone", after.Timezone, "country_code", rb.Deref(after.CountryCode)))
})

// pgMust raises a database error as the ActiveRecord exception Rails would.
func pgMust(err error) {
	if err == nil {
		return
	}
	if class, msg, ok := db.RubyError(err); ok {
		panic(&web.StandardError{Class: class, Message: msg})
	}
	panic(err)
}

// UsersDestroy ports UsersController#destroy (Users::DestroyAccount).
func UsersDestroy(c *web.Context) {
	auth.AuthenticateRequired(c)
	u := auth.CurrentUser(c)
	uid := rb.ToS(rb.Deref(u.ProviderUID))
	if rb.BlankString(uid) {
		panic(&web.StandardError{Class: "ArgumentError", Message: "UID is required"})
	}
	if res := auth.DeleteFirebaseUser(c.Ctx, uid); !res.Success {
		c.JSON(502, rb.M("error", "Unable to delete account at this time", "code", res.ErrorCode, "request_id", c.RequestID))
		return
	}
	blobs, err := users.Destroy(c.Ctx, u.ID)
	pgMust(err)
	for _, b := range blobs {
		activestorage.PurgeLater(b)
	}
	c.JSON(200, rb.M("message", "Account deleted successfully"))
}
