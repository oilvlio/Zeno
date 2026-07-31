package api

import (
	"context"
	"database/sql"
	"time"

	"github.com/shui1iao/zeno/internal/controller/history"
)

// Rollup bucket widths, metric list and SQL generation live in
// internal/controller/history. This file executes one compaction batch inside
// a transaction and scans aggregated rows back into API types.

// Read paths in sqlite_latency.go bind these prebuilt statements.
var (
	stateHistorySourceQuery   = history.StateSourceQuery
	stateHistoryAverageSelect = history.StateAverageSelect
)

func (s *SQLiteStore) compactStateHistoryBatch(ctx context.Context, before time.Time) (int64, error) {
	stepSeconds := history.StepSeconds(history.StateRollupStep)
	return s.compactHistoryBatch(ctx, history.StateRollupInsertQuery,
		[]any{before.UTC().Unix(), historyRetentionBatchSize, stepSeconds, stepSeconds},
		history.PruneExpiredStateSamplesSQL,
		before,
	)
}

func (s *SQLiteStore) compactLatencyHistoryBatch(ctx context.Context, before time.Time) (int64, error) {
	stepSeconds := history.StepSeconds(history.LatencyRollupStep)
	return s.compactHistoryBatch(ctx, history.LatencyRollupInsertSQL,
		[]any{before.UTC().Unix(), historyRetentionBatchSize, stepSeconds, stepSeconds},
		history.PruneExpiredProbeRoundsSQL,
		before,
	)
}

// compactHistoryBatch folds one batch of raw rows into its rollup tier and
// deletes exactly those rows in the same transaction, so a crash can never
// drop raw data that was not aggregated.
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

// scanStateHistoryPoint unpacks one aggregated row. Column order must match
// history.StateRollupMetrics.
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
