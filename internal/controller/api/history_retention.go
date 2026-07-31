package api

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/shui1iao/zeno/internal/controller/history"
)

// Retention policy and SQL live in internal/controller/history. This file
// keeps the store-facing execution: batching against the agent write
// scheduler, the rollback grace decision, and the background loop.

const (
	rawHistoryRetention                   = history.RawRetention
	historyRollupRetention                = history.RollupRetention
	stalePendingNotificationDeliveryAfter = history.StalePendingNotificationDeliveryAfter
	historyRetentionBatchSize             = history.BatchSize
	historyRetentionBatchPause            = history.BatchPause
	historyRetentionMaxBatchCycles        = history.MaxBatchCycles
	historyRetentionScheduleOffset        = history.ScheduleOffset
	historyRetentionVacuumPages           = history.VacuumPagesPerPass
)

// sqliteAutoVacuumIncremental is the PRAGMA auto_vacuum value for INCREMENTAL.
const sqliteAutoVacuumIncremental = 2

const (
	pruneExpiredProbeRoundsSQL  = history.PruneExpiredProbeRoundsSQL
	pruneExpiredStateSamplesSQL = history.PruneExpiredStateSamplesSQL
)

type historyRetentionStore interface {
	MaintainHistory(ctx context.Context, now time.Time) error
}

// PruneRawHistory compacts raw high-frequency samples into the rollup tiers
// once the one-day views no longer need them. Probe child samples disappear
// through the round foreign-key cascade.
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
	cutoffs := history.CutoffsAt(now)
	rollupReady, err := s.historyRollupReady(ctx, now)
	if err != nil {
		return err
	}
	if err := s.pruneRawHistoryTier(ctx, cutoffs, rollupReady); err != nil {
		return err
	}
	if err := s.pruneRollupTiers(ctx, cutoffs); err != nil {
		return err
	}
	if err := s.pruneNotificationHistory(ctx, cutoffs); err != nil {
		return err
	}
	return s.reclaimFreePages(ctx)
}

// reclaimFreePages returns pages freed by this pass to the filesystem so a
// database that just shed millions of rows also shrinks on disk. It is a
// bounded no-op when auto_vacuum is NONE (every database created before the
// INCREMENTAL default) or when nothing is on the freelist, and it never runs
// a full VACUUM: that would hold the single writer for minutes.
func (s *SQLiteStore) reclaimFreePages(ctx context.Context) error {
	var autoVacuum int
	if err := s.db.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		return err
	}
	if autoVacuum != sqliteAutoVacuumIncremental {
		return nil
	}
	var freelist int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&freelist); err != nil {
		return err
	}
	if freelist <= 0 {
		return nil
	}
	return s.withAgentWrite(ctx, historyRetentionWriteKey, func(writeCtx context.Context) error {
		_, err := s.db.ExecContext(writeCtx, `PRAGMA incremental_vacuum(`+strconv.Itoa(historyRetentionVacuumPages)+`)`)
		return err
	})
}

// pruneRawHistoryTier either compacts raw rows into rollups or, during the
// rollback grace period, preserves the previous release's complete 30-day raw
// view and removes only data already outside the supported range.
func (s *SQLiteStore) pruneRawHistoryTier(ctx context.Context, cutoffs history.Cutoffs, rollupReady bool) error {
	if rollupReady {
		return s.PruneRawHistory(ctx, time.Unix(cutoffs.Raw, 0).UTC())
	}
	if err := s.pruneRowsInBatches(ctx, history.PruneExpiredProbeRoundsSQL, cutoffs.LegacyRaw); err != nil {
		return err
	}
	return s.pruneRowsInBatches(ctx, history.PruneExpiredStateSamplesSQL, cutoffs.LegacyRaw)
}

func (s *SQLiteStore) pruneRollupTiers(ctx context.Context, cutoffs history.Cutoffs) error {
	if err := s.pruneRowsInBatches(ctx, history.PruneExpiredLatencyRollupsSQL, cutoffs.LatencyRollup); err != nil {
		return err
	}
	return s.pruneRowsInBatches(ctx, history.PruneExpiredStateRollupsSQL, cutoffs.StateRollup)
}

func (s *SQLiteStore) pruneNotificationHistory(ctx context.Context, cutoffs history.Cutoffs) error {
	if err := s.pruneRowsInBatches(ctx, history.PruneTerminalNotificationDeliveriesSQL, cutoffs.NotificationHistory); err != nil {
		return err
	}
	return s.expirePendingNotificationDeliveriesInBatches(ctx, cutoffs.StalePendingNotification, cutoffs.Now)
}

func (s *SQLiteStore) historyRollupReady(ctx context.Context, now time.Time) (bool, error) {
	var enabledAfter int64
	if err := s.db.QueryRowContext(ctx, `SELECT enabled_after FROM history_rollup_meta WHERE id = 1`).Scan(&enabledAfter); err != nil {
		return false, err
	}
	return !now.Before(time.Unix(enabledAfter, 0).UTC()), nil
}

// runHistoryBatches repeats a bounded write until it stops filling a batch or
// the cycle budget runs out, leaving any remainder for the next pass.
func (s *SQLiteStore) runHistoryBatches(ctx context.Context, exec func(context.Context) (int64, error)) error {
	for cycle := 0; cycle < historyRetentionMaxBatchCycles; cycle++ {
		var affected int64
		err := s.withAgentWrite(ctx, historyRetentionWriteKey, func(writeCtx context.Context) error {
			var execErr error
			affected, execErr = exec(writeCtx)
			return execErr
		})
		if err != nil {
			return err
		}
		if affected < historyRetentionBatchSize {
			return nil
		}
		if err := pauseHistoryRetentionBatch(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) pruneRowsInBatches(ctx context.Context, query string, cutoff int64) error {
	return s.runHistoryBatches(ctx, func(writeCtx context.Context) (int64, error) {
		result, err := s.db.ExecContext(writeCtx, query, cutoff, historyRetentionBatchSize)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	})
}

func (s *SQLiteStore) expirePendingNotificationDeliveriesInBatches(ctx context.Context, stalePendingCutoff, now int64) error {
	return s.runHistoryBatches(ctx, func(writeCtx context.Context) (int64, error) {
		result, err := s.db.ExecContext(writeCtx, history.ExpirePendingNotificationDeliveriesSQL,
			now, stalePendingCutoff, historyRetentionBatchSize)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	})
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
	return history.FirstDelay(interval)
}
