package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var bearerRe = regexp.MustCompile(`(?i)\ABearer\s+(\S+)\z`)

// BearerToken ports extract_token_from_header.
func BearerToken(c *web.Context) string {
	h := c.HeaderValue("Authorization")
	if strings.TrimSpace(h) == "" {
		return ""
	}
	m := bearerRe.FindStringSubmatch(h)
	if m == nil {
		return ""
	}
	return m[1]
}

const currentUserKey = "current_user"

// CurrentUser returns the authenticated user or nil.
func CurrentUser(c *web.Context) *users.User {
	u, _ := c.Get(currentUserKey).(*users.User)
	return u
}

// SetCurrentUser replaces the request's user (after a reload).
func SetCurrentUser(c *web.Context, u *users.User) { c.Set(currentUserKey, u) }

// userFromToken ports FirebaseAuthService.verify_and_get_user.
func userFromToken(ctx context.Context, token string) (*users.User, error) {
	claims, err := VerifyToken(token, false)
	if err != nil {
		return nil, err
	}
	return findOrCreateUser(ctx, claims)
}

func findExisting(ctx context.Context, claims jwt.MapClaims) (*users.User, error) {
	u, err := users.ByProviderUID(ctx, ClaimString(claims, "sub"))
	if err != nil || u != nil {
		return u, err
	}
	if email := ClaimString(claims, "email"); strings.TrimSpace(email) != "" {
		return users.ByEmail(ctx, email)
	}
	return nil, nil
}

func findOrCreateUser(ctx context.Context, claims jwt.MapClaims) (*users.User, error) {
	u, err := findExisting(ctx, claims)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return synchronize(ctx, u, claims)
	}
	sub, _ := claims["sub"].(string)
	var created *users.User
	err = withCreationLock(ctx, sub, func(conn *pgx.Conn) error {
		existing, err := findExisting(ctx, claims)
		if err != nil {
			return err
		}
		if existing != nil {
			created, err = synchronize(ctx, existing, claims)
			return err
		}
		email := ClaimString(claims, "email")
		if email == "" {
			email = sub + "@firebase.user"
		}
		created, err = users.Create(ctx, db.Q(), sub, email, optString(claims, "name"), optString(claims, "picture"))
		return err
	})
	if err != nil {
		if db.UniqueViolation(err) {
			if u, err2 := findExisting(ctx, claims); err2 == nil && u != nil {
				return u, nil
			}
		}
		return nil, err
	}
	return created, nil
}

func optString(c jwt.MapClaims, key string) *string {
	if s, ok := c[key].(string); ok {
		return &s
	}
	return nil
}

func withCreationLock(ctx context.Context, uid string, f func(conn *pgx.Conn) error) error {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	id := users.AdvisoryLockID(uid)
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", id); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", id)
	return f(conn.Conn())
}

// synchronize ports synchronize_user_from_payload.
func synchronize(ctx context.Context, u *users.User, claims jwt.MapClaims) (*users.User, error) {
	cols := map[string]any{}
	sub := ClaimString(claims, "sub")
	if u.ProviderUID == nil || *u.ProviderUID != sub {
		cols["provider_uid"] = claims["sub"]
	}
	if email := ClaimString(claims, "email"); strings.TrimSpace(email) != "" && u.Email != email {
		cols["email"] = email
	}
	if (u.Name == nil || rb.BlankString(*u.Name)) && strings.TrimSpace(ClaimString(claims, "name")) != "" {
		cols["name"] = ClaimString(claims, "name")
	}
	if (u.PhotoURL == nil || rb.BlankString(*u.PhotoURL)) && strings.TrimSpace(ClaimString(claims, "picture")) != "" {
		attached, err := avatarAttached(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		if !attached {
			cols["photo_url"] = ClaimString(claims, "picture")
		}
	}
	if len(cols) == 0 {
		return u, nil
	}
	// user.update(...) returns false on validation/uniqueness failure and the
	// request continues with the unchanged user.
	if email, ok := cols["email"].(string); ok {
		var taken bool
		_ = db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND id <> $2)`, email, u.ID).Scan(&taken)
		if taken {
			return u, nil
		}
	}
	if err := users.UpdateColumns(ctx, db.Q(), u.ID, cols); err != nil {
		if db.UniqueViolation(err) {
			return u, nil
		}
		return nil, err
	}
	return u.Reload(ctx)
}

func avatarAttached(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM active_storage_attachments WHERE record_type = 'User' AND record_id = $1 AND name = 'avatar')`, userID).Scan(&exists)
	return exists, err
}

// AuthenticateOptional ports authenticate_user_optional.
func AuthenticateOptional(c *web.Context) {
	token := BearerToken(c)
	if token == "" {
		return
	}
	u, err := userFromToken(c.Ctx, token)
	if err != nil {
		var infra *web.InfraError
		if errors.As(err, &infra) {
			panic(infra)
		}
		if errors.Is(err, ErrInvalidToken) {
			return
		}
		panic(err)
	}
	SetCurrentUser(c, u)
}

// AuthenticateRequired ports authenticate_user!.
func AuthenticateRequired(c *web.Context) {
	token := BearerToken(c)
	var u *users.User
	if token != "" {
		var err error
		u, err = userFromToken(c.Ctx, token)
		if err != nil {
			var infra *web.InfraError
			if errors.As(err, &infra) {
				panic(infra)
			}
			if errors.Is(err, ErrInvalidToken) {
				c.RenderJSON(401, rb.M("error", "Authentication failed", "code", "AUTHENTICATION_FAILED", "request_id", c.RequestID))
			}
			panic(err)
		}
	}
	if u != nil {
		SetCurrentUser(c, u)
		return
	}
	c.RenderJSON(401, rb.M("error", "Unauthorized", "code", "AUTHENTICATION_REQUIRED", "request_id", c.RequestID))
}

// RequirePremium ports require_premium!.
func RequirePremium(c *web.Context) {
	u := CurrentUser(c)
	if u == nil || !u.Premium() {
		c.RenderJSON(403, rb.M("error", "Premium subscription required", "premium", false,
			"message", "This feature requires an active premium subscription"))
	}
}

// appVerificationRequired ports app_request_verification_required?.
func appVerificationRequired() bool {
	if v, set := config.Bool("REQUIRE_APP_REQUEST_VERIFICATION"); set {
		return v
	}
	if _, exists := lookupEnv("REQUIRE_APP_REQUEST_VERIFICATION"); exists {
		// Set to "" casts to nil, which is falsy.
		return false
	}
	return config.Present("APP_INTERNAL_IDENTIFIER") || config.Production
}

// VerifyAppRequest ports verify_app_request!.
func VerifyAppRequest(c *web.Context) {
	if !appVerificationRequired() {
		return
	}
	if APIKey(c) != nil {
		return
	}
	if CurrentUser(c) != nil {
		return
	}
	appID := c.HeaderValue("X-App-Internal-Id")
	expected := config.Get("APP_INTERNAL_IDENTIFIER")
	if strings.TrimSpace(expected) == "" {
		c.RenderJSON(503, rb.M("error", "Anonymous app verification unavailable", "code", "APP_VERIFICATION_UNAVAILABLE", "request_id", c.RequestID))
	}
	if strings.TrimSpace(appID) == "" || appID != expected {
		c.RenderJSON(401, rb.M("error", "Unauthorized access", "code", "APP_VERIFICATION_REQUIRED",
			"message", "This endpoint requires authentication or a valid app identifier.", "request_id", c.RequestID))
	}
}

// IsAdmin ports AdminPolicy.allowed?.
func IsAdmin(u *users.User) bool {
	if u == nil {
		return false
	}
	if u.Admin {
		return true
	}
	allowed := strings.TrimSpace(config.Get("DASHBOARD_ADMIN_EMAIL"))
	return allowed != "" && strings.EqualFold(u.Email, allowed)
}

// AuthenticateAdmin ports authenticate_admin!.
func AuthenticateAdmin(c *web.Context) {
	AuthenticateRequired(c)
	if !IsAdmin(CurrentUser(c)) {
		c.RenderJSON(403, rb.M("error", "Admin access required", "code", "ADMIN_ACCESS_REQUIRED", "request_id", c.RequestID))
	}
}

// SecureCompare is ActiveSupport::SecurityUtils.secure_compare.
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var _ = time.Now
