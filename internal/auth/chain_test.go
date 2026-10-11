package auth

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// chainKeyStore is a minimal Storage stub for resolveKey.
type chainKeyStore struct {
	store.Storage
	key  *store.APIKey
	user *store.User
}

func (s *chainKeyStore) GetAPIKeyByPrefix(context.Context, string) (*store.APIKey, error) {
	return s.key, nil
}

func (s *chainKeyStore) GetUser(context.Context, string) (*store.User, error) {
	return s.user, nil
}

func (s *chainKeyStore) UpdateAPIKey(context.Context, *store.APIKey) error { return nil }

func TestResolveKeyCarriesChain(t *testing.T) {
	plaintext, prefix, hash, err := GenerateKey(false)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	st := &chainKeyStore{
		key: &store.APIKey{
			ID: "k", UserID: "u", Prefix: prefix, Hash: hash,
			Scopes: "search", ChainID: "chain-1",
		},
		user: &store.User{ID: "u", Name: "u", Role: "viewer"},
	}
	a := &Authenticator{Store: st}
	id, err := a.resolveKey(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("resolveKey: %v", err)
	}
	if id.ChainID != "chain-1" {
		t.Fatalf("key chain binding not propagated to identity: %q", id.ChainID)
	}
}
