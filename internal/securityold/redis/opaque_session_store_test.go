package redis

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
)

func TestOpaqueSessionState(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *SessionStore, security.Session, string)
	}{
		{name: "single logout", invalidate: func(t *testing.T, store *SessionStore, session security.Session, _ string) {
			if err := store.Revoke(t.Context(), session.UserID, session.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "account logout", invalidate: func(t *testing.T, store *SessionStore, session security.Session, _ string) {
			if err := store.RevokeAll(t.Context(), session.UserID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing lookup", invalidate: func(t *testing.T, store *SessionStore, _ security.Session, digest string) {
			if err := store.client.Del(t.Context(), opaqueSessionKey(digest)).Err(); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing generation", invalidate: func(t *testing.T, store *SessionStore, session security.Session, _ string) {
			if err := store.client.Del(t.Context(), sessionKeys(session.UserID, session.ID)[0]).Err(); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, token := testStore(t)
			session := token.Session
			session.ID = newTestUUID(t)
			session.ExpiresAt = time.Now().UTC().Truncate(time.Second).Add(30 * 24 * time.Hour)
			digest := hash("opaque credential")

			if err := store.CreateSession(t.Context(), session, digest); err != nil {
				t.Fatal(err)
			}

			key := sessionKeys(session.UserID, session.ID)[1]
			originalTTL := mini.TTL(key)

			if originalTTL != 30*24*time.Hour || mini.TTL(opaqueSessionKey(digest)) != originalTTL {
				t.Fatal("opaque session lifetime incorrectly depends on JWT idle policy")
			}

			mini.FastForward(8 * 24 * time.Hour)
			current, err := store.FindSession(t.Context(), digest)

			if err != nil || current.ID != session.ID || current.UserID != session.UserID || current.CredentialVersion != session.CredentialVersion || !current.ExpiresAt.Equal(session.ExpiresAt) || mini.TTL(key) != originalTTL-8*24*time.Hour {
				t.Fatalf("opaque session changed on read: %+v, %v", current, err)
			}

			if _, err := store.FindByID(t.Context(), session.UserID, session.ID); !errors.Is(err, security.ErrInvalidSession) {
				t.Fatalf("opaque session accepted as token session: %v", err)
			}

			tt.invalidate(t, store, session, digest)

			if _, err := store.FindSession(t.Context(), digest); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("invalidated session accepted: %v", err)
			}
		})
	}
}

func TestOpaqueSessionAbsoluteExpiration(t *testing.T) {
	store, mini, token := testStore(t)
	session := token.Session
	session.ID = newTestUUID(t)
	digest := hash("opaque")

	if err := store.CreateSession(t.Context(), session, digest); err != nil {
		t.Fatal(err)
	}

	// Exercise the server's absolute-expiration check while both keys still exist.
	mini.SetTime(session.ExpiresAt)

	if _, err := store.FindSession(t.Context(), digest); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatalf("expired server-side session accepted: %v", err)
	}

	mini.FastForward(time.Hour)

	if mini.Exists(opaqueSessionKey(digest)) || mini.Exists(sessionKeys(session.UserID, session.ID)[1]) {
		t.Fatal("expired session storage not removed")
	}
}

func TestOpaqueSessionLookupCannotAuthorizeJWTState(t *testing.T) {
	store, _, token := testStore(t)
	digest := hash("opaque")
	locator := uuid.UUID(token.UserID).String() + ":" + token.ID.String()

	if err := store.client.Set(t.Context(), opaqueSessionKey(digest), locator, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.FindSession(t.Context(), digest); !errors.Is(err, security.ErrInvalidSession) {
		t.Fatalf("token session accepted as opaque session: %v", err)
	}
}

func TestOpaqueSessionCreationCannotReplaceExistingState(t *testing.T) {
	store, _, token := testStore(t)
	session := token.Session
	session.ID = newTestUUID(t)
	digest := hash("opaque")

	if err := store.CreateSession(t.Context(), session, digest); err != nil {
		t.Fatal(err)
	}

	next := session
	next.ID = newTestUUID(t)

	if err := store.CreateSession(t.Context(), next, digest); !errors.Is(err, security.ErrInvalidSession) {
		t.Fatalf("existing session credential replaced: %v", err)
	}

	current, err := store.FindSession(t.Context(), digest)

	if err != nil || current.ID != session.ID || current.UserID != session.UserID || current.CredentialVersion != session.CredentialVersion || !current.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatal("collision modified the original session")
	}

	if err := store.CreateSession(t.Context(), token.Session, hash("partial")); !errors.Is(err, security.ErrInvalidSession) {
		t.Fatalf("existing device state replaced: %v", err)
	}

	if _, err := store.FindSession(t.Context(), hash("partial")); !errors.Is(err, security.ErrInvalidSession) {
		t.Fatalf("incomplete lookup authorized existing token state: %v", err)
	}
}
