package emailpass

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var (
	ErrAuthenticatorNil = errors.New("email and password authenticator is nil")
	ErrConfigInvalid    = errors.New("invalid email and password authenticator configuration")
	ErrVerificationBusy = errors.New("too many concurrent password verifications")
	ErrSessionNotStored = errors.New("email and password sessions are not stored")
)

//
// Credentials
//

var (
	ErrCredentialsEmailInvalid    = errors.New("invalid email")
	ErrCredentialsPasswordInvalid = errors.New("invalid password")
	ErrCredentialsMismatch        = errors.New("email or password does not match")
)

type Credentials struct {
	Email    users.Email
	Password string
}

// Only the byte limit applies to the password, so passwords accepted under
// earlier password rules can still sign in.

func (c Credentials) Validate() error {
	var err error

	if c.Email.Validate() != nil {
		err = ErrCredentialsEmailInvalid
	} else if c.Password == "" || len(c.Password) > users.MaxPasswordBytes {
		err = ErrCredentialsPasswordInvalid
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, err)
	}

	return err
}

//
// Authenticator
//

const (
	MinSessionTTL              = time.Second
	MaxSessionTTL              = 90 * 24 * time.Hour
	MaxConcurrentVerifications = 4
)

type Authenticator struct {
	credentials   users.CredentialsSnapshotRepository
	hasher        users.PasswordHasher
	sessionTTL    time.Duration
	dummyHash     users.PasswordHash
	verifications *semaphore.Weighted
}

var _ security.Authenticator[Credentials] = (*Authenticator)(nil)

// The constructor hashes a random password once, so an unknown email costs the
// same verification work as a wrong password.

func NewAuthenticator(
	ctx context.Context,
	credentials users.CredentialsSnapshotRepository,
	hasher users.PasswordHasher,
	sessionTTL time.Duration,
) (*Authenticator, error) {
	var (
		authenticator *Authenticator
		err           error
	)

	if credentials == nil || hasher == nil {
		err = ErrConfigInvalid
	} else if sessionTTL < MinSessionTTL || sessionTTL > MaxSessionTTL {
		err = ErrConfigInvalid
	} else {
		dummyHash, hashErr := hasher.Hash(ctx, rand.Text())

		if hashErr != nil {
			err = fmt.Errorf("hash the dummy password: %w", hashErr)
		} else {
			authenticator = &Authenticator{
				credentials:   credentials,
				hasher:        hasher,
				sessionTTL:    sessionTTL,
				dummyHash:     dummyHash,
				verifications: semaphore.NewWeighted(MaxConcurrentVerifications),
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("create an email and password authenticator: %w", err)
	}

	return authenticator, err
}

// The returned Session is not stored anywhere. Hand it to the mechanism that
// will carry it, such as sessionid or jwt, to obtain a credential for the
// client.

func (a *Authenticator) SignIn(
	ctx context.Context,
	credentials Credentials,
) (*security.Session, error) {
	var (
		session *security.Session
		err     error
	)

	if a == nil || a.credentials == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := credentials.Validate(); validationErr != nil {
		err = validationErr
	} else if !a.verifications.TryAcquire(1) {
		err = ErrVerificationBusy
	} else {
		var snapshot *users.CredentialsSnapshot

		snapshot, err = a.verify(ctx, credentials)

		a.verifications.Release(1)

		if err == nil {
			session, err = a.newSession(snapshot)
		}
	}

	if err != nil {
		session = nil
		err = fmt.Errorf("sign in with an email and password: %w", err)
	}

	return session, err
}

// Ending one device Session belongs to the mechanism that stores it, so only
// signing out everywhere is possible here: rotating the credentials version
// revokes the User's Sessions in every mechanism.

func (a *Authenticator) SignOut(
	ctx context.Context,
	session *security.Session,
	all bool,
) error {
	var err error

	if a == nil || a.credentials == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else if !all {
		err = ErrSessionNotStored
	} else {
		err = a.credentials.RotateVersion(ctx, session.UserID)
	}

	if err != nil {
		err = fmt.Errorf("sign out with an email and password: %w", err)
	}

	return err
}

//
// Helpers
//

func (a *Authenticator) verify(
	ctx context.Context,
	credentials Credentials,
) (*users.CredentialsSnapshot, error) {
	var (
		matches bool
		err     error
	)

	snapshot, findErr := a.credentials.FindByEmail(ctx, credentials.Email)

	if errors.Is(findErr, users.ErrUserNotFound) {
		_, err = a.hasher.Verify(ctx, credentials.Password, a.dummyHash)

		if err == nil {
			err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, ErrCredentialsMismatch)
		}
	} else if findErr != nil {
		err = findErr
	} else if validationErr := snapshot.Validate(); validationErr != nil {
		err = validationErr
	} else {
		matches, err = a.hasher.Verify(ctx, credentials.Password, snapshot.User.PasswordHash)

		if err == nil && !matches {
			err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, ErrCredentialsMismatch)
		}
	}

	if err != nil {
		snapshot = nil
	}

	return snapshot, err
}

func (a *Authenticator) newSession(snapshot *users.CredentialsSnapshot) (*security.Session, error) {
	var session *security.Session

	id, err := uuid.NewV7()

	if err != nil {
		err = fmt.Errorf("generate a session ID: %w", err)
	} else {
		session = &security.Session{
			ID:                 id,
			UserID:             snapshot.User.ID,
			CredentialsVersion: snapshot.Version,
			ExpiresAt:          time.Now().Add(a.sessionTTL).Truncate(time.Millisecond),
		}

		err = session.Validate()
	}

	if err != nil {
		session = nil
	}

	return session, err
}
