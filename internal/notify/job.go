package notify

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/dodopok/estevao-api-go/internal/jobs"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

func jobID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// EnqueueBroadcast ports BroadcastNotificationJob.perform_later: retried
// (3 executions, polynomially longer waits) while FCM is unavailable; the
// job id keys each user's delivery, as in NotificationService.broadcast.
func EnqueueBroadcast(title, body any, data *rb.Map) {
	id := jobID()
	jobs.Enqueue(jobs.Job{Name: "BroadcastNotificationJob", Attempts: 3,
		Classify: func(err error) jobs.Outcome {
			if e, ok := err.(*web.InfraError); ok && e.Class == "ExternalServiceUnavailable" {
				return jobs.Retry
			}
			return jobs.Fail
		},
		Perform: func(ctx context.Context) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					if e, ok := isUnavailable(rec); ok {
						err = e
						return
					}
					err = fmt.Errorf("%v", rec)
				}
			}()
			Broadcast(ctx, title, body, data, id)
			return nil
		}})
}
