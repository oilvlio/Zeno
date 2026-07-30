package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"strings"

	"github.com/shui1iao/zeno/internal/shared/probe"
)

type existingAgentProbeRound struct {
	targetID, targetType, payloadHash string
	ts                                int64
}

func (existing existingAgentProbeRound) matches(round preparedAgentProbeRound) bool {
	if strings.HasPrefix(existing.payloadHash, agentProbePayloadHashV2Prefix) {
		return existing.targetID == strings.TrimSpace(round.targetID) &&
			existing.ts == round.ts.UTC().Unix() &&
			existing.payloadHash == round.payloadHash
	}
	legacyHash := strings.TrimPrefix(existing.payloadHash, "v1:")
	return existing.targetID == strings.TrimSpace(round.targetID) &&
		existing.ts == round.ts.UTC().Unix() &&
		existing.targetType == strings.TrimSpace(round.targetType) &&
		legacyHash == probeRoundIdempotencyKey(round.samples)
}

func loadAgentProbeRoundsByIDTx(ctx context.Context, tx *sql.Tx, nodeID string, rounds []preparedAgentProbeRound) (map[string]existingAgentProbeRound, error) {
	roundIDs := make([]string, 0, len(rounds))
	seen := make(map[string]struct{}, len(rounds))
	for _, round := range rounds {
		roundID := strings.TrimSpace(round.agentRoundID)
		if roundID == "" {
			continue
		}
		if _, ok := seen[roundID]; ok {
			return nil, errInvalidAgentProbeResults
		}
		seen[roundID] = struct{}{}
		roundIDs = append(roundIDs, roundID)
	}
	result := make(map[string]existingAgentProbeRound, len(roundIDs))
	if len(roundIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roundIDs)), ",")
	args := make([]any, 0, len(roundIDs)+1)
	args = append(args, nodeID)
	for _, roundID := range roundIDs {
		args = append(args, roundID)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT agent_round_id, target_id, ts, type, payload_hash
		FROM probe_rounds INDEXED BY idx_probe_rounds_agent_id
		WHERE node_id = ? AND agent_round_id IN (`+placeholders+`)
		  AND agent_round_id IS NOT NULL AND agent_round_id <> ''
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var roundID string
		var existing existingAgentProbeRound
		if err := rows.Scan(&roundID, &existing.targetID, &existing.ts, &existing.targetType, &existing.payloadHash); err != nil {
			return nil, err
		}
		result[roundID] = existing
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func agentProbeRoundPayloadHash(round preparedAgentProbeRound) string {
	digest := sha256.New()
	writeProbeDigestBytes(digest, []byte(strings.TrimSpace(round.targetID)))
	writeProbeDigestUint64(digest, uint64(round.ts.UTC().Unix()))
	writeProbeDigestBytes(digest, []byte(strings.TrimSpace(round.targetType)))
	for index, sample := range round.samples {
		seq := sample.Seq
		if seq == 0 {
			seq = index + 1
		}
		writeProbeDigestUint64(digest, uint64(seq))
		if sample.Success {
			writeProbeDigestUint64(digest, 1)
		} else {
			writeProbeDigestUint64(digest, 0)
		}
		if sample.LatencyMS == nil {
			writeProbeDigestUint64(digest, 0)
		} else {
			writeProbeDigestUint64(digest, 1)
			writeProbeDigestUint64(digest, math.Float64bits(*sample.LatencyMS))
		}
		writeProbeDigestBytes(digest, []byte(strings.TrimSpace(sample.Error)))
	}
	return agentProbePayloadHashV2Prefix + hex.EncodeToString(digest.Sum(nil))
}

func writeProbeDigestBytes(digest hash.Hash, value []byte) {
	writeProbeDigestUint64(digest, uint64(len(value)))
	_, _ = digest.Write(value)
}

func probeRoundIdempotencyKey(samples []probe.Sample) string {
	digest := sha256.New()
	for index, sample := range samples {
		seq := sample.Seq
		if seq == 0 {
			seq = index + 1
		}
		writeProbeDigestUint64(digest, uint64(seq))
		if sample.Success {
			writeProbeDigestUint64(digest, 1)
		} else {
			writeProbeDigestUint64(digest, 0)
		}
		if sample.LatencyMS == nil {
			writeProbeDigestUint64(digest, 0)
		} else {
			writeProbeDigestUint64(digest, 1)
			writeProbeDigestUint64(digest, math.Float64bits(*sample.LatencyMS))
		}
		errorText := []byte(sample.Error)
		writeProbeDigestUint64(digest, uint64(len(errorText)))
		_, _ = digest.Write(errorText)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func migratedProbeRoundLegacyRetryPattern(idempotencyKey string) (string, bool) {
	const legacyPrefix = "legacy:"
	if !strings.HasPrefix(idempotencyKey, legacyPrefix) {
		return "", false
	}
	digest := strings.TrimPrefix(idempotencyKey, legacyPrefix)
	if len(digest) != sha256.Size*2 || !isLowerHex(digest) {
		return "", false
	}
	return legacyPrefix + "[0-9]*:" + digest, true
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return false
	}
	return true
}

func writeProbeDigestUint64(digest hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}

// Four bind parameters are used per row by the set-based VALUES update. Keep
// the batch below SQLite's conservative 999-variable compatibility ceiling.
