package crypt

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	for name, material := range map[string]string{
		"hex":    hex.EncodeToString(key),
		"base64": base64.StdEncoding.EncodeToString(key),
		"raw":    string(key),
	} {
		c, err := New(material)
		if err != nil {
			t.Fatalf("%s: New: %v", name, err)
		}
		if !c.Enabled() {
			t.Fatalf("%s: cipher not enabled", name)
		}
		enc := c.Encrypt("super-secret")
		if !strings.HasPrefix(enc, prefix) {
			t.Fatalf("%s: missing prefix: %q", name, enc)
		}
		if enc == "super-secret" {
			t.Fatalf("%s: plaintext not encrypted", name)
		}
		if got := c.Decrypt(enc); got != "super-secret" {
			t.Fatalf("%s: Decrypt = %q", name, got)
		}
		// Legacy plaintext stays readable.
		if got := c.Decrypt("legacy"); got != "legacy" {
			t.Fatalf("%s: legacy Decrypt = %q", name, got)
		}
	}
}

func TestEmptyAndInvalidKey(t *testing.T) {
	c, err := New("")
	if err != nil || c != nil {
		t.Fatalf("empty key: c=%v err=%v", c, err)
	}
	if _, err := New("too-short"); err == nil {
		t.Fatal("expected error for short key")
	}
	// A nil cipher is a transparent no-op.
	var nilCipher *Cipher
	if nilCipher.Encrypt("x") != "x" || nilCipher.Decrypt("x") != "x" {
		t.Fatal("nil cipher must be a no-op")
	}
}

func TestWrongKeyDoesNotPanic(t *testing.T) {
	key := strings.Repeat("a", 32)
	c, _ := New(key)
	enc := c.Encrypt("value")
	other, _ := New(strings.Repeat("b", 32))
	if got := other.Decrypt(enc); got == "value" {
		t.Fatal("wrong key must not decrypt")
	}
}
