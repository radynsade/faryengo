package redis

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
local expiry = tonumber(ARGV[3])
if tonumber(ARGV[5]) > 0 then expiry = math.min(expiry, milliseconds + tonumber(ARGV[5])) end
redis.call('PEXPIREAT', KEYS[2], expiry)
return 1
`)

var findSession = redislib.NewScript(checkSession + `return {fields[2], fields[3], fields[4]}`)

// RefreshOverlapWindow tolerates concurrent requests carrying the previous
// token. Redis time fixes this deadline at the first successful rotation.
const RefreshOverlapWindow = 10 * time.Second

var rotateSession = redislib.NewScript(checkSession + `
if fields[2] ~= ARGV[1] then return {'revoked'} end
if fields[3] ~= ARGV[2] then
    local previous = redis.call('HMGET', KEYS[2], 'previous_refresh_hash', 'refresh_overlap_until', 'refresh_issuance')
    if previous[1] == ARGV[2] and previous[2] and tonumber(previous[2]) > milliseconds and previous[3] then
        return {'ok', previous[3]}
    end
    redis.call('DEL', KEYS[2])
    return {'reused'}
end
redis.call('HSET', KEYS[2], 'refresh_hash', ARGV[3], 'previous_refresh_hash', ARGV[2],
    'refresh_overlap_until', math.min(milliseconds + tonumber(ARGV[5]), tonumber(ARGV[7])), 'refresh_issuance', ARGV[6])
redis.call('PEXPIREAT', KEYS[2], math.min(tonumber(fields[4]), milliseconds + tonumber(ARGV[4])))
return {'ok', ARGV[6]}
`)

// SessionStore stores token hashes and public issuance claims, never raw tokens.
// Overlapping refreshes return one issuance; reuse after the window revokes the
// entire device session, including its access tokens.
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

func validSession(session security.TokenSession) bool {
	return validSessionState(session.Session) && validHash(session.RefreshHash)
}

func (s *SessionStore) Create(ctx context.Context, session security.TokenSession) error {
	var err error

	if !validSession(session) {
		err = security.ErrInvalidSession
	} else {
		generation, randomErr := uuid.NewV7()

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

func (s *SessionStore) FindByID(ctx context.Context, userID security.UserID, id uuid.UUID) (security.TokenSession, error) {
	var session security.TokenSession
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
		candidate := security.TokenSession{Session: security.Session{ID: id, UserID: userID, CredentialVersion: version, ExpiresAt: time.UnixMilli(expiry)}, RefreshHash: fields[1]}

		if versionErr != nil || expiryErr != nil || !validSession(candidate) {
			err = security.ErrInvalidSession
		} else {
			session = candidate
		}
	}

	return session, err
}

func (s *SessionStore) Rotate(ctx context.Context, session security.TokenSession, newHash string, proposal security.TokenIssuance) (security.TokenIssuance, error) {
	var issuance security.TokenIssuance
	var err error

	if !validSession(session) || !validHash(newHash) || session.RefreshHash == newHash ||
		proposal.Validate() != nil || !proposal.RefreshExpiresAt.Equal(session.ExpiresAt) {
		err = security.ErrInvalidSession
	} else {
		encoded, encodeErr := json.Marshal(proposal)

		if encodeErr != nil {
			err = fmt.Errorf("encode refresh issuance: %w", encodeErr)
		} else {
			result, rotateErr := rotateSession.Run(ctx, s.client, sessionKeys(session.UserID, session.ID),
				session.CredentialVersion.String(), session.RefreshHash, newHash, (7 * 24 * time.Hour).Milliseconds(),
				RefreshOverlapWindow.Milliseconds(), string(encoded), proposal.AccessExpiresAt.UnixMilli()).StringSlice()

			if errors.Is(rotateErr, redislib.Nil) {
				err = security.ErrSessionRevoked
			} else if rotateErr != nil {
				err = fmt.Errorf("rotate session: %w", rotateErr)
			} else if len(result) == 0 {
				err = security.ErrInvalidSession
			} else {
				switch result[0] {
				case "revoked":
					err = security.ErrSessionRevoked
				case "ok":
					if len(result) != 2 {
						err = security.ErrInvalidSession
					} else if decodeErr := json.Unmarshal([]byte(result[1]), &issuance); decodeErr != nil {
						err = fmt.Errorf("decode refresh issuance: %w", decodeErr)
					} else if issuance.Validate() != nil || !issuance.RefreshExpiresAt.Equal(session.ExpiresAt) {
						err = security.ErrInvalidSession
					}
				case "reused":
					err = security.ErrRefreshTokenReused
				default:
					err = security.ErrInvalidSession
				}
			}
		}
	}

	if err != nil {
		issuance = security.TokenIssuance{}
	}

	return issuance, err
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
	generation, randomErr := uuid.NewV7()

	if userID.Validate() != nil {
		err = security.ErrInvalidSession
	} else if randomErr != nil {
		err = fmt.Errorf("generate session generation: %w", randomErr)
	} else if setErr := s.client.Set(ctx, sessionKeys(userID, uuid.Nil)[0], generation.String(), 0).Err(); setErr != nil {
		err = fmt.Errorf("revoke user sessions: %w", setErr)
	}

	return err
}
