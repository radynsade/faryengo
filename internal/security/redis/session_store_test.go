package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/internal/security"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testStore(t *testing.T) (*SessionStore, *miniredis.Miniredis, security.TokenSession) {
	t.Helper()
	mini := miniredis.RunT(t)
	now := time.Now().UTC().Truncate(time.Second)
	mini.SetTime(now)
	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	session := security.TokenSession{Session: security.Session{ID: newTestUUID(t), UserID: security.UserID(newTestUUID(t)), AuthenticationSnapshotVersion: newTestUUID(t), ExpiresAt: now.Add(time.Hour)}, RefreshHash: hash("old")}

	if err := store.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	raw, err := mini.Get(sessionKeys(session.UserID, session.ID)[0])

	if err != nil {
		t.Fatal(err)
	}

	generation, err := uuid.Parse(raw)

	if err != nil || generation.Version() != 7 {
		t.Fatalf("session generation must be UUIDv7: %v, %v", generation, err)
	}

	return store, mini, session
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func testIssuance(t *testing.T, session security.TokenSession) security.TokenIssuance {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	return security.TokenIssuance{KeyID: "test", AccessID: newTestUUID(t), RefreshID: newTestUUID(t), IssuedAt: now,
		AccessExpiresAt: now.Add(5 * time.Minute), RefreshExpiresAt: session.ExpiresAt}
}

func TestSessionRevocation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *SessionStore, *miniredis.Miniredis, security.TokenSession)
	}{
		{"logout", func(t *testing.T, s *SessionStore, _ *miniredis.Miniredis, session security.TokenSession) {
			if err := s.Revoke(context.Background(), session.UserID, session.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{"all devices", func(t *testing.T, s *SessionStore, mini *miniredis.Miniredis, session security.TokenSession) {
			if err := s.RevokeAll(context.Background(), session.UserID); err != nil {
				t.Fatal(err)
			}

			raw, err := mini.Get(sessionKeys(session.UserID, session.ID)[0])

			if err != nil {
				t.Fatal(err)
			}

			generation, err := uuid.Parse(raw)

			if err != nil || generation.Version() != 7 {
				t.Fatalf("revoked session generation must be UUIDv7: %v, %v", generation, err)
			}
		}},
		{"expiry", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, _ security.TokenSession) {
			m.FastForward(time.Hour)
		}},
		{"missing generation", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, session security.TokenSession) {
			m.Del(sessionKeys(session.UserID, session.ID)[0])
		}},
		{"missing session", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, session security.TokenSession) {
			m.Del(sessionKeys(session.UserID, session.ID)[1])
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := testStore(t)
			current, err := store.FindByID(context.Background(), session.UserID, session.ID)

			if err != nil || current.AuthenticationSnapshotVersion != session.AuthenticationSnapshotVersion {
				t.Fatalf("session = %v, error = %v", current, err)
			}

			tt.invalidate(t, store, mini, session)

			if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}

			if _, err := store.Rotate(context.Background(), session, hash("new"), testIssuance(t, session)); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshReplayAndAbsoluteExpiry(t *testing.T) {
	store, mini, session := testStore(t)
	key := sessionKeys(session.UserID, session.ID)[1]
	originalTTL := mini.TTL(key)

	if _, err := store.Rotate(context.Background(), session, hash("next"), testIssuance(t, session)); err != nil {
		t.Fatal(err)
	}

	current, err := store.FindByID(context.Background(), session.UserID, session.ID)

	if err != nil || current.RefreshHash != hash("next") || !current.ExpiresAt.Equal(session.ExpiresAt) || mini.TTL(key) != originalTTL {
		t.Fatalf("rotation = %v, %v", current, err)
	}

	mini.SetTime(time.Now().UTC().Truncate(time.Second).Add(RefreshOverlapWindow))

	if _, err := store.Rotate(context.Background(), session, hash("replay"), testIssuance(t, session)); !errors.Is(err, security.ErrRefreshTokenReused) {
		t.Fatal(err)
	}

	if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestConcurrentRefresh(t *testing.T) {
	store, _, session := testStore(t)
	group, ctx := errgroup.WithContext(t.Context())
	results := make([]security.TokenIssuance, 12)

	for index := 0; index < 12; index++ {
		issuance := testIssuance(t, session)

		group.Go(func() error {
			var err error
			results[index], err = store.Rotate(ctx, session, hash(fmt.Sprint(index)), issuance)
			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	for _, result := range results {
		if result != results[0] {
			t.Fatal("concurrent rotations returned different issuance claims")
		}
	}

	if _, err := store.FindByID(t.Context(), session.UserID, session.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNewSessionsCannotResurrectOldGeneration(t *testing.T) {
	store, mini, old := testStore(t)
	mini.Del(sessionKeys(old.UserID, old.ID)[0])
	next := old
	next.ID = newTestUUID(t)

	if err := store.Create(context.Background(), next); err != nil {
		t.Fatal(err)
	}

	if _, err := store.FindByID(context.Background(), old.UserID, old.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}

	if _, err := store.FindByID(context.Background(), next.UserID, next.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSessionStoreCancellationAndOutage(t *testing.T) {
	store, mini, session := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.FindByID(ctx, session.UserID, session.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	mini.Close()

	if _, err := store.FindByID(context.Background(), session.UserID, session.ID); err == nil {
		t.Fatal("outage accepted session")
	}
}

func TestRateLimiter(t *testing.T) {
	store, mini, _ := testStore(t)
	limiter, err := NewRateLimiter(store.client)

	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, key string
		allowed   bool
	}{
		{"first", "account", true}, {"second", "account", true}, {"blocked", "account", false}, {"other account", "other", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := limiter.Allow(context.Background(), tt.key, 2, time.Minute)

			if err != nil || allowed != tt.allowed {
				t.Fatalf("allowed = %t, error = %v", allowed, err)
			}
		})
	}

	mini.FastForward(time.Minute)
	allowed, err := limiter.Allow(context.Background(), "account", 2, time.Minute)

	if err != nil || !allowed {
		t.Fatalf("allowed after expiry = %t, error = %v", allowed, err)
	}
}

func TestIdleSessionExpiry(t *testing.T) {
	store, mini, session := testStore(t)
	session.ID = newTestUUID(t)
	session.ExpiresAt = time.Now().UTC().Truncate(time.Second).Add(30 * 24 * time.Hour)

	if err := store.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	if ttl := mini.TTL(sessionKeys(session.UserID, session.ID)[1]); ttl != 7*24*time.Hour {
		t.Fatalf("idle TTL = %v", ttl)
	}

	mini.FastForward(7 * 24 * time.Hour)

	if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestRefreshOverlapDeadline(t *testing.T) {
	for _, tt := range []struct {
		name  string
		delay time.Duration
		valid bool
	}{
		{name: "inside window", delay: RefreshOverlapWindow - time.Millisecond, valid: true},
		{name: "at deadline", delay: RefreshOverlapWindow},
		{name: "past deadline", delay: RefreshOverlapWindow + time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := testStore(t)
			proposal := testIssuance(t, session)
			winner, err := store.Rotate(t.Context(), session, hash("winner"), proposal)

			if err != nil {
				t.Fatal(err)
			}

			mini.FastForward(tt.delay)
			mini.SetTime(proposal.IssuedAt.Add(tt.delay))
			key := sessionKeys(session.UserID, session.ID)[1]
			ttl := mini.TTL(key)
			deadline := mini.HGet(key, "refresh_overlap_until")
			duplicate, err := store.Rotate(t.Context(), session, hash("loser"), testIssuance(t, session))

			if tt.valid {
				if err != nil || duplicate != winner || mini.TTL(key) != ttl || mini.HGet(key, "refresh_overlap_until") != deadline || mini.HGet(key, "refresh_hash") != hash("winner") {
					t.Fatalf("duplicate changed rotation or expiration: %v", err)
				}

				mini.SetTime(proposal.IssuedAt.Add(RefreshOverlapWindow))

				if _, err := store.Rotate(t.Context(), session, hash("retry"), testIssuance(t, session)); !errors.Is(err, security.ErrRefreshTokenReused) {
					t.Fatalf("duplicate extended the overlap window: %v", err)
				}
			} else if !errors.Is(err, security.ErrRefreshTokenReused) {
				t.Fatalf("late replay = %v", err)
			}

			if _, err := store.FindByID(t.Context(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal("replay did not revoke the session")
			}
		})
	}
}

func TestRefreshOverlapRevocation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *SessionStore, *miniredis.Miniredis, *security.TokenSession)
	}{
		{name: "logout", invalidate: func(t *testing.T, store *SessionStore, _ *miniredis.Miniredis, session *security.TokenSession) {
			if err := store.Revoke(t.Context(), session.UserID, session.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "all devices", invalidate: func(t *testing.T, store *SessionStore, _ *miniredis.Miniredis, session *security.TokenSession) {
			if err := store.RevokeAll(t.Context(), session.UserID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "credential version", invalidate: func(t *testing.T, _ *SessionStore, _ *miniredis.Miniredis, session *security.TokenSession) {
			session.AuthenticationSnapshotVersion = newTestUUID(t)
		}},
		{name: "missing generation", invalidate: func(_ *testing.T, _ *SessionStore, mini *miniredis.Miniredis, session *security.TokenSession) {
			mini.Del(sessionKeys(session.UserID, session.ID)[0])
		}},
		{name: "expired session", invalidate: func(_ *testing.T, _ *SessionStore, mini *miniredis.Miniredis, session *security.TokenSession) {
			mini.SetTime(session.ExpiresAt)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := testStore(t)

			if _, err := store.Rotate(t.Context(), session, hash("winner"), testIssuance(t, session)); err != nil {
				t.Fatal(err)
			}

			tt.invalidate(t, store, mini, &session)

			if _, err := store.Rotate(t.Context(), session, hash("duplicate"), testIssuance(t, session)); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("overlap bypassed revocation: %v", err)
			}
		})
	}
}

func TestRefreshOverlapCannotRecoverOlderRotations(t *testing.T) {
	store, _, original := testStore(t)

	if _, err := store.Rotate(t.Context(), original, hash("second"), testIssuance(t, original)); err != nil {
		t.Fatal(err)
	}

	current, err := store.FindByID(t.Context(), original.UserID, original.ID)

	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Rotate(t.Context(), current, hash("third"), testIssuance(t, current)); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Rotate(t.Context(), original, hash("replay"), testIssuance(t, original)); !errors.Is(err, security.ErrRefreshTokenReused) {
		t.Fatalf("older rotation was accepted during overlap: %v", err)
	}
}

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()

	if err != nil {
		t.Fatalf("generate UUIDv7: %v", err)
	}

	return id
}
