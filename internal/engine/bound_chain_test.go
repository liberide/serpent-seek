package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// chainSelectStore returns a fixed active chain and a map of explicit chains.
type chainSelectStore struct {
	store.Storage
	active *store.Chain
	byID   map[string]*store.Chain
}

func (s *chainSelectStore) GetActiveChain(context.Context) (*store.Chain, error) {
	if s.active == nil {
		return nil, store.ErrNotFound
	}
	return s.active, nil
}

func (s *chainSelectStore) GetChain(_ context.Context, id string) (*store.Chain, error) {
	if c, ok := s.byID[id]; ok {
		return c, nil
	}
	return nil, store.ErrNotFound
}

func TestResolveChainPrefersBound(t *testing.T) {
	st := &chainSelectStore{
		active: &store.Chain{ID: "active"},
		byID:   map[string]*store.Chain{"bound": {ID: "bound"}},
	}
	e := &Engine{store: st}

	got, err := e.resolveChain(context.Background(), "")
	if err != nil || got.ID != "active" {
		t.Fatalf("empty id should pick the active chain: %+v %v", got, err)
	}

	got, err = e.resolveChain(context.Background(), "bound")
	if err != nil || got.ID != "bound" {
		t.Fatalf("explicit id should pick the bound chain: %+v %v", got, err)
	}

	if _, err := e.resolveChain(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing bound chain should fail closed, got %v", err)
	}

	noActive := &Engine{store: &chainSelectStore{byID: map[string]*store.Chain{}}}
	if _, err := noActive.resolveChain(context.Background(), ""); err == nil {
		t.Fatal("missing active chain should fail")
	}
}
