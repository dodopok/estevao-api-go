package v1

import (
	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/notify"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// permitAny ports permit_any_in_parameters (a `key: {}` filter).
func permitAny(m *rb.Map) *rb.Map {
	out := rb.NewMap()
	m.Each(func(k string, v any) {
		switch x := v.(type) {
		case []any:
			out.Set(k, permitAnyArray(x))
		case *rb.Map:
			out.Set(k, permitAny(x))
		default:
			out.Set(k, v)
		}
	})
	return out
}

// permitAnyArray ports permit_any_in_array.
func permitAnyArray(a []any) []any {
	out := []any{}
	for _, el := range a {
		switch e := el.(type) {
		case []any:
			out = append(out, permitAnyArray(e))
		case *rb.Map:
			out = append(out, permitAny(e))
		default:
			out = append(out, e)
		}
	}
	return out
}

// notificationParams ports `params.permit(:title, :body, user_ids: [], data: {})`.
func notificationParams(c *web.Context) (title, body any, userIDs []any, data *rb.Map) {
	p := c.Params()
	if v, ok := p.Lookup("title"); ok && web.PermittedScalar(v) {
		title = v
	}
	if v, ok := p.Lookup("body"); ok && web.PermittedScalar(v) {
		body = v
	}
	if v, ok := p.Get("user_ids").([]any); ok {
		all := true
		for _, el := range v {
			all = all && web.PermittedScalar(el)
		}
		if all {
			userIDs = v
		}
	}
	if v, ok := p.Get("data").(*rb.Map); ok {
		data = permitAny(v)
	}
	return
}

// NotificationsSendToUsers ports NotificationsController#send_to_users.
func NotificationsSendToUsers(c *web.Context) {
	auth.AuthenticateAdmin(c)
	title, body, userIDs, data := notificationParams(c)
	if len(userIDs) == 0 || rb.Blank(title) || rb.Blank(body) {
		c.JSON(422, rb.M("error", "user_ids, title, and body are required"))
		return
	}
	var ids []int64
	for _, v := range userIDs {
		if n, ok := rb.CastInteger(v); ok {
			ids = append(ids, n)
		}
	}
	success, failed := notify.SendToUsers(c.Ctx, ids, title, body, data)
	c.JSON(200, rb.M("message", "Notificações enviadas", "sent_count", success, "failed_count", failed))
}

// NotificationsBroadcast ports #broadcast (always through the job).
func NotificationsBroadcast(c *web.Context) {
	auth.AuthenticateAdmin(c)
	title, body, _, data := notificationParams(c)
	if rb.Blank(title) || rb.Blank(body) {
		c.JSON(422, rb.M("error", "title and body are required"))
		return
	}
	must(notify.EnqueueBroadcast(c.Ctx, title, body, data))
	c.JSON(202, rb.M("message", "Broadcast iniciado em background", "total_users", notify.BroadcastUsers(c.Ctx)))
}
