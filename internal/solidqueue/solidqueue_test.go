package solidqueue

import (
	"testing"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Next times Fugit gives for config/recurring.yml's schedules from
// 2026-09-28 13:46 UTC, read in a UTC process.
func TestScheduleMatchesFugit(t *testing.T) {
	time.Local = time.UTC
	from := time.Date(2026, 9, 28, 13, 46, 0, 0, time.UTC)
	for schedule, want := range map[string]string{
		"every hour at minute 12": "2026-09-28 14:12:00",
		"55 2 * * * UTC":          "2026-09-29 02:55:00",
		"20 3 * * * UTC":          "2026-09-29 03:20:00",
		"every day at 3am":        "2026-09-29 03:00:00",
		"every 5 minutes":         "2026-09-28 13:50:00",
		"every minute":            "2026-09-28 13:47:00",
		"*/15 9-17 * * 1-5":       "2026-09-28 14:00:00",
		"0 0 1 * 0":               "2026-10-01 00:00:00",
	} {
		c, err := ParseSchedule(schedule)
		if err != nil {
			t.Fatalf("%s: %v", schedule, err)
		}
		if got := c.Next(from).Format("2006-01-02 15:04:05"); got != want {
			t.Errorf("%s: next %s, want %s", schedule, got, want)
		}
	}
}

// The serialized ActiveJob, as Rails wrote it for
// CleanupAudioClipsJob.perform_later({"profile_status" => "legacy"}, operation_id: 5).
func TestSerializeMatchesActiveJob(t *testing.T) {
	at := time.Date(2026, 9, 28, 13, 46, 35, 820106838, time.UTC)
	j := Job{Class: "CleanupAudioClipsJob", Queue: "maintenance", JobID: "c02fb12b-5519-4a18-aca0-a844a6070833",
		Arguments: []any{StringHash(rb.M("profile_status", "legacy")), Kwargs("operation_id", 5)}}
	j.defaults(at)
	got := rb.JSONGenerate(j.Serialize(time.Date(2026, 9, 28, 13, 46, 35, 820187029, time.UTC)))
	want := `{"job_class":"CleanupAudioClipsJob","job_id":"c02fb12b-5519-4a18-aca0-a844a6070833","provider_job_id":null,"queue_name":"maintenance","priority":null,"arguments":[{"profile_status":"legacy","_aj_symbol_keys":[]},{"operation_id":5,"_aj_ruby2_keywords":["operation_id"]}],"executions":0,"exception_executions":{},"locale":"en","timezone":"America/Sao_Paulo","enqueued_at":"2026-09-28T13:46:35.820187029Z","scheduled_at":"2026-09-28T13:46:35.820106838Z"}`
	if got != want {
		t.Errorf("serialize\n got %s\nwant %s", got, want)
	}
	args, err := Deserialize(rb.M("a", 1, "b", Symbol("x"), "_aj_symbol_keys", []any{"b"}))
	if err != nil || rb.JSONGenerate(args) != `{"a":1,"b":"x"}` {
		t.Errorf("deserialize: %v %v", rb.JSONGenerate(args), err)
	}
}
