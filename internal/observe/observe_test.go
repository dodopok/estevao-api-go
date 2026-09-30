package observe

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/dodopok/estevao-api-go/internal/metrics"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/web"
)

type recorded struct {
	mu    sync.Mutex
	names []string
}

func (r *recorded) record(name string, _ float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
}

func (r *recorded) has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.names {
		if n == name {
			return true
		}
	}
	return false
}

// start runs the agent against an unreachable collector: transactions,
// hooks and metrics run; nothing leaves the machine.
func start(t *testing.T) *recorded {
	t.Helper()
	t.Setenv("NEW_RELIC_LICENSE_KEY", "0000000000000000000000000000000000000000")
	t.Setenv("NEW_RELIC_HOST", "127.0.0.1:1")
	t.Setenv("NEW_RELIC_APP_NAME", "estevao-test")
	if err := Start(slog.New(slog.NewTextHandler(io.Discard, nil)), "test"); err != nil {
		t.Fatal(err)
	}
	rec := &recorded{}
	metrics.Recorder = rec.record
	t.Cleanup(func() {
		Shutdown(0)
		app, metrics.Recorder, solidqueue.Instrument = nil, nil, nil
	})
	return rec
}

func TestWebRequest(t *testing.T) {
	rec := start(t)
	srv := &web.Server{
		Router: web.NewRouter(web.RailsRoutes),
		Endpoints: map[string]web.Endpoint{
			"api/v1/calendar#day": {Application: true, Handler: func(c *web.Context) {
				c.Raw(200, "application/json; charset=utf-8", []byte(`{"ok":true}`))
			}},
		},
	}
	Install(srv)
	w := httptest.NewRecorder()
	Middleware(srv).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/calendar/2026/1/1", nil))
	if w.Code != 200 || w.Body.String() != `{"ok":true}` {
		t.Fatalf("response changed: %d %s", w.Code, w.Body.String())
	}
	for _, name := range []string{"API/calendar#day/Duration", "API/Status/200"} {
		if !rec.has(name) {
			sort.Strings(rec.names)
			t.Errorf("metric %s not recorded (got %v)", name, rec.names)
		}
	}
}

func TestJobs(t *testing.T) {
	rec := start(t)
	solidqueue.Register("ObserveTestJob", solidqueue.Handler{Queue: "default", Perform: func(ctx context.Context, e *solidqueue.Execution) error {
		if len(e.Arguments) > 0 && e.Arguments[0] == "fail" {
			return errors.New("boom")
		}
		if len(e.Arguments) > 0 && e.Arguments[0] == "panic" {
			panic("kaboom")
		}
		return nil
	}})
	if err := solidqueue.PerformNow(context.Background(), "ObserveTestJob"); err != nil {
		t.Fatal(err)
	}
	if !rec.has("Jobs/ObserveTestJob/Success") || !rec.has("Jobs/ObserveTestJob/Duration") {
		t.Fatalf("success not recorded: %v", rec.names)
	}
	rec.names = nil
	if err := solidqueue.PerformNow(context.Background(), "ObserveTestJob", "fail"); err == nil {
		t.Fatal("the failure was swallowed")
	}
	if !rec.has("Jobs/ObserveTestJob/Failure") {
		t.Fatalf("failure not recorded: %v", rec.names)
	}
	rec.names = nil
	if err := solidqueue.PerformNow(context.Background(), "ObserveTestJob", "panic"); err == nil {
		t.Fatal("the panic was swallowed")
	}
	if !rec.has("Jobs/ObserveTestJob/Failure") {
		t.Fatalf("a panicking job was recorded as a success: %v", rec.names)
	}
}
