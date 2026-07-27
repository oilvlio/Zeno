package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type failingNotificationSender struct{}

func (failingNotificationSender) Send(context.Context, notificationDispatchChannel, notificationEvent) error {
	return errors.New("temporary network failure")
}

type notificationSenderFunc func(context.Context, notificationDispatchChannel, notificationEvent) error

func (fn notificationSenderFunc) Send(ctx context.Context, channel notificationDispatchChannel, event notificationEvent) error {
	return fn(ctx, channel, event)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestNotificationMessageTextUsesMaskedIPv4AndCompactStatusFormat(t *testing.T) {
	cases := []struct {
		name  string
		event notificationEvent
		want  string
	}{
		{
			name:  "offline",
			event: notificationEvent{EventType: "node_offline", NodeName: "Example Relay", NodeIP: "203.0.113.9", PreviousStatus: "online", Status: "offline"},
			want:  "🔴[离线] Example Relay(203.0.***.***)",
		},
		{
			name:  "offline recovery",
			event: notificationEvent{EventType: "node_offline", NodeName: "Example Relay", NodeIP: "203.0.113.9", PreviousStatus: "offline", Status: "online"},
			want:  "🟢[恢复] Example Relay(203.0.***.***)",
		},
		{
			name:  "cpu warning",
			event: notificationEvent{EventType: "probe_unhealthy", NodeName: "Example Relay", NodeIP: "203.0.113.9", PreviousStatus: "online", Status: "warning", Detail: "CPU持续占用过高"},
			want:  "⚠️[警告] Example Relay(203.0.***.***)CPU持续占用过高",
		},
		{
			name:  "cpu recovery",
			event: notificationEvent{EventType: "probe_unhealthy", NodeName: "Example Relay", NodeIP: "203.0.113.9", PreviousStatus: "warning", Status: "online", Detail: "CPU恢复正常"},
			want:  "🟢[恢复] Example Relay(203.0.***.***)CPU恢复正常",
		},
		{
			name:  "renewal due future",
			event: notificationEvent{EventType: "renewal_due", NodeName: "Example Harbor", Detail: "还有 1 天到期，2026-07-10"},
			want:  "⚠️[到期] Example Harbor 将于 1 天后（2026-7-10）到期",
		},
		{
			name:  "renewal due today",
			event: notificationEvent{EventType: "renewal_due", NodeName: "Example Harbor", Detail: "今天到期，2026-07-10"},
			want:  "⚠️[到期] Example Harbor 今天（2026-7-10）到期",
		},
		{
			name:  "renewal due expired",
			event: notificationEvent{EventType: "renewal_due", NodeName: "Example Harbor", Detail: "已过期 2 天，2026-07-10"},
			want:  "⚠️[到期] Example Harbor 已于 2 天前（2026-7-10）到期",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.messageText(); got != tt.want {
				t.Fatalf("messageText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDispatchAgentStatusNotificationDedupesActiveWarningsUntilRecovery(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	h := &handler{store: store, notificationSender: newHTTPNotificationSender(telegram.server.Client(), telegram.server.URL), liveHub: newLiveUpdateHub(), presence: newAgentPresenceManager()}
	warning := notificationStatusTransition{
		Previous: notificationNodeSnapshot{ID: "example-node-a", DisplayName: "Example Node A", Status: "online", PublicIPv4: "203.0.113.9"},
		Current:  notificationNodeSnapshot{ID: "example-node-a", DisplayName: "Example Node A", Status: "warning", PublicIPv4: "203.0.113.9"},
		Detail:   "CPU持续占用过高",
	}
	h.dispatchAgentStatusNotification(store, warning, time.Unix(1783491510, 0))
	h.dispatchAgentStatusNotification(store, warning, time.Unix(1783491513, 0))
	_, forms, errors := telegram.waitForCalls(t, 1)
	if len(errors) != 0 {
		t.Fatalf("telegram errors = %+v", errors)
	}
	if len(forms) != 1 || !strings.Contains(decodedTelegramText(forms[0]), "⚠️[警告]") {
		t.Fatalf("forms after duplicate warning = %+v, want one warning", forms)
	}
	time.Sleep(100 * time.Millisecond)
	_, forms, errors = telegram.waitForCalls(t, 1)
	if len(errors) != 0 || len(forms) != 1 {
		t.Fatalf("forms after duplicate settle = %+v errors=%+v, want still one warning", forms, errors)
	}

	recovery := notificationStatusTransition{
		Previous: notificationNodeSnapshot{ID: "example-node-a", DisplayName: "Example Node A", Status: "warning", PublicIPv4: "203.0.113.9"},
		Current:  notificationNodeSnapshot{ID: "example-node-a", DisplayName: "Example Node A", Status: "online", PublicIPv4: "203.0.113.9"},
		Detail:   "CPU恢复正常",
	}
	h.dispatchAgentStatusNotification(store, recovery, time.Unix(1783491600, 0))
	if _, err := store.db.ExecContext(ctx, `UPDATE notification_deliveries SET next_attempt_at = 0 WHERE status = 'online'`); err != nil {
		t.Fatalf("make recovery due: %v", err)
	}
	h.dispatchPendingNotificationDeliveries(ctx)
	_, forms, errors = telegram.waitForCalls(t, 2)
	if len(errors) != 0 || len(forms) != 2 || !strings.Contains(decodedTelegramText(forms[1]), "🟢[恢复]") {
		t.Fatalf("forms after recovery = %+v errors=%+v, want one recovery", forms, errors)
	}

	h.dispatchAgentStatusNotification(store, warning, time.Unix(1783491660, 0))
	_, forms, errors = telegram.waitForCalls(t, 3)
	if len(errors) != 0 || len(forms) != 3 || !strings.Contains(decodedTelegramText(forms[2]), "⚠️[警告]") {
		t.Fatalf("forms after new warning cycle = %+v errors=%+v, want warning allowed after recovery", forms, errors)
	}
}

func TestTelegramTimeoutAfterRequestWriteIsOutcomeUnknown(t *testing.T) {
	requestReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived <- struct{}{}
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	sender := newHTTPNotificationSender(&http.Client{Timeout: 20 * time.Millisecond}, server.URL)
	err := sender.Send(context.Background(), notificationDispatchChannel{
		Type: "telegram", Destination: "7579942307", Credential: "test-token",
	}, notificationEvent{EventType: "test_notification"})
	if !errors.Is(err, errNotificationDeliveryOutcomeUnknown) {
		t.Fatalf("send error=%v, want outcome unknown", err)
	}
	select {
	case <-requestReceived:
	default:
		t.Fatal("server did not receive written request")
	}
}

func TestTelegramConnectionFailureBeforeRequestWriteRemainsRetryable(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network is unreachable")
	})}
	sender := newHTTPNotificationSender(client, "https://api.telegram.invalid")
	err := sender.Send(context.Background(), notificationDispatchChannel{
		Type: "telegram", Destination: "7579942307", Credential: "test-token",
	}, notificationEvent{EventType: "test_notification"})
	if err == nil || errors.Is(err, errNotificationDeliveryOutcomeUnknown) {
		t.Fatalf("send error=%v, want retryable pre-write failure", err)
	}
	if got := sanitizeNotificationDeliveryError(err); got != "delivery connection failed" {
		t.Fatalf("sanitized error=%q", got)
	}
}

func TestNotificationOutcomeUnknownRequiresManualRetry(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", AgentToken: "token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops", Name: "Ops", Destination: "7579942307", Credential: "credential", Enabled: &enabled})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	queued, err := store.QueueNotificationEvent(ctx, notificationEvent{EventType: "test_notification", TS: time.Now().UTC().Format(time.RFC3339)}, []notificationDispatchChannel{{
		ID: channel.ID, Name: channel.Name, Destination: channel.Destination, Credential: "credential", Type: "telegram",
	}})
	if err != nil || !queued {
		t.Fatalf("queue event=%v err=%v", queued, err)
	}
	var calls atomic.Int32
	sender := notificationSenderFunc(func(context.Context, notificationDispatchChannel, notificationEvent) error {
		calls.Add(1)
		return fmt.Errorf("%w: deadline exceeded", errNotificationDeliveryOutcomeUnknown)
	})
	h := &handler{store: store, notificationSender: sender}
	h.dispatchPendingNotificationDeliveries(ctx)
	h.dispatchPendingNotificationDeliveries(ctx)

	var state, lastError string
	var attempts int
	var nextAttemptAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT state, attempts, next_attempt_at, last_error FROM notification_deliveries`).Scan(&state, &attempts, &nextAttemptAt, &lastError); err != nil {
		t.Fatalf("read delivery: %v", err)
	}
	if calls.Load() != 1 || state != "failed" || attempts != 1 || nextAttemptAt != notificationDeliveryManualRetryAtUnix || lastError != "delivery outcome unknown; automatic retry suppressed" {
		t.Fatalf("calls=%d state=%q attempts=%d next=%d error=%q", calls.Load(), state, attempts, nextAttemptAt, lastError)
	}
}

func TestNotificationOutboxPersistsFailureAndRetriesAfterRestart(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops", Name: "Ops", Destination: "7579942307", Credential: "super-secret-bot-token", Enabled: &enabled})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	event := notificationEvent{EventType: "node_offline", Label: "离线", NodeID: "example-node-a", NodeName: "Example Node A", PreviousStatus: "online", Status: "offline", TS: time.Now().UTC().Format(time.RFC3339)}
	queued, err := store.QueueNotificationEvent(ctx, event, []notificationDispatchChannel{{ID: channel.ID, Name: channel.Name, Destination: channel.Destination, Credential: "super-secret-bot-token", Type: "telegram"}})
	if err != nil || !queued {
		t.Fatalf("queue event = %v, %v", queued, err)
	}

	failing := &handler{store: store, notificationSender: failingNotificationSender{}}
	failing.dispatchPendingNotificationDeliveries(ctx)
	var state, lastError string
	var attempts int
	if err := store.db.QueryRowContext(ctx, `SELECT state, attempts, last_error FROM notification_deliveries ORDER BY id DESC LIMIT 1`).Scan(&state, &attempts, &lastError); err != nil {
		t.Fatalf("read failed delivery: %v", err)
	}
	if state != "pending" || attempts != 1 || lastError == "" || strings.Contains(lastError, "super-secret") {
		t.Fatalf("failed delivery = state %q attempts %d error %q", state, attempts, lastError)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE notification_deliveries SET next_attempt_at = 0`); err != nil {
		t.Fatalf("make delivery retryable: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	restarted := &handler{store: store, notificationSender: newHTTPNotificationSender(telegram.server.Client(), telegram.server.URL)}
	restarted.dispatchPendingNotificationDeliveries(ctx)
	_, forms, captureErrors := telegram.waitForCalls(t, 1)
	if len(captureErrors) != 0 || len(forms) != 1 {
		t.Fatalf("retry calls=%d errors=%v", len(forms), captureErrors)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT state, attempts, last_error FROM notification_deliveries ORDER BY id DESC LIMIT 1`).Scan(&state, &attempts, &lastError); err != nil {
		t.Fatalf("read delivered row: %v", err)
	}
	if state != "delivered" || attempts != 2 || lastError != "" {
		t.Fatalf("delivered row = state %q attempts %d error %q", state, attempts, lastError)
	}
}

func TestRenewalNotificationOutboxRetriesWithoutLosingRenewalMessage(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops", Name: "Ops", Destination: "7579942307", Credential: "super-secret-bot-token", Enabled: &enabled})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	event := notificationEvent{EventType: "renewal_due", Label: "续费", NodeID: "example-node-a", NodeName: "Example Node A", Status: "renewal_due", TS: time.Now().UTC().Format(time.RFC3339), Detail: "还有 3 天到期，2026-07-14"}
	queued, err := store.QueueNotificationEvent(ctx, event, []notificationDispatchChannel{{ID: channel.ID, Name: channel.Name, Destination: channel.Destination, Credential: "super-secret-bot-token", Type: "telegram"}})
	if err != nil || !queued {
		t.Fatalf("queue renewal event = %v, %v", queued, err)
	}

	failing := &handler{store: store, notificationSender: failingNotificationSender{}}
	failing.dispatchPendingNotificationDeliveries(ctx)
	if _, err := store.db.ExecContext(ctx, `UPDATE notification_deliveries SET next_attempt_at = 0 WHERE event_type = 'renewal_due'`); err != nil {
		t.Fatalf("make renewal delivery retryable: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	restarted := &handler{store: store, notificationSender: newHTTPNotificationSender(telegram.server.Client(), telegram.server.URL)}
	restarted.dispatchPendingNotificationDeliveries(ctx)
	_, forms, captureErrors := telegram.waitForCalls(t, 1)
	if len(captureErrors) != 0 || len(forms) != 1 {
		t.Fatalf("renewal retry calls=%d errors=%v", len(forms), captureErrors)
	}
	messageText := decodedTelegramText(forms[0])
	if !strings.Contains(messageText, "⚠️[到期]") || !strings.Contains(messageText, "3 天后") || !strings.Contains(messageText, "2026-7-14") {
		t.Fatalf("renewal retry text = %q", messageText)
	}
	var state string
	var attempts int
	if err := store.db.QueryRowContext(ctx, `SELECT state, attempts FROM notification_deliveries WHERE event_type = 'renewal_due' ORDER BY id DESC LIMIT 1`).Scan(&state, &attempts); err != nil {
		t.Fatalf("read renewal delivery: %v", err)
	}
	if state != "delivered" || attempts != 2 {
		t.Fatalf("renewal delivery state=%q attempts=%d, want delivered/2", state, attempts)
	}
}

func decodedTelegramText(form string) string {
	values, err := url.ParseQuery(form)
	if err != nil {
		return form
	}
	return values.Get("text")
}
