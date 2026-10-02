//go:build integration

package redis

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/internal/security"
)

func TestIntegrationRedisSessionRotation(t *testing.T) {
	socket := os.Getenv("FARYEN_TEST_REDIS_SOCKET")

	if socket == "" {
		t.Skip("set FARYEN_TEST_REDIS_SOCKET for an isolated Redis instance")
	}

	client := redislib.NewClient(&redislib.Options{Network: "unix", Addr: socket, MaxRetries: -1, ContextTimeoutEnabled: true})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	session := security.Session{ID: uuid.New(), UserID: security.UserID(uuid.New()), CredentialVersion: uuid.New(), RefreshHash: hash("initial"), ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)}

	if err := store.Create(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	for _, key := range sessionKeys(session.UserID, session.ID) {
		t.Cleanup(func() {
			if err := client.Del(context.Background(), key).Err(); err != nil {
				t.Error(err)
			}
		})
	}

	stored, err := store.FindByID(t.Context(), session.UserID, session.ID)

	if err != nil || stored.CredentialVersion != session.CredentialVersion {
		t.Fatalf("stored = %v, error = %v", stored, err)
	}

	ttl, err := client.PTTL(t.Context(), sessionKeys(session.UserID, session.ID)[1]).Result()

	if err != nil || ttl > 7*24*time.Hour || ttl < 7*24*time.Hour-time.Minute {
		t.Fatalf("idle TTL = %v, error = %v", ttl, err)
	}

	group, ctx := errgroup.WithContext(t.Context())
	var successes atomic.Int32

	for index := 0; index < 8; index++ {
		group.Go(func() error {
			err := store.Rotate(ctx, session, hash(uuid.NewString()))

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

	if _, err := store.FindByID(t.Context(), session.UserID, session.ID); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}
