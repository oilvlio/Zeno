package api

import (
	"context"
	"database/sql"
	"strings"
)

// Probe-target selection helpers for admin node updates. They run inside the
// caller's transaction so selection checks and the writes they guard cannot be
// separated by a concurrent target deletion.

// verifyAdminNodeProbeSelectionTx rejects updates that reference probe targets
// which are missing or already queued for deletion, and rejects a home target
// that is not part of the selection being written.
func verifyAdminNodeProbeSelectionTx(ctx context.Context, tx *sql.Tx, update AdminNodeUpdateRequest) error {
	selectedTargetIDs := make(map[string]struct{}, len(update.ProbeTargetIDs))
	for _, targetID := range update.ProbeTargetIDs {
		if err := requireActiveProbeTargetTx(ctx, tx, targetID); err != nil {
			return err
		}
		selectedTargetIDs[targetID] = struct{}{}
	}
	if update.HomeProbeTargetID == nil || *update.HomeProbeTargetID == "" {
		return nil
	}
	if err := requireActiveProbeTargetTx(ctx, tx, *update.HomeProbeTargetID); err != nil {
		return err
	}
	if update.ProbeTargetIDs == nil {
		return nil
	}
	if _, selected := selectedTargetIDs[*update.HomeProbeTargetID]; !selected {
		return errInvalidAdminNodeUpdate
	}
	return nil
}

func requireActiveProbeTargetTx(ctx context.Context, tx *sql.Tx, targetID string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, activeAdminProbeTargetExistsSQL, targetID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return errInvalidAdminNodeUpdate
		}
		return err
	}
	return nil
}

// replaceAdminNodeProbeAssignmentsTx makes the node's enabled assignments match
// targetIDs exactly. Rows are disabled rather than deleted so historical probe
// data keeps its foreign key.
func replaceAdminNodeProbeAssignmentsTx(ctx context.Context, tx *sql.Tx, nodeID string, targetIDs []string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE node_probe_targets SET enabled = 0 WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	for _, targetID := range targetIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO node_probe_targets (node_id, target_id, enabled)
			VALUES (?, ?, 1)
			ON CONFLICT(node_id, target_id) DO UPDATE SET enabled = 1
		`, nodeID, targetID); err != nil {
			return err
		}
	}
	return nil
}

// clearUnselectedHomeProbeTargetTx drops a home target the caller did not set
// explicitly once it falls outside the new selection, preventing a node from
// pointing at a target it no longer probes.
func clearUnselectedHomeProbeTargetTx(ctx context.Context, tx *sql.Tx, nodeID string, targetIDs []string) error {
	if len(targetIDs) == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE nodes SET home_probe_target_id = NULL WHERE id = ?`, nodeID)
		return err
	}
	placeholders := make([]string, len(targetIDs))
	args := make([]any, 0, len(targetIDs)+1)
	args = append(args, nodeID)
	for index, targetID := range targetIDs {
		placeholders[index] = "?"
		args = append(args, targetID)
	}
	query := `UPDATE nodes SET home_probe_target_id = NULL WHERE id = ? AND home_probe_target_id IS NOT NULL AND home_probe_target_id NOT IN (` +
		strings.Join(placeholders, ",") + `)`
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
