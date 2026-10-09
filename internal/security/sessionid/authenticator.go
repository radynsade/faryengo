package sessionid

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var (
	ErrAuthenticatorNil = errors.New("session ID authenticator is nil")
	ErrConfigInvalid    = errors.New("invalid session ID authenticator configuration")
	ErrSessionExpired   = errors.New("session expired")
)

//
// ID
//

const (
	secretBytes = 32
	idBytes     = len(uuid.UUID{}) + secretBytes
)

var (
	ErrIDInvalid    = errors.New("invalid session ID")
	ErrIDFormat     = errors.New("invalid format")
	ErrIDSessionNil = errors.New("session part is nil")
	ErrIDMismatch   = errors.New("secret does not match")

	idEncoding = base64.RawURLEncoding.Strict()
)

// An ID is the opaque credential handed to the client. It carries the internal
// Session ID followed by a random secret; storage keeps only the secret's
// digest, so a leaked store cannot be replayed as credentials.

type ID string

func (id ID) Validate() error {
	_, _, err := id.decode()

	return err
}

func (id ID) decode() (uuid.UUID, []byte, error) {
	var (
		sessionID uuid.UUID
		secret    []byte
		err       error
	)

	if len(id) != idEncoding.EncodedLen(idBytes) {
		err = ErrIDFormat
	} else if decoded, decodeErr := idEncoding.DecodeString(string(id)); decodeErr != nil {
		err = fmt.Errorf("%w: %w", ErrIDFormat, decodeErr)
	} else {
		sessionID = uuid.UUID(decoded[:len(uuid.UUID{})])
		secret = decoded[len(uuid.UUID{}):]

		if sessionID == uuid.Nil {
			err = ErrIDSessionNil
		}
	}

	if err != nil {
		sessionID, secret = uuid.Nil, nil
		err = fmt.Errorf("%w: %w", ErrIDInvalid, err)
	}

	return sessionID, secret, err
}

//
// Digest
//

type Digest [sha256.Size]byte

func newDigest(secret []byte) Digest {
	return sha256.Sum256(secret)
}

//
// Session storage
//

var ErrSessionNotFound = errors.New("session not found")

// Create never replaces an existing Session. FindByID reports a missing or
// expired Session as ErrSessionNotFound.

type SessionStorage interface {
	Create(ctx context.Context, session *security.Session, digest Digest) error
	FindByID(ctx context.Context, id uuid.UUID) (*security.Session, Digest, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

//
// Authenticator
//

type Authenticator struct {
	sessions    SessionStorage
	credentials users.CredentialsSnapshotRepository
}

var _ security.Authenticator[ID] = (*Authenticator)(nil)

func NewAuthenticator(
	sessions SessionStorage,
	credentials users.CredentialsSnapshotRepository,
) (*Authenticator, error) {
	var (
		authenticator *Authenticator
		err           error
	)

	if sessions == nil || credentials == nil {
		err = fmt.Errorf("create a session ID authenticator: %w", ErrConfigInvalid)
	} else {
		authenticator = &Authenticator{
			sessions:    sessions,
			credentials: credentials,
		}
	}

	return authenticator, err
}

// Issue stores a Session established by another mechanism, such as emailpass,
// and returns a new ID for it. The ID is always freshly generated, so a client
// can never choose its own.

func (a *Authenticator) Issue(
	ctx context.Context,
	session *security.Session,
) (ID, error) {
	var (
		id  ID
		err error
	)

	if a == nil || a.sessions == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else if !time.Now().Before(session.ExpiresAt) {
		err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrSessionExpired)
	} else {
		secret := make([]byte, secretBytes)
		_, _ = rand.Read(secret)

		err = a.sessions.Create(ctx, session, newDigest(secret))

		if err == nil {
			id = ID(idEncoding.EncodeToString(append(session.ID[:], secret...)))
		}
	}

	if err != nil {
		err = fmt.Errorf("issue a session ID: %w", err)
	}

	return id, err
}

// Every sign-in checks the stored Session and the User's current credentials
// version, so revocation takes effect on the next request.

func (a *Authenticator) SignIn(
	ctx context.Context,
	id ID,
) (*security.Session, error) {
	var (
		session *security.Session
		err     error
	)

	if a == nil || a.sessions == nil {
		err = ErrAuthenticatorNil
	} else {
		session, err = a.find(ctx, id)

		if err == nil {
			err = a.checkVersion(ctx, session)
		}
	}

	if err != nil {
		session = nil
		err = fmt.Errorf("sign in with a session ID: %w", err)
	}

	return session, err
}

// Signing out everywhere rotates the durable credentials version before
// deleting the current Session, so a storage failure cannot leave the other
// Sessions valid.

func (a *Authenticator) SignOut(
	ctx context.Context,
	session *security.Session,
	all bool,
) error {
	var err error

	if a == nil || a.sessions == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else {
		if all {
			err = a.credentials.RotateVersion(ctx, session.UserID)
		}

		if err == nil {
			err = a.sessions.Delete(ctx, session.ID)
		}
	}

	if err != nil {
		err = fmt.Errorf("sign out with a session ID: %w", err)
	}

	return err
}

//
// Helpers
//

func (a *Authenticator) find(
	ctx context.Context,
	id ID,
) (*security.Session, error) {
	var session *security.Session

	sessionID, secret, err := id.decode()

	if err != nil {
		err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, err)
	} else {
		var digest Digest

		session, digest, err = a.sessions.FindByID(ctx, sessionID)
		actual := newDigest(secret)

		if errors.Is(err, ErrSessionNotFound) {
			err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, err)
		} else if err != nil {
			err = fmt.Errorf("find a session %s: %w", sessionID, err)
		} else if subtle.ConstantTimeCompare(actual[:], digest[:]) != 1 {
			err = fmt.Errorf("%w: %w: %w", security.ErrCredentialsInvalid, ErrIDInvalid, ErrIDMismatch)
		} else if validationErr := session.Validate(); validationErr != nil {
			err = validationErr
		} else if session.ID != sessionID {
			err = security.ErrSessionInvalid
		} else if !time.Now().Before(session.ExpiresAt) {
			err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrSessionExpired)
		}
	}

	if err != nil {
		session = nil
	}

	return session, err
}

func (a *Authenticator) checkVersion(
	ctx context.Context,
	session *security.Session,
) error {
	version, err := a.credentials.FindVersionByUserID(ctx, session.UserID)

	if errors.Is(err, users.ErrUserNotFound) {
		err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, err)
	} else if err != nil {
		err = fmt.Errorf("check the credentials version: %w", err)
	} else if version != session.CredentialsVersion {
		err = security.ErrSessionRevoked
	}

	return err
}
