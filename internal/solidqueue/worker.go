package solidqueue

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// Error is a Ruby exception as a failed execution records it.
type Error struct {
	Class     string
	Message   string
	Backtrace []string
}

func (e *Error) Error() string { return e.Message }

// RubyClassed is an error that knows its Ruby exception class.
type RubyClassed interface{ RubyClass() string }

// ClassOf names the Ruby exception class an error stands for.
func ClassOf(err error) string {
	var rc RubyClassed
	if errors.As(err, &rc) {
		return rc.RubyClass()
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	var de *web.DomainError
	if errors.As(err, &de) {
		return de.Class
	}
	var ie *web.InfraError
	if errors.As(err, &ie) {
		return ie.Class
	}
	var se *web.StandardError
	if errors.As(err, &se) {
		return se.Class
	}
	if class, _, ok := db.RubyError(err); ok {
		return class
	}
	return "RuntimeError"
}

// Rescue is one retry_on or discard_on declaration.
type Rescue struct {
	// Key is `exceptions.to_s` of the declaration ("[Klass]"), the counter
	// key in exception_executions.
	Key string
	// Match reports whether the declaration rescues err.
	Match func(err error) bool
	// Attempts > 0 is retry_on with that many attempts; 0 is discard_on.
	Attempts int
	// Wait is the delay before retry number executions (nil:
	// :polynomially_longer).
	Wait func(executions int) time.Duration
}

// Classes matches errors whose Ruby class is one of names.
func Classes(names ...string) func(error) bool {
	return func(err error) bool {
		c := ClassOf(err)
		for _, n := range names {
			if n == c {
				return true
			}
		}
		return false
	}
}

// Handler is one ActiveJob class: its queue_as and its perform.
type Handler struct {
	Queue   string
	Perform func(ctx context.Context, e *Execution) error
	// Rescue lists retry_on/discard_on in declaration order.
	Rescue []Rescue
	// Timeout bounds one execution (0: no bound).
	Timeout time.Duration
}

// Execution is a job being performed.
type Execution struct {
	ID          int64 // provider_job_id
	Class       string
	Data        *rb.Map // the serialized ActiveJob
	Arguments   []any   // deserialized arguments
	Executions  int
	Enqueuer    func(ctx context.Context, j Job) (*Enqueued, error)
	serialArgs  []any
	exceptionEx *rb.Map
}

// Arg returns positional argument i (nil when absent).
func (e *Execution) Arg(i int) any {
	if i < len(e.Arguments) {
		return e.Arguments[i]
	}
	return nil
}

// Kwarg returns a keyword argument (the trailing keyword hash).
func (e *Execution) Kwarg(name string) any {
	if n := len(e.serialArgs); n > 0 && IsKwargs(e.serialArgs[n-1]) {
		if m, ok := e.Arguments[n-1].(*rb.Map); ok {
			return m.Get(name)
		}
	}
	return nil
}

// kwargs returns the trailing keyword hash (nil when the job has none).
func (e *Execution) kwargs() *rb.Map {
	if n := len(e.serialArgs); n > 0 && IsKwargs(e.serialArgs[n-1]) {
		if m, ok := e.Arguments[n-1].(*rb.Map); ok {
			return m
		}
	}
	return nil
}

// HasKwarg reports whether the keyword was passed (a Ruby default applies
// otherwise).
func (e *Execution) HasKwarg(name string) bool {
	m := e.kwargs()
	return m != nil && m.Has(name)
}

// KwargOr returns the keyword argument, or def when it was not passed.
func (e *Execution) KwargOr(name string, def any) any {
	if m := e.kwargs(); m != nil && m.Has(name) {
		return m.Get(name)
	}
	return def
}

// Positional returns the arguments before the keyword hash.
func (e *Execution) Positional() []any {
	if e.kwargs() != nil {
		return e.Arguments[:len(e.Arguments)-1]
	}
	return e.Arguments
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Handler{}
)

// Register declares an ActiveJob class.
func Register(class string, h Handler) {
	registryMu.Lock()
	registry[class] = h
	registryMu.Unlock()
}

// QueueOf is the queue_as of a registered class ("default" otherwise).
func QueueOf(class string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if h, ok := registry[class]; ok && h.Queue != "" {
		return h.Queue
	}
	return "default"
}

// PerformLater enqueues class with serialized arguments on its queue.
func PerformLater(ctx context.Context, class string, args ...any) (*Enqueued, error) {
	return Enqueue(ctx, Job{Class: class, Queue: QueueOf(class), Arguments: args})
}

// PerformLaterAt enqueues class to run at t.
func PerformLaterAt(ctx context.Context, t time.Time, class string, args ...any) (*Enqueued, error) {
	return Enqueue(ctx, Job{Class: class, Queue: QueueOf(class), Arguments: args, ScheduledAt: t})
}

// polynomiallyLonger is executions**4 + jitter + 2 seconds (jitter up to
// 15%, ActiveJob's retry_jitter).
func polynomiallyLonger(executions int) time.Duration {
	base := math.Pow(float64(executions), 4)
	return time.Duration((base + rand.Float64()*base*0.15 + 2) * float64(time.Second))
}

// --- performing -------------------------------------------------------------

type claimed struct {
	jobID     int64
	class     string
	arguments string
}

// perform ports ClaimedExecution#perform around ActiveJob::Base.execute.
func perform(ctx context.Context, c claimed) error {
	err := execute(ctx, c)
	if err == nil {
		return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			t := now()
			if _, err := tx.Exec(ctx, `UPDATE solid_queue_jobs SET finished_at = $2, updated_at = $2 WHERE id = $1`, c.jobID, t); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM solid_queue_claimed_executions WHERE job_id = $1`, c.jobID)
			return err
		})
	}
	slog.Error("[solid_queue] job failed", "class", c.class, "job_id", c.jobID, "error", err.Error())
	return failWith(ctx, c.jobID, err)
}

// failWith records a failed execution and drops the claim.
func failWith(ctx context.Context, jobID int64, err error) error {
	e := &Error{Class: ClassOf(err), Message: err.Error()}
	var re *Error
	if errors.As(err, &re) && re.Backtrace != nil {
		e.Backtrace = re.Backtrace
	}
	if e.Backtrace == nil {
		e.Backtrace = []string{}
	}
	payload := rb.JSONGenerate(rb.M("exception_class", e.Class, "message", e.Message, "backtrace", strs(e.Backtrace)))
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_failed_executions (job_id, error, created_at) VALUES ($1, $2, $3)
			ON CONFLICT (job_id) DO NOTHING`, jobID, payload, now()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM solid_queue_claimed_executions WHERE job_id = $1`, jobID)
		return err
	})
}

func strs(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// execute ports ActiveJob::Base.execute: deserialize, perform_now, and the
// rescue_from handlers retry_on/discard_on declare. A nil error means the
// job finished (performed, retried or discarded).
func execute(ctx context.Context, c claimed) (err error) {
	parsed, perr := rb.ParseJSON([]byte(c.arguments))
	data, ok := parsed.(*rb.Map)
	if perr != nil || !ok {
		return &Error{Class: "JSON::ParserError", Message: "invalid job arguments"}
	}
	class := rb.ToS(data.Get("job_class"))
	registryMu.RLock()
	h, known := registry[class]
	registryMu.RUnlock()
	if !known {
		return &Error{Class: "ActiveJob::UnknownJobClassError", Message: "Failed to instantiate job, class `" + class + "` doesn't exist"}
	}
	executions := rb.ToI(data.Get("executions")) + 1
	ee, _ := data.Get("exception_executions").(*rb.Map)
	if ee == nil {
		ee = rb.NewMap()
	} else {
		ee = ee.Dup()
	}
	serialArgs, _ := data.Get("arguments").([]any)
	exec := &Execution{ID: c.jobID, Class: class, Data: data, Executions: executions, serialArgs: serialArgs, exceptionEx: ee}
	runErr := func() (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = panicError(rec)
			}
		}()
		args, derr := Deserialize(serialArgs)
		if derr != nil {
			return derr
		}
		exec.Arguments, _ = args.([]any)
		pctx := ctx
		if h.Timeout > 0 {
			var cancel context.CancelFunc
			pctx, cancel = context.WithTimeout(ctx, h.Timeout)
			defer cancel()
		}
		return h.Perform(pctx, exec)
	}()
	if runErr == nil {
		return nil
	}
	// rescue_from handlers are searched from the last declared.
	for i := len(h.Rescue) - 1; i >= 0; i-- {
		r := h.Rescue[i]
		if !r.Match(runErr) {
			continue
		}
		if r.Attempts == 0 {
			slog.Info("[solid_queue] job discarded", "class", class, "error", runErr.Error())
			return nil
		}
		count := rb.ToI(ee.Get(r.Key)) + 1
		ee.Set(r.Key, count)
		if count < r.Attempts {
			wait := polynomiallyLonger(count)
			if r.Wait != nil {
				wait = r.Wait(count)
			}
			return retryJob(ctx, exec, data, wait)
		}
		return runErr
	}
	return runErr
}

// retryJob ports retry_job(wait:): the same job, with its counters, due
// after wait.
func retryJob(ctx context.Context, e *Execution, data *rb.Map, wait time.Duration) error {
	var priority *int
	if p, ok := data.Get("priority").(int); ok {
		priority = &p
	}
	_, err := Enqueue(ctx, Job{
		Class: e.Class, Queue: rb.ToS(data.Get("queue_name")), Priority: priority, Arguments: e.serialArgs,
		ScheduledAt: time.Now().Add(wait), JobID: rb.ToS(data.Get("job_id")), Executions: e.Executions,
		ExceptionExecutions: e.exceptionEx, Locale: rb.ToS(data.Get("locale")), Timezone: rb.ToS(data.Get("timezone")),
	})
	return err
}

func panicError(rec any) error {
	stack := strings.Split(strings.TrimSpace(string(debug.Stack())), "\n")
	if err, ok := rec.(error); ok {
		return &Error{Class: ClassOf(err), Message: err.Error(), Backtrace: stack}
	}
	return &Error{Class: "RuntimeError", Message: fmt.Sprint(rec), Backtrace: stack}
}

// --- processes --------------------------------------------------------------

// Options configure the worker processes (config/queue.yml).
type Options struct {
	Queues          []string
	Threads         int
	PollingInterval time.Duration
	DispatchEvery   time.Duration
	BatchSize       int
	Recurring       []RecurringTask
}

// DefaultOptions are config/queue.yml's production settings.
func DefaultOptions() Options {
	return Options{Queues: []string{"*"}, Threads: 3, PollingInterval: 100 * time.Millisecond,
		DispatchEvery: time.Second, BatchSize: 500}
}

const (
	heartbeatInterval = 60 * time.Second
	aliveThreshold    = 5 * time.Minute
)

type process struct {
	id   int64
	kind string
}

func processName(kind string) string {
	var b [10]byte
	_, _ = randRead(b[:])
	return strings.ToLower(kind) + "-" + hex.EncodeToString(b[:])
}

func register(ctx context.Context, kind string, supervisor *int64, metadata *rb.Map) (*process, error) {
	host, _ := os.Hostname()
	var id int64
	err := db.Q().QueryRow(ctx, `INSERT INTO solid_queue_processes (kind, last_heartbeat_at, supervisor_id, pid, hostname, metadata, name, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $2) RETURNING id`,
		kind, now(), supervisor, os.Getpid(), host, rb.JSONGenerate(metadata), processName(kind)).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &process{id: id, kind: kind}, nil
}

// deregister destroys the row; a worker's claims go back to ready.
func (p *process) deregister(ctx context.Context) {
	if p.kind == "Worker" {
		_ = db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_ready_executions (job_id, queue_name, priority, created_at)
				SELECT j.id, j.queue_name, j.priority, $2 FROM solid_queue_claimed_executions c JOIN solid_queue_jobs j ON j.id = c.job_id
				WHERE c.process_id = $1 ON CONFLICT (job_id) DO NOTHING`, p.id, now()); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM solid_queue_claimed_executions WHERE process_id = $1`, p.id)
			return err
		})
	}
	_, _ = db.Q().Exec(ctx, `DELETE FROM solid_queue_processes WHERE id = $1`, p.id)
}

func (p *process) heartbeat(ctx context.Context) {
	_, _ = db.Q().Exec(ctx, `UPDATE solid_queue_processes SET last_heartbeat_at = $2 WHERE id = $1`, p.id, now())
}

// failClaims fails every execution a (dead) process had claimed.
func failClaims(ctx context.Context, where string, arg any, err *Error) {
	rows, qerr := db.Q().Query(ctx, `SELECT job_id FROM solid_queue_claimed_executions c WHERE `+where, arg)
	if qerr != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_ = failWith(ctx, id, err)
	}
}

// prune ports Process.prune: processes that missed their heartbeats fail
// their claims and are removed.
func prune(ctx context.Context, self int64) {
	rows, err := db.Q().Query(ctx, `SELECT id, last_heartbeat_at FROM solid_queue_processes
		WHERE last_heartbeat_at <= $1 AND id <> $2`, now().Add(-aliveThreshold), self)
	if err != nil {
		return
	}
	type dead struct {
		id int64
		at time.Time
	}
	var list []dead
	for rows.Next() {
		var d dead
		if rows.Scan(&d.id, &d.at) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		msg := "Process was found dead and pruned (last heartbeat at: " + d.at.In(rb.AppZone).Format("2006-01-02 15:04:05 -0700") + ")"
		failClaims(ctx, "c.process_id = $1", d.id, &Error{Class: "SolidQueue::Processes::ProcessPrunedError", Message: msg, Backtrace: []string{}})
		_, _ = db.Q().Exec(ctx, `DELETE FROM solid_queue_processes WHERE id = $1`, d.id)
	}
}

// failOrphaned ports fail_orphaned_executions (claims whose process row is gone).
func failOrphaned(ctx context.Context) {
	failClaims(ctx, "NOT EXISTS (SELECT 1 FROM solid_queue_processes p WHERE p.id = c.process_id) AND $1::int = 1", 1,
		&Error{Class: "SolidQueue::Processes::ProcessMissingError", Message: "The process that was running this job no longer exists", Backtrace: []string{}})
}

// --- claim / dispatch ---------------------------------------------------------

func pausedQueues(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Q().Query(ctx, `SELECT queue_name FROM solid_queue_pauses`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var q string
		if rows.Scan(&q) == nil {
			out[q] = true
		}
	}
	return out
}

// queueFilters ports QueueSelector: nil means every queue.
func queueFilters(ctx context.Context, queues []string) ([]string, bool) {
	paused := pausedQueues(ctx)
	all := false
	var exact, prefixes []string
	for _, q := range queues {
		q = strings.TrimSpace(q)
		switch {
		case q == "*":
			all = true
		case strings.HasSuffix(q, "*"):
			prefixes = append(prefixes, strings.TrimSuffix(q, "*"))
		default:
			exact = append(exact, q)
		}
	}
	if all && len(paused) == 0 {
		return nil, true
	}
	var names []string
	rows, err := db.Q().Query(ctx, `SELECT DISTINCT queue_name FROM solid_queue_ready_executions`)
	if err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) != nil {
				continue
			}
			match := all
			for _, e := range exact {
				match = match || e == n
			}
			for _, p := range prefixes {
				match = match || strings.HasPrefix(n, p)
			}
			if match && !paused[n] {
				names = append(names, n)
			}
		}
		rows.Close()
	}
	return names, false
}

// claim ports ReadyExecution.claim.
func claim(ctx context.Context, queues []string, limit int, processID int64) ([]claimed, error) {
	names, all := queueFilters(ctx, queues)
	var out []claimed
	selects := []string{""}
	if !all {
		selects = names
	}
	for _, q := range selects {
		if limit <= 0 {
			break
		}
		where, args := "", []any{limit}
		if !all {
			where, args = "WHERE queue_name = $2", []any{limit, q}
		}
		err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id, job_id FROM solid_queue_ready_executions `+where+`
				ORDER BY priority ASC, job_id ASC LIMIT $1 FOR UPDATE SKIP LOCKED`, args...)
			if err != nil {
				return err
			}
			var ids, jobIDs []int64
			for rows.Next() {
				var id, jobID int64
				if err := rows.Scan(&id, &jobID); err != nil {
					rows.Close()
					return err
				}
				ids, jobIDs = append(ids, id), append(jobIDs, jobID)
			}
			rows.Close()
			if len(ids) == 0 {
				return nil
			}
			t := now()
			for _, jobID := range jobIDs {
				if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_claimed_executions (job_id, process_id, created_at) VALUES ($1, $2, $3)`,
					jobID, processID, t); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `DELETE FROM solid_queue_ready_executions WHERE id = ANY($1)`, ids); err != nil {
				return err
			}
			jrows, err := tx.Query(ctx, `SELECT id, class_name, arguments FROM solid_queue_jobs WHERE id = ANY($1) ORDER BY id`, jobIDs)
			if err != nil {
				return err
			}
			defer jrows.Close()
			for jrows.Next() {
				var c claimed
				var args *string
				if err := jrows.Scan(&c.jobID, &c.class, &args); err != nil {
					return err
				}
				if args != nil {
					c.arguments = *args
				}
				out = append(out, c)
			}
			return jrows.Err()
		})
		if err != nil {
			return out, err
		}
		limit -= len(out)
	}
	return out, nil
}

// dispatchScheduled ports ScheduledExecution.dispatch_next_batch.
func dispatchScheduled(ctx context.Context, batch int) (int, error) {
	n := 0
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT job_id FROM solid_queue_scheduled_executions WHERE scheduled_at <= $1
			ORDER BY scheduled_at ASC, priority ASC, job_id ASC LIMIT $2 FOR UPDATE SKIP LOCKED`, now(), batch)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_ready_executions (job_id, queue_name, priority, created_at)
			SELECT id, queue_name, priority, $2 FROM solid_queue_jobs WHERE id = ANY($1) ON CONFLICT (job_id) DO NOTHING`, ids, now()); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM solid_queue_scheduled_executions WHERE job_id = ANY($1)
			AND job_id IN (SELECT job_id FROM solid_queue_ready_executions WHERE job_id = ANY($1))`, ids)
		n = int(tag.RowsAffected())
		return err
	})
	return n, err
}

// --- supervisor -------------------------------------------------------------

// Run starts the supervisor, a worker, a dispatcher and (when tasks are
// configured) a scheduler in this process, like Solid Queue's async
// supervisor, and blocks until ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	base := context.WithoutCancel(ctx)
	sup, err := register(base, "Supervisor(async)", nil, rb.NewMap())
	if err != nil {
		return err
	}
	failOrphaned(base)
	queues := strings.Join(o.Queues, ",")
	worker, err := register(base, "Worker", &sup.id, rb.M("polling_interval", o.PollingInterval.Seconds(), "queues", queues, "thread_pool_size", o.Threads))
	if err != nil {
		return err
	}
	dispatcher, err := register(base, "Dispatcher", &sup.id, rb.M("polling_interval", int(o.DispatchEvery.Seconds()), "batch_size", o.BatchSize, "concurrency_maintenance_interval", 600))
	if err != nil {
		return err
	}
	procs := []*process{sup, worker, dispatcher}
	var sched *scheduler
	if len(o.Recurring) > 0 {
		keys := make([]any, len(o.Recurring))
		for i, t := range o.Recurring {
			keys[i] = t.Key
		}
		sp, err := register(base, "Scheduler", &sup.id, rb.M("recurring_schedule", keys))
		if err != nil {
			return err
		}
		procs = append(procs, sp)
		sched = newScheduler(o.Recurring)
		if err := sched.persist(base); err != nil {
			slog.Error("[solid_queue] recurring tasks", "error", err)
		}
	}
	var wg sync.WaitGroup
	// Heartbeats and maintenance.
	wg.Add(1)
	go func() {
		defer wg.Done()
		hb := time.NewTicker(heartbeatInterval)
		maint := time.NewTicker(aliveThreshold)
		defer hb.Stop()
		defer maint.Stop()
		prune(base, sup.id)
		for {
			select {
			case <-ctx.Done():
				return
			case <-hb.C:
				for _, p := range procs {
					p.heartbeat(base)
				}
			case <-maint.C:
				prune(base, sup.id)
			}
		}
	}()
	// Dispatcher.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			n, err := dispatchScheduled(base, o.BatchSize)
			if err != nil {
				slog.Error("[solid_queue] dispatch", "error", err)
			}
			delay := o.DispatchEvery
			if n > 0 {
				delay = 0
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}()
	// Scheduler.
	if sched != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sched.run(ctx, base)
		}()
	}
	// Worker pool.
	sem := make(chan struct{}, o.Threads)
	var running sync.WaitGroup
	for {
		idle := o.Threads - len(sem)
		var delay time.Duration = o.PollingInterval
		if idle > 0 {
			jobs, err := claim(base, o.Queues, idle, worker.id)
			if err != nil {
				slog.Error("[solid_queue] claim", "error", err)
			}
			for _, c := range jobs {
				sem <- struct{}{}
				running.Add(1)
				go func(c claimed) {
					defer func() { <-sem; running.Done() }()
					if err := perform(base, c); err != nil {
						slog.Error("[solid_queue] record outcome", "job_id", c.jobID, "error", err)
					}
				}(c)
			}
		}
		select {
		case <-ctx.Done():
			waitTimeout(&running, 60*time.Second)
			wg.Wait()
			for i := len(procs) - 1; i >= 0; i-- {
				procs[i].deregister(base)
			}
			return nil
		case <-time.After(delay):
		}
	}
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
	}
}

// Drain performs every due job whose class is in classes (all when empty)
// until none is left, in id order, as the difftest's oracle runner does.
// Jobs they enqueue run in following passes; retries due later stay.
func Drain(ctx context.Context, classes []string) (int, error) {
	p, err := register(ctx, "Worker", nil, rb.M("queues", "*", "drain", true))
	if err != nil {
		return 0, err
	}
	defer p.deregister(ctx)
	if _, err := dispatchScheduled(ctx, 500); err != nil {
		return 0, err
	}
	count := 0
	for pass := 0; pass < 5; pass++ {
		where, args := "", []any{}
		if len(classes) > 0 {
			where, args = "AND j.class_name = ANY($1)", []any{classes}
		}
		rows, err := db.Q().Query(ctx, `SELECT r.job_id FROM solid_queue_ready_executions r JOIN solid_queue_jobs j ON j.id = r.job_id
			WHERE TRUE `+where+` ORDER BY r.job_id`, args...)
		if err != nil {
			return count, err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			var c claimed
			err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `DELETE FROM solid_queue_ready_executions WHERE job_id = $1`, id)
				if err != nil || tag.RowsAffected() == 0 {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_claimed_executions (job_id, process_id, created_at) VALUES ($1, $2, $3)`, id, p.id, now()); err != nil {
					return err
				}
				var args *string
				if err := tx.QueryRow(ctx, `SELECT id, class_name, arguments FROM solid_queue_jobs WHERE id = $1`, id).Scan(&c.jobID, &c.class, &args); err != nil {
					return err
				}
				if args != nil {
					c.arguments = *args
				}
				return nil
			})
			if err != nil {
				return count, err
			}
			if c.jobID == 0 {
				continue
			}
			if err := perform(ctx, c); err != nil {
				return count, err
			}
			count++
		}
		if _, err := dispatchScheduled(ctx, 500); err != nil {
			return count, err
		}
	}
	return count, nil
}
