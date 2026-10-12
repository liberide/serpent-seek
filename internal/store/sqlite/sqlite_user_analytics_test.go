package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// seedOwnerRequests inserts requests for several owners (plus anonymous ones)
// so per-user aggregation queries have something to chew on.
func seedOwnerRequests(t *testing.T, st store.Storage) {
	t.Helper()
	ctx := context.Background()
	now := store.Now()
	reqs := []*store.Request{
		{ID: "u1-a", RID: "u1-a", Query: "q1", Status: "ok", TotalMS: 100, UserID: "user-1", CreatedAt: now},
		{ID: "u1-b", RID: "u1-b", Query: "q1", Status: "fail", TotalMS: 300, UserID: "user-1", CreatedAt: now},
		{ID: "u2-a", RID: "u2-a", Query: "q2", Status: "empty", TotalMS: 200, UserID: "user-2", CreatedAt: now},
		{ID: "anon-a", RID: "anon-a", Query: "q3", Status: "ok", TotalMS: 50, CreatedAt: now},
	}
	for _, r := range reqs {
		if err := st.CreateRequest(ctx, r); err != nil {
			t.Fatalf("CreateRequest %s: %v", r.ID, err)
		}
	}
	// A billed step for user-1 so the per-user cost aggregation is covered.
	if err := st.AddStep(ctx, &store.RequestStep{
		ID: "u1-s1", RequestID: "u1-a", Idx: 0, Provider: "p", Status: "ok",
		Kind: "ok", Cost: 0.25, StartedAt: now, FinishedAt: now,
	}); err != nil {
		t.Fatalf("AddStep: %v", err)
	}
}

func TestAnalyticsUserFilter(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	seedOwnerRequests(t, st)

	report, err := st.Analytics(ctx, store.AnalyticsFilter{UserID: "user-1"})
	if err != nil {
		t.Fatalf("Analytics: %v", err)
	}
	if report.TotalRequests != 2 || report.OK != 1 || report.Fail != 1 {
		t.Fatalf("user-1 aggregates = %+v, want 2 requests (1 ok, 1 fail)", report)
	}
	if report.AvgMS != 200 {
		t.Fatalf("user-1 avg_ms = %d, want 200", report.AvgMS)
	}
	if report.CostTotal != 0.25 {
		t.Fatalf("user-1 cost = %v, want 0.25", report.CostTotal)
	}
	if len(report.Providers) != 1 || report.Providers[0].Provider != "p" || report.Providers[0].Cost != 0.25 {
		t.Fatalf("user-1 providers = %+v, want one provider p with cost 0.25", report.Providers)
	}

	// The anonymous bucket matches requests without a user only.
	anon, err := st.Analytics(ctx, store.AnalyticsFilter{UserID: store.AnonymousUserID})
	if err != nil {
		t.Fatalf("Analytics anonymous: %v", err)
	}
	if anon.TotalRequests != 1 || anon.OK != 1 {
		t.Fatalf("anonymous aggregates = %+v, want 1 ok request", anon)
	}

	// Without the filter everything is included.
	all, err := st.Analytics(ctx, store.AnalyticsFilter{})
	if err != nil {
		t.Fatalf("Analytics unfiltered: %v", err)
	}
	if all.TotalRequests != 4 {
		t.Fatalf("unfiltered total = %d, want 4", all.TotalRequests)
	}
	if len(all.Providers) != 1 || all.Providers[0].Cost != 0.25 {
		t.Fatalf("unfiltered providers = %+v", all.Providers)
	}

	// Unknown owners produce an empty report, not an error (requests outlive
	// deleted users).
	ghost, err := st.Analytics(ctx, store.AnalyticsFilter{UserID: "ghost"})
	if err != nil {
		t.Fatalf("Analytics ghost: %v", err)
	}
	if ghost.TotalRequests != 0 || len(ghost.Daily) == 0 {
		t.Fatalf("ghost aggregates = %+v, want empty report with filled days", ghost)
	}
}

func TestUsersOverview(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	seedOwnerRequests(t, st)

	rows, err := st.UsersOverview(ctx, "", "")
	if err != nil {
		t.Fatalf("UsersOverview: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want 3 owners", rows)
	}
	// Volume ordering: user-1 first, then user-2 and anonymous (1 request each).
	if rows[0].UserID != "user-1" || rows[0].Requests != 2 || rows[0].OK != 1 || rows[0].Fail != 1 {
		t.Fatalf("top row = %+v, want user-1 with 2 requests", rows[0])
	}
	if rows[0].Cost != 0.25 {
		t.Fatalf("user-1 cost = %v, want 0.25", rows[0].Cost)
	}
	if rows[0].LastActive == "" {
		t.Fatal("user-1 last_active must be set")
	}
	anonSeen, user2Seen := false, false
	for _, r := range rows[1:] {
		switch r.UserID {
		case store.AnonymousUserID:
			anonSeen = true
			if r.Requests != 1 || r.OK != 1 || r.Cost != 0 {
				t.Fatalf("anonymous row = %+v", r)
			}
		case "user-2":
			user2Seen = true
			if r.Requests != 1 || r.Empty != 1 {
				t.Fatalf("user-2 row = %+v", r)
			}
		default:
			t.Fatalf("unexpected owner %q in %+v", r.UserID, r)
		}
	}
	if !anonSeen || !user2Seen {
		t.Fatalf("missing rows: anonymous=%v user-2=%v", anonSeen, user2Seen)
	}

	// The anonymous pseudo id never collides with a real user id.
	for _, r := range rows {
		if r.UserID == "" {
			t.Fatal("anonymous bucket must be reported under the pseudo id, not \"\"")
		}
	}

	// Invalid range errors instead of silently returning everything.
	if _, err := st.UsersOverview(ctx, "nope", ""); err == nil {
		t.Fatal("expected error for invalid date range")
	}
}
