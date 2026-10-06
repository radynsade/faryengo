package app

import (
	"crypto/sha256"
	"encoding/hex"
)

// Persist digests rather than bearer credentials for either mechanism.
func credentialDigest(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
