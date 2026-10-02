package redis

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security"
)

var ErrNilClient = errors.New("nil Redis client")

// Both keys share a user hash tag, so each script is atomic on one Redis slot.
// A missing generation never authorizes an old session, even after partial data loss.
const checkSession = `
local generation = redis.call('GET', KEYS[1])
local fields = redis.call('HMGET', KEYS[2], 'generation', 'version', 'refresh_hash', 'expires')
local now = redis.call('TIME')
local milliseconds = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if not generation or fields[1] ~= generation or not fields[4] or tonumber(fields[4]) <= milliseconds then
    return nil
end
`

var createSession = redislib.NewScript(`
local now = redis.call('TIME')
local milliseconds = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if tonumber(ARGV[3]) <= milliseconds or redis.call('EXISTS', KEYS[2]) == 1 then return 0 end
redis.call('SET', KEYS[1], ARGV[4], 'NX')
local generation = redis.call('GET', KEYS[1])
redis.call('HSET', KEYS[2], 'generation', generation, 'version', ARGV[1], 'refresh_hash', ARGV[2], 'expires', ARGV[3])
redis.call('PEXPIREAT', KEYS[2], math.min(tonumber(ARGV[3]), milliseconds + tonumber(ARGV[5])))
return 1
`)

var findSession = redislib.NewScript(checkSession + `return {fields[2], fields[3], fields[4]}`)

var rotateSession = redislib.NewScript(checkSession + `
if fields[2] ~= ARGV[1] then return 0 end
if fields[3] ~= ARGV[2] then
    redis.call('DEL', KEYS[2])
    return 2
end
redis.call('HSET', KEYS[2], 'refresh_hash', ARGV[3])
redis.call('PEXPIREAT', KEYS[2], math.min(tonumber(fields[4]), milliseconds + tonumber(ARGV[4])))
return 1
`)

// SessionStore stores no raw tokens. Refresh rotation uses a compare-and-swap;
// replay revokes the entire device session, including its access tokens.
type SessionStore struct{ client *redislib.Client }

func NewSessionStore(client *redislib.Client) (*SessionStore, error) {
	var store *SessionStore
	var err error

	if client == nil {
		err = ErrNilClient
	} else {
		store = &SessionStore{client: client}
	}

	return store, err
}

func sessionKeys(userID security.UserID, sessionID uuid.UUID) []string {
	prefix := "faryen:security:{" + uuid.UUID(userID).String() + "}"
	return []string{prefix + ":generation", prefix + ":session:" + sessionID.String()}
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validSession(session security.Session) bool {
	return uuid.UUID(session.UserID) != uuid.Nil && session.ID != uuid.Nil &&
		session.CredentialVersion != uuid.Nil && validHash(session.RefreshHash) && !session.ExpiresAt.IsZero()
}

func (s *SessionStore) Create(ctx context.Context, session security.Session) error {
	var err error

	if !validSession(session) {
		err = security.ErrInvalidSession
	} else {
		generation, randomErr := uuid.NewRandom()

		if randomErr != nil {
			err = fmt.Errorf("generate session generation: %w", randomErr)
		} else {
			created, createErr := createSession.Run(ctx, s.client, sessionKeys(session.UserID, session.ID),
				session.CredentialVersion.String(), session.RefreshHash, session.ExpiresAt.UnixMilli(), generation.String(), (7 * 24 * time.Hour).Milliseconds()).Int()

			if createErr != nil {
				err = fmt.Errorf("create session: %w", createErr)
			} else if created != 1 {
				err = security.ErrInvalidSession
			}
		}
	}

	return err
}

func (s *SessionStore) FindByID(ctx context.Context, userID security.UserID, id uuid.UUID) (security.Session, error) {
	var session security.Session
	fields, err := findSession.Run(ctx, s.client, sessionKeys(userID, id)).StringSlice()

	if errors.Is(err, redislib.Nil) {
		err = security.ErrSessionRevoked
	} else if err != nil {
		err = fmt.Errorf("load session: %w", err)
	} else if len(fields) != 3 {
		err = security.ErrInvalidSession
	} else {
		version, versionErr := uuid.Parse(fields[0])
		expiry, expiryErr := strconv.ParseInt(fields[2], 10, 64)
		candidate := security.Session{ID: id, UserID: userID, CredentialVersion: version, RefreshHash: fields[1], ExpiresAt: time.UnixMilli(expiry)}

		if versionErr != nil || expiryErr != nil || !validSession(candidate) {
			err = security.ErrInvalidSession
		} else {
			session = candidate
		}
	}

	return session, err
}

func (s *SessionStore) Rotate(ctx context.Context, session security.Session, newHash string) error {
	var err error

	if !validSession(session) || !validHash(newHash) || session.RefreshHash == newHash {
		err = security.ErrInvalidSession
	} else {
		result, rotateErr := rotateSession.Run(ctx, s.client, sessionKeys(session.UserID, session.ID), session.CredentialVersion.String(), session.RefreshHash, newHash, (7 * 24 * time.Hour).Milliseconds()).Int()

		if errors.Is(rotateErr, redislib.Nil) {
			err = security.ErrSessionRevoked
		} else if rotateErr != nil {
			err = fmt.Errorf("rotate session: %w", rotateErr)
		} else {
			switch result {
			case 0:
				err = security.ErrSessionRevoked
			case 1:
			case 2:
				err = security.ErrRefreshTokenReused
			default:
				err = security.ErrInvalidSession
			}
		}
	}

	return err
}

func (s *SessionStore) Revoke(ctx context.Context, userID security.UserID, id uuid.UUID) error {
	err := s.client.Del(ctx, sessionKeys(userID, id)[1]).Err()

	if err != nil {
		err = fmt.Errorf("revoke session: %w", err)
	}

	return err
}

func (s *SessionStore) RevokeAll(ctx context.Context, userID security.UserID) error {
	var err error
	generation, randomErr := uuid.NewRandom()

	if uuid.UUID(userID) == uuid.Nil {
		err = security.ErrInvalidSession
	} else if randomErr != nil {
		err = fmt.Errorf("generate session generation: %w", randomErr)
	} else if setErr := s.client.Set(ctx, sessionKeys(userID, uuid.Nil)[0], generation.String(), 0).Err(); setErr != nil {
		err = fmt.Errorf("revoke user sessions: %w", setErr)
	}

	return err
}
