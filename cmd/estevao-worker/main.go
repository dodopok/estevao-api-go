// Command estevao-worker performs background jobs: the Go counterpart of
// the Rails worker service (bin/jobs), on the same Solid Queue tables.
//
//	estevao-worker            run the supervisor, worker, dispatcher and scheduler
//	estevao-worker -drain     perform every due job once and exit (tests)
//	estevao-worker -enqueue -drain Class...  enqueue the classes, then drain them
//	estevao-worker -warm-calendar  warm the calendar grid and exit 0 (rails cache:warm_calendar)
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/wiring"
	"github.com/dodopok/estevao-api-go/internal/workers"
)

func main() {
	drain := flag.Bool("drain", false, "perform the due jobs (optionally of the given classes) and exit")
	enqueue := flag.Bool("enqueue", false, "enqueue each given class with no arguments before draining (tests)")
	warmCalendar := flag.Bool("warm-calendar", false, "warm the calendar grid (MONTHS_BACK, MONTHS_AHEAD) and exit; never fails a deploy")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	ctx := context.Background()
	if err := db.Open(ctx, config.Get("DATABASE_URL"), int32(config.Int("DB_MAX_CONNS", 10))); err != nil {
		logger.Error("database", "error", err)
		os.Exit(1)
	}
	if url := config.Get("REDIS_URL"); url != "" {
		if err := rediscache.Open(url); err != nil {
			logger.Error("redis", "error", err)
			os.Exit(1)
		}
	}
	wiring.Install()
	if *warmCalendar {
		// cache:warm_calendar: runs before a release takes traffic, so it
		// reports what it could not warm and never fails the deploy.
		back, badBack := envInteger("MONTHS_BACK", 3)
		ahead, badAhead := envInteger("MONTHS_AHEAD", 12)
		if bad := badBack + badAhead; bad != "" {
			logger.Warn("[cache:warm_calendar] skipped: ArgumentError: " + bad)
			return
		}
		if err := solidqueue.PerformNow(ctx, "CalendarWarmerJob", solidqueue.Kwargs("months_back", back, "months_ahead", ahead)); err != nil {
			logger.Warn("[cache:warm_calendar] skipped: " + solidqueue.ClassOf(err) + ": " + err.Error())
		}
		return
	}
	if *enqueue {
		for _, class := range flag.Args() {
			if _, err := solidqueue.PerformLater(ctx, class); err != nil {
				logger.Error("enqueue", "class", class, "error", err)
				os.Exit(1)
			}
		}
	}
	if *drain {
		n, err := solidqueue.Drain(ctx, flag.Args())
		if err != nil {
			logger.Error("drain", "error", err)
			os.Exit(1)
		}
		logger.Info("performed jobs", "count", n)
		return
	}
	run, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := solidqueue.Run(run, workers.Options()); err != nil {
		logger.Error("solid_queue", "error", err)
		os.Exit(1)
	}
}

// envInteger ports Integer(ENV.fetch(key, def)); the second value is the
// ArgumentError message when the variable is not an integer.
func envInteger(key string, def int64) (int64, string) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def, ""
	}
	n, errClass := rb.KernelInteger(raw)
	if errClass != "" {
		return 0, "invalid value for Integer(): " + rb.Inspect(raw)
	}
	return n, ""
}
