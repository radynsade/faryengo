package jwt_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/jwt"
	"github.com/radynsade/faryengo/internal/security/jwt/redis"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type fixture struct {
	authenticator *jwt.Authenticator
	config        jwt.Config
	server        *miniredis.Miniredis
	versions      map[users.UserID]uuid.UUID
}

func newConfig(t *testing.T) jwt.Config {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)

	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return jwt.Config{
		PrivateKey:      key,
		KeyID:           "v1",
		Issuer:          "faryen",
		AccessAudience:  "faryen-api",
		RefreshAudience: "faryen-refresh",
		AccessTTL:       5 * time.Minute,
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	storage, err := redis.NewTokenStorage(client)

	if err != nil {
		t.Fatalf("create storage: %v", err)
	}

	f := &fixture{config: newConfig(t), server: server, versions: map[users.UserID]uuid.UUID{}}

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

	f.authenticator, err = jwt.NewAuthenticator(f.config, storage, credentials)

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

func (f *fixture) issue(t *testing.T, session *security.Session) *jwt.Tokens {
	t.Helper()

	tokens, err := f.authenticator.Issue(context.Background(), session)

	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	return tokens
}

// forge signs a token with the fixture's key, so tests can produce tokens the
// authenticator would never issue, such as already expired ones.

func (f *fixture) forge(
	t *testing.T,
	session *security.Session,
	tokenType string,
	purpose string,
	audience string,
	issuedAt time.Time,
	expiresAt time.Time,
) string {
	t.Helper()

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, jwtlib.MapClaims{
		"iss":     f.config.Issuer,
		"sub":     uuid.UUID(session.UserID).String(),
		"aud":     []string{audience},
		"exp":     expiresAt.Unix(),
		"nbf":     issuedAt.Unix(),
		"iat":     issuedAt.Unix(),
		"jti":     uuid.Must(uuid.NewV7()).String(),
		"sid":     session.ID.String(),
		"purpose": purpose,
	})

	token.Header["typ"] = tokenType
	token.Header["kid"] = f.config.KeyID

	signed, err := token.SignedString(f.config.PrivateKey)

	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	return signed
}

func (f *fixture) expiredAccess(t *testing.T, session *security.Session) string {
	t.Helper()

	issuedAt := time.Now().Add(-10 * time.Minute)

	return f.forge(t, session, "at+jwt", "access", f.config.AccessAudience, issuedAt, issuedAt.Add(5*time.Minute))
}

func TestNewAuthenticatorRejectsInvalidConfig(t *testing.T) {
	valid := newConfig(t)

	tests := []struct {
		name   string
		change func(config *jwt.Config)
	}{
		{"missing key", func(config *jwt.Config) { config.PrivateKey = nil }},
		{"corrupt key", func(config *jwt.Config) {
			config.PrivateKey = append(ed25519.PrivateKey{}, config.PrivateKey...)
			config.PrivateKey[ed25519.SeedSize] ^= 1
		}},
		{"blank key ID", func(config *jwt.Config) { config.KeyID = " " }},
		{"blank issuer", func(config *jwt.Config) { config.Issuer = "" }},
		{"blank audience", func(config *jwt.Config) { config.AccessAudience = "" }},
		{"shared audience", func(config *jwt.Config) { config.RefreshAudience = config.AccessAudience }},
		{"too short access TTL", func(config *jwt.Config) { config.AccessTTL = jwt.MinAccessTTL - 1 }},
		{"too long access TTL", func(config *jwt.Config) { config.AccessTTL = jwt.MaxAccessTTL + 1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.change(&config)

			if _, err := jwt.NewAuthenticator(config, &redis.TokenStorage{}, &mock.CredentialsRepository{}); !errors.Is(err, jwt.ErrConfigInvalid) {
				t.Fatalf("got %v, want %v", err, jwt.ErrConfigInvalid)
			}
		})
	}
}

func TestIssue(t *testing.T) {
	f := newFixture(t)
	session := f.newSession(time.Minute)
	tokens := f.issue(t, session)

	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.AccessToken == tokens.RefreshToken {
		t.Fatalf("got %+v, want two distinct tokens", tokens)
	}

	if !tokens.RefreshExpiresAt.Equal(session.ExpiresAt.Truncate(time.Second)) {
		t.Fatalf("refresh expires at %v, want the session deadline %v", tokens.RefreshExpiresAt, session.ExpiresAt)
	}

	if tokens.AccessExpiresAt.After(tokens.RefreshExpiresAt) {
		t.Fatalf("access expires at %v, after the session deadline %v", tokens.AccessExpiresAt, tokens.RefreshExpiresAt)
	}

	if _, err := f.authenticator.Issue(context.Background(), session); !errors.Is(err, security.ErrSessionInvalid) {
		t.Fatalf("issue twice: got %v, want %v", err, security.ErrSessionInvalid)
	}
}

func TestSignIn(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	session := f.newSession(time.Hour)
	tokens := f.issue(t, session)
	expired := f.expiredAccess(t, session)
	now := time.Now()

	tests := []struct {
		name        string
		credentials jwt.Credentials
		want        error
	}{
		{"access token", jwt.Credentials{AccessToken: tokens.AccessToken}, nil},
		{"refresh token", jwt.Credentials{RefreshToken: tokens.RefreshToken}, nil},
		{"both tokens", jwt.Credentials{tokens.AccessToken, tokens.RefreshToken}, nil},
		{"expired access token with a refresh token", jwt.Credentials{expired, tokens.RefreshToken}, nil},
		{"expired access token alone", jwt.Credentials{AccessToken: expired}, jwt.ErrTokenExpired},
		{"no tokens", jwt.Credentials{}, jwt.ErrTokenMissing},
		{"tampered access token with a refresh token", jwt.Credentials{
			tokens.AccessToken + "x",
			tokens.RefreshToken,
		}, jwt.ErrTokenInvalid},
		{"refresh token as an access token", jwt.Credentials{AccessToken: tokens.RefreshToken}, jwt.ErrTokenInvalid},
		{"access token as a refresh token", jwt.Credentials{RefreshToken: tokens.AccessToken}, jwt.ErrTokenInvalid},
		{"access type with the refresh purpose", jwt.Credentials{
			AccessToken: f.forge(t, session, "at+jwt", "refresh", f.config.AccessAudience, now, now.Add(time.Minute)),
		}, jwt.ErrTokenInvalid},
		{"wrong audience", jwt.Credentials{
			AccessToken: f.forge(t, session, "at+jwt", "access", "elsewhere", now, now.Add(time.Minute)),
		}, jwt.ErrTokenInvalid},
		{"expired with a wrong audience", jwt.Credentials{
			f.forge(t, session, "at+jwt", "access", "elsewhere", now.Add(-time.Hour), now.Add(-50*time.Minute)),
			tokens.RefreshToken,
		}, jwt.ErrTokenInvalid},
		{"issued in the future", jwt.Credentials{
			AccessToken: f.forge(t, session, "at+jwt", "access", f.config.AccessAudience, now.Add(time.Hour), now.Add(2*time.Hour)),
		}, jwt.ErrTokenInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := f.authenticator.SignIn(ctx, test.credentials)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want == nil && (found == nil || *found != *session) {
				t.Fatalf("got session %+v, want %+v", found, session)
			}

			if test.want != nil && (found != nil || !errors.Is(err, security.ErrCredentialsInvalid)) {
				t.Fatalf("got (%+v, %v), want an invalid credentials error", found, err)
			}
		})
	}
}

func TestSignInRejectsTokensFromAnotherKey(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	session := other.newSession(time.Hour)
	tokens := other.issue(t, session)

	f.versions[session.UserID] = session.CredentialsVersion

	_, err := f.authenticator.SignIn(context.Background(), jwt.Credentials{AccessToken: tokens.AccessToken})

	if !errors.Is(err, jwt.ErrTokenInvalid) {
		t.Fatalf("got %v, want %v", err, jwt.ErrTokenInvalid)
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
		{"storage lost", func(f *fixture, _ *security.Session) {
			f.server.FlushAll()
		}, jwt.ErrSessionNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			session := f.newSession(time.Hour)
			tokens := f.issue(t, session)

			test.revoke(f, session)

			for _, credentials := range []jwt.Credentials{
				{AccessToken: tokens.AccessToken},
				{RefreshToken: tokens.RefreshToken},
			} {
				if _, err := f.authenticator.SignIn(ctx, credentials); !errors.Is(err, test.want) {
					t.Fatalf("got %v, want %v", err, test.want)
				}
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	session := f.newSession(time.Hour)
	first := f.issue(t, session)

	second, err := f.authenticator.Refresh(ctx, first.RefreshToken)

	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if second.RefreshToken == first.RefreshToken || !second.RefreshExpiresAt.Equal(first.RefreshExpiresAt) {
		t.Fatalf("got %+v, want a new refresh token with the same deadline as %+v", second, first)
	}

	if _, err := f.authenticator.SignIn(ctx, jwt.Credentials{RefreshToken: first.RefreshToken}); !errors.Is(err, jwt.ErrTokenSuperseded) {
		t.Fatalf("sign in with the replaced token: got %v, want %v", err, jwt.ErrTokenSuperseded)
	}

	if _, err := f.authenticator.SignIn(ctx, jwt.Credentials{AccessToken: second.AccessToken}); err != nil {
		t.Fatalf("sign in with the new token: %v", err)
	}

	if _, err := f.authenticator.Refresh(ctx, first.RefreshToken); !errors.Is(err, jwt.ErrRefreshTokenReused) {
		t.Fatalf("reuse: got %v, want %v", err, jwt.ErrRefreshTokenReused)
	}

	for _, credentials := range []jwt.Credentials{
		{AccessToken: second.AccessToken},
		{RefreshToken: second.RefreshToken},
	} {
		if _, err := f.authenticator.SignIn(ctx, credentials); !errors.Is(err, security.ErrSessionRevoked) {
			t.Fatalf("after reuse: got %v, want %v", err, security.ErrSessionRevoked)
		}
	}
}

func TestRefreshRejectsAccessTokens(t *testing.T) {
	f := newFixture(t)
	tokens := f.issue(t, f.newSession(time.Hour))

	if _, err := f.authenticator.Refresh(context.Background(), tokens.AccessToken); !errors.Is(err, jwt.ErrTokenInvalid) {
		t.Fatalf("got %v, want %v", err, jwt.ErrTokenInvalid)
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
			tokens := f.issue(t, session)

			other := *session
			other.ID = uuid.Must(uuid.NewV7())
			otherTokens := f.issue(t, &other)

			if err := f.authenticator.SignOut(ctx, session, test.all); err != nil {
				t.Fatalf("sign out: %v", err)
			}

			if _, err := f.authenticator.SignIn(ctx, jwt.Credentials{AccessToken: tokens.AccessToken}); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("signed-out session: got %v, want %v", err, security.ErrSessionRevoked)
			}

			_, err := f.authenticator.SignIn(ctx, jwt.Credentials{AccessToken: otherTokens.AccessToken})

			if test.wantOtherLive != (err == nil) {
				t.Fatalf("other session: got %v, want live %v", err, test.wantOtherLive)
			}
		})
	}
}
