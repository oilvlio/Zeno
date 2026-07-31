package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A retention pass that deletes millions of rows must also return the freed
// pages to the filesystem, otherwise the database file only ever grows.
func TestReclaimFreePagesShrinksDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeno.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if autoVacuum := readAutoVacuum(ctx, t, store); autoVacuum != sqliteAutoVacuumIncremental {
		t.Fatalf("auto_vacuum = %d, want INCREMENTAL for a freshly created store", autoVacuum)
	}

	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO nodes (id, display_name, token_hash, status, created_at, updated_at) VALUES ('vacuum-node', 'Vacuum Node', 'hash', 'online', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 60000)
		INSERT INTO state_samples (node_id, ts, cpu_percent)
		SELECT 'vacuum-node', x, 1 FROM c
	`); err != nil {
		t.Fatalf("seed samples: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	grownSize := databaseFileSize(t, path)

	if _, err := store.db.ExecContext(ctx, `DELETE FROM state_samples`); err != nil {
		t.Fatalf("delete samples: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint after delete: %v", err)
	}
	if freelist := readFreelistCount(ctx, t, store); freelist == 0 {
		t.Fatal("freelist is empty after deleting every sample, cannot exercise reclaim")
	}

	if err := store.reclaimFreePages(ctx); err != nil {
		t.Fatalf("reclaim free pages: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint after reclaim: %v", err)
	}

	if freelist := readFreelistCount(ctx, t, store); freelist != 0 {
		t.Fatalf("freelist = %d after reclaim, want every freed page returned", freelist)
	}
	if reclaimedSize := databaseFileSize(t, path); reclaimedSize >= grownSize {
		t.Fatalf("database file = %d bytes after reclaim, want smaller than %d", reclaimedSize, grownSize)
	}
}

// Databases created before the INCREMENTAL default stay at auto_vacuum=NONE
// until an operator runs a full offline VACUUM. Retention must not fail or
// attempt a blocking vacuum on them.
func TestReclaimFreePagesSkipsLegacyAutoVacuumNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Emulate a legacy file: auto_vacuum can only change through a full VACUUM.
	if _, err := store.db.ExecContext(ctx, `PRAGMA auto_vacuum = NONE`); err != nil {
		t.Fatalf("set auto_vacuum none: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `VACUUM`); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	if autoVacuum := readAutoVacuum(ctx, t, store); autoVacuum == sqliteAutoVacuumIncremental {
		t.Skip("sqlite build kept INCREMENTAL through VACUUM; legacy path is unreachable here")
	}

	if err := store.reclaimFreePages(ctx); err != nil {
		t.Fatalf("reclaim on legacy database = %v, want a silent no-op", err)
	}
}

func readAutoVacuum(ctx context.Context, t *testing.T, store *SQLiteStore) int {
	t.Helper()
	var autoVacuum int
	if err := store.db.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		t.Fatalf("read auto_vacuum: %v", err)
	}
	return autoVacuum
}

func readFreelistCount(ctx context.Context, t *testing.T, store *SQLiteStore) int64 {
	t.Helper()
	var freelist int64
	if err := store.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&freelist); err != nil {
		t.Fatalf("read freelist_count: %v", err)
	}
	return freelist
}

func databaseFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	return info.Size()
}
