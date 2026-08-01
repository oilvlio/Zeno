package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const schemaMigrationSlowLogThreshold = 100 * time.Millisecond

type schemaStageMetrics struct {
	Duration            time.Duration
	RowsAffected        int64
	DatabaseBytesBefore int64
	DatabaseBytesAfter  int64
}

type schemaStageSnapshot struct {
	totalChanges  int64
	databaseBytes int64
}

func (s *sqliteSchemaStore) runValidatedSchemaMigration(ctx context.Context, name string, current func(context.Context) (bool, error), migrate func(context.Context) error) error {
	name = strings.TrimSpace(name)
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite store is closed")
	}
	if name == "" || migrate == nil {
		return fmt.Errorf("invalid schema migration")
	}
	var applied int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = ?)`, name).Scan(&applied); err != nil {
		return err
	}
	if current != nil {
		valid, err := current(ctx)
		if err != nil {
			return fmt.Errorf("validate schema migration %s: %w", name, err)
		}
		if valid {
			if applied == 0 {
				if err := s.recordSchemaMigration(ctx, name, 0); err != nil {
					return err
				}
				log.Printf("sqlite schema migration adopted name=%s", name)
			}
			return nil
		}
	} else if applied != 0 {
		return nil
	}

	started := time.Now()
	if err := migrate(ctx); err != nil {
		return fmt.Errorf("schema migration %s: %w", name, err)
	}
	duration := time.Since(started)
	if err := s.recordSchemaMigration(ctx, name, duration); err != nil {
		return err
	}
	log.Printf("sqlite schema migration completed name=%s duration=%s", name, duration.Round(time.Millisecond))
	return nil
}

func (s *sqliteSchemaStore) recordSchemaMigration(ctx context.Context, name string, duration time.Duration) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO schema_migrations (name, applied_at, duration_ms)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			applied_at = excluded.applied_at,
			duration_ms = excluded.duration_ms
	`, name, time.Now().UTC().Unix(), duration.Milliseconds()); err != nil {
		return fmt.Errorf("record schema migration %s: %w", name, err)
	}
	return nil
}

func (s *sqliteSchemaStore) schemaStageSnapshot(ctx context.Context) (schemaStageSnapshot, error) {
	var snapshot schemaStageSnapshot
	var pageCount, pageSize int64
	if err := s.db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&snapshot.totalChanges); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return snapshot, err
	}
	if pageCount < 0 || pageSize < 0 || (pageCount > 0 && pageSize > (1<<63-1)/pageCount) {
		return snapshot, fmt.Errorf("invalid sqlite page metrics count=%d size=%d", pageCount, pageSize)
	}
	snapshot.databaseBytes = pageCount * pageSize
	return snapshot, nil
}

func (s *sqliteSchemaStore) measureSchemaStage(ctx context.Context, name string, operation func() error) (schemaStageMetrics, error) {
	metrics := schemaStageMetrics{RowsAffected: -1, DatabaseBytesBefore: -1, DatabaseBytesAfter: -1}
	before, beforeErr := s.schemaStageSnapshot(ctx)
	if beforeErr == nil {
		metrics.DatabaseBytesBefore = before.databaseBytes
	}
	started := time.Now()
	err := operation()
	metrics.Duration = time.Since(started)
	after, afterErr := s.schemaStageSnapshot(ctx)
	if afterErr == nil {
		metrics.DatabaseBytesAfter = after.databaseBytes
	}
	if beforeErr == nil && afterErr == nil && after.totalChanges >= before.totalChanges {
		metrics.RowsAffected = after.totalChanges - before.totalChanges
	}
	status := "completed"
	if err != nil {
		status = "failed"
	}
	databaseBytesDelta := int64(-1)
	if metrics.DatabaseBytesBefore >= 0 && metrics.DatabaseBytesAfter >= 0 {
		databaseBytesDelta = metrics.DatabaseBytesAfter - metrics.DatabaseBytesBefore
	}
	log.Printf("sqlite schema stage %s name=%s duration_ms=%d rows_affected=%d database_bytes_before=%d database_bytes_after=%d database_bytes_delta=%d slow=%t metrics_before_error=%q metrics_after_error=%q",
		status,
		name,
		metrics.Duration.Milliseconds(),
		metrics.RowsAffected,
		metrics.DatabaseBytesBefore,
		metrics.DatabaseBytesAfter,
		databaseBytesDelta,
		metrics.Duration >= schemaMigrationSlowLogThreshold,
		metricErrorText(beforeErr),
		metricErrorText(afterErr),
	)
	if err != nil {
		return metrics, fmt.Errorf("sqlite schema stage %s: %w", name, err)
	}
	return metrics, nil
}

func metricErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
