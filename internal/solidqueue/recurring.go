package solidqueue

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

func randRead(b []byte) (int, error) { return rand.Read(b) }

// RecurringTask is one entry of config/recurring.yml.
type RecurringTask struct {
	Key      string
	Class    string // empty for a command task
	Command  string
	Args     []any // ActiveJob-serialized
	Schedule string
	Queue    string
	Priority *int
	Desc     string
}

// Cron is a parsed schedule (Fugit::Cron).
type Cron struct {
	minutes, hours, doms, months, dows [64]bool
	domStar, dowStar                   bool
	loc                                *time.Location
}

var (
	everyMinutesRe = regexp.MustCompile(`^every (\d+) minutes?$`)
	everyHourAtRe  = regexp.MustCompile(`^every hour at minute (\d+)$`)
	everyDayAtRe   = regexp.MustCompile(`^every day at (\d{1,2})(?::(\d{2}))?\s*(am|pm)?$`)
)

// ParseSchedule ports Fugit.parse for the schedules Solid Queue accepts:
// five-field crons with an optional time zone, and the natural-language
// forms recurring.yml uses. Natural forms and zoneless crons are read in
// the process' local zone, as Fugit does.
func ParseSchedule(s string) (*Cron, error) {
	src := strings.TrimSpace(s)
	switch {
	case src == "every minute":
		return parseCron("* * * * *")
	case everyMinutesRe.MatchString(src):
		n, _ := strconv.Atoi(everyMinutesRe.FindStringSubmatch(src)[1])
		if n <= 0 || n > 59 {
			return nil, fmt.Errorf("unsupported schedule %q", s)
		}
		var mins []string
		for m := 0; m < 60; m += n {
			mins = append(mins, strconv.Itoa(m))
		}
		return parseCron(strings.Join(mins, ",") + " * * * *")
	case src == "every hour":
		return parseCron("0 * * * *")
	case everyHourAtRe.MatchString(src):
		return parseCron(everyHourAtRe.FindStringSubmatch(src)[1] + " * * * *")
	case src == "every day":
		return parseCron("0 0 * * *")
	case everyDayAtRe.MatchString(src):
		m := everyDayAtRe.FindStringSubmatch(src)
		h, _ := strconv.Atoi(m[1])
		switch m[3] {
		case "am":
			if h == 12 {
				h = 0
			}
		case "pm":
			if h != 12 {
				h += 12
			}
		}
		minute := "0"
		if m[2] != "" {
			minute = strings.TrimLeft(m[2], "0")
			if minute == "" {
				minute = "0"
			}
		}
		return parseCron(minute + " " + strconv.Itoa(h) + " * * *")
	}
	return parseCron(src)
}

func parseCron(s string) (*Cron, error) {
	f := strings.Fields(s)
	c := &Cron{loc: time.Local}
	if len(f) == 6 {
		loc, err := time.LoadLocation(f[5])
		if err != nil {
			return nil, err
		}
		c.loc = loc
		f = f[:5]
	}
	if len(f) != 5 {
		return nil, fmt.Errorf("unsupported schedule %q", s)
	}
	specs := []struct {
		dst    *[64]bool
		lo, hi int
	}{{&c.minutes, 0, 59}, {&c.hours, 0, 23}, {&c.doms, 1, 31}, {&c.months, 1, 12}, {&c.dows, 0, 7}}
	for i, sp := range specs {
		if err := parseField(f[i], sp.lo, sp.hi, sp.dst); err != nil {
			return nil, fmt.Errorf("schedule %q: %w", s, err)
		}
	}
	if c.dows[7] {
		c.dows[0] = true
	}
	c.domStar, c.dowStar = f[2] == "*", f[4] == "*"
	return c, nil
}

func parseField(field string, lo, hi int, dst *[64]bool) error {
	for _, part := range strings.Split(field, ",") {
		step := 1
		if base, s, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return fmt.Errorf("bad step %q", part)
			}
			step, part = n, base
		}
		from, to := lo, hi
		if part != "*" {
			a, b, isRange := strings.Cut(part, "-")
			x, err := strconv.Atoi(a)
			if err != nil {
				return fmt.Errorf("bad value %q", part)
			}
			from, to = x, x
			if isRange {
				y, err := strconv.Atoi(b)
				if err != nil {
					return fmt.Errorf("bad range %q", part)
				}
				to = y
			} else if step > 1 {
				to = hi
			}
		}
		if from < lo || to > hi || from > to {
			return fmt.Errorf("out of range %q", part)
		}
		for v := from; v <= to; v += step {
			dst[v] = true
		}
	}
	return nil
}

func (c *Cron) matches(t time.Time) bool {
	if !c.minutes[t.Minute()] || !c.hours[t.Hour()] || !c.months[int(t.Month())] {
		return false
	}
	dom, dow := c.doms[t.Day()], c.dows[int(t.Weekday())]
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dow
	case c.dowStar:
		return dom
	}
	return dom || dow
}

// Next is the first matching minute strictly after t.
func (c *Cron) Next(t time.Time) time.Time {
	x := t.In(c.loc).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if c.matches(x) {
			return x.UTC()
		}
		x = x.Add(time.Minute)
	}
	return time.Time{}
}

// --- scheduler ----------------------------------------------------------------

type scheduler struct {
	tasks []RecurringTask
	crons map[string]*Cron
}

func newScheduler(tasks []RecurringTask) *scheduler {
	s := &scheduler{crons: map[string]*Cron{}}
	for _, t := range tasks {
		c, err := ParseSchedule(t.Schedule)
		if err != nil {
			slog.Error("[solid_queue] recurring task skipped", "key", t.Key, "error", err)
			continue
		}
		s.tasks = append(s.tasks, t)
		s.crons[t.Key] = c
	}
	return s
}

// persist ports RecurringSchedule#persist_tasks.
func (s *scheduler) persist(ctx context.Context) error {
	keys := make([]string, len(s.tasks))
	for i, t := range s.tasks {
		keys[i] = t.Key
	}
	return db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM solid_queue_recurring_tasks WHERE static AND NOT (key = ANY($1))`, keys); err != nil {
			return err
		}
		for _, t := range s.tasks {
			args := t.Args
			if args == nil {
				args = []any{}
			}
			var class, command, queue, desc any
			if t.Class != "" {
				class = t.Class
			}
			if t.Command != "" {
				command = t.Command
			}
			if t.Queue != "" {
				queue = t.Queue
			}
			if t.Desc != "" {
				desc = t.Desc
			}
			if _, err := tx.Exec(ctx, `INSERT INTO solid_queue_recurring_tasks
				(key, schedule, command, class_name, arguments, queue_name, priority, description, static, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9, $9)
				ON CONFLICT (key) DO UPDATE SET schedule = EXCLUDED.schedule, command = EXCLUDED.command,
				  class_name = EXCLUDED.class_name, arguments = EXCLUDED.arguments, queue_name = EXCLUDED.queue_name,
				  priority = EXCLUDED.priority, description = EXCLUDED.description, static = EXCLUDED.static,
				  updated_at = CASE WHEN (solid_queue_recurring_tasks.schedule, solid_queue_recurring_tasks.command,
				    solid_queue_recurring_tasks.class_name, solid_queue_recurring_tasks.arguments, solid_queue_recurring_tasks.queue_name,
				    solid_queue_recurring_tasks.priority, solid_queue_recurring_tasks.description, solid_queue_recurring_tasks.static)
				    IS DISTINCT FROM (EXCLUDED.schedule, EXCLUDED.command, EXCLUDED.class_name, EXCLUDED.arguments,
				    EXCLUDED.queue_name, EXCLUDED.priority, EXCLUDED.description, EXCLUDED.static)
				    THEN EXCLUDED.updated_at ELSE solid_queue_recurring_tasks.updated_at END`,
				t.Key, t.Schedule, command, class, string(rb.ToJSON(args)), queue, t.Priority, desc, now()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *scheduler) run(ctx, base context.Context) {
	pending := map[string]time.Time{}
	for _, t := range s.tasks {
		pending[t.Key] = s.crons[t.Key].Next(clock.Now())
	}
	for {
		var soonest time.Time
		for _, at := range pending {
			if soonest.IsZero() || at.Before(soonest) {
				soonest = at
			}
		}
		if soonest.IsZero() {
			<-ctx.Done()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(soonest.Sub(clock.Now())):
		}
		for _, t := range s.tasks {
			at := pending[t.Key]
			if at.After(clock.Now()) {
				continue
			}
			pending[t.Key] = s.crons[t.Key].Next(at)
			if err := EnqueueRecurring(base, t, at); err != nil {
				slog.Error("[solid_queue] recurring enqueue", "key", t.Key, "error", err)
			}
		}
	}
}

// EnqueueRecurring ports RecurringTask#enqueue(at:): the job and its
// recurring execution in one transaction; a run another scheduler already
// recorded (task_key, run_at) enqueues nothing.
func EnqueueRecurring(ctx context.Context, t RecurringTask, runAt time.Time) error {
	class, args := t.Class, t.Args
	if class == "" {
		class, args = "SolidQueue::RecurringJob", []any{t.Command}
	}
	queue := t.Queue
	if queue == "" {
		queue = QueueOf(class)
		if class == "SolidQueue::RecurringJob" {
			queue = "solid_queue_recurring"
		}
	}
	err := db.Transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		enq, err := enqueueIn(ctx, tx, Job{Class: class, Queue: queue, Priority: t.Priority, Arguments: args})
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO solid_queue_recurring_executions (job_id, task_key, run_at, created_at)
			VALUES ($1, $2, $3, $4) ON CONFLICT (task_key, run_at) DO NOTHING`, enq.ID, t.Key, runAt.UTC(), now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errAlreadyRecorded
		}
		return nil
	})
	if err == errAlreadyRecorded {
		return nil
	}
	return err
}

var errAlreadyRecorded = fmt.Errorf("already recorded")

// ClearFinishedCommand is the command recurring.yml schedules through
// SolidQueue::RecurringJob; it is the only command this runner evaluates.
const ClearFinishedCommand = "SolidQueue::Job.clear_finished_in_batches(sleep_between_batches: 0.3)"

func init() {
	Register("SolidQueue::RecurringJob", Handler{Queue: "solid_queue_recurring",
		Perform: func(ctx context.Context, e *Execution) error {
			command := rb.ToS(e.Arg(0))
			if command != ClearFinishedCommand {
				return &Error{Class: "NotImplementedError", Message: "recurring command not supported by this runner: " + command}
			}
			return ClearFinished(ctx, 500, 24*time.Hour, 300*time.Millisecond)
		}})
}

// ClearFinished ports SolidQueue::Job.clear_finished_in_batches.
func ClearFinished(ctx context.Context, batch int, after, pause time.Duration) error {
	for {
		tag, err := db.Q().Exec(ctx, `DELETE FROM solid_queue_jobs WHERE id IN (SELECT id FROM solid_queue_jobs
			WHERE finished_at IS NOT NULL AND finished_at < $1 LIMIT $2)`, now().Add(-after), batch)
		if err != nil {
			return err
		}
		if pause > 0 {
			time.Sleep(pause)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
	}
}
