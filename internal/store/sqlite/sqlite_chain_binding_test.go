package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func newTestChain(id, name string) *store.Chain {
	return &store.Chain{
		ID: id, Name: name, Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{{
			Key: "a", ProviderID: "p", IsStart: true, TimeoutMS: 1000, Params: map[string]string{},
		}},
	}
}

func TestAPIKeyChainBinding(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)

	user := &store.User{ID: "u1", Name: "admin", Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, c := range []*store.Chain{newTestChain("c1", "A"), newTestChain("c2", "B")} {
		if err := st.SaveChain(ctx, c); err != nil {
			t.Fatalf("SaveChain: %v", err)
		}
	}

	key := &store.APIKey{
		ID: "k1", UserID: "u1", Name: "bound", Prefix: "pre1", Hash: "h",
		Scopes: "search", ChainID: "c1", CreatedAt: store.Now(),
	}
	if err := st.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := st.GetAPIKey(ctx, "k1")
	if err != nil || got.ChainID != "c1" {
		t.Fatalf("GetAPIKey chain: %+v %v", got, err)
	}
	byPrefix, err := st.GetAPIKeyByPrefix(ctx, "pre1")
	if err != nil || byPrefix.ChainID != "c1" {
		t.Fatalf("GetAPIKeyByPrefix chain: %+v %v", byPrefix, err)
	}

	// Rebind to another chain.
	got.ChainID = "c2"
	if err := st.UpdateAPIKey(ctx, got); err != nil {
		t.Fatalf("UpdateAPIKey: %v", err)
	}
	if n, err := st.CountKeysByChain(ctx, "c2"); err != nil || n != 1 {
		t.Fatalf("CountKeysByChain c2: %d %v", n, err)
	}
	if n, err := st.CountKeysByChain(ctx, "c1"); err != nil || n != 0 {
		t.Fatalf("CountKeysByChain c1: %d %v", n, err)
	}

	// Deleting the chain clears the binding (ON DELETE SET NULL).
	if err := st.DeleteChain(ctx, "c2"); err != nil {
		t.Fatalf("DeleteChain: %v", err)
	}
	got, _ = st.GetAPIKey(ctx, "k1")
	if got.ChainID != "" {
		t.Fatalf("binding should be cleared after chain delete, got %q", got.ChainID)
	}

	// ClearKeyChainRefs unbinds remaining keys and reports the count.
	got.ChainID = "c1"
	if err := st.UpdateAPIKey(ctx, got); err != nil {
		t.Fatalf("UpdateAPIKey rebind: %v", err)
	}
	n, err := st.ClearKeyChainRefs(ctx, "c1")
	if err != nil || n != 1 {
		t.Fatalf("ClearKeyChainRefs: %d %v", n, err)
	}
	got, _ = st.GetAPIKey(ctx, "k1")
	if got.ChainID != "" {
		t.Fatalf("binding should be cleared, got %q", got.ChainID)
	}
}

func TestCountActiveKeysByChain(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)

	if err := st.CreateUser(ctx, &store.User{ID: "u1", Name: "admin", Role: "admin", CreatedAt: store.Now()}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SaveChain(ctx, newTestChain("c1", "A")); err != nil {
		t.Fatalf("SaveChain: %v", err)
	}
	key := &store.APIKey{
		ID: "k1", UserID: "u1", Name: "bound", Prefix: "p1", Hash: "h",
		Scopes: "search", ChainID: "c1", CreatedAt: store.Now(),
	}
	if err := st.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if n, err := st.CountKeysByChain(ctx, "c1"); err != nil || n != 1 {
		t.Fatalf("CountKeysByChain: %d %v", n, err)
	}
	if n, err := st.CountActiveKeysByChain(ctx, "c1"); err != nil || n != 1 {
		t.Fatalf("CountActiveKeysByChain (active): %d %v", n, err)
	}

	// Revoking keeps the binding (the row still references the chain) but the
	// token no longer counts as active for the deletion warning.
	key, _ = st.GetAPIKey(ctx, "k1")
	key.RevokedAt = store.Now()
	if err := st.UpdateAPIKey(ctx, key); err != nil {
		t.Fatalf("UpdateAPIKey: %v", err)
	}
	if n, err := st.CountActiveKeysByChain(ctx, "c1"); err != nil || n != 0 {
		t.Fatalf("CountActiveKeysByChain (revoked): %d %v", n, err)
	}
	if n, err := st.CountKeysByChain(ctx, "c1"); err != nil || n != 1 {
		t.Fatalf("CountKeysByChain after revoke: %d %v", n, err)
	}
}
