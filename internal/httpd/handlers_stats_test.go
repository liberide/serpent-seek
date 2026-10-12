package httpd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/store"
)

// seedStatsRequests stores one request per owner so the per-user analytics
// endpoints have data without driving the engine.
func seedStatsRequests(t *testing.T, st store.Storage, counts map[string]int) {
	t.Helper()
	ctx := context.Background()
	for owner, n := range counts {
		for i := 0; i < n; i++ {
			req := &store.Request{
				ID: uuid.NewString(), RID: uuid.NewString()[:8], Query: "stats q",
				Status: "ok", TotalMS: 42, CreatedAt: store.Now(),
			}
			if owner != "anonymous" {
				req.UserID = owner
			}
			if err := st.CreateRequest(ctx, req); err != nil {
				t.Fatalf("CreateRequest: %v", err)
			}
		}
	}
}

// createViewerWithKey registers a non-admin viewer user with a read-scope key
// and returns the user and the key plaintext.
func createViewerWithKey(t *testing.T, st store.Storage) (*store.User, string) {
	t.Helper()
	ctx := context.Background()
	viewer := &store.User{ID: uuid.NewString(), Name: "viewer", Role: "viewer", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	plaintext, prefix, hash, err := auth.GenerateKey(false)
	if err != nil {
		t.Fatalf("generate viewer key: %v", err)
	}
	if err := st.CreateAPIKey(ctx, &store.APIKey{
		ID: uuid.NewString(), UserID: viewer.ID, Name: "viewer key", Prefix: prefix,
		Hash: hash, Scopes: "search,read", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create viewer key: %v", err)
	}
	return viewer, plaintext
}

func TestStatsUsersLeaderboard(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()

	viewer, viewerPlaintext := createViewerWithKey(t, ts.store)
	// A quota for the viewer ties the leaderboard to the quota feature.
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: viewer.ID, Requests: 50, Period: store.QuotaPeriodDaily,
		WindowHours: 24, TzOffset: 0, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("set viewer quota: %v", err)
	}

	seedStatsRequests(t, ts.store, map[string]int{
		ts.user.ID:  3,
		viewer.ID:   1,
		"anonymous": 2,
	})
	// Quota usage is counted by the search guard in usage_daily (not from the
	// requests table), so simulate one admitted request for the viewer using
	// the same period key the guard would write.
	day := quotaPeriodKey(&store.UserQuota{UserID: viewer.ID, Requests: 50, Period: store.QuotaPeriodDaily}, time.Now())
	if _, err := ts.store.IncrementUsage(ctx, "user:"+viewer.ID, day); err != nil {
		t.Fatalf("increment usage: %v", err)
	}

	// Non-admin callers are rejected.
	denied := ts.do(t, http.MethodGet, "/api/stats/users", nil, false,
		map[string]string{"Authorization": "Bearer " + viewerPlaintext})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d body %s", denied.Code, denied.Body.String())
	}

	rec := ts.do(t, http.MethodGet, "/api/stats/users", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats/users status %d body %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Rows []struct {
			UserID     string `json:"user_id"`
			Name       string `json:"name"`
			Role       string `json:"role"`
			Exists     bool   `json:"exists"`
			Requests   int    `json:"requests"`
			OK         int    `json:"ok"`
			AvgMS      int    `json:"avg_ms"`
			KeysCount  int    `json:"keys_count"`
			LastActive string `json:"last_active"`
			Quota      *struct {
				Requests  int    `json:"requests"`
				Period    string `json:"period"`
				Remaining int    `json:"remaining"`
				ResetAt   string `json:"reset_at"`
			} `json:"quota"`
		} `json:"rows"`
		Totals struct {
			Requests    int `json:"requests"`
			ActiveUsers int `json:"active_users"`
		} `json:"totals"`
		Range map[string]string `json:"range"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode stats/users: %v", err)
	}
	if payload.Range["from"] == "" || payload.Range["to"] == "" {
		t.Fatalf("missing effective range: %v", payload.Range)
	}
	if payload.Totals.Requests != 6 {
		t.Fatalf("totals.requests = %d, want 6", payload.Totals.Requests)
	}
	if payload.Totals.ActiveUsers != 2 {
		t.Fatalf("totals.active_users = %d, want 2", payload.Totals.ActiveUsers)
	}

	type quotaView struct {
		Requests  int    `json:"requests"`
		Period    string `json:"period"`
		Remaining int    `json:"remaining"`
		ResetAt   string `json:"reset_at"`
	}
	requests := map[string]int{}
	keys := map[string]int{}
	quotas := map[string]*quotaView{}
	names := map[string]string{}
	for _, row := range payload.Rows {
		requests[row.UserID] = row.Requests
		keys[row.UserID] = row.KeysCount
		names[row.UserID] = row.Name
		if row.Quota != nil {
			quotas[row.UserID] = &quotaView{
				Requests: row.Quota.Requests, Period: row.Quota.Period,
				Remaining: row.Quota.Remaining, ResetAt: row.Quota.ResetAt,
			}
		}
	}
	if requests[ts.user.ID] != 3 || keys[ts.user.ID] != 1 || names[ts.user.ID] != "admin" || quotas[ts.user.ID] != nil {
		t.Fatalf("admin row wrong: requests=%v keys=%v names=%v quotas=%v", requests, keys, names, quotas)
	}
	if requests[viewer.ID] != 1 || keys[viewer.ID] != 1 || names[viewer.ID] != "viewer" {
		t.Fatalf("viewer row wrong: %v %v %v", requests, keys, names)
	}
	q := quotas[viewer.ID]
	if q == nil || q.Requests != 50 || q.Period != store.QuotaPeriodDaily || q.Remaining != 49 || q.ResetAt == "" {
		t.Fatalf("viewer quota = %+v", q)
	}
	if requests[store.AnonymousUserID] != 2 {
		t.Fatalf("anonymous row wrong: %v", requests)
	}

	// Invalid range is rejected like on /api/stats/analytics.
	bad := ts.do(t, http.MethodGet, "/api/stats/users?from=nope", nil, true)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid range, got %d", bad.Code)
	}
}

func TestStatsAnalyticsUserFilter(t *testing.T) {
	ts := newTestServer(t, true)
	viewer, viewerPlaintext := createViewerWithKey(t, ts.store)
	seedStatsRequests(t, ts.store, map[string]int{
		ts.user.ID:  2,
		viewer.ID:   1,
		"anonymous": 1,
	})

	fetch := func(userID string, key string) (int, int) {
		t.Helper()
		path := "/api/stats/analytics"
		if userID != "" {
			path += "?user_id=" + userID
		}
		var rec *httptest.ResponseRecorder
		if key == "" {
			rec = ts.do(t, http.MethodGet, path, nil, true)
		} else {
			rec = ts.do(t, http.MethodGet, path, nil, false,
				map[string]string{"Authorization": "Bearer " + key})
		}
		if rec.Code != http.StatusOK {
			return rec.Code, -1
		}
		var payload struct {
			Report struct {
				TotalRequests int `json:"total_requests"`
			} `json:"report"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode analytics: %v body %s", err, rec.Body.String())
		}
		return rec.Code, payload.Report.TotalRequests
	}

	// Admin can filter by any owner, including the anonymous bucket.
	if code, total := fetch(ts.user.ID, ""); code != http.StatusOK || total != 2 {
		t.Fatalf("admin user filter: code %d total %d", code, total)
	}
	if code, total := fetch(store.AnonymousUserID, ""); code != http.StatusOK || total != 1 {
		t.Fatalf("admin anonymous filter: code %d total %d", code, total)
	}
	// Unfiltered keeps its global meaning.
	if code, total := fetch("", ""); code != http.StatusOK || total != 4 {
		t.Fatalf("unfiltered: code %d total %d", code, total)
	}
	// Non-admin sees their own traffic.
	if code, total := fetch(viewer.ID, viewerPlaintext); code != http.StatusOK || total != 1 {
		t.Fatalf("viewer own filter: code %d total %d", code, total)
	}
	// Non-admin cannot inspect someone else, nor the anonymous bucket.
	if code, _ := fetch(ts.user.ID, viewerPlaintext); code != http.StatusForbidden {
		t.Fatalf("viewer other filter: expected 403, got %d", code)
	}
	if code, _ := fetch(store.AnonymousUserID, viewerPlaintext); code != http.StatusForbidden {
		t.Fatalf("viewer anonymous filter: expected 403, got %d", code)
	}
}

func TestStatsUsersOnlyActiveKeysCounted(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()

	viewer := &store.User{ID: uuid.NewString(), Name: "viewer", Role: "viewer", CreatedAt: store.Now()}
	if err := ts.store.CreateUser(ctx, viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	_, prefix, hash, err := auth.GenerateKey(false)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := ts.store.CreateAPIKey(ctx, &store.APIKey{
		ID: uuid.NewString(), UserID: viewer.ID, Name: "revoked", Prefix: prefix,
		Hash: hash, Scopes: "search", RevokedAt: store.Now(), CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create revoked key: %v", err)
	}
	seedStatsRequests(t, ts.store, map[string]int{viewer.ID: 1})

	rec := ts.do(t, http.MethodGet, "/api/stats/users", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats/users status %d body %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Rows []struct {
			UserID    string `json:"user_id"`
			KeysCount int    `json:"keys_count"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode stats/users: %v", err)
	}
	for _, row := range payload.Rows {
		if row.UserID == viewer.ID && row.KeysCount != 0 {
			t.Fatalf("revoked keys must not count: %+v", row)
		}
	}
}
