package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/ar"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/users"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Log is a NotificationLog row.
type Log struct {
	ID             int64
	Sent           bool
	ErrorMessage   *string
	DeliveryStatus []byte
}

func (l *Log) deliveryStatus() map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(l.DeliveryStatus, &out)
	return out
}

// persistDeliveryStatus ports FcmService.persist_delivery_status
// (update_columns).
func (l *Log) persistDeliveryStatus(ctx context.Context, token string, r tokenResult) {
	status := "failed"
	switch {
	case r.Success:
		status = "sent"
	case r.Invalid:
		status = "invalid"
	}
	statuses := l.deliveryStatus()
	statuses[tokenKey(token)] = status
	raw, err := json.Marshal(statuses)
	must(err)
	l.DeliveryStatus = raw
	_, err = db.Q().Exec(ctx, `UPDATE notification_logs SET delivery_status = $1, updated_at = $2 WHERE id = $3`,
		string(raw), users.Now(), l.ID)
	must(err)
}

// logAttrs are the attributes NotificationLog.create! receives.
type logAttrs struct {
	UserID         int64
	Type           any
	Title, Body    any
	Data           *rb.Map
	Sent           bool
	ErrorMessage   any
	IdempotencyKey string
}

// createLog ports NotificationLog.create! (validations, then the insert).
func createLog(ctx context.Context, a logAttrs) *Log {
	errs := &ar.Errors{}
	typ, title := ar.CastString(a.Type), ar.CastString(a.Title)
	if ar.Blank(typ) {
		errs.Add("notification_type", "can't be blank")
	}
	if ar.Blank(title) {
		errs.Add("title", "can't be blank")
	}
	if a.IdempotencyKey != "" {
		var taken bool
		must(db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_logs WHERE idempotency_key = $1)`, a.IdempotencyKey).Scan(&taken))
		if taken {
			errs.Add("idempotency_key", "has already been taken")
		}
	}
	if errs.Any() {
		panic(&ar.RecordInvalid{Errors: errs})
	}
	data := a.Data
	if data == nil {
		data = rb.NewMap()
	}
	var key any
	if a.IdempotencyKey != "" {
		key = a.IdempotencyKey
	}
	now := users.Now()
	l := &Log{Sent: a.Sent, DeliveryStatus: []byte("{}")}
	if s, ok := ar.CastString(a.ErrorMessage).(string); ok {
		l.ErrorMessage = &s
	}
	err := db.Q().QueryRow(ctx, `INSERT INTO notification_logs (user_id, notification_type, title, body, data, sent,
		error_message, idempotency_key, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9) RETURNING id`,
		a.UserID, typ, title, ar.CastString(a.Body), rb.JSON(data), a.Sent, l.ErrorMessage, key, now).Scan(&l.ID)
	if db.UniqueViolation(err) {
		panic(errNotUnique)
	}
	must(err)
	return l
}

// recordFailed ports record_failed_notification (log errors are swallowed).
func recordFailed(ctx context.Context, u *users.User, n *Notification, typ any, message string) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("Failed to create notification log", "error", rec)
		}
	}()
	createLog(ctx, logAttrs{UserID: u.ID, Type: typ, Title: n.Title, Body: n.Body, Data: n.Data, ErrorMessage: message})
}

// Result is the hash send_notification_with_log returns.
type Result struct {
	Sent  bool
	Error any
}

// advisoryLockID ports with_delivery_lock's key.
func advisoryLockID(key string) int64 {
	sum := sha256.Sum256([]byte(key))
	n, _ := strconv.ParseInt(hex.EncodeToString(sum[:])[:15], 16, 64)
	return n
}

// sendWithLog ports NotificationService.send_notification_with_log.
func sendWithLog(ctx context.Context, u *users.User, n *Notification, typ any, idempotencyKey string) (res Result) {
	if u.Preferences != nil {
		if v, ok := u.Preferences.Lookup("notifications"); ok && v == false {
			return Result{Error: "Notifications disabled for user"}
		}
	}
	defer func() {
		rec := recover()
		if rec == nil {
			return
		}
		if rec == errNotUnique {
			var sent bool
			var msg *string
			err := db.Q().QueryRow(ctx, `SELECT sent, error_message FROM notification_logs WHERE idempotency_key = $1 LIMIT 1`,
				idempotencyKey).Scan(&sent, &msg)
			if err == nil {
				res = Result{Sent: sent, Error: rb.Deref(msg)}
				return
			}
			panic(&web.StandardError{Class: "ActiveRecord::RecordNotUnique", Message: "duplicate notification log"})
		}
		if _, ok := isUnavailable(rec); ok {
			panic(rec)
		}
		slog.Error("Error sending notification", "user_id", u.ID, "error", rec)
		recordFailed(ctx, u, n, typ, "Notification delivery failed")
		res = Result{Error: "Notification delivery failed"}
	}()

	var log *Log
	if idempotencyKey != "" {
		log = findOrCreateDeliveryLog(ctx, u, n, typ, idempotencyKey)
		if log.Sent {
			return Result{Sent: true, Error: rb.Deref(log.ErrorMessage)}
		}
	}
	deliver := func() *Log {
		resp := sendToUser(ctx, u, n, log)
		sent := resp.Success
		if log != nil {
			var msg *string
			if s, ok := ar.CastString(resp.Error).(string); ok {
				msg = &s
			}
			_, err := db.Q().Exec(ctx, `UPDATE notification_logs SET sent = $1, error_message = $2, updated_at = $3 WHERE id = $4`,
				sent, msg, users.Now(), log.ID)
			must(err)
			log.Sent, log.ErrorMessage = sent, msg
			return log
		}
		return createLog(ctx, logAttrs{UserID: u.ID, Type: typ, Title: n.Title, Body: n.Body, Data: n.Data, Sent: sent,
			ErrorMessage: resp.Error})
	}
	if idempotencyKey != "" {
		log = withDeliveryLock(ctx, idempotencyKey, func() *Log {
			reloadLog(ctx, log)
			if log.Sent {
				return log
			}
			return deliver()
		})
	} else {
		log = deliver()
	}
	return Result{Sent: log.Sent, Error: rb.Deref(log.ErrorMessage)}
}

func reloadLog(ctx context.Context, l *Log) {
	must(db.Q().QueryRow(ctx, `SELECT sent, error_message, delivery_status FROM notification_logs WHERE id = $1`, l.ID).
		Scan(&l.Sent, &l.ErrorMessage, &l.DeliveryStatus))
}

// findOrCreateDeliveryLog ports find_or_create_delivery_log. Its create!
// validates the key's uniqueness first, so an existing log raises
// RecordInvalid, not RecordNotUnique.
func findOrCreateDeliveryLog(ctx context.Context, u *users.User, n *Notification, typ any, key string) *Log {
	return createLog(ctx, logAttrs{UserID: u.ID, Type: typ, Title: n.Title, Body: n.Body, Data: n.Data, IdempotencyKey: key})
}

// withDeliveryLock ports with_delivery_lock: a session advisory lock held
// around the delivery.
func withDeliveryLock(ctx context.Context, key string, f func() *Log) *Log {
	conn, err := db.Pool.Acquire(ctx)
	must(err)
	defer conn.Release()
	id := advisoryLockID(key)
	_, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, id)
	must(err)
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, id) }()
	return f()
}

// withClickAction ports `data.merge(click_action: "FLUTTER_NOTIFICATION_CLICK")`.
func withClickAction(data *rb.Map) *rb.Map {
	out := rb.NewMap()
	if data != nil {
		data.Each(func(k string, v any) { out.Set(k, v) })
	}
	out.Set("click_action", "FLUTTER_NOTIFICATION_CLICK")
	return out
}

func typeOr(data *rb.Map, def string) any {
	if data != nil {
		if v := data.Get("type"); v != nil && v != false {
			return v
		}
	}
	return def
}

// loadUsers loads users by the where clause, ordered by id (find_each).
func loadUsers(ctx context.Context, where string, args ...any) []*users.User {
	rows, err := db.Q().Query(ctx, `SELECT users.id FROM users WHERE `+where+` ORDER BY users.id ASC`, args...)
	must(err)
	var ids []int64
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	must(rows.Err())
	out := make([]*users.User, 0, len(ids))
	for _, id := range ids {
		u, err := users.ByID(ctx, id)
		must(err)
		if u != nil {
			out = append(out, u)
		}
	}
	return out
}

// SendToUsers ports NotificationService.send_to_users.
func SendToUsers(ctx context.Context, userIDs []int64, title, body any, data *rb.Map) (success, failed int) {
	n := &Notification{Title: title, Body: body, Data: withClickAction(data), Params: data != nil}
	typ := typeOr(data, "custom")
	for _, u := range loadUsers(ctx, `users.id = ANY($1)`, userIDs) {
		r := func() (r Result) {
			defer func() {
				if rec := recover(); rec != nil {
					e, ok := isUnavailable(rec)
					if !ok {
						panic(rec)
					}
					slog.Warn("FCM unavailable", "user_id", u.ID, "error", e.Message)
					recordFailed(ctx, u, n, typ, e.Message)
					r = Result{Error: e.Message}
				}
			}()
			return sendWithLog(ctx, u, n, typ, "")
		}()
		if r.Sent {
			success++
		} else {
			failed++
		}
	}
	return success, failed
}

// Broadcast ports NotificationService.broadcast with an idempotency key (the
// job id). An unavailable FCM propagates, for the job to retry.
func Broadcast(ctx context.Context, title, body any, data *rb.Map, idempotencyKey string) (total, success, failed int) {
	cutoff := time.Now().Add(-60 * 24 * time.Hour)
	must(db.Q().QueryRow(ctx, `SELECT COUNT(DISTINCT users.id) FROM users INNER JOIN fcm_tokens ON fcm_tokens.user_id = users.id
		WHERE (fcm_tokens.updated_at > $1)`, cutoff).Scan(&total))
	n := &Notification{Title: title, Body: body, Data: withClickAction(data)}
	typ := typeOr(data, "broadcast")
	for _, u := range loadUsers(ctx, `users.id IN (SELECT fcm_tokens.user_id FROM fcm_tokens WHERE fcm_tokens.updated_at > $1)`, cutoff) {
		key := ""
		if idempotencyKey != "" {
			key = idempotencyKey + ":" + strconv.FormatInt(u.ID, 10)
		}
		if sendWithLog(ctx, u, n, typ, key).Sent {
			success++
		} else {
			failed++
		}
	}
	return total, success, failed
}

// BroadcastUsers ports `User.joins(:fcm_tokens).distinct.count`.
func BroadcastUsers(ctx context.Context) int64 {
	var n int64
	must(db.Q().QueryRow(ctx, `SELECT COUNT(DISTINCT users.id) FROM users INNER JOIN fcm_tokens ON fcm_tokens.user_id = users.id`).Scan(&n))
	return n
}

var _ = pgx.ErrNoRows
