package argon2id

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/goleak"
	"golang.org/x/crypto/argon2"

	"github.com/radynsade/faryengo/internal/users"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestPasswordHasherRoundTrip(t *testing.T) {
	ctx := context.Background()
	hasher := NewPasswordHasher()
	hash, err := hasher.Hash(ctx, "correct horse battery staple")

	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if !strings.HasPrefix(string(hash), "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("got %q, want the Argon2id PHC format", hash)
	}

	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"matching password", "correct horse battery staple", true},
		{"wrong password", "wrong password", false},
		{"empty candidate", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches, err := hasher.Verify(ctx, test.password, hash)

			if err != nil || matches != test.want {
				t.Fatalf("got (%v, %v), want (%v, nil)", matches, err, test.want)
			}
		})
	}
}

func TestHashUsesFreshSalt(t *testing.T) {
	ctx := context.Background()
	hasher := NewPasswordHasher()
	first, firstErr := hasher.Hash(ctx, "same password")
	second, secondErr := hasher.Hash(ctx, "same password")

	if firstErr != nil || secondErr != nil {
		t.Fatalf("hash: %v, %v", firstErr, secondErr)
	}

	firstSalt := strings.Split(string(first), "$")[4]
	secondSalt := strings.Split(string(second), "$")[4]

	if firstSalt == secondSalt {
		t.Fatal("two hashes share a salt")
	}
}

func TestHashRejectsInvalidInputs(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name     string
		ctx      context.Context
		hasher   *PasswordHasher
		password string
		want     error
	}{
		{"nil hasher", context.Background(), nil, "password", ErrHasherNil},
		{"zero hasher", context.Background(), &PasswordHasher{}, "password", ErrHasherNil},
		{"canceled context", canceled, NewPasswordHasher(), "password", context.Canceled},
		{"empty password", context.Background(), NewPasswordHasher(), "", users.ErrPasswordEmpty},
		{"short password", context.Background(), NewPasswordHasher(), "short", users.ErrPasswordTooShort},
		{
			"long password",
			context.Background(),
			NewPasswordHasher(),
			strings.Repeat("x", users.MaxPasswordBytes+1),
			users.ErrPasswordTooLong,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hash, err := test.hasher.Hash(test.ctx, test.password)

			if hash != "" || !errors.Is(err, test.want) {
				t.Fatalf("got (%q, %v), want an empty hash and %v", hash, err, test.want)
			}
		})
	}
}

func TestVerifyRejectsInvalidInputs(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name     string
		ctx      context.Context
		hasher   *PasswordHasher
		password string
		want     error
	}{
		{"nil hasher", context.Background(), nil, "password", ErrHasherNil},
		{"zero hasher", context.Background(), &PasswordHasher{}, "password", ErrHasherNil},
		{"canceled context", canceled, NewPasswordHasher(), "password", context.Canceled},
		{
			"long password",
			context.Background(),
			NewPasswordHasher(),
			strings.Repeat("x", users.MaxPasswordBytes+1),
			users.ErrPasswordTooLong,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches, err := test.hasher.Verify(test.ctx, test.password, "hash")

			if matches || !errors.Is(err, test.want) {
				t.Fatalf("got (%v, %v), want (false, %v)", matches, err, test.want)
			}
		})
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	ctx := context.Background()
	hasher := NewPasswordHasher()
	valid, err := hasher.Hash(ctx, "password")

	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	segments := strings.Split(string(valid), "$")
	withSegment := func(index int, value string) users.PasswordHash {
		replaced := append([]string(nil), segments...)
		replaced[index] = value

		return users.PasswordHash(strings.Join(replaced, "$"))
	}

	tests := []struct {
		name string
		hash users.PasswordHash
		want error
	}{
		{"empty", "", users.ErrPasswordHashEmpty},
		{"oversized", users.PasswordHash(strings.Repeat("x", MaxHashBytes+1)), ErrHashTooLong},
		{"wrong algorithm", withSegment(1, "argon2i"), ErrHashFormatInvalid},
		{"wrong version", withSegment(2, "v=16"), ErrHashFormatInvalid},
		{"missing separator", users.PasswordHash(strings.Replace(string(valid), "$m=", "m=", 1)), ErrHashFormatInvalid},
		{"zero memory", withSegment(3, "m=0,t=3,p=4"), ErrHashParametersInvalid},
		{"excessive memory", withSegment(3, fmt.Sprintf("m=%d,t=3,p=4", MaxMemoryKiB+1)), ErrHashParametersInvalid},
		{"zero iterations", withSegment(3, "m=65536,t=0,p=4"), ErrHashParametersInvalid},
		{"excessive iterations", withSegment(3, "m=65536,t=11,p=4"), ErrHashParametersInvalid},
		{"zero parallelism", withSegment(3, "m=65536,t=3,p=0"), ErrHashParametersInvalid},
		{"excessive parallelism", withSegment(3, "m=65536,t=3,p=17"), ErrHashParametersInvalid},
		{"noncanonical parameters", withSegment(3, "m=065536,t=3,p=4"), ErrHashParametersInvalid},
		{"short salt", withSegment(4, segments[4][:len(segments[4])-1]), ErrHashSaltInvalid},
		{"invalid salt encoding", withSegment(4, "!"+segments[4][1:]), ErrHashSaltInvalid},
		{"short key", withSegment(5, segments[5][:len(segments[5])-1]), ErrHashKeyInvalid},
		{"invalid key encoding", withSegment(5, "!"+segments[5][1:]), ErrHashKeyInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches, err := hasher.Verify(ctx, "password", test.hash)

			if matches || !errors.Is(err, users.ErrPasswordHashInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("got (%v, %v), want (false, %v)", matches, err, test.want)
			}
		})
	}
}

func TestVerifyUsesStoredParameters(t *testing.T) {
	password := "different profile"
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, keyBytes)
	hash := users.PasswordHash(fmt.Sprintf(
		"$argon2id$v=%d$m=8192,t=1,p=1$%s$%s",
		argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	))

	matches, err := NewPasswordHasher().Verify(context.Background(), password, hash)

	if err != nil || !matches {
		t.Fatalf("got (%v, %v), want (true, nil)", matches, err)
	}
}
