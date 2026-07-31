package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	latencyHistoryRollupStep = time.Minute
	stateHistoryRollupStep   = 30 * time.Second
)

var stateHistoryRollupMetrics = []string{
	"cpu_percent",
	"load1",
	"load5",
	"load15",
	"memory_used_bytes",
	"memory_total_bytes",
	"swap_used_bytes",
	"swap_total_bytes",
	"disk_used_bytes",
	"disk_total_bytes",
	"net_in_total_bytes",
	"net_out_total_bytes",
	"net_in_speed_bps",
	"net_out_speed_bps",
	"process_count",
	"tcp_connection_count",
	"udp_connection_count",
	"uptime_seconds",
}

func stateHistoryRollupInsertSQL() string {
	columns := []string{"node_id", "bucket_start"}
	values := []string{"node_id", "(ts / ?) * ? AS bucket_start"}
	updates := make([]string, 0, len(stateHistoryRollupMetrics)*2)
	for _, metric := range stateHistoryRollupMetrics {
		sumColumn := metric + "_sum"
		countColumn := metric + "_count"
		columns = append(columns, sumColumn, countColumn)
		values = append(values, "COALESCE(SUM("+metric+"), 0)", "COUNT("+metric+")")
		updates = append(updates,
			sumColumn+" = state_history_rollups."+sumColumn+" + excluded."+sumColumn,
			countColumn+" = state_history_rollups."+countColumn+" + excluded."+countColumn,
		)
	}
	return fmt.Sprintf(`
		WITH candidates AS (
			SELECT samples.*
			FROM nodes
			CROSS JOIN state_samples AS samples INDEXED BY idx_state_samples_node_ts
			WHERE samples.node_id = nodes.id AND samples.ts < ?
			ORDER BY samples.node_id, samples.ts, samples.id
			LIMIT ?
		)
		INSERT INTO state_history_rollups (%s)
		SELECT %s FROM candidates GROUP BY node_id, bucket_start
		ON CONFLICT(node_id, bucket_start) DO UPDATE SET %s
	`, strings.Join(columns, ", "), strings.Join(values, ", "), strings.Join(updates, ", "))
}

var stateHistoryRollupInsertQuery = stateHistoryRollupInsertSQL()

const latencyHistoryRollupInsertSQL = `
	WITH candidates AS (
		SELECT id, node_id, target_id, ts, median_ms, avg_ms, loss_percent
		FROM probe_rounds INDEXED BY idx_probe_rounds_ts_target_node
		WHERE ts < ?
		ORDER BY ts, target_id, node_id, id
		LIMIT ?
	)
	INSERT INTO latency_history_rollups (
		node_id, target_id, bucket_start,
		median_sum, median_count, avg_sum, avg_count, loss_sum, loss_count
	)
	SELECT node_id, target_id, (ts / ?) * ? AS bucket_start,
	       COALESCE(SUM(median_ms), 0), COUNT(median_ms),
	       COALESCE(SUM(avg_ms), 0), COUNT(avg_ms),
	       COALESCE(SUM(loss_percent), 0), COUNT(loss_percent)
	FROM candidates
	GROUP BY node_id, target_id, bucket_start
	ON CONFLICT(node_id, target_id, bucket_start) DO UPDATE SET
		median_sum = latency_history_rollups.median_sum + excluded.median_sum,
		median_count = latency_history_rollups.median_count + excluded.median_count,
		avg_sum = latency_history_rollups.avg_sum + excluded.avg_sum,
		avg_count = latency_history_rollups.avg_count + excluded.avg_count,
		loss_sum = latency_history_rollups.loss_sum + excluded.loss_sum,
		loss_count = latency_history_rollups.loss_count + excluded.loss_count
`

func (s *SQLiteStore) compactStateHistoryBatch(ctx context.Context, before time.Time) (int64, error) {
	stepSeconds := int64(stateHistoryRollupStep / time.Second)
	return s.compactHistoryBatch(ctx, stateHistoryRollupInsertQuery,
		[]any{before.UTC().Unix(), historyRetentionBatchSize, stepSeconds, stepSeconds},
		pruneExpiredStateSamplesSQL,
		before,
	)
}

func (s *SQLiteStore) compactLatencyHistoryBatch(ctx context.Context, before time.Time) (int64, error) {
	stepSeconds := int64(latencyHistoryRollupStep / time.Second)
	return s.compactHistoryBatch(ctx, latencyHistoryRollupInsertSQL,
		[]any{before.UTC().Unix(), historyRetentionBatchSize, stepSeconds, stepSeconds},
		pruneExpiredProbeRoundsSQL,
		before,
	)
}

func (s *SQLiteStore) compactHistoryBatch(ctx context.Context, insertSQL string, insertArgs []any, deleteSQL string, before time.Time) (int64, error) {
	var removed int64
	err := s.withAgentWrite(ctx, historyRetentionWriteKey, func(writeCtx context.Context) error {
		tx, err := s.db.BeginTx(writeCtx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(writeCtx, insertSQL, insertArgs...); err != nil {
			return err
		}
		result, err := tx.ExecContext(writeCtx, deleteSQL, before.UTC().Unix(), historyRetentionBatchSize)
		if err != nil {
			return err
		}
		removed, err = result.RowsAffected()
		if err != nil {
			return err
		}
		return tx.Commit()
	})
	return removed, err
}

func stateHistorySourceSQL() string {
	columns := make([]string, 0, len(stateHistoryRollupMetrics)*2)
	for _, metric := range stateHistoryRollupMetrics {
		columns = append(columns,
			metric+" AS "+metric+"_sum",
			"CASE WHEN "+metric+" IS NULL THEN 0 ELSE 1 END AS "+metric+"_count",
		)
	}
	rawColumns := strings.Join(columns, ", ")
	rollupColumns := make([]string, 0, len(stateHistoryRollupMetrics)*2)
	for _, metric := range stateHistoryRollupMetrics {
		rollupColumns = append(rollupColumns, metric+"_sum", metric+"_count")
	}
	return `
		SELECT ts, ` + rawColumns + `
		FROM state_samples
		WHERE node_id = ? AND ts >= ?
		UNION ALL
		SELECT bucket_start AS ts, ` + strings.Join(rollupColumns, ", ") + `
		FROM state_history_rollups
		WHERE node_id = ? AND bucket_start >= ?
	`
}

func stateHistoryAverageSelectSQL() string {
	averages := make([]string, 0, len(stateHistoryRollupMetrics))
	for _, metric := range stateHistoryRollupMetrics {
		averages = append(averages, "SUM("+metric+"_sum) / NULLIF(SUM("+metric+"_count), 0)")
	}
	return strings.Join(averages, ", ")
}

var (
	stateHistorySourceQuery   = stateHistorySourceSQL()
	stateHistoryAverageSelect = stateHistoryAverageSelectSQL()
)

func scanStateHistoryPoint(rows *sql.Rows) (StatePoint, error) {
	var ts int64
	var cpuPercent, load1, load5, load15, memoryUsed, memoryTotal, swapUsed, swapTotal, diskUsed, diskTotal, netInTotal, netOutTotal, netInSpeed, netOutSpeed, processCount, tcpConnectionCount, udpConnectionCount, uptimeSeconds sql.NullFloat64
	if err := rows.Scan(&ts, &cpuPercent, &load1, &load5, &load15, &memoryUsed, &memoryTotal, &swapUsed, &swapTotal, &diskUsed, &diskTotal, &netInTotal, &netOutTotal, &netInSpeed, &netOutSpeed, &processCount, &tcpConnectionCount, &udpConnectionCount, &uptimeSeconds); err != nil {
		return StatePoint{}, err
	}
	return StatePoint{
		TS:                 time.Unix(ts, 0).UTC().Format(time.RFC3339),
		CPUPercent:         floatPtr(cpuPercent),
		Load1:              floatPtr(load1),
		Load5:              floatPtr(load5),
		Load15:             floatPtr(load15),
		MemoryUsedBytes:    floatPtr(memoryUsed),
		MemoryTotalBytes:   floatPtr(memoryTotal),
		SwapUsedBytes:      floatPtr(swapUsed),
		SwapTotalBytes:     floatPtr(swapTotal),
		DiskUsedBytes:      floatPtr(diskUsed),
		DiskTotalBytes:     floatPtr(diskTotal),
		NetInTotalBytes:    floatPtr(netInTotal),
		NetOutTotalBytes:   floatPtr(netOutTotal),
		NetInSpeedBps:      floatPtr(netInSpeed),
		NetOutSpeedBps:     floatPtr(netOutSpeed),
		ProcessCount:       floatPtr(processCount),
		TCPConnectionCount: floatPtr(tcpConnectionCount),
		UDPConnectionCount: floatPtr(udpConnectionCount),
		UptimeSeconds:      floatPtr(uptimeSeconds),
	}, nil
}
