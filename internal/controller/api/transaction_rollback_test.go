package api

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The store's transaction idiom is:
//
//	tx, _ := db.BeginTx(...)
//	defer func() { rollbackUnlessCommitted(tx) }()
//	...
//	tx = nil   // set only after a successful Commit
//
// The closure is required. `defer rollbackUnlessCommitted(tx)` evaluates tx at
// defer time, so the later `tx = nil` would never be observed and the rollback
// would always run (returning ErrTxDone after a commit). That was harmless but
// it made the assignment dead code and hid the intended contract, so these
// tests pin both halves of the behaviour.

// After a commit, the deferred rollback must be skipped entirely.
func TestRollbackUnlessCommittedSkippedAfterCommit(t *testing.T) {
	db := newRollbackTestDB(t)

	rolledBack := func() bool {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		observed := false
		defer func() {
			if tx != nil {
				observed = true
			}
			rollbackUnlessCommitted(tx)
		}()
		if _, err := tx.Exec(`INSERT INTO rollback_probe (value) VALUES (1)`); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		tx = nil
		return observed
	}()

	if rolledBack {
		t.Fatal("committed transaction must not reach rollback")
	}
	if got := countRollbackProbe(t, db); got != 1 {
		t.Fatalf("committed row missing: count=%d", got)
	}
}

// Returning early without committing must still roll the work back.
func TestRollbackUnlessCommittedRunsOnEarlyReturn(t *testing.T) {
	db := newRollbackTestDB(t)

	func() {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { rollbackUnlessCommitted(tx) }()
		if _, err := tx.Exec(`INSERT INTO rollback_probe (value) VALUES (2)`); err != nil {
			t.Fatalf("insert: %v", err)
		}
		// Abandon the transaction the way a validation failure would.
	}()

	if got := countRollbackProbe(t, db); got != 0 {
		t.Fatalf("abandoned work was not rolled back: count=%d", got)
	}
}

// A nil handle must be inert, since that is how call sites signal "committed".
func TestRollbackUnlessCommittedNilIsNoop(t *testing.T) {
	rollbackUnlessCommitted(nil)
}

// Guard the idiom itself: a bare `defer rollbackUnlessCommitted(tx)` silently
// disables every `tx = nil` in that function, so it must not reappear.
func TestNoBareDeferredRollbackArgument(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, path := range paths {
		// Test files are skipped: this file documents the forbidden form in
		// prose, and the guard targets production call sites.
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(source), "defer rollbackUnlessCommitted(") {
			t.Errorf("%s defers rollbackUnlessCommitted with a direct argument; wrap it in a closure so `tx = nil` is honoured", path)
		}
	}
}

func newRollbackTestDB(t *testing.T) *sql.DB {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.Exec(`CREATE TABLE rollback_probe (value INTEGER)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	return store.db
}

func countRollbackProbe(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rollback_probe`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}
