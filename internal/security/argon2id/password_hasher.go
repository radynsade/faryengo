package argon2id

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/radynsade/faryengo/internal/security"
)

const (
	memoryKiB           uint32 = 64 * 1024
	iterations          uint32 = 3
	parallelism         uint8  = 4
	saltBytes                  = 16
	keyBytes                   = 32
	maxEncodedHashBytes        = 256

	maxMemoryKiB   uint32 = 256 * 1024
	maxIterations  uint32 = 10
	maxParallelism uint8  = 16
)

var ErrNilHasher = errors.New("nil Argon2id password hasher")

type Hasher struct {
	saltReader io.Reader
}

var _ security.PasswordHasher = (*Hasher)(nil)

func NewHasher() *Hasher {
	return &Hasher{saltReader: rand.Reader}
}

func (h *Hasher) Hash(ctx context.Context, password string) (security.PasswordHash, error) {
	var hash security.PasswordHash
	var err error

	if h == nil || h.saltReader == nil {
		err = ErrNilHasher
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("hash password: %w", contextErr)
	} else if password == "" {
		err = security.ErrInvalidPassword
	} else {
		salt := make([]byte, saltBytes)
		if _, readErr := io.ReadFull(h.saltReader, salt); readErr != nil {
			err = fmt.Errorf("read password salt: %w", readErr)
		} else if contextErr := ctx.Err(); contextErr != nil {
			err = fmt.Errorf("hash password: %w", contextErr)
		} else {
			key := argon2.IDKey([]byte(password), salt, iterations, memoryKiB, parallelism, keyBytes)
			encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
				argon2.Version, memoryKiB, iterations, parallelism,
				base64.RawStdEncoding.EncodeToString(salt),
				base64.RawStdEncoding.EncodeToString(key),
			)

			if contextErr := ctx.Err(); contextErr != nil {
				err = fmt.Errorf("hash password: %w", contextErr)
			} else {
				hash, err = security.NewPasswordHash(encoded)
				if err != nil {
					err = fmt.Errorf("encode password hash: %w", err)
				}
			}
		}
	}

	return hash, err
}

func (h *Hasher) Verify(ctx context.Context, password string, hash security.PasswordHash) (bool, error) {
	var matches bool
	var err error

	if h == nil {
		err = ErrNilHasher
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("verify password: %w", contextErr)
	} else {
		var params hashParameters
		var salt []byte
		var expected []byte
		params, salt, expected, err = parseHash(hash)
		if err == nil {
			actual := argon2.IDKey([]byte(password), salt, params.iterations, params.memoryKiB, params.parallelism, uint32(len(expected)))
			if contextErr := ctx.Err(); contextErr != nil {
				err = fmt.Errorf("verify password: %w", contextErr)
			} else {
				matches = subtle.ConstantTimeCompare(actual, expected) == 1
			}
		}
	}

	return matches, err
}

type hashParameters struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

func parseHash(hash security.PasswordHash) (hashParameters, []byte, []byte, error) {
	var params hashParameters
	var salt []byte
	var key []byte

	if len(hash) > maxEncodedHashBytes {
		return params, nil, nil, fmt.Errorf("parse Argon2id hash length: %w", security.ErrInvalidPasswordHash)
	}

	parts := strings.Split(string(hash), "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return params, nil, nil, fmt.Errorf("parse Argon2id hash header: %w", security.ErrInvalidPasswordHash)
	}

	count, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memoryKiB, &params.iterations, &params.parallelism)
	if err != nil || count != 3 || parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", params.memoryKiB, params.iterations, params.parallelism) ||
		params.memoryKiB < 8*uint32(params.parallelism) || params.memoryKiB > maxMemoryKiB ||
		params.iterations == 0 || params.iterations > maxIterations || params.parallelism == 0 || params.parallelism > maxParallelism {
		return hashParameters{}, nil, nil, fmt.Errorf("parse Argon2id hash parameters: %w", security.ErrInvalidPasswordHash)
	}

	encoding := base64.RawStdEncoding.Strict()
	if len(parts[4]) != encoding.EncodedLen(saltBytes) || len(parts[5]) != encoding.EncodedLen(keyBytes) {
		return hashParameters{}, nil, nil, fmt.Errorf("parse Argon2id hash lengths: %w", security.ErrInvalidPasswordHash)
	}

	salt, err = encoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltBytes {
		return hashParameters{}, nil, nil, fmt.Errorf("decode Argon2id salt: %w", security.ErrInvalidPasswordHash)
	}

	key, err = encoding.DecodeString(parts[5])
	if err != nil || len(key) != keyBytes {
		return hashParameters{}, nil, nil, fmt.Errorf("decode Argon2id key: %w", security.ErrInvalidPasswordHash)
	}

	return params, salt, key, nil
}
