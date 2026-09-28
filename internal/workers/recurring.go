package workers

import (
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
)

// Recurring is config/recurring.yml's production schedule.
var Recurring = []solidqueue.RecurringTask{
	{Key: "clear_solid_queue_finished_jobs", Command: solidqueue.ClearFinishedCommand, Schedule: "every hour at minute 12"},
	{Key: "cache_warmer", Class: "CacheWarmerJob", Queue: "maintenance", Schedule: "55 2 * * * UTC"},
	{Key: "calendar_warmer", Class: "CalendarWarmerJob", Queue: "maintenance", Schedule: "20 3 * * * UTC"},
	{Key: "database_cleanup", Class: "DatabaseCleanupJob", Queue: "maintenance", Schedule: "every day at 3am"},
	{Key: "reconcile_custom_rosary_publications", Class: "CustomRosaryPrayers::ReconcilePublicationJobsJob", Queue: "maintenance", Schedule: "every 5 minutes"},
	{Key: "flush_api_key_usage", Class: "FlushApiKeyUsageJob", Queue: "maintenance", Schedule: "every minute"},
}

// Options are config/queue.yml's production settings: JOB_CONCURRENCY
// processes of three threads (one pool of 3 x JOB_CONCURRENCY here), and
// the recurring schedule unless SOLID_QUEUE_SKIP_RECURRING is set.
func Options() solidqueue.Options {
	o := solidqueue.DefaultOptions()
	o.Threads *= config.Int("JOB_CONCURRENCY", 1)
	if config.Get("SOLID_QUEUE_SKIP_RECURRING") == "" {
		o.Recurring = Recurring
	}
	return o
}
