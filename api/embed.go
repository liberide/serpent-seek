// Package api embeds the OpenAPI specification so the running server can expose
// it (GET /openapi.yaml) for API clients and for the Developer section.
package api

import _ "embed"

// OpenAPISpec is the raw OpenAPI 3.1 document (api/openapi.yaml). It stays the
// single source of truth for the HTTP contract and is guarded against drift by
// internal/httpd/openapi_spec_test.go.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
