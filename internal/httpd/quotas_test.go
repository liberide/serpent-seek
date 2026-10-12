package httpd

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/store"
)

func TestQuotaPeriodKey(t *testing.T) {
	// Wednesday 2026-10-07 15:30 UTC.
	now := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)

	daily := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodDaily}
	if got := quotaPeriodKey(daily, now); got != "2026-10-07#daily" {
		t.Fatalf("daily key = %q", got)
	}

	weekly := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodWeekly}
	// 2026-10-07 is a Wednesday; the ISO week started Monday 2026-10-05.
	if got := quotaPeriodKey(weekly, now); got != "2026-10-05#weekly" {
		t.Fatalf("weekly key = %q", got)
	}
	// Monday itself belongs to the week starting that day.
	monday := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if got := quotaPeriodKey(weekly, monday); got != "2026-10-05#weekly" {
		t.Fatalf("weekly key on Monday = %q", got)
	}
	// Sunday belongs to the week that started the previous Monday.
	sunday := time.Date(2026, 10, 11, 23, 59, 0, 0, time.UTC)
	if got := quotaPeriodKey(weekly, sunday); got != "2026-10-05#weekly" {
		t.Fatalf("weekly key on Sunday = %q", got)
	}

	monthly := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodMonthly}
	if got := quotaPeriodKey(monthly, now); got != "2026-10-01#monthly" {
		t.Fatalf("monthly key = %q", got)
	}

	// tz_offset shifts the calendar boundary: UTC+8 (480) at 2026-10-07
	// 17:00 UTC is already 2026-10-08 01:00 local.
	tz := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodDaily, TzOffset: 480}
	if got := quotaPeriodKey(tz, now.Add(90*time.Minute)); got != "2026-10-08#daily" {
		t.Fatalf("daily key with tz = %q", got)
	}
}

func TestQuotaResetAt(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC) // Wednesday

	daily := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodDaily}
	reset, ok := quotaResetAt(daily, now)
	if !ok || !reset.Equal(time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily reset = %v ok=%v", reset, ok)
	}

	weekly := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodWeekly}
	reset, ok = quotaResetAt(weekly, now)
	if !ok || !reset.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly reset = %v ok=%v (want Monday 2026-10-12)", reset, ok)
	}
	// On Monday itself the reset is the *next* Monday, not today.
	monday := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	reset, _ = quotaResetAt(weekly, monday)
	if !reset.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly reset on Monday = %v", reset)
	}

	monthly := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodMonthly}
	reset, ok = quotaResetAt(monthly, now)
	if !ok || !reset.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("monthly reset = %v ok=%v", reset, ok)
	}

	// tz_offset: UTC+8 daily reset at 17:00 UTC is the next day 00:00+08:00.
	tz := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodDaily, TzOffset: 480}
	reset, ok = quotaResetAt(tz, now)
	if !ok || !reset.Equal(time.Date(2026, 10, 7, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily tz reset = %v ok=%v (want 2026-10-07 16:00 UTC = 2026-10-08 00:00+08:00)", reset, ok)
	}

	rolling := &store.UserQuota{UserID: "u", Requests: 10, Period: store.QuotaPeriodRolling, WindowHours: 6}
	if _, ok := quotaResetAt(rolling, now); ok {
		t.Fatal("rolling quota must not report a fixed reset time")
	}
}

func TestQuotaCache(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	cache := newQuotaCache(ts.store)

	// Unknown / unlimited users read as no quota.
	if q := cache.get(ctx, ts.user.ID); q != nil {
		t.Fatalf("no-row user: got %+v, want nil", q)
	}

	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 42, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}
	// The cached "no quota" entry expires after the TTL; invalidate forces a
	// re-read, which is what the admin handlers call.
	cache.invalidate(ts.user.ID)
	if q := cache.get(ctx, ts.user.ID); q == nil || q.Requests != 42 {
		t.Fatalf("after set: got %+v, want requests=42", q)
	}

	// Requests <= 0 means unlimited even when a row exists.
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 0, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota 0: %v", err)
	}
	cache.invalidate(ts.user.ID)
	if q := cache.get(ctx, ts.user.ID); q != nil {
		t.Fatalf("zero-request quota: got %+v, want nil", q)
	}
}

// TestSearchGuardUserQuotaDaily sets a per-user quota of 1 and verifies the
// second search is rejected with quota headers.
func TestSearchGuardUserQuotaDaily(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 1, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}

	first := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, true)
	if first.Code != http.StatusOK {
		t.Fatalf("first search status %d: %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("X-Serpent-Quota-Limit"); got != "1" {
		t.Fatalf("quota limit header = %q, want 1", got)
	}
	if got := first.Header().Get("X-Serpent-Quota-Remaining"); got != "0" {
		t.Fatalf("quota remaining header = %q, want 0", got)
	}
	if got := first.Header().Get("X-Serpent-Quota-Reset-At"); got == "" {
		t.Fatal("daily quota must set X-Serpent-Quota-Reset-At")
	}

	second := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "two", "count": 1}, true)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second search status %d, want 429: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "quota_exceeded") {
		t.Fatalf("expected quota_exceeded, got %s", second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("rejection must set Retry-After")
	}
	if got := second.Header().Get("X-Serpent-Quota-Reset-At"); got == "" {
		t.Fatal("rejection must set X-Serpent-Quota-Reset-At")
	}
}

// TestSearchGuardUserQuotaSharedAcrossKeys verifies that all of a user's API
// keys draw from the same per-user allowance.
func TestSearchGuardUserQuotaSharedAcrossKeys(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 1, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}

	// Burn the allowance with the primary key.
	if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, true); rec.Code != http.StatusOK {
		t.Fatalf("first search status %d: %s", rec.Code, rec.Body.String())
	}
	// A second key of the same user is rejected: the quota is per user.
	plaintext2, prefix2, hash2, err := auth.GenerateKey(false)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := ts.store.CreateAPIKey(ctx, &store.APIKey{
		ID: "key-two", UserID: ts.user.ID, Name: "two", Prefix: prefix2, Hash: hash2,
		Scopes: "search", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create second key: %v", err)
	}
	second := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "two", "count": 1}, false,
		map[string]string{"Authorization": "Bearer " + plaintext2})
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second key status %d, want 429 (quota is per user)", second.Code)
	}
}

// TestSearchGuardUserQuotaRolling uses a 1-hour rolling window and seeds hits
// directly in the store to verify the sliding-window count.
func TestSearchGuardUserQuotaRolling(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 1, Period: store.QuotaPeriodRolling, WindowHours: 1, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}

	// Seed one hit inside the window: usage is already at the limit.
	subject := "user:" + ts.user.ID
	if err := ts.store.AddRollingHit(ctx, subject, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("AddRollingHit: %v", err)
	}
	rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, true)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rolling quota status %d, want 429: %s", rec.Code, rec.Body.String())
	}
	// Rolling quotas have no fixed reset moment but must set Retry-After.
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("rolling rejection must set Retry-After")
	}
	if got := rec.Header().Get("X-Serpent-Quota-Reset-At"); got != "" {
		t.Fatalf("rolling rejection must not set a fixed reset time, got %q", got)
	}
}

// TestSearchGuardUserQuotaUnlimitedToggle replays the UI "unlimited"
// checkbox scenario end to end: a limit is set and enforced, then the admin
// ticks "unlimited" and saves. Both save paths the UI can take are covered
// (PUT requests=0 and DELETE), and after each the searches flow freely and a
// re-read of the quota reports no limit — so reopening the modal can never
// resurrect the old limit (no "chain" of limits behind a ticked checkbox).
func TestSearchGuardUserQuotaUnlimitedToggle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clear func(t *testing.T, ts *testServer, userID string)
	}{
		{"put_zero", func(t *testing.T, ts *testServer, userID string) {
			q, err := ts.store.GetUserQuota(context.Background(), userID)
			if err != nil {
				t.Fatalf("GetUserQuota: %v", err)
			}
			q.Requests = 0
			if err := ts.store.SetUserQuota(context.Background(), q); err != nil {
				t.Fatalf("SetUserQuota 0: %v", err)
			}
			ts.server.quotas.invalidate(userID)
		}},
		{"delete", func(t *testing.T, ts *testServer, userID string) {
			if err := ts.store.DeleteUserQuota(context.Background(), userID); err != nil {
				t.Fatalf("DeleteUserQuota: %v", err)
			}
			ts.server.quotas.invalidate(userID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t, true)
			ctx := context.Background()
			uid := ts.user.ID

			// 1. A limit of 1 is set: the second search is rejected.
			if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
				UserID: uid, Requests: 1, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
			}); err != nil {
				t.Fatalf("SetUserQuota: %v", err)
			}
			ts.server.quotas.invalidate(uid)
			if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, true); rec.Code != http.StatusOK {
				t.Fatalf("search with quota: %d %s", rec.Code, rec.Body.String())
			}
			if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "two", "count": 1}, true); rec.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429 under quota, got %d", rec.Code)
			}

			// 2. Admin ticks "unlimited" and saves.
			tc.clear(t, ts, uid)

			// 3. Searches flow freely again — no quota headers, no 429.
			for i := 0; i < 3; i++ {
				rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "free", "count": 1}, true)
				if rec.Code != http.StatusOK {
					t.Fatalf("after unlimited, search %d: %d %s", i, rec.Code, rec.Body.String())
				}
				if got := rec.Header().Get("X-Serpent-Quota-Limit"); got != "" {
					t.Fatalf("after unlimited, search %d carries quota header %q", i, got)
				}
			}

			// 4. Re-reading the quota (what the modal does on open) reports
			//    "no limit": PUT-zero leaves requests=0, DELETE leaves no row.
			q, err := ts.store.GetUserQuota(ctx, uid)
			if err == nil && q.Requests > 0 {
				t.Fatalf("quota row still holds requests=%d after unlimited save", q.Requests)
			}
			if err == nil && q.Requests == 0 {
				// requests=0 must behave as unlimited in the guard cache.
				if got := ts.server.quotas.get(ctx, uid); got != nil {
					t.Fatalf("cache returned quota %+v for a zero-request row", got)
				}
			}
			if err != nil && err != store.ErrNotFound {
				t.Fatalf("GetUserQuota: %v", err)
			}
		})
	}
}

// TestSearchGuardNoUserQuotaAnonymous verifies that with auth disabled (no
// user identity) a user quota row is simply not applied.
func TestSearchGuardNoUserQuotaAnonymous(t *testing.T) {
	ts := newTestServer(t, false)
	ctx := context.Background()
	if err := ts.store.SetUserQuota(ctx, &store.UserQuota{
		UserID: ts.user.ID, Requests: 1, Period: store.QuotaPeriodDaily, UpdatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}
	// Anonymous requests carry no user identity, so the quota does not apply.
	rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous search status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Serpent-Quota-Limit"); got != "" {
		t.Fatalf("anonymous search must not carry quota headers, got %q", got)
	}
}
