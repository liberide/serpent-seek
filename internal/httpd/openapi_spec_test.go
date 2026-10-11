package httpd

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/auth"
)

// TestOpenAPISpecMatchesRouter guards against documentation drift: every route
// registered on the chi router must be described in api/openapi.yaml and every
// documented path must exist. It intentionally avoids a YAML dependency (a
// small line scanner keyed on the spec's 2-/4-space indentation) so it runs as
// part of `go test ./...` in CI.
func TestOpenAPISpecMatchesRouter(t *testing.T) {
	// Routes() only needs the middleware holder to build the tree; handlers are
	// never invoked by chi.Walk.
	srv := &Server{mw: &auth.Middleware{}}
	router, ok := srv.Routes().(chi.Routes)
	if !ok {
		t.Fatal("Routes() does not implement chi.Routes")
	}

	routerPaths := map[string]map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = normalizeSpecRoute(route)
		if route == "" {
			return nil
		}
		if routerPaths[route] == nil {
			routerPaths[route] = map[string]bool{}
		}
		routerPaths[route][strings.ToLower(method)] = true
		return nil
	}); err != nil {
		t.Fatalf("walk router: %v", err)
	}

	specPaths, err := parseOpenAPIPaths(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	for route := range routerPaths {
		if _, ok := specPaths[route]; !ok {
			t.Errorf("router path %q is not documented in api/openapi.yaml", route)
		}
	}
	for path := range specPaths {
		if _, ok := routerPaths[path]; !ok {
			t.Errorf("openapi path %q has no matching router route", path)
			continue
		}
		// Every method documented for a path must be served. The reverse is not
		// asserted because chi's Handle() registers every method (e.g. /mcp).
		for method := range specPaths[path] {
			if !routerPaths[path][method] {
				t.Errorf("openapi documents %s %s but the router does not serve it", strings.ToUpper(method), path)
			}
		}
	}
}

// normalizeSpecRoute trims the trailing slash chi adds to collection routes
// (/api/chains/ -> /api/chains) and drops wildcard routes (the SPA fallback).
func normalizeSpecRoute(route string) string {
	if route == "/" {
		return route
	}
	if strings.Contains(route, "*") {
		return ""
	}
	return strings.TrimSuffix(route, "/")
}

// parseOpenAPIPaths extracts path -> set(method) from the top-level `paths:`
// block, keyed on the spec's 2-space path / 4-space method indentation.
func parseOpenAPIPaths(file string) (map[string]map[string]bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]map[string]bool{}
	inPaths := false
	current := ""
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !inPaths {
			if strings.HasPrefix(line, "paths:") {
				inPaths = true
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != ' ' {
			break // left the paths block (e.g. components:)
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if key == "" || strings.HasPrefix(key, "#") {
			continue
		}
		switch indent {
		case 2:
			current = key
			if out[current] == nil {
				out[current] = map[string]bool{}
			}
		case 4:
			switch key {
			case "get", "post", "put", "patch", "delete", "head", "options", "trace":
				if current != "" && out[current] != nil {
					out[current][key] = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
