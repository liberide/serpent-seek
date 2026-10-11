package sqlite

import (
	"context"
	"strconv"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func TestCursorPaginationRequests(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	for i, ts := range []string{"2026-01-01T00:00:01Z", "2026-01-01T00:00:02Z", "2026-01-01T00:00:03Z"} {
		req := &store.Request{
			ID: "r" + strconv.Itoa(i+1), RID: "rid" + strconv.Itoa(i+1),
			Query: "q", Status: "ok", CreatedAt: ts,
		}
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}
	first, total, err := st.ListRequests(ctx, store.RequestFilter{}, store.Page{Limit: 2})
	if err != nil || total != 3 || len(first) != 2 {
		t.Fatalf("first page: total=%d len=%d err=%v", total, len(first), err)
	}
	if first[0].ID != "r3" || first[1].ID != "r2" {
		t.Fatalf("unexpected first page order: %s %s", first[0].ID, first[1].ID)
	}
	cursor := store.EncodeCursor(first[len(first)-1].CreatedAt, first[len(first)-1].ID)
	second, total, err := st.ListRequests(ctx, store.RequestFilter{Cursor: cursor}, store.Page{Limit: 2})
	if err != nil || total != 3 || len(second) != 1 {
		t.Fatalf("second page: total=%d len=%d err=%v", total, len(second), err)
	}
	if second[0].ID != "r1" {
		t.Fatalf("expected r1 on the second page, got %s", second[0].ID)
	}
}

func TestCursorPaginationLogs(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	for _, msg := range []string{"one", "two", "three"} {
		if err := st.AddLog(ctx, &store.LogEntry{TS: store.Now(), Level: "info", Message: msg}); err != nil {
			t.Fatalf("AddLog: %v", err)
		}
	}
	first, total, err := st.ListLogs(ctx, store.LogFilter{}, store.Page{Limit: 2})
	if err != nil || total != 3 || len(first) != 2 {
		t.Fatalf("first page: total=%d len=%d err=%v", total, len(first), err)
	}
	if first[0].Message != "three" || first[1].Message != "two" {
		t.Fatalf("unexpected first page order: %s %s", first[0].Message, first[1].Message)
	}
	cursor := store.EncodeCursor(strconv.FormatInt(first[len(first)-1].ID, 10))
	second, _, err := st.ListLogs(ctx, store.LogFilter{Cursor: cursor}, store.Page{Limit: 2})
	if err != nil || len(second) != 1 || second[0].Message != "one" {
		t.Fatalf("second page: len=%d err=%v", len(second), err)
	}
}
