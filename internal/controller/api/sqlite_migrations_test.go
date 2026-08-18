package api

import (
	"bytes"
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (s *sqliteSchemaStore) runSchemaMigration(ctx context.Context, name string, migrate func(context.Context) error) error {
	return s.runValidatedSchemaMigration(ctx, name, nil, migrate)
}

func TestNormalizeProbeTargetDisplayOrderPreservesExplicitOrderAndAppendsLegacyZerosByCreation(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, display_order, created_at, updated_at)
		VALUES ('explicit-first', 'explicit-first', 'ping', '192.0.2.1', NULL, 3, 1000, 30, 50, ?, ?)
	`, created+4, created+4); err != nil {
		t.Fatalf("insert explicitly ordered target: %v", err)
	}
	for index, target := range []struct {
		id string
		at int64
	}{
		{id: "created-second", at: created + 2},
		{id: "created-first", at: created + 1},
		{id: "created-third", at: created + 3},
	} {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO probe_targets (id, name, type, address, port, count, timeout_ms, interval_sec, display_order, created_at, updated_at)
			VALUES (?, ?, 'ping', '192.0.2.1', NULL, 3, 1000, 30, 0, ?, ?)
		`, target.id, target.id, target.at, target.at); err != nil {
			t.Fatalf("insert legacy target %d: %v", index, err)
		}
	}
	if err := store.normalizeProbeTargetDisplayOrder(ctx); err != nil {
		t.Fatalf("normalize target order: %v", err)
	}
	rows, err := store.db.QueryContext(ctx, `SELECT id, display_order FROM probe_targets ORDER BY display_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for index, wantID := range []string{"explicit-first", "created-first", "created-second", "created-third"} {
		if !rows.Next() {
			t.Fatalf("missing target %d", index)
		}
		var id string
		var displayOrder int
		if err := rows.Scan(&id, &displayOrder); err != nil {
			t.Fatal(err)
		}
		if id != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("target %d = %s/%d, want %s/%d", index, id, displayOrder, wantID, (index+1)*10)
		}
	}
}

func TestRunSchemaMigrationRecordsAndSkipsCompletedWork(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	calls := 0
	migration := func(context.Context) error {
		calls++
		return nil
	}
	if err := store.runSchemaMigration(context.Background(), "test_once", migration); err != nil {
		t.Fatal(err)
	}
	if err := store.runSchemaMigration(context.Background(), "test_once", migration); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("migration calls = %d, want 1", calls)
	}

	var applied, durationMS int64
	if err := store.db.QueryRow(`SELECT applied_at, duration_ms FROM schema_migrations WHERE name = 'test_once'`).Scan(&applied, &durationMS); err != nil {
		t.Fatal(err)
	}
	if applied <= 0 || durationMS < 0 {
		t.Fatalf("invalid migration metadata applied_at=%d duration_ms=%d", applied, durationMS)
	}
}

func TestRunSchemaMigrationDoesNotRecordFailureAndCanRetry(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	calls := 0
	migration := func(context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("injected failure")
		}
		return nil
	}
	if err := store.runSchemaMigration(context.Background(), "test_retry", migration); err == nil {
		t.Fatal("first migration unexpectedly succeeded")
	}
	var recorded int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = 'test_retry'`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Fatalf("failed migration records = %d, want 0", recorded)
	}
	if err := store.runSchemaMigration(context.Background(), "test_retry", migration); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("migration calls = %d, want 2", calls)
	}
}

func TestRunValidatedSchemaMigrationAdoptsAlreadyCurrentSchema(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	migrationCalls := 0
	err = store.runValidatedSchemaMigration(context.Background(), "test_adopt_current", func(context.Context) (bool, error) {
		return true, nil
	}, func(context.Context) error {
		migrationCalls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if migrationCalls != 0 {
		t.Fatalf("migration calls = %d, want 0", migrationCalls)
	}
	var durationMS int64
	if err := store.db.QueryRow(`SELECT duration_ms FROM schema_migrations WHERE name = 'test_adopt_current'`).Scan(&durationMS); err != nil {
		t.Fatal(err)
	}
	if durationMS != 0 {
		t.Fatalf("adopted migration duration = %d, want 0", durationMS)
	}
}

func TestOpenSQLiteStoreRecordsVersionedMigrations(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatalf("schema migration count = %d, want at least 1", count)
	}
}

func TestMeasureSchemaStageReportsRowsAndDatabaseSize(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.db.SetMaxOpenConns(1)
	store.db.SetMaxIdleConns(1)

	metrics, err := store.measureSchemaStage(context.Background(), "test-observability", func() error {
		_, err := store.db.Exec(`
			CREATE TABLE schema_stage_observability (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
			INSERT INTO schema_stage_observability (value) VALUES ('one'), ('two');
		`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RowsAffected != 2 {
		t.Fatalf("rows affected = %d, want 2", metrics.RowsAffected)
	}
	if metrics.DatabaseBytesBefore <= 0 || metrics.DatabaseBytesAfter < metrics.DatabaseBytesBefore {
		t.Fatalf("invalid database bytes before=%d after=%d", metrics.DatabaseBytesBefore, metrics.DatabaseBytesAfter)
	}
	if metrics.Duration < 0 {
		t.Fatalf("duration = %s, want non-negative", metrics.Duration)
	}
}

func TestOpenSQLiteStoreLogsEverySchemaStageMetric(t *testing.T) {
	var output bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, stage := range []string{
		"base-ddl",
		"notification-channels",
		"state-sample-columns",
		"state-sample-idempotency",
		"traffic-lifetime-columns",
		"probe-round-columns",
		"probe-round-idempotency",
		"node-columns",
		"exchange-rate-default",
		"legacy-agent-credentials",
		"notification-channel-columns",
		"notification-delivery-columns",
		"notification-routing-bindings",
		"notification-delivery-indexes",
		"traffic-monthly-schema",
		"traffic-monthly-columns",
		"traffic-aggregate-normalize",
		"probe-target-columns",
		"probe-target-display-order",
		"probe-target-global-enabled",
		"traffic-last-sample-backfill",
		"traffic-lifetime-backfill",
		"alert-rule-state-columns",
		"default-alert-rules",
		"retired-notification-config-prune",
	} {
		entry := "sqlite schema stage completed name=" + stage
		if !strings.Contains(output.String(), entry) {
			t.Fatalf("missing schema stage metric %q in logs:\n%s", entry, output.String())
		}
	}
	for _, field := range []string{"duration_ms=", "rows_affected=", "database_bytes_before=", "database_bytes_after=", "database_bytes_delta="} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("missing schema stage field %q in logs", field)
		}
	}
}
