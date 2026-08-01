package api

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *sqliteReadQueries) latestLatencySummary(ctx context.Context, nodeID string) (*LatencySummary, error) {
	var preferredTarget sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT home_probe_target_id FROM nodes WHERE id = ?`, nodeID).Scan(&preferredTarget); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if preferredTarget.Valid && strings.TrimSpace(preferredTarget.String) != "" {
		return s.latestLatencySummaryForTarget(ctx, nodeID, strings.TrimSpace(preferredTarget.String))
	}
	return nil, nil
}

func (s *sqliteReadQueries) latestLatencySummaryForTarget(ctx context.Context, nodeID, preferredTargetID string) (*LatencySummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pr.target_id, pt.name, pr.median_ms, pr.avg_ms, pr.loss_percent, pr.ts
		FROM probe_rounds pr
		JOIN probe_targets pt ON pt.id = pr.target_id
		LEFT JOIN node_probe_targets npt ON npt.node_id = pr.node_id AND npt.target_id = pr.target_id
		WHERE pr.node_id = ?
		  AND pr.target_id = ?
		  AND COALESCE(npt.enabled, 0) = 1
		  AND pr.ts >= ?
		ORDER BY pr.ts DESC, pr.id DESC
	`, nodeID, strings.TrimSpace(preferredTargetID), time.Now().UTC().Add(-24*time.Hour).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaryTargetID, targetName string
	var latestMedian, latestAvg *float64
	var latestTS int64
	var lossTotal float64
	var lossCount int
	for rows.Next() {
		var rowTargetID, rowTargetName string
		var median, avg sql.NullFloat64
		var loss float64
		var ts int64
		if err := rows.Scan(&rowTargetID, &rowTargetName, &median, &avg, &loss, &ts); err != nil {
			return nil, err
		}
		if summaryTargetID == "" {
			summaryTargetID = rowTargetID
			targetName = rowTargetName
			latestTS = ts
			latestMedian = floatPtr(median)
			latestAvg = floatPtr(avg)
			if latestAvg == nil {
				latestAvg = latestMedian
			}
		}
		lossTotal += loss
		lossCount++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if summaryTargetID == "" {
		return nil, nil
	}
	loss := 0.0
	if lossCount > 0 {
		loss = lossTotal / float64(lossCount)
	}
	return &LatencySummary{
		TargetID:    summaryTargetID,
		TargetName:  targetName,
		MedianMS:    latestMedian,
		AvgMS:       latestAvg,
		LossPercent: &loss,
		UpdatedAt:   time.Unix(latestTS, 0).UTC().Format(time.RFC3339),
	}, nil

}

func (s *sqliteReadQueries) latestLatencySummaries(ctx context.Context, nodeID string) ([]LatencySummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pt.id, pt.name, pr.median_ms, pr.avg_ms, pr.loss_percent, pr.ts
		FROM node_probe_targets npt
		JOIN probe_targets pt ON pt.id = npt.target_id
		LEFT JOIN probe_rounds pr ON pr.id = (
			SELECT pr2.id
			FROM probe_rounds pr2
			WHERE pr2.node_id = npt.node_id AND pr2.target_id = npt.target_id
			ORDER BY pr2.ts DESC, pr2.id DESC
			LIMIT 1
		)
		WHERE npt.node_id = ?
		  AND npt.enabled = 1
		ORDER BY pt.display_order ASC, pt.name ASC, pt.id ASC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []LatencySummary{}
	for rows.Next() {
		var targetID, targetName string
		var median, avg, loss sql.NullFloat64
		var ts sql.NullInt64
		if err := rows.Scan(&targetID, &targetName, &median, &avg, &loss, &ts); err != nil {
			return nil, err
		}
		if !ts.Valid {
			continue
		}
		summaries = append(summaries, LatencySummary{
			TargetID:    targetID,
			TargetName:  targetName,
			MedianMS:    floatPtr(median),
			AvgMS:       floatPtr(avg),
			LossPercent: floatPtr(loss),
			UpdatedAt:   time.Unix(ts.Int64, 0).UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}
