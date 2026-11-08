package httpd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleListRequests returns a filtered history page. A `cursor` from a
// previous response's next_cursor takes precedence over `page`.
func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset, page := pagination(r, 50)
	q := r.URL.Query()
	cursor := q.Get("cursor")
	filter := store.RequestFilter{
		Status:   q.Get("status"),
		Provider: q.Get("provider"),
		From:     q.Get("from"),
		To:       q.Get("to"),
		Query:    q.Get("q"),
		Cursor:   cursor,
	}
	if cursor != "" {
		offset, page = 0, 1
	}
	// Scope to the caller for non-admins.
	if !s.isAdmin(r) {
		filter.UserID = s.identityUserID(r)
	}
	items, total, err := s.store.ListRequests(ctx, filter, store.Page{Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if items == nil {
		items = []*store.Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
		"next_cursor": nextRequestCursor(items, limit),
	})
}

// nextRequestCursor returns the keyset cursor of the last row, or nil when the
// page is the last one.
func nextRequestCursor(items []*store.Request, limit int) any {
	if limit <= 0 || len(items) < limit {
		return nil
	}
	last := items[len(items)-1]
	return store.EncodeCursor(last.CreatedAt, last.ID)
}

// exportPageSize is the keyset page size for exports.
const exportPageSize = 500

// exportMaxRows caps one export.
const exportMaxRows = 100_000

// exportRow is one history entry; results, steps and chain snapshots are omitted.
type exportRow struct {
	ID           string `json:"id"`
	RID          string `json:"rid"`
	CreatedAt    string `json:"created_at"`
	Query        string `json:"query"`
	Status       string `json:"status"`
	UsedProvider string `json:"used_provider"`
	ResultsCount int    `json:"results_count"`
	TotalMS      int    `json:"total_ms"`
	StepsCount   int    `json:"steps_count"`
	Client       string `json:"client"`
	Error        string `json:"error"`
	UserID       string `json:"user_id"`
}

// handleExportRequests streams filtered history as JSONL or CSV with keyset
// pagination. Non-admins only export their own rows.
func (s *Server) handleExportRequests(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}
	if format != "jsonl" && format != "csv" {
		writeError(w, r, http.StatusBadRequest, "bad_format", "format must be jsonl or csv")
		return
	}

	q := r.URL.Query()
	filter := store.RequestFilter{
		Status:   q.Get("status"),
		Provider: q.Get("provider"),
		From:     q.Get("from"),
		To:       q.Get("to"),
		Query:    q.Get("q"),
	}
	if !s.isAdmin(r) {
		filter.UserID = s.identityUserID(r)
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="serpentseek-history.csv"`)
	} else {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="serpentseek-history.jsonl"`)
	}
	w.Header().Set("Cache-Control", "no-store")

	ctx := r.Context()
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	flusher, _ := w.(http.Flusher)
	jsonEnc := json.NewEncoder(bw)
	csvW := csv.NewWriter(bw)
	if format == "csv" {
		_ = csvW.Write([]string{"id", "rid", "created_at", "query", "status", "used_provider",
			"results_count", "total_ms", "steps_count", "client", "error", "user_id"})
	}

	writeRow := func(item *store.Request) {
		row := exportRow{
			ID: item.ID, RID: item.RID, CreatedAt: item.CreatedAt, Query: item.Query,
			Status: item.Status, UsedProvider: item.UsedProvider, ResultsCount: item.ResultsCount,
			TotalMS: item.TotalMS, StepsCount: item.StepsCount, Client: item.Client,
			Error: item.Error, UserID: item.UserID,
		}
		if format == "csv" {
			_ = csvW.Write([]string{row.ID, row.RID, row.CreatedAt, row.Query, row.Status, row.UsedProvider,
				strconv.Itoa(row.ResultsCount), strconv.Itoa(row.TotalMS), strconv.Itoa(row.StepsCount),
				row.Client, row.Error, row.UserID})
		} else {
			_ = jsonEnc.Encode(row)
		}
	}

	cursor := ""
	written := 0
	for written < exportMaxRows {
		pageFilter := filter
		pageFilter.Cursor = cursor
		items, _, err := s.store.ListRequests(ctx, pageFilter, store.Page{Limit: exportPageSize})
		if err != nil {
			// Headers are already sent; truncate the export.
			s.log.Warn("", "", "history export truncated: "+err.Error())
			break
		}
		for _, item := range items {
			writeRow(item)
		}
		written += len(items)
		if format == "csv" {
			csvW.Flush()
		}
		_ = bw.Flush()
		if flusher != nil {
			flusher.Flush()
		}
		if len(items) < exportPageSize {
			break
		}
		last := items[len(items)-1]
		cursor = store.EncodeCursor(last.CreatedAt, last.ID)
	}
}

// handleGetRequest returns one request with steps and its chain snapshot.
func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	req, err := s.store.GetRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "request not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if !s.isAdmin(r) {
		if owner := s.identityUserID(r); owner == "" || req.UserID != owner {
			writeError(w, r, http.StatusNotFound, "not_found", "request not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, req)
}
