package argon2id

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var (
	ErrHasherNil = errors.New("argon2id password hasher is nil")

	ErrHashTooLong           = fmt.Errorf("exceeds the limit of %d bytes", MaxHashBytes)
	ErrHashFormatInvalid     = errors.New("invalid format")
	ErrHashParametersInvalid = errors.New("invalid parameters")
	ErrHashSaltInvalid       = errors.New("invalid salt")
	ErrHashKeyInvalid        = errors.New("invalid key")
)

//
// Parameters
//

const (
	MaxMemoryKiB          uint32 = 256 * 1024
	MaxIterations         uint32 = 10
	MaxParallelism        uint8  = 16
	MinMemoryKiBPerThread uint32 = 8
)

type parameters struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

var defaultParameters = parameters{
	memoryKiB:   64 * 1024,
	iterations:  3,
	parallelism: 4,
}

func (p parameters) Validate() error {
	var err error

	if p.parallelism == 0 || p.parallelism > MaxParallelism {
		err = ErrHashParametersInvalid
	} else if p.iterations == 0 || p.iterations > MaxIterations {
		err = ErrHashParametersInvalid
	} else if p.memoryKiB < MinMemoryKiBPerThread*uint32(p.parallelism) || p.memoryKiB > MaxMemoryKiB {
		err = ErrHashParametersInvalid
	}

	return err
}

func (p parameters) encode() string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", p.memoryKiB, p.iterations, p.parallelism)
}

//
// Password hasher
//

const (
	MaxHashBytes = 256

	algorithmSegment = "argon2id"
	hashSegments     = 6
	saltBytes        = 16
	keyBytes         = 32
)

var versionSegment = fmt.Sprintf("v=%d", argon2.Version)

type PasswordHasher struct {
	params parameters
}

var _ users.PasswordHasher = (*PasswordHasher)(nil)

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{params: defaultParameters}
}

// Salts come from crypto/rand.Read, which never returns an error and aborts the
// program when the system random source fails.

func (h *PasswordHasher) Hash(ctx context.Context, password string) (users.PasswordHash, error) {
	var (
		hash users.PasswordHash
		err  error
	)

	if h == nil || h.params == (parameters{}) {
		err = ErrHasherNil
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if validationErr := users.Password(password).Validate(); validationErr != nil {
		err = validationErr
	} else {
		salt := make([]byte, saltBytes)
		_, _ = rand.Read(salt)

		key := argon2.IDKey(
			[]byte(password),
			salt,
			h.params.iterations,
			h.params.memoryKiB,
			h.params.parallelism,
			keyBytes,
		)

		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		} else {
			hash = encodeHash(h.params, salt, key)
		}
	}

	if err != nil {
		err = fmt.Errorf("hash a password: %w", err)
	}

	return hash, err
}

// A candidate password is checked only against the byte limit, so stored
// passwords remain verifiable after the password rules change. The hash's own
// parameters drive the derivation, so hashes made with older parameters remain
// verifiable too.

func (h *PasswordHasher) Verify(
	ctx context.Context,
	password string,
	hash users.PasswordHash,
) (bool, error) {
	var (
		matches bool
		err     error
	)

	if h == nil || h.params == (parameters{}) {
		err = ErrHasherNil
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if len(password) > users.MaxPasswordBytes {
		err = fmt.Errorf("%w: %w", users.ErrPasswordInvalid, users.ErrPasswordTooLong)
	} else if validationErr := hash.Validate(); validationErr != nil {
		err = validationErr
	} else {
		var (
			params         parameters
			salt, expected []byte
		)

		params, salt, expected, err = decodeHash(hash)

		if err == nil {
			actual := argon2.IDKey(
				[]byte(password),
				salt,
				params.iterations,
				params.memoryKiB,
				params.parallelism,
				uint32(len(expected)),
			)

			err = ctx.Err()
			matches = err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
		}
	}

	if err != nil {
		err = fmt.Errorf("verify a password: %w", err)
	}

	return matches, err
}

//
// Helpers
//

func encodeHash(params parameters, salt []byte, key []byte) users.PasswordHash {
	encoded := strings.Join([]string{
		"",
		algorithmSegment,
		versionSegment,
		params.encode(),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$")

	return users.PasswordHash(encoded)
}

func decodeHash(hash users.PasswordHash) (parameters, []byte, []byte, error) {
	var (
		params    parameters
		segments  []string
		salt, key []byte
		err       error
	)

	if len(hash) > MaxHashBytes {
		err = ErrHashTooLong
	} else {
		segments = strings.Split(string(hash), "$")

		if len(segments) != hashSegments ||
			segments[0] != "" ||
			segments[1] != algorithmSegment ||
			segments[2] != versionSegment {
			err = ErrHashFormatInvalid
		}
	}

	if err == nil {
		params, err = decodeParameters(segments[3])
	}

	if err == nil {
		salt, err = decodeBytes(segments[4], saltBytes, ErrHashSaltInvalid)
	}

	if err == nil {
		key, err = decodeBytes(segments[5], keyBytes, ErrHashKeyInvalid)
	}

	if err != nil {
		params, salt, key = parameters{}, nil, nil
		err = fmt.Errorf("%w: %w", users.ErrPasswordHashInvalid, err)
	}

	return params, salt, key, err
}

// Only the canonical encoding is accepted, so each hash has exactly one
// textual form.

func decodeParameters(segment string) (parameters, error) {
	var (
		params parameters
		err    error
	)

	count, scanErr := fmt.Sscanf(
		segment,
		"m=%d,t=%d,p=%d",
		&params.memoryKiB,
		&params.iterations,
		&params.parallelism,
	)

	if scanErr != nil || count != 3 || segment != params.encode() {
		err = ErrHashParametersInvalid
	} else {
		err = params.Validate()
	}

	if err != nil {
		params = parameters{}
	}

	return params, err
}

func decodeBytes(segment string, size int, invalidErr error) ([]byte, error) {
	var (
		decoded []byte
		err     error
	)

	encoding := base64.RawStdEncoding.Strict()

	if len(segment) != encoding.EncodedLen(size) {
		err = invalidErr
	} else if decoded, err = encoding.DecodeString(segment); err != nil {
		err = fmt.Errorf("%w: %w", invalidErr, err)
	}

	if err != nil {
		decoded = nil
	}

	return decoded, err
}
