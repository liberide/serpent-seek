package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
)

type contextKey int

const identityKey contextKey = iota

// Identity is the authenticated principal for a request.
type Identity struct {
	User   *store.User
	Scopes map[string]bool
	Via    string // session|key|disabled
	KeyID  string
}

// HasScope reports whether the identity carries a scope.
func (i *Identity) HasScope(scope string) bool {
	if i == nil {
		return false
	}
	if i.Via == "disabled" {
		return true
	}
	return i.Scopes[scope]
}

// IsAdmin reports admin capability.
func (i *Identity) IsAdmin() bool { return i.HasScope("admin") }

// WithIdentity stores an identity in the context.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom returns the identity from the context (may be nil).
func IdentityFrom(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}

// Authenticator resolves request identities.
type Authenticator struct {
	Store    store.Storage
	Settings *config.Manager
	Log      *logging.Logger
}

// AuthEnabled reports whether authentication is currently required.
func (a *Authenticator) AuthEnabled(ctx context.Context) bool {
	return a.Settings.GetBool(ctx, config.KeyAuthEnabled)
}

// Secure reports whether cookies should carry the Secure attribute.
func (a *Authenticator) Secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	origin := strings.ToLower(a.Settings.GetString(r.Context(), config.KeyPublicOrigin))
	return strings.HasPrefix(origin, "https://")
}

// Resolve determines the identity from the Authorization header or session
// cookie. It returns (nil, nil) when no credentials are present.
func (a *Authenticator) Resolve(r *http.Request) (*Identity, error) {
	ctx := r.Context()
	if !a.AuthEnabled(ctx) {
		return &Identity{Via: "disabled", Scopes: map[string]bool{"admin": true, "search": true, "read": true}}, nil
	}
	if header := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return a.resolveKey(ctx, strings.TrimSpace(header[7:]))
	}
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		return a.resolveSession(ctx, cookie.Value)
	}
	return nil, nil
}

func (a *Authenticator) resolveSession(ctx context.Context, token string) (*Identity, error) {
	_, user, err := LookupSession(ctx, a.Store, token)
	if err != nil {
		return nil, ErrInvalidKey
	}
	scopes := map[string]bool{"search": true, "read": true}
	if user.Role == "admin" {
		scopes["admin"] = true
	}
	return &Identity{User: user, Scopes: scopes, Via: "session"}, nil
}

func (a *Authenticator) resolveKey(ctx context.Context, token string) (*Identity, error) {
	kind, prefix, secret, ok := ParseKey(token)
	if !ok {
		return nil, ErrInvalidKey
	}
	key, err := a.Store.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return nil, ErrInvalidKey
	}
	if key.RevokedAt != "" {
		return nil, ErrInvalidKey
	}
	if key.ExpiresAt != "" {
		if expires, perr := time.Parse(time.RFC3339Nano, key.ExpiresAt); perr == nil && time.Now().UTC().After(expires) {
			return nil, ErrInvalidKey
		}
	}
	if !VerifySecret(key.Hash, secret) {
		return nil, ErrInvalidKey
	}
	user, err := a.Store.GetUser(ctx, key.UserID)
	if err != nil || user.Disabled {
		return nil, ErrInvalidKey
	}
	scopes := map[string]bool{}
	for _, s := range strings.Split(key.Scopes, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			scopes[s] = true
		}
	}
	if kind == "admin" || user.Role == "admin" {
		scopes["admin"] = true
	}
	scopes["search"] = true
	key.LastUsedAt = store.Now()
	_ = a.Store.UpdateAPIKey(ctx, key)
	return &Identity{User: user, Scopes: scopes, Via: "key", KeyID: key.ID}, nil
}

// LoginWithKey validates an API key for the login endpoint, regardless of the
// current AUTH_ENABLED toggle.
func (a *Authenticator) LoginWithKey(ctx context.Context, token string) (*Identity, error) {
	return a.resolveKey(ctx, token)
}

// ClientIP extracts the best-effort client IP.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma > 0 {
			return strings.TrimSpace(forwarded[:comma])
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware wires identity resolution, CSRF protection and scope guards.
type Middleware struct {
	Auth *Authenticator
}

// Authenticate resolves the identity and enforces CSRF on cookie mutations.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.Auth.AuthEnabled(r.Context()) {
			ctx := WithIdentity(r.Context(), &Identity{Via: "disabled", Scopes: map[string]bool{"admin": true, "search": true, "read": true}})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		id, err := m.Auth.Resolve(r)
		if err != nil || id == nil {
			writeUnauthorized(w)
			return
		}
		if id.Via == "session" && !ValidateCSRF(r) {
			writeForbidden(w, "csrf validation failed")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireAdmin rejects non-admin identities.
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := IdentityFrom(r.Context())
		if id == nil || !id.IsAdmin() {
			writeForbidden(w, "admin scope required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope rejects identities missing a scope.
func (m *Middleware) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFrom(r.Context())
			if id == nil || !id.HasScope(scope) {
				writeForbidden(w, "scope "+scope+" required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Optional resolves an identity without rejecting anonymous requests.
func (m *Middleware) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := m.Auth.Resolve(r)
		if err == nil && id != nil {
			r = r.WithContext(WithIdentity(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"authentication required"}}`))
}

func writeForbidden(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"` + reason + `"}}`))
}
