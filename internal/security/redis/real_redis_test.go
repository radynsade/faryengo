//go:build integration

package redis

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/internal/security"
)

func TestIntegrationRedisOpaqueSession(t *testing.T) {
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

	session := security.Session{ID: uuid.New(), UserID: security.UserID(uuid.New()), CredentialVersion: uuid.New(), ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)}
	digest := hash(uuid.NewString())

	for _, key := range append(sessionKeys(session.UserID, session.ID), opaqueSessionKey(digest)) {
		t.Cleanup(func() {
			if err := client.Del(context.Background(), key).Err(); err != nil {
				t.Error(err)
			}
		})
	}

	if err := store.CreateSession(t.Context(), session, digest); err != nil {
		t.Fatal(err)
	}

	stored, err := store.FindSession(t.Context(), digest)

	if err != nil || stored.ID != session.ID || stored.CredentialVersion != session.CredentialVersion || !stored.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatalf("stored opaque session = %+v, %v", stored, err)
	}

	for _, key := range []string{sessionKeys(session.UserID, session.ID)[1], opaqueSessionKey(digest)} {
		ttl, err := client.PTTL(t.Context(), key).Result()

		if err != nil || ttl > 30*24*time.Hour || ttl < 30*24*time.Hour-time.Minute {
			t.Fatalf("absolute session TTL = %v, %v", ttl, err)
		}
	}

	if err := store.Revoke(t.Context(), session.UserID, session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.FindSession(t.Context(), digest); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatalf("logout retained opaque session: %v", err)
	}
}

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

	session := security.TokenSession{Session: security.Session{ID: uuid.New(), UserID: security.UserID(uuid.New()), CredentialVersion: uuid.New(), ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)}, RefreshHash: hash("initial")}

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
	results := make([]security.TokenIssuance, 8)

	for index := 0; index < 8; index++ {
		group.Go(func() error {
			var err error
			results[index], err = store.Rotate(ctx, session, hash(uuid.NewString()), testIssuance(session))
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
