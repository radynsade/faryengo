package redis

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func newStorage(t *testing.T) (*AttemptStorage, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	storage, err := NewAttemptStorage(client)

	if err != nil {
		t.Fatalf("create storage: %v", err)
	}

	return storage, server
}

func TestNewAttemptStorageRejectsNilClient(t *testing.T) {
	if _, err := NewAttemptStorage(nil); !errors.Is(err, ErrClientNil) {
		t.Fatalf("got %v, want %v", err, ErrClientNil)
	}
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	storage, server := newStorage(t)

	for _, want := range [][]int64{{1, 1}, {2, 2}, {3, 3}} {
		counts, err := storage.Add(ctx, time.Minute, "email:a", "client:b")

		if err != nil || !slices.Equal(counts, want) {
			t.Fatalf("got (%v, %v), want (%v, nil)", counts, err, want)
		}
	}

	counts, err := storage.Add(ctx, time.Minute, "email:c", "client:b")

	if err != nil || !slices.Equal(counts, []int64{1, 4}) {
		t.Fatalf("got (%v, %v), want ([1 4], nil)", counts, err)
	}

	if ttl := server.TTL(keyPrefix + "client:b"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("got TTL %v, want one within the window", ttl)
	}

	server.FastForward(time.Minute)

	counts, err = storage.Add(ctx, time.Minute, "client:b")

	if err != nil || !slices.Equal(counts, []int64{1}) {
		t.Fatalf("got (%v, %v), want ([1], nil) after the window", counts, err)
	}
}

func TestAddRejectsInvalidArguments(t *testing.T) {
	storage, _ := newStorage(t)

	tests := []struct {
		name   string
		window time.Duration
		keys   []string
		want   error
	}{
		{"no keys", time.Minute, nil, ErrKeysEmpty},
		{"zero window", 0, []string{"email:a"}, ErrWindowInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := storage.Add(context.Background(), test.window, test.keys...); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestAddFailsWhenStorageIsUnavailable(t *testing.T) {
	storage, server := newStorage(t)

	server.Close()

	if _, err := storage.Add(context.Background(), time.Minute, "email:a"); err == nil {
		t.Fatal("got nil, want a storage failure")
	}
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)

	if _, err := storage.Add(ctx, time.Minute, "email:a", "client:b"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := storage.Reset(ctx, "email:a"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	counts, err := storage.Add(ctx, time.Minute, "email:a", "client:b")

	if err != nil || !slices.Equal(counts, []int64{1, 2}) {
		t.Fatalf("got (%v, %v), want ([1 2], nil)", counts, err)
	}
}
