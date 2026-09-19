package httpd

import (
	"net/http"
)

// handleClearLogs deletes every log line and reports the count.
func (s *Server) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.ClearLogs(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.log.Info("", "", "logs cleared deleted="+itoa(int(n)))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// handleClearHistory deletes all search requests.
func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.ClearHistory(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.log.Info("", "", "history cleared deleted="+itoa(int(n)))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// handleVacuumNow compacts the SQLite database.
func (s *Server) handleVacuumNow(w http.ResponseWriter, r *http.Request) {
	if err := s.jobs.Vacuum(r.Context()); err != nil {
		writeError(w, r, http.StatusInternalServerError, "vacuum_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "driver": s.store.Driver()})
}

// handleRunCleanup triggers the retention cleanup job immediately.
func (s *Server) handleRunCleanup(w http.ResponseWriter, r *http.Request) {
	report, err := s.jobs.Cleanup(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "cleanup_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
