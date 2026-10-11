package httpd

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/updatecheck"
	"github.com/liberide/serpent-seek/internal/version"
)

// updateCheckTimeout bounds a single outbound release check.
const updateCheckTimeout = 6 * time.Second

// updateLoop checks for a newer release shortly after startup and then every
// configured interval. It is a no-op when the check is disabled.
func (s *Server) updateLoop(ctx context.Context) {
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
		return
	}
	for {
		s.refreshUpdate(ctx)
		interval := time.Duration(s.settings.GetInt(ctx, config.KeyUpdateCheckInterval)) * time.Hour
		if interval <= 0 {
			interval = 24 * time.Hour
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
	}
}

// refreshUpdate runs one check and caches the result. Failures are logged only.
func (s *Server) refreshUpdate(ctx context.Context) {
	if s.updater == nil || !s.settings.GetBool(ctx, config.KeyUpdateCheck) {
		return
	}
	url := strings.TrimSpace(s.settings.GetString(ctx, config.KeyUpdateCheckURL))
	if url == "" {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	if _, err := s.updater.Check(cctx, url, version.Version); err != nil {
		s.log.Warn("", "", "update check failed: "+err.Error())
	}
}

// unofficialFeed reports whether the configured release feed differs from the
// official project feed.
func unofficialFeed(url string) bool {
	u := strings.TrimRight(strings.TrimSpace(url), "/")
	return u != "" && !strings.EqualFold(u, config.DefaultUpdateCheckURL)
}

// handleUpdate returns the cached update status (read scope). It never performs
// a network request itself, so it is safe to poll from the UI.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	unofficial := unofficialFeed(s.settings.GetString(ctx, config.KeyUpdateCheckURL))
	if !s.settings.GetBool(ctx, config.KeyUpdateCheck) {
		writeJSON(w, http.StatusOK, updatecheck.Result{Enabled: false, Current: version.Version, Unofficial: unofficial})
		return
	}
	if res, ok := s.updater.Status(); ok {
		res.Enabled = true
		res.Unofficial = unofficial
		writeJSON(w, http.StatusOK, res)
		return
	}
	writeJSON(w, http.StatusOK, updatecheck.Result{Enabled: true, Current: version.Version, Unofficial: unofficial})
}

// handleUpdateCheck forces an immediate release check (admin).
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.settings.GetBool(ctx, config.KeyUpdateCheck) {
		writeError(w, r, http.StatusBadRequest, "update_check_disabled", "update checks are disabled")
		return
	}
	url := strings.TrimSpace(s.settings.GetString(ctx, config.KeyUpdateCheckURL))
	if url == "" {
		writeError(w, r, http.StatusBadRequest, "update_check_no_url", "no release URL configured")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	res, err := s.updater.Check(cctx, url, version.Version)
	res.Enabled = true
	res.Unofficial = unofficialFeed(url)
	if err != nil {
		res.Error = err.Error()
	}
	s.audit(ctx, r, "update.check", "", nil)
	writeJSON(w, http.StatusOK, res)
}
