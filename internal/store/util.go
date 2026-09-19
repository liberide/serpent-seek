package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Now returns the current UTC timestamp in RFC3339 format.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// EncodeMap serialises a string map to compact JSON (never returns an error).
func EncodeMap(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// DecodeMap parses a JSON string map, tolerating empty/invalid input.
func DecodeMap(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// EncodeJSON serialises any value to JSON.
func EncodeJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeJSON parses JSON into v; a blank or invalid payload is ignored.
func DecodeJSON(s string, v any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

// Bool is a portable boolean that scans INTEGER (SQLite) and BOOLEAN
// (PostgreSQL) columns.
type Bool bool

// Scan implements sql.Scanner.
func (b *Bool) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*b = false
	case bool:
		*b = Bool(v)
	case int64:
		*b = v != 0
	case int:
		*b = v != 0
	case []byte:
		*b = len(v) == 1 && v[0] != '0' && v[0] != 'f'
	case string:
		*b = v == "1" || v == "true" || v == "t"
	default:
		return fmt.Errorf("store: cannot scan %T into Bool", src)
	}
	return nil
}

// Value implements driver.Valuer.
func (b Bool) Value() (driver.Value, error) { return bool(b), nil }
