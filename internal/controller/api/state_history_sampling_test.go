package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shui1iao/zeno/internal/controller/history"
)

func TestStateHistoryUsesLatestRawSamplePerDisplayBucket(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at)
		VALUES ('sampled-node', 'Sampled Node', 'hash', 'online', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	window := latencyWindow{Name: "7d", Samples: 1, Step: 30 * time.Minute}
	start, _, _ := latencyGridBounds(window)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO state_samples (node_id, ts, cpu_percent, memory_used_bytes)
		VALUES ('sampled-node', ?, 10, 100), ('sampled-node', ?, 30, 300)
	`, start.Unix()+5, start.Unix()+10); err != nil {
		t.Fatalf("seed raw state: %v", err)
	}
	points, err := store.statePoints(ctx, "sampled-node", window)
	if err != nil {
		t.Fatalf("read sampled state history: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("state points = %d, want 1", len(points))
	}
	if points[0].CPUPercent == nil || *points[0].CPUPercent != 30 || points[0].MemoryUsedBytes == nil || *points[0].MemoryUsedBytes != 300 {
		t.Fatalf("raw bucket = %+v, want latest raw sample", points[0])
	}
}

func TestHistoricalStateSamplingExcludesRealtimeWindow(t *testing.T) {
	for rangeName, want := range map[string]bool{"1h": false, "1d": true, "7d": true, "30d": true} {
		window, ok := resolveStateWindow(rangeName)
		if !ok {
			t.Fatalf("resolve state window %q", rangeName)
		}
		if got := useHistoricalStateSampling(window); got != want {
			t.Fatalf("historical sampling for %s = %t, want %t", rangeName, got, want)
		}
	}
}

func TestStateHistoryFutureSampleDoesNotShiftDisplayWindow(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at)
		VALUES ('future-node', 'Future Node', 'hash', 'online', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	window, _ := resolveStateWindow("1d")
	_, end, _ := latencyGridBounds(window)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO state_samples (node_id, ts, cpu_percent)
		VALUES ('future-node', ?, 10), ('future-node', ?, 99)
	`, end.Unix()+1, end.Add(7*24*time.Hour).Unix()); err != nil {
		t.Fatalf("seed state samples: %v", err)
	}

	points, err := store.statePoints(ctx, "future-node", window)
	if err != nil {
		t.Fatalf("read state history: %v", err)
	}
	if len(points) != 1 || points[0].CPUPercent == nil || *points[0].CPUPercent != 10 {
		t.Fatalf("state points = %+v, want only the current display-window sample", points)
	}
}

func TestStateHistorySupportsEveryDisplayWindow(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at)
		VALUES ('window-node', 'Window Node', 'hash', 'online', ?, ?);
		INSERT INTO state_samples (node_id, ts, cpu_percent)
		VALUES ('window-node', ?, 10)
	`, now, now, now); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	for _, rangeName := range []string{"1h", "1d", "7d", "30d"} {
		t.Run(rangeName, func(t *testing.T) {
			window, ok := resolveStateWindow(rangeName)
			if !ok {
				t.Fatalf("resolve state window %q", rangeName)
			}
			points, err := store.statePoints(ctx, "window-node", window)
			if err != nil {
				t.Fatalf("read %s state history: %v", rangeName, err)
			}
			if len(points) != 1 {
				t.Fatalf("%s state points = %d, want 1", rangeName, len(points))
			}
		})
	}
}

func TestStateHistoryQueryPlanSeeksCompositeIndexes(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	window, _ := resolveStateWindow("30d")
	start, _, stepSeconds := latencyGridBounds(window)
	plan := explainSQLiteQueryPlan(t, store, history.StateLatestGridQuery,
		start.Unix(), stepSeconds, window.Samples,
		"node-id", stepSeconds,
		"node-id", stepSeconds,
	)
	for _, want := range []string{"idx_state_samples_node_ts", "sqlite_autoindex_state_history_rollups_1"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("state history plan missing %q:\n%s", want, plan)
		}
	}
	for _, forbidden := range []string{"SCAN state_samples", "SCAN state_history_rollups"} {
		if strings.Contains(plan, forbidden) {
			t.Fatalf("state history plan contains %q:\n%s", forbidden, plan)
		}
	}
}
