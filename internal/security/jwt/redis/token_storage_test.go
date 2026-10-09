package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/jwt"
	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func newStorage(t *testing.T) (*TokenStorage, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	storage, err := NewTokenStorage(client)

	if err != nil {
		t.Fatalf("create storage: %v", err)
	}

	return storage, server
}

func newSession(expiresAt time.Time) *security.Session {
	return &security.Session{
		ID:                 uuid.Must(uuid.NewV7()),
		UserID:             users.UserID(uuid.Must(uuid.NewV7())),
		CredentialsVersion: uuid.Must(uuid.NewV7()),
		ExpiresAt:          expiresAt.Truncate(time.Millisecond),
	}
}

func TestNewTokenStorageRejectsNilClient(t *testing.T) {
	if _, err := NewTokenStorage(nil); !errors.Is(err, ErrClientNil) {
		t.Fatalf("got %v, want %v", err, ErrClientNil)
	}
}

func TestTokenStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)
	session := newSession(time.Now().Add(time.Hour))
	refreshID := uuid.Must(uuid.NewV7())

	if err := storage.Create(ctx, session, refreshID); err != nil {
		t.Fatalf("create: %v", err)
	}

	found, foundRefreshID, err := storage.FindByID(ctx, session.ID)

	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if *found != *session || foundRefreshID != refreshID {
		t.Fatalf("got (%+v, %s), want (%+v, %s)", found, foundRefreshID, session, refreshID)
	}

	if err := storage.Create(ctx, session, uuid.Must(uuid.NewV7())); !errors.Is(err, security.ErrSessionInvalid) {
		t.Fatalf("create twice: got %v, want %v", err, security.ErrSessionInvalid)
	}

	if err := storage.Delete(ctx, session.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, _, err := storage.FindByID(ctx, session.ID); !errors.Is(err, jwt.ErrSessionNotFound) {
		t.Fatalf("find deleted: got %v, want %v", err, jwt.ErrSessionNotFound)
	}
}

func TestTokenStorageRotate(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)
	session := newSession(time.Now().Add(time.Hour))
	first := uuid.Must(uuid.NewV7())
	second := uuid.Must(uuid.NewV7())
	third := uuid.Must(uuid.NewV7())

	if err := storage.Create(ctx, session, first); err != nil {
		t.Fatalf("create: %v", err)
	}

	tests := []struct {
		name    string
		id      uuid.UUID
		current uuid.UUID
		next    uuid.UUID
		want    error
		wantID  uuid.UUID
	}{
		{"current ID", session.ID, first, second, nil, second},
		{"replaced ID", session.ID, first, third, jwt.ErrRefreshTokenMismatch, second},
		{"missing session", uuid.Must(uuid.NewV7()), second, third, jwt.ErrSessionNotFound, second},
		{"same IDs", session.ID, second, second, jwt.ErrTokenInvalid, second},
		{"nil session ID", uuid.Nil, second, third, security.ErrSessionNilID, second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := storage.Rotate(ctx, test.id, test.current, test.next); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			_, refreshID, err := storage.FindByID(ctx, session.ID)

			if err != nil || refreshID != test.wantID {
				t.Fatalf("got (%s, %v), want (%s, nil)", refreshID, err, test.wantID)
			}
		})
	}
}

func TestTokenStorageRotatesOnceUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)
	session := newSession(time.Now().Add(time.Hour))
	current := uuid.Must(uuid.NewV7())

	if err := storage.Create(ctx, session, current); err != nil {
		t.Fatalf("create: %v", err)
	}

	const attempts = 8

	var (
		group sync.WaitGroup
		mutex sync.Mutex
	)

	rotated := 0

	for range attempts {
		group.Go(func() {
			err := storage.Rotate(ctx, session.ID, current, uuid.Must(uuid.NewV7()))

			mutex.Lock()
			defer mutex.Unlock()

			if err == nil {
				rotated++
			} else if !errors.Is(err, jwt.ErrRefreshTokenMismatch) {
				t.Errorf("rotate: %v", err)
			}
		})
	}

	group.Wait()

	if rotated != 1 {
		t.Fatalf("got %d successful rotations, want 1", rotated)
	}
}

func TestTokenStorageExpiresSessions(t *testing.T) {
	ctx := context.Background()
	storage, server := newStorage(t)
	session := newSession(time.Now().Add(time.Minute))

	if err := storage.Create(ctx, session, uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("create: %v", err)
	}

	server.FastForward(2 * time.Minute)

	if _, _, err := storage.FindByID(ctx, session.ID); !errors.Is(err, jwt.ErrSessionNotFound) {
		t.Fatalf("got %v, want %v", err, jwt.ErrSessionNotFound)
	}
}
