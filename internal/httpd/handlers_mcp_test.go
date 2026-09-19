package httpd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMCPEndpoint exercises the JSON-RPC-over-HTTP MCP surface exposed at
// /mcp: auth, initialize, tools/list and a real tools/call through the engine.
func TestMCPEndpoint(t *testing.T) {
	ts := newTestServer(t, true)

	// The endpoint is guarded by the search scope (Bearer key required).
	if rec := ts.do(t, http.MethodPost, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", rec.Code)
	}

	// initialize negotiates the protocol revision and reports server info.
	rec := ts.do(t, http.MethodPost, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize status %d: %s", rec.Code, rec.Body.String())
	}
	var initResp struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("unmarshal initialize: %v", err)
	}
	if initResp.Result.ProtocolVersion != "2025-06-18" || initResp.Result.ServerInfo.Name != "serpentseek" {
		t.Fatalf("unexpected initialize result: %s", rec.Body.String())
	}

	// tools/list advertises the search tool.
	rec = ts.do(t, http.MethodPost, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"name":"search"`)) {
		t.Fatalf("tools/list should include the search tool: %d %s", rec.Code, rec.Body.String())
	}

	// tools/call runs a real search through the stub provider.
	rec = ts.do(t, http.MethodPost, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "search",
			"arguments": map[string]any{"query": "hello", "count": 3},
		},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call status %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("https://a")) {
		t.Fatalf("tools/call should contain the stub result link: %s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"isError":true`)) {
		t.Fatalf("tools/call should not be an error: %s", rec.Body.String())
	}

	// count omitted: defaults to 5 and must still succeed.
	rec = ts.do(t, http.MethodPost, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params":  map[string]any{"name": "search", "arguments": map[string]any{"query": "hello"}},
	}, true)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte(`"isError":true`)) {
		t.Fatalf("tools/call without count should default to 5 and succeed: %d %s", rec.Code, rec.Body.String())
	}

	// Unknown tool yields a JSON-RPC invalid-params error.
	rec = ts.do(t, http.MethodPost, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params":  map[string]any{"name": "nope", "arguments": map[string]any{}},
	}, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"code":-32602`)) {
		t.Fatalf("unknown tool should return -32602: %d %s", rec.Code, rec.Body.String())
	}

	// A notification (no id) is acknowledged with 202 and no body.
	rec = ts.do(t, http.MethodPost, "/mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, true)
	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Fatalf("notification should be 202 with empty body, got %d %q", rec.Code, rec.Body.String())
	}
}

// TestMCPStreamableHTTP exercises the SSE side of the streamable HTTP transport:
// GET /mcp returns a session id, POST /mcp with that session id delivers the
// response over the SSE stream, and DELETE /mcp terminates the session.
func TestMCPStreamableHTTP(t *testing.T) {
	ts := newTestServer(t, true)

	// 1) Open SSE stream.
	getReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	getReq.Header.Set("Authorization", "Bearer "+ts.key)
	getReq.Header.Set("Accept", "text/event-stream")
	getRec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		ts.server.Routes().ServeHTTP(getRec, getReq)
		close(done)
	}()

	// Wait until the SSE headers/session id are available.
	var sid string
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		sid = getRec.Header().Get("Mcp-Session-Id")
		if sid != "" {
			break
		}
	}
	if sid == "" {
		t.Fatalf("GET /mcp must return Mcp-Session-Id")
	}

	// 2) POST a request with the session id.
	postRec := ts.do(t, http.MethodPost, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "tools/list",
	}, true, map[string]string{"Mcp-Session-Id": sid})
	if postRec.Code != http.StatusAccepted || postRec.Body.Len() != 0 {
		t.Fatalf("streamable POST should be 202 with empty body, got %d %s", postRec.Code, postRec.Body.String())
	}

	// 3) Read the response from the SSE stream.
	var gotResponse bool
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-done:
			// Stream closed; check if we already got the response.
			body := getRec.Body.String()
			if strings.Contains(body, `"id":7`) && strings.Contains(body, `"name":"search"`) {
				gotResponse = true
			}
			break loop
		case <-deadline:
			t.Fatalf("timeout waiting for SSE response")
		case <-time.After(50 * time.Millisecond):
			body := getRec.Body.String()
			if strings.Contains(body, `"id":7`) && strings.Contains(body, `"name":"search"`) {
				gotResponse = true
				break loop
			}
		}
	}
	if !gotResponse {
		t.Fatalf("did not receive tools/list response over SSE: %s", getRec.Body.String())
	}

	// 4) DELETE terminates the session.
	delRec := ts.do(t, http.MethodDelete, "/mcp", nil, true, map[string]string{"Mcp-Session-Id": sid})
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE /mcp should return 200, got %d", delRec.Code)
	}
}
