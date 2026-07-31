package api

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestTieredHistoryPreservesWeightedLatencyAndStateValues(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-48 * time.Hour).Truncate(time.Minute)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at)
		VALUES ('node-rollup', 'Rollup Node', 'hash', 'online', ?, ?);
		INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, created_at, updated_at)
		VALUES ('target-rollup', 'Rollup Target', 'tcp', '127.0.0.1', 443, 1, 1000, 30, ?, ?);
		INSERT INTO node_probe_targets (node_id, target_id, enabled) VALUES ('node-rollup', 'target-rollup', 1);
	`, now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed rollup configuration: %v", err)
	}

	for index, sample := range []struct {
		offset int64
		median float64
		avg    float64
		loss   float64
		cpu    float64
		memory any
	}{
		{offset: 5, median: 10, avg: 12, loss: 0, cpu: 10, memory: nil},
		{offset: 20, median: 30, avg: 32, loss: 10, cpu: 30, memory: 100.0},
	} {
		ts := old.Unix() + sample.offset
		result, err := store.db.ExecContext(ctx, `
			INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent, median_ms, avg_ms)
			VALUES ('node-rollup', 'target-rollup', ?, 'tcp', 1, 1, ?, ?, ?)
		`, ts, sample.loss, sample.median, sample.avg)
		if err != nil {
			t.Fatalf("insert probe round %d: %v", index, err)
		}
		roundID, _ := result.LastInsertId()
		if _, err := store.db.ExecContext(ctx, `INSERT INTO probe_samples (round_id, seq, success, latency_ms) VALUES (?, 1, 1, ?)`, roundID, sample.median); err != nil {
			t.Fatalf("insert probe sample %d: %v", index, err)
		}
		if _, err := store.db.ExecContext(ctx, `INSERT INTO state_samples (node_id, ts, cpu_percent, memory_used_bytes) VALUES ('node-rollup', ?, ?, ?)`, ts, sample.cpu, sample.memory); err != nil {
			t.Fatalf("insert state sample %d: %v", index, err)
		}
	}

	cutoff := now.Add(-rawHistoryRetention)
	if err := store.PruneRawHistory(ctx, cutoff); err != nil {
		t.Fatalf("compact history: %v", err)
	}
	if err := store.PruneRawHistory(ctx, cutoff); err != nil {
		t.Fatalf("repeat compact history: %v", err)
	}

	for table, want := range map[string]int{
		"probe_rounds":            0,
		"probe_samples":           0,
		"state_samples":           0,
		"latency_history_rollups": 1,
		"state_history_rollups":   1,
	} {
		var got int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}

	latencyWindow, _ := resolveLatencyGridWindow("7d")
	latency, err := store.latencyGridPoints(ctx, "node-rollup", latencyWindow)
	if err != nil {
		t.Fatalf("read tiered latency: %v", err)
	}
	latencyBucket := (old.Unix() / int64(latencyWindow.Step/time.Second)) * int64(latencyWindow.Step/time.Second)
	var matchedLatency *LatencyPoint
	for index := range latency {
		if latency[index].TargetID == "target-rollup" && latency[index].TS == time.Unix(latencyBucket, 0).UTC().Format(time.RFC3339) {
			matchedLatency = &latency[index]
			break
		}
	}
	if matchedLatency == nil || matchedLatency.MedianMS == nil || matchedLatency.AvgMS == nil {
		t.Fatalf("missing compacted latency bucket at %d", latencyBucket)
	}
	if math.Abs(*matchedLatency.MedianMS-20) > 0.001 || math.Abs(*matchedLatency.AvgMS-22) > 0.001 || math.Abs(matchedLatency.LossPercent-5) > 0.001 {
		t.Fatalf("weighted latency = median %.3f avg %.3f loss %.3f, want 20/22/5", *matchedLatency.MedianMS, *matchedLatency.AvgMS, matchedLatency.LossPercent)
	}

	stateWindow, _ := resolveStateWindow("7d")
	state, err := store.statePoints(ctx, "node-rollup", stateWindow)
	if err != nil {
		t.Fatalf("read tiered state: %v", err)
	}
	stateBucket := (old.Unix() / int64(stateWindow.Step/time.Second)) * int64(stateWindow.Step/time.Second)
	var matchedState *StatePoint
	for index := range state {
		if state[index].TS == time.Unix(stateBucket, 0).UTC().Format(time.RFC3339) {
			matchedState = &state[index]
			break
		}
	}
	if matchedState == nil || matchedState.CPUPercent == nil || matchedState.MemoryUsedBytes == nil {
		t.Fatalf("missing compacted state bucket at %d", stateBucket)
	}
	if math.Abs(*matchedState.CPUPercent-20) > 0.001 || math.Abs(*matchedState.MemoryUsedBytes-100) > 0.001 {
		t.Fatalf("weighted state = cpu %.3f memory %.3f, want 20/100", *matchedState.CPUPercent, *matchedState.MemoryUsedBytes)
	}
}
