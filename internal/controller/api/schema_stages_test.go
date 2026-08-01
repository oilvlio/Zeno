package api

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// Stages must run in declaration order. Migrations depend on earlier ones
// having completed, so reordering would corrupt an upgrade.
func TestSchemaStageRunnerRunsInOrder(t *testing.T) {
	runner := newSchemaStageRunner(context.Background(), newSchemaTestStore(t))

	var order []string
	runner.run("first", func() error { order = append(order, "first"); return nil })
	runner.run("second", func() error { order = append(order, "second"); return nil })
	runner.run("third", func() error { order = append(order, "third"); return nil })

	if err := runner.result(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("stages ran out of order: %v", order)
	}
}

// The runner must fail fast. Accumulating an error and continuing would run
// later migrations against a half-migrated database, which is exactly what the
// original `if err != nil { return err }` chain prevented.
func TestSchemaStageRunnerStopsAtFirstFailure(t *testing.T) {
	runner := newSchemaStageRunner(context.Background(), newSchemaTestStore(t))
	boom := errors.New("stage failed")

	ran := map[string]bool{}
	runner.run("before", func() error { ran["before"] = true; return nil })
	runner.run("failing", func() error { ran["failing"] = true; return boom })
	runner.run("after", func() error { ran["after"] = true; return nil })
	runner.columns("after-columns", "nodes", map[string]string{"never_added": "TEXT"})
	runner.exec("after-exec", `CREATE TABLE never_created (v INTEGER)`)

	if !errors.Is(runner.result(), boom) {
		t.Fatalf("want the first failure, got %v", runner.result())
	}
	if !ran["before"] || !ran["failing"] {
		t.Fatalf("stages up to the failure must run: %v", ran)
	}
	if ran["after"] {
		t.Fatal("stages after a failure must be skipped")
	}
}

// The first error must win: a later stage cannot mask the original cause.
func TestSchemaStageRunnerKeepsFirstError(t *testing.T) {
	runner := newSchemaStageRunner(context.Background(), newSchemaTestStore(t))
	first := errors.New("first failure")
	second := errors.New("second failure")

	runner.run("one", func() error { return first })
	runner.run("two", func() error { return second })

	if !errors.Is(runner.result(), first) {
		t.Fatalf("want the first error, got %v", runner.result())
	}
}

// A runner with no failures reports success, and columns/exec stages actually
// apply their effect rather than being silently skipped.
func TestSchemaStageRunnerAppliesEffects(t *testing.T) {
	store := newSchemaTestStore(t)
	ctx := context.Background()
	runner := newSchemaStageRunner(ctx, store)

	runner.exec("create-probe-table", `CREATE TABLE stage_probe (v INTEGER)`)
	runner.columns("add-probe-column", "stage_probe", map[string]string{"added": "TEXT"})

	if err := runner.result(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	exists, err := store.columnExists(ctx, "stage_probe", "added")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !exists {
		t.Fatal("columns stage did not add the column")
	}
}

// ensureSchema is idempotent: it runs on every startup, so a second pass over
// an already-migrated database must succeed unchanged.
func TestEnsureSchemaIsIdempotent(t *testing.T) {
	store := newSchemaTestStore(t)
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatalf("second ensureSchema pass failed: %v", err)
	}
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatalf("third ensureSchema pass failed: %v", err)
	}
}

func newSchemaTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func (r *schemaStageRunner) exec(name, statement string) {
	r.run(name, func() error {
		_, err := r.store.db.ExecContext(r.ctx, statement)
		return err
	})
}
