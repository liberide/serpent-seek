package httpd

import (
	"net/http"
	"strings"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/providers"
)

type searchRequest struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// handleSearch is the Open WebUI-compatible machine endpoint. It always answers
// HTTP 200 with a rows array (or []) so a failing chain never breaks Open WebUI.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var body searchRequest
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Query) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "query is required")
		return
	}
	client := "api"
	if id := s.identity(r); id != nil && id.User != nil {
		client = id.User.Name
	}
	out, err := s.engine.Execute(r.Context(), engine.Input{Query: body.Query, Count: body.Count, Client: client})
	if err != nil {
		s.log.Error("", "", "search failed: "+err.Error())
		w.Header().Set("X-Serpent-Api", "-")
		writeJSON(w, http.StatusOK, providers.Rows{})
		return
	}
	w.Header().Set("X-Serpent-Rid", out.Request.RID)
	w.Header().Set("X-Serpent-Api", out.Request.UsedProvider)
	rows := out.Rows
	if rows == nil {
		rows = providers.Rows{}
	}
	// The machine endpoint keeps the Open WebUI row shape; merge provenance
	// (sources) is only exposed in the history/dedupe view.
	for i := range rows {
		rows[i].Sources = nil
	}
	writeJSON(w, http.StatusOK, rows)
}

// handleSearchUI starts an asynchronous search for the playground and returns
// the request id so the SPA can subscribe to the live SSE trace.
func (s *Server) handleSearchUI(w http.ResponseWriter, r *http.Request) {
	var body searchRequest
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Query) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "query is required")
		return
	}
	client := "ui"
	if id := s.identity(r); id != nil && id.User != nil {
		client = id.User.Name
	}
	req, err := s.engine.Start(r.Context(), engine.Input{Query: body.Query, Count: body.Count, Client: client})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "search_start_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": req.ID, "rid": req.RID})
}

var _ = auth.ClientIP
