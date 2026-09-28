package rediscache

import (
	"context"
	"strings"
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

// DeleteJSON removes a FetchJSON entry (Rails.cache.delete).
func DeleteJSON(ctx context.Context, key string) {
	if Client != nil {
		_ = Client.Del(ctx, Key("go/"+key)).Err()
	}
}

// DeleteRails removes entries the Rails app caches (Rails.cache.delete, or
// delete_matched for a glob), so a write made by the Go stack does not leave
// the Rails stack serving the value it replaced. Only deletion is shared:
// the Rails entries are Marshal data the Go stack never reads.
func DeleteRails(ctx context.Context, keys ...string) {
	if Client == nil {
		return
	}
	for _, k := range keys {
		if strings.ContainsAny(k, "*?[") {
			iter := Client.Scan(ctx, 0, Key(k), 500).Iterator()
			for iter.Next(ctx) {
				_ = Client.Del(ctx, iter.Val()).Err()
			}
			continue
		}
		_ = Client.Del(ctx, Key(k)).Err()
	}
}
