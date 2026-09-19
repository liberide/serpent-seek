package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/store"
)

func openTest(t *testing.T) *store.Storage {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	driver, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	var st store.Storage = driver
	return &st
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	if err := st.SetSetting(ctx, "k", "v"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got, _ := st.GetSetting(ctx, "k"); got != "v" {
		t.Fatalf("GetSetting: %q", got)
	}
	if err := st.SetSetting(ctx, "k", "v2"); err != nil {
		t.Fatalf("SetSetting update: %v", err)
	}
	all, err := st.AllSettings(ctx)
	if err != nil || all["k"] != "v2" {
		t.Fatalf("AllSettings: %v %v", all, err)
	}
	if _, err := st.GetSetting(ctx, "missing"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUsersAndKeys(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	user := &store.User{ID: "u1", Name: "admin", Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.GetUser(ctx, "u1"); err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if _, err := st.GetUserByName(ctx, "admin"); err != nil {
		t.Fatalf("GetUserByName: %v", err)
	}
	n, err := st.CountAdmins(ctx)
	if err != nil || n != 1 {
		t.Fatalf("CountAdmins: %d %v", n, err)
	}
	user.Disabled = true
	if err := st.UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if n, _ := st.CountAdmins(ctx); n != 0 {
		t.Fatalf("expected 0 enabled admins, got %d", n)
	}

	key := &store.APIKey{ID: "k1", UserID: "u1", Name: "key", Prefix: "abc123", Hash: "hash", Scopes: "search", CreatedAt: store.Now()}
	if err := st.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	got, err := st.GetAPIKeyByPrefix(ctx, "abc123")
	if err != nil || got.ID != "k1" {
		t.Fatalf("GetAPIKeyByPrefix: %+v %v", got, err)
	}
	got.RevokedAt = store.Now()
	if err := st.UpdateAPIKey(ctx, got); err != nil {
		t.Fatalf("UpdateAPIKey: %v", err)
	}
	keys, err := st.ListAPIKeys(ctx, "u1")
	if err != nil || len(keys) != 1 || keys[0].RevokedAt == "" {
		t.Fatalf("ListAPIKeys: %+v %v", keys, err)
	}
	if err := st.DeleteUser(ctx, "u1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if keys, _ := st.ListAPIKeys(ctx, "u1"); len(keys) != 0 {
		t.Fatalf("expected cascade delete of keys, got %d", len(keys))
	}
}

func TestChains(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	chain := &store.Chain{
		ID: "c1", Name: "main", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{
			{Key: "a", ProviderID: "apiserpent", TimeoutMS: 1000, OnSuccess: "stop", OnEmpty: "next", OnFail: "next", IsStart: true, Params: map[string]string{"engine": "google"}},
			{Key: "b", ProviderID: "searxng", Mode: store.NodeModeAnswer, TimeoutMS: 1000, OnSuccess: "stop", OnEmpty: "next", OnFail: "next"},
		},
		Edges: []store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	}
	if err := st.SaveChain(ctx, chain); err != nil {
		t.Fatalf("SaveChain: %v", err)
	}
	if err := st.ActivateChain(ctx, "c1"); err != nil {
		t.Fatalf("ActivateChain: %v", err)
	}
	active, err := st.GetActiveChain(ctx)
	if err != nil {
		t.Fatalf("GetActiveChain: %v", err)
	}
	if !active.Active || len(active.Nodes) != 2 || len(active.Edges) != 1 {
		t.Fatalf("unexpected active chain: %+v", active)
	}
	if active.Nodes[0].Params["engine"] != "google" {
		t.Fatalf("node params not persisted: %+v", active.Nodes[0].Params)
	}
	if active.Mode != store.ChainModeFullChain {
		t.Fatalf("chain mode not persisted: %+v", active.Mode)
	}
	if !active.Nodes[0].IsStart || active.Nodes[1].IsStart {
		t.Fatalf("is_start not persisted: %+v", active.Nodes)
	}
	if active.Nodes[0].Mode != store.NodeModeSearch || active.Nodes[1].Mode != store.NodeModeAnswer {
		t.Fatalf("node mode not persisted: %+v", active.Nodes)
	}
	chain.Name = "renamed"
	if err := st.SaveChain(ctx, chain); err != nil {
		t.Fatalf("SaveChain update: %v", err)
	}
	list, err := st.ListChains(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "renamed" {
		t.Fatalf("ListChains: %+v %v", list, err)
	}
	if err := st.DeleteChain(ctx, "c1"); err != nil {
		t.Fatalf("DeleteChain: %v", err)
	}
	if list, _ := st.ListChains(ctx); len(list) != 0 {
		t.Fatalf("expected empty chains after delete")
	}
}

func TestRequestsAndSteps(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	req := &store.Request{
		ID: "r1", RID: "0001-abcd", Query: "hello", Count: 5, Status: "running",
		ChainID: "c1", Client: "api", CreatedAt: store.Now(),
		ChainSnapshot: &store.Chain{ID: "c1", Name: "snap"},
	}
	if err := st.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	step := &store.RequestStep{ID: "s1", RequestID: "r1", Idx: 0, Provider: "searxng", Status: "ok", Kind: "ok", ResultsCount: 2, TookMS: 12, StartedAt: store.Now(), FinishedAt: store.Now()}
	if err := st.AddStep(ctx, step); err != nil {
		t.Fatalf("AddStep: %v", err)
	}
	req.Status = "ok"
	req.UsedProvider = "searxng"
	req.ResultsCount = 2
	req.StepsCount = 1
	req.Merge = &store.MergeStats{CollectedTotal: 4, UniqueLinks: 2, DuplicatesRemoved: 2}
	req.Results = []store.RequestResult{{Link: "https://a.example", Title: "A", Sources: []string{"searxng", "yandex"}}}
	req.Answer = "generated answer text"
	if err := st.UpdateRequest(ctx, req); err != nil {
		t.Fatalf("UpdateRequest: %v", err)
	}
	got, err := st.GetRequest(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.UsedProvider != "searxng" || len(got.Steps) != 1 || got.ChainSnapshot == nil || got.ChainSnapshot.Name != "snap" {
		t.Fatalf("unexpected request: %+v", got)
	}
	if got.Merge == nil || got.Merge.CollectedTotal != 4 || got.Merge.UniqueLinks != 2 || got.Merge.DuplicatesRemoved != 2 {
		t.Fatalf("merge stats not persisted: %+v", got.Merge)
	}
	if got.Answer != "generated answer text" {
		t.Fatalf("answer not persisted: %q", got.Answer)
	}
	if len(got.Results) != 1 || len(got.Results[0].Sources) != 2 {
		t.Fatalf("results not persisted: %+v", got.Results)
	}
	if byRID, err := st.GetRequestByRID(ctx, "0001-abcd"); err != nil || byRID.ID != "r1" {
		t.Fatalf("GetRequestByRID: %+v %v", byRID, err)
	}
	items, total, err := st.ListRequests(ctx, store.RequestFilter{Status: "ok"}, store.Page{Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("ListRequests: %d/%d %v", total, len(items), err)
	}
	summary, err := st.Summary(ctx, 7)
	if err != nil || summary.RequestsToday != 1 || len(summary.Recent) != 1 {
		t.Fatalf("Summary: %+v %v", summary, err)
	}
	if err := st.RecomputeDailyStats(ctx, time.Now().UTC().Format("2006-01-02")); err != nil {
		t.Fatalf("RecomputeDailyStats: %v", err)
	}
	if _, err := st.ClearHistory(ctx); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{}, store.Page{}); total != 0 {
		t.Fatalf("expected empty history, got %d", total)
	}
}

func TestLogsAndSessions(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	user := &store.User{ID: "u1", Name: "session-user", Role: "viewer", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.AddLog(ctx, &store.LogEntry{TS: store.Now(), Level: "info", RID: "r", API: "a", Message: "hello"}); err != nil {
		t.Fatalf("AddLog: %v", err)
	}
	logs, total, err := st.ListLogs(ctx, store.LogFilter{Level: "info"}, store.Page{Limit: 10})
	if err != nil || total != 1 || logs[0].Message != "hello" {
		t.Fatalf("ListLogs: %+v %d %v", logs, total, err)
	}
	if n, err := st.DeleteOldLogs(ctx, time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)); err != nil || n != 1 {
		t.Fatalf("DeleteOldLogs: %d %v", n, err)
	}

	sess := &store.Session{ID: "sess1", UserID: "u1", TokenHash: "hash1", CreatedAt: store.Now(), ExpiresAt: store.Now(), IP: "127.0.0.1", UA: "test"}
	if err := st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := st.GetSessionByTokenHash(ctx, "hash1"); err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if err := st.UpdateSessionExpiry(ctx, "sess1", time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("UpdateSessionExpiry: %v", err)
	}
	if n, err := st.DeleteExpiredSessions(ctx, time.Now().UTC().Format(time.RFC3339Nano)); err != nil || n != 0 {
		t.Fatalf("DeleteExpiredSessions: %d %v", n, err)
	}
	if n, err := st.DeleteExpiredSessions(ctx, time.Now().Add(2*time.Hour).UTC().Format(time.RFC3339Nano)); err != nil || n != 1 {
		t.Fatalf("DeleteExpiredSessions (expired): %d %v", n, err)
	}
	if err := st.Vacuum(ctx); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
}

func TestProviders(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	p := &store.Provider{ID: "p1", Code: "searxng", Name: "SearXNG", Enabled: true, BaseURL: "http://x", Credentials: map[string]string{"api_key": "k"}, Params: map[string]string{"language": "ru"}}
	if err := st.UpsertProvider(ctx, p); err != nil {
		t.Fatalf("UpsertProvider: %v", err)
	}
	got, err := st.GetProvider(ctx, "p1")
	if err != nil || got.BaseURL != "http://x" || got.Credentials["api_key"] != "k" {
		t.Fatalf("GetProvider: %+v %v", got, err)
	}
	if err := st.UpdateProviderBaseURL(ctx, "p1", "http://y"); err != nil {
		t.Fatalf("UpdateProviderBaseURL: %v", err)
	}
	got, _ = st.GetProvider(ctx, "p1")
	if got.BaseURL != "http://y" {
		t.Fatalf("base url not updated: %+v", got)
	}
	list, _ := st.ListProviders(ctx)
	if len(list) != 1 {
		t.Fatalf("ListProviders: %d", len(list))
	}
	if err := st.DeleteProvider(ctx, "p1"); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
}

func TestPasskeys(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	user := &store.User{ID: "u1", Name: "admin", Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	pk := &store.Passkey{ID: "pk1", UserID: "u1", Name: "key", CredentialID: []byte("cred"), PublicKey: []byte("pub"), CreatedAt: store.Now()}
	if err := st.CreatePasskey(ctx, pk); err != nil {
		t.Fatalf("CreatePasskey: %v", err)
	}
	got, err := st.GetPasskeyByCredentialID(ctx, []byte("cred"))
	if err != nil || got.ID != "pk1" {
		t.Fatalf("GetPasskeyByCredentialID: %+v %v", got, err)
	}
	list, err := st.ListPasskeys(ctx, "u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListPasskeys: %+v %v", list, err)
	}
	got.SignCount = 5
	if err := st.UpdatePasskey(ctx, got); err != nil {
		t.Fatalf("UpdatePasskey: %v", err)
	}
	if err := st.DeletePasskey(ctx, "pk1"); err != nil {
		t.Fatalf("DeletePasskey: %v", err)
	}
}
