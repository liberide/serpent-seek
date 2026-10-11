package httpd

import (
	"net/http"
	"sort"
	"strings"

	"github.com/liberide/serpent-seek/internal/config"
)

// handleGetSettings returns effective settings with env pinning and masked
// secrets.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	values, env := s.settings.Snapshot(ctx)
	descriptors := s.cfg.Descriptors()
	visible := make([]config.SettingDef, 0, len(descriptors))
	out := map[string]string{}
	secretSet := map[string]bool{}
	for _, d := range descriptors {
		if d.Hidden {
			continue
		}
		visible = append(visible, d)
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
		"descriptors": visible,
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
	var rejected, applied []string
	for key, value := range body {
		d, ok := descriptors[key]
		if !ok || d.Hidden {
			continue
		}
		if d.Secret && value == "" {
			continue // blank secret keeps the stored value
		}
		// Environment-managed settings are read-only: an unchanged value echoed
		// back by a full-snapshot PUT is ignored, a changed one is rejected.
		if d.EnvOnly || s.cfg.IsEnv(key) {
			if value == s.settings.GetString(ctx, key) {
				continue
			}
			rejected = append(rejected, key)
			continue
		}
		if err := s.settings.Set(ctx, key, value); err != nil {
			rejected = append(rejected, key)
		} else {
			applied = append(applied, key)
		}
	}
	if len(rejected) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"code":    "readonly_setting",
			"message": "some settings are read-only (pinned by an environment variable or locked)",
			"details": rejected,
		}})
		return
	}
	if len(applied) > 0 {
		sort.Strings(applied)
		// Only key names are recorded; secret values are never logged.
		s.audit(ctx, r, "settings.update", "", map[string]string{"keys": strings.Join(applied, ",")})
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
