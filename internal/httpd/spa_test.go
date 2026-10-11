package httpd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHasFileExtension(t *testing.T) {
	cases := map[string]bool{
		"/_app/immutable/nodes/2.abc.js": true,
		"/favicon.svg":                   true,
		"/settings":                      false,
		"/history/6f2f":                  false,
		"/":                              false,
		"/chains/abc":                    false,
	}
	for path, want := range cases {
		if got := hasFileExtension(path); got != want {
			t.Errorf("hasFileExtension(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestSPAMissingAssetIs404 guards the fix for "Importing a module script
// failed": a missing build asset must not be served as the HTML shell.
func TestSPAMissingAssetIs404(t *testing.T) {
	srv := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/nodes/does-not-exist.js", nil)
	srv.handleSPA(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "text/html") {
		t.Fatalf("missing asset must not be served as HTML (content-type %q)", ct)
	}
}
