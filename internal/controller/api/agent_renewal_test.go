package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRenewalNotificationScannerDispatchesOnlyOnConfiguredLeadDay(t *testing.T) {
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
	// Anchor at the current UTC day's start so the "same day" assertion below
	// cannot cross a calendar boundary and the outbox due time is never future.
	now := dateOnlyUTC(time.Now().UTC())
	expiryDate := now.Add(3 * 24 * time.Hour).Format("2006-01-02")
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &expiryDate}); err != nil {
		t.Fatalf("set expiry date: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enable renewal_due alert rule: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	h := &handler{store: store, notificationSender: newHTTPNotificationSender(telegram.server.Client(), telegram.server.URL)}
	if queued := h.queueDueRenewalNotifications(ctx, now); queued != 1 {
		t.Fatalf("first renewal scan queued %d deliveries, want 1", queued)
	}
	paths, forms, errors := telegram.waitForCalls(t, 1)
	if len(errors) != 0 {
		t.Fatalf("telegram handler errors = %+v", errors)
	}
	messageText := ""
	if len(forms) == 1 {
		messageText = decodedTelegramText(forms[0])
	}
	if len(paths) != 1 || len(forms) != 1 || !strings.Contains(messageText, "⚠️[到期]") || !strings.Contains(messageText, formatRenewalMessageDate(expiryDate)) {
		t.Fatalf("telegram request paths=%+v forms=%+v, want one renewal due notification", paths, forms)
	}
	assertTelegramFormsDoNotLeakCredential(t, forms, "telegram-bot-credential-value")

	if queued := h.queueDueRenewalNotifications(ctx, now.Add(time.Minute)); queued != 0 {
		t.Fatalf("same-day duplicate renewal scan queued %d deliveries, want 0", queued)
	}
	paths, forms, errors = telegram.waitForCalls(t, 1)
	if len(errors) != 0 {
		t.Fatalf("telegram handler errors after duplicate scan = %+v", errors)
	}
	if len(paths) != 1 || len(forms) != 1 {
		t.Fatalf("telegram calls after duplicate scan paths=%+v forms=%+v, want still one renewal notification", paths, forms)
	}

	if queued := h.queueDueRenewalNotifications(ctx, now.Add(24*time.Hour)); queued != 0 {
		t.Fatalf("day after configured reminder queued %d deliveries, want 0", queued)
	}
	var deliveryCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE event_type = 'renewal_due'`).Scan(&deliveryCount); err != nil {
		t.Fatalf("count renewal deliveries after configured reminder day: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("renewal delivery count after configured reminder day = %d, want 1", deliveryCount)
	}
	var markCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_event_marks WHERE event_type = 'renewal_due'`).Scan(&markCount); err != nil {
		t.Fatalf("count renewal marks after configured reminder day: %v", err)
	}
	if markCount != 1 {
		t.Fatalf("renewal mark count after configured reminder day = %d, want 1", markCount)
	}
}
func TestRenewalNotificationScannerDispatchesRecurringBillingCycle(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Harbor", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	cycleDueDate := dateOnlyUTC(now).AddDate(0, 0, 1)
	finalExpiryDate := addMonthsFromAnchorClampedUTC(cycleDueDate, 1).Format("2006-01-02")
	billingCycle := "月"
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &finalExpiryDate, BillingCycle: &billingCycle}); err != nil {
		t.Fatalf("set recurring expiry date: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	threshold := 1.0
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled, Threshold: &threshold}); err != nil {
		t.Fatalf("enable renewal_due alert rule: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	h := &handler{store: store, notificationSender: newHTTPNotificationSender(telegram.server.Client(), telegram.server.URL)}
	if queued := h.queueDueRenewalNotifications(ctx, now); queued != 1 {
		t.Fatalf("recurring renewal scan queued %d deliveries, want 1", queued)
	}
	paths, forms, errors := telegram.waitForCalls(t, 1)
	if len(errors) != 0 {
		t.Fatalf("telegram handler errors = %+v", errors)
	}
	cycleDueText := cycleDueDate.Format("2006-01-02")
	messageText := ""
	if len(forms) == 1 {
		messageText = decodedTelegramText(forms[0])
	}
	if len(paths) != 1 || len(forms) != 1 || !strings.Contains(messageText, "⚠️[到期]") || !strings.Contains(messageText, formatRenewalMessageDate(cycleDueText)) {
		t.Fatalf("telegram request paths=%+v forms=%+v, want renewal due notification for recurring billing date %s", paths, forms, cycleDueText)
	}
	if strings.Contains(messageText, finalExpiryDate) || strings.Contains(messageText, formatRenewalMessageDate(finalExpiryDate)) {
		t.Fatalf("telegram text %q used final expiry date %s, want recurring billing date %s", messageText, finalExpiryDate, cycleDueText)
	}
	if queued := h.queueDueRenewalNotifications(ctx, cycleDueDate.Add(12*time.Hour)); queued != 0 {
		t.Fatalf("recurring renewal scan on due day queued %d deliveries after 1-day reminder, want 0", queued)
	}
	var deliveryCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE event_type = 'renewal_due'`).Scan(&deliveryCount); err != nil {
		t.Fatalf("count recurring renewal deliveries: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("recurring renewal delivery count = %d, want one configured 1-day reminder", deliveryCount)
	}
}

func TestRenewalRulesMatchCalendarMonthBoundaries(t *testing.T) {
	removedSameDayRule := []AdminAlertRule{{
		Metric:                "expiry_days",
		NotificationEventType: "renewal_due",
		Enabled:               true,
		Threshold:             0,
	}}
	dueDay := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if renewalRulesMatch(removedSameDayRule, dueDay, dueDay) {
		t.Fatal("removed same-day reminder still matched")
	}

	multiDayRule := []AdminAlertRule{{
		Metric:                "expiry_days",
		NotificationEventType: "renewal_due",
		Enabled:               true,
		Threshold:             7,
		RenewalDays:           []int{1, 3, 7},
	}}
	multiDayDue := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	for _, reminderDay := range []time.Time{
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
	} {
		if !renewalRulesMatch(multiDayRule, multiDayDue, reminderDay) {
			t.Fatalf("multi-day renewal reminder did not match %s", reminderDay.Format("2006-01-02"))
		}
	}
	if renewalRulesMatch(multiDayRule, multiDayDue, time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("multi-day renewal reminder matched an unselected day")
	}

	monthRule := []AdminAlertRule{{
		Metric:                "expiry_days",
		NotificationEventType: "renewal_due",
		Enabled:               true,
		Threshold:             renewalNoticeCalendarMonthThreshold,
	}}
	cases := []struct {
		name string
		due  string
		now  string
	}{
		{name: "non leap march end", due: "2026-03-31", now: "2026-02-28"},
		{name: "leap march end", due: "2024-03-31", now: "2024-02-29"},
		{name: "thirty day month", due: "2026-05-31", now: "2026-04-30"},
		{name: "ordinary month", due: "2026-06-18", now: "2026-05-18"},
		{name: "leap day due", due: "2024-02-29", now: "2024-01-29"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			due, err := time.Parse("2006-01-02", tc.due)
			if err != nil {
				t.Fatalf("parse due date: %v", err)
			}
			now, err := time.Parse("2006-01-02", tc.now)
			if err != nil {
				t.Fatalf("parse reminder date: %v", err)
			}
			if !renewalRulesMatch(monthRule, due, now) {
				t.Fatalf("calendar-month reminder did not match due=%s now=%s", tc.due, tc.now)
			}
			if renewalRulesMatch(monthRule, due, now.AddDate(0, 0, -1)) || renewalRulesMatch(monthRule, due, now.AddDate(0, 0, 1)) {
				t.Fatalf("calendar-month reminder matched outside its single trigger day")
			}
		})
	}
}

func TestRenewalNotificationScannerUsesCalendarMonthForRecurringFebruaryCycle(t *testing.T) {
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
	finalExpiryDate := "2026-04-01"
	billingCycle := "月"
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &finalExpiryDate, BillingCycle: &billingCycle}); err != nil {
		t.Fatalf("set recurring expiry date: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "test-credential", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	threshold := float64(renewalNoticeCalendarMonthThreshold)
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled, Threshold: &threshold}); err != nil {
		t.Fatalf("enable renewal rule: %v", err)
	}
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	queued, err := store.QueueDueRenewalNotifications(ctx, now)
	if err != nil {
		t.Fatalf("queue recurring calendar-month reminder: %v", err)
	}
	if queued != 1 {
		t.Fatalf("calendar-month scan queued %d deliveries, want 1 for 2026-03-01 due date", queued)
	}
	if queued, err := store.QueueDueRenewalNotifications(ctx, now.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("queue day-after reminder: %v", err)
	} else if queued != 0 {
		t.Fatalf("day-after scan queued %d deliveries, want 0", queued)
	}
}
func TestAgentHeartbeatHostAndStateDoNotDispatchRenewalDueNotification(t *testing.T) {
	tests := []struct {
		name string
		path string
		body func(time.Time) map[string]any
	}{
		{
			name: "heartbeat",
			path: "/api/agent/v1/heartbeat",
			body: func(now time.Time) map[string]any {
				return map[string]any{"ts": now.Unix(), "status": "online", "agent_version": "agent-test"}
			},
		},
		{
			name: "host",
			path: "/api/agent/v1/host",
			body: func(time.Time) map[string]any {
				return map[string]any{"hostname": "example-node-a", "os_name": "Linux", "arch": "amd64", "cpu_cores": 2, "memory_total_bytes": 1024, "disk_total_bytes": 2048}
			},
		},
		{
			name: "state",
			path: "/api/agent/v1/state",
			body: func(now time.Time) map[string]any {
				return map[string]any{"ts": now.Unix(), "cpu_percent": 10, "memory_used_bytes": 512, "memory_total_bytes": 1024, "disk_used_bytes": 1024, "disk_total_bytes": 2048, "net_in_total_bytes": 100, "net_out_total_bytes": 200, "net_in_speed_bps": 1, "net_out_speed_bps": 2, "uptime_seconds": 60}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			expiryDate := time.Now().UTC().Add(3 * 24 * time.Hour).Format("2006-01-02")
			if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &expiryDate}); err != nil {
				t.Fatalf("set expiry date: %v", err)
			}
			enabled := true
			if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
				t.Fatalf("create notification channel: %v", err)
			}
			if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled}); err != nil {
				t.Fatalf("enable renewal rule: %v", err)
			}

			telegram := newTelegramTestCapture(t)
			now := time.Now().UTC().Truncate(time.Second)
			payload, err := json.Marshal(tc.body(now))
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(payload))
			request.Header.Set("X-Node-ID", "example-node-a")
			request.Header.Set("Authorization", "Bearer test-agent-token")
			request.Header.Set("Content-Type", "application/json")
			NewHandler(telegram.handlerOptions(store)).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
			}
			_, forms, captureErrors := telegram.waitForCalls(t, 0)
			if len(captureErrors) != 0 || len(forms) != 0 {
				t.Fatalf("renewal calls=%d errors=%v forms=%v, want no high-frequency renewal dispatch", len(forms), captureErrors, forms)
			}
		})
	}
}
func TestRenewalNotificationScheduledScannerRunsIndependently(t *testing.T) {
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
	expiryDate := time.Now().UTC().Add(3 * 24 * time.Hour).Format("2006-01-02")
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &expiryDate}); err != nil {
		t.Fatalf("set expiry date: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enable renewal rule: %v", err)
	}

	telegram := newTelegramTestCapture(t)
	backgroundCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpHandler := NewHandler(HandlerOptions{Store: store, NotificationClient: telegram.server.Client(), TelegramAPIBaseURL: telegram.server.URL, RenewalNotificationInterval: 10 * time.Millisecond, BackgroundContext: backgroundCtx})
	defer cleanupTestHandler(t, httpHandler)
	_, forms, errors := telegram.waitForCalls(t, 1)
	if len(errors) != 0 || len(forms) != 1 || !strings.Contains(decodedTelegramText(forms[0]), "⚠️[到期]") {
		t.Fatalf("scheduled renewal calls=%d errors=%v forms=%v", len(forms), errors, forms)
	}
}
func TestQueueDueRenewalNotificationsDeduplicatesConcurrentScans(t *testing.T) {
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
	now := time.Now().UTC().Truncate(time.Second)
	expiryDate := now.Add(3 * 24 * time.Hour).Format("2006-01-02")
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &expiryDate}); err != nil {
		t.Fatalf("set expiry date: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enable renewal rule: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	results := make(chan int, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			queued, err := store.QueueDueRenewalNotifications(ctx, now)
			if err != nil {
				errs <- err
				return
			}
			results <- queued
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent renewal scan failed: %v", err)
	}
	queuedTotal := 0
	for queued := range results {
		queuedTotal += queued
	}
	if queuedTotal != 1 {
		t.Fatalf("concurrent scans queued %d deliveries, want exactly 1", queuedTotal)
	}
	var deliveryCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE event_type = 'renewal_due'`).Scan(&deliveryCount); err != nil {
		t.Fatalf("count renewal deliveries: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("renewal delivery count = %d, want 1", deliveryCount)
	}
	var markCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_event_marks WHERE event_type = 'renewal_due'`).Scan(&markCount); err != nil {
		t.Fatalf("count renewal marks: %v", err)
	}
	if markCount != 1 {
		t.Fatalf("renewal mark count = %d, want 1", markCount)
	}
}
func TestQueueDueRenewalNotificationsSkipsPermanentNode(t *testing.T) {
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
	expiryDate := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	permanent := true
	if _, err := store.UpdateAdminNode(ctx, "example-node-a", AdminNodeUpdateRequest{ExpiryDate: &expiryDate, ExpiryPermanent: &permanent}); err != nil {
		t.Fatalf("set permanent expiry: %v", err)
	}
	enabled := true
	if _, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{ID: "ops-telegram", Name: "Ops Telegram", Destination: "7579942307", Credential: "telegram-bot-credential-value", Enabled: &enabled}); err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	if _, err := store.UpdateAdminAlertRule(ctx, "renewal_due", AdminAlertRuleUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enable renewal rule: %v", err)
	}
	if queued, err := store.QueueDueRenewalNotifications(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("queue permanent renewal notifications: %v", err)
	} else if queued != 0 {
		t.Fatalf("permanent renewal scan queued %d deliveries, want 0", queued)
	}
	var deliveryCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE event_type = 'renewal_due'`).Scan(&deliveryCount); err != nil {
		t.Fatalf("count renewal deliveries: %v", err)
	}
	if deliveryCount != 0 {
		t.Fatalf("permanent renewal delivery count = %d, want 0", deliveryCount)
	}
}
