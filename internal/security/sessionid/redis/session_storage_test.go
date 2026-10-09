package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func newStorage(t *testing.T) (*SessionStorage, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	storage, err := NewSessionStorage(client)

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

func TestNewSessionStorageRejectsNilClient(t *testing.T) {
	if _, err := NewSessionStorage(nil); !errors.Is(err, ErrClientNil) {
		t.Fatalf("got %v, want %v", err, ErrClientNil)
	}
}

func TestSessionStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)
	session := newSession(time.Now().Add(time.Hour))
	digest := sessionid.Digest{1, 2, 3}

	if err := storage.Create(ctx, session, digest); err != nil {
		t.Fatalf("create: %v", err)
	}

	found, foundDigest, err := storage.FindByID(ctx, session.ID)

	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if *found != *session || foundDigest != digest {
		t.Fatalf("got (%+v, %x), want (%+v, %x)", found, foundDigest, session, digest)
	}

	if err := storage.Create(ctx, session, sessionid.Digest{9}); !errors.Is(err, security.ErrSessionInvalid) {
		t.Fatalf("create twice: got %v, want %v", err, security.ErrSessionInvalid)
	}

	if err := storage.Delete(ctx, session.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, _, err := storage.FindByID(ctx, session.ID); !errors.Is(err, sessionid.ErrSessionNotFound) {
		t.Fatalf("find deleted: got %v, want %v", err, sessionid.ErrSessionNotFound)
	}
}

func TestSessionStorageExpiresSessions(t *testing.T) {
	ctx := context.Background()
	storage, server := newStorage(t)
	session := newSession(time.Now().Add(time.Minute))

	if err := storage.Create(ctx, session, sessionid.Digest{1}); err != nil {
		t.Fatalf("create: %v", err)
	}

	server.FastForward(2 * time.Minute)

	if _, _, err := storage.FindByID(ctx, session.ID); !errors.Is(err, sessionid.ErrSessionNotFound) {
		t.Fatalf("got %v, want %v", err, sessionid.ErrSessionNotFound)
	}
}

func TestSessionStorageRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	storage, _ := newStorage(t)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return storage.Create(ctx, nil, sessionid.Digest{}) }, security.ErrSessionInvalid},
		{"create invalid", func() error {
			return storage.Create(ctx, &security.Session{}, sessionid.Digest{})
		}, security.ErrSessionInvalid},
		{"find nil ID", func() error { _, _, err := storage.FindByID(ctx, uuid.Nil); return err }, security.ErrSessionNilID},
		{"delete nil ID", func() error { return storage.Delete(ctx, uuid.Nil) }, security.ErrSessionNilID},
		{"missing client", func() error { return (&SessionStorage{}).Delete(ctx, uuid.New()) }, ErrClientNil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestSessionStorageRejectsCorruptRecords(t *testing.T) {
	ctx := context.Background()
	storage, server := newStorage(t)
	id := uuid.Must(uuid.NewV7())

	server.HSet(sessionKey(id), "user_id", "not a UUID", "credentials_version", uuid.NewString())

	if _, _, err := storage.FindByID(ctx, id); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("got %v, want %v", err, ErrRecordInvalid)
	}
}
