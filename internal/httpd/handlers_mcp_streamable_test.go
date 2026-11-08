package httpd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMCPStreamableRealServer verifies that a session-bound tools/call
// completes over the SSE stream after the POST request context is canceled.
func TestMCPStreamableRealServer(t *testing.T) {
	ts := newTestServer(t, true)
	srv := httptest.NewServer(ts.server.Routes())
	defer srv.Close()

	// 1) Open the SSE stream (GET /mcp).
	getReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	getReq.Header.Set("Authorization", "Bearer "+ts.key)
	resp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer resp.Body.Close()
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("no Mcp-Session-Id header")
	}

	events := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var data strings.Builder
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data: ") {
				data.WriteString(strings.TrimPrefix(line, "data: "))
			} else if line == "" && data.Len() > 0 {
				events <- data.String()
				data.Reset()
			}
		}
	}()

	// 2) POST tools/call bound to the session.
	payload := map[string]any{
		"jsonrpc": "2.0", "id": 42, "method": "tools/call",
		"params": map[string]any{"name": "search", "arguments": map[string]any{"query": "hello"}},
	}
	body, _ := json.Marshal(payload)
	postReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+ts.key)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Mcp-Session-Id", sid)
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", postResp.StatusCode)
	}

	// 3) The SSE stream must deliver the search result.
	select {
	case ev := <-events:
		if strings.Contains(ev, "context canceled") || strings.Contains(ev, `"isError":true`) {
			t.Fatalf("tools/call over the streamable transport failed: %s", ev)
		}
		if !strings.Contains(ev, `"id":42`) || !strings.Contains(ev, "https://a") {
			t.Fatalf("unexpected SSE payload: %s", ev)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("no response delivered over the SSE stream")
	}
}
