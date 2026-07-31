package api

import (
	"context"
	"path/filepath"
	"testing"
)

// A freshly migrated database must report itself up to date, so repeated
// startups skip the migration entirely instead of rebuilding indexes on a
// multi-million row table every boot.
func TestProbeRoundIndexStateUpToDateAfterMigration(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	state, err := store.inspectProbeRoundIndexState(ctx)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !state.upToDate() {
		t.Fatalf("fresh database is not up to date: %+v", state)
	}
	if !state.idempotencyIndexCorrect() || !state.agentIndexCorrect() {
		t.Fatalf("indexes not in final shape: %+v", state)
	}
	if state.rowsNeedBackfill || state.duplicateAgentIDs {
		t.Fatalf("fresh database reports outstanding work: %+v", state)
	}

	// Re-running must stay a no-op and leave the state unchanged.
	if err := store.migrateProbeRoundIdempotency(ctx); err != nil {
		t.Fatalf("re-running migration failed: %v", err)
	}
	after, err := store.inspectProbeRoundIndexState(ctx)
	if err != nil {
		t.Fatalf("inspect after: %v", err)
	}
	if !after.upToDate() {
		t.Fatalf("state regressed after re-running: %+v", after)
	}
}

// upToDate must require every condition; any single outstanding item has to
// keep the migration running, otherwise a partially upgraded database would be
// declared finished.
func TestProbeRoundIndexStateUpToDateRequiresEveryCondition(t *testing.T) {
	complete := probeRoundIndexState{
		idempotencyColumns: probeRoundIdempotencyIndexColumns,
		idempotencyUnique:  true,
		agentColumns:       probeRoundAgentIndexColumns,
		agentUnique:        true,
	}
	if !complete.upToDate() {
		t.Fatal("fully migrated state must report up to date")
	}

	degrade := map[string]func(*probeRoundIndexState){
		"rows need backfill":      func(s *probeRoundIndexState) { s.rowsNeedBackfill = true },
		"duplicate agent ids":     func(s *probeRoundIndexState) { s.duplicateAgentIDs = true },
		"idempotency not unique":  func(s *probeRoundIndexState) { s.idempotencyUnique = false },
		"idempotency wrong shape": func(s *probeRoundIndexState) { s.idempotencyColumns = []string{"node_id"} },
		"agent not unique":        func(s *probeRoundIndexState) { s.agentUnique = false },
		"agent wrong shape":       func(s *probeRoundIndexState) { s.agentColumns = []string{"node_id"} },
		"agent index missing":     func(s *probeRoundIndexState) { s.agentColumns = nil },
	}
	for name, apply := range degrade {
		t.Run(name, func(t *testing.T) {
			state := complete
			apply(&state)
			if state.upToDate() {
				t.Fatalf("%s must not be considered up to date", name)
			}
		})
	}
}
