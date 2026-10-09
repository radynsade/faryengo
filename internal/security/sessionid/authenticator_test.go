package sessionid_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/security/sessionid/redis"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type fixture struct {
	authenticator *sessionid.Authenticator
	server        *miniredis.Miniredis
	versions      map[users.UserID]uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	storage, err := redis.NewSessionStorage(client)

	if err != nil {
		t.Fatalf("create storage: %v", err)
	}

	f := &fixture{server: server, versions: map[users.UserID]uuid.UUID{}}

	credentials := &mock.CredentialsRepository{
		FindVersionByUserIDFunc: func(_ context.Context, id users.UserID) (uuid.UUID, error) {
			version, ok := f.versions[id]

			if !ok {
				return uuid.Nil, users.ErrUserNotFound
			}

			return version, nil
		},
		RotateVersionFunc: func(_ context.Context, id users.UserID) error {
			f.versions[id] = uuid.Must(uuid.NewV7())

			return nil
		},
	}

	f.authenticator, err = sessionid.NewAuthenticator(storage, credentials)

	if err != nil {
		t.Fatalf("create authenticator: %v", err)
	}

	return f
}

func (f *fixture) newSession(ttl time.Duration) *security.Session {
	session := &security.Session{
		ID:                 uuid.Must(uuid.NewV7()),
		UserID:             users.UserID(uuid.Must(uuid.NewV7())),
		CredentialsVersion: uuid.Must(uuid.NewV7()),
		ExpiresAt:          time.Now().Add(ttl).Truncate(time.Millisecond),
	}

	f.versions[session.UserID] = session.CredentialsVersion

	return session
}

func (f *fixture) issue(t *testing.T, session *security.Session) sessionid.ID {
	t.Helper()

	id, err := f.authenticator.Issue(context.Background(), session)

	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	return id
}

func TestNewAuthenticatorRejectsMissingDependencies(t *testing.T) {
	if _, err := sessionid.NewAuthenticator(nil, nil); !errors.Is(err, sessionid.ErrConfigInvalid) {
		t.Fatalf("got %v, want %v", err, sessionid.ErrConfigInvalid)
	}
}

func TestIssueGeneratesDistinctIDs(t *testing.T) {
	f := newFixture(t)
	first := f.issue(t, f.newSession(time.Hour))
	second := f.issue(t, f.newSession(time.Hour))

	if first == second || first.Validate() != nil || second.Validate() != nil {
		t.Fatalf("got %q and %q, want two distinct valid IDs", first, second)
	}
}

func TestIssueRejectsInvalidSessions(t *testing.T) {
	f := newFixture(t)
	expired := f.newSession(-time.Second)

	tests := []struct {
		name    string
		session *security.Session
		want    error
	}{
		{"nil", nil, security.ErrSessionInvalid},
		{"invalid", &security.Session{}, security.ErrSessionInvalid},
		{"expired", expired, sessionid.ErrSessionExpired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.authenticator.Issue(context.Background(), test.session); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestSignIn(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	session := f.newSession(time.Hour)
	id := f.issue(t, session)

	decoded, err := base64.RawURLEncoding.DecodeString(string(id))

	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	decoded[len(decoded)-1] ^= 1
	forged := sessionid.ID(base64.RawURLEncoding.EncodeToString(decoded))

	tests := []struct {
		name string
		id   sessionid.ID
		want error
	}{
		{"issued ID", id, nil},
		{"empty", "", security.ErrCredentialsInvalid},
		{"malformed", sessionid.ID(strings.Repeat("!", len(id))), security.ErrCredentialsInvalid},
		{"wrong secret", forged, sessionid.ErrIDMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := f.authenticator.SignIn(ctx, test.id)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want == nil && (found == nil || found.Validate() != nil) {
				t.Fatalf("got session %+v, want a valid one", found)
			}

			if test.want != nil && found != nil {
				t.Fatalf("got session %+v, want nil", found)
			}
		})
	}

	found, err := f.authenticator.SignIn(ctx, id)

	if err != nil || *found != *session {
		t.Fatalf("got (%+v, %v), want (%+v, nil)", found, err, session)
	}
}

func TestSignInRejectsRevokedSessions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		revoke func(f *fixture, session *security.Session)
		want   error
	}{
		{"credentials changed", func(f *fixture, session *security.Session) {
			f.versions[session.UserID] = uuid.Must(uuid.NewV7())
		}, security.ErrSessionRevoked},
		{"user deleted", func(f *fixture, session *security.Session) {
			delete(f.versions, session.UserID)
		}, users.ErrUserNotFound},
		{"session expired in storage", func(f *fixture, _ *security.Session) {
			f.server.FastForward(2 * time.Hour)
		}, sessionid.ErrSessionNotFound},
		{"storage lost", func(f *fixture, _ *security.Session) {
			f.server.FlushAll()
		}, security.ErrSessionRevoked},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			session := f.newSession(time.Hour)
			id := f.issue(t, session)

			test.revoke(f, session)

			if _, err := f.authenticator.SignIn(ctx, id); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestSignInFailsClosedWhenStorageIsUnavailable(t *testing.T) {
	f := newFixture(t)
	id := f.issue(t, f.newSession(time.Hour))

	f.server.Close()

	_, err := f.authenticator.SignIn(context.Background(), id)

	if err == nil || errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrCredentialsInvalid) {
		t.Fatalf("got %v, want a storage failure", err)
	}
}

func TestSignOut(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		all           bool
		wantOtherLive bool
	}{
		{"one session", false, true},
		{"every session", true, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			session := f.newSession(time.Hour)
			id := f.issue(t, session)

			other := *session
			other.ID = uuid.Must(uuid.NewV7())
			otherID := f.issue(t, &other)

			if err := f.authenticator.SignOut(ctx, session, test.all); err != nil {
				t.Fatalf("sign out: %v", err)
			}

			if _, err := f.authenticator.SignIn(ctx, id); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("signed-out session: got %v, want %v", err, security.ErrSessionRevoked)
			}

			_, err := f.authenticator.SignIn(ctx, otherID)

			if test.wantOtherLive != (err == nil) {
				t.Fatalf("other session: got %v, want live %v", err, test.wantOtherLive)
			}
		})
	}
}
