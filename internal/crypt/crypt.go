// Package crypt provides optional AES-256-GCM encryption for secrets at rest.
// Encrypted values use the prefix "enc:v1:"; unprefixed values are treated as
// plaintext.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// prefix marks an encrypted value and its scheme version.
const prefix = "enc:v1:"

// Cipher encrypts and decrypts short secret strings.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a 32-byte hex/base64 key; an empty key disables it.
func New(keyMaterial string) (*Cipher, error) {
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return nil, nil
	}
	key, err := parseKey(keyMaterial)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Enabled reports whether encryption is active.
func (c *Cipher) Enabled() bool { return c != nil && c.aead != nil }

// Encrypt returns the encrypted form of plain; nil cipher and empty values pass
// through unchanged.
func (c *Cipher) Encrypt(plain string) string {
	if !c.Enabled() || plain == "" {
		return plain
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plain
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(sealed)
}

// Decrypt reverses Encrypt; unrecognized values pass through unchanged.
func (c *Cipher) Decrypt(value string) string {
	if !c.Enabled() || value == "" || !strings.HasPrefix(value, prefix) {
		return value
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return value
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return value
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return value
	}
	return string(plain)
}

// parseKey accepts a 32-byte key as hex, base64 or raw text.
func parseKey(material string) ([]byte, error) {
	if len(material) == 64 {
		if b, err := hex.DecodeString(material); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(material); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	if len(material) == 32 {
		return []byte(material), nil
	}
	return nil, errors.New("encryption key must decode to 32 bytes (hex or base64)")
}
