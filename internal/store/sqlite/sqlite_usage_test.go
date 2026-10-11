package sqlite

import (
	"context"
	"testing"
)

func TestUsageCounters(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	const subject = "key:abc"
	const day = "2026-10-09"

	// A missing counter reads as zero.
	if got, err := st.GetUsage(ctx, subject, day); err != nil || got != 0 {
		t.Fatalf("GetUsage missing: got %d err %v", got, err)
	}

	// Increments are atomic and cumulative.
	for want := 1; want <= 3; want++ {
		got, err := st.IncrementUsage(ctx, subject, day)
		if err != nil {
			t.Fatalf("IncrementUsage: %v", err)
		}
		if got != want {
			t.Fatalf("IncrementUsage: got %d want %d", got, want)
		}
	}
	if got, _ := st.GetUsage(ctx, subject, day); got != 3 {
		t.Fatalf("GetUsage after increments: %d", got)
	}

	// Subjects and days are independent.
	if _, err := st.IncrementUsage(ctx, "ip:127.0.0.1", day); err != nil {
		t.Fatalf("IncrementUsage other subject: %v", err)
	}
	if _, err := st.IncrementUsage(ctx, subject, "2026-10-10"); err != nil {
		t.Fatalf("IncrementUsage other day: %v", err)
	}

	usage, err := st.UsageByDay(ctx, day)
	if err != nil {
		t.Fatalf("UsageByDay: %v", err)
	}
	if usage[subject] != 3 || usage["ip:127.0.0.1"] != 1 || len(usage) != 2 {
		t.Fatalf("UsageByDay: %+v", usage)
	}
	if next, _ := st.UsageByDay(ctx, "2026-10-10"); next[subject] != 1 || len(next) != 1 {
		t.Fatalf("UsageByDay next day: %+v", next)
	}

	// Cleanup deletes only days strictly before the cutoff.
	n, err := st.DeleteOldUsage(ctx, "2026-10-10")
	if err != nil {
		t.Fatalf("DeleteOldUsage: %v", err)
	}
	if n != 2 {
		t.Fatalf("DeleteOldUsage removed %d rows, want 2", n)
	}
	if usage, _ := st.UsageByDay(ctx, day); len(usage) != 0 {
		t.Fatalf("day should be empty after cleanup: %+v", usage)
	}
	if usage, _ := st.UsageByDay(ctx, "2026-10-10"); usage[subject] != 1 {
		t.Fatalf("cutoff day must be kept: %+v", usage)
	}
}
