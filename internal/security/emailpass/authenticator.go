package emailpass

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	ErrAttemptsExceeded = errors.New("too many sign-in attempts")
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

// Client is the network address the attempt comes from. Attempts are bounded
// per Client as well as per Email; an empty Client is bounded only per Email.

type Credentials struct {
	Email    users.Email
	Password string
	Client   string
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
// Attempt storage
//

// Add counts one attempt under every key and returns each key's count, in key
// order. A key's count starts at its first attempt and is forgotten once the
// window has passed since then. Reset forgets a key's attempts at once.

type AttemptStorage interface {
	Add(ctx context.Context, window time.Duration, keys ...string) ([]int64, error)
	Reset(ctx context.Context, key string) error
}

//
// Authenticator
//

const (
	MinSessionTTL              = time.Second
	MaxSessionTTL              = 90 * 24 * time.Hour
	MaxConcurrentVerifications = 4
	MaxVerificationWait        = 2 * time.Second
	AttemptWindow              = 15 * time.Minute
	MaxAttemptsPerEmail        = 10
	MaxAttemptsPerClient       = 100
)

type Authenticator struct {
	credentials   users.CredentialsSnapshotRepository
	hasher        users.PasswordHasher
	attempts      AttemptStorage
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
	attempts AttemptStorage,
	sessionTTL time.Duration,
) (*Authenticator, error) {
	var (
		authenticator *Authenticator
		err           error
	)

	if credentials == nil || hasher == nil || attempts == nil {
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
				attempts:      attempts,
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
//
// Every attempt counts against its Email and its Client before the password
// is verified, whether it succeeds or not, and one over either limit is
// refused with ErrAttemptsExceeded until its window passes. A successful
// sign-in forgets its Email's attempts. An attempt that cannot be counted is
// refused, since the limits cannot be enforced without their storage. An
// attempt that finds no free verification within MaxVerificationWait is
// refused with ErrVerificationBusy.

func (a *Authenticator) SignIn(
	ctx context.Context,
	credentials Credentials,
) (*security.Session, error) {
	var (
		session  *security.Session
		snapshot *users.CredentialsSnapshot
		err      error
	)

	if a == nil || a.credentials == nil || a.attempts == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := credentials.Validate(); validationErr != nil {
		err = validationErr
	} else {
		err = a.countAttempt(ctx, credentials)
	}

	if err == nil {
		snapshot, err = a.verifyBounded(ctx, credentials)
	}

	if err == nil {
		session, err = a.newSession(snapshot)
	}

	// A count that cannot be forgotten only lasts until its window passes, so
	// the sign-in still succeeds.
	if err == nil {
		_ = a.attempts.Reset(ctx, emailAttemptKey(credentials.Email))
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

func (a *Authenticator) countAttempt(ctx context.Context, credentials Credentials) error {
	keys := []string{emailAttemptKey(credentials.Email)}
	limits := []int64{MaxAttemptsPerEmail}

	if credentials.Client != "" {
		keys = append(keys, clientAttemptKey(credentials.Client))
		limits = append(limits, MaxAttemptsPerClient)
	}

	counts, err := a.attempts.Add(ctx, AttemptWindow, keys...)

	if err != nil {
		err = fmt.Errorf("count the sign-in attempt: %w", err)
	} else if len(counts) != len(keys) {
		err = fmt.Errorf("count the sign-in attempt: got %d counts for %d keys", len(counts), len(keys))
	}

	for index := 0; err == nil && index < len(keys); index++ {
		if counts[index] > limits[index] {
			err = ErrAttemptsExceeded
		}
	}

	return err
}

func (a *Authenticator) verifyBounded(
	ctx context.Context,
	credentials Credentials,
) (*users.CredentialsSnapshot, error) {
	var snapshot *users.CredentialsSnapshot

	waitCtx, cancel := context.WithTimeout(ctx, MaxVerificationWait)
	err := a.verifications.Acquire(waitCtx, 1)

	cancel()

	if err != nil {
		err = ErrVerificationBusy
	} else {
		snapshot, err = a.verify(ctx, credentials)

		a.verifications.Release(1)
	}

	return snapshot, err
}

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

// Attempt keys carry digests rather than the addresses themselves, so the
// attempt storage holds no emails or client addresses. Emails differing only
// in case share one count.

func emailAttemptKey(email users.Email) string {
	return attemptKey("email", strings.ToLower(string(email)))
}

func clientAttemptKey(client string) string {
	return attemptKey("client", client)
}

func attemptKey(kind string, value string) string {
	digest := sha256.Sum256([]byte(value))

	return kind + ":" + hex.EncodeToString(digest[:])
}
