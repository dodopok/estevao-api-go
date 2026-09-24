package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// health ports Rails::HealthController#show.
func health(c *web.Context) {
	format := c.Format
	if format == "" {
		switch negotiate(c.R) {
		case "json":
			format = "json"
		default:
			format = "html"
		}
	}
	if format == "json" {
		c.JSON(200, rb.M("status", "up", "timestamp", time.Now().In(rb.AppZone).Format("2006-01-02T15:04:05-07:00")))
		return
	}
	c.Raw(200, "text/html; charset=utf-8", []byte(`<!DOCTYPE html><html><body style="background-color: green"></body></html>`))
}

func negotiate(r *http.Request) string {
	a := r.Header.Get("Accept")
	if strings.Contains(a, "application/json") && !strings.Contains(a, "*/*") && !strings.Contains(a, "text/html") {
		return "json"
	}
	return "html"
}

// readiness ports HealthController#readiness.
func readiness(c *web.Context) {
	var one int
	if err := db.Q().QueryRow(c.Ctx, "SELECT 1").Scan(&one); err != nil {
		c.JSON(503, rb.M("status", "not_ready"))
		return
	}
	c.JSON(200, rb.M("status", "ready"))
}

// home ports HomeController#index.
func home(c *web.Context) {
	c.JSON(200, rb.M(
		"api", "Calendário Litúrgico Anglicano",
		"version", "1.0",
		"docs", "/api-docs",
		"endpoints", rb.M(
			"calendar", rb.M("today", "/api/v1/calendar/today", "day", "/api/v1/calendar/:year/:month/:day",
				"month", "/api/v1/calendar/:year/:month", "year", "/api/v1/calendar/:year"),
			"liturgical_explanations", rb.M("show", "/api/v1/liturgical_explanation/:year/:month/:day"),
			"celebrations", rb.M("list", "/api/v1/celebrations", "details", "/api/v1/celebrations/:id",
				"search", "/api/v1/celebrations/search?q=term", "by_date", "/api/v1/celebrations/date/:month/:day",
				"types", "/api/v1/celebrations/types"),
			"lectionary", rb.M("day", "/api/v1/lectionary/:year/:month/:day",
				"all_services", "/api/v1/lectionary/:year/:month/:day/all_services", "cycle", "/api/v1/lectionary/cycle/:year"),
			"daily_office", rb.M("today", "/api/v1/daily_office/today/:office_type",
				"show", "/api/v1/daily_office/:year/:month/:day/:office_type",
				"family", "/api/v1/daily_office/:year/:month/:day/:office_type/family",
				"preferences", "/api/v1/daily_office/preferences"),
			"users", rb.M("me", "GET /api/v1/users/me", "delete_account", "DELETE /api/v1/users/me",
				"update_preferences", "PATCH /api/v1/users/preferences", "update_timezone", "PATCH /api/v1/users/timezone",
				"completions", "GET /api/v1/users/completions", "save_fcm_token", "POST /api/v1/users/fcm_token",
				"delete_fcm_token", "DELETE /api/v1/users/fcm_token"),
			"onboarding", rb.M("create", "POST /api/v1/users/onboarding", "show", "GET /api/v1/users/me/onboarding"),
			"completions", rb.M("create", "POST /api/v1/completions", "destroy", "DELETE /api/v1/completions/:id",
				"show", "/api/v1/completions/:year/:month/:day/:office_type"),
			"journals", rb.M("create", "POST /api/v1/journals", "update", "PATCH /api/v1/journals/:id",
				"destroy", "DELETE /api/v1/journals/:id", "day", "/api/v1/journals/:year/:month/:day",
				"month", "/api/v1/journals/:year/:month"),
			"notifications", rb.M("send", "POST /api/v1/notifications/send (admin)", "broadcast", "POST /api/v1/notifications/broadcast (admin)"),
			"prayer_books", rb.M("list", "/api/v1/prayer_books", "show", "/api/v1/prayer_books/:code",
				"preferences", "/api/v1/prayer_books/:code/preferences"),
			"bible_versions", rb.M("list", "/api/v1/bible_versions"),
			"shared_offices", rb.M("create", "POST /api/v1/shared_offices", "show", "/api/v1/shared_offices/:code"),
			"life_rules", rb.M("list", "/api/v1/life_rules", "my", "/api/v1/life_rules/my", "show", "/api/v1/life_rules/:id",
				"create", "POST /api/v1/life_rules", "update", "PATCH /api/v1/life_rules/:id",
				"destroy", "DELETE /api/v1/life_rules/:id", "adopt", "POST /api/v1/life_rules/:id/adopt",
				"approve", "POST /api/v1/life_rules/:id/approve (admin)"),
		),
	))
}

// Static ports ActionDispatch::Static over public/ (robots.txt).
func Static(publicDir string) func(r *http.Request) *web.Response {
	return func(r *http.Request) *web.Response {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return nil
		}
		p := filepath.Clean(r.URL.Path)
		if p == "/" || strings.Contains(p, "..") {
			return nil
		}
		full := filepath.Join(publicDir, p)
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			return nil
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		h := http.Header{}
		h.Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		ct := "application/octet-stream"
		if strings.HasSuffix(p, ".txt") {
			ct = "text/plain"
		}
		h.Set("Content-Type", ct)
		h.Set("Cache-Control", "public, max-age=31556952")
		return &web.Response{Status: 200, Header: h, Body: b, Static: true}
	}
}
