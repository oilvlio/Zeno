package api

import "context"

// Desired shape of the two probe_rounds uniqueness indexes. Declared once so
// the "is the schema already correct?" check and the rebuild cannot disagree.
var (
	probeRoundIdempotencyIndexColumns = []string{"node_id", "target_id", "ts", "type", "idempotency_key"}
	probeRoundAgentIndexColumns       = []string{"node_id", "agent_round_id"}
)

// probeRoundIndexState is the observed shape of both indexes plus whether any
// row still needs backfill or duplicate repair.
//
// The migration reads six independent facts before it can decide what to do.
// Gathering them into one value keeps the decision readable and lets the
// already-correct fast path be a single expression.
type probeRoundIndexState struct {
	idempotencyColumns []string
	idempotencyUnique  bool
	agentColumns       []string
	agentUnique        bool
	rowsNeedBackfill   bool
	duplicateAgentIDs  bool
}

// inspectProbeRoundIndexState reads current index shape and outstanding data work.
func (s *SQLiteStore) inspectProbeRoundIndexState(ctx context.Context) (probeRoundIndexState, error) {
	var state probeRoundIndexState
	var err error

	if state.idempotencyColumns, err = sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_idempotency"); err != nil {
		return probeRoundIndexState{}, err
	}
	if state.idempotencyUnique, err = sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_idempotency"); err != nil {
		return probeRoundIndexState{}, err
	}
	if state.agentColumns, err = sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_agent_id"); err != nil {
		return probeRoundIndexState{}, err
	}
	if state.agentUnique, err = sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_agent_id"); err != nil {
		return probeRoundIndexState{}, err
	}
	if state.rowsNeedBackfill, err = s.probeRoundRowsNeedBackfill(ctx); err != nil {
		return probeRoundIndexState{}, err
	}
	if state.duplicateAgentIDs, err = s.probeRoundDuplicateAgentIDsExist(ctx); err != nil {
		return probeRoundIndexState{}, err
	}
	return state, nil
}

// upToDate reports that both indexes already have their final shape and no row
// needs repair, so the migration can return without touching the database.
func (state probeRoundIndexState) upToDate() bool {
	return !state.rowsNeedBackfill &&
		!state.duplicateAgentIDs &&
		state.idempotencyIndexCorrect() &&
		state.agentIndexCorrect()
}

func (state probeRoundIndexState) idempotencyIndexCorrect() bool {
	return stringSlicesEqual(state.idempotencyColumns, probeRoundIdempotencyIndexColumns) && state.idempotencyUnique
}

func (state probeRoundIndexState) agentIndexCorrect() bool {
	return stringSlicesEqual(state.agentColumns, probeRoundAgentIndexColumns) && state.agentUnique
}

// probeRoundRowsNeedBackfill reports whether any row still lacks an idempotency
// key, a payload hash, or a promoted agent_round_id.
//
// EXISTS with LIMIT 1 keeps this bounded on a multi-million row table; it is
// re-run after every batch, so a full count would make the migration quadratic.
func (s *SQLiteStore) probeRoundRowsNeedBackfill(ctx context.Context) (bool, error) {
	var pending int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM probe_rounds
			WHERE idempotency_key = '' OR payload_hash = ''
			   OR (COALESCE(agent_round_id, '') = '' AND idempotency_key GLOB 'agent:*')
			LIMIT 1
		)
	`).Scan(&pending); err != nil {
		return false, err
	}
	return pending != 0, nil
}
