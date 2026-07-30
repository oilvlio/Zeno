package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminNotificationChannelsAreTelegramOnlyAndDoNotExposeChannelType(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels", bytes.NewBufferString(`{
		"name": "Telegram Home",
		"destination": "7579942307",
		"credential": "telegram-bot-secret-value",
		"enabled": true
	}`))
	createRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	if bytes.Contains(createRecorder.Body.Bytes(), []byte(`"type"`)) {
		t.Fatalf("telegram-only channel create response exposed channel type: %s", createRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-channels", nil)
	listRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	if bytes.Contains(listRecorder.Body.Bytes(), []byte(`"type"`)) {
		t.Fatalf("telegram-only channel list response exposed channel type: %s", listRecorder.Body.String())
	}

	explicitTypeRecorder := httptest.NewRecorder()
	explicitTypeRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels", bytes.NewBufferString(`{
		"name": "Explicit Type",
		"type": "unsupported",
		"destination": "7579942307",
		"credential": "telegram-bot-secret-value"
	}`))
	explicitTypeRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(explicitTypeRecorder, explicitTypeRequest)
	if explicitTypeRecorder.Code != http.StatusBadRequest {
		t.Fatalf("explicit type create status = %d, want 400; body=%s", explicitTypeRecorder.Code, explicitTypeRecorder.Body.String())
	}
	assertNoSensitiveNotificationLeak(t, explicitTypeRecorder.Body.String())
}
func TestAdminNotificationChannelsCreateListAndPatchHideStoredCredentials(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels", bytes.NewBufferString(`{
		"name": "  Telegram Home  ",
		"destination": "  7579942307  ",
		"credential": "telegram-bot-secret-value",
		"enabled": true
	}`))
	createRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(createRecorder, createRequest)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	assertNoNotificationCredentialField(t, createRecorder.Body.String())
	assertNoSensitiveNotificationLeak(t, createRecorder.Body.String())
	var createResponse struct {
		Channel struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Destination   string `json:"destination"`
			CredentialSet bool   `json:"credential_set"`
			Enabled       bool   `json:"enabled"`
		} `json:"channel"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(createRecorder.Body.String())).Decode(&createResponse); err != nil {
		t.Fatalf("decode created notification channel: %v", err)
	}
	if createResponse.Channel.ID == "" || createResponse.Channel.Name != "Telegram Home" || createResponse.Channel.Destination != "7579942307" || !createResponse.Channel.CredentialSet || !createResponse.Channel.Enabled {
		t.Fatalf("created channel = %+v, want normalized enabled telegram channel with credential_set only", createResponse.Channel)
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-channels", nil)
	listRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	assertNoNotificationCredentialField(t, listRecorder.Body.String())
	assertNoSensitiveNotificationLeak(t, listRecorder.Body.String())
	var listResponse struct {
		Channels []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Destination   string `json:"destination"`
			CredentialSet bool   `json:"credential_set"`
			Enabled       bool   `json:"enabled"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(listRecorder.Body.String())).Decode(&listResponse); err != nil {
		t.Fatalf("decode notification channels: %v", err)
	}
	if len(listResponse.Channels) != 1 || listResponse.Channels[0].ID != createResponse.Channel.ID || !listResponse.Channels[0].CredentialSet {
		t.Fatalf("listed channels = %+v, want one persisted channel with credential_set only", listResponse.Channels)
	}

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notification-channels/"+createResponse.Channel.ID, bytes.NewBufferString(`{
		"name": "  Home Telegram Updated  ",
		"enabled": false
	}`))
	patchRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}
	assertNoNotificationCredentialField(t, patchRecorder.Body.String())
	assertNoSensitiveNotificationLeak(t, patchRecorder.Body.String())
	var patchResponse struct {
		Channel struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			CredentialSet bool   `json:"credential_set"`
			Enabled       bool   `json:"enabled"`
		} `json:"channel"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(patchRecorder.Body.String())).Decode(&patchResponse); err != nil {
		t.Fatalf("decode patched notification channel: %v", err)
	}
	if patchResponse.Channel.ID != createResponse.Channel.ID || patchResponse.Channel.Name != "Home Telegram Updated" || !patchResponse.Channel.CredentialSet || patchResponse.Channel.Enabled {
		t.Fatalf("patched channel = %+v, want renamed disabled channel preserving credential_set only", patchResponse.Channel)
	}
	if _, err := store.UpdateAdminNotificationChannel(ctx, createResponse.Channel.ID, AdminNotificationChannelUpdateRequest{Credential: stringPtr("   ")}); err == nil {
		t.Fatalf("blank-only notification channel patch succeeded, want no-op patch to be rejected")
	} else if err != errInvalidAdminNotificationChannelWrite {
		t.Fatalf("blank-only notification channel patch error = %v, want invalid write", err)
	}
	if _, err := store.UpdateAdminNotificationChannel(ctx, createResponse.Channel.ID, AdminNotificationChannelUpdateRequest{Credential: stringPtr("telegram-bot-secret-value")}); err != nil {
		t.Fatalf("rewrite notification credential: %v", err)
	}
	if _, err := store.UpdateAdminNotificationChannel(ctx, createResponse.Channel.ID, AdminNotificationChannelUpdateRequest{Name: stringPtr("Home Telegram Still Secret"), Credential: stringPtr("   ")}); err != nil {
		t.Fatalf("blank credential with other patch fields should preserve old credential: %v", err)
	}
	dispatchChannel, err := store.AdminNotificationDispatchChannel(ctx, createResponse.Channel.ID)
	if err != errNotificationChannelNotFound {
		t.Fatalf("disabled dispatch channel lookup err = %v, want not found while channel disabled", err)
	}
	enabled := true
	if _, err := store.UpdateAdminNotificationChannel(ctx, createResponse.Channel.ID, AdminNotificationChannelUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("re-enable notification channel: %v", err)
	}
	dispatchChannel, err = store.AdminNotificationDispatchChannel(ctx, createResponse.Channel.ID)
	if err != nil {
		t.Fatalf("lookup dispatch channel after preserving credential: %v", err)
	}
	if dispatchChannel.Credential != "telegram-bot-secret-value" {
		t.Fatalf("stored credential after blank edit = %q, want original secret preserved", dispatchChannel.Credential)
	}
}
func TestAdminNotificationChannelTestSendsTelegramAndReturnsSanitizedDelivery(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	var receivedPath string
	var receivedForm string
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("telegram method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse telegram form: %v", err)
		}
		receivedPath = r.URL.Path
		receivedForm = r.Form.Encode()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer telegramAPI.Close()
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{
		ID:          "smoke-telegram",
		Name:        "Smoke Telegram",
		Destination: "7579942307",
		Credential:  "telegram-bot-secret-value",
		Enabled:     &enabled,
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass"), NotificationClient: telegramAPI.Client(), TelegramAPIBaseURL: telegramAPI.URL})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels/"+channel.ID+"/test", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("test status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoSensitiveNotificationLeak(t, recorder.Body.String())
	if receivedPath != "/bottelegram-bot-secret-value/sendMessage" || !strings.Contains(receivedForm, "chat_id=7579942307") || !strings.Contains(receivedForm, "%E9%80%9A%E7%9F%A5%E6%B8%A0%E9%81%93%E6%B5%8B%E8%AF%95") {
		t.Fatalf("telegram test request path=%q form=%q, want test sendMessage", receivedPath, receivedForm)
	}
	if strings.Contains(receivedForm, "telegram-bot-secret-value") {
		t.Fatalf("telegram form leaked credential: %s", receivedForm)
	}
	var response struct {
		Delivery struct {
			EventType   string `json:"event_type"`
			Label       string `json:"label"`
			ChannelID   string `json:"channel_id"`
			ChannelName string `json:"channel_name"`
			Success     bool   `json:"success"`
			Error       string `json:"error"`
		} `json:"delivery"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode test delivery response: %v", err)
	}
	if response.Delivery.EventType != "test_notification" || response.Delivery.Label != "测试发送" || response.Delivery.ChannelID != channel.ID || response.Delivery.ChannelName != "Smoke Telegram" || !response.Delivery.Success || response.Delivery.Error != "" {
		t.Fatalf("test delivery response = %+v, want successful sanitized test delivery", response.Delivery)
	}
}
func TestAdminNotificationChannelTestIsBlockedWithoutNotificationAuthority(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(context.Background(), AdminNotificationChannelCreateRequest{
		ID: "candidate-telegram", Name: "Candidate Telegram", Destination: "7579942307", Credential: "telegram-bot-secret-value", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass"), DisableNotifications: true})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels/"+channel.ID+"/test", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("disabled notification test status=%d want=%d body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}
func TestAdminNotificationChannelTestRespectsChannelEnabledState(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	enabled := false
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{
		ID:          "disabled-telegram",
		Name:        "Disabled Telegram",
		Destination: "7579942307",
		Credential:  "telegram-bot-secret-value",
		Enabled:     &enabled,
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-channels/"+channel.ID+"/test", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled channel test status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
}
func TestAdminNotificationChannelDeleteRemovesChannelWithoutCredentialLeak(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	enableTestNotificationCredentialEncryption(t, store)
	ctx := context.Background()
	enabled := true
	channel, err := store.CreateAdminNotificationChannel(ctx, AdminNotificationChannelCreateRequest{
		ID:          "smoke-telegram",
		Name:        "Smoke Telegram",
		Destination: "https://example.com/notify",
		Credential:  "telegram-bot-secret-value",
		Enabled:     &enabled,
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notification-channels/"+channel.ID, nil)
	deleteRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(deleteRecorder, deleteRequest)

	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204; body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	assertNoSensitiveNotificationLeak(t, deleteRecorder.Body.String())
	channels, err := store.AdminNotificationChannels(ctx)
	if err != nil {
		t.Fatalf("list notification channels after delete: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels after delete = %+v, want none", channels)
	}

	missingRecorder := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notification-channels/"+channel.ID, nil)
	missingRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404; body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}
	assertNoSensitiveNotificationLeak(t, missingRecorder.Body.String())
}
func TestAdminNotificationChannelsRejectUnauthorizedUnknownAndInvalidRequests(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		adminToken string
		wantStatus int
	}{
		{name: "create missing admin token", method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Home","type":"telegram","destination":"7579942307","credential":"telegram-bot-secret-value"}`, wantStatus: http.StatusUnauthorized},
		{name: "create blank name", method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"   ","type":"telegram","destination":"7579942307","credential":"telegram-bot-secret-value"}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create unsupported type", method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Home","type":"email","destination":"ops@example.com","credential":"email-secret-value"}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create missing credential", method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Home","type":"telegram","destination":"7579942307"}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "patch unknown channel", method: http.MethodPatch, path: "/api/admin/v1/notification-channels/missing", body: `{"enabled":false}`, adminToken: "admin-pass", wantStatus: http.StatusNotFound},
		{name: "patch empty body", method: http.MethodPatch, path: "/api/admin/v1/notification-channels/missing", body: `{}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "delete missing admin token", method: http.MethodDelete, path: "/api/admin/v1/notification-channels/missing", wantStatus: http.StatusUnauthorized},
		{name: "delete unknown channel", method: http.MethodDelete, path: "/api/admin/v1/notification-channels/missing", adminToken: "admin-pass", wantStatus: http.StatusNotFound},
		{name: "test missing admin token", method: http.MethodPost, path: "/api/admin/v1/notification-channels/missing/test", wantStatus: http.StatusUnauthorized},
		{name: "test unknown channel", method: http.MethodPost, path: "/api/admin/v1/notification-channels/missing/test", adminToken: "admin-pass", wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.adminToken != "" {
				request.Header.Set("X-Admin-Token", tc.adminToken)
			}
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			assertNoSensitiveNotificationLeak(t, recorder.Body.String())
		})
	}
}
func TestAdminNotificationTypesPatch(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notification-types/node_offline", bytes.NewBufferString(`{"enabled": false}`))
	patchRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}
	var patchResponse struct {
		Type struct {
			EventType string `json:"event_type"`
			Label     string `json:"label"`
			Enabled   bool   `json:"enabled"`
		} `json:"type"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(patchRecorder.Body.String())).Decode(&patchResponse); err != nil {
		t.Fatalf("decode patched notification type: %v", err)
	}
	if patchResponse.Type.EventType != "node_offline" || patchResponse.Type.Label != "离线" || patchResponse.Type.Enabled {
		t.Fatalf("patched notification type = %+v, want disabled offline type", patchResponse.Type)
	}
	var enabled int
	if err := store.db.QueryRowContext(context.Background(), `SELECT enabled FROM alert_rules WHERE id = 'node_offline'`).Scan(&enabled); err != nil {
		t.Fatalf("query alert rule enabled: %v", err)
	}
	if enabled != 0 {
		t.Fatalf("node_offline alert rule enabled = %d, want notification type compatibility endpoint to update alert_rules", enabled)
	}
}
func TestAdminNotificationTypesPatchRejectsSharedEventCompatibilityNoOp(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE alert_rules SET enabled = 0 WHERE notification_event_type = 'probe_unhealthy'`); err != nil {
		t.Fatalf("disable shared event rules: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notification-types/probe_unhealthy", bytes.NewBufferString(`{"enabled": true}`))
	patchRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusGone {
		t.Fatalf("patch status = %d, want 410; body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}
	rows, err := store.db.QueryContext(ctx, `SELECT id, enabled FROM alert_rules WHERE notification_event_type = 'probe_unhealthy' ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query shared event rules: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ruleID string
		var enabled int
		if err := rows.Scan(&ruleID, &enabled); err != nil {
			t.Fatalf("scan shared event rule: %v", err)
		}
		if enabled != 0 {
			t.Fatalf("shared event rule %s enabled = %d, want notification-types patch not to batch enable alert rules", ruleID, enabled)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("shared event rows: %v", err)
	}
}
func TestAdminNotificationTypesRejectUnauthorizedUnknownAndInvalidRequests(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		adminToken string
		wantStatus int
	}{
		{name: "patch unknown type", method: http.MethodPatch, path: "/api/admin/v1/notification-types/missing", body: `{"enabled":true}`, adminToken: "admin-pass", wantStatus: http.StatusNotFound},
		{name: "patch missing enabled", method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.adminToken != "" {
				request.Header.Set("X-Admin-Token", tc.adminToken)
			}
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			assertNoSensitiveNotificationLeak(t, recorder.Body.String())
		})
	}
}
