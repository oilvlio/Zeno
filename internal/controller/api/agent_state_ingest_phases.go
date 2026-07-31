package api

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Ingest phases for a single agent state sample. insertAgentStateSampleTx runs
// them in order inside one transaction; they are separated so each rule can be
// stated and tested on its own rather than read out of one long function.

// agentStateSampleDuplicate reports whether this sample was already stored.
//
// Agents retry, so the same report can arrive more than once. When the agent
// supplies a sample id that id is authoritative: a repeat with the same payload
// is ignored, but a repeat with a *different* payload is rejected, because a
// reused id carrying new numbers means the agent is misbehaving and silently
// accepting it would overwrite history. Without a sample id the only safe
// signal is an identical (ts, payload) pair.
func agentStateSampleDuplicate(
	ctx context.Context,
	tx *sql.Tx,
	nodeID, sampleID, payloadHash string,
	sampleTS int64,
) (bool, error) {
	if sampleID != "" {
		var existingHash string
		err := tx.QueryRowContext(ctx, agentStateSampleLookupSQL, nodeID, sampleID).Scan(&existingHash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			// An empty stored hash predates payload hashing; treat it as the
			// same sample rather than failing an upgrade.
			if existingHash == payloadHash || existingHash == "" {
				return true, nil
			}
			return false, errInvalidAgentStateReport
		}
		return false, nil
	}

	var existingID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM state_samples
		WHERE node_id = ? AND ts = ? AND payload_hash = ?
		LIMIT 1
	`, nodeID, sampleTS, payloadHash).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return err == nil, nil
}

// agentStateRateLimited reports whether this sample arrives too soon after the
// node's newest stored sample, or is not newer than it at all.
//
// Out-of-order and too-frequent reports are dropped rather than rejected: a
// clock-skewed or over-eager agent should not fail, it should simply not widen
// history.
func agentStateRateLimited(ctx context.Context, tx *sql.Tx, nodeID string, sampleTS int64) (bool, error) {
	var lastSampleTS sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT ts
		FROM state_samples
		WHERE node_id = ?
		ORDER BY ts DESC, id DESC
		LIMIT 1
	`, nodeID).Scan(&lastSampleTS); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if !lastSampleTS.Valid {
		return false, nil
	}
	minIntervalSec := int64(minAgentStateReportInterval / time.Second)
	if minIntervalSec < 1 {
		minIntervalSec = 1
	}
	if sampleTS <= lastSampleTS.Int64 {
		return true, nil
	}
	return sampleTS-lastSampleTS.Int64 < minIntervalSec, nil
}

// agentStateOptionalCounters holds counter groups that may be absent.
//
// Values are `any` because an invalid group is stored as SQL NULL rather than
// zero: a failed collection is unknown, not "no traffic".
type agentStateOptionalCounters struct {
	netInTotalBytes    any
	netOutTotalBytes   any
	tcpConnectionCount any
	udpConnectionCount any
}

// resolveAgentStateOptionalCounters nulls out counter groups the agent reported
// as invalid.
//
// Invalid optional collector groups are unknown, not zero. Persisting NULL keeps
// public summaries and history from presenting a failed collection as a real
// counter value. Traffic accounting independently refuses to advance its
// baseline unless net_totals_valid is true (or absent for legacy agents).
func resolveAgentStateOptionalCounters(state AgentStateRequest) agentStateOptionalCounters {
	counters := agentStateOptionalCounters{
		netInTotalBytes:    state.NetInTotalBytes,
		netOutTotalBytes:   state.NetOutTotalBytes,
		tcpConnectionCount: state.TCPConnectionCount,
		udpConnectionCount: state.UDPConnectionCount,
	}
	if state.NetTotalsValid != nil && !*state.NetTotalsValid {
		counters.netInTotalBytes = nil
		counters.netOutTotalBytes = nil
	}
	if state.ConnectionCountsValid != nil && !*state.ConnectionCountsValid {
		counters.tcpConnectionCount = nil
		counters.udpConnectionCount = nil
	}
	return counters
}

// applyAgentStateTraffic advances the lifetime and monthly billing baselines
// for a stored sample.
//
// The node row is always read, even when the counters are skipped: a sample for
// an unknown or disabled node must still fail with errNodeNotFound rather than
// being silently accepted.
//
// A failed platform counter read is not a real zero sample. Skipping the
// billing baseline is essential for first-sample failures: otherwise a later
// recovery would bill the machine's full lifetime counter as new use.
func applyAgentStateTraffic(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	state AgentStateRequest,
	sampleTS time.Time,
	receivedUnix int64,
) error {
	var billingMode string
	var monthlyResetDay int
	var billingEpoch int64
	if err := tx.QueryRowContext(ctx,
		`SELECT billing_mode, monthly_reset_day, billing_traffic_epoch FROM nodes WHERE id = ? AND disabled = 0`,
		nodeID,
	).Scan(&billingMode, &monthlyResetDay, &billingEpoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNodeNotFound
		}
		return err
	}
	if state.NetTotalsValid != nil && !*state.NetTotalsValid {
		return nil
	}

	counterSource := strings.TrimSpace(state.NetCounterSource)
	month := billingPeriodKey(sampleTS, monthlyResetDay)
	if err := upsertLifetimeTraffic(ctx, tx, nodeID,
		state.NetInTotalBytes, state.NetOutTotalBytes, counterSource, sampleTS.Unix(), receivedUnix); err != nil {
		return err
	}
	return upsertMonthlyTraffic(ctx, tx, nodeID, month, billingEpoch, monthlyResetDay, billingMode,
		state.NetInTotalBytes, state.NetOutTotalBytes, counterSource, sampleTS.Unix(), receivedUnix)
}
