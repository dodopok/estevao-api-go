// Package app wires the HTTP stack: routes, endpoints and middleware.
package app

import (
	"log/slog"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/activestorage"
	"github.com/dodopok/estevao-api-go/internal/api/v1"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/ratelimit"
	"github.com/dodopok/estevao-api-go/internal/web"
	"github.com/dodopok/estevao-api-go/internal/wiring"
)

// Endpoints maps Rails "controller#action" names to Go handlers.
func Endpoints() map[string]web.Endpoint {
	app := func(h web.HandlerFunc) web.Endpoint { return web.Endpoint{Handler: h, Application: true} }
	e := map[string]web.Endpoint{
		"rails/health#show":                 {Handler: health},
		"health#readiness":                  {Handler: readiness},
		"home#index":                        app(home),
		"api/v1/calendar#month":             app(v1.CalendarMonth),
		"api/v1/calendar#year":              app(v1.CalendarYear),
		"api/v1/calendar#overview":          app(v1.CalendarOverview),
		"api/v1/calendar#year_seasons":      app(v1.CalendarYearSeasons),
		"api/v1/calendar#year_key_dates":    app(v1.CalendarYearKeyDates),
		"api/v1/calendar#year_celebrations": app(v1.CalendarYearCelebrations),
	}
	for k, v := range activeStorageEndpoints() {
		live := k == "active_storage/blobs/proxy#show" || k == "active_storage/representations/proxy#show"
		e[k] = web.Endpoint{Handler: v, Live: live}
	}
	for k, v := range v1.Endpoints() {
		e[k] = app(v)
	}
	return e
}

// NewServer builds the HTTP handler.
func NewServer(logger *slog.Logger, publicDir string) *web.Server {
	wiring.Install()
	var origins []string
	for _, o := range strings.Split(config.Get("CORS_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return &web.Server{
		Router:    web.NewRouter(web.RailsRoutes),
		Endpoints: Endpoints(),
		Attack:    ratelimit.Middleware(),
		Static:    Static(publicDir),
		CORS:      &web.CORS{Origins: origins},
		Logger:    logger,
	}
}

// activeStorageEndpoints are the routes the Active Storage and Action
// Mailbox engines mount (ActionController::Base controllers).
func activeStorageEndpoints() map[string]web.HandlerFunc {
	notFound := func(c *web.Context) { c.HeadBase(404) }
	forbidden := func(c *web.Context) { c.HeadBase(403) }
	e := map[string]web.HandlerFunc{
		"active_storage/blobs/redirect#show":           activestorage.BlobRedirect,
		"active_storage/blobs/proxy#show":              activestorage.BlobProxy,
		"active_storage/representations/redirect#show": activestorage.Representation,
		"active_storage/representations/proxy#show":    activestorage.Representation,
		"active_storage/disk#show":                     activestorage.DiskShow,
		"active_storage/disk#update":                   activestorage.DiskUpdate,
		"active_storage/direct_uploads#create":         activestorage.DirectUploadsCreate,
		// No ingress is configured: every ingress answers ensure_configured.
		"action_mailbox/ingresses/mailgun/inbound_emails#create":        notFound,
		"action_mailbox/ingresses/mandrill/inbound_emails#create":       notFound,
		"action_mailbox/ingresses/mandrill/inbound_emails#health_check": notFound,
		"action_mailbox/ingresses/postmark/inbound_emails#create":       notFound,
		"action_mailbox/ingresses/relay/inbound_emails#create":          notFound,
		"action_mailbox/ingresses/sendgrid/inbound_emails#create":       notFound,
	}
	// The conductor only runs in development (ensure_development_env).
	for _, k := range []string{
		"rails/conductor/action_mailbox/inbound_emails#index", "rails/conductor/action_mailbox/inbound_emails#create",
		"rails/conductor/action_mailbox/inbound_emails#new", "rails/conductor/action_mailbox/inbound_emails#show",
		"rails/conductor/action_mailbox/inbound_emails/sources#new", "rails/conductor/action_mailbox/inbound_emails/sources#create",
		"rails/conductor/action_mailbox/reroutes#create", "rails/conductor/action_mailbox/incinerates#create",
	} {
		e[k] = forbidden
	}
	return e
}
