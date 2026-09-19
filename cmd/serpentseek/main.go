// Command serpentseek runs the SerpentSeek search gateway.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/httpd"
	"github.com/liberide/serpent-seek/internal/jobs"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/sse"
	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/store/postgres"
	"github.com/liberide/serpent-seek/internal/store/sqlite"
	"github.com/liberide/serpent-seek/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logging.New(logging.Options{
		Writer: os.Stdout,
		Level:  cfg.LogLevel,
		JSON:   cfg.LogFormat == "json",
		Redact: logging.NewRedactor(),
	})
	log.AddSecret(cfg.DefaultAPISerpentAPIKey, cfg.DefaultSerpBaseAPIKey, cfg.DefaultSearxngAPIKey,
		cfg.DefaultYandexAPIKey, cfg.DefaultGoogleAPIKey, cfg.DefaultGoogleCX, cfg.EncryptionKey, cfg.DatabaseURL)

	ctx := context.Background()

	var st store.Storage
	switch cfg.StorageDriver {
	case "postgres":
		st, err = postgres.Open(ctx, cfg.DatabaseURL)
	default:
		st, err = sqlite.Open(ctx, cfg.SQLitePath)
	}
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// Persist log lines to the in-DB log viewer asynchronously.
	sink := logging.NewBufferSink(4096, func(rec logging.Record) {
		_ = st.AddLog(context.Background(), &store.LogEntry{
			TS: rec.Time.UTC().Format(time.RFC3339Nano), Level: rec.Level,
			RID: rec.RID, API: rec.API, Message: rec.Message,
		})
	})
	log.AddSink(sink)
	defer sink.Close()

	settings := config.NewManager(cfg, st)
	if err := settings.Refresh(ctx); err != nil {
		return err
	}

	if err := seed(ctx, st, cfg, log); err != nil {
		return err
	}

	client := providers.NewHTTPClient(cfg.UserAgent)
	client.SetRedactor(log.Redact)
	registry := providers.NewRegistry(client)
	hub := sse.NewHub()
	engineSink := engine.NewStoreSink(st, hub, log)
	eng := engine.New(st, registry, settings, engineSink, log)

	authenticator := &auth.Authenticator{Store: st, Settings: settings, Log: log}
	passkeys, err := auth.NewPasskeyService(
		settings.GetString(ctx, config.KeyPublicOrigin),
		settings.GetString(ctx, config.KeyRPID),
		settings.GetString(ctx, config.KeyRPName),
	)
	if err != nil {
		log.Warn("", "", "passkeys disabled: "+err.Error())
	}

	jobManager := jobs.New(st, settings, log)
	if err := jobManager.Register("cleanup", cfg.CleanupCron, func(jobCtx context.Context) error {
		_, err := jobManager.Cleanup(jobCtx)
		return err
	}); err != nil {
		return err
	}
	if err := jobManager.Register("vacuum", cfg.VacuumCron, jobManager.Vacuum); err != nil {
		return err
	}
	jobManager.Start()
	defer jobManager.Stop(context.Background())

	setupToken := cfg.SetupToken
	if admins, cerr := st.CountAdmins(ctx); cerr == nil && admins == 0 {
		if setupToken == "" {
			// Persist the generated token so restarts do not invalidate it.
			if stored, serr := st.GetSetting(ctx, config.KeySetupToken); serr == nil && strings.TrimSpace(stored) != "" {
				setupToken = stored
			} else {
				setupToken = randomToken()
				if err := st.SetSetting(ctx, config.KeySetupToken, setupToken); err != nil {
					log.Warn("", "", "failed to persist setup token: "+err.Error())
				}
			}
		}
		log.Info("", "", "SETUP_TOKEN: "+setupToken+" (valid until the first admin is created)")
	} else {
		// Setup already completed: drop a stale persisted token if any.
		_ = st.SetSetting(ctx, config.KeySetupToken, "")
	}

	logBootLine(cfg, registry, passkeys, st, log)

	srv := httpd.New(cfg, settings, st, eng, hub, log, authenticator, passkeys, jobManager, setupToken)

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-runCtx.Done()
		jobManager.Stop(context.Background())
	}()
	return srv.Run(runCtx)
}

func logBootLine(cfg *config.Config, registry *providers.Registry, passkeys *auth.PasskeyService, st store.Storage, log *logging.Logger) {
	providerList := strings.Join(registry.Codes(), ",")
	searxngURL := cfg.DefaultSearxngURL
	searxngState := "off"
	if searxngURL != "" {
		searxngState = "on"
	}
	keyState := "off"
	if cfg.DefaultSearxngAPIKey != "" {
		keyState = "set"
	}
	passkeyState := "off"
	if passkeys != nil && passkeys.Enabled() {
		passkeyState = "on"
	}
	pathOnly := searxngURL
	if idx := strings.IndexByte(pathOnly, '?'); idx >= 0 {
		pathOnly = pathOnly[:idx]
	}
	storageInfo := "storage=" + st.Driver() + " data_dir=" + cfg.DataDir
	if st.Driver() == "sqlite" {
		storageInfo += " db=" + cfg.SQLitePath
	} else if cfg.DatabaseURL != "" {
		storageInfo += " db=<DATABASE_URL>"
	}
	line := fmt.Sprintf("start version=%s %s providers=%s searxng=%s url=%s key=%s passkeys=%s ua=%q",
		version.String(), storageInfo, providerList, searxngState, pathOnly, keyState, passkeyState, cfg.UserAgent)
	log.Info("", "", line)
}

func randomToken() string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
