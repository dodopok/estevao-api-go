// Package workers holds the ActiveJob classes that are not tied to one
// domain package (maintenance, cache warming), registered on the Solid
// Queue–compatible runner.
package workers

import (
	"context"
	"encoding/base64"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
)

func init() {
	solidqueue.Register("FlushApiKeyUsageJob", solidqueue.Handler{Queue: "maintenance",
		Perform: func(ctx context.Context, _ *solidqueue.Execution) error {
			if n := FlushAPIKeyUsage(ctx, 500); n > 0 {
				slog.Info("Flushed " + strconv.Itoa(n) + " API key requests")
			}
			return nil
		}})
	solidqueue.Register("DatabaseCleanupJob", solidqueue.Handler{Queue: "default", Perform: databaseCleanup})
	solidqueue.Register("CleanupExpiredSharedOfficesJob", solidqueue.Handler{Queue: "default",
		Perform: func(ctx context.Context, _ *solidqueue.Execution) error {
			tag, err := db.Q().Exec(ctx, `DELETE FROM shared_offices WHERE expires_at <= $1`, time.Now().UTC())
			if err == nil {
				slog.Info("[CleanupExpiredSharedOfficesJob] Deleted " + strconv.FormatInt(tag.RowsAffected(), 10) + " expired shared offices")
			}
			return err
		}})
}

// --- ApiKeyUsageCounter.flush! ---------------------------------------------------

const (
	counterPrefix = "v8/api_key_usage/counter"
	pendingPrefix = "v8/api_key_usage/pending"
)

var moveToPending = redis.NewScript(`local count = tonumber(redis.call("GET", KEYS[1]) or "0")
redis.call("DEL", KEYS[1])

if count > 0 then
  redis.call("SET", KEYS[2], tostring(count))
end

return count
`)

var batchIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// FlushAPIKeyUsage ports ApiKeyUsageCounter.flush!: counters move to a
// pending key atomically, and each pending batch is applied to PostgreSQL
// once (its receipt in api_key_usage_flushes) before it is deleted.
func FlushAPIKeyUsage(ctx context.Context, maxKeys int) int {
	r := rediscache.Client
	if r == nil || maxKeys <= 0 {
		return 0
	}
	flushed, remaining := 0, maxKeys
	for _, key := range scanKeys(ctx, r, rediscache.Key(pendingPrefix+"/*"), remaining) {
		flushed += flushPending(ctx, r, key)
		remaining--
		if remaining <= 0 {
			return flushed
		}
	}
	for _, key := range scanKeys(ctx, r, rediscache.Key(counterPrefix+"/*"), remaining) {
		flushed += moveAndFlush(ctx, r, key)
		remaining--
		if remaining <= 0 {
			break
		}
	}
	return flushed
}

func scanKeys(ctx context.Context, r *redis.Client, pattern string, limit int) []string {
	count := int64(limit)
	if count > 1000 {
		count = 1000
	}
	var keys []string
	iter := r.Scan(ctx, 0, pattern, count).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= limit {
			break
		}
	}
	return keys
}

func decodeEndpoint(s string) (string, bool) {
	s = strings.TrimRight(s, "=")
	b, err := base64.RawURLEncoding.DecodeString(s)
	return string(b), err == nil
}

func moveAndFlush(ctx context.Context, r *redis.Client, key string) int {
	suffix, ok := strings.CutPrefix(key, rediscache.Key(counterPrefix+"/"))
	parts := strings.SplitN(suffix, "/", 3)
	if !ok || len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		r.Del(ctx, key)
		return 0
	}
	id, err1 := strconv.ParseInt(parts[0], 10, 64)
	endpoint, ok2 := decodeEndpoint(parts[2])
	_, ok3 := civil.ParseISO(parts[1])
	if err1 != nil || !ok2 || !ok3 {
		r.Del(ctx, key)
		return 0
	}
	pending := rediscache.Key(pendingPrefix + "/" + solidqueue.NewJobID() + "/" + strconv.FormatInt(id, 10) + "/" + parts[1] + "/" +
		base64.RawURLEncoding.EncodeToString([]byte(endpoint)))
	count, err := moveToPending.Run(ctx, r, []string{key, pending}).Int()
	if err != nil || count <= 0 {
		return 0
	}
	return flushPending(ctx, r, pending)
}

func flushPending(ctx context.Context, r *redis.Client, key string) int {
	suffix, ok := strings.CutPrefix(key, rediscache.Key(pendingPrefix+"/"))
	parts := strings.SplitN(suffix, "/", 4)
	if !ok || len(parts) != 4 || !batchIDRe.MatchString(parts[0]) || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		r.Del(ctx, key)
		return 0
	}
	id, err1 := strconv.ParseInt(parts[1], 10, 64)
	date, ok2 := civil.ParseISO(parts[2])
	endpoint, ok3 := decodeEndpoint(parts[3])
	if err1 != nil || !ok2 || !ok3 {
		r.Del(ctx, key)
		return 0
	}
	count, _ := r.Get(ctx, key).Int()
	if count <= 0 {
		r.Del(ctx, key)
		return 0
	}
	applied := false
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO api_key_usage_flushes (id, created_at) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
			parts[0], time.Now().UTC())
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		t := time.Now().UTC()
		_, err = tx.Exec(ctx, `WITH updated_api_key AS (
				UPDATE api_keys SET requests_count = COALESCE(requests_count, 0) + $1, last_used_at = $2 WHERE id = $3 RETURNING id)
			INSERT INTO api_key_usage_logs (api_key_id, endpoint, date, requests_count, created_at, updated_at)
			SELECT updated_api_key.id, $4, $5, $1, $2, $2 FROM updated_api_key
			ON CONFLICT (api_key_id, endpoint, date) DO UPDATE SET
				requests_count = api_key_usage_logs.requests_count + EXCLUDED.requests_count, updated_at = EXCLUDED.updated_at`,
			count, t, id, endpoint, date.ISO())
		applied = err == nil
		return err
	})
	if err != nil {
		slog.Warn("Failed to flush API key usage: " + err.Error())
		return 0
	}
	r.Del(ctx, key)
	if applied {
		return count
	}
	return 0
}

// --- DatabaseCleanupJob ------------------------------------------------------------

func deleteInBatches(ctx context.Context, table, where string, arg any) (int64, error) {
	var total int64
	for {
		tag, err := db.Q().Exec(ctx, `DELETE FROM `+table+` WHERE id IN (SELECT id FROM `+table+` WHERE `+where+` ORDER BY id LIMIT 1000)`, arg)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() == 0 {
			return total, nil
		}
	}
}

// databaseCleanup ports DatabaseCleanupJob#perform.
func databaseCleanup(ctx context.Context, _ *solidqueue.Execution) error {
	now := time.Now().UTC()
	var total int64
	for _, step := range []struct {
		table, where, label string
		arg                 time.Time
	}{
		{"notification_logs", "created_at < $1", "old notification logs", now.Add(-90 * 24 * time.Hour)},
		{"shared_offices", "expires_at < $1", "expired shared offices", now},
		{"api_key_usage_flushes", "created_at < $1", "API key usage flush receipts", now.Add(-30 * 24 * time.Hour)},
	} {
		n, err := deleteInBatches(ctx, step.table, step.where, step.arg)
		if err != nil {
			return err
		}
		slog.Info("[DatabaseCleanupJob] Deleted " + strconv.FormatInt(n, 10) + " " + step.label)
		total += n
	}
	slog.Info("[DatabaseCleanupJob] Total deleted: " + strconv.FormatInt(total, 10) + " records")
	return nil
}

var _ = rb.ToS
