package httpd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/store"
)

// seedExportRows inserts n history rows owned by the test user.
func seedExportRows(t *testing.T, ts *testServer, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := ts.store.CreateRequest(t.Context(), &store.Request{
			ID: uuid.NewString(), RID: uuid.NewString()[:8], Query: "query",
			Status: "ok", UsedProvider: "stub_ok", ResultsCount: i, TotalMS: 42,
			StepsCount: 1, Client: "test", UserID: ts.user.ID, CreatedAt: store.Now(),
		}); err != nil {
			t.Fatalf("seed request: %v", err)
		}
	}
}

func TestExportRequestsJSONL(t *testing.T) {
	ts := newTestServer(t, true)
	seedExportRows(t, ts, 5)

	rec := ts.do(t, http.MethodGet, "/api/requests/export?format=jsonl", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "x-ndjson") {
		t.Fatalf("Content-Type = %q, want ndjson", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}

	var rows []exportRow
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	for sc.Scan() {
		var row exportRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatalf("line is not valid JSON: %q: %v", sc.Text(), err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	if rows[0].Query != "query" || rows[0].UsedProvider != "stub_ok" || rows[0].UserID == "" {
		t.Fatalf("unexpected row payload: %+v", rows[0])
	}
}

func TestExportRequestsCSV(t *testing.T) {
	ts := newTestServer(t, true)
	seedExportRows(t, ts, 3)

	rec := ts.do(t, http.MethodGet, "/api/requests/export?format=csv", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	r := csv.NewReader(strings.NewReader(rec.Body.String()))
	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(all) != 4 { // header + 3 rows
		t.Fatalf("csv rows = %d, want 4", len(all))
	}
	if all[0][0] != "id" || all[0][3] != "query" {
		t.Fatalf("unexpected header: %v", all[0])
	}
}

func TestExportRequestsValidatesFormat(t *testing.T) {
	ts := newTestServer(t, true)
	if rec := ts.do(t, http.MethodGet, "/api/requests/export?format=xml", nil, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestExportRequestsRespectsScopeAndFilters(t *testing.T) {
	ts := newTestServer(t, true)
	seedExportRows(t, ts, 2)

	// Anonymous request: reject.
	if rec := ts.do(t, http.MethodGet, "/api/requests/export", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", rec.Code)
	}
	// Empty result stream.
	rec := ts.do(t, http.MethodGet, "/api/requests/export?status=fail", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "" {
		t.Fatalf("expected empty jsonl stream, got %q", rec.Body.String())
	}
}
