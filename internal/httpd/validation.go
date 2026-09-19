// Package httpd wires the HTTP server: routing, handlers, SSE and SPA serving.
package httpd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/auth"
)

// MaxRequestBody caps incoming JSON bodies (64 KiB, matching serpent-shim).
const MaxRequestBody = 64 * 1024

// errorBody is the canonical API error envelope.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON serializes v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError writes the canonical error envelope.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// readJSON decodes a size-limited JSON body.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, r, http.StatusBadRequest, "body_too_large", "request body exceeds 64KiB")
			return false
		}
		if errors.Is(err, io.EOF) {
			return true // empty body is allowed for endpoints with optional payloads
		}
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// pagination reads limit/page query parameters.
func pagination(r *http.Request, defaultLimit int) (limit, offset, page int) {
	page = 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	limit = defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset = (page - 1) * limit
	return limit, offset, page
}

// newID returns a new random entity id.
func newID() string { return uuid.NewString() }

// identity returns the current identity (nil when auth is disabled and no
// session is present, in which case a synthetic admin is used).
func (s *Server) identity(r *http.Request) *auth.Identity {
	return auth.IdentityFrom(r.Context())
}
