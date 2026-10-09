package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/jwt"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var (
	ErrClientNil     = errors.New("nil Redis client")
	ErrRecordInvalid = errors.New("invalid stored session")
)

//
// Token storage
//

const keyPrefix = "faryen:security:jwt:"

type TokenStorage struct {
	client *redislib.Client
}

var _ jwt.TokenStorage = (*TokenStorage)(nil)

func NewTokenStorage(client *redislib.Client) (*TokenStorage, error) {
	var (
		storage *TokenStorage
		err     error
	)

	if client == nil {
		err = ErrClientNil
	} else {
		storage = &TokenStorage{client: client}
	}

	return storage, err
}

// The existence check and the write run in one script, so two creations of
// the same Session cannot overwrite each other.

func (s *TokenStorage) Create(
	ctx context.Context,
	session *security.Session,
	refreshID uuid.UUID,
) error {
	var err error

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else if refreshID == uuid.Nil {
		err = jwt.ErrTokenInvalid
	} else {
		created, runErr := createScript.Run(
			ctx,
			s.client,
			[]string{sessionKey(session.ID)},
			uuid.UUID(session.UserID).String(),
			session.CredentialsVersion.String(),
			session.ExpiresAt.UnixMilli(),
			refreshID.String(),
		).Int()

		if runErr != nil {
			err = runErr
		} else if created != 1 {
			err = security.ErrSessionInvalid
		}
	}

	if err != nil {
		err = fmt.Errorf("failed to create a session: %w", err)
	}

	return err
}

// Find by an ID

func (s *TokenStorage) FindByID(
	ctx context.Context,
	id uuid.UUID,
) (*security.Session, uuid.UUID, error) {
	var (
		session   *security.Session
		refreshID uuid.UUID
		err       error
	)

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if id == uuid.Nil {
		err = security.ErrSessionNilID
	} else {
		var fields map[string]string

		fields, err = s.client.HGetAll(ctx, sessionKey(id)).Result()

		if err == nil && len(fields) == 0 {
			err = jwt.ErrSessionNotFound
		} else if err == nil {
			session, refreshID, err = decodeSession(id, fields)
		}
	}

	if err != nil {
		session, refreshID = nil, uuid.Nil
		err = fmt.Errorf("failed to find a session %s: %w", id, err)
	}

	return session, refreshID, err
}

// The comparison and the replacement run in one script, so of two concurrent
// rotations with the same current ID exactly one succeeds.

func (s *TokenStorage) Rotate(
	ctx context.Context,
	id uuid.UUID,
	current uuid.UUID,
	next uuid.UUID,
) error {
	var err error

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if id == uuid.Nil {
		err = security.ErrSessionNilID
	} else if current == uuid.Nil || next == uuid.Nil || current == next {
		err = jwt.ErrTokenInvalid
	} else {
		rotated, runErr := rotateScript.Run(
			ctx,
			s.client,
			[]string{sessionKey(id)},
			current.String(),
			next.String(),
		).Int()

		if runErr != nil {
			err = runErr
		} else if rotated == rotateMissing {
			err = jwt.ErrSessionNotFound
		} else if rotated == rotateMismatch {
			err = jwt.ErrRefreshTokenMismatch
		}
	}

	if err != nil {
		err = fmt.Errorf("failed to rotate a refresh token of a session %s: %w", id, err)
	}

	return err
}

// Delete

func (s *TokenStorage) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	var err error

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if id == uuid.Nil {
		err = security.ErrSessionNilID
	} else {
		err = s.client.Del(ctx, sessionKey(id)).Err()
	}

	if err != nil {
		err = fmt.Errorf("failed to delete a session %s: %w", id, err)
	}

	return err
}

//
// Helpers
//

const (
	rotateMissing  = 0
	rotateMismatch = -1
)

var createScript = redislib.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
    return 0
end
redis.call('HSET', KEYS[1], 'user_id', ARGV[1], 'credentials_version', ARGV[2], 'expires_at', ARGV[3], 'refresh_id', ARGV[4])
redis.call('PEXPIREAT', KEYS[1], ARGV[3])
return 1
`)

var rotateScript = redislib.NewScript(`
local current = redis.call('HGET', KEYS[1], 'refresh_id')
if not current then
    return 0
end
if current ~= ARGV[1] then
    return -1
end
redis.call('HSET', KEYS[1], 'refresh_id', ARGV[2])
return 1
`)

func sessionKey(id uuid.UUID) string {
	return keyPrefix + id.String()
}

func decodeSession(
	id uuid.UUID,
	fields map[string]string,
) (*security.Session, uuid.UUID, error) {
	var session *security.Session

	userID, userErr := uuid.Parse(fields["user_id"])
	version, versionErr := uuid.Parse(fields["credentials_version"])
	expiresAt, expiresErr := strconv.ParseInt(fields["expires_at"], 10, 64)
	refreshID, refreshErr := uuid.Parse(fields["refresh_id"])

	err := errors.Join(userErr, versionErr, expiresErr, refreshErr)

	if err == nil && refreshID == uuid.Nil {
		err = jwt.ErrTokenInvalid
	}

	if err == nil {
		session = &security.Session{
			ID:                 id,
			UserID:             users.UserID(userID),
			CredentialsVersion: version,
			ExpiresAt:          time.UnixMilli(expiresAt),
		}

		err = session.Validate()
	}

	if err != nil {
		session, refreshID = nil, uuid.Nil
		err = fmt.Errorf("%w: %w", ErrRecordInvalid, err)
	}

	return session, refreshID, err
}
