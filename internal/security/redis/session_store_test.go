package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
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

func testStore(t *testing.T) (*SessionStore, *miniredis.Miniredis, security.Session) {
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

	session := security.Session{ID: uuid.New(), UserID: security.UserID(uuid.New()), CredentialVersion: uuid.New(), RefreshHash: hash("old"), ExpiresAt: now.Add(time.Hour)}

	if err := store.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	return store, mini, session
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func TestSessionRevocation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *SessionStore, *miniredis.Miniredis, security.Session)
	}{
		{"logout", func(t *testing.T, s *SessionStore, _ *miniredis.Miniredis, session security.Session) {
			if err := s.Revoke(context.Background(), session.UserID, session.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{"all devices", func(t *testing.T, s *SessionStore, _ *miniredis.Miniredis, session security.Session) {
			if err := s.RevokeAll(context.Background(), session.UserID); err != nil {
				t.Fatal(err)
			}
		}},
		{"expiry", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, _ security.Session) {
			m.FastForward(time.Hour)
		}},
		{"missing generation", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, session security.Session) {
			m.Del(sessionKeys(session.UserID, session.ID)[0])
		}},
		{"missing session", func(_ *testing.T, _ *SessionStore, m *miniredis.Miniredis, session security.Session) {
			m.Del(sessionKeys(session.UserID, session.ID)[1])
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := testStore(t)
			current, err := store.FindByID(context.Background(), session.UserID, session.ID)

			if err != nil || current.CredentialVersion != session.CredentialVersion {
				t.Fatalf("session = %v, error = %v", current, err)
			}

			tt.invalidate(t, store, mini, session)

			if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}

			if err := store.Rotate(context.Background(), session, hash("new")); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshReplayAndAbsoluteExpiry(t *testing.T) {
	store, mini, session := testStore(t)
	key := sessionKeys(session.UserID, session.ID)[1]
	originalTTL := mini.TTL(key)

	if err := store.Rotate(context.Background(), session, hash("next")); err != nil {
		t.Fatal(err)
	}

	current, err := store.FindByID(context.Background(), session.UserID, session.ID)

	if err != nil || current.RefreshHash != hash("next") || !current.ExpiresAt.Equal(session.ExpiresAt) || mini.TTL(key) != originalTTL {
		t.Fatalf("rotation = %v, %v", current, err)
	}

	if err := store.Rotate(context.Background(), session, hash("replay")); !errors.Is(err, security.ErrRefreshTokenReused) {
		t.Fatal(err)
	}

	if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestConcurrentRefresh(t *testing.T) {
	store, _, session := testStore(t)
	group, ctx := errgroup.WithContext(context.Background())
	var successes atomic.Int32

	for index := 0; index < 12; index++ {
		group.Go(func() error {
			err := store.Rotate(ctx, session, hash(fmt.Sprint(index)))

			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, security.ErrRefreshTokenReused) && !errors.Is(err, security.ErrSessionRevoked) {
				return err
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	if successes.Load() != 1 {
		t.Fatalf("successful rotations = %d", successes.Load())
	}

	if _, err := store.FindByID(context.Background(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestNewSessionsCannotResurrectOldGeneration(t *testing.T) {
	store, mini, old := testStore(t)
	mini.Del(sessionKeys(old.UserID, old.ID)[0])
	next := old
	next.ID = uuid.New()

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
	session.ID = uuid.New()
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
