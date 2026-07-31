package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

func (s *sqliteNotificationDomain) QueueNotificationEvent(ctx context.Context, event notificationEvent, channels []notificationDispatchChannel) (bool, error) {
	if len(channels) == 0 {
		return false, nil
	}
	now := time.Now().UTC()
	event = notificationEventWithIdentity(event, now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { rollbackUnlessCommitted(tx) }()

	if shouldClaimStatusNotification(event) {
		claimed, err := claimStatusNotificationTx(ctx, tx, event)
		if err != nil || !claimed {
			return false, err
		}
	}

	if err := insertNotificationDeliveriesTx(ctx, tx, event, channels, now.Unix()); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	tx = nil
	return true, nil
}

func insertNotificationDeliveriesTx(ctx context.Context, tx *sql.Tx, event notificationEvent, channels []notificationDispatchChannel, nowUnix int64) error {
	event = notificationEventWithIdentity(event, time.Unix(nowUnix, 0).UTC())
	recoveryDelay, err := notificationRecoveryDelayTx(ctx, tx, event)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		channel = notificationChannelWithRoutingIdentity(channel)
		predecessorEventID := ""
		if strings.TrimSpace(event.NodeID) != "" {
			err := tx.QueryRowContext(ctx, `
				SELECT event_id
				FROM notification_deliveries
				WHERE channel_id = ? AND channel_version = ?
				  AND destination_fingerprint = ?
				  AND event_type = ? AND node_id = ?
				ORDER BY id DESC
				LIMIT 1
			`, channel.ID, channel.DeliveryVersion, channel.DestinationFingerprint,
				event.EventType, event.NodeID).Scan(&predecessorEventID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		if shouldClaimStatusNotification(event) {
			visibleStatus, visible, err := notificationVisibleStatusTx(ctx, tx, channel, event)
			if err != nil {
				return err
			}
			// Only the newest unsent state is useful. Keeping every failed flap in
			// causal order creates a burst when Telegram becomes reachable again.
			if _, err := tx.ExecContext(ctx, `
				UPDATE notification_deliveries
				SET state = 'canceled',
				    last_error = CASE
				      WHEN last_error = ? THEN 'delivery outcome unknown; superseded by newer status'
				      ELSE 'superseded by newer status'
				    END,
				    superseded_by_event_id = ?, lease_until = 0, claim_token = '', updated_at = ?
				WHERE channel_id = ? AND channel_version = ?
				  AND destination_fingerprint = ?
				  AND event_type = ? AND node_id = ?
				  AND state IN ('pending', 'paused', 'failed')
			`, notificationDeliveryOutcomeUnknownMessage, event.EventID, nowUnix, channel.ID, channel.DeliveryVersion,
				channel.DestinationFingerprint, event.EventType, event.NodeID); err != nil {
				return err
			}
			// User-visible state, not raw Agent transitions, decides whether a new
			// message is useful. This suppresses recovery-only messages when the
			// corresponding alert was never delivered and suppresses repeated alerts
			// while a delayed recovery is canceled by another flap.
			if !notificationStatusEventChangesVisibleState(event, visibleStatus, visible) {
				continue
			}
		}
		nextAttemptAt := nowUnix
		if notificationEventIsRecovery(event) {
			nextAttemptAt = time.Unix(nowUnix, 0).UTC().Add(recoveryDelay).Unix()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notification_deliveries (
				event_id, event_type, label, node_id, node_name, node_ip,
				previous_status, status, event_ts, detail,
				channel_id, channel_name, channel_version, destination_fingerprint,
				causal_predecessor_event_id, state, attempts, next_attempt_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?, ?)
		`, event.EventID, event.EventType, event.Label, event.NodeID, event.NodeName, event.NodeIP,
			event.PreviousStatus, event.Status, event.TS, event.Detail, channel.ID, channel.Name,
			channel.DeliveryVersion, channel.DestinationFingerprint, predecessorEventID,
			nextAttemptAt, nowUnix, nowUnix); err != nil {
			return err
		}
	}
	return nil
}

func notificationRecoveryDelayTx(ctx context.Context, tx *sql.Tx, event notificationEvent) (time.Duration, error) {
	if !notificationEventIsRecovery(event) {
		return 0, nil
	}
	var durationSeconds sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT MAX(ar.duration_sec)
		FROM alert_rules ar
		WHERE ar.enabled = 1
		  AND ar.notification_event_type = ?
		  AND (
		    NOT EXISTS (SELECT 1 FROM alert_rule_node_scopes scope_all WHERE scope_all.rule_id = ar.id)
		    OR EXISTS (SELECT 1 FROM alert_rule_node_scopes scope_node WHERE scope_node.rule_id = ar.id AND scope_node.node_id = ?)
		  )
	`, event.EventType, event.NodeID).Scan(&durationSeconds); err != nil {
		return 0, err
	}
	if !durationSeconds.Valid || durationSeconds.Int64 <= 0 {
		return 0, nil
	}
	return time.Duration(durationSeconds.Int64) * time.Second, nil
}

func notificationVisibleStatusTx(ctx context.Context, tx *sql.Tx, channel notificationDispatchChannel,
	event notificationEvent) (string, bool, error) {
	var status string
	err := tx.QueryRowContext(ctx, `
		SELECT status
		FROM notification_deliveries
		WHERE channel_id = ? AND channel_version = ?
		  AND destination_fingerprint = ?
		  AND event_type = ? AND node_id = ?
		  AND (
		    state = 'delivered' OR state = 'leased' OR
		    last_error = ? OR last_error LIKE 'delivery outcome unknown; superseded%'
		  )
		ORDER BY id DESC
		LIMIT 1
	`, channel.ID, channel.DeliveryVersion, channel.DestinationFingerprint,
		event.EventType, event.NodeID, notificationDeliveryOutcomeUnknownMessage).Scan(&status)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(status), true, nil
}

func notificationStatusEventChangesVisibleState(event notificationEvent, visibleStatus string, visible bool) bool {
	status := strings.TrimSpace(event.Status)
	if notificationEventIsRecovery(event) {
		return visible && visibleStatus == strings.TrimSpace(event.PreviousStatus)
	}
	return !visible || visibleStatus != status
}

func enabledNotificationChannelsTx(ctx context.Context, tx *sql.Tx) ([]notificationDispatchChannel, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, destination, delivery_version, destination_fingerprint
		FROM notification_channels
		WHERE enabled = 1 AND TRIM(destination) <> '' AND TRIM(credential) <> ''
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]notificationDispatchChannel, 0)
	for rows.Next() {
		var channel notificationDispatchChannel
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Destination,
			&channel.DeliveryVersion, &channel.DestinationFingerprint); err != nil {
			return nil, err
		}
		channel.Type = "telegram"
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func enabledNotificationChannelsForEventTx(ctx context.Context, tx *sql.Tx, eventType, nodeID string) (string, []notificationDispatchChannel, error) {
	label, ok := adminNotificationTypeLabel(eventType)
	if !ok {
		return "", nil, errNotificationTypeNotFound
	}
	enabled, err := notificationEventEnabledTx(ctx, tx, eventType, nodeID)
	if err != nil || !enabled {
		return label, nil, err
	}
	channels, err := enabledNotificationChannelsTx(ctx, tx)
	if err != nil {
		return "", nil, err
	}
	return label, channels, nil
}

func notificationEventEnabledTx(ctx context.Context, tx *sql.Tx, eventType, nodeID string) (bool, error) {
	var enabledRuleCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM alert_rules ar
		WHERE ar.notification_event_type = ?
		  AND ar.enabled = 1
		  AND (
		    ? = ''
		    OR NOT EXISTS (SELECT 1 FROM alert_rule_node_scopes scope_all WHERE scope_all.rule_id = ar.id)
		    OR EXISTS (SELECT 1 FROM alert_rule_node_scopes scope_node WHERE scope_node.rule_id = ar.id AND scope_node.node_id = ?)
		  )
	`, eventType, strings.TrimSpace(nodeID), strings.TrimSpace(nodeID)).Scan(&enabledRuleCount); err != nil {
		return false, err
	}
	return enabledRuleCount > 0, nil
}

func notificationDestinationFingerprint(channelType, destination string) string {
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if channelType == "" {
		channelType = "telegram"
	}
	sum := sha256.Sum256([]byte(channelType + "\x00" + strings.TrimSpace(destination)))
	return hex.EncodeToString(sum[:])
}

func notificationChannelWithRoutingIdentity(channel notificationDispatchChannel) notificationDispatchChannel {
	if channel.DeliveryVersion < 1 {
		channel.DeliveryVersion = 1
	}
	channel.DestinationFingerprint = notificationDestinationFingerprint(channel.Type, channel.Destination)
	return channel
}

func notificationEventWithIdentity(event notificationEvent, fallback time.Time) notificationEvent {
	if strings.TrimSpace(event.TS) == "" {
		event.TS = fallback.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(event.EventID) == "" {
		parts := []string{
			strings.TrimSpace(event.EventType),
			strings.TrimSpace(event.NodeID),
			strings.TrimSpace(event.NodeName),
			strings.TrimSpace(event.NodeIP),
			strings.TrimSpace(event.PreviousStatus),
			strings.TrimSpace(event.Status),
			strings.TrimSpace(event.TS),
			strings.TrimSpace(event.Detail),
		}
		sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
		event.EventID = hex.EncodeToString(sum[:16])
	}
	return event
}

func notificationEventIsRecovery(event notificationEvent) bool {
	return strings.TrimSpace(event.NodeID) != "" &&
		strings.TrimSpace(event.Status) == "online" &&
		strings.TrimSpace(event.PreviousStatus) != "" &&
		strings.TrimSpace(event.PreviousStatus) != "online"
}

func claimStatusNotificationTx(ctx context.Context, tx *sql.Tx, event notificationEvent) (bool, error) {
	eventType := strings.TrimSpace(event.EventType)
	nodeID := strings.TrimSpace(event.NodeID)
	status := strings.TrimSpace(event.Status)
	previousStatus := strings.TrimSpace(event.PreviousStatus)
	if eventType == "" || nodeID == "" || status == "" || previousStatus == status {
		return false, nil
	}
	mark := activeStatusNotificationMark(status)
	clearMark := recoveredStatusNotificationMark(status)
	if status == "online" && previousStatus != "" {
		mark = recoveredStatusNotificationMark(previousStatus)
		clearMark = activeStatusNotificationMark(previousStatus)
		if clearMark == "" {
			return false, nil
		}
		var activeIncident int
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM notification_event_marks
				WHERE event_type = ? AND node_id = ? AND mark = ?
			)
		`, eventType, nodeID, clearMark).Scan(&activeIncident); err != nil {
			return false, err
		}
		if activeIncident == 0 {
			return false, nil
		}
	}
	if mark == "" {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO notification_event_marks (event_type, node_id, mark, created_at)
		VALUES (?, ?, ?, ?)
	`, eventType, nodeID, mark, time.Now().UTC().Unix())
	if err != nil {
		return false, err
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if claimed > 0 && clearMark != "" {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM notification_event_marks
			WHERE event_type = ? AND node_id = ? AND mark = ?
		`, eventType, nodeID, clearMark); err != nil {
			return false, err
		}
	}
	return claimed > 0, nil
}
