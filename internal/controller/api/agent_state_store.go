package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"time"
)

func (s *SQLiteStore) InsertAgentState(ctx context.Context, nodeID string, state AgentStateRequest) error {
	if err := s.ensureTelemetryStorage(); err != nil {
		return err
	}
	return s.withAgentWrite(ctx, nodeID, func(ctx context.Context) error {
		return s.insertAgentStateOnce(ctx, nodeID, state)
	})
}

func (s *SQLiteStore) insertAgentStateOnce(ctx context.Context, nodeID string, state AgentStateRequest) error {
	now := time.Now().UTC()
	nowUnix := now.Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	if _, err := insertAgentStateSampleTx(ctx, tx, nodeID, state, now, false); err != nil {
		return err
	}
	if err := updateAgentLivenessOnlyTx(ctx, tx, nodeID, nowUnix); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *SQLiteStore) RecordAgentStateReport(ctx context.Context, nodeID string, state AgentStateRequest) (bool, notificationStatusTransition, error) {
	if err := s.ensureTelemetryStorage(); err != nil {
		return false, notificationStatusTransition{}, err
	}
	var accepted bool
	var transition notificationStatusTransition
	err := s.withAgentWrite(ctx, nodeID, func(ctx context.Context) error {
		var err error
		accepted, transition, err = s.recordAgentStateReportOnce(ctx, nodeID, state)
		return err
	})
	return accepted, transition, err
}

func (s *SQLiteStore) recordAgentStateReportOnce(ctx context.Context, nodeID string, state AgentStateRequest) (bool, notificationStatusTransition, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, notificationStatusTransition{}, err
	}
	defer func() { rollbackUnlessCommitted(tx) }()

	accepted, err := insertAgentStateSampleTx(ctx, tx, nodeID, state, now, true)
	if err != nil {
		return false, notificationStatusTransition{}, err
	}
	if !accepted {
		if err := updateAgentLivenessOnlyTx(ctx, tx, nodeID, now.Unix()); err != nil {
			return false, notificationStatusTransition{}, err
		}
		if err := tx.Commit(); err != nil {
			return false, notificationStatusTransition{}, err
		}
		tx = nil
		return false, notificationStatusTransition{}, nil
	}

	transition, err := recordAgentStateAlertRuleTransitionTx(ctx, tx, nodeID, time.Unix(state.TS, 0).UTC(), state)
	if err != nil {
		return false, notificationStatusTransition{}, err
	}
	if err := queueStatusTransitionNotificationTx(ctx, tx, transition, time.Unix(state.TS, 0).UTC()); err != nil {
		return false, notificationStatusTransition{}, err
	}
	if err := tx.Commit(); err != nil {
		return false, notificationStatusTransition{}, err
	}
	tx = nil
	return true, transition, nil
}

func insertAgentStateSampleTx(ctx context.Context, tx *sql.Tx, nodeID string, state AgentStateRequest, receivedAt time.Time, enforceRateLimit bool) (bool, error) {
	receivedUnix := receivedAt.UTC().Unix()
	sampleTS := time.Unix(state.TS, 0).UTC()
	sampleID := strings.TrimSpace(state.effectiveSampleID())
	if sampleID != "" && !validAgentStateSampleID(sampleID) {
		return false, errInvalidAgentStateReport
	}
	payloadHash, err := agentStatePayloadHash(state)
	if err != nil {
		return false, errInvalidAgentStateReport
	}
	if err := lockAgentNodeWriteTx(ctx, tx, nodeID); err != nil {
		return false, err
	}
	if sampleID != "" {
		var existingHash string
		err := tx.QueryRowContext(ctx, agentStateSampleLookupSQL, nodeID, sampleID).Scan(&existingHash)
		if err != nil && err != sql.ErrNoRows {
			return false, err
		}
		if err == nil {
			if existingHash == payloadHash || existingHash == "" {
				return false, nil
			}
			return false, errInvalidAgentStateReport
		}
	} else {
		var existingID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM state_samples
			WHERE node_id = ? AND ts = ? AND payload_hash = ?
			LIMIT 1
		`, nodeID, state.TS, payloadHash).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return false, err
		}
		if err == nil {
			return false, nil
		}
	}
	if enforceRateLimit {
		var lastSampleTS sql.NullInt64
		if err := tx.QueryRowContext(ctx, `
			SELECT ts
			FROM state_samples
			WHERE node_id = ?
			ORDER BY ts DESC, id DESC
			LIMIT 1
		`, nodeID).Scan(&lastSampleTS); err != nil && err != sql.ErrNoRows {
			return false, err
		}
		minIntervalSec := int64(minAgentStateReportInterval / time.Second)
		if minIntervalSec < 1 {
			minIntervalSec = 1
		}
		if lastSampleTS.Valid && state.TS <= lastSampleTS.Int64 {
			return false, nil
		}
		if lastSampleTS.Valid && state.TS-lastSampleTS.Int64 < minIntervalSec {
			return false, nil
		}
	}

	// Invalid optional collector groups are unknown, not zero. Persist NULL so
	// public summaries and history do not present a failed collection as a real
	// counter value. Traffic accounting independently refuses to advance its
	// baseline unless net_totals_valid is true (or absent for legacy agents).
	var netInTotalBytes any = state.NetInTotalBytes
	var netOutTotalBytes any = state.NetOutTotalBytes
	if state.NetTotalsValid != nil && !*state.NetTotalsValid {
		netInTotalBytes = nil
		netOutTotalBytes = nil
	}
	var tcpConnectionCount any = state.TCPConnectionCount
	var udpConnectionCount any = state.UDPConnectionCount
	if state.ConnectionCountsValid != nil && !*state.ConnectionCountsValid {
		tcpConnectionCount = nil
		udpConnectionCount = nil
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO state_samples (
			node_id, sample_id, payload_hash, received_at, ts, cpu_percent, load1, load5, load15,
			memory_used_bytes, memory_total_bytes, swap_used_bytes, swap_total_bytes,
			disk_used_bytes, disk_total_bytes, net_in_total_bytes, net_out_total_bytes,
			net_in_speed_bps, net_out_speed_bps, process_count, tcp_connection_count, udp_connection_count, uptime_seconds
		)
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nodeID, sampleID, payloadHash, receivedUnix, state.TS, state.CPUPercent, state.Load1, state.Load5, state.Load15, state.MemoryUsedBytes, state.MemoryTotalBytes, state.SwapUsedBytes, state.SwapTotalBytes, state.DiskUsedBytes, state.DiskTotalBytes, netInTotalBytes, netOutTotalBytes, state.NetInSpeedBps, state.NetOutSpeedBps, state.ProcessCount, tcpConnectionCount, udpConnectionCount, state.UptimeSeconds); err != nil {
		return false, err
	}

	var billingMode string
	var monthlyResetDay int
	var billingEpoch int64
	if err := tx.QueryRowContext(ctx, `SELECT billing_mode, monthly_reset_day, billing_traffic_epoch FROM nodes WHERE id = ? AND disabled = 0`, nodeID).Scan(&billingMode, &monthlyResetDay, &billingEpoch); err != nil {
		if err == sql.ErrNoRows {
			return false, errNodeNotFound
		}
		return false, err
	}
	month := billingPeriodKey(sampleTS, monthlyResetDay)
	// A failed platform counter read is not a real zero sample. Skipping the
	// billing baseline here is essential for first-sample failures: otherwise a
	// later recovery would bill the machine's full lifetime counter as new use.
	if state.NetTotalsValid == nil || *state.NetTotalsValid {
		if err := upsertLifetimeTraffic(ctx, tx, nodeID, state.NetInTotalBytes, state.NetOutTotalBytes, strings.TrimSpace(state.NetCounterSource), sampleTS.Unix(), receivedUnix); err != nil {
			return false, err
		}
		if err := upsertMonthlyTraffic(ctx, tx, nodeID, month, billingEpoch, monthlyResetDay, billingMode, state.NetInTotalBytes, state.NetOutTotalBytes, strings.TrimSpace(state.NetCounterSource), sampleTS.Unix(), receivedUnix); err != nil {
			return false, err
		}
	}
	return true, nil
}

const agentStateSampleLookupSQL = `
	SELECT payload_hash
	FROM state_samples INDEXED BY idx_state_samples_node_sample_id
	WHERE node_id = ?
	  AND sample_id = ?
	  AND sample_id IS NOT NULL
	  AND sample_id <> ''
	LIMIT 1
`

func lockAgentNodeWriteTx(ctx context.Context, tx *sql.Tx, nodeID string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE nodes
		SET updated_at = updated_at
		WHERE id = ? AND disabled = 0
	`, nodeID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errNodeNotFound
	}
	return nil
}

func updateAgentLivenessOnlyTx(ctx context.Context, tx *sql.Tx, nodeID string, nowUnix int64) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE nodes
		SET status = CASE WHEN status IN ('warning', 'offline') THEN status ELSE 'online' END,
		    last_seen_at = CASE WHEN last_seen_at IS NULL OR last_seen_at <= ? THEN ? ELSE last_seen_at END,
		    updated_at = CASE WHEN updated_at <= ? THEN ? ELSE updated_at END
		WHERE id = ? AND disabled = 0
	`, nowUnix, nowUnix, nowUnix, nowUnix, nodeID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errNodeNotFound
	}
	return nil
}

func agentStatePayloadHash(state AgentStateRequest) (string, error) {
	copy := state
	copy.SampleID = ""
	copy.IdempotencyKey = ""
	payload, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeAgentPublicIP(value string, family int) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	if family == 4 {
		ipv4 := ip.To4()
		if ipv4 == nil {
			return ""
		}
		return ipv4.String()
	}
	if family == 6 {
		if ip.To4() != nil || ip.To16() == nil {
			return ""
		}
		return ip.String()
	}
	return ""
}

func normalizeAgentCountryCode(value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	if len(trimmed) != 2 {
		return ""
	}
	for _, r := range trimmed {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return trimmed
}
