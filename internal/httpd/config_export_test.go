package httpd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestConfigExportImportRoundTrip(t *testing.T) {
	src := newTestServer(t, true)
	exported := src.do(t, http.MethodGet, "/api/config/export", nil, true)
	if exported.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", exported.Code, exported.Body.String())
	}
	if !bytes.Contains(exported.Body.Bytes(), []byte("\n  \"version\"")) {
		t.Fatalf("export must be pretty-printed: %s", exported.Body.String())
	}
	var bundle map[string]any
	if err := json.Unmarshal(exported.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if _, ok := bundle["providers"]; !ok {
		t.Fatalf("bundle missing providers: %s", exported.Body.String())
	}
	if _, ok := bundle["chains"]; !ok {
		t.Fatalf("bundle missing chains: %s", exported.Body.String())
	}

	// Import into a fresh instance that already has the seeded stub provider and
	// "test chain"; with overwrite the existing ones are updated.
	dst := newTestServer(t, true)
	imported := dst.do(t, http.MethodPost, "/api/config/import?overwrite=true",
		json.RawMessage(exported.Body.Bytes()), true)
	if imported.Code != http.StatusOK {
		t.Fatalf("import status %d: %s", imported.Code, imported.Body.String())
	}
	var result struct {
		ProvidersUpdated int `json:"providers_updated"`
		ChainsCreated    int `json:"chains_created"`
		ChainsUpdated    int `json:"chains_updated"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if result.ProvidersUpdated < 1 {
		t.Fatalf("expected the seeded provider to be updated, got %+v", result)
	}
	if result.ChainsCreated+result.ChainsUpdated < 1 {
		t.Fatalf("expected a chain to be imported, got %+v", result)
	}

	// Without overwrite, existing names are skipped instead.
	skipped := dst.do(t, http.MethodPost, "/api/config/import",
		json.RawMessage(exported.Body.Bytes()), true)
	var skippedResult struct {
		ChainsSkipped int `json:"chains_skipped"`
	}
	_ = json.Unmarshal(skipped.Body.Bytes(), &skippedResult)
	if skippedResult.ChainsSkipped < 1 {
		t.Fatalf("expected the chain to be skipped, got %s", skipped.Body.String())
	}
}
