package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func postCalendarTestState(t *testing.T, ctx context.Context, store *SQLiteStore, at time.Time, inTotal, outTotal int64) {
	t.Helper()
	state := AgentStateRequest{
		TS:               at.Unix(),
		CPUPercent:       1,
		MemoryUsedBytes:  1,
		MemoryTotalBytes: 2,
		DiskUsedBytes:    1,
		DiskTotalBytes:   2,
		NetInTotalBytes:  inTotal,
		NetOutTotalBytes: outTotal,
		NetInSpeedBps:    1,
		NetOutSpeedBps:   1,
		UptimeSeconds:    1,
	}
	if err := store.InsertAgentState(ctx, "example-node-a", state); err != nil {
		t.Fatalf("insert state: %v", err)
	}
}

func calendarMonthUsage(t *testing.T, ctx context.Context, store *SQLiteStore) (month string, in, out int64) {
	t.Helper()
	if err := store.db.QueryRowContext(ctx, `
		SELECT month, in_bytes, out_bytes FROM traffic_calendar_monthly WHERE node_id = 'example-node-a'
	`).Scan(&month, &in, &out); err != nil {
		t.Fatalf("query calendar traffic: %v", err)
	}
	return month, in, out
}

func summaryCalendarUsage(t *testing.T, ctx context.Context, store *SQLiteStore) (in, out float64) {
	t.Helper()
	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(summary.Nodes))
	}
	node := summary.Nodes[0]
	if node.CalendarMonthInBytes == nil || node.CalendarMonthOutBytes == nil {
		t.Fatalf("calendar month usage = %v/%v, want non-nil", node.CalendarMonthInBytes, node.CalendarMonthOutBytes)
	}
	return *node.CalendarMonthInBytes, *node.CalendarMonthOutBytes
}

func TestCalendarMonthAccumulatesWithinMonth(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	postCalendarTestState(t, ctx, store, now, 1_000_000, 2_000_000)
	postCalendarTestState(t, ctx, store, now.Add(time.Second), 1_400_000, 2_600_000)

	month, in, out := calendarMonthUsage(t, ctx, store)
	if month != now.UTC().Format("2006-01") {
		t.Fatalf("calendar month = %s, want current month", month)
	}
	if in != 400_000 || out != 600_000 {
		t.Fatalf("calendar usage = %d/%d, want 400000/600000", in, out)
	}
	if gotIn, gotOut := summaryCalendarUsage(t, ctx, store); gotIn != 400_000 || gotOut != 600_000 {
		t.Fatalf("summary calendar usage = %v/%v, want 400000/600000", gotIn, gotOut)
	}
}

func TestCalendarMonthResetsOnMonthRollover(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	firstOfMonth := time.Date(2026, 9, 1, 0, 0, 5, 0, time.UTC)
	lastOfPrevMonth := time.Date(2026, 8, 31, 23, 59, 55, 0, time.UTC)
	postCalendarTestState(t, ctx, store, lastOfPrevMonth, 1_000_000, 2_000_000)
	postCalendarTestState(t, ctx, store, lastOfPrevMonth.Add(2*time.Second), 1_400_000, 2_600_000)
	// First sample of the new month carries the cross-boundary delta, then the
	// running month accumulates normally.
	postCalendarTestState(t, ctx, store, firstOfMonth, 1_900_000, 3_100_000)
	postCalendarTestState(t, ctx, store, firstOfMonth.Add(time.Second), 2_400_000, 3_700_000)

	month, in, out := calendarMonthUsage(t, ctx, store)
	if month != "2026-09" {
		t.Fatalf("calendar month = %s, want 2026-09", month)
	}
	if in != 1_000_000 || out != 1_100_000 {
		t.Fatalf("calendar usage = %d/%d, want 1000000/1100000", in, out)
	}

	var rows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM traffic_calendar_monthly`).Scan(&rows); err != nil {
		t.Fatalf("count calendar rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("calendar rows = %d, want exactly 1 (no history retained)", rows)
	}
}

func TestCalendarMonthIgnoresOutOfOrderSamples(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	postCalendarTestState(t, ctx, store, now, 1_000_000, 2_000_000)
	postCalendarTestState(t, ctx, store, now.Add(2*time.Second), 1_400_000, 2_600_000)
	// Late duplicate of the first sample must not double-count.
	postCalendarTestState(t, ctx, store, now, 1_000_000, 2_000_000)

	if _, in, out := calendarMonthUsage(t, ctx, store); in != 400_000 || out != 600_000 {
		t.Fatalf("calendar usage = %d/%d, want 400000/600000", in, out)
	}
}
