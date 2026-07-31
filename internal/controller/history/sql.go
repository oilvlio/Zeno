package history

import (
	"fmt"
	"strings"
)

// StateRollupMetrics lists every numeric state column carried through the
// rollup tier. Order is part of the scan contract: readers unpack aggregated
// columns in exactly this sequence.
var StateRollupMetrics = []string{
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

// Deletion statements force the covering index. Without INDEXED BY, SQLite
// picks a full scan on multi-million-row tables and the batch exceeds the
// agent write timeout.
const (
	// PruneExpiredProbeRoundsSQL removes raw probe rounds before a cutoff.
	PruneExpiredProbeRoundsSQL = `
	DELETE FROM probe_rounds
	WHERE id IN (
		SELECT id FROM probe_rounds INDEXED BY idx_probe_rounds_ts_target_node
		WHERE ts < ?
		ORDER BY ts, target_id, node_id, id
		LIMIT ?
	)
`

	// PruneExpiredStateSamplesSQL removes raw state samples before a cutoff.
	// The CROSS JOIN turns a global scan into a per-node index seek.
	PruneExpiredStateSamplesSQL = `
	DELETE FROM state_samples
	WHERE id IN (
		SELECT samples.id
		FROM nodes
		CROSS JOIN state_samples AS samples INDEXED BY idx_state_samples_node_ts
		WHERE samples.node_id = nodes.id AND samples.ts < ?
		ORDER BY samples.node_id, samples.ts, samples.id
		LIMIT ?
	)
`

	// PruneExpiredLatencyRollupsSQL trims the latency rollup tier.
	PruneExpiredLatencyRollupsSQL = `DELETE FROM latency_history_rollups WHERE rowid IN (SELECT rowid FROM latency_history_rollups WHERE bucket_start < ? ORDER BY bucket_start LIMIT ?)`

	// PruneExpiredStateRollupsSQL trims the state rollup tier.
	PruneExpiredStateRollupsSQL = `DELETE FROM state_history_rollups WHERE rowid IN (SELECT rowid FROM state_history_rollups WHERE bucket_start < ? ORDER BY bucket_start LIMIT ?)`

	// PruneTerminalNotificationDeliveriesSQL trims delivered/failed history.
	PruneTerminalNotificationDeliveriesSQL = `DELETE FROM notification_deliveries WHERE id IN (SELECT id FROM notification_deliveries WHERE state IN ('delivered', 'failed', 'canceled') AND updated_at < ? ORDER BY id LIMIT ?)`

	// ExpirePendingNotificationDeliveriesSQL fails deliveries that never
	// reached a terminal state. Arguments are (now, cutoff, limit).
	ExpirePendingNotificationDeliveriesSQL = `
				UPDATE notification_deliveries
				SET state = 'failed', last_error = 'expired before delivery', lease_until = 0, claim_token = '', updated_at = ?
				WHERE id IN (
					SELECT id FROM notification_deliveries
					WHERE state IN ('pending', 'leased') AND created_at < ?
					ORDER BY id
					LIMIT ?
				)
			`

	// LatencyRollupInsertSQL folds a bounded batch of raw probe rounds into
	// weighted latency buckets. Arguments are (cutoff, limit, step, step).
	// The candidate ordering matches the delete statement so insert and
	// delete always cover the same rows.
	LatencyRollupInsertSQL = `
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
)

// StateRollupInsertSQL builds the state compaction statement. Arguments are
// (cutoff, limit, step, step).
func StateRollupInsertSQL() string {
	columns := []string{"node_id", "bucket_start"}
	values := []string{"node_id", "(ts / ?) * ? AS bucket_start"}
	updates := make([]string, 0, len(StateRollupMetrics)*2)
	for _, metric := range StateRollupMetrics {
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

// StateSourceSQL unions raw samples with the rollup tier so a single read
// path serves both windows. Arguments are (node, rawFrom, node, bucketFrom).
func StateSourceSQL() string {
	columns := make([]string, 0, len(StateRollupMetrics)*2)
	for _, metric := range StateRollupMetrics {
		columns = append(columns,
			metric+" AS "+metric+"_sum",
			"CASE WHEN "+metric+" IS NULL THEN 0 ELSE 1 END AS "+metric+"_count",
		)
	}
	rollupColumns := make([]string, 0, len(StateRollupMetrics)*2)
	for _, metric := range StateRollupMetrics {
		rollupColumns = append(rollupColumns, metric+"_sum", metric+"_count")
	}
	return `
		SELECT ts, ` + strings.Join(columns, ", ") + `
		FROM state_samples
		WHERE node_id = ? AND ts >= ?
		UNION ALL
		SELECT bucket_start AS ts, ` + strings.Join(rollupColumns, ", ") + `
		FROM state_history_rollups
		WHERE node_id = ? AND bucket_start >= ?
	`
}

// StateAverageSelectSQL renders the weighted-average projection over the
// unioned source, preserving StateRollupMetrics order.
func StateAverageSelectSQL() string {
	averages := make([]string, 0, len(StateRollupMetrics))
	for _, metric := range StateRollupMetrics {
		averages = append(averages, "SUM("+metric+"_sum) / NULLIF(SUM("+metric+"_count), 0)")
	}
	return strings.Join(averages, ", ")
}

// Prebuilt statements. Generation is deterministic, so building once at
// init keeps hot read paths free of string assembly.
var (
	StateRollupInsertQuery = StateRollupInsertSQL()
	StateSourceQuery       = StateSourceSQL()
	StateAverageSelect     = StateAverageSelectSQL()
)
