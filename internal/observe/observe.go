// Package observe reports to New Relic what the Rails app reported: a
// transaction per request and per job, errors with the same custom
// parameters, PostgreSQL segments, and the custom metrics (Custom/API/*,
// Custom/Jobs/*, and those domain code records through internal/metrics).
// It is off unless NEW_RELIC_LICENSE_KEY is set; the agent's own settings
// come from the NEW_RELIC_* environment (newrelic.ConfigFromEnvironment).
package observe

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/dodopok/estevao-api-go/internal/auth"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/metrics"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/web"
)

var app *newrelic.Application

// Start creates the agent when a license key is configured and installs the
// hooks: database tracer (call before db.Open), job instrumentation and the
// metrics recorder. Web requests need Middleware and Install as well.
func Start(logger *slog.Logger, role string) error {
	key := strings.TrimSpace(config.Get("NEW_RELIC_LICENSE_KEY"))
	if key == "" || strings.EqualFold(config.Get("NEW_RELIC_AGENT_ENABLED"), "false") {
		logger.Info("new relic: off (no NEW_RELIC_LICENSE_KEY)")
		return nil
	}
	a, err := newrelic.NewApplication(
		newrelic.ConfigFromEnvironment(),
		newrelic.ConfigAppName(config.PresenceOr("NEW_RELIC_APP_NAME", "estevao-api")),
		newrelic.ConfigLicense(key),
		newrelic.ConfigDistributedTracerEnabled(true),
		func(c *newrelic.Config) {
			c.Labels = map[string]string{"stack": "go", "role": role}
			// Errors are noticed explicitly, with Rails' parameters; a
			// status code alone does not make one.
			c.ErrorCollector.IgnoreStatusCodes = append(c.ErrorCollector.IgnoreStatusCodes, 400, 401, 403, 404, 405, 409, 422, 429)
		},
	)
	if err != nil {
		return fmt.Errorf("new relic: %w", err)
	}
	app = a
	db.Tracer = nrpgx5.NewTracer()
	solidqueue.Instrument = instrumentJob
	metrics.Recorder = func(name string, value float64) { app.RecordCustomMetric(name, value) }
	logger.Info("new relic: on", "app", config.PresenceOr("NEW_RELIC_APP_NAME", "estevao-api"), "role", role)
	return nil
}

// Enabled reports whether the agent runs.
func Enabled() bool { return app != nil }

// Shutdown flushes the agent's data (on SIGTERM).
func Shutdown(timeout time.Duration) {
	if app != nil {
		app.Shutdown(timeout)
	}
}

// Middleware starts a web transaction per request; Install names it after
// the route once the server has routed it.
func Middleware(next http.Handler) http.Handler {
	if app == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		txn := app.StartTransaction("unrouted")
		defer txn.End()
		txn.SetWebRequestHTTP(r)
		w = txn.SetWebResponse(w)
		next.ServeHTTP(w, newrelic.RequestWithTransactionContext(r, txn))
	})
}

// Install sets the server's hooks.
func Install(s *web.Server) {
	if app == nil {
		return
	}
	s.Observe = observeEndpoint
	prev := s.ReportError
	s.ReportError = func(c *web.Context, err any, stack []byte) {
		reportError(c, err)
		if prev != nil {
			prev(c, err, stack)
		}
	}
}

// observeEndpoint ports ApplicationController#track_request_timing.
func observeEndpoint(c *web.Context, ep web.Endpoint, elapsed time.Duration) {
	txn := newrelic.FromContext(c.Ctx)
	if txn == nil {
		return
	}
	txn.SetName(strings.Replace(c.Endpoint, "#", "/", 1))
	if !ep.Application {
		return
	}
	short := c.Controller[strings.LastIndex(c.Controller, "/")+1:]
	status := c.Status
	metrics.Record("API/"+short+"#"+c.Action+"/Duration", float64(elapsed.Milliseconds()))
	metrics.Record("API/Status/"+strconv.Itoa(status), 1)
	txn.AddAttribute("controller", short)
	txn.AddAttribute("action", c.Action)
	txn.AddAttribute("status", status)
	if v := rb.ToS(c.Param("office_type")); v != "" {
		txn.AddAttribute("office_type", v)
	}
	book := rb.ToS(c.Param("prayer_book_code"))
	if book == "" {
		if v, ok := c.Get("resolved_prayer_book_code").(string); ok {
			book = v
		}
	}
	if book != "" {
		txn.AddAttribute("prayer_book", book)
	}
}

// reportError ports report_error_to_newrelic.
func reportError(c *web.Context, err any) {
	txn := newrelic.FromContext(c.Ctx)
	if txn == nil {
		return
	}
	class := fmt.Sprintf("%T", err)
	msg := fmt.Sprint(err)
	if ie, ok := err.(*web.InfraError); ok {
		class, msg = ie.Class, ie.Message
	}
	attrs := map[string]any{
		"controller": c.Controller[strings.LastIndex(c.Controller, "/")+1:],
		"action":     c.Action,
		"request_id": c.RequestID,
	}
	if u := auth.CurrentUser(c); u != nil {
		attrs["user_id"] = u.ID
	}
	txn.NoticeError(newrelic.Error{Message: msg, Class: class, Attributes: attrs})
}

// instrumentJob ports ApplicationJob#track_job_metrics.
func instrumentJob(ctx context.Context, e *solidqueue.Execution) (context.Context, func(error)) {
	txn := app.StartTransaction(e.Class)
	start := time.Now()
	return newrelic.NewContext(ctx, txn), func(err error) {
		result := "Success"
		if err != nil {
			result = "Failure"
			jobID := ""
			if e.Data != nil {
				jobID = rb.ToS(e.Data.Get("job_id"))
			}
			txn.NoticeError(newrelic.Error{Message: err.Error(), Class: fmt.Sprintf("%T", err), Attributes: map[string]any{
				"job_class": e.Class, "job_id": jobID, "arguments": string(rb.JSON(e.Arguments)),
			}})
		}
		metrics.Record("Jobs/"+e.Class+"/Duration", float64(time.Since(start).Milliseconds()))
		metrics.Increment("Jobs/" + e.Class + "/" + result)
		txn.End()
	}
}
