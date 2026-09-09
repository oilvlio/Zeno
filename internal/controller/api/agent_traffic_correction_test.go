package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func postCorrectionTestState(t *testing.T, ctx context.Context, store *SQLiteStore, at time.Time, inTotal, outTotal int64) {
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

func floatPtrValue(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}

func correctionBytes(t *testing.T, ctx context.Context, store *SQLiteStore) (in, out, billable int64) {
	t.Helper()
	if err := store.db.QueryRowContext(ctx, `
		SELECT in_bytes, out_bytes, billable_bytes FROM traffic_monthly WHERE node_id = 'example-node-a'
	`).Scan(&in, &out, &billable); err != nil {
		t.Fatalf("query monthly traffic: %v", err)
	}
	return in, out, billable
}

func TestMonthlyTrafficCorrectionSnapshotsBilledUsage(t *testing.T) {
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
	postCorrectionTestState(t, ctx, store, now, 1_000_000, 2_000_000)
	postCorrectionTestState(t, ctx, store, now.Add(time.Second), 1_400_000, 2_600_000)

	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyInCorrectionBytes:  adminOptionalInt64{Set: true, Valid: true, Value: 1_000},
		MonthlyOutCorrectionBytes: adminOptionalInt64{Set: true, Valid: true, Value: 2_000},
	}); err != nil {
		t.Fatalf("update correction: %v", err)
	}

	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(summary.Nodes))
	}
	node := summary.Nodes[0]
	// Snapshot semantics: the entered values ARE the provider's billed totals,
	// so previously measured usage is discarded and only the snapshot bills.
	if node.MonthlyBillableBytes == nil || *node.MonthlyBillableBytes != 3_000 {
		t.Fatalf("monthly billable = %v, want 3000 (snapshot only)", floatPtrValue(node.MonthlyBillableBytes))
	}
	// Lifetime counters stay honest: the first sample seeds the baseline at its
	// full counter value, later samples add deltas. Corrections never apply.
	if node.NetInLifetimeBytes == nil || *node.NetInLifetimeBytes != 1_400_000 {
		t.Fatalf("lifetime receive = %v, want 1400000 untouched by correction", floatPtrValue(node.NetInLifetimeBytes))
	}
	if node.NetOutLifetimeBytes == nil || *node.NetOutLifetimeBytes != 2_600_000 {
		t.Fatalf("lifetime send = %v, want 2600000 untouched by correction", floatPtrValue(node.NetOutLifetimeBytes))
	}

	measuredIn, measuredOut, measuredBillable := correctionBytes(t, ctx, store)
	if measuredIn != 0 || measuredOut != 0 || measuredBillable != 0 {
		t.Fatalf("stored measured = %d/%d/%d, want 0/0/0 reset by snapshot", measuredIn, measuredOut, measuredBillable)
	}

	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("admin nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].MonthlyInCorrectionBytes == nil || *nodes[0].MonthlyInCorrectionBytes != 1_000 {
		t.Fatalf("admin in correction = %v, want 1000", nodes[0].MonthlyInCorrectionBytes)
	}
	if nodes[0].MonthlyOutCorrectionBytes == nil || *nodes[0].MonthlyOutCorrectionBytes != 2_000 {
		t.Fatalf("admin out correction = %v, want 2000", nodes[0].MonthlyOutCorrectionBytes)
	}

	// Later samples accumulate on top of the snapshot: the first sample after
	// the reset only re-establishes the counter baseline, the second bills.
	postCorrectionTestState(t, ctx, store, now.Add(2*time.Second), 1_900_000, 3_100_000)
	postCorrectionTestState(t, ctx, store, now.Add(3*time.Second), 2_400_000, 3_700_000)
	summary, err = store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary after later samples: %v", err)
	}
	if got := *summary.Nodes[0].MonthlyBillableBytes; got != 1_103_000 {
		t.Fatalf("monthly billable after later samples = %v, want 1103000 (500000+600000 deltas + 3000 snapshot)", got)
	}
	measuredIn, measuredOut, measuredBillable = correctionBytes(t, ctx, store)
	if measuredIn != 500_000 || measuredOut != 600_000 || measuredBillable != 1_100_000 {
		t.Fatalf("stored measured = %d/%d/%d, want 500000/600000/1100000", measuredIn, measuredOut, measuredBillable)
	}
}

func TestMonthlyTrafficCorrectionRejectsOutOfRange(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyInCorrectionBytes: adminOptionalInt64{Set: true, Valid: true, Value: -1},
	}); err != errInvalidAdminNodeUpdate {
		t.Fatalf("negative correction err = %v, want errInvalidAdminNodeUpdate", err)
	}
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyOutCorrectionBytes: adminOptionalInt64{Set: true, Valid: true, Value: maxMonthlyTrafficCorrectionBytes + 1},
	}); err != errInvalidAdminNodeUpdate {
		t.Fatalf("oversize correction err = %v, want errInvalidAdminNodeUpdate", err)
	}

	// Rejected updates must not leave a correction row behind.
	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("admin nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].MonthlyInCorrectionBytes != nil || nodes[0].MonthlyOutCorrectionBytes != nil {
		t.Fatalf("admin corrections = %v/%v, want nil/nil after rejected updates",
			nodes[0].MonthlyInCorrectionBytes, nodes[0].MonthlyOutCorrectionBytes)
	}
}

func TestMonthlyTrafficCorrectionClearsWithNull(t *testing.T) {
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
	postCorrectionTestState(t, ctx, store, now, 1_000_000, 2_000_000)
	postCorrectionTestState(t, ctx, store, now.Add(time.Second), 1_400_000, 2_600_000)

	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyInCorrectionBytes: adminOptionalInt64{Set: true, Valid: true, Value: 5_000},
	}); err != nil {
		t.Fatalf("set correction: %v", err)
	}
	// Absent out correction stays zero; explicit null clears the in direction.
	// Clearing re-snapshots at zero, so previously measured usage is dropped.
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyInCorrectionBytes: adminOptionalInt64{Set: true, Valid: false},
	}); err != nil {
		t.Fatalf("clear correction: %v", err)
	}

	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got := *summary.Nodes[0].MonthlyBillableBytes; got != 0 {
		t.Fatalf("monthly billable after clear = %v, want 0 (re-snapshotted)", got)
	}
	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if nodes[0].MonthlyInCorrectionBytes != nil || nodes[0].MonthlyOutCorrectionBytes != nil {
		t.Fatalf("admin corrections = %v/%v, want nil/nil after clear",
			nodes[0].MonthlyInCorrectionBytes, nodes[0].MonthlyOutCorrectionBytes)
	}
}

func TestMonthlyTrafficCorrectionBillsLumpUnderMaxMode(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	modeMax := "max"
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{BillingMode: &modeMax}); err != nil {
		t.Fatalf("set billing mode: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	postCorrectionTestState(t, ctx, store, now, 1_000_000, 2_000_000)
	postCorrectionTestState(t, ctx, store, now.Add(time.Second), 1_100_000, 2_600_000)

	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{
		MonthlyInCorrectionBytes:  adminOptionalInt64{Set: true, Valid: true, Value: 100},
		MonthlyOutCorrectionBytes: adminOptionalInt64{Set: true, Valid: true, Value: 900},
	}); err != nil {
		t.Fatalf("update correction: %v", err)
	}

	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// Snapshot semantics discard the measured 600000: only the correction lump
	// bills, max(100, 900) = 900.
	if got := *summary.Nodes[0].MonthlyBillableBytes; got != 900 {
		t.Fatalf("monthly billable under max = %v, want 900", got)
	}
}
