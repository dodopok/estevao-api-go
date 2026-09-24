// Package rediscache shares Redis with the Rails application using the
// same key namespace as its RedisCacheStore ("estevao_api_v8:"), for the
// values both stacks must agree on during a transition: Rack::Attack
// counters and API-key usage counters (plain integers, never Marshal data).
package rediscache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Namespace is the Rails cache store namespace.
const Namespace = "estevao_api_v8"

// Client is nil when REDIS_URL is not configured.
var Client *redis.Client

// Open connects to url.
func Open(url string) error {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return err
	}
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 5 * time.Second
	opt.WriteTimeout = 5 * time.Second
	opt.PoolSize = 15
	Client = redis.NewClient(opt)
	return nil
}

// Key applies the Rails namespace.
func Key(k string) string { return Namespace + ":" + k }

// Increment ports RedisCacheStore#increment with expires_in: INCRBY then
// EXPIRE NX in one pipeline. ok=false when Redis is unavailable (Rails'
// failsafe returns nil).
func Increment(ctx context.Context, key string, amount int64, expiresIn time.Duration) (int64, bool) {
	if Client == nil {
		return 0, false
	}
	full := Key(key)
	var incr *redis.IntCmd
	_, err := Client.Pipelined(ctx, func(p redis.Pipeliner) error {
		incr = p.IncrBy(ctx, full, amount)
		if expiresIn > 0 {
			p.Do(ctx, "expire", full, int64(expiresIn/time.Second), "NX")
		}
		return nil
	})
	if err != nil {
		return 0, false
	}
	return incr.Val(), true
}
