package notify

import (
	"context"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/solidqueue"
)

const broadcastJob = "BroadcastNotificationJob"

func init() {
	solidqueue.Register(broadcastJob, solidqueue.Handler{
		Queue: "default",
		Rescue: []solidqueue.Rescue{
			{Key: "[ExternalServiceUnavailable]", Match: solidqueue.Classes("ExternalServiceUnavailable"), Attempts: 3},
			{Key: "[ActiveJob::DeserializationError]", Match: solidqueue.Classes("ActiveJob::DeserializationError")},
		},
		Perform: performBroadcast,
	})
}

// EnqueueBroadcast ports BroadcastNotificationJob.perform_later(title, body,
// data): data is the permitted params hash (indifferent access), or {} when
// the request sent none.
func EnqueueBroadcast(ctx context.Context, title, body any, data *rb.Map) error {
	arg := solidqueue.StringHash(rb.NewMap())
	if data != nil {
		arg = solidqueue.IndifferentHash(data)
	}
	_, err := solidqueue.PerformLater(ctx, broadcastJob, title, body, arg)
	return err
}

// performBroadcast ports BroadcastNotificationJob#perform: the job id keys
// each user's delivery (NotificationService.with_idempotency_key).
func performBroadcast(ctx context.Context, e *solidqueue.Execution) error {
	data, _ := e.Arg(2).(*rb.Map)
	if data == nil {
		data = rb.NewMap()
	}
	Broadcast(ctx, e.Arg(0), e.Arg(1), data, rb.ToS(e.Data.Get("job_id")))
	return nil
}
