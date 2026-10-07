package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security"
)

func opaqueSessionKey(digest string) string {
	return "faryen:security:session-lookup:" + digest
}

func validSessionState(session security.Session) bool {
	return session.Validate() == nil
}

// CreateSession uses the same generation and session hash as JWT revocation and
// authenticated flashes. The lookup contains no bearer secret, only its digest.
// Publish the lookup first: a partial write can never authorize a missing hash.
// The two writes may use different Redis Cluster slots; abandoned lookups expire
// at the absolute deadline and every lookup still checks the authoritative hash.
func (s *SessionStore) CreateSession(ctx context.Context, session security.Session, digest string) error {
	var err error

	if !validSessionState(session) || !validHash(digest) {
		err = security.ErrInvalidSession
	} else {
		generation, randomErr := uuid.NewV7()

		if randomErr != nil {
			err = fmt.Errorf("generate session generation: %w", randomErr)
		} else {
			locator := uuid.UUID(session.UserID).String() + ":" + session.ID.String()
			_, lookupErr := s.client.SetArgs(ctx, opaqueSessionKey(digest), locator, redislib.SetArgs{Mode: "NX", ExpireAt: session.ExpiresAt}).Result()

			if errors.Is(lookupErr, redislib.Nil) {
				err = security.ErrInvalidSession
			} else if lookupErr != nil {
				err = fmt.Errorf("create session lookup: %w", lookupErr)
			} else {
				created, createErr := createSession.Run(ctx, s.client, sessionKeys(session.UserID, session.ID),
					session.CredentialVersion.String(), "", session.ExpiresAt.UnixMilli(), generation.String(),
					0).Int()

				if createErr != nil {
					err = fmt.Errorf("create opaque session: %w", createErr)
				} else if created != 1 {
					err = security.ErrInvalidSession
				}
			}
		}
	}

	return err
}

func (s *SessionStore) FindSession(ctx context.Context, digest string) (security.Session, error) {
	var session security.Session
	var err error

	if !validHash(digest) {
		err = security.ErrInvalidSession
	} else {
		locator, lookupErr := s.client.Get(ctx, opaqueSessionKey(digest)).Result()

		if errors.Is(lookupErr, redislib.Nil) {
			err = security.ErrSessionRevoked
		} else if lookupErr != nil {
			err = fmt.Errorf("load session lookup: %w", lookupErr)
		} else {
			parts := strings.Split(locator, ":")

			if len(parts) != 2 {
				err = security.ErrInvalidSession
			} else {
				userID, userErr := security.NewUserID(parts[0])
				id, idErr := uuid.Parse(parts[1])

				if userErr != nil || idErr != nil || id == uuid.Nil {
					err = security.ErrInvalidSession
				} else {
					session, err = s.findOpaqueSession(ctx, userID, id)
				}
			}
		}
	}

	return session, err
}

func (s *SessionStore) findOpaqueSession(ctx context.Context, userID security.UserID, id uuid.UUID) (security.Session, error) {
	var session security.Session
	fields, err := findSession.Run(ctx, s.client, sessionKeys(userID, id)).StringSlice()

	if errors.Is(err, redislib.Nil) {
		err = security.ErrSessionRevoked
	} else if err != nil {
		err = fmt.Errorf("load opaque session: %w", err)
	} else if len(fields) != 3 || fields[1] != "" {
		// JWT refresh sessions cannot be used as opaque browser sessions.
		err = security.ErrInvalidSession
	} else {
		version, versionErr := uuid.Parse(fields[0])
		expiry, expiryErr := strconv.ParseInt(fields[2], 10, 64)
		candidate := security.Session{ID: id, UserID: userID, CredentialVersion: version, ExpiresAt: time.UnixMilli(expiry)}

		if versionErr != nil || expiryErr != nil || !validSessionState(candidate) {
			err = security.ErrInvalidSession
		} else {
			session = candidate
		}
	}

	return session, err
}
