package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security/emailpass"
)

//
// Errors
//

var (
	ErrClientNil     = errors.New("nil Redis client")
	ErrKeysEmpty     = errors.New("no attempt keys")
	ErrWindowInvalid = errors.New("invalid attempt window")
	ErrCountsInvalid = errors.New("invalid stored attempt counts")
)

//
// Attempt storage
//

const keyPrefix = "faryen:security:emailpass:attempts:"

type AttemptStorage struct {
	client *redislib.Client
}

var _ emailpass.AttemptStorage = (*AttemptStorage)(nil)

func NewAttemptStorage(client *redislib.Client) (*AttemptStorage, error) {
	var (
		storage *AttemptStorage
		err     error
	)

	if client == nil {
		err = ErrClientNil
	} else {
		storage = &AttemptStorage{client: client}
	}

	return storage, err
}

// Every key is counted in one script, so concurrent attempts never lose a
// count and a key never stays without its expiry.

func (s *AttemptStorage) Add(
	ctx context.Context,
	window time.Duration,
	keys ...string,
) ([]int64, error) {
	var (
		counts []int64
		err    error
	)

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if len(keys) == 0 {
		err = ErrKeysEmpty
	} else if window < time.Millisecond {
		err = ErrWindowInvalid
	} else {
		storageKeys := make([]string, len(keys))

		for index, key := range keys {
			storageKeys[index] = keyPrefix + key
		}

		counts, err = addScript.Run(ctx, s.client, storageKeys, window.Milliseconds()).Int64Slice()

		if err == nil && len(counts) != len(keys) {
			err = ErrCountsInvalid
		}
	}

	if err != nil {
		counts = nil
		err = fmt.Errorf("failed to count attempts: %w", err)
	}

	return counts, err
}

// Reset

func (s *AttemptStorage) Reset(ctx context.Context, key string) error {
	var err error

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else {
		err = s.client.Del(ctx, keyPrefix+key).Err()
	}

	if err != nil {
		err = fmt.Errorf("failed to reset attempts: %w", err)
	}

	return err
}

//
// Helpers
//

var addScript = redislib.NewScript(`
local counts = {}
for index, key in ipairs(KEYS) do
    counts[index] = redis.call('INCR', key)
    if redis.call('PTTL', key) < 0 then
        redis.call('PEXPIRE', key, ARGV[1])
    end
end
return counts
`)
