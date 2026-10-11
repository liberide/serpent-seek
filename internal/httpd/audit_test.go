package httpd

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAuditEndpointRecordsChainCreate(t *testing.T) {
	ts := newTestServer(t, true)

	chain := map[string]any{
		"name": "audited",
		"nodes": []map[string]any{
			{"key": "a", "provider_id": "stub_ok", "timeout_ms": 1000, "retries": 0, "retry_delay_ms": 0,
				"delay_policy": "none", "on_success": "stop", "on_empty": "next", "on_fail": "next", "is_start": true},
		},
	}
	created := ts.do(t, http.MethodPost, "/api/chains", chain, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("create chain status %d: %s", created.Code, created.Body.String())
	}

	rec := ts.do(t, http.MethodGet, "/api/audit?action=chain.create", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit status %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			Action  string            `json:"action"`
			Target  string            `json:"target"`
			User    string            `json:"user"`
			Via     string            `json:"via"`
			Details map[string]string `json:"details"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected exactly one chain.create audit entry, got total=%d len=%d", payload.Total, len(payload.Items))
	}
	entry := payload.Items[0]
	if entry.User != "admin" || entry.Via != "key" || entry.Target == "" || entry.Details["name"] != "audited" {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
}
