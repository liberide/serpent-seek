package httpd

import (
	"net/http"

	"github.com/liberide/serpent-seek/internal/config"
)

// handleGetSettings returns effective settings with env pinning and masked
// secrets.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	values, env := s.settings.Snapshot(ctx)
	descriptors := s.cfg.Descriptors()
	out := map[string]string{}
	secretSet := map[string]bool{}
	for _, d := range descriptors {
		v := values[d.Key]
		if d.Secret {
			if v != "" {
				secretSet[d.Key] = true
			}
			out[d.Key] = ""
			continue
		}
		out[d.Key] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"values":      out,
		"env":         env,
		"secret_set":  secretSet,
		"descriptors": descriptors,
		"storage": map[string]any{
			"driver":      s.store.Driver(),
			"sqlite_path": s.cfg.SQLitePath,
		},
	})
}

// handlePutSettings persists UI edits, rejecting environment-pinned keys.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body map[string]string
	if !readJSON(w, r, &body) {
		return
	}
	descriptors := map[string]config.SettingDef{}
	for _, d := range s.cfg.Descriptors() {
		descriptors[d.Key] = d
	}
	var rejected []string
	for key, value := range body {
		d, ok := descriptors[key]
		if !ok {
			continue
		}
		if d.Secret && value == "" {
			continue // blank secret keeps the stored value
		}
		if err := s.settings.Set(ctx, key, value); err != nil {
			rejected = append(rejected, key)
		}
	}
	if len(rejected) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"code":    "readonly_setting",
			"message": "some settings are pinned by environment variables",
			"details": rejected,
		}})
		return
	}
	values, env := s.settings.Snapshot(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"values": values, "env": env})
}

// handleStorageTest reports storage connectivity and the active driver.
func (s *Server) handleStorageTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	errText := ""
	ok := true
	if err := s.store.Ping(ctx); err != nil {
		ok = false
		errText = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"driver":                      s.store.Driver(),
		"ok":                          ok,
		"error":                       errText,
		"restart_required_for_switch": true,
	})
}
