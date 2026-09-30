package features

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/users"
)

// The operations of rake feature_flags:* (FeatureFlagsRakeSupport).

// OverrideRow is one persisted override.
type OverrideRow struct {
	Feature, TargetType string
	UserID              *int64
	Email               *string
	Enabled             bool
}

// ValidateFeature ports feature_key!.
func ValidateFeature(key string) (string, error) {
	key = strings.TrimSpace(key)
	if _, ok := Definitions[key]; ok {
		return key, nil
	}
	shown := key
	if shown == "" {
		shown = "(vazia)"
	}
	return "", fmt.Errorf("Feature desconhecida: %s. Disponíveis: %s", shown, strings.Join(Keys, ", "))
}

// ValidateTarget ports target_type!.
func ValidateTarget(v string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(v))
	if t == "" {
		t = "global"
	}
	if t != "global" && t != "user" {
		return "", fmt.Errorf("Target inválido: %s. Use global ou user.", t)
	}
	return t, nil
}

// FindTargetUser ports find_user!: email, uid:<firebase uid>, id:<id>, or
// a bare e-mail or Firebase uid.
func FindTargetUser(ctx context.Context, target string) (*users.User, error) {
	raw := strings.TrimSpace(target)
	selector, value, _ := strings.Cut(raw, ":")
	var id int64
	query := ""
	var arg any
	switch strings.ToLower(selector) {
	case "email":
		query, arg = `LOWER(email) = $1`, strings.ToLower(value)
	case "uid", "provider_uid":
		query, arg = `provider_uid = $1`, value
	case "id":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Usuário não encontrado: %s. Use e-mail, uid:<firebase_uid> ou id:<user_id>.", raw)
		}
		query, arg = `id = $1`, n
	default:
		if strings.Contains(raw, "@") {
			query, arg = `LOWER(email) = $1`, strings.ToLower(raw)
		} else {
			query, arg = `provider_uid = $1`, raw
		}
	}
	err := db.Q().QueryRow(ctx, `SELECT id FROM users WHERE `+query+` ORDER BY id LIMIT 1`, arg).Scan(&id)
	if db.NoRows(err) {
		return nil, fmt.Errorf("Usuário não encontrado: %s. Use e-mail, uid:<firebase_uid> ou id:<user_id>.", raw)
	}
	if err != nil {
		return nil, err
	}
	return users.ByID(ctx, id)
}

// SetOverride ports set_override!: one row per (feature, target), created
// or updated; updated_at moves only when enabled changes.
func SetOverride(ctx context.Context, feature, targetType string, user *users.User, enabled bool) error {
	var userID any
	if user != nil {
		userID = user.ID
	}
	now := users.Now()
	tag, err := db.Q().Exec(ctx, `UPDATE feature_flags SET enabled = $4::boolean, updated_at = $5::timestamp
		WHERE feature_key = $1::varchar AND target_type = $2::varchar AND user_id IS NOT DISTINCT FROM $3::bigint AND enabled IS DISTINCT FROM $4::boolean`,
		feature, targetType, userID, enabled, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := db.Q().Exec(ctx, `INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at)
			SELECT $1::varchar, $2::varchar, $3::bigint, $4::boolean, $5::timestamp, $5::timestamp WHERE NOT EXISTS (
			  SELECT 1 FROM feature_flags WHERE feature_key = $1::varchar AND target_type = $2::varchar AND user_id IS NOT DISTINCT FROM $3::bigint)`,
			feature, targetType, userID, enabled, now); err != nil {
			return err
		}
	}
	expireOverrides(ctx, feature)
	return nil
}

// ResetOverride ports reset!: removes the override; false when none existed.
func ResetOverride(ctx context.Context, feature, targetType string, user *users.User) (bool, error) {
	var userID any
	if user != nil {
		userID = user.ID
	}
	tag, err := db.Q().Exec(ctx, `DELETE FROM feature_flags WHERE feature_key = $1::varchar AND target_type = $2::varchar AND user_id IS NOT DISTINCT FROM $3::bigint`,
		feature, targetType, userID)
	if err != nil {
		return false, err
	}
	expireOverrides(ctx, feature)
	return tag.RowsAffected() > 0, nil
}

// ListOverrides ports list, in its order.
func ListOverrides(ctx context.Context) ([]OverrideRow, error) {
	rows, err := db.Q().Query(ctx, `SELECT f.feature_key, f.target_type, f.user_id, u.email, f.enabled
		FROM feature_flags f LEFT JOIN users u ON u.id = f.user_id ORDER BY f.feature_key, f.target_type, f.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OverrideRow
	for rows.Next() {
		var r OverrideRow
		if err := rows.Scan(&r.Feature, &r.TargetType, &r.UserID, &r.Email, &r.Enabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// expireOverrides ports FeatureFlag#expire_overrides_cache for a Rails app
// still reading the same database (the Go stack does not cache overrides).
func expireOverrides(ctx context.Context, feature string) {
	rediscache.DeleteRails(ctx, "feature_flags/v1/"+feature)
}
