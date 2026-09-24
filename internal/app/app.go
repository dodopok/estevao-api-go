// Package app wires the HTTP stack: routes, endpoints and middleware.
package app

import (
	"log/slog"
	"strings"

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
