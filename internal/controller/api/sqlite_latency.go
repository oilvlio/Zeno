package api

import (
	"context"
	"database/sql"
	"time"
)

func (s *SQLiteStore) latencyPoints(ctx context.Context, nodeID string, window latencyWindow) ([]LatencyPoint, error) {
	if useLatencyGrid(window) {
		return s.latencyGridPoints(ctx, nodeID, window)
	}
	since := time.Now().UTC().Add(-time.Duration(window.Samples) * window.Step).Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT pr.ts, pr.target_id, pt.name, pr.median_ms, pr.avg_ms, pr.loss_percent
		FROM probe_rounds pr
		JOIN probe_targets pt ON pt.id = pr.target_id
		LEFT JOIN node_probe_targets npt ON npt.node_id = pr.node_id AND npt.target_id = pr.target_id
		WHERE pr.node_id = ?
		  AND pr.ts >= ?
		  AND COALESCE(npt.enabled, 0) = 1
		ORDER BY pr.ts ASC, pt.display_order ASC, pt.name ASC, pr.id ASC
	`, nodeID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []LatencyPoint
	for rows.Next() {
		var ts int64
		var targetID, targetName string
		var median, avg sql.NullFloat64
		var loss float64
		if err := rows.Scan(&ts, &targetID, &targetName, &median, &avg, &loss); err != nil {
			return nil, err
		}
		points = append(points, LatencyPoint{
			TS:          time.Unix(ts, 0).UTC().Format(time.RFC3339),
			TargetID:    targetID,
			TargetName:  targetName,
			MedianMS:    floatPtr(median),
			AvgMS:       floatPtr(avg),
			LossPercent: loss,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, nil
}

func useLatencyGrid(window latencyWindow) bool {
	gridWindow, ok := resolveLatencyGridWindow(window.Name)
	if !ok {
		return false
	}
	// Some unit tests pass a custom 1h latencyWindow directly to the store to
	// assert raw round storage. Public 1h requests use resolveLatencyWindow's
	// canonical 20 × 3m realtime grid and should stay bucketed for fast initial
	// chart paint.
	if window.Name == "1h" && (window.Samples != gridWindow.Samples || window.Step != gridWindow.Step) {
		return false
	}
	return true
}

type latencyGridTarget struct {
	ID   string
	Name string
}

type latencyGridBucket struct {
	medianSum   float64
	medianCount int
	avgSum      float64
	avgCount    int
	lossSum     float64
	lossCount   int
}

func (bucket *latencyGridBucket) add(median, avg sql.NullFloat64, loss float64) {
	if median.Valid {
		bucket.medianSum += median.Float64
		bucket.medianCount++
	}
	if avg.Valid {
		bucket.avgSum += avg.Float64
		bucket.avgCount++
	}
	bucket.lossSum += loss
	bucket.lossCount++
}

func (bucket latencyGridBucket) medianPtr() *float64 {
	if bucket.medianCount == 0 {
		return nil
	}
	value := bucket.medianSum / float64(bucket.medianCount)
	return &value
}

func (bucket latencyGridBucket) avgPtr() *float64 {
	if bucket.avgCount > 0 {
		value := bucket.avgSum / float64(bucket.avgCount)
		return &value
	}
	if bucket.medianCount > 0 {
		value := bucket.medianSum / float64(bucket.medianCount)
		return &value
	}
	return nil
}

func (bucket latencyGridBucket) lossPercent() float64 {
	if bucket.lossCount == 0 {
		return 0
	}
	return bucket.lossSum / float64(bucket.lossCount)
}

func (s *SQLiteStore) latencyGridPoints(ctx context.Context, nodeID string, window latencyWindow) ([]LatencyPoint, error) {
	gridWindow, ok := resolveLatencyGridWindow(window.Name)
	if !ok {
		return nil, nil
	}
	targets, err := s.enabledLatencyTargetsForNode(ctx, nodeID)
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

func (s *SQLiteStore) serviceLatencyGridPoints(ctx context.Context, targetID string, window latencyWindow) ([]ServiceLatencyPoint, error) {
	gridWindow, ok := resolveLatencyGridWindow(window.Name)
	if !ok {
		return nil, nil
	}
	nodes, err := s.enabledLatencyNodesForTarget(ctx, targetID)
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

func (s *SQLiteStore) enabledLatencyTargetsForNode(ctx context.Context, nodeID string) ([]latencyGridTarget, error) {
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

func (s *SQLiteStore) enabledLatencyNodesForTarget(ctx context.Context, targetID string) ([]latencyGridTarget, error) {
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

func latencyGridBounds(window latencyWindow) (time.Time, time.Time, int64) {
	step := window.Step
	if step <= 0 {
		step = time.Minute
	}
	stepSeconds := int64(step.Seconds())
	if stepSeconds <= 0 {
		stepSeconds = 1
	}
	endUnix := (time.Now().UTC().Unix() / stepSeconds) * stepSeconds
	end := time.Unix(endUnix, 0).UTC()
	start := end.Add(-time.Duration(window.Samples-1) * step)
	return start, end, stepSeconds
}

func (s *SQLiteStore) statePoints(ctx context.Context, nodeID string, window latencyWindow) ([]StatePoint, error) {
	since := time.Now().UTC().Add(-time.Duration(window.Samples) * window.Step).Unix()
	stepSeconds := int64(window.Step.Seconds())
	if stepSeconds <= 0 {
		stepSeconds = 1
	}
	query := `WITH measurements AS (` + stateHistorySourceQuery + `)
		SELECT (ts / ?) * ? AS bucket_ts, ` + stateHistoryAverageSelect + `
		FROM measurements
		GROUP BY bucket_ts
		ORDER BY bucket_ts ASC`
	rows, err := s.db.QueryContext(ctx, query, nodeID, since, nodeID, since, stepSeconds, stepSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []StatePoint
	for rows.Next() {
		point, err := scanStateHistoryPoint(rows)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, nil
}
