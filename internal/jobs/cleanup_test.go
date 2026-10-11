package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
	sqlitestore "github.com/liberide/serpent-seek/internal/store/sqlite"
)

func newJobsManager(t *testing.T) (*Manager, store.Storage) {
	t.Helper()
	ctx := context.Background()
	driver, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	var st store.Storage = driver

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	settings := config.NewManager(cfg, st)
	if err := settings.Refresh(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	log := logging.New(logging.Options{Level: "error"})
	return New(st, settings, log), st
}

func TestCleanupPrunesOldRecords(t *testing.T) {
	ctx := context.Background()
	m, st := newJobsManager(t)

	old := time.Now().UTC().AddDate(0, 0, -400)
	oldTS := old.Format(time.RFC3339Nano)
	oldDay := old.Format("2006-01-02")

	if err := st.CreateRequest(ctx, &store.Request{
		ID: "r-old", RID: "0000-old", Query: "old", Status: "ok", CreatedAt: oldTS,
	}); err != nil {
		t.Fatalf("create request: %v", err)
	}
	if err := st.AddLog(ctx, &store.LogEntry{TS: oldTS, Level: "info", Message: "old log"}); err != nil {
		t.Fatalf("add log: %v", err)
	}
	if _, err := st.IncrementUsage(ctx, "key:old", oldDay); err != nil {
		t.Fatalf("increment usage: %v", err)
	}
	if err := st.AddAudit(ctx, &store.AuditEntry{ID: "a-old", TS: oldTS, Action: "old.action"}); err != nil {
		t.Fatalf("add audit: %v", err)
	}

	report, err := m.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.Requests != 1 || report.Logs != 1 || report.Usage != 1 || report.Audit != 1 {
		t.Fatalf("unexpected cleanup report: %+v", report)
	}
	if report.MS < 0 {
		t.Fatalf("negative duration: %d", report.MS)
	}
}

func TestVacuumRuns(t *testing.T) {
	ctx := context.Background()
	m, _ := newJobsManager(t)
	if err := m.Vacuum(ctx); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
}
