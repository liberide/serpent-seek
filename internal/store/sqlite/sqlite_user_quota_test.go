package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/store"
)

func quotaTestUser(t *testing.T, st *store.Storage, name string) *store.User {
	t.Helper()
	u := &store.User{ID: name + "-id", Name: name, Role: "viewer", CreatedAt: store.Now()}
	if err := (*st).CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func TestUserQuotaCRUD(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	u := quotaTestUser(t, st, "quota-crud")

	// No row: ErrNotFound.
	if _, err := (*st).GetUserQuota(ctx, u.ID); err != store.ErrNotFound {
		t.Fatalf("GetUserQuota missing: got %v, want ErrNotFound", err)
	}

	// Upsert creates.
	q := &store.UserQuota{UserID: u.ID, Requests: 100, Period: store.QuotaPeriodDaily, WindowHours: 24, UpdatedAt: store.Now()}
	if err := (*st).SetUserQuota(ctx, q); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}
	got, err := (*st).GetUserQuota(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserQuota: %v", err)
	}
	if got.Requests != 100 || got.Period != store.QuotaPeriodDaily || got.WindowHours != 24 || got.TzOffset != 0 {
		t.Fatalf("GetUserQuota: %+v", got)
	}

	// Upsert updates in place (still exactly one row).
	q.Requests = 5
	q.Period = store.QuotaPeriodRolling
	q.WindowHours = 12
	q.TzOffset = 480
	q.UpdatedAt = store.Now()
	if err := (*st).SetUserQuota(ctx, q); err != nil {
		t.Fatalf("SetUserQuota update: %v", err)
	}
	got, err = (*st).GetUserQuota(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserQuota after update: %v", err)
	}
	if got.Requests != 5 || got.Period != store.QuotaPeriodRolling || got.WindowHours != 12 || got.TzOffset != 480 {
		t.Fatalf("GetUserQuota after update: %+v", got)
	}

	// List returns the row keyed by user id.
	all, err := (*st).ListUserQuotas(ctx)
	if err != nil {
		t.Fatalf("ListUserQuotas: %v", err)
	}
	if len(all) != 1 || all[u.ID] == nil || all[u.ID].Requests != 5 {
		t.Fatalf("ListUserQuotas: %+v", all)
	}

	// Delete removes the row.
	if err := (*st).DeleteUserQuota(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUserQuota: %v", err)
	}
	if _, err := (*st).GetUserQuota(ctx, u.ID); err != store.ErrNotFound {
		t.Fatalf("GetUserQuota after delete: got %v, want ErrNotFound", err)
	}
	if all, _ := (*st).ListUserQuotas(ctx); len(all) != 0 {
		t.Fatalf("ListUserQuotas after delete: %+v", all)
	}
}

func TestUserQuotaCascadeOnUserDelete(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	u := quotaTestUser(t, st, "quota-cascade")

	q := &store.UserQuota{UserID: u.ID, Requests: 10, Period: store.QuotaPeriodWeekly, UpdatedAt: store.Now()}
	if err := (*st).SetUserQuota(ctx, q); err != nil {
		t.Fatalf("SetUserQuota: %v", err)
	}
	if err := (*st).DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := (*st).GetUserQuota(ctx, u.ID); err != store.ErrNotFound {
		t.Fatalf("GetUserQuota after user delete: got %v, want ErrNotFound", err)
	}
}

func TestUserQuotaInvalidPeriodRejected(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	u := quotaTestUser(t, st, "quota-bad-period")

	q := &store.UserQuota{UserID: u.ID, Requests: 10, Period: "hourly", UpdatedAt: store.Now()}
	if err := (*st).SetUserQuota(ctx, q); err == nil {
		t.Fatalf("SetUserQuota with invalid period: expected CHECK constraint error, got nil")
	}
}

func TestRollingHits(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	const subject = "user:roll-1"
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	ts := func(minAgo int) string {
		return base.Add(-time.Duration(minAgo) * time.Minute).Format(time.RFC3339Nano)
	}

	// No hits.
	if n, err := (*st).CountRollingHits(ctx, subject, ts(60)); err != nil || n != 0 {
		t.Fatalf("CountRollingHits empty: got %d err %v", n, err)
	}

	// Three hits at 50, 30 and 5 minutes ago.
	for _, ago := range []int{50, 30, 5} {
		if err := (*st).AddRollingHit(ctx, subject, ts(ago)); err != nil {
			t.Fatalf("AddRollingHit: %v", err)
		}
	}
	// Duplicate insert is a no-op (same subject+ts).
	if err := (*st).AddRollingHit(ctx, subject, ts(5)); err != nil {
		t.Fatalf("AddRollingHit duplicate: %v", err)
	}

	// A 24h window sees all three.
	if n, _ := (*st).CountRollingHits(ctx, subject, ts(24*60)); n != 3 {
		t.Fatalf("CountRollingHits 24h: got %d, want 3", n)
	}
	// A 40-minute window sees only the two recent hits (inclusive bound).
	if n, _ := (*st).CountRollingHits(ctx, subject, ts(40)); n != 2 {
		t.Fatalf("CountRollingHits 40m: got %d, want 2", n)
	}
	// Subjects are independent.
	if n, _ := (*st).CountRollingHits(ctx, "user:other", ts(24*60)); n != 0 {
		t.Fatalf("CountRollingHits other subject: got %d, want 0", n)
	}

	// Cleanup removes hits strictly before the cutoff.
	n, err := (*st).DeleteOldRollingHits(ctx, ts(20))
	if err != nil {
		t.Fatalf("DeleteOldRollingHits: %v", err)
	}
	if n != 2 {
		t.Fatalf("DeleteOldRollingHits removed %d, want 2", n)
	}
	if n, _ := (*st).CountRollingHits(ctx, subject, ts(24*60)); n != 1 {
		t.Fatalf("CountRollingHits after cleanup: got %d, want 1", n)
	}
}

// TestDeleteOldUsagePrunesQuotaPeriodKeys verifies that the date-prefix
// cutoff used by the cleanup job also prunes weekly/monthly quota period
// keys ("<start-date>#<period>").
func TestDeleteOldUsagePrunesQuotaPeriodKeys(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	for _, day := range []string{"2026-10-01", "2026-10-05#weekly", "2026-09-01#monthly", "2026-10-09"} {
		if _, err := (*st).IncrementUsage(ctx, "user:x", day); err != nil {
			t.Fatalf("IncrementUsage %q: %v", day, err)
		}
	}

	// Cutoff at the start of 2026-10-09 keeps only that day; the weekly key
	// for W40 (which ended 2026-10-05) and monthly 2026-09 both sort before it.
	n, err := (*st).DeleteOldUsage(ctx, "2026-10-09")
	if err != nil {
		t.Fatalf("DeleteOldUsage: %v", err)
	}
	if n != 3 {
		t.Fatalf("DeleteOldUsage removed %d, want 3", n)
	}
	if got, _ := (*st).GetUsage(ctx, "user:x", "2026-10-09"); got != 1 {
		t.Fatalf("remaining day counter: got %d, want 1", got)
	}
}
