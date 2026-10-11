package httpd

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// alwaysAllowedEndpoints are reachable by every authenticated identity, even a
// developer token restricted to a subset of endpoints: they are identity/UI
// plumbing, not API capabilities.
var alwaysAllowedEndpoints = map[string]bool{
	"GET /api/me":           true,
	"POST /api/auth/logout": true,
}

// authenticate is the identity middleware plus the per-endpoint permission guard
// for developer tokens. It wraps auth.Middleware.Authenticate so the guard runs
// only after the identity is resolved and after chi has matched the route.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return s.mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.endpointAllowed(r) {
			writeError(w, r, http.StatusForbidden, "endpoint_forbidden",
				"this developer token is not allowed to call this endpoint")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// endpointAllowed enforces a restricted developer token's permission list.
// Sessions, regular keys and full-access developer tokens are always allowed.
func (s *Server) endpointAllowed(r *http.Request) bool {
	id := s.identity(r)
	if id == nil || !id.Developer || id.AllowsAll() {
		return true
	}
	endpoint := strings.ToUpper(r.Method) + " " + normalizeRoutePattern(routePattern(r))
	return alwaysAllowedEndpoints[endpoint] || id.AllowsEndpoint(endpoint)
}

// routePattern returns the chi route pattern matched for the request, or "".
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}

// normalizeRoutePattern trims the trailing slash chi adds to mounted collection
// routes ("/api/chains/" -> "/api/chains") so ids match the spec/route catalog.
func normalizeRoutePattern(pattern string) string {
	if pattern == "" || pattern == "/" {
		return pattern
	}
	return strings.TrimSuffix(pattern, "/")
}
