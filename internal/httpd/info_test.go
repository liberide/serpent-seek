package httpd

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestInfoEndpoint(t *testing.T) {
	ts := newTestServer(t, true)
	rec := ts.do(t, http.MethodGet, "/api/info", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("info status %d: %s", rec.Code, rec.Body.String())
	}
	var info struct {
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		GoVersion     string `json:"go_version"`
		StorageDriver string `json:"storage_driver"`
		StartedAt     string `json:"started_at"`
		UptimeSeconds int    `json:"uptime_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if info.Version == "" || info.Commit == "" || info.GoVersion == "" {
		t.Fatalf("info missing build metadata: %+v", info)
	}
	if info.StorageDriver != "sqlite" || info.StartedAt == "" {
		t.Fatalf("info missing runtime metadata: %+v", info)
	}
}
