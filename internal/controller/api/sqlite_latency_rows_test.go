package api

import (
	"context"
	"testing"
	"time"
)

func TestRawLatencyRowsPreserveDimensionAndEmptyShapes(t *testing.T) {
	t.Parallel()
	store := newLatencyGridTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, display_order, disabled, created_at, updated_at)
		VALUES ('node-a', 'Node A', 'hash-a', 'online', 1, 0, ?, ?);
		INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, display_order, created_at, updated_at)
		VALUES ('target-a', 'Target A', 'tcp', '127.0.0.1', 443, 1, 1000, 30, 1, ?, ?);
		INSERT INTO node_probe_targets (node_id, target_id, enabled) VALUES ('node-a', 'target-a', 1);
		INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent, median_ms, avg_ms)
		VALUES ('node-a', 'target-a', ?, 'tcp', 1, 1, 2.5, NULL, 12.5);
	`, now.Unix(), now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}

	window := latencyWindow{Name: "1h", Samples: 2, Step: 7 * time.Minute}
	nodePoints, err := store.latencyPoints(ctx, "node-a", window)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodePoints) != 1 || nodePoints[0].TS != now.Format(time.RFC3339) || nodePoints[0].TargetID != "target-a" || nodePoints[0].TargetName != "Target A" || nodePoints[0].MedianMS != nil || nodePoints[0].AvgMS == nil || *nodePoints[0].AvgMS != 12.5 || nodePoints[0].LossPercent != 2.5 {
		t.Fatalf("node points = %+v", nodePoints)
	}

	servicePoints, err := store.serviceLatencyPoints(ctx, "target-a", window)
	if err != nil {
		t.Fatal(err)
	}
	if len(servicePoints) != 1 || servicePoints[0].TS != now.Format(time.RFC3339) || servicePoints[0].NodeID != "node-a" || servicePoints[0].NodeName != "Node A" || servicePoints[0].MedianMS != nil || servicePoints[0].AvgMS == nil || *servicePoints[0].AvgMS != 12.5 || servicePoints[0].LossPercent != 2.5 {
		t.Fatalf("service points = %+v", servicePoints)
	}

	emptyNodePoints, err := store.latencyPoints(ctx, "missing-node", window)
	if err != nil || emptyNodePoints != nil {
		t.Fatalf("empty node points = (%v, %v), want (nil, nil)", emptyNodePoints, err)
	}
	emptyServicePoints, err := store.serviceLatencyPoints(ctx, "missing-target", window)
	if err != nil || emptyServicePoints == nil || len(emptyServicePoints) != 0 {
		t.Fatalf("empty service points = (%v, %v), want (non-nil empty, nil)", emptyServicePoints, err)
	}
}

func TestLatencyPointCountFollowsConfiguredProbeInterval(t *testing.T) {
	t.Parallel()
	store := newLatencyGridTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, display_order, disabled, created_at, updated_at)
		VALUES ('node-a', 'Node A', 'hash-a', 'online', 1, 0, ?, ?);
		INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, display_order, created_at, updated_at)
		VALUES ('target-a', 'Target A', 'tcp', '127.0.0.1', 443, 1, 1000, 120, 1, ?, ?);
		INSERT INTO node_probe_targets (node_id, target_id, enabled) VALUES ('node-a', 'target-a', 1);
	`, now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}

	firstSample := now.Add(-59 * time.Minute)
	for index := 0; index < 60; index++ {
		value := float64(index)
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent, median_ms, avg_ms)
			VALUES ('node-a', 'target-a', ?, 'tcp', 1, 1, 0, ?, ?);
		`, firstSample.Add(time.Duration(index)*time.Minute).Unix(), value, value); err != nil {
			t.Fatal(err)
		}
	}

	window, ok := resolveLatencyWindow("1h")
	if !ok {
		t.Fatal("1h latency window not found")
	}
	points120s, err := store.latencyPoints(ctx, "node-a", window)
	if err != nil {
		t.Fatal(err)
	}
	if len(points120s) != 30 {
		t.Fatalf("120-second interval returned %d points, want 30", len(points120s))
	}
	if got := timeBetweenPoints(t, points120s[0].TS, points120s[1].TS); got != 2*time.Minute {
		t.Fatalf("120-second interval point step = %s, want 2m", got)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE probe_targets SET interval_sec = 60 WHERE id = 'target-a'`); err != nil {
		t.Fatal(err)
	}
	points60s, err := store.latencyPoints(ctx, "node-a", window)
	if err != nil {
		t.Fatal(err)
	}
	if len(points60s) != 60 {
		t.Fatalf("60-second interval returned %d points, want 60", len(points60s))
	}
	if got := timeBetweenPoints(t, points60s[0].TS, points60s[1].TS); got != time.Minute {
		t.Fatalf("60-second interval point step = %s, want 1m", got)
	}
}

func timeBetweenPoints(t *testing.T, first, second string) time.Duration {
	t.Helper()
	firstTime, err := time.Parse(time.RFC3339, first)
	if err != nil {
		t.Fatal(err)
	}
	secondTime, err := time.Parse(time.RFC3339, second)
	if err != nil {
		t.Fatal(err)
	}
	return secondTime.Sub(firstTime)
}
