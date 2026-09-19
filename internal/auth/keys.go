// Package auth implements API-key, session, CSRF and passkey authentication.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Key prefixes distinguish search and admin keys.
const (
	SearchKeyPrefix = "seek_ak_"
	AdminKeyPrefix  = "seek_admin_"
)

// argon2id parameters: 16 MiB, 3 iterations, 2 lanes, 32-byte tag.
const (
	argonTime    = 3
	argonMemory  = 16 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrInvalidKey is returned for malformed or unknown keys.
var ErrInvalidKey = errors.New("auth: invalid api key")

// GenerateKey creates a new plaintext API key with its stored prefix and hash.
// The plaintext is returned exactly once and never persisted.
func GenerateKey(admin bool) (plaintext, prefix, hash string, err error) {
	prefix, err = randomHex(3) // 6 hex chars
	if err != nil {
		return "", "", "", err
	}
	secret, err := randomHex(12) // 24 hex chars
	if err != nil {
		return "", "", "", err
	}
	hash, err = HashSecret(secret)
	if err != nil {
		return "", "", "", err
	}
	if admin {
		return AdminKeyPrefix + prefix + "_" + secret, prefix, hash, nil
	}
	return SearchKeyPrefix + prefix + "_" + secret, prefix, hash, nil
}

// ParseKey splits a key into its kind, prefix and secret.
func ParseKey(token string) (kind, prefix, secret string, ok bool) {
	token = strings.TrimSpace(token)
	switch {
	case strings.HasPrefix(token, AdminKeyPrefix):
		kind = "admin"
		token = strings.TrimPrefix(token, AdminKeyPrefix)
	case strings.HasPrefix(token, SearchKeyPrefix):
		kind = "search"
		token = strings.TrimPrefix(token, SearchKeyPrefix)
	default:
		return "", "", "", false
	}
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
		return "", "", "", false
	}
	return kind, parts[0], parts[1], true
}

// HashSecret derives an argon2id hash string for a key secret.
func HashSecret(secret string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifySecret compares a plaintext secret against an encoded argon2id hash in
// constant time.
func VerifySecret(encoded, secret string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// RandomToken returns a URL-safe random token.
func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
