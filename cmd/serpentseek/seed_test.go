package main

import (
	"context"
	"io"
	"testing"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/store/sqlite"
)

// TestSeedSmoke verifies once-per-install seeding and idempotency.
func TestSeedSmoke(t *testing.T) {
	ctx := context.Background()
	driver, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var st store.Storage = driver

	cfg := &config.Config{DefaultGoogleAPIKey: "gkey", DefaultGoogleModel: "m1", DefaultGoogleDriver: "vertex"}
	log := logging.New(logging.Options{Writer: io.Discard, Level: "error"})

	if err := seed(ctx, st, cfg, log); err != nil {
		t.Fatalf("seed: %v", err)
	}

	providers, err := st.ListProviders(ctx)
	if err != nil || len(providers) != 1 {
		t.Fatalf("providers = %d (err %v), want exactly 1 seeded instance", len(providers), err)
	}
	p := providers[0]
	if p.Code != "google_vertex" || !p.Enabled {
		t.Fatalf("unexpected seeded provider: %+v", p)
	}
	if p.Credentials["api_key"] != "gkey" || p.Params["model"] != "m1" {
		t.Fatalf("config was not propagated into the seeded instance: %+v", p)
	}

	chain, err := st.GetActiveChain(ctx)
	if err != nil || chain == nil {
		t.Fatalf("active chain: %v", err)
	}
	if len(chain.Nodes) != 1 || chain.Nodes[0].ProviderID != p.ID || !chain.Nodes[0].IsStart {
		t.Fatalf("default chain must contain the seeded provider as start node: %+v", chain.Nodes)
	}

	// A second seed must be a no-op.
	if err := seed(ctx, st, cfg, log); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if again, _ := st.ListProviders(ctx); len(again) != 1 {
		t.Fatalf("second seed duplicated providers: %d", len(again))
	}

	// A later seed leaves a deleted provider deleted.
	if err := st.DeleteProvider(ctx, p.ID); err != nil {
		t.Fatalf("delete provider: %v", err)
	}
	if err := seed(ctx, st, cfg, log); err != nil {
		t.Fatalf("seed after delete: %v", err)
	}
	if gone, _ := st.ListProviders(ctx); len(gone) != 0 {
		t.Fatalf("deleted provider was resurrected: %d rows", len(gone))
	}
}
