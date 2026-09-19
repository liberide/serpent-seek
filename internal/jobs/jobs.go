// Package jobs registers and runs background maintenance jobs.
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
)

// Manager owns the cron scheduler and the manual job triggers.
type Manager struct {
	store    store.Storage
	settings *config.Manager
	log      *logging.Logger
	cron     *cron.Cron
	mu       sync.Mutex
	handlers map[string]func(context.Context) error
	lastRun  map[string]time.Time
	lastErr  map[string]string
}

// New builds a job manager.
func New(st store.Storage, settings *config.Manager, log *logging.Logger) *Manager {
	logger := cronLogAdapter{log}
	c := cron.New(
		cron.WithChain(cron.Recover(logger), cron.SkipIfStillRunning(logger)),
		cron.WithLogger(logger),
	)
	return &Manager{
		store: st, settings: settings, log: log, cron: c,
		handlers: map[string]func(context.Context) error{},
		lastRun:  map[string]time.Time{},
		lastErr:  map[string]string{},
	}
}

// Register adds a named job at the given cron spec.
func (m *Manager) Register(name, spec string, fn func(context.Context) error) error {
	m.mu.Lock()
	m.handlers[name] = fn
	m.mu.Unlock()
	if spec == "" {
		return nil
	}
	_, err := m.cron.AddFunc(spec, func() {
		if err := m.RunNow(context.Background(), name); err != nil {
			m.log.Error("", "", "job "+name+" failed: "+err.Error())
		}
	})
	if err != nil {
		return fmt.Errorf("jobs: register %s (%s): %w", name, spec, err)
	}
	return nil
}

// Start launches the scheduler.
func (m *Manager) Start() { m.cron.Start() }

// Stop waits for running jobs to finish.
func (m *Manager) Stop(ctx context.Context) {
	done := m.cron.Stop()
	select {
	case <-done.Done():
	case <-ctx.Done():
	}
}

// RunNow executes a registered job immediately.
func (m *Manager) RunNow(ctx context.Context, name string) error {
	m.mu.Lock()
	fn, ok := m.handlers[name]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("jobs: unknown job %q", name)
	}
	start := time.Now()
	err := fn(ctx)
	m.mu.Lock()
	m.lastRun[name] = start
	if err != nil {
		m.lastErr[name] = err.Error()
	} else {
		m.lastErr[name] = ""
	}
	m.mu.Unlock()
	if err != nil {
		m.log.Error("", "", "job "+name+" error: "+err.Error())
		return err
	}
	m.log.Info("", "", "job "+name+" done in "+time.Since(start).Round(time.Millisecond).String())
	return nil
}

// Status reports the last run time and error for each job.
func (m *Manager) Status() map[string]map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]map[string]string{}
	for name := range m.handlers {
		entry := map[string]string{"last_error": m.lastErr[name]}
		if t, ok := m.lastRun[name]; ok {
			entry["last_run"] = t.UTC().Format(time.RFC3339)
		}
		out[name] = entry
	}
	return out
}

type cronLogAdapter struct{ log *logging.Logger }

func (c cronLogAdapter) Info(msg string, keysAndValues ...any) {
	c.log.Info("", "", msg+" "+fmt.Sprint(keysAndValues...))
}

func (c cronLogAdapter) Error(err error, msg string, keysAndValues ...any) {
	c.log.Error("", "", msg+" "+fmt.Sprint(keysAndValues...)+" err="+err.Error())
}
