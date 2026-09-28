// Command estevao-api serves the Estêvão HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dodopok/estevao-api-go/internal/app"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rediscache"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
	"github.com/dodopok/estevao-api-go/internal/workers"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	if err := db.Open(ctx, config.Get("DATABASE_URL"), int32(config.Int("DB_MAX_CONNS", 20))); err != nil {
		logger.Error("database", "error", err)
		os.Exit(1)
	}
	if url := config.Get("REDIS_URL"); url != "" {
		if err := rediscache.Open(url); err != nil {
			logger.Error("redis", "error", err)
			os.Exit(1)
		}
	}
	srv := &http.Server{
		Addr:              ":" + config.PresenceOr("PORT", "3000"),
		Handler:           app.NewServer(logger, config.PresenceOr("PUBLIC_DIR", "public")),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// config/puma.rb: `plugin :solid_queue if ENV["SOLID_QUEUE_IN_PUMA"]` -
	// any value, even "false", runs the jobs inside the web process.
	jobsCtx, stopJobs := context.WithCancel(ctx)
	jobsDone := make(chan struct{})
	if _, set := os.LookupEnv("SOLID_QUEUE_IN_PUMA"); set {
		go func() {
			defer close(jobsDone)
			if err := solidqueue.Run(jobsCtx, workers.Options()); err != nil {
				logger.Error("solid_queue", "error", err)
			}
		}()
	} else {
		close(jobsDone)
	}
	// PPROF_ADDR (e.g. 127.0.0.1:6060) exposes the Go profiler on its own
	// listener; unset, nothing is served.
	if addr := config.Get("PPROF_ADDR"); addr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		go func() { _ = http.ListenAndServe(addr, mux) }()
	}
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	stopJobs()
	<-jobsDone
}
