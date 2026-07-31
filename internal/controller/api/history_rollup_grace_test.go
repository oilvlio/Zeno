package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTieredHistoryPreservesPreviousReleaseRawWindowDuringGrace(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	oldTS := now.Add(-29 * 24 * time.Hour).Unix()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at) VALUES ('node-grace', 'Grace Node', 'hash', 'online', ?, ?)`, now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed grace node: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO state_samples (node_id, ts, cpu_percent) VALUES ('node-grace', ?, 10)`, oldTS); err != nil {
		t.Fatalf("seed grace sample: %v", err)
	}

	if err := store.MaintainHistory(ctx, now); err != nil {
		t.Fatalf("maintain history during grace: %v", err)
	}
	var rawRows, rollupRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM state_samples`).Scan(&rawRows); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM state_history_rollups`).Scan(&rollupRows); err != nil {
		t.Fatal(err)
	}
	if rawRows != 1 || rollupRows != 0 {
		t.Fatalf("grace rows raw=%d rollup=%d, want 1/0", rawRows, rollupRows)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE history_rollup_meta SET enabled_after = 0 WHERE id = 1`); err != nil {
		t.Fatalf("expire grace: %v", err)
	}
	ready, err := store.historyRollupReady(ctx, now)
	if err != nil || !ready {
		t.Fatalf("rollup readiness after grace override = %t, err=%v", ready, err)
	}
	if err := store.MaintainHistory(ctx, now); err != nil {
		t.Fatalf("maintain history after grace: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM state_samples`).Scan(&rawRows); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM state_history_rollups`).Scan(&rollupRows); err != nil {
		t.Fatal(err)
	}
	if rawRows != 0 || rollupRows != 1 {
		t.Fatalf("post-grace rows raw=%d rollup=%d, want 0/1", rawRows, rollupRows)
	}
}
