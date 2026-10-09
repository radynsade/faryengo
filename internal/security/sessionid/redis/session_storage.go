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
	"github.com/radynsade/faryengo/internal/security/sessionid"
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
// Session storage
//

const keyPrefix = "faryen:security:sessionid:"

type SessionStorage struct {
	client *redislib.Client
}

var _ sessionid.SessionStorage = (*SessionStorage)(nil)

func NewSessionStorage(client *redislib.Client) (*SessionStorage, error) {
	var (
		storage *SessionStorage
		err     error
	)

	if client == nil {
		err = ErrClientNil
	} else {
		storage = &SessionStorage{client: client}
	}

	return storage, err
}

// The existence check and the write run in one script, so two creations of
// the same Session cannot overwrite each other.

func (s *SessionStorage) Create(
	ctx context.Context,
	session *security.Session,
	digest sessionid.Digest,
) error {
	var err error

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else {
		created, runErr := createScript.Run(
			ctx,
			s.client,
			[]string{sessionKey(session.ID)},
			uuid.UUID(session.UserID).String(),
			session.CredentialsVersion.String(),
			session.ExpiresAt.UnixMilli(),
			hex.EncodeToString(digest[:]),
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

func (s *SessionStorage) FindByID(
	ctx context.Context,
	id uuid.UUID,
) (*security.Session, sessionid.Digest, error) {
	var (
		session *security.Session
		digest  sessionid.Digest
		err     error
	)

	if s == nil || s.client == nil {
		err = ErrClientNil
	} else if id == uuid.Nil {
		err = security.ErrSessionNilID
	} else {
		var fields map[string]string

		fields, err = s.client.HGetAll(ctx, sessionKey(id)).Result()

		if err == nil && len(fields) == 0 {
			err = sessionid.ErrSessionNotFound
		} else if err == nil {
			session, digest, err = decodeSession(id, fields)
		}
	}

	if err != nil {
		session, digest = nil, sessionid.Digest{}
		err = fmt.Errorf("failed to find a session %s: %w", id, err)
	}

	return session, digest, err
}

// Delete

func (s *SessionStorage) Delete(
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

var createScript = redislib.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
    return 0
end
redis.call('HSET', KEYS[1], 'user_id', ARGV[1], 'credentials_version', ARGV[2], 'expires_at', ARGV[3], 'digest', ARGV[4])
redis.call('PEXPIREAT', KEYS[1], ARGV[3])
return 1
`)

func sessionKey(id uuid.UUID) string {
	return keyPrefix + id.String()
}

func decodeSession(
	id uuid.UUID,
	fields map[string]string,
) (*security.Session, sessionid.Digest, error) {
	var (
		session *security.Session
		digest  sessionid.Digest
	)

	userID, userErr := uuid.Parse(fields["user_id"])
	version, versionErr := uuid.Parse(fields["credentials_version"])
	expiresAt, expiresErr := strconv.ParseInt(fields["expires_at"], 10, 64)
	decoded, digestErr := hex.DecodeString(fields["digest"])

	err := errors.Join(userErr, versionErr, expiresErr, digestErr)

	if err == nil && len(decoded) != len(digest) {
		err = ErrRecordInvalid
	}

	if err == nil {
		copy(digest[:], decoded)

		session = &security.Session{
			ID:                 id,
			UserID:             users.UserID(userID),
			CredentialsVersion: version,
			ExpiresAt:          time.UnixMilli(expiresAt),
		}

		err = session.Validate()
	}

	if err != nil {
		session, digest = nil, sessionid.Digest{}
		err = fmt.Errorf("%w: %w", ErrRecordInvalid, err)
	}

	return session, digest, err
}
