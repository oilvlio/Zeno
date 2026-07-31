package api

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

func (s *SQLiteStore) PendingNotificationDeliveries(ctx context.Context, now time.Time, limit int) ([]queuedNotificationDelivery, error) {
	// Claim a single row so the 5s serial send timeout stays well within the 30s
	// lease. Continue past a bounded number of malformed rows: one bad encrypted
	// credential is isolated on the low-frequency retry cadence and cannot roll
	// back or block the next healthy channel in the claim scan.
	now = now.UTC()
	for scanned := 0; scanned < notificationDeliveryScanLimit; scanned++ {
		var delivery queuedNotificationDelivery
		var found, quarantined bool
		err := s.withAgentWrite(ctx, notificationOutboxWriteKey, func(writeCtx context.Context) error {
			var claimErr error
			delivery, found, quarantined, claimErr = s.claimNextNotificationDelivery(writeCtx, now)
			return claimErr
		})
		if err != nil {
			return nil, err
		}
		if found {
			return []queuedNotificationDelivery{delivery}, nil
		}
		if !quarantined {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("notification delivery quarantine scan limit reached")
}

func (s *SQLiteStore) claimNextNotificationDelivery(ctx context.Context, now time.Time) (queuedNotificationDelivery, bool, bool, error) {
	nowUnix := now.Unix()
	claimToken := notificationClaimToken(now)
	leaseUntil := now.Add(notificationDeliveryLease).Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	defer rollbackUnlessCommitted(tx)

	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'failed', attempts = attempts + 1,
		    next_attempt_at = ?, last_error = ?,
		    lease_until = 0, claim_token = '', updated_at = ?
		WHERE state = 'leased' AND lease_until <= ?
	`, notificationDeliveryManualRetryAtUnix, notificationDeliveryOutcomeUnknownMessage, nowUnix, nowUnix); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	// Legacy paused rows may resume only when their immutable routing binding is
	// still the currently enabled channel generation. Disabled, deleted, or
	// changed routes are canceled below and never silently rebound.
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'paused', lease_until = 0, claim_token = '',
		    last_error = 'notification channel disabled', updated_at = ?
		WHERE state = 'pending'
		  AND EXISTS (
		    SELECT 1 FROM notification_channels c
		    WHERE c.id = notification_deliveries.channel_id
		      AND c.enabled = 0
		      AND c.delivery_version = notification_deliveries.channel_version
		      AND c.destination_fingerprint = notification_deliveries.destination_fingerprint
		  )
	`, nowUnix); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'pending', next_attempt_at = MAX(next_attempt_at, ?),
		    last_error = '', updated_at = ?
		WHERE state = 'paused'
		  AND EXISTS (
		    SELECT 1 FROM notification_channels c
		    WHERE c.id = notification_deliveries.channel_id
		      AND c.enabled = 1
		      AND TRIM(c.destination) <> '' AND TRIM(c.credential) <> ''
		      AND c.delivery_version = notification_deliveries.channel_version
		      AND c.destination_fingerprint = notification_deliveries.destination_fingerprint
		  )
	`, nowUnix, nowUnix); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'canceled', lease_until = 0, claim_token = '',
		    last_error = 'notification channel changed or unavailable', updated_at = ?
		WHERE state IN ('pending', 'paused', 'failed')
		  AND NOT EXISTS (
		    SELECT 1 FROM notification_channels c
		    WHERE c.id = notification_deliveries.channel_id
		      AND c.delivery_version = notification_deliveries.channel_version
		      AND c.destination_fingerprint = notification_deliveries.destination_fingerprint
		      AND (
		        c.enabled = 0 OR
		        (TRIM(c.destination) <> '' AND TRIM(c.credential) <> '')
		      )
		  )
	`, nowUnix); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	// A newer status can arrive while an older delivery is leased. Once that
	// lease is released after a failure, cancel the obsolete row before it can be
	// retried and unblock the current state.
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries AS stale
		SET state = 'canceled', lease_until = 0, claim_token = '',
		    last_error = CASE
		      WHEN last_error = ? THEN 'delivery outcome unknown; superseded by newer status'
		      ELSE 'superseded by newer status'
		    END,
		    superseded_by_event_id = COALESCE((
		      SELECT newer.event_id
		      FROM notification_deliveries newer
		      WHERE newer.channel_id = stale.channel_id
		        AND newer.channel_version = stale.channel_version
		        AND newer.destination_fingerprint = stale.destination_fingerprint
		        AND newer.event_type = stale.event_type
		        AND newer.node_id = stale.node_id
		        AND newer.id > stale.id
		      ORDER BY newer.id DESC
		      LIMIT 1
		    ), ''),
		    updated_at = ?
		WHERE stale.state IN ('pending', 'paused', 'failed')
		  AND TRIM(stale.node_id) <> ''
		  AND TRIM(stale.previous_status) <> ''
		  AND TRIM(stale.status) <> ''
		  AND EXISTS (
		    SELECT 1
		    FROM notification_deliveries newer
		    WHERE newer.channel_id = stale.channel_id
		      AND newer.channel_version = stale.channel_version
		      AND newer.destination_fingerprint = stale.destination_fingerprint
		      AND newer.event_type = stale.event_type
		      AND newer.node_id = stale.node_id
		      AND newer.id > stale.id
		  )
	`, notificationDeliveryOutcomeUnknownMessage, nowUnix); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	// Re-evaluate the user-visible state after any older lease has resolved. This
	// closes the race where a newer opposite event was queued while the previous
	// request was in flight but that request then failed before being written.
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries AS current
		SET state = 'canceled', lease_until = 0, claim_token = '',
		    last_error = 'status suppressed because user-visible state did not change', updated_at = ?
		WHERE current.state IN ('pending', 'paused', 'failed')
		  AND TRIM(current.node_id) <> ''
		  AND TRIM(current.previous_status) <> ''
		  AND TRIM(current.status) <> ''
		  AND (
		    (
		      current.status = 'online' AND
		      COALESCE((
		        SELECT prior.status
		        FROM notification_deliveries prior
		        WHERE prior.channel_id = current.channel_id
		          AND prior.channel_version = current.channel_version
		          AND prior.destination_fingerprint = current.destination_fingerprint
		          AND prior.event_type = current.event_type
		          AND prior.node_id = current.node_id
		          AND prior.id < current.id
		          AND (
		            prior.state = 'delivered' OR
		            prior.last_error = ? OR
		            prior.last_error LIKE 'delivery outcome unknown; superseded%'
		          )
		        ORDER BY prior.id DESC
		        LIMIT 1
		      ), '') <> current.previous_status
		    ) OR (
		      current.status <> 'online' AND
		      COALESCE((
		        SELECT prior.status
		        FROM notification_deliveries prior
		        WHERE prior.channel_id = current.channel_id
		          AND prior.channel_version = current.channel_version
		          AND prior.destination_fingerprint = current.destination_fingerprint
		          AND prior.event_type = current.event_type
		          AND prior.node_id = current.node_id
		          AND prior.id < current.id
		          AND (
		            prior.state = 'delivered' OR
		            prior.last_error = ? OR
		            prior.last_error LIKE 'delivery outcome unknown; superseded%'
		          )
		        ORDER BY prior.id DESC
		        LIMIT 1
		      ), '') = current.status
		    )
		  )
	`, nowUnix, notificationDeliveryOutcomeUnknownMessage, notificationDeliveryOutcomeUnknownMessage); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	var deliveryID int64
	err = tx.QueryRowContext(ctx, `
		SELECT d.id
		FROM notification_deliveries d
		JOIN notification_channels c
		  ON c.id = d.channel_id
		 AND c.delivery_version = d.channel_version
		 AND c.destination_fingerprint = d.destination_fingerprint
		WHERE d.state IN ('pending', 'failed') AND d.next_attempt_at <= ?
		  AND c.enabled = 1
		  AND TRIM(c.destination) <> '' AND TRIM(c.credential) <> ''
		  AND (
		    TRIM(d.causal_predecessor_event_id) = '' OR NOT EXISTS (
		      SELECT 1 FROM notification_deliveries predecessor
		      WHERE predecessor.channel_id = d.channel_id
		        AND predecessor.channel_version = d.channel_version
		        AND predecessor.destination_fingerprint = d.destination_fingerprint
		        AND predecessor.event_id = d.causal_predecessor_event_id
		        AND predecessor.state IN ('pending', 'leased', 'paused', 'failed')
		    )
		  )
		ORDER BY d.next_attempt_at ASC, d.id ASC
		LIMIT 1
	`, nowUnix).Scan(&deliveryID)
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return queuedNotificationDelivery{}, false, false, err
		}
		tx = nil
		return queuedNotificationDelivery{}, false, false, nil
	}
	if err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'leased', lease_until = ?, claim_token = ?, updated_at = ?
		WHERE id = ? AND state IN ('pending', 'failed')
	`, leaseUntil, claimToken, nowUnix, deliveryID)
	if err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	if affected != 1 {
		return queuedNotificationDelivery{}, false, false, errNotificationDeliveryLeaseLost
	}

	var delivery queuedNotificationDelivery
	var storedCredential string
	err = tx.QueryRowContext(ctx, `
		SELECT d.id, d.event_id, d.event_type, d.label, d.node_id, d.node_name,
		       d.node_ip, d.previous_status, d.status, d.event_ts, d.detail,
		       d.channel_id, d.channel_name, d.channel_version,
		       d.destination_fingerprint, d.attempts, c.destination, c.credential
		FROM notification_deliveries d
		JOIN notification_channels c
		  ON c.id = d.channel_id
		 AND c.delivery_version = d.channel_version
		 AND c.destination_fingerprint = d.destination_fingerprint
		WHERE d.id = ? AND d.state = 'leased' AND d.claim_token = ?
		  AND c.enabled = 1
	`, deliveryID, claimToken).Scan(
		&delivery.ID, &delivery.Event.EventID, &delivery.Event.EventType,
		&delivery.Event.Label, &delivery.Event.NodeID, &delivery.Event.NodeName,
		&delivery.Event.NodeIP, &delivery.Event.PreviousStatus, &delivery.Event.Status,
		&delivery.Event.TS, &delivery.Event.Detail, &delivery.Channel.ID,
		&delivery.Channel.Name, &delivery.Channel.DeliveryVersion,
		&delivery.Channel.DestinationFingerprint, &delivery.Attempts,
		&delivery.Channel.Destination, &storedCredential,
	)
	if err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	credential, err := s.decryptNotificationCredentialFromStorage(delivery.Channel.ID, "telegram", storedCredential)
	if err != nil {
		attempts := delivery.Attempts + 1
		if attempts < notificationDeliveryMaxAttempts {
			attempts = notificationDeliveryMaxAttempts
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE notification_deliveries
			SET state = 'failed', attempts = ?, last_error = 'notification credential unavailable',
			    next_attempt_at = ?, lease_until = 0, claim_token = '', updated_at = ?
			WHERE id = ? AND state = 'leased' AND claim_token = ?
		`, attempts, now.Add(notificationDeliveryLongRetryDelay).Unix(), nowUnix, delivery.ID, claimToken)
		if updateErr != nil {
			return queuedNotificationDelivery{}, false, false, updateErr
		}
		if updateErr = requireOneNotificationDeliveryRow(result); updateErr != nil {
			return queuedNotificationDelivery{}, false, false, updateErr
		}
		if err := tx.Commit(); err != nil {
			return queuedNotificationDelivery{}, false, false, err
		}
		tx = nil
		log.Printf("notification outbox fetch failed-safe quarantine delivery_id=%s event_id=%s event_type=%s node_id=%s channel_id=%s error=credential_unavailable", notificationDeliveryStableID(delivery), notificationEventStableID(delivery.Event), delivery.Event.EventType, delivery.Event.NodeID, delivery.Channel.ID)
		return queuedNotificationDelivery{}, false, true, nil
	}
	delivery.Channel.Type = "telegram"
	delivery.Channel.Credential = credential
	delivery.ClaimToken = claimToken
	if err := tx.Commit(); err != nil {
		return queuedNotificationDelivery{}, false, false, err
	}
	tx = nil
	return delivery, true, false, nil
}

func notificationClaimToken(now time.Time) string {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err == nil {
		return fmt.Sprintf("claim:%d:%s", now.UnixNano(), hex.EncodeToString(random[:]))
	}
	return fmt.Sprintf("claim:%d", now.UnixNano())
}
