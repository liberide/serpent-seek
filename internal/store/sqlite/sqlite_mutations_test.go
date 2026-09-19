package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func TestMiscMutations(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	user := &store.User{ID: "u1", Name: "u", Role: "viewer", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sess := &store.Session{ID: "s1", UserID: "u1", TokenHash: "h", CreatedAt: store.Now(), ExpiresAt: store.Now()}
	if err := st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := st.DeleteUserSessions(ctx, "u1"); err != nil {
		t.Fatalf("DeleteUserSessions: %v", err)
	}
	if _, err := st.GetSessionByTokenHash(ctx, "h"); err != store.ErrNotFound {
		t.Fatalf("session should be deleted, got %v", err)
	}

	key := &store.APIKey{ID: "k1", UserID: "u1", Name: "k", Prefix: "p1", Hash: "h", Scopes: "search", CreatedAt: store.Now()}
	if err := st.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := st.DeleteAPIKey(ctx, "k1"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	if _, err := st.GetAPIKeyByPrefix(ctx, "p1"); err != store.ErrNotFound {
		t.Fatalf("key should be deleted, got %v", err)
	}

	_ = st.AddLog(ctx, &store.LogEntry{TS: store.Now(), Level: "warn", RID: "r1", Message: "find me"})
	if logs, total, err := st.ListLogs(ctx, store.LogFilter{Query: "find"}, store.Page{Limit: 10}); err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("ListLogs query filter: %d %v", total, err)
	}
	if logs, _, _ := st.ListLogs(ctx, store.LogFilter{Query: "missing"}, store.Page{Limit: 10}); len(logs) != 0 {
		t.Fatalf("expected no logs, got %d", len(logs))
	}
}

func TestClearHistoryCascadesSteps(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	req := &store.Request{ID: "r1", RID: "rid1", Query: "q", Status: "ok", CreatedAt: store.Now()}
	if err := st.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if err := st.AddStep(ctx, &store.RequestStep{ID: "st1", RequestID: "r1", Idx: 0, Provider: "p", Status: "ok"}); err != nil {
		t.Fatalf("AddStep: %v", err)
	}
	if _, err := st.ClearHistory(ctx); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	if steps, err := st.ListSteps(ctx, "r1"); err != nil || len(steps) != 0 {
		t.Fatalf("expected cascaded step deletion, got %d %v", len(steps), err)
	}
}
