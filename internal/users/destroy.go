package users

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
)

// Destroy ports user.destroy!: the dependent associations in declaration
// order (children before parents), the avatar attachment, then the row.
// A table that references users without a dependent option (feature_flags)
// makes the final DELETE fail with a foreign-key violation, rolling
// everything back, as in Rails. It returns the ids of blobs whose
// attachment was removed (purged later).
func Destroy(ctx context.Context, id int64) ([]int64, error) {
	var blobs []int64
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		stmts := []string{
			`DELETE FROM completions WHERE user_id = $1`,
			`DELETE FROM fcm_tokens WHERE user_id = $1`,
			`DELETE FROM notification_logs WHERE user_id = $1`,
			`DELETE FROM journals WHERE user_id = $1`,
			`DELETE FROM shared_offices WHERE user_id = $1`,
			`DELETE FROM user_onboardings WHERE user_id = $1`,
			// has_one :life_rule: its steps, then adoptions are nullified.
			`DELETE FROM life_rule_steps WHERE life_rule_id IN (SELECT id FROM life_rules WHERE user_id = $1)`,
			`UPDATE life_rules SET original_life_rule_id = NULL WHERE original_life_rule_id IN (SELECT id FROM life_rules WHERE user_id = $1)`,
			`DELETE FROM life_rules WHERE user_id = $1`,
			`DELETE FROM life_rule_exam_items WHERE life_rule_exam_id IN (SELECT id FROM life_rule_exams WHERE user_id = $1)`,
			`DELETE FROM life_rule_exams WHERE user_id = $1`,
			`DELETE FROM prayer_requests WHERE user_id = $1`,
			`DELETE FROM weekly_prayers WHERE user_id = $1`,
			`DELETE FROM user_favorites WHERE user_id = $1`,
			`DELETE FROM custom_rosary_blocks WHERE custom_rosary_prayer_id IN (SELECT id FROM custom_rosary_prayers WHERE user_id = $1)`,
			`DELETE FROM custom_rosary_prayers WHERE user_id = $1`,
			`DELETE FROM premium_subscription_events WHERE user_id = $1`,
			`DELETE FROM user_audio_usages WHERE user_id = $1`,
			`DELETE FROM user_background_track_favorites WHERE user_id = $1`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s, id); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `DELETE FROM active_storage_attachments WHERE record_type = 'User' AND record_id = $1 AND name = 'avatar' RETURNING blob_id`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var b int64
			if err := rows.Scan(&b); err != nil {
				rows.Close()
				return err
			}
			blobs = append(blobs, b)
		}
		rows.Close()
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return blobs, nil
}
