package rediscache

import (
	"context"
	"time"
)

// FetchJSON is Rails.cache.fetch for values only the Go stack reads. They
// are stored as JSON under "go/<key>", never beside the Marshal entries the
// Rails app keeps under the same names. As with Rails' failsafe, a Redis
// error only means computing the value again.
func FetchJSON(ctx context.Context, key string, expiresIn time.Duration, compute func() []byte) []byte {
	full := Key("go/" + key)
	if Client != nil {
		if b, err := Client.Get(ctx, full).Bytes(); err == nil {
			return b
		}
	}
	v := compute()
	if Client != nil {
		_ = Client.Set(ctx, full, v, expiresIn).Err()
	}
	return v
}
