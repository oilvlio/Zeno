package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"
)

func (h *handler) runNotificationOutboxLoop(ctx context.Context, interval time.Duration, worker *notificationOutboxWorker) {
	defer func() {
		worker.mu.Lock()
		worker.running = false
		pending := len(worker.wake) > 0
		worker.mu.Unlock()
		if pending && h.backgroundContext().Err() == nil {
			h.ensureNotificationOutboxWorker(interval)
		}
	}()
	if interval <= 0 {
		for {
			// The wake that created this worker is already represented by the
			// dispatch below. Drain it first so one event does not cause two scans.
			select {
			case <-worker.wake:
			default:
			}
			h.dispatchPendingNotificationDeliveries(ctx)
			idle := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				if !idle.Stop() {
					<-idle.C
				}
				return
			case <-worker.wake:
				if !idle.Stop() {
					<-idle.C
				}
				continue
			case <-idle.C:
				return
			}
		}
	}
	h.dispatchPendingNotificationDeliveries(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-worker.wake:
			h.dispatchPendingNotificationDeliveries(ctx)
		case <-ticker.C:
			h.dispatchPendingNotificationDeliveries(ctx)
		}
	}
}

func (h *handler) dispatchPendingNotificationDeliveries(ctx context.Context) {
	store, ok := h.store.(notificationOutboxStore)
	if !ok || h.notificationSender == nil {
		return
	}
	if ctx.Err() != nil || !h.automaticNotificationsAllowed() {
		return
	}
	h.notificationDrainMu.Lock()
	defer h.notificationDrainMu.Unlock()
	for drained := 0; drained < notificationDeliveryScanLimit; drained++ {
		deliveries, err := store.PendingNotificationDeliveries(ctx, time.Now().UTC(), notificationDeliveryBatchSize)
		if err != nil {
			log.Printf("notification outbox fetch failed: %v", err)
			return
		}
		if len(deliveries) == 0 {
			return
		}
		// The SQLite implementation returns one row because that is the lease
		// safety contract. Iterate defensively for test/alternate stores that may
		// return more than the requested limit.
		for _, delivery := range deliveries {
			if ctx.Err() != nil {
				return
			}
			var sendErr error
			if delivery.Channel.Credential == "" || delivery.Channel.Destination == "" {
				sendErr = context.Canceled
			} else {
				// Persist the handoff boundary before invoking the sender. A crash while
				// the row is merely claimed is safe to retry; after this point the
				// process may have written the request and must suppress an automatic
				// duplicate when the outcome cannot be observed.
				if err := store.MarkNotificationDeliveryRequestStarted(ctx, delivery, time.Now().UTC()); err != nil {
					log.Printf("notification outbox start marker failed delivery_id=%s event_id=%s event_type=%s node_id=%s channel_id=%s: %v", notificationDeliveryStableID(delivery), notificationEventStableID(delivery.Event), delivery.Event.EventType, delivery.Event.NodeID, delivery.Channel.ID, err)
					return
				}
				sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				sendErr = h.notificationSender.Send(sendCtx, delivery.Channel, delivery.Event)
				cancel()
			}
			// Recording the outcome is the acknowledgement boundary. A crash after
			// request start leaves the delivery for manual review because Telegram has
			// no idempotency key and may already have accepted the stable event_id.
			if err := store.RecordNotificationDeliveryAttempt(ctx, delivery, sendErr, time.Now().UTC()); err != nil {
				log.Printf("notification outbox update failed delivery_id=%s event_id=%s event_type=%s node_id=%s channel_id=%s: %v", notificationDeliveryStableID(delivery), notificationEventStableID(delivery.Event), delivery.Event.EventType, delivery.Event.NodeID, delivery.Channel.ID, err)
				// Do not tight-loop a delivery whose acknowledgement failed. Its lease
				// provides the retry boundary and prevents an immediate duplicate.
				return
			}
			if sendErr != nil {
				log.Printf("notification delivery failed delivery_id=%s event_id=%s event_type=%s node_id=%s channel_id=%s attempt=%d error=%s", notificationDeliveryStableID(delivery), notificationEventStableID(delivery.Event), delivery.Event.EventType, delivery.Event.NodeID, delivery.Channel.ID, delivery.Attempts+1, sanitizeNotificationDeliveryError(sendErr))
			}
		}
	}
	// Keep draining without leasing a serial batch whose tail could expire.
	h.wakeNotificationOutbox()
}

func notificationEventStableID(event notificationEvent) string {
	if eventID := strings.TrimSpace(event.EventID); eventID != "" {
		return eventID
	}
	parts := []string{
		strings.TrimSpace(event.EventType),
		strings.TrimSpace(event.NodeID),
		strings.TrimSpace(event.TS),
		strings.TrimSpace(event.PreviousStatus),
		strings.TrimSpace(event.Status),
		strings.TrimSpace(event.Detail),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func notificationDeliveryStableID(delivery queuedNotificationDelivery) string {
	if delivery.ID > 0 {
		return fmt.Sprintf("outbox:%d", delivery.ID)
	}
	parts := []string{notificationEventStableID(delivery.Event), strings.TrimSpace(delivery.Channel.ID)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "outbox:" + hex.EncodeToString(sum[:8])
}
