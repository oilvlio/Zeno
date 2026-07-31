package api

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The reference implementations below are verbatim copies of latencyGridPoints
// and serviceLatencyGridPoints as they existed before the shared
// latencyGridPointsFor rewrite (v1.9.1, df1cde9). They exist purely so the
// merged implementation can be differentially tested against the behaviour it
// replaced: SQL, bucket bounds, ordering, null handling and error paths must all
// stay identical.

func refLatencyGridPoints(ctx context.Context, s *SQLiteStore, nodeID string, window latencyWindow) ([]LatencyPoint, error) {
	gridWindow, ok := resolveLatencyGridWindow(window.Name)
	if !ok {
		return nil, nil
	}
	targets, err := refEnabledLatencyTargetsForNode(ctx, s, nodeID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return []LatencyPoint{}, nil
	}

	start, end, stepSeconds := latencyGridBounds(gridWindow)
	buckets := make(map[string]map[int64]*latencyGridBucket, len(targets))
	rows, err := s.db.QueryContext(ctx, `
		WITH measurements AS (
			SELECT ts, node_id, target_id,
			       COALESCE(median_ms, 0) AS median_sum, CASE WHEN median_ms IS NULL THEN 0 ELSE 1 END AS median_count,
			       COALESCE(avg_ms, 0) AS avg_sum, CASE WHEN avg_ms IS NULL THEN 0 ELSE 1 END AS avg_count,
			       loss_percent AS loss_sum, 1 AS loss_count
			FROM probe_rounds WHERE node_id = ? AND ts >= ? AND ts < ?
			UNION ALL
			SELECT bucket_start AS ts, node_id, target_id,
			       median_sum, median_count, avg_sum, avg_count, loss_sum, loss_count
			FROM latency_history_rollups WHERE node_id = ? AND bucket_start >= ? AND bucket_start < ?
		)
		SELECT (measurements.ts / ?) * ? AS bucket_ts, measurements.target_id,
		       SUM(median_sum) / NULLIF(SUM(median_count), 0),
		       SUM(avg_sum) / NULLIF(SUM(avg_count), 0),
		       SUM(loss_sum) / NULLIF(SUM(loss_count), 0)
		FROM measurements
		JOIN probe_targets pt ON pt.id = measurements.target_id
		LEFT JOIN node_probe_targets npt ON npt.node_id = measurements.node_id AND npt.target_id = measurements.target_id
		WHERE measurements.node_id = ?
		  AND COALESCE(npt.enabled, 0) = 1
		GROUP BY bucket_ts, measurements.target_id
	`, nodeID, start.Unix(), end.Add(gridWindow.Step).Unix(), nodeID, start.Unix(), end.Add(gridWindow.Step).Unix(), stepSeconds, stepSeconds, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bucketTS int64
		var targetID string
		var median, avg, loss sql.NullFloat64
		if err := rows.Scan(&bucketTS, &targetID, &median, &avg, &loss); err != nil {
			return nil, err
		}
		if bucketTS < start.Unix() || bucketTS > end.Unix() {
			continue
		}
		if buckets[targetID] == nil {
			buckets[targetID] = map[int64]*latencyGridBucket{}
		}
		if buckets[targetID][bucketTS] == nil {
			buckets[targetID][bucketTS] = &latencyGridBucket{}
		}
		buckets[targetID][bucketTS].add(median, avg, nullFloatOr(loss, 0))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points := make([]LatencyPoint, 0, gridWindow.Samples*len(targets))
	for index := 0; index < gridWindow.Samples; index++ {
		bucketTS := start.Add(time.Duration(index) * gridWindow.Step).Unix()
		ts := time.Unix(bucketTS, 0).UTC().Format(time.RFC3339)
		for _, target := range targets {
			bucket := buckets[target.ID][bucketTS]
			point := LatencyPoint{TS: ts, TargetID: target.ID, TargetName: target.Name, LossPercent: 0}
			if bucket != nil {
				point.MedianMS = bucket.medianPtr()
				point.AvgMS = bucket.avgPtr()
				point.LossPercent = bucket.lossPercent()
			}
			points = append(points, point)
		}
	}
	return points, nil
}

func refServiceLatencyGridPoints(ctx context.Context, s *SQLiteStore, targetID string, window latencyWindow) ([]ServiceLatencyPoint, error) {
	gridWindow, ok := resolveLatencyGridWindow(window.Name)
	if !ok {
		return nil, nil
	}
	nodes, err := refEnabledLatencyNodesForTarget(ctx, s, targetID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return []ServiceLatencyPoint{}, nil
	}

	start, end, stepSeconds := latencyGridBounds(gridWindow)
	buckets := make(map[string]map[int64]*latencyGridBucket, len(nodes))
	rows, err := s.db.QueryContext(ctx, `
		WITH measurements AS (
			SELECT ts, node_id, target_id,
			       COALESCE(median_ms, 0) AS median_sum, CASE WHEN median_ms IS NULL THEN 0 ELSE 1 END AS median_count,
			       COALESCE(avg_ms, 0) AS avg_sum, CASE WHEN avg_ms IS NULL THEN 0 ELSE 1 END AS avg_count,
			       loss_percent AS loss_sum, 1 AS loss_count
			FROM probe_rounds WHERE target_id = ? AND ts >= ? AND ts < ?
			UNION ALL
			SELECT bucket_start AS ts, node_id, target_id,
			       median_sum, median_count, avg_sum, avg_count, loss_sum, loss_count
			FROM latency_history_rollups WHERE target_id = ? AND bucket_start >= ? AND bucket_start < ?
		)
		SELECT (measurements.ts / ?) * ? AS bucket_ts, measurements.node_id,
		       SUM(median_sum) / NULLIF(SUM(median_count), 0),
		       SUM(avg_sum) / NULLIF(SUM(avg_count), 0),
		       SUM(loss_sum) / NULLIF(SUM(loss_count), 0)
		FROM measurements
		JOIN nodes n ON n.id = measurements.node_id
		JOIN probe_targets pt ON pt.id = measurements.target_id
		LEFT JOIN node_probe_targets npt ON npt.node_id = measurements.node_id AND npt.target_id = measurements.target_id
		WHERE measurements.target_id = ?
		  AND n.disabled = 0
		  AND COALESCE(npt.enabled, 0) = 1
		GROUP BY bucket_ts, measurements.node_id
	`, targetID, start.Unix(), end.Add(gridWindow.Step).Unix(), targetID, start.Unix(), end.Add(gridWindow.Step).Unix(), stepSeconds, stepSeconds, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bucketTS int64
		var nodeID string
		var median, avg, loss sql.NullFloat64
		if err := rows.Scan(&bucketTS, &nodeID, &median, &avg, &loss); err != nil {
			return nil, err
		}
		if bucketTS < start.Unix() || bucketTS > end.Unix() {
			continue
		}
		if buckets[nodeID] == nil {
			buckets[nodeID] = map[int64]*latencyGridBucket{}
		}
		if buckets[nodeID][bucketTS] == nil {
			buckets[nodeID][bucketTS] = &latencyGridBucket{}
		}
		buckets[nodeID][bucketTS].add(median, avg, nullFloatOr(loss, 0))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points := make([]ServiceLatencyPoint, 0, gridWindow.Samples*len(nodes))
	for index := 0; index < gridWindow.Samples; index++ {
		bucketTS := start.Add(time.Duration(index) * gridWindow.Step).Unix()
		ts := time.Unix(bucketTS, 0).UTC().Format(time.RFC3339)
		for _, node := range nodes {
			bucket := buckets[node.ID][bucketTS]
			point := ServiceLatencyPoint{TS: ts, NodeID: node.ID, NodeName: node.Name, LossPercent: 0}
			if bucket != nil {
				point.MedianMS = bucket.medianPtr()
				point.AvgMS = bucket.avgPtr()
				point.LossPercent = bucket.lossPercent()
			}
			points = append(points, point)
		}
	}
	return points, nil
}

func refEnabledLatencyTargetsForNode(ctx context.Context, s *SQLiteStore, nodeID string) ([]latencyGridTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pt.id, pt.name
		FROM probe_targets pt
		LEFT JOIN node_probe_targets npt ON npt.target_id = pt.id AND npt.node_id = ?
		WHERE COALESCE(npt.enabled, 0) = 1
		ORDER BY pt.display_order ASC, pt.name ASC, pt.id ASC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []latencyGridTarget
	for rows.Next() {
		var target latencyGridTarget
		if err := rows.Scan(&target.ID, &target.Name); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func refEnabledLatencyNodesForTarget(ctx context.Context, s *SQLiteStore, targetID string) ([]latencyGridTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.display_name
		FROM nodes n
		LEFT JOIN node_probe_targets npt ON npt.node_id = n.id AND npt.target_id = ?
		WHERE n.disabled = 0 AND COALESCE(npt.enabled, 0) = 1
		ORDER BY n.display_order ASC, n.display_name ASC, n.id ASC
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []latencyGridTarget
	for rows.Next() {
		var node latencyGridTarget
		if err := rows.Scan(&node.ID, &node.Name); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func newLatencyGridTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// seedLatencyGridFixture creates two nodes (one disabled), three targets (one
// disabled for the primary node) and spreads probe rounds plus rollup rows
// across the requested grid window, including rows exactly on the first and last
// bucket boundaries and rows just outside the window on both sides.
func seedLatencyGridFixture(ctx context.Context, t *testing.T, store *SQLiteStore, window latencyWindow) {
	t.Helper()
	now := time.Now().UTC().Unix()
	start, end, step := latencyGridBounds(window)

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, display_order, disabled, created_at, updated_at)
		VALUES ('node-a', 'Node A', 'hash-a', 'online', 1, 0, ?, ?),
		       ('node-b', 'Node B', 'hash-b', 'online', 2, 0, ?, ?),
		       ('node-off', 'Node Disabled', 'hash-c', 'online', 3, 1, ?, ?);
		INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, display_order, created_at, updated_at)
		VALUES ('t-1', 'Target One', 'tcp', '127.0.0.1', 443, 1, 1000, 30, 1, ?, ?),
		       ('t-2', 'Target Two', 'tcp', '127.0.0.2', 443, 1, 1000, 30, 2, ?, ?),
		       ('t-off', 'Target Off', 'tcp', '127.0.0.3', 443, 1, 1000, 30, 3, ?, ?);
		INSERT INTO node_probe_targets (node_id, target_id, enabled) VALUES
		       ('node-a', 't-1', 1), ('node-a', 't-2', 1), ('node-a', 't-off', 0),
		       ('node-b', 't-1', 1), ('node-b', 't-2', 0),
		       ('node-off', 't-1', 1);
	`, now, now, now, now, now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed nodes and targets: %v", err)
	}

	insertRound := func(nodeID, targetID string, ts int64, median, avg any, loss float64) {
		t.Helper()
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent, median_ms, avg_ms)
			VALUES (?, ?, ?, 'tcp', 1, 1, ?, ?, ?)
		`, nodeID, targetID, ts, loss, median, avg); err != nil {
			t.Fatalf("insert probe round %s/%s@%d: %v", nodeID, targetID, ts, err)
		}
	}
	insertRollup := func(nodeID, targetID string, bucket int64, medianSum float64, medianCount int, avgSum float64, avgCount int, lossSum float64, lossCount int) {
		t.Helper()
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO latency_history_rollups (node_id, target_id, bucket_start, median_sum, median_count, avg_sum, avg_count, loss_sum, loss_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, nodeID, targetID, bucket, medianSum, medianCount, avgSum, avgCount, lossSum, lossCount); err != nil {
			t.Fatalf("insert rollup %s/%s@%d: %v", nodeID, targetID, bucket, err)
		}
	}

	// First bucket boundary, inclusive.
	insertRound("node-a", "t-1", start.Unix(), 10.0, 12.0, 0)
	insertRound("node-a", "t-1", start.Unix()+1, 20.0, 24.0, 10)
	// NULL median/avg must fall through to the NULLIF/COALESCE handling.
	insertRound("node-a", "t-2", start.Unix(), nil, nil, 100)
	// avg NULL but median present exercises avgPtr's median fallback.
	insertRound("node-a", "t-2", start.Unix()+2, 30.0, nil, 5)
	// Mid-window bucket.
	mid := start.Add(time.Duration(window.Samples/2) * window.Step).Unix()
	insertRound("node-a", "t-1", mid, 40.0, 44.0, 1)
	insertRound("node-b", "t-1", mid, 50.0, 55.0, 2)
	// Last bucket boundary, inclusive.
	insertRound("node-a", "t-1", end.Unix(), 60.0, 66.0, 3)
	insertRound("node-b", "t-1", end.Unix()+step-1, 70.0, 77.0, 4)
	// Outside the window on both sides: must be excluded.
	insertRound("node-a", "t-1", start.Unix()-1, 999.0, 999.0, 99)
	insertRound("node-a", "t-1", end.Unix()+step, 888.0, 888.0, 88)
	// Rows on disabled node/target links must be filtered out.
	insertRound("node-a", "t-off", mid, 777.0, 777.0, 77)
	insertRound("node-off", "t-1", mid, 666.0, 666.0, 66)
	insertRound("node-b", "t-2", mid, 555.0, 555.0, 55)
	// Rollups share the same bucket as raw rounds so the UNION ALL aggregation is covered.
	insertRollup("node-a", "t-1", mid, 100.0, 2, 120.0, 2, 8.0, 2)
	insertRollup("node-b", "t-1", start.Unix(), 90.0, 1, 99.0, 1, 6.0, 1)
	insertRollup("node-a", "t-1", start.Unix()-int64(step), 999.0, 1, 999.0, 1, 99.0, 1)
}

func TestLatencyGridPointsMatchesPreMergeImplementation(t *testing.T) {
	ctx := context.Background()
	for _, rangeName := range []string{"1h", "7d"} {
		t.Run("node/"+rangeName, func(t *testing.T) {
			store := newLatencyGridTestStore(t)
			window, ok := resolveLatencyGridWindow(rangeName)
			if !ok {
				t.Fatalf("resolve grid window %q", rangeName)
			}
			seedLatencyGridFixture(ctx, t, store, window)

			want, err := refLatencyGridPoints(ctx, store, "node-a", window)
			if err != nil {
				t.Fatalf("reference latency grid: %v", err)
			}
			got, err := store.latencyGridPoints(ctx, "node-a", window)
			if err != nil {
				t.Fatalf("merged latency grid: %v", err)
			}
			assertLatencyPointsEqual(t, got, want)
			if len(got) != window.Samples*2 {
				t.Fatalf("point count = %d, want %d (samples × enabled targets)", len(got), window.Samples*2)
			}
		})

		t.Run("service/"+rangeName, func(t *testing.T) {
			store := newLatencyGridTestStore(t)
			window, ok := resolveLatencyGridWindow(rangeName)
			if !ok {
				t.Fatalf("resolve grid window %q", rangeName)
			}
			seedLatencyGridFixture(ctx, t, store, window)

			want, err := refServiceLatencyGridPoints(ctx, store, "t-1", window)
			if err != nil {
				t.Fatalf("reference service latency grid: %v", err)
			}
			got, err := store.serviceLatencyGridPoints(ctx, "t-1", window)
			if err != nil {
				t.Fatalf("merged service latency grid: %v", err)
			}
			assertServiceLatencyPointsEqual(t, got, want)
			if len(got) != window.Samples*2 {
				t.Fatalf("point count = %d, want %d (samples × enabled nodes)", len(got), window.Samples*2)
			}
		})
	}
}

func TestLatencyGridPointsWindowBoundaryAndValues(t *testing.T) {
	ctx := context.Background()
	store := newLatencyGridTestStore(t)
	window, _ := resolveLatencyGridWindow("1h")
	seedLatencyGridFixture(ctx, t, store, window)
	start, end, step := latencyGridBounds(window)

	points, err := store.latencyGridPoints(ctx, "node-a", window)
	if err != nil {
		t.Fatalf("latency grid: %v", err)
	}

	byKey := map[string]LatencyPoint{}
	for _, point := range points {
		byKey[point.TargetID+"@"+point.TS] = point
		if point.TargetID == "t-off" {
			t.Fatalf("disabled target leaked into grid: %+v", point)
		}
	}
	tsOf := func(unix int64) string { return time.Unix(unix, 0).UTC().Format(time.RFC3339) }

	// First bucket: raw 10/20 median and 12/24 avg, loss 0/10.
	first := byKey["t-1@"+tsOf(start.Unix())]
	assertFloatPtr(t, "first median", first.MedianMS, 15)
	assertFloatPtr(t, "first avg", first.AvgMS, 18)
	assertFloat(t, "first loss", first.LossPercent, 5)

	// NULL median and avg collapse to nil while loss still averages.
	nullPoint := byKey["t-2@"+tsOf(start.Unix())]
	if nullPoint.MedianMS == nil {
		t.Fatalf("t-2 first bucket median should come from the non-null round")
	}
	assertFloatPtr(t, "t-2 median", nullPoint.MedianMS, 30)
	assertFloatPtr(t, "t-2 avg falls back to median", nullPoint.AvgMS, 30)
	assertFloat(t, "t-2 loss", nullPoint.LossPercent, 52.5)

	// Mid bucket mixes raw rounds with a rollup row.
	mid := start.Add(time.Duration(window.Samples/2) * window.Step).Unix()
	midPoint := byKey["t-1@"+tsOf(mid)]
	assertFloatPtr(t, "mid median", midPoint.MedianMS, 140.0/3.0)
	assertFloatPtr(t, "mid avg", midPoint.AvgMS, 164.0/3.0)
	assertFloat(t, "mid loss", midPoint.LossPercent, 3)

	// Last bucket boundary is inclusive and absorbs the sub-step row.
	last := byKey["t-1@"+tsOf(end.Unix())]
	assertFloatPtr(t, "last median", last.MedianMS, 60)
	assertFloatPtr(t, "last avg", last.AvgMS, 66)
	assertFloat(t, "last loss", last.LossPercent, 3)

	// Rows before start and at end+step are outside the window.
	for _, unix := range []int64{start.Unix() - int64(step), end.Unix() + int64(step)} {
		if point, ok := byKey["t-1@"+tsOf(unix)]; ok {
			t.Fatalf("out-of-window bucket %d present: %+v", unix, point)
		}
	}
	for _, point := range points {
		if point.MedianMS != nil && (*point.MedianMS > 900 || *point.MedianMS == 777) {
			t.Fatalf("excluded row leaked into grid: %+v", point)
		}
	}
}

func TestServiceLatencyGridPointsHonoursNodeDimension(t *testing.T) {
	ctx := context.Background()
	store := newLatencyGridTestStore(t)
	window, _ := resolveLatencyGridWindow("1h")
	seedLatencyGridFixture(ctx, t, store, window)
	start, _, _ := latencyGridBounds(window)

	points, err := store.serviceLatencyGridPoints(ctx, "t-1", window)
	if err != nil {
		t.Fatalf("service latency grid: %v", err)
	}

	seen := map[string]bool{}
	for _, point := range points {
		seen[point.NodeID] = true
		if point.NodeID == "node-off" {
			t.Fatalf("disabled node leaked into service grid: %+v", point)
		}
	}
	if !seen["node-a"] || !seen["node-b"] {
		t.Fatalf("expected node-a and node-b series, got %v", seen)
	}
	// Series order follows display_order, so node-a precedes node-b in every bucket.
	if points[0].NodeID != "node-a" || points[1].NodeID != "node-b" {
		t.Fatalf("series order = %s,%s, want node-a,node-b", points[0].NodeID, points[1].NodeID)
	}

	tsOf := func(unix int64) string { return time.Unix(unix, 0).UTC().Format(time.RFC3339) }
	var firstNodeB *ServiceLatencyPoint
	for index := range points {
		if points[index].NodeID == "node-b" && points[index].TS == tsOf(start.Unix()) {
			firstNodeB = &points[index]
			break
		}
	}
	if firstNodeB == nil {
		t.Fatal("missing node-b first bucket")
	}
	// node-b's first bucket comes only from the rollup row (90/1, 99/1, 6/1).
	assertFloatPtr(t, "node-b rollup median", firstNodeB.MedianMS, 90)
	assertFloatPtr(t, "node-b rollup avg", firstNodeB.AvgMS, 99)
	assertFloat(t, "node-b rollup loss", firstNodeB.LossPercent, 6)
}

func TestLatencyGridPointsEmptyResults(t *testing.T) {
	ctx := context.Background()
	store := newLatencyGridTestStore(t)
	window, _ := resolveLatencyGridWindow("1h")

	// No configured series at all: both dimensions return non-nil empty slices.
	points, err := store.latencyGridPoints(ctx, "missing-node", window)
	if err != nil {
		t.Fatalf("latency grid on empty db: %v", err)
	}
	if points == nil || len(points) != 0 {
		t.Fatalf("latency grid = %#v, want empty non-nil slice", points)
	}
	servicePoints, err := store.serviceLatencyGridPoints(ctx, "missing-target", window)
	if err != nil {
		t.Fatalf("service latency grid on empty db: %v", err)
	}
	if servicePoints == nil || len(servicePoints) != 0 {
		t.Fatalf("service latency grid = %#v, want empty non-nil slice", servicePoints)
	}

	// Series configured but no measurements: dense grid of empty buckets.
	seedLatencyGridFixture(ctx, t, store, window)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM probe_rounds; DELETE FROM latency_history_rollups;`); err != nil {
		t.Fatalf("clear measurements: %v", err)
	}
	points, err = store.latencyGridPoints(ctx, "node-a", window)
	if err != nil {
		t.Fatalf("latency grid without measurements: %v", err)
	}
	if len(points) != window.Samples*2 {
		t.Fatalf("point count = %d, want %d", len(points), window.Samples*2)
	}
	for _, point := range points {
		if point.MedianMS != nil || point.AvgMS != nil || point.LossPercent != 0 {
			t.Fatalf("empty bucket carries data: %+v", point)
		}
	}

	// Unknown range names short-circuit to (nil, nil) on both dimensions.
	if points, err := store.latencyGridPoints(ctx, "node-a", latencyWindow{Name: "nope", Samples: 5, Step: time.Minute}); err != nil || points != nil {
		t.Fatalf("unknown window = (%#v, %v), want (nil, nil)", points, err)
	}
	if points, err := store.serviceLatencyGridPoints(ctx, "t-1", latencyWindow{Name: "nope", Samples: 5, Step: time.Minute}); err != nil || points != nil {
		t.Fatalf("unknown window = (%#v, %v), want (nil, nil)", points, err)
	}
}

func TestLatencyGridPointsPropagatesQueryErrors(t *testing.T) {
	ctx := context.Background()
	window, _ := resolveLatencyGridWindow("1h")

	// Series lookup failure: the whole schema is gone.
	seriesStore := newLatencyGridTestStore(t)
	seedLatencyGridFixture(ctx, t, seriesStore, window)
	if _, err := seriesStore.db.ExecContext(ctx, `DROP TABLE node_probe_targets;`); err != nil {
		t.Fatalf("drop node_probe_targets: %v", err)
	}
	if _, err := seriesStore.latencyGridPoints(ctx, "node-a", window); err == nil {
		t.Fatal("expected series lookup error for latency grid")
	}
	if _, err := seriesStore.serviceLatencyGridPoints(ctx, "t-1", window); err == nil {
		t.Fatal("expected series lookup error for service latency grid")
	}

	// Measurement query failure: series resolve fine, the aggregation source is gone.
	measurementStore := newLatencyGridTestStore(t)
	seedLatencyGridFixture(ctx, t, measurementStore, window)
	if _, err := measurementStore.db.ExecContext(ctx, `DROP TABLE latency_history_rollups;`); err != nil {
		t.Fatalf("drop latency_history_rollups: %v", err)
	}
	if _, err := measurementStore.latencyGridPoints(ctx, "node-a", window); err == nil {
		t.Fatal("expected measurement query error for latency grid")
	}
	if _, err := measurementStore.serviceLatencyGridPoints(ctx, "t-1", window); err == nil {
		t.Fatal("expected measurement query error for service latency grid")
	}

	// Closed database: both dimensions surface the driver error.
	closedStore, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	seedLatencyGridFixture(ctx, t, closedStore, window)
	if err := closedStore.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := closedStore.latencyGridPoints(ctx, "node-a", window); err == nil {
		t.Fatal("expected error from closed db on latency grid")
	}
	if _, err := closedStore.serviceLatencyGridPoints(ctx, "t-1", window); err == nil {
		t.Fatal("expected error from closed db on service latency grid")
	}
}

func TestLatencyGridQueryMatchesPreMergeSQL(t *testing.T) {
	nodeQuery := latencyGridQuery(latencyGridByNode)
	for _, want := range []string{
		"FROM probe_rounds WHERE node_id = ? AND ts >= ? AND ts < ?",
		"FROM latency_history_rollups WHERE node_id = ? AND bucket_start >= ? AND bucket_start < ?",
		"SELECT (measurements.ts / ?) * ? AS bucket_ts, measurements.target_id,",
		"WHERE measurements.node_id = ?",
		"GROUP BY bucket_ts, measurements.target_id",
	} {
		if !contains(nodeQuery, want) {
			t.Fatalf("node grid query missing %q\n%s", want, nodeQuery)
		}
	}
	if contains(nodeQuery, "JOIN nodes n") || contains(nodeQuery, "n.disabled = 0") {
		t.Fatalf("node grid query must not join nodes:\n%s", nodeQuery)
	}

	serviceQuery := latencyGridQuery(latencyGridByTarget)
	for _, want := range []string{
		"FROM probe_rounds WHERE target_id = ? AND ts >= ? AND ts < ?",
		"FROM latency_history_rollups WHERE target_id = ? AND bucket_start >= ? AND bucket_start < ?",
		"SELECT (measurements.ts / ?) * ? AS bucket_ts, measurements.node_id,",
		"JOIN nodes n ON n.id = measurements.node_id",
		"WHERE measurements.target_id = ?",
		"AND n.disabled = 0",
		"GROUP BY bucket_ts, measurements.node_id",
	} {
		if !contains(serviceQuery, want) {
			t.Fatalf("service grid query missing %q\n%s", want, serviceQuery)
		}
	}
	// Both dimensions keep the enabled-link join and the probe_targets join.
	for name, query := range map[string]string{"node": nodeQuery, "service": serviceQuery} {
		for _, want := range []string{
			"JOIN probe_targets pt ON pt.id = measurements.target_id",
			"LEFT JOIN node_probe_targets npt ON npt.node_id = measurements.node_id AND npt.target_id = measurements.target_id",
			"AND COALESCE(npt.enabled, 0) = 1",
		} {
			if !contains(query, want) {
				t.Fatalf("%s grid query missing %q", name, want)
			}
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}

func assertLatencyPointsEqual(t *testing.T, got, want []LatencyPoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("point count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if !reflect.DeepEqual(latencyPointKey(got[index]), latencyPointKey(want[index])) {
			t.Fatalf("point %d = %s, want %s", index, latencyPointKey(got[index]), latencyPointKey(want[index]))
		}
	}
}

func assertServiceLatencyPointsEqual(t *testing.T, got, want []ServiceLatencyPoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("point count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if !reflect.DeepEqual(serviceLatencyPointKey(got[index]), serviceLatencyPointKey(want[index])) {
			t.Fatalf("point %d = %s, want %s", index, serviceLatencyPointKey(got[index]), serviceLatencyPointKey(want[index]))
		}
	}
}

func latencyPointKey(point LatencyPoint) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%.9f", point.TS, point.TargetID, point.TargetName,
		formatFloatPtr(point.MedianMS), formatFloatPtr(point.AvgMS), point.LossPercent)
}

func serviceLatencyPointKey(point ServiceLatencyPoint) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%.9f", point.TS, point.NodeID, point.NodeName,
		formatFloatPtr(point.MedianMS), formatFloatPtr(point.AvgMS), point.LossPercent)
}

func formatFloatPtr(value *float64) string {
	if value == nil {
		return "nil"
	}
	return fmt.Sprintf("%.9f", *value)
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %.6f", name, want)
	}
	assertFloat(t, name, *got, want)
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if diff := got - want; diff > 0.000001 || diff < -0.000001 {
		t.Fatalf("%s = %.6f, want %.6f", name, got, want)
	}
}
