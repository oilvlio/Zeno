package api

import (
	"context"
	"log"
	"time"
)

const rawHistoryRetention = 26 * time.Hour

const historyRollupRetention = 30 * 24 * time.Hour

const stalePendingNotificationDeliveryAfter = 7 * 24 * time.Hour

const historyRetentionBatchSize = 1000

const historyRetentionBatchPause = 10 * time.Millisecond

// Bound each hourly maintenance slice so a first deployment can progressively
// compact a large database without monopolizing SQLite's single writer.
const historyRetentionMaxBatchCycles = 24

const historyRetentionScheduleOffset = 5 * time.Minute

const pruneExpiredProbeRoundsSQL = `
	DELETE FROM probe_rounds
	WHERE id IN (
		SELECT id FROM probe_rounds INDEXED BY idx_probe_rounds_ts_target_node
		WHERE ts < ?
		ORDER BY ts, target_id, node_id, id
		LIMIT ?
	)
`

const pruneExpiredStateSamplesSQL = `
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

type historyRetentionStore interface {
	MaintainHistory(ctx context.Context, now time.Time) error
}

// PruneRawHistory compacts raw high-frequency samples once the one-day views no
// longer need them. Latency keeps exact one-minute weighted aggregates and
// state keeps exact thirty-second weighted aggregates, which are finer than the
// public 7d/30d chart grids. Probe child samples disappear through the round
// foreign-key cascade.
func (s *SQLiteStore) PruneRawHistory(ctx context.Context, before time.Time) error {
	latencyDone := false
	stateDone := false
	for cycle := 0; cycle < historyRetentionMaxBatchCycles && (!latencyDone || !stateDone); cycle++ {
		if !latencyDone {
			removed, err := s.compactLatencyHistoryBatch(ctx, before)
			if err != nil {
				return err
			}
			latencyDone = removed < historyRetentionBatchSize
		}
		if !stateDone {
			removed, err := s.compactStateHistoryBatch(ctx, before)
			if err != nil {
				return err
			}
			stateDone = removed < historyRetentionBatchSize
		}
		if !latencyDone || !stateDone {
			if err := pauseHistoryRetentionBatch(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SQLiteStore) MaintainHistory(ctx context.Context, now time.Time) error {
	now = now.UTC()
	rollupReady, err := s.historyRollupReady(ctx, now)
	if err != nil {
		return err
	}
	if rollupReady {
		if err := s.PruneRawHistory(ctx, now.Add(-rawHistoryRetention)); err != nil {
			return err
		}
	} else {
		// Preserve the previous release's complete 30-day raw-history view during
		// the rollback grace period. Only data already outside the supported range
		// is removed before tiered compaction becomes destructive.
		legacyCutoff := now.Add(-historyRollupRetention).Unix()
		if err := s.pruneRowsInBatches(ctx, pruneExpiredProbeRoundsSQL, legacyCutoff); err != nil {
			return err
		}
		if err := s.pruneRowsInBatches(ctx, pruneExpiredStateSamplesSQL, legacyCutoff); err != nil {
			return err
		}
	}
	retentionCutoff := now.Add(-historyRollupRetention).Unix()
	latencyStepSeconds := int64(latencyHistoryRollupStep / time.Second)
	latencyRollupCutoff := (retentionCutoff / latencyStepSeconds) * latencyStepSeconds
	stateStepSeconds := int64(stateHistoryRollupStep / time.Second)
	stateRollupCutoff := (retentionCutoff / stateStepSeconds) * stateStepSeconds
	if err := s.pruneRowsInBatches(ctx, `DELETE FROM latency_history_rollups WHERE rowid IN (SELECT rowid FROM latency_history_rollups WHERE bucket_start < ? ORDER BY bucket_start LIMIT ?)`, latencyRollupCutoff); err != nil {
		return err
	}
	if err := s.pruneRowsInBatches(ctx, `DELETE FROM state_history_rollups WHERE rowid IN (SELECT rowid FROM state_history_rollups WHERE bucket_start < ? ORDER BY bucket_start LIMIT ?)`, stateRollupCutoff); err != nil {
		return err
	}
	if err := s.pruneRowsInBatches(ctx, `DELETE FROM notification_deliveries WHERE id IN (SELECT id FROM notification_deliveries WHERE state IN ('delivered', 'failed', 'canceled') AND updated_at < ? ORDER BY id LIMIT ?)`, retentionCutoff); err != nil {
		return err
	}
	stalePendingCutoff := now.Add(-stalePendingNotificationDeliveryAfter).Unix()
	return s.expirePendingNotificationDeliveriesInBatches(ctx, stalePendingCutoff, now.Unix())
}

func (s *SQLiteStore) historyRollupReady(ctx context.Context, now time.Time) (bool, error) {
	var enabledAfter int64
	if err := s.db.QueryRowContext(ctx, `SELECT enabled_after FROM history_rollup_meta WHERE id = 1`).Scan(&enabledAfter); err != nil {
		return false, err
	}
	return !now.Before(time.Unix(enabledAfter, 0).UTC()), nil
}

func (s *SQLiteStore) pruneRowsInBatches(ctx context.Context, query string, cutoff int64) error {
	for cycle := 0; cycle < historyRetentionMaxBatchCycles; cycle++ {
		var removed int64
		err := s.withAgentWrite(ctx, historyRetentionWriteKey, func(writeCtx context.Context) error {
			result, err := s.db.ExecContext(writeCtx, query, cutoff, historyRetentionBatchSize)
			if err != nil {
				return err
			}
			removed, err = result.RowsAffected()
			return err
		})
		if err != nil {
			return err
		}
		if removed < historyRetentionBatchSize {
			return nil
		}
		if err := pauseHistoryRetentionBatch(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) expirePendingNotificationDeliveriesInBatches(ctx context.Context, stalePendingCutoff, now int64) error {
	for cycle := 0; cycle < historyRetentionMaxBatchCycles; cycle++ {
		var updated int64
		err := s.withAgentWrite(ctx, historyRetentionWriteKey, func(writeCtx context.Context) error {
			result, err := s.db.ExecContext(writeCtx, `
				UPDATE notification_deliveries
				SET state = 'failed', last_error = 'expired before delivery', lease_until = 0, claim_token = '', updated_at = ?
				WHERE id IN (
					SELECT id FROM notification_deliveries
					WHERE state IN ('pending', 'leased') AND created_at < ?
					ORDER BY id
					LIMIT ?
				)
			`, now, stalePendingCutoff, historyRetentionBatchSize)
			if err != nil {
				return err
			}
			updated, err = result.RowsAffected()
			return err
		})
		if err != nil {
			return err
		}
		if updated < historyRetentionBatchSize {
			return nil
		}
		if err := pauseHistoryRetentionBatch(ctx); err != nil {
			return err
		}
	}
	return nil
}

func pauseHistoryRetentionBatch(ctx context.Context) error {
	timer := time.NewTimer(historyRetentionBatchPause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (h *handler) runHistoryRetention(ctx context.Context, interval time.Duration) {
	store, ok := h.store.(historyRetentionStore)
	if !ok || interval <= 0 {
		return
	}
	prune := func() {
		pruneCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := store.MaintainHistory(pruneCtx, time.Now().UTC()); err != nil {
			log.Printf("history retention cleanup failed: %v", err)
		}
	}
	first := time.NewTimer(historyRetentionFirstDelay(interval))
	select {
	case <-ctx.Done():
		if !first.Stop() {
			select {
			case <-first.C:
			default:
			}
		}
		return
	case <-first.C:
		prune()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

func historyRetentionFirstDelay(interval time.Duration) time.Duration {
	if interval >= time.Hour {
		return interval + historyRetentionScheduleOffset
	}
	return interval
}
