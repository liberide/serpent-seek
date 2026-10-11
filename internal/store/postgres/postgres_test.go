package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/store"
)

// TestPostgresCRUD is an integration test against a real PostgreSQL instance.
// It is skipped unless TEST_DATABASE_URL is set (see the "postgres" CI job).
func TestPostgresCRUD(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Users + API keys (random ids/names keep repeated runs independent).
	user := &store.User{ID: uuid.NewString(), Name: "pg-" + uuid.NewString(), Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if got, err := st.GetUser(ctx, user.ID); err != nil || got.Name != user.Name {
		t.Fatalf("GetUser: %+v %v", got, err)
	}
	prefix := "pg" + uuid.NewString()[:8]
	key := &store.APIKey{ID: uuid.NewString(), UserID: user.ID, Name: "k", Prefix: prefix, Hash: "x", Scopes: "search", CreatedAt: store.Now()}
	if err := st.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if got, err := st.GetAPIKeyByPrefix(ctx, prefix); err != nil || got.ID != key.ID {
		t.Fatalf("GetAPIKeyByPrefix: %+v %v", got, err)
	}

	// Usage counters.
	subject := "key:" + key.ID
	day := time.Now().UTC().Format("2006-01-02")
	if n, err := st.IncrementUsage(ctx, subject, day); err != nil || n != 1 {
		t.Fatalf("IncrementUsage: n=%d err=%v", n, err)
	}
	if n, err := st.IncrementUsage(ctx, subject, day); err != nil || n != 2 {
		t.Fatalf("IncrementUsage 2: n=%d err=%v", n, err)
	}
	usage, err := st.UsageByDay(ctx, day)
	if err != nil || usage[subject] != 2 {
		t.Fatalf("UsageByDay: %+v %v", usage, err)
	}

	// Audit log.
	if err := st.AddAudit(ctx, &store.AuditEntry{ID: uuid.NewString(), TS: store.Now(), Action: "pg.test", Target: subject}); err != nil {
		t.Fatalf("AddAudit: %v", err)
	}
	items, total, err := st.ListAudit(ctx, store.AuditFilter{Action: "pg.test"}, store.Page{Limit: 10})
	if err != nil || total < 1 || len(items) == 0 {
		t.Fatalf("ListAudit: total=%d items=%d err=%v", total, len(items), err)
	}

	// Cleanup paths used by the maintenance job.
	if _, err := st.DeleteOldUsage(ctx, "2000-01-01"); err != nil {
		t.Fatalf("DeleteOldUsage: %v", err)
	}
	if _, err := st.DeleteOldAudit(ctx, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("DeleteOldAudit: %v", err)
	}
}
