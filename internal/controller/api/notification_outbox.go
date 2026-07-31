package api

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	// One leased delivery is sent at a time. The sender timeout is 5s and the
	// lease is 30s, so serial batch duration cannot consume another row's lease.
	// This also keeps the claim model safe when multiple controller instances are
	// introduced later.
	notificationDeliveryBatchSize   = 1
	notificationDeliveryScanLimit   = 32
	notificationDeliveryMaxAttempts = 5
	notificationDeliveryLease       = 30 * time.Second
	// An ambiguous Telegram attempt may already have delivered. Keep it visible
	// as failed for manual review but never retry it automatically.
	notificationDeliveryManualRetryAtUnix int64 = 253402300799
	// Deliveries that exhaust the fast retry budget remain retryable at a low
	// frequency. They intentionally stay in the failed state between attempts so
	// operators can identify and manually retry them without creating a tight
	// retry loop during a prolonged Telegram outage.
	notificationDeliveryLongRetryDelay = 6 * time.Hour
)

var errNotificationDeliveryLeaseLost = errors.New("notification delivery lease lost")

type queuedNotificationDelivery struct {
	ID         int64
	Event      notificationEvent
	Channel    notificationDispatchChannel
	Attempts   int
	ClaimToken string
}

type notificationOutboxStore interface {
	QueueNotificationEvent(ctx context.Context, event notificationEvent, channels []notificationDispatchChannel) (bool, error)
	PendingNotificationDeliveries(ctx context.Context, now time.Time, limit int) ([]queuedNotificationDelivery, error)
	RecordNotificationDeliveryAttempt(ctx context.Context, delivery queuedNotificationDelivery, sendErr error, now time.Time) error
}

type notificationOutboxWorker struct {
	wake    chan struct{}
	mu      sync.Mutex
	running bool
}

func (h *handler) notificationOutboxWorker() *notificationOutboxWorker {
	if h == nil {
		return nil
	}
	h.notificationWorkerMu.Lock()
	defer h.notificationWorkerMu.Unlock()
	if h.backgroundCtx == nil {
		h.backgroundCtx, h.backgroundCancel = context.WithCancel(context.Background())
	}
	if h.notificationWorker == nil {
		h.notificationWorker = &notificationOutboxWorker{wake: make(chan struct{}, 1)}
	}
	return h.notificationWorker
}

func (h *handler) wakeNotificationOutbox() {
	worker := h.notificationOutboxWorker()
	if worker == nil {
		return
	}
	select {
	case worker.wake <- struct{}{}:
	default:
	}
	h.ensureNotificationOutboxWorker(0)
}

func (h *handler) ensureNotificationOutboxWorker(interval time.Duration) {
	worker := h.notificationOutboxWorker()
	if worker == nil || h.backgroundContext().Err() != nil {
		return
	}
	worker.mu.Lock()
	if worker.running {
		worker.mu.Unlock()
		return
	}
	worker.running = true
	worker.mu.Unlock()
	h.startBackground(func(ctx context.Context) { h.runNotificationOutboxLoop(ctx, interval, worker) })
}

// QueueNotificationEvent persists both the incident claim and its deliveries in
// one transaction. A controller crash after this point cannot silently lose the
// notification; the outbox worker resumes it after restart.
