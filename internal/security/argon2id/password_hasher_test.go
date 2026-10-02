package argon2id

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"github.com/radynsade/faryengo/internal/security"
)

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestHasherRoundTrip(t *testing.T) {
	hasher := NewHasher()
	hash, err := hasher.Hash(context.Background(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if !strings.HasPrefix(string(hash), "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("Hash() = %q, want Argon2id PHC format", hash)
	}

	for _, tt := range []struct {
		name     string
		password string
		want     bool
	}{
		{name: "matching password", password: "correct horse battery staple", want: true},
		{name: "wrong password", password: "wrong password"},
		{name: "empty candidate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := hasher.Verify(context.Background(), tt.password, hash)
			if err != nil || matches != tt.want {
				t.Fatalf("Verify() = (%v, %v), want (%v, nil)", matches, err, tt.want)
			}
		})
	}
}

func TestHasherUsesFreshSalt(t *testing.T) {
	hasher := NewHasher()
	first, err := hasher.Hash(context.Background(), "same password")
	if err != nil {
		t.Fatalf("first Hash() error = %v", err)
	}

	second, err := hasher.Hash(context.Background(), "same password")
	if err != nil {
		t.Fatalf("second Hash() error = %v", err)
	}

	if first == second {
		t.Fatal("Hash() produced identical hashes for two independent salts")
	}

	firstParts := strings.Split(string(first), "$")
	secondParts := strings.Split(string(second), "$")
	if firstParts[4] == secondParts[4] {
		t.Fatal("Hash() reused the same salt")
	}
}

func TestHasherRejectsInvalidInputs(t *testing.T) {
	hasher := NewHasher()
	for _, tt := range []struct {
		name     string
		hasher   *Hasher
		password string
		wantErr  error
	}{
		{name: "empty password", hasher: hasher, wantErr: security.ErrInvalidPassword},
		{name: "nil hasher", password: "password", wantErr: ErrNilHasher},
		{name: "nil salt reader", hasher: &Hasher{}, password: "password", wantErr: ErrNilHasher},
		{name: "salt read fails", hasher: &Hasher{saltReader: failingReader{err: io.ErrUnexpectedEOF}}, password: "password", wantErr: io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := tt.hasher.Hash(context.Background(), tt.password)
			if hash != "" || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Hash() = (%q, %v), want empty hash and %v", hash, err, tt.wantErr)
			}
		})
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	hasher := NewHasher()
	valid, err := hasher.Hash(context.Background(), "password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	parts := strings.Split(string(valid), "$")
	withPart := func(index int, value string) security.PasswordHash {
		copyParts := append([]string(nil), parts...)
		copyParts[index] = value
		return security.PasswordHash(strings.Join(copyParts, "$"))
	}

	for _, tt := range []struct {
		name string
		hash security.PasswordHash
	}{
		{name: "empty"},
		{name: "oversized", hash: security.PasswordHash(strings.Repeat("x", maxEncodedHashBytes+1))},
		{name: "wrong algorithm", hash: withPart(1, "argon2i")},
		{name: "wrong version", hash: withPart(2, "v=16")},
		{name: "missing separator", hash: security.PasswordHash(strings.Replace(string(valid), "$m=", "m=", 1))},
		{name: "zero memory", hash: withPart(3, "m=0,t=3,p=4")},
		{name: "excessive memory", hash: withPart(3, fmt.Sprintf("m=%d,t=3,p=4", maxMemoryKiB+1))},
		{name: "zero iterations", hash: withPart(3, "m=65536,t=0,p=4")},
		{name: "excessive iterations", hash: withPart(3, "m=65536,t=11,p=4")},
		{name: "zero parallelism", hash: withPart(3, "m=65536,t=3,p=0")},
		{name: "excessive parallelism", hash: withPart(3, "m=65536,t=3,p=17")},
		{name: "noncanonical parameters", hash: withPart(3, "m=065536,t=3,p=4")},
		{name: "short salt", hash: withPart(4, parts[4][:len(parts[4])-1])},
		{name: "invalid salt encoding", hash: withPart(4, "!"+parts[4][1:])},
		{name: "short key", hash: withPart(5, parts[5][:len(parts[5])-1])},
		{name: "invalid key encoding", hash: withPart(5, "!"+parts[5][1:])},
	} {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := hasher.Verify(context.Background(), "password", tt.hash)
			if matches || !errors.Is(err, security.ErrInvalidPasswordHash) {
				t.Fatalf("Verify() = (%v, %v), want (false, ErrInvalidPasswordHash)", matches, err)
			}
		})
	}

	var nilHasher *Hasher
	matches, err := nilHasher.Verify(context.Background(), "password", valid)
	if matches || !errors.Is(err, ErrNilHasher) {
		t.Fatalf("Verify(nil receiver) = (%v, %v), want (false, ErrNilHasher)", matches, err)
	}
}

func TestHasherHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hasher := NewHasher()
	hash, err := hasher.Hash(ctx, "password")
	if hash != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("Hash(canceled) = (%q, %v), want empty hash and context.Canceled", hash, err)
	}

	matches, err := hasher.Verify(ctx, "password", security.PasswordHash("invalid"))
	if matches || !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify(canceled) = (%v, %v), want false and context.Canceled", matches, err)
	}
}

func TestVerifyUsesStoredParameters(t *testing.T) {
	password := "different profile"
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, keyBytes)
	hash := security.PasswordHash(fmt.Sprintf("$argon2id$v=%d$m=8192,t=1,p=1$%s$%s",
		argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	))

	matches, err := NewHasher().Verify(context.Background(), password, hash)
	if err != nil || !matches {
		t.Fatalf("Verify(other profile) = (%v, %v), want (true, nil)", matches, err)
	}
}
