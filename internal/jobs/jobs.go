// Package jobs runs the application's background jobs in process, with the
// retry semantics of the ActiveJob declarations they port (retry_on with
// :polynomially_longer waits, discard_on). Unlike Solid Queue, the queue is
// not durable: a job pending when the process stops is lost. The jobs whose
// loss matters have a reconciler that re-enqueues them from database state.
package jobs

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Outcome classifies a job error.
type Outcome int

const (
	// Fail stops the job (an error ActiveJob neither retries nor discards).
	Fail Outcome = iota
	// Retry reruns the job after the polynomially longer wait.
	Retry
	// Discard drops the job silently (discard_on).
	Discard
)

// Job is one enqueued unit of work.
type Job struct {
	Name     string
	Attempts int // retry_on attempts (total executions); 1 means no retry
	Perform  func(ctx context.Context) error
	Classify func(err error) Outcome
	Timeout  time.Duration
}

var wg sync.WaitGroup

// polynomiallyLonger is ActiveJob's wait before retry number `executions`:
// executions**4 + 2 seconds, plus up to 15% jitter.
func polynomiallyLonger(executions int) time.Duration {
	base := math.Pow(float64(executions), 4)
	jitter := rand.Float64() * base * 0.15
	return time.Duration((base + jitter + 2) * float64(time.Second))
}

// Enqueue runs the job in the background (perform_later).
func Enqueue(j Job) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		run(j, 1)
	}()
}

func run(j Job, execution int) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("[job] "+j.Name+" failed", "error", rec)
		}
	}()
	timeout := j.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	err := j.Perform(ctx)
	cancel()
	if err == nil {
		return
	}
	outcome := Fail
	if j.Classify != nil {
		outcome = j.Classify(err)
	}
	switch {
	case outcome == Discard:
		slog.Info("[job] "+j.Name+" discarded", "error", err)
	case outcome == Retry && execution < j.Attempts:
		time.Sleep(polynomiallyLonger(execution))
		run(j, execution+1)
	default:
		slog.Error("[job] "+j.Name+" failed", "error", err, "executions", execution)
	}
}

// Wait blocks until every enqueued job has finished (tests and shutdown).
func Wait() { wg.Wait() }
