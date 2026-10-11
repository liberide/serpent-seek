package store

import (
	"encoding/base64"
	"strings"
)

// cursorSep separates the ordered keyset values inside a cursor. It cannot
// appear in RFC3339 timestamps or ids.
const cursorSep = "\x1f"

// EncodeCursor packs the ordered keyset values of the last row of a page into
// an opaque, URL-safe token. Callers must pass the values in the same order as
// the ORDER BY clause.
func EncodeCursor(values ...string) string {
	if len(values) == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(values, cursorSep)))
}

// DecodeCursor reverses EncodeCursor. It returns ok=false for a malformed or
// empty token so callers can fall back to offset pagination.
func DecodeCursor(token string) ([]string, bool) {
	if token == "" {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, false
	}
	if len(raw) == 0 {
		return nil, false
	}
	return strings.Split(string(raw), cursorSep), true
}
