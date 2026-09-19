package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/version"
)

// MCP (Model Context Protocol) server.
//
// SerpentSeek exposes a single `search` tool so AI assistants (Claude Desktop,
// Cursor, VS Code, etc.) can run searches through the configured provider chain.
//
// The endpoint supports the official MCP "Streamable HTTP" transport:
//   - GET /mcp  opens an SSE stream associated with a session id.
//   - POST /mcp sends JSON-RPC requests. Without a session id it returns a
//     single JSON-RPC response in the POST body. With a session id the response
//     is delivered over the matching SSE stream.
//
// For clients that only understand plain HTTP JSON-RPC 2.0, POST /mcp without a
// session id continues to work as a simple request/response endpoint.

const (
	mcpPreferredVersion = "2025-03-26"
	mcpServerName       = "serpentseek"
)

// mcpSupportedVersions are the protocol revisions this server can speak. The
// client advertises its preferred revision in `initialize`; if we support it we
// echo it back, otherwise we fall back to a stable preferred version.
var mcpSupportedVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05", "2024-10-07"}

// rpcRequest is a single JSON-RPC 2.0 request or notification. A missing id
// marks a notification (no response is sent).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcResponse is a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC standard error codes.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// mcpSession holds one client session for the streamable HTTP transport.
type mcpSession struct {
	id       string
	ctx      context.Context
	cancel   context.CancelFunc
	outgoing chan rpcResponse
	created  time.Time
}

// mcpSessionStore keeps active MCP SSE sessions.
type mcpSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*mcpSession
	ttl      time.Duration
}

func newMCPSessionStore() *mcpSessionStore {
	return &mcpSessionStore{
		sessions: map[string]*mcpSession{},
		ttl:      30 * time.Minute,
	}
}

func (st *mcpSessionStore) create() *mcpSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	s := &mcpSession{
		id:       uuid.NewString(),
		ctx:      ctx,
		cancel:   cancel,
		outgoing: make(chan rpcResponse, 256),
		created:  time.Now(),
	}
	st.sessions[s.id] = s
	return s
}

func (st *mcpSessionStore) get(id string) *mcpSession {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.sessions[id]
}

func (st *mcpSessionStore) remove(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s, ok := st.sessions[id]; ok {
		s.cancel()
		delete(st.sessions, id)
	}
}

func (st *mcpSessionStore) closeAll() {
	st.mu.Lock()
	defer st.mu.Unlock()
	for id, s := range st.sessions {
		s.cancel()
		delete(st.sessions, id)
	}
}

func (st *mcpSessionStore) reap() {
	st.mu.Lock()
	defer st.mu.Unlock()
	cutoff := time.Now().Add(-st.ttl)
	for id, s := range st.sessions {
		if s.created.Before(cutoff) {
			s.cancel()
			delete(st.sessions, id)
		}
	}
}

// mcpSessionID extracts the Mcp-Session-Id header value.
func mcpSessionID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Mcp-Session-Id"))
}

// handleMCP dispatches one JSON-RPC message. It supports both simple HTTP
// request/response and the streamable HTTP transport.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleMCPStream(w, r)
	case http.MethodPost:
		s.handleMCPPost(w, r)
	case http.MethodDelete:
		s.handleMCPDelete(w, r)
	default:
		writeRPC(w, nil, nil, &rpcError{Code: rpcInvalidRequest, Message: "method not allowed"})
	}
}

// handleMCPStream opens a new SSE stream and returns its session id in the
// Mcp-Session-Id header. Responses and server-initiated notifications are sent
// as SSE events of type "message" whose data is a JSON-RPC object.
func (s *Server) handleMCPStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeRPC(w, nil, nil, &rpcError{Code: rpcInternalError, Message: "streaming unsupported"})
		return
	}

	s.mcpSessions.reap()
	session := s.mcpSessions.create()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Mcp-Session-Id", session.id)
	w.WriteHeader(http.StatusOK)

	// Send an empty comment to force headers to the client.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.mcpSessions.remove(session.id)
			return
		case <-session.ctx.Done():
			return
		case resp, open := <-session.outgoing:
			if !open {
				return
			}
			if err := s.writeMCPStreamEvent(w, resp); err != nil {
				s.mcpSessions.remove(session.id)
				return
			}
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// writeMCPStreamEvent writes one JSON-RPC message as an SSE event.
func (s *Server) writeMCPStreamEvent(w http.ResponseWriter, msg rpcResponse) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	return err
}

// handleMCPPost reads a JSON-RPC message and either returns the response in
// the POST body (no session) or forwards it to the matching SSE stream.
func (s *Server) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxRequestBody))
	if err := dec.Decode(&req); err != nil {
		writeRPC(w, nil, nil, &rpcError{Code: rpcParseError, Message: "parse error: " + err.Error()})
		return
	}
	if req.JSONRPC != "2.0" {
		writeRPC(w, req.ID, nil, &rpcError{Code: rpcInvalidRequest, Message: "invalid request: jsonrpc must be \"2.0\""})
		return
	}

	sid := mcpSessionID(r)
	if sid != "" {
		// Streamable HTTP: deliver the response (or a notification result)
		// over the SSE stream associated with this session.
		session := s.mcpSessions.get(sid)
		if session == nil {
			writeRPC(w, req.ID, nil, &rpcError{Code: rpcInvalidRequest, Message: "unknown session"})
			return
		}
		if len(req.ID) == 0 {
			// Notification: process in the background, no response required.
			go s.dispatchMCP(r, &req)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		go s.dispatchAndSend(session, r, &req)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Simple HTTP JSON-RPC 2.0 fallback: notifications are acknowledged
	// with HTTP 202 and an empty body; requests get a JSON-RPC response body.
	if len(req.ID) == 0 {
		go s.dispatchMCP(r, &req)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	res, rpcErr := s.dispatchMCP(r, &req)
	if rpcErr != nil {
		writeRPC(w, req.ID, nil, rpcErr)
		return
	}
	writeRPC(w, req.ID, res, nil)
}

// dispatchAndSend runs the JSON-RPC method and sends the result to the SSE
// session. It also handles cancellation notifications for in-flight requests.
func (s *Server) dispatchAndSend(session *mcpSession, r *http.Request, req *rpcRequest) {
	res, rpcErr := s.dispatchMCP(r, req)
	if rpcErr != nil {
		res = nil
	}
	select {
	case <-session.ctx.Done():
		return
	case session.outgoing <- rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: res, Error: rpcErr}:
	}
}

// handleMCPDelete terminates the SSE session referenced by Mcp-Session-Id.
func (s *Server) handleMCPDelete(w http.ResponseWriter, r *http.Request) {
	sid := mcpSessionID(r)
	if sid == "" {
		writeRPC(w, nil, nil, &rpcError{Code: rpcInvalidRequest, Message: "missing Mcp-Session-Id"})
		return
	}
	s.mcpSessions.remove(sid)
	w.WriteHeader(http.StatusOK)
}

// dispatchMCP routes a request to the matching JSON-RPC method.
func (s *Server) dispatchMCP(r *http.Request, req *rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.mcpInitialize(req)
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": []any{mcpSearchTool()}}, nil
	case "tools/call":
		return s.mcpToolCall(r, req)
	default:
		return nil, &rpcError{Code: rpcMethodNotFound, Message: "method not found: " + req.Method}
	}
}

// mcpInitialize negotiates the protocol revision and reports capabilities.
func (s *Server) mcpInitialize(req *rpcRequest) (any, *rpcError) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "invalid initialize params"}
		}
	}
	proto := mcpPreferredVersion
	if params.ProtocolVersion != "" && containsString(mcpSupportedVersions, params.ProtocolVersion) {
		proto = params.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": proto,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo": map[string]any{
			"name":    mcpServerName,
			"version": version.Version,
		},
	}, nil
}

// mcpSearchTool describes the `search` tool for tools/list.
func mcpSearchTool() map[string]any {
	return map[string]any{
		"name":        "search",
		"description": "Search the web through SerpentSeek's configured provider chain. Returns a list of results with title, URL and snippet.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query text.",
				},
				"count": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     20,
					"default":     5,
					"description": "Maximum number of results to return. 0 returns everything found (no limit). Defaults to 5.",
				},
			},
			"required": []string{"query"},
		},
	}
}

// mcpToolCall executes a tool invocation. Only `search` is implemented.
func (s *Server) mcpToolCall(r *http.Request, req *rpcRequest) (any, *rpcError) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "invalid tools/call params"}
	}
	if params.Name != "search" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "unknown tool: " + params.Name}
	}

	var args struct {
		Query string `json:"query"`
		Count *int   `json:"count"`
	}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "invalid search arguments"}
		}
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "missing required argument: query"}
	}
	count := 5
	if args.Count != nil {
		count = *args.Count
	}

	client := "mcp"
	if id := s.identity(r); id != nil && id.User != nil {
		client = id.User.Name
	}
	out, err := s.engine.Execute(r.Context(), engine.Input{Query: query, Count: count, Client: client})
	if err != nil {
		s.log.Error("", "", "mcp search failed: "+err.Error())
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Search failed: " + err.Error()}},
			"isError": true,
		}, nil
	}

	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": formatSearchResults(query, out.Rows)}},
		"isError": false,
	}, nil
}

// formatSearchResults renders search rows as a compact markdown list suitable
// for consumption by a language model.
func formatSearchResults(query string, rows providers.Rows) string {
	if len(rows) == 0 {
		return fmt.Sprintf("No results found for %q.", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d result(s) for %q:\n", len(rows), query)
	for i, row := range rows {
		title := row.Title
		if strings.TrimSpace(title) == "" {
			title = row.Link
		}
		fmt.Fprintf(&b, "\n%d. %s\n   %s\n", i+1, title, row.Link)
		if s := strings.TrimSpace(row.Snippet); s != "" {
			fmt.Fprintf(&b, "   %s\n", s)
		}
	}
	return b.String()
}

// writeRPC serializes a JSON-RPC response.
func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, rpcErr *rpcError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr}
	_ = json.NewEncoder(w).Encode(resp)
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
