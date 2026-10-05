// Package redis persists flash-message bags atomically in Redis session hashes.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/pkg/flashmsg"
)

var (
	ErrInvalidConfig  = errors.New("invalid Redis flash store configuration")
	ErrInvalidSession = errors.New("invalid flash session key")
	ErrSessionRevoked = errors.New("flash session expired or revoked")
)

// Authenticated hashes contain generation and expires fields, with expires
// expressed as Unix milliseconds. Both keys must share a Redis Cluster hash slot.
func sessionKeys(s flashmsg.Session) ([]string, string, error) {
	var keys []string
	var mode string
	var err error

	if strings.TrimSpace(s.Key) == "" {
		err = ErrInvalidSession
	} else if s.GenerationKey == "" {
		keys = []string{s.Key, s.Key}
		mode = "anonymous"
	} else {
		keys = []string{s.GenerationKey, s.Key}
		mode = "authenticated"
	}

	return keys, mode, err
}

// Store appends and consumes messages without overwriting concurrent additions.
// The caller owns the Redis client and must disable automatic retries to prevent
// replaying an ambiguous add or consume operation.
type Store struct {
	client       *redislib.Client
	application  string
	anonymousTTL time.Duration
}

func NewStore(client *redislib.Client, application string, anonymousTTL time.Duration) (*Store, error) {
	var store *Store
	var err error

	if client == nil || strings.TrimSpace(application) == "" || anonymousTTL < time.Second {
		err = ErrInvalidConfig
	} else {
		store = &Store{client: client, application: application, anonymousTTL: anonymousTTL}
	}

	return store, err
}

// Flashes share an authenticated session's existing expiration. The script never
// recreates a missing or revoked authenticated session.
const flashScript = `
local session = KEYS[2]
if ARGV[1] == 'authenticated' then
    local generation = redis.call('GET', KEYS[1])
    local fields = redis.call('HMGET', session, 'generation', 'expires')
    local now = redis.call('TIME')
    local milliseconds = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
    if not generation or fields[1] ~= generation or not fields[2] or tonumber(fields[2]) <= milliseconds then
        return nil
    end
elseif ARGV[2] == 'add' then
    redis.call('HSET', session, 'application', ARGV[5])
    redis.call('EXPIRE', session, ARGV[4])
end
local field = ARGV[6]
local data = redis.call('HGET', session, field)
if ARGV[2] == 'take' then
    if ARGV[3] ~= '' and data then
        local messages = cjson.decode(data)
        local selected = {}
        if messages[ARGV[3]] then
            selected[ARGV[3]] = messages[ARGV[3]]
            messages[ARGV[3]] = nil
        end
        if next(messages) then
            redis.call('HSET', session, field, cjson.encode(messages))
        else
            redis.call('HDEL', session, field)
        end
        return cjson.encode(selected)
    end
    redis.call('HDEL', session, field)
    return data or '{}'
end
local messages = cjson.decode(data or '{}')
local added = cjson.decode(ARGV[3])
for kind, items in pairs(added) do
    if not messages[kind] then messages[kind] = {} end
    for _, message in ipairs(items) do
        table.insert(messages[kind], message)
    end
end
redis.call('HSET', session, field, cjson.encode(messages))
return 1
`

// Add persists a message. Only an anonymous add creates or renews a session.
func (s *Store) Add(ctx context.Context, session flashmsg.Session, kind, message string) error {
	keys, mode, err := sessionKeys(session)

	if err == nil {
		bag := flashmsg.New()
		bag.Add(kind, message)
		var data []byte
		data, err = json.Marshal(bag)

		if err == nil {
			_, err = s.client.Eval(ctx, flashScript, keys, mode, "add", string(data), int(s.anonymousTTL.Seconds()), s.application, s.application+":flashes").Int()
		}

		if errors.Is(err, redislib.Nil) {
			err = ErrSessionRevoked
		}
	}

	if err != nil {
		err = fmt.Errorf("add %s flash: %w", kind, err)
	}

	return err
}

// Take atomically consumes one type, or all types when kind is empty. Reading an
// absent anonymous session returns an empty bag without creating the session.
func (s *Store) Take(ctx context.Context, session flashmsg.Session, kind string) (*flashmsg.Bag, error) {
	bag := flashmsg.New()
	keys, mode, err := sessionKeys(session)

	if err == nil {
		var data string
		data, err = s.client.Eval(ctx, flashScript, keys, mode, "take", kind, int(s.anonymousTTL.Seconds()), s.application, s.application+":flashes").Text()

		if errors.Is(err, redislib.Nil) {
			err = ErrSessionRevoked
		} else if err == nil {
			err = json.Unmarshal([]byte(data), bag)
		}
	}

	if err != nil {
		bag = nil
		err = fmt.Errorf("read flashes: %w", err)
	}

	return bag, err
}
