package api

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *SQLiteStore) RecordNotificationDeliveryAttempt(ctx context.Context, delivery queuedNotificationDelivery, sendErr error, now time.Time) error {
	return s.withAgentWrite(ctx, notificationOutboxWriteKey, func(writeCtx context.Context) error {
		return s.recordNotificationDeliveryAttemptOnce(writeCtx, delivery, sendErr, now)
	})
}

func (s *SQLiteStore) recordNotificationDeliveryAttemptOnce(ctx context.Context, delivery queuedNotificationDelivery, sendErr error, now time.Time) error {
	nowUnix := now.UTC().Unix()
	if sendErr == nil {
		result, err := s.db.ExecContext(ctx, `
			UPDATE notification_deliveries
			SET state = 'delivered', attempts = attempts + 1, last_error = '',
			    lease_until = 0, claim_token = '', updated_at = ?, delivered_at = ?
			WHERE id = ? AND state = 'leased' AND claim_token = ?
		`, nowUnix, nowUnix, delivery.ID, delivery.ClaimToken)
		if err != nil {
			return err
		}
		return requireOneNotificationDeliveryRow(result)
	}

	attempts := delivery.Attempts + 1
	state := "pending"
	nextAttemptAt := now.Add(notificationRetryDelay(attempts)).UTC().Unix()
	if errors.Is(sendErr, errNotificationDeliveryOutcomeUnknown) {
		state = "failed"
		nextAttemptAt = notificationDeliveryManualRetryAtUnix
	} else if attempts >= notificationDeliveryMaxAttempts {
		state = "failed"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	result, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = ?, attempts = ?, next_attempt_at = ?, last_error = ?, lease_until = 0, claim_token = '', updated_at = ?
		WHERE id = ? AND state = 'leased' AND claim_token = ?
	`, state, attempts, nextAttemptAt, sanitizeNotificationDeliveryError(sendErr), nowUnix, delivery.ID, delivery.ClaimToken)
	if err != nil {
		return err
	}
	if err := requireOneNotificationDeliveryRow(result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func requireOneNotificationDeliveryRow(result sql.Result) error {
	if result == nil {
		return errNotificationDeliveryLeaseLost
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errNotificationDeliveryLeaseLost
	}
	return nil
}

func (s *SQLiteStore) RetryFailedNotificationDelivery(ctx context.Context, deliveryID int64, now time.Time) error {
	if deliveryID <= 0 {
		return errNotificationDeliveryNotFound
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET state = 'pending', next_attempt_at = ?, lease_until = 0, claim_token = '', last_error = '', updated_at = ?
		WHERE id = ? AND state = 'failed'
	`, now.UTC().Unix(), now.UTC().Unix(), deliveryID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var state string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM notification_deliveries WHERE id = ?`, deliveryID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotificationDeliveryNotFound
		}
		return err
	}
	return errNotificationDeliveryNotFailed
}

func notificationRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Second
	case 2:
		return 5 * time.Second
	case 3:
		return 30 * time.Second
	case 4:
		return 2 * time.Minute
	default:
		return notificationDeliveryLongRetryDelay
	}
}
