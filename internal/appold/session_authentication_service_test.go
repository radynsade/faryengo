package app

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

func sessionFixture(t *testing.T) (*SessionAuthenticationService, *JWTAuthenticationService, *authRepository, *miniredis.Miniredis) {
	t.Helper()
	tokens, repository, _, mini := authFixture(t)
	store, ok := tokens.dependencies.Sessions.(OpaqueSessionRepository)

	if !ok {
		t.Fatal("fixture storage does not support opaque sessions")
	}

	sessions, err := NewSessionAuthenticationService(tokens.authentication, store, 24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}

	return sessions, tokens, repository, mini
}

func sessionSignIn(t *testing.T, service *SessionAuthenticationService) security.SessionGrant {
	t.Helper()
	grant, err := service.SignIn(t.Context(), input.SignInInput{Email: "person@example.com", Password: "correct"})

	if err != nil {
		t.Fatal(err)
	}

	return grant
}

func TestSessionLogin(t *testing.T) {
	sessions, tokens, repository, mini := sessionFixture(t)
	first, second := sessionSignIn(t, sessions), sessionSignIn(t, sessions)
	decoded, err := base64.RawURLEncoding.DecodeString(first.ID)

	if err != nil || len(decoded) != 32 || first.ID == second.ID || strings.Contains(first.ID, ".") {
		t.Fatal("session login did not issue distinct opaque 256-bit credentials")
	}

	principal, err := sessions.Authenticate(t.Context(), first.ID)

	if err != nil || principal.UserID != repository.credentials.User.ID() || principal.Email != "person@example.com" {
		t.Fatalf("session authentication = %+v, %v", principal, err)
	}

	stored, err := sessions.sessions.FindSession(t.Context(), credentialDigest(first.ID))

	if err != nil || stored.ID.Version() != 7 || stored.ID != principal.SessionID || stored.AuthenticationSnapshotVersion != repository.credentials.Version || !stored.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("server session = %+v, %v", stored, err)
	}

	if strings.Contains(mini.Dump(), first.ID) || strings.Contains(mini.Dump(), second.ID) {
		t.Fatal("Redis stored a raw bearer session credential")
	}

	if _, err := tokens.Authenticate(t.Context(), first.ID); !errors.Is(err, security.ErrInvalidToken) {
		t.Fatalf("opaque session accepted as JWT: %v", err)
	}

	pair := signIn(t, tokens)

	for _, raw := range []string{pair.AccessToken, pair.RefreshToken} {
		if _, err := sessions.Authenticate(t.Context(), raw); !errors.Is(err, security.ErrInvalidSession) {
			t.Fatalf("JWT accepted as opaque session: %v", err)
		}
	}
}

func TestSessionInvalidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*testing.T, *SessionAuthenticationService, *authRepository, *miniredis.Miniredis, security.SessionGrant)
	}{
		{name: "logout", change: func(t *testing.T, s *SessionAuthenticationService, _ *authRepository, _ *miniredis.Miniredis, grant security.SessionGrant) {
			if err := s.SignOut(t.Context(), grant.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "absolute expiration", change: func(_ *testing.T, _ *SessionAuthenticationService, _ *authRepository, mini *miniredis.Miniredis, _ security.SessionGrant) {
			mini.FastForward(24 * time.Hour)
		}},
		{name: "password change", change: func(t *testing.T, _ *SessionAuthenticationService, repo *authRepository, _ *miniredis.Miniredis, _ security.SessionGrant) {
			repo.credentials.Version = newTestUUID(t)
		}},
		{name: "user deletion", change: func(_ *testing.T, _ *SessionAuthenticationService, repo *authRepository, _ *miniredis.Miniredis, _ security.SessionGrant) {
			repo.deleted = true
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sessions, _, repository, mini := sessionFixture(t)
			first, other := sessionSignIn(t, sessions), sessionSignIn(t, sessions)
			tt.change(t, sessions, repository, mini, first)
			principal, err := sessions.Authenticate(t.Context(), first.ID)

			if !errors.Is(err, security.ErrSessionRevoked) || principal.UserID != security.UserID(uuid.Nil) {
				t.Fatalf("invalidated session = %+v, %v", principal, err)
			}

			if tt.name == "logout" {
				if _, err := sessions.Authenticate(t.Context(), other.ID); err != nil {
					t.Fatalf("single logout revoked another device: %v", err)
				}
			}
		})
	}
}

func TestSessionLogoutWithoutRole(t *testing.T) {
	sessions, _, repo, _ := sessionFixture(t)
	grant := sessionSignIn(t, sessions)
	repo.role = nil

	if err := sessions.SignOut(t.Context(), grant.ID); err != nil {
		t.Fatalf("logout required an authorization role: %v", err)
	}

	if _, err := sessions.Authenticate(t.Context(), grant.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestSessionFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		fail func(*authRepository, *miniredis.Miniredis)
	}{
		{name: "credential storage", fail: func(repo *authRepository, _ *miniredis.Miniredis) { repo.failure = errors.New("database unavailable") }},
		{name: "session storage", fail: func(_ *authRepository, mini *miniredis.Miniredis) { mini.Close() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sessions, _, repository, mini := sessionFixture(t)
			grant := sessionSignIn(t, sessions)
			tt.fail(repository, mini)

			if _, err := sessions.Authenticate(t.Context(), grant.ID); err == nil {
				t.Fatal("storage failure authenticated a session")
			}

			failed, err := sessions.SignIn(t.Context(), input.SignInInput{Email: "person@example.com", Password: "correct"})

			if err == nil || failed.ID != "" {
				t.Fatal("storage failure issued a session credential")
			}
		})
	}
}

func TestAuthenticationMechanismsShareAuthorization(t *testing.T) {
	sessions, tokens, repository, _ := sessionFixture(t)
	grant, pair := sessionSignIn(t, sessions), signIn(t, tokens)

	for _, tt := range []struct {
		name        string
		permissions []security.Permission
		super       bool
	}{
		{name: "view users", permissions: []security.Permission{security.PermissionViewUser}},
		{name: "manage roles", permissions: []security.Permission{security.PermissionManageRole}},
		{name: "no permissions"},
		{name: "super role", super: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := repository.role.SetPermissions(tt.permissions); err != nil {
				t.Fatal(err)
			}

			repository.role.SetIsSuper(tt.super)
			browserPrincipal, browserErr := sessions.Authenticate(t.Context(), grant.ID)
			tokenPrincipal, tokenErr := tokens.Authenticate(t.Context(), pair.AccessToken)

			if browserErr != nil || tokenErr != nil || browserPrincipal.SessionID == tokenPrincipal.SessionID {
				t.Fatalf("authentication = %v, %v", browserErr, tokenErr)
			}

			// Device IDs differ; every identity and authorization field must match.
			tokenPrincipal.SessionID = browserPrincipal.SessionID

			if !reflect.DeepEqual(browserPrincipal, tokenPrincipal) {
				t.Fatalf("different principals: %+v, %+v", browserPrincipal, tokenPrincipal)
			}

			for _, permission := range append(security.AllPermissions(), security.Permission("unknown")) {
				want := security.ErrPermissionDenied

				if permission.Validate() != nil {
					want = security.ErrInvalidPermission
				} else if tt.super || slices.Contains(tt.permissions, permission) {
					want = nil
				}

				for _, principal := range []security.Principal{browserPrincipal, tokenPrincipal} {
					if err := Authorize(principal, permission); !errors.Is(err, want) {
						t.Fatalf("authorize %s = %v, want %v", permission, err, want)
					}
				}
			}
		})
	}
}

func TestAccountLogoutRevokesEveryMechanism(t *testing.T) {
	for _, method := range []string{"session", "JWT"} {
		t.Run(method, func(t *testing.T) {
			sessions, tokens, _, _ := sessionFixture(t)
			grant, pair := sessionSignIn(t, sessions), signIn(t, tokens)
			principal, err := sessions.Authenticate(t.Context(), grant.ID)

			if err != nil {
				t.Fatal(err)
			}

			if method == "session" {
				err = tokens.authentication.SignOutAll(t.Context(), principal)
			} else {
				err = tokens.SignOutAll(t.Context(), pair.AccessToken)
			}

			if err != nil {
				t.Fatal(err)
			}

			if _, err := sessions.Authenticate(t.Context(), grant.ID); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("account logout retained browser session: %v", err)
			}

			if _, err := tokens.Authenticate(t.Context(), pair.AccessToken); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("account logout retained access token: %v", err)
			}

			if _, err := tokens.Refresh(t.Context(), pair.RefreshToken); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("account logout retained refresh token: %v", err)
			}
		})
	}
}

func TestSessionCredentialValidation(t *testing.T) {
	sessions, _, _, _ := sessionFixture(t)

	for _, raw := range []string{"", "invalid", newTestUUID(t).String(), base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		t.Run(raw, func(t *testing.T) {
			principal, err := sessions.Authenticate(t.Context(), raw)

			if err == nil || principal.UserID != security.UserID(uuid.Nil) {
				t.Fatal("unknown or malformed credential authenticated")
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	grant := sessionSignIn(t, sessions)
	cancel()

	if _, err := sessions.Authenticate(ctx, grant.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation ignored: %v", err)
	}
}
