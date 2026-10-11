package httpd

import (
	"net/http"
	"runtime"
	"time"

	"github.com/liberide/serpent-seek/internal/version"
)

// handleInfo returns build and runtime metadata for the Settings → Info panel.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"version":        version.Version,
		"commit":         version.Revision(),
		"build_date":     version.BuiltAt(),
		"go_version":     runtime.Version(),
		"storage_driver": s.store.Driver(),
	}
	if !s.startedAt.IsZero() {
		info["started_at"] = s.startedAt.UTC().Format(time.RFC3339Nano)
		info["uptime_seconds"] = int(time.Since(s.startedAt).Seconds())
	}
	writeJSON(w, http.StatusOK, info)
}
