package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateDecision struct {
	Allowed    bool
	Remaining  int64
	RetryAfter time.Duration
}

type RateLimiter struct {
	client *redis.Client
	prefix string
}

func (r *RedisCoordinator) RateLimiter() *RateLimiter {
	return &RateLimiter{client: r.client, prefix: r.prefix + ":limit"}
}

func (r *RateLimiter) Allow(
	ctx context.Context,
	bucket, identity string,
	limit int64,
	window time.Duration,
) (RateDecision, error) {
	digest := sha256.Sum256([]byte(identity))
	key := fmt.Sprintf("%s:%s:%s", r.prefix, bucket, hex.EncodeToString(digest[:16]))
	result, err := fixedWindowScript.Run(
		ctx,
		r.client,
		[]string{key},
		limit,
		window.Milliseconds(),
	).Result()
	if err != nil {
		return RateDecision{}, err
	}
	values, ok := result.([]any)
	if !ok || len(values) != 3 {
		return RateDecision{}, fmt.Errorf("unexpected rate-limit response %T", result)
	}
	allowed := fmt.Sprint(values[0]) == "1"
	remaining, err := parseInt64(values[1])
	if err != nil {
		return RateDecision{}, err
	}
	retryMilliseconds, err := parseInt64(values[2])
	if err != nil {
		return RateDecision{}, err
	}
	return RateDecision{
		Allowed: allowed, Remaining: remaining,
		RetryAfter: time.Duration(retryMilliseconds) * time.Millisecond,
	}, nil
}

func parseInt64(value any) (int64, error) {
	var result int64
	_, err := fmt.Sscan(fmt.Sprint(value), &result)
	return result, err
}

var fixedWindowScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('PTTL', KEYS[1])
local limit = tonumber(ARGV[1])
local remaining = limit - current
if remaining < 0 then remaining = 0 end
if current > limit then
  return {0, remaining, ttl}
end
return {1, remaining, 0}
`)
