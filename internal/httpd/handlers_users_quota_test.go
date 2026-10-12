package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/store"
)

// putQuota issues PUT /api/users/{id}/quota with the given JSON body.
func putQuota(t *testing.T, ts *testServer, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return ts.do(t, http.MethodPut, "/api/users/"+userID+"/quota", json.RawMessage(body), true)
}

func TestUserQuotaHandlers(t *testing.T) {
	ts := newTestServer(t, true)
	uid := ts.user.ID

	// GET on a user without a quota: unlimited shape.
	rec := ts.do(t, http.MethodGet, "/api/users/"+uid+"/quota", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET quota status %d: %s", rec.Code, rec.Body.String())
	}
	var empty map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if empty["requests"] != float64(0) {
		t.Fatalf("empty quota requests = %v, want 0", empty["requests"])
	}

	// PUT creates the quota.
	rec = putQuota(t, ts, uid, `{"requests": 5, "period": "weekly", "tz_offset": 480}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT quota status %d: %s", rec.Code, rec.Body.String())
	}
	var status store.UserQuotaStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Requests != 5 || status.Period != store.QuotaPeriodWeekly || status.TzOffset != 480 {
		t.Fatalf("status = %+v", status)
	}
	if status.WindowHours != 24 {
		t.Fatalf("window_hours default = %d, want 24", status.WindowHours)
	}
	if status.Remaining != 5 || status.Used != 0 {
		t.Fatalf("used/remaining = %d/%d, want 0/5", status.Used, status.Remaining)
	}
	if status.ResetAt == "" {
		t.Fatal("weekly quota must expose reset_at")
	}

	// The row is persisted and the guard cache was invalidated: a search now
	// counts against the user quota.
	q, err := ts.store.GetUserQuota(t.Context(), uid)
	if err != nil || q.Requests != 5 {
		t.Fatalf("GetUserQuota: %+v err %v", q, err)
	}

	// GET reflects the row.
	rec = ts.do(t, http.MethodGet, "/api/users/"+uid+"/quota", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET quota after PUT status %d", rec.Code)
	}

	// PUT with a partial body keeps the other fields.
	rec = putQuota(t, ts, uid, `{"requests": 7}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial PUT status %d: %s", rec.Code, rec.Body.String())
	}
	q, _ = ts.store.GetUserQuota(t.Context(), uid)
	if q.Requests != 7 || q.Period != store.QuotaPeriodWeekly || q.TzOffset != 480 {
		t.Fatalf("after partial PUT = %+v", q)
	}

	// Invalid values are rejected.
	for _, body := range []string{
		`{"requests": -1}`,
		`{"period": "hourly"}`,
		`{"window_hours": 0}`,
		`{"window_hours": 721}`,
		`{"tz_offset": 900}`,
	} {
		if rec := putQuota(t, ts, uid, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s: status %d, want 400", body, rec.Code)
		}
	}

	// DELETE removes the quota: the user is unlimited again.
	rec = ts.do(t, http.MethodDelete, "/api/users/"+uid+"/quota", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE quota status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := ts.store.GetUserQuota(t.Context(), uid); err != store.ErrNotFound {
		t.Fatalf("after DELETE: got err %v, want ErrNotFound", err)
	}
}

func TestUserQuotaHandlersUnknownUser(t *testing.T) {
	ts := newTestServer(t, true)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := ts.do(t, method, "/api/users/no-such-user/quota", json.RawMessage(`{}`), true)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s unknown user: status %d, want 404", method, rec.Code)
		}
	}
}

// TestUserQuotaRequiresAdmin verifies the quota endpoints sit behind the
// admin scope like the other user mutations.
func TestUserQuotaRequiresAdmin(t *testing.T) {
	ts := newTestServer(t, true)

	viewer := &store.User{ID: "viewer-1", Name: "v", Role: "viewer", CreatedAt: store.Now()}
	if err := ts.store.CreateUser(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	viewerKey := createSearchKey(t, ts, viewer.ID)

	rec := ts.do(t, http.MethodPut, "/api/users/"+ts.user.ID+"/quota", json.RawMessage(`{"requests": 1}`), false,
		map[string]string{"Authorization": "Bearer " + viewerKey})
	if rec.Code == http.StatusOK {
		t.Fatalf("viewer PUT quota: status %d, want 403", rec.Code)
	}
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("viewer PUT quota: status %d, want 403/401", rec.Code)
	}
}

// TestListUsersEmbedsQuota verifies GET /api/users carries the runtime quota
// view and that a search consumes it.
func TestListUsersEmbedsQuota(t *testing.T) {
	ts := newTestServer(t, true)
	uid := ts.user.ID
	rec := putQuota(t, ts, uid, `{"requests": 3, "period": "daily"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT quota: %d %s", rec.Code, rec.Body.String())
	}

	// One search burns one unit.
	if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "q", "count": 1}, true); rec.Code != http.StatusOK {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}

	rec = ts.do(t, http.MethodGet, "/api/users", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET users: %d", rec.Code)
	}
	var users []store.User
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1", len(users))
	}
	q := users[0].Quota
	if q == nil {
		t.Fatal("user must embed quota")
	}
	if q.Requests != 3 || q.Used != 1 || q.Remaining != 2 {
		t.Fatalf("quota view = %+v, want requests=3 used=1 remaining=2", q)
	}
}

// createSearchKey mints an API key with only the search scope for a user.
func createSearchKey(t *testing.T, ts *testServer, userID string) string {
	t.Helper()
	plaintext, prefix, hash, err := auth.GenerateKey(false)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := ts.store.CreateAPIKey(t.Context(), &store.APIKey{
		ID: "k-" + prefix, UserID: userID, Name: "k", Prefix: prefix, Hash: hash,
		Scopes: "search", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}
	return plaintext
}
