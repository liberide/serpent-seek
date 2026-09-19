package httpd

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/jobs"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/sse"
	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/version"
)

// Server holds all HTTP dependencies.
type Server struct {
	cfg         *config.Config
	settings    *config.Manager
	store       store.Storage
	engine      *engine.Engine
	hub         *sse.Hub
	log         *logging.Logger
	authn       *auth.Authenticator
	mw          *auth.Middleware
	passkeys    *auth.PasskeyService
	jobs        *jobs.Manager
	limiter     *auth.RateLimiter
	mcpSessions *mcpSessionStore

	setupMu    sync.Mutex
	setupToken string
	startedAt  time.Time
}

// New builds a Server.
func New(
	cfg *config.Config,
	settings *config.Manager,
	st store.Storage,
	eng *engine.Engine,
	hub *sse.Hub,
	log *logging.Logger,
	authn *auth.Authenticator,
	passkeys *auth.PasskeyService,
	jobManager *jobs.Manager,
	setupToken string,
) *Server {
	if passkeys != nil && passkeys.Enabled() {
		passkeys.SetLookup(func(rawID []byte) (*store.User, []*store.Passkey, error) {
			ctx := context.Background()
			pk, err := st.GetPasskeyByCredentialID(ctx, rawID)
			if err != nil {
				return nil, nil, err
			}
			user, err := st.GetUser(ctx, pk.UserID)
			if err != nil {
				return nil, nil, err
			}
			pks, err := st.ListPasskeys(ctx, pk.UserID)
			if err != nil {
				return nil, nil, err
			}
			return user, pks, nil
		})
	}
	return &Server{
		cfg:         cfg,
		settings:    settings,
		store:       st,
		engine:      eng,
		hub:         hub,
		log:         log,
		authn:       authn,
		mw:          &auth.Middleware{Auth: authn},
		passkeys:    passkeys,
		jobs:        jobManager,
		limiter:     auth.NewRateLimiter(10, time.Minute),
		mcpSessions: newMCPSessionStore(),
		setupToken:  setupToken,
		startedAt:   time.Now(),
	}
}

// Routes builds the chi router.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.requestContext)
	r.Use(middleware.Recoverer)
	r.Use(s.securityHeaders)

	// Machine endpoints.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/version", s.handleVersion)
	r.With(s.mw.Authenticate, s.mw.RequireScope("search")).Post("/search", s.handleSearch)
	// MCP (Model Context Protocol) HTTP JSON-RPC 2.0 endpoint exposing the search tool.
	r.With(s.mw.Authenticate, s.mw.RequireScope("search")).Handle("/mcp", http.HandlerFunc(s.handleMCP))

	// API.
	r.Route("/api", func(r chi.Router) {
		r.Get("/setup/status", s.handleSetupStatus)
		r.With(s.rateLimit).Post("/auth/setup", s.handleSetup)
		r.With(s.rateLimit).Post("/auth/login", s.handleLogin)
		r.With(s.rateLimit).Post("/auth/passkey/assert/begin", s.handlePasskeyAssertBegin)
		r.With(s.rateLimit).Post("/auth/passkey/assert/finish", s.handlePasskeyAssertFinish)

		r.Group(func(r chi.Router) {
			r.Use(s.mw.Authenticate)
			r.Get("/me", s.handleMe)
			r.Post("/auth/logout", s.handleLogout)
			r.Post("/auth/passkey/register/begin", s.handlePasskeyRegisterBegin)
			r.Post("/auth/passkey/register/finish", s.handlePasskeyRegisterFinish)
			r.Get("/requests", s.handleListRequests)
			r.Get("/requests/{id}", s.handleGetRequest)
			r.Get("/requests/{id}/events", s.handleRequestEvents)
			r.Get("/events", s.handleGlobalEvents)
			r.Get("/stats/summary", s.handleStatsSummary)
			r.Get("/logs", s.handleListLogs)
			r.Get("/settings", s.handleGetSettings)
			r.With(s.mw.RequireScope("search")).Post("/search/ui", s.handleSearchUI)

			r.Group(func(r chi.Router) {
				r.Use(s.mw.RequireAdmin)
				r.Put("/settings", s.handlePutSettings)
				r.Post("/settings/storage/test", s.handleStorageTest)
				r.Get("/maintenance/status", s.handleMaintenanceStatus)
				r.Post("/maintenance/clear-logs", s.handleClearLogs)
				r.Post("/maintenance/clear-history", s.handleClearHistory)
				r.Post("/maintenance/vacuum-now", s.handleVacuumNow)
				r.Post("/maintenance/run-cleanup", s.handleRunCleanup)

				r.Route("/chains", func(r chi.Router) {
					r.Get("/", s.handleListChains)
					r.Post("/", s.handleCreateChain)
					r.Post("/validate", s.handleValidateChain)
					r.Get("/{id}", s.handleGetChain)
					r.Put("/{id}", s.handleUpdateChain)
					r.Delete("/{id}", s.handleDeleteChain)
					r.Post("/{id}/activate", s.handleActivateChain)
					r.Post("/{id}/duplicate", s.handleDuplicateChain)
				})
				r.Route("/providers", func(r chi.Router) {
					r.Get("/", s.handleListProviders)
					r.Get("/drivers", s.handleListDrivers)
					r.Post("/", s.handleCreateProvider)
					r.Get("/{id}", s.handleGetProvider)
					r.Put("/{id}", s.handleUpdateProvider)
					r.Patch("/{id}", s.handlePatchProvider)
					r.Delete("/{id}", s.handleDeleteProvider)
					r.Post("/{id}/test", s.handleTestProvider)
				})
				r.Route("/users", func(r chi.Router) {
					r.Get("/", s.handleListUsers)
					r.Post("/", s.handleCreateUser)
					r.Patch("/{id}", s.handleUpdateUser)
					r.Delete("/{id}", s.handleDeleteUser)
					r.Get("/{id}/passkeys", s.handleListPasskeys)
				})
				r.Route("/keys", func(r chi.Router) {
					r.Get("/", s.handleListKeys)
					r.Post("/", s.handleCreateKey)
					r.Delete("/{id}", s.handleRevokeKey)
				})
				r.Delete("/passkeys/{id}", s.handleDeletePasskey)
			})
		})
	})

	// SPA fallback.
	r.NotFound(s.handleSPA)
	return r
}

// requestContext sets compatibility headers and logs the completed request.
// A rid only exists for actual search requests (assigned by the engine), so
// plain HTTP access logs carry no rid.
func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Serpent-Api", "-")
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		if !strings.HasPrefix(r.URL.Path, "/api/events") && !strings.HasSuffix(r.URL.Path, "/events") {
			s.log.Info("", "", r.Method+" "+shortPath(r.URL.Path)+" "+itoa(ww.Status())+" "+time.Since(start).Round(time.Millisecond).String())
		}
	})
}

// securityHeaders applies conservative defaults for the SPA and API.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// rateLimit limits /auth/* attempts per IP.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.Allow(auth.ClientIP(r)) {
			writeError(w, r, http.StatusTooManyRequests, "rate_limited", "too many authentication attempts")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(version.Version + "\n"))
}

// handleSPA serves the embedded SvelteKit bundle with an index.html fallback.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, r, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	fsys := FS()
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := fsys.Open(path); err == nil {
		_ = f.Close()
		http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
		return
	}
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(index)
}

func (s *Server) handleMaintenanceStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.jobs.Status()})
}

// Run starts the HTTP server and shuts it down gracefully when ctx is done.
func (s *Server) Run(ctx context.Context) error {
	addr := ":" + itoa(s.cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      0, // SSE streams must not be cut off
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("", "", "listening on "+addr+" version="+version.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.hub.Close()
	if s.mcpSessions != nil {
		s.mcpSessions.closeAll()
	}
	return srv.Shutdown(shutdownCtx)
}

func shortPath(p string) string {
	if len(p) > 120 {
		return p[:120]
	}
	return p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
