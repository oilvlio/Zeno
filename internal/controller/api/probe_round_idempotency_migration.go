package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"strings"
)

const probeRoundIdempotencyMigrationBatchSize = 200

type probeRoundIdempotencyBackfill struct {
	id             int64
	idempotencyKey string
	agentRoundID   string
	payloadHash    string
}

func (s *SQLiteStore) probeRoundIdempotencyMigrationCurrent(ctx context.Context) (bool, error) {
	indexColumns, err := sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_idempotency")
	if err != nil {
		return false, err
	}
	indexUnique, err := sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_idempotency")
	if err != nil {
		return false, err
	}
	agentIndexColumns, err := sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_agent_id")
	if err != nil {
		return false, err
	}
	agentIndexUnique, err := sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_agent_id")
	if err != nil {
		return false, err
	}
	return stringSlicesEqual(indexColumns, []string{"node_id", "target_id", "ts", "type", "idempotency_key"}) && indexUnique &&
		stringSlicesEqual(agentIndexColumns, []string{"node_id", "agent_round_id"}) && agentIndexUnique, nil
}

func (s *SQLiteStore) migrateProbeRoundIdempotency(ctx context.Context) error {
	indexColumns, err := sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_idempotency")
	if err != nil {
		return err
	}
	indexUnique, err := sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_idempotency")
	if err != nil {
		return err
	}
	agentIndexColumns, err := sqliteIndexColumns(ctx, s.db, "idx_probe_rounds_agent_id")
	if err != nil {
		return err
	}
	agentIndexUnique, err := sqliteIndexUnique(ctx, s.db, "idx_probe_rounds_agent_id")
	if err != nil {
		return err
	}
	var rowsNeedBackfill int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM probe_rounds
			WHERE idempotency_key = '' OR payload_hash = ''
			   OR (COALESCE(agent_round_id, '') = '' AND idempotency_key GLOB 'agent:*')
			LIMIT 1
		)
	`).Scan(&rowsNeedBackfill); err != nil {
		return err
	}
	duplicateAgentIDs, err := s.probeRoundDuplicateAgentIDsExist(ctx)
	if err != nil {
		return err
	}
	wantColumns := []string{"node_id", "target_id", "ts", "type", "idempotency_key"}
	wantAgentColumns := []string{"node_id", "agent_round_id"}
	if rowsNeedBackfill == 0 && !duplicateAgentIDs && stringSlicesEqual(indexColumns, wantColumns) && indexUnique && stringSlicesEqual(agentIndexColumns, wantAgentColumns) && agentIndexUnique {
		return nil
	}

	// A partially upgraded database can already have the final unique Agent-id
	// index while still containing legacy agent:<id> keys that have not been
	// copied into agent_round_id. Drop the index before bounded data backfill so
	// two conflicting legacy keys can be repaired deterministically instead of
	// making startup fail halfway through. Startup has not exposed the store to
	// request handlers yet, and a restart safely resumes before recreating it.
	if rowsNeedBackfill != 0 && len(agentIndexColumns) != 0 {
		if _, err := s.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_probe_rounds_agent_id`); err != nil {
			return err
		}
		agentIndexColumns = nil
		agentIndexUnique = false
	}

	// Backfill in bounded batches. Each batch streams one JOIN over its rounds
	// and samples, computes hashes without retaining every sample, then applies a
	// single set-based UPDATE in a short transaction. This avoids the legacy
	// all-rows allocation, one giant write transaction, and per-round N+1 query.
	for rowsNeedBackfill != 0 {
		backfills, err := s.loadProbeRoundIdempotencyBackfillBatch(ctx, probeRoundIdempotencyMigrationBatchSize)
		if err != nil {
			return err
		}
		if len(backfills) == 0 {
			break
		}
		if err := s.applyProbeRoundIdempotencyBackfillBatch(ctx, backfills); err != nil {
			return err
		}
		if err := s.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM probe_rounds
				WHERE idempotency_key = '' OR payload_hash = ''
				   OR (COALESCE(agent_round_id, '') = '' AND idempotency_key GLOB 'agent:*')
				LIMIT 1
			)
		`).Scan(&rowsNeedBackfill); err != nil {
			return err
		}
	}

	// Older schemas scoped idempotency to target/timestamp and could therefore
	// contain the same Agent round id more than once for a node. Preserve every
	// historical measurement, but keep the oldest row as the canonical Agent-id
	// binding and demote later rows to stable legacy keys. This makes the new
	// node-wide uniqueness rule recoverable and idempotent without deleting data.
	for {
		repaired, err := s.repairProbeRoundDuplicateAgentIDBatch(ctx, probeRoundIdempotencyMigrationBatchSize)
		if err != nil {
			return err
		}
		if repaired == 0 {
			break
		}
	}

	// Index replacement is a separate, short schema transaction after all data
	// batches are durable. A restart can resume the idempotent backfill safely.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	if !stringSlicesEqual(indexColumns, wantColumns) || !indexUnique {
		if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_probe_rounds_idempotency`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			CREATE UNIQUE INDEX idx_probe_rounds_idempotency
			ON probe_rounds(node_id, target_id, ts, type, idempotency_key)
		`); err != nil {
			return err
		}
	}
	if !stringSlicesEqual(agentIndexColumns, wantAgentColumns) || !agentIndexUnique {
		if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_probe_rounds_agent_id`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			CREATE UNIQUE INDEX idx_probe_rounds_agent_id
			ON probe_rounds(node_id, agent_round_id)
			WHERE agent_round_id IS NOT NULL AND agent_round_id <> ''
		`); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *SQLiteStore) probeRoundDuplicateAgentIDsExist(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM probe_rounds
			WHERE COALESCE(agent_round_id, '') <> ''
			GROUP BY node_id, agent_round_id
			HAVING COUNT(*) > 1
			LIMIT 1
		)
	`).Scan(&exists)
	return exists != 0, err
}

func (s *SQLiteStore) repairProbeRoundDuplicateAgentIDBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT id, payload_hash,
			       ROW_NUMBER() OVER (
			         PARTITION BY node_id, agent_round_id
			         ORDER BY id ASC
			       ) AS duplicate_rank
			FROM probe_rounds
			WHERE COALESCE(agent_round_id, '') <> ''
		)
		SELECT id, payload_hash
		FROM ranked
		WHERE duplicate_rank > 1
		ORDER BY id ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, err
	}
	type duplicate struct {
		id          int64
		payloadHash string
	}
	duplicates := make([]duplicate, 0, limit)
	for rows.Next() {
		var candidate duplicate
		if err := rows.Scan(&candidate.id, &candidate.payloadHash); err != nil {
			_ = rows.Close()
			return 0, err
		}
		duplicates = append(duplicates, candidate)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(duplicates) == 0 {
		return 0, nil
	}

	values := make([]string, 0, len(duplicates))
	args := make([]any, 0, len(duplicates)*3)
	for _, candidate := range duplicates {
		values = append(values, "(?, ?, ?)")
		args = append(args, candidate.id, fmt.Sprintf("legacy:%d:%s", candidate.id, candidate.payloadHash), candidate.payloadHash)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	statement := `
		WITH repairs(id, idempotency_key, payload_hash) AS (
			VALUES ` + strings.Join(values, ",") + `
		)
		UPDATE probe_rounds
		SET idempotency_key = (SELECT r.idempotency_key FROM repairs r WHERE r.id = probe_rounds.id),
		    agent_round_id = NULL,
		    payload_hash = (SELECT r.payload_hash FROM repairs r WHERE r.id = probe_rounds.id)
		WHERE id IN (SELECT id FROM repairs)
	`
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	return int(affected), nil
}

func (s *SQLiteStore) loadProbeRoundIdempotencyBackfillBatch(ctx context.Context, limit int) ([]probeRoundIdempotencyBackfill, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH candidates AS (
		SELECT id, idempotency_key, COALESCE(agent_round_id, '') AS agent_round_id
		FROM probe_rounds
		WHERE idempotency_key = '' OR payload_hash = ''
		   OR (COALESCE(agent_round_id, '') = '' AND idempotency_key GLOB 'agent:*')
			ORDER BY id ASC
			LIMIT ?
		)
		SELECT c.id, c.idempotency_key, c.agent_round_id,
		       ps.seq, ps.success, ps.latency_ms, ps.error
		FROM candidates c
		LEFT JOIN probe_samples ps ON ps.round_id = c.id
		ORDER BY c.id ASC, ps.seq ASC
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	backfills := make([]probeRoundIdempotencyBackfill, 0, limit)
	var current probeRoundIdempotencyBackfill
	var digest hash.Hash
	var sampleIndex int
	flush := func() {
		if digest == nil {
			return
		}
		current.payloadHash = hex.EncodeToString(digest.Sum(nil))
		if current.idempotencyKey == "" {
			current.idempotencyKey = fmt.Sprintf("legacy:%d:%s", current.id, current.payloadHash)
		}
		if current.agentRoundID == "" && strings.HasPrefix(current.idempotencyKey, "agent:") {
			candidateAgentRoundID := strings.TrimPrefix(current.idempotencyKey, "agent:")
			if validAgentProbeRoundID(candidateAgentRoundID) {
				current.agentRoundID = candidateAgentRoundID
			} else {
				current.idempotencyKey = fmt.Sprintf("legacy:%d:%s", current.id, current.payloadHash)
			}
		}
		backfills = append(backfills, current)
	}
	for rows.Next() {
		var id int64
		var idempotencyKey, agentRoundID string
		var seq, success sql.NullInt64
		var latency sql.NullFloat64
		var errorText sql.NullString
		if err := rows.Scan(&id, &idempotencyKey, &agentRoundID, &seq, &success, &latency, &errorText); err != nil {
			return nil, err
		}
		if digest == nil || current.id != id {
			flush()
			current = probeRoundIdempotencyBackfill{id: id, idempotencyKey: idempotencyKey, agentRoundID: agentRoundID}
			digest = sha256.New()
			sampleIndex = 0
		}
		if !seq.Valid {
			continue
		}
		sampleIndex++
		sequence := seq.Int64
		if sequence == 0 {
			sequence = int64(sampleIndex)
		}
		writeProbeDigestUint64(digest, uint64(sequence))
		if success.Valid && success.Int64 != 0 {
			writeProbeDigestUint64(digest, 1)
		} else {
			writeProbeDigestUint64(digest, 0)
		}
		if latency.Valid {
			writeProbeDigestUint64(digest, 1)
			writeProbeDigestUint64(digest, math.Float64bits(latency.Float64))
		} else {
			writeProbeDigestUint64(digest, 0)
		}
		errorBytes := []byte(errorText.String)
		writeProbeDigestUint64(digest, uint64(len(errorBytes)))
		_, _ = digest.Write(errorBytes)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	flush()
	return backfills, nil
}

func (s *SQLiteStore) applyProbeRoundIdempotencyBackfillBatch(ctx context.Context, backfills []probeRoundIdempotencyBackfill) error {
	if len(backfills) == 0 {
		return nil
	}
	values := make([]string, 0, len(backfills))
	args := make([]any, 0, len(backfills)*4)
	for _, backfill := range backfills {
		values = append(values, "(?, ?, ?, ?)")
		args = append(args, backfill.id, backfill.idempotencyKey, backfill.agentRoundID, backfill.payloadHash)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	statement := `
		WITH backfill(id, idempotency_key, agent_round_id, payload_hash) AS (
			VALUES ` + strings.Join(values, ",") + `
		)
		UPDATE probe_rounds
		SET idempotency_key = (SELECT b.idempotency_key FROM backfill b WHERE b.id = probe_rounds.id),
		    agent_round_id = NULLIF((SELECT b.agent_round_id FROM backfill b WHERE b.id = probe_rounds.id), ''),
		    payload_hash = (SELECT b.payload_hash FROM backfill b WHERE b.id = probe_rounds.id)
		WHERE id IN (SELECT id FROM backfill)
	`
	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func sqliteIndexColumns(ctx context.Context, db *sql.DB, indexName string) ([]string, error) {
	var statement string
	switch indexName {
	case "idx_probe_rounds_idempotency":
		statement = `PRAGMA index_info('idx_probe_rounds_idempotency')`
	case "idx_probe_rounds_agent_id":
		statement = `PRAGMA index_info('idx_probe_rounds_agent_id')`
	default:
		return nil, fmt.Errorf("unsupported sqlite index %q", indexName)
	}
	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var sequence, columnID int
		var name string
		if err := rows.Scan(&sequence, &columnID, &name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func sqliteIndexUnique(ctx context.Context, db *sql.DB, indexName string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA index_list('probe_rounds')`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return false, err
		}
		if name == indexName {
			return unique != 0, nil
		}
	}
	return false, rows.Err()
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
