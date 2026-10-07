package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

var rateLimitScript = redislib.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return count
`)

type RateLimiter struct{ client *redislib.Client }

func NewRateLimiter(client *redislib.Client) (*RateLimiter, error) {
	var limiter *RateLimiter
	var err error

	if client == nil {
		err = ErrNilClient
	} else {
		limiter = &RateLimiter{client: client}
	}

	return limiter, err
}

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	var allowed bool
	var err error

	if key == "" || limit < 1 || window < time.Millisecond {
		err = fmt.Errorf("invalid rate limit parameters")
	} else {
		digest := sha256.Sum256([]byte(key))
		count, countErr := rateLimitScript.Run(ctx, l.client, []string{"faryen:security:rate:" + hex.EncodeToString(digest[:])}, window.Milliseconds()).Int()

		if countErr != nil {
			err = fmt.Errorf("check authentication rate limit: %w", countErr)
		} else {
			allowed = count <= limit
		}
	}

	return allowed, err
}
