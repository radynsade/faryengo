package redis

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/pkg/flashmsg"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func flashFixture(t *testing.T) (*Store, *miniredis.Miniredis, flashmsg.Session) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		if err := client.Close(); err != nil && !errors.Is(err, redislib.ErrClosed) {
			t.Error(err)
		}
	})
	store, err := NewStore(client, "admin", 15*time.Minute)

	if err != nil {
		t.Fatal(err)
	}

	session := flashmsg.Session{Key: "sessions:{user}:device", GenerationKey: "sessions:{user}:generation"}

	if err := mini.Set(session.GenerationKey, "current"); err != nil {
		t.Fatal(err)
	}

	mini.HSet(session.Key, "generation", "current", "expires", strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10))
	mini.SetTTL(session.Key, time.Hour)
	return store, mini, session
}

func TestFlashSessionStorage(t *testing.T) {
	store, mini, session := flashFixture(t)

	originalTTL := mini.TTL(session.Key)
	mini.HSet(session.Key, "another-app:flashes", `{"info":["Keep this"]}`)

	for _, message := range []struct{ kind, text string }{
		{flashmsg.Success, "First"}, {flashmsg.Error, "Invalid"}, {flashmsg.Success, "Second"},
	} {
		if err := store.Add(t.Context(), session, message.kind, message.text); err != nil {
			t.Fatal(err)
		}
	}

	bag, err := store.Take(t.Context(), session, flashmsg.Error)

	if err != nil || !slices.Equal(bag.Get(flashmsg.Error), []string{"Invalid"}) || len(bag.Peek(flashmsg.Success)) != 0 {
		t.Fatalf("typed consumption = %v, %v", bag, err)
	}

	bag, err = store.Take(t.Context(), session, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Success), []string{"First", "Second"}) {
		t.Fatalf("remaining successes = %v, %v", bag, err)
	}

	bag, err = store.Take(t.Context(), session, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatalf("messages replayed: %v, %v", bag, err)
	}

	if mini.TTL(session.Key) != originalTTL || mini.HGet(session.Key, "another-app:flashes") != `{"info":["Keep this"]}` {
		t.Fatal("flashes changed session expiration or another application's data")
	}

	if mini.HGet(session.Key, "generation") != "current" || mini.HGet(session.Key, "expires") == "" {
		t.Fatal("flash consumption damaged the authentication session")
	}
}

func TestConcurrentFlashStorage(t *testing.T) {
	store, _, session := flashFixture(t)
	group, ctx := errgroup.WithContext(t.Context())

	for writer := range 10 {
		group.Go(func() error {
			var err error

			for message := range 10 {
				err = store.Add(ctx, session, flashmsg.Success, fmt.Sprintf("%d:%d", writer, message))

				if err != nil {
					break
				}
			}

			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	var consumed atomic.Int64
	group, ctx = errgroup.WithContext(t.Context())

	for range 10 {
		group.Go(func() error {
			bag, err := store.Take(ctx, session, "")

			if err == nil {
				messages := bag.Get(flashmsg.Success)
				consumed.Add(int64(len(messages)))

				if len(messages) != 0 && len(messages) != 100 {
					err = fmt.Errorf("non-atomic consumption returned %d messages", len(messages))
				}

				slices.Sort(messages)

				for i := 1; i < len(messages); i++ {
					if messages[i] == messages[i-1] {
						err = fmt.Errorf("duplicate flash %q", messages[i])
						break
					}
				}
			}

			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	if consumed.Load() != 100 {
		t.Fatalf("concurrent requests consumed %d flashes, want 100", consumed.Load())
	}
}

func TestFlashStorageCancellation(t *testing.T) {
	store, _, session := flashFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := store.Add(ctx, session, flashmsg.Success, "Cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled add = %v", err)
	}

	if _, err := store.Take(ctx, session, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}

	bag, err := store.Take(t.Context(), session, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatal("cancelled write left a flash")
	}
}

func TestFlashSessionIsolation(t *testing.T) {
	store, mini, first := flashFixture(t)
	second := flashmsg.Session{Key: "sessions:{user}:other-device", GenerationKey: first.GenerationKey}
	mini.HSet(second.Key, "generation", "current", "expires", strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10))
	mini.SetTTL(second.Key, time.Hour)

	if err := store.Add(t.Context(), first, flashmsg.Success, "First session only"); err != nil {
		t.Fatal(err)
	}

	bag, err := store.Take(t.Context(), second, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatal("one device read another device's flashes")
	}

	bag, err = store.Take(t.Context(), first, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Success), []string{"First session only"}) {
		t.Fatal("the other device consumed the first device's flashes")
	}
}

func TestFlashSessionRevocation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		revoke func(*miniredis.Miniredis, flashmsg.Session)
	}{
		{name: "sign-out", revoke: func(mini *miniredis.Miniredis, session flashmsg.Session) { mini.Del(session.Key) }},
		{name: "all devices", revoke: func(mini *miniredis.Miniredis, session flashmsg.Session) { mini.HSet(session.Key, "generation", "old") }},
		{name: "expired key", revoke: func(mini *miniredis.Miniredis, _ flashmsg.Session) { mini.FastForward(time.Hour) }},
		{name: "absolute expiry", revoke: func(mini *miniredis.Miniredis, session flashmsg.Session) { mini.HSet(session.Key, "expires", "1") }},
		{name: "missing generation", revoke: func(mini *miniredis.Miniredis, session flashmsg.Session) { mini.Del(session.GenerationKey) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := flashFixture(t)

			if err := store.Add(t.Context(), session, flashmsg.Success, "Queued"); err != nil {
				t.Fatal(err)
			}

			tt.revoke(mini, session)
			existed := mini.Exists(session.Key)
			ttl := mini.TTL(session.Key)

			if err := store.Add(t.Context(), session, flashmsg.Error, "Do not resurrect"); !errors.Is(err, ErrSessionRevoked) {
				t.Fatalf("add to revoked session = %v", err)
			}

			if bag, err := store.Take(t.Context(), session, ""); bag != nil || !errors.Is(err, ErrSessionRevoked) {
				t.Fatalf("read revoked session = %v, %v", bag, err)
			}

			if mini.Exists(session.Key) != existed || mini.TTL(session.Key) != ttl {
				t.Fatal("flash access resurrected or prolonged a revoked session")
			}
		})
	}
}

func TestAnonymousFlashSession(t *testing.T) {
	store, mini, _ := flashFixture(t)
	session := flashmsg.Session{Key: "anonymous:{guest}"}
	bag, err := store.Take(t.Context(), session, "")

	if err != nil || len(bag.All()) != 0 || mini.Exists(session.Key) {
		t.Fatal("reading flashes created an anonymous session")
	}

	if err := store.Add(t.Context(), session, flashmsg.Error, "Sign in again"); err != nil {
		t.Fatal(err)
	}

	if mini.TTL(session.Key) != 15*time.Minute || mini.HGet(session.Key, "application") != "admin" {
		t.Fatal("anonymous session lifetime or application differs")
	}

	mini.FastForward(time.Minute)
	bag, err = store.Take(t.Context(), session, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Error), []string{"Sign in again"}) || mini.TTL(session.Key) != 14*time.Minute {
		t.Fatalf("anonymous consumption = %v, %v", bag, err)
	}

	mini.FastForward(14 * time.Minute)
	bag, err = store.Take(t.Context(), session, "")

	if err != nil || len(bag.All()) != 0 || mini.Exists(session.Key) {
		t.Fatal("reading an expired guest session recreated it")
	}
}

func TestStoreConfiguration(t *testing.T) {
	store, mini, _ := flashFixture(t)

	for _, tt := range []struct {
		name        string
		client      *redislib.Client
		application string
		ttl         time.Duration
	}{
		{name: "nil client", application: "admin", ttl: time.Minute},
		{name: "empty application", client: store.client, ttl: time.Minute},
		{name: "blank application", client: store.client, application: " ", ttl: time.Minute},
		{name: "zero lifetime", client: store.client, application: "admin"},
		{name: "negative lifetime", client: store.client, application: "admin", ttl: -time.Minute},
		{name: "subsecond lifetime", client: store.client, application: "admin", ttl: time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := NewStore(tt.client, tt.application, tt.ttl); got != nil || !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("invalid configuration = %v, %v", got, err)
			}
		})
	}

	other, err := NewStore(store.client, "another-app", time.Minute)

	if err != nil {
		t.Fatal(err)
	}

	session := flashmsg.Session{Key: "anonymous:{other-app}"}

	if err := other.Add(t.Context(), session, flashmsg.Info, "Keep this"); err != nil {
		t.Fatal(err)
	}

	bag, err := store.Take(t.Context(), session, "")

	if err != nil || len(bag.All()) != 0 || mini.TTL(session.Key) != time.Minute {
		t.Fatal("one application consumed another application's messages or changed its lifetime")
	}

	bag, err = other.Take(t.Context(), session, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Info), []string{"Keep this"}) {
		t.Fatalf("configured application consumption = %v, %v", bag, err)
	}
}

func TestFlashStorageErrors(t *testing.T) {
	for _, key := range []string{"", " "} {
		t.Run("invalid key "+key, func(t *testing.T) {
			store, mini, _ := flashFixture(t)
			session := flashmsg.Session{Key: key}
			originalKeys := mini.Keys()

			if err := store.Add(t.Context(), session, flashmsg.Error, "Invalid"); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("invalid session add = %v", err)
			}

			if bag, err := store.Take(t.Context(), session, ""); bag != nil || !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("invalid session read = %v, %v", bag, err)
			}

			if !slices.Equal(mini.Keys(), originalKeys) {
				t.Fatal("invalid session changed Redis keys")
			}
		})
	}

	for _, tt := range []struct{ name, data string }{
		{name: "invalid JSON", data: `{`},
		{name: "invalid message type", data: `{"error":"invalid"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, mini, session := flashFixture(t)
			mini.HSet(session.Key, "admin:flashes", tt.data)

			if bag, err := store.Take(t.Context(), session, ""); bag != nil || err == nil {
				t.Fatalf("corrupt flash storage = %v, %v", bag, err)
			}
		})
	}

	t.Run("closed client", func(t *testing.T) {
		store, _, session := flashFixture(t)

		if err := store.client.Close(); err != nil {
			t.Fatal(err)
		}

		if err := store.Add(t.Context(), session, flashmsg.Error, "Offline"); !errors.Is(err, redislib.ErrClosed) {
			t.Fatalf("closed client add = %v", err)
		}

		if bag, err := store.Take(t.Context(), session, ""); bag != nil || !errors.Is(err, redislib.ErrClosed) {
			t.Fatalf("closed client read = %v, %v", bag, err)
		}
	})
}
