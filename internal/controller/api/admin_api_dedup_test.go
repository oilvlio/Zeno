package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type adminAPIDedupEndpoint string

const (
	adminAPIDedupSettings         adminAPIDedupEndpoint = "settings"
	adminAPIDedupChannels         adminAPIDedupEndpoint = "channels"
	adminAPIDedupNotificationType adminAPIDedupEndpoint = "notification-type"
	adminAPIDedupAlertRule        adminAPIDedupEndpoint = "alert-rule"
)

type adminAPIDedupStore struct {
	Store
	adminStore
	adminAuthStore

	trace             []string
	sessionAuthorized bool

	settings         SiteSettings
	updatedSettings  SiteSettings
	channels         []AdminNotificationChannel
	createdChannel   AdminNotificationChannel
	notificationType AdminNotificationType
	alertRule        AdminAlertRule

	settingsErr         error
	updateSettingsErr   error
	channelsErr         error
	createChannelErr    error
	notificationTypeErr error
	alertRuleErr        error
}

func newAdminAPIDedupStore() *adminAPIDedupStore {
	return &adminAPIDedupStore{
		settings:        SiteSettings{SiteTitle: "Baseline", Theme: "system", UpdatedAt: "2026-08-01T00:00:00Z"},
		updatedSettings: SiteSettings{SiteTitle: "Updated", Theme: "dark", UpdatedAt: "2026-08-01T00:01:00Z"},
		channels: []AdminNotificationChannel{{
			ID: "ops", Name: "Ops", Destination: "7579942307", CredentialSet: true, Enabled: true,
			CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-08-01T00:00:00Z",
		}},
		createdChannel: AdminNotificationChannel{
			ID: "created", Name: "Created", Destination: "10001", CredentialSet: true, Enabled: true,
			CreatedAt: "2026-08-01T00:02:00Z", UpdatedAt: "2026-08-01T00:02:00Z",
		},
		notificationType: AdminNotificationType{EventType: "node_offline", Label: "离线通知", Enabled: false, UpdatedAt: "2026-08-01T00:03:00Z"},
		alertRule: AdminAlertRule{
			ID: "cpu_high", Name: "CPU 使用率", Category: "resource", Metric: "cpu_percent", Comparator: ">=",
			Threshold: 95, ThresholdUnit: "%", DurationSec: 600, Enabled: false,
			NotificationEventType: "probe_unhealthy", NotificationLabel: "异常", ScopeNodeIDs: []string{"node-a"},
			CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-08-01T00:04:00Z",
		},
	}
}

func (s *adminAPIDedupStore) AuthorizeAdminSession(_ context.Context, token string) (bool, error) {
	s.trace = append(s.trace, "auth:session:"+token)
	return s.sessionAuthorized, nil
}

func (s *adminAPIDedupStore) AdminAccountConfigured(context.Context) (bool, error) {
	s.trace = append(s.trace, "auth:configured")
	return false, nil
}

func (s *adminAPIDedupStore) AdminSettings(context.Context) (SiteSettings, error) {
	s.trace = append(s.trace, "settings:get")
	return s.settings, s.settingsErr
}

func (s *adminAPIDedupStore) UpdateAdminSettings(_ context.Context, update AdminSettingsUpdateRequest) (SiteSettings, error) {
	s.trace = append(s.trace, "settings:update:"+adminAPIDedupJSON(update))
	return s.updatedSettings, s.updateSettingsErr
}

func (s *adminAPIDedupStore) AdminNotificationChannels(context.Context) ([]AdminNotificationChannel, error) {
	s.trace = append(s.trace, "channels:get")
	return s.channels, s.channelsErr
}

func (s *adminAPIDedupStore) CreateAdminNotificationChannel(_ context.Context, create AdminNotificationChannelCreateRequest) (AdminNotificationChannel, error) {
	s.trace = append(s.trace, "channels:create:"+adminAPIDedupJSON(create))
	return s.createdChannel, s.createChannelErr
}

func (s *adminAPIDedupStore) UpdateAdminNotificationType(_ context.Context, eventType string, update AdminNotificationTypeUpdateRequest) (AdminNotificationType, error) {
	s.trace = append(s.trace, "notification-type:update:"+eventType+":"+adminAPIDedupJSON(update))
	return s.notificationType, s.notificationTypeErr
}

func (s *adminAPIDedupStore) UpdateAdminAlertRule(_ context.Context, ruleID string, update AdminAlertRuleUpdateRequest) (AdminAlertRule, error) {
	s.trace = append(s.trace, "alert-rule:update:"+ruleID+":"+adminAPIDedupJSON(update))
	return s.alertRule, s.alertRuleErr
}

func adminAPIDedupJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

type adminAPIDedupCase struct {
	name      string
	endpoint  adminAPIDedupEndpoint
	method    string
	path      string
	body      string
	token     string
	cookie    bool
	secure    bool
	configure func(*adminAPIDedupStore)
}

type adminAPIDedupResult struct {
	status int
	header http.Header
	body   []byte
	trace  []string
}

func TestAdminAPIDedupMatchesPreRefactorHandlers(t *testing.T) {
	internalErr := errors.New("injected store failure")
	oversizedSettings := `{"site_title":"` + strings.Repeat("x", int(adminJSONBodyLimit)) + `"}`
	tests := []adminAPIDedupCase{
		{name: "settings GET success", endpoint: adminAPIDedupSettings, method: http.MethodGet, path: "/api/admin/v1/settings", token: "admin-pass"},
		{name: "settings PATCH success", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"expected_revision":0,"site_title":"Updated"}`, token: "admin-pass"},
		{name: "settings wrong method after authorization", endpoint: adminAPIDedupSettings, method: http.MethodPost, path: "/api/admin/v1/settings", token: "admin-pass"},
		{name: "settings wrong method without authorization", endpoint: adminAPIDedupSettings, method: http.MethodPost, path: "/api/admin/v1/settings"},
		{name: "settings unauthorized", endpoint: adminAPIDedupSettings, method: http.MethodGet, path: "/api/admin/v1/settings"},
		{name: "settings forbidden cookie mutation", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"site_title":"Updated"}`, token: "session", cookie: true, secure: true},
		{name: "settings invalid JSON", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"site_title":`, token: "admin-pass"},
		{name: "settings unknown JSON field", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"unknown":true}`, token: "admin-pass"},
		{name: "settings oversized JSON body", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: oversizedSettings, token: "admin-pass"},
		{name: "settings business error", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"expected_revision":0,"site_title":"Updated"}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.updateSettingsErr = errInvalidAdminSettingsUpdate }},
		{name: "settings internal update error", endpoint: adminAPIDedupSettings, method: http.MethodPatch, path: "/api/admin/v1/settings", body: `{"expected_revision":0,"site_title":"Updated"}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.updateSettingsErr = internalErr }},
		{name: "settings internal GET error", endpoint: adminAPIDedupSettings, method: http.MethodGet, path: "/api/admin/v1/settings", token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.settingsErr = internalErr }},

		{name: "channels GET success", endpoint: adminAPIDedupChannels, method: http.MethodGet, path: "/api/admin/v1/notification-channels", token: "admin-pass"},
		{name: "channels POST success", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Created","destination":"10001","credential":"secret","enabled":true}`, token: "admin-pass"},
		{name: "channels wrong method", endpoint: adminAPIDedupChannels, method: http.MethodPatch, path: "/api/admin/v1/notification-channels", token: "admin-pass"},
		{name: "channels unauthorized", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Created","destination":"10001","credential":"secret"}`},
		{name: "channels invalid JSON", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":`, token: "admin-pass"},
		{name: "channels unknown JSON field", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Created","destination":"10001","credential":"secret","unknown":true}`, token: "admin-pass"},
		{name: "channels business error", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Created","destination":"10001","credential":"secret"}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.createChannelErr = errInvalidAdminNotificationChannelWrite }},
		{name: "channels internal create error", endpoint: adminAPIDedupChannels, method: http.MethodPost, path: "/api/admin/v1/notification-channels", body: `{"name":"Created","destination":"10001","credential":"secret"}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.createChannelErr = internalErr }},
		{name: "channels internal GET error", endpoint: adminAPIDedupChannels, method: http.MethodGet, path: "/api/admin/v1/notification-channels", token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.channelsErr = internalErr }},

		{name: "notification type PATCH success", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false}`, token: "admin-pass"},
		{name: "notification type wrong method before authorization", endpoint: adminAPIDedupNotificationType, method: http.MethodGet, path: "/api/admin/v1/notification-types/node_offline"},
		{name: "notification type missing resource", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/", body: `{"enabled":false}`, token: "admin-pass"},
		{name: "notification type unauthorized", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false}`},
		{name: "notification type forbidden cookie mutation", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false}`, token: "session", cookie: true, secure: true},
		{name: "notification type invalid JSON", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":`, token: "admin-pass"},
		{name: "notification type unknown JSON field", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false,"unknown":true}`, token: "admin-pass"},
		{name: "notification type business error", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.notificationTypeErr = errInvalidAdminNotificationTypeWrite }},
		{name: "notification type internal error", endpoint: adminAPIDedupNotificationType, method: http.MethodPatch, path: "/api/admin/v1/notification-types/node_offline", body: `{"enabled":false}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.notificationTypeErr = internalErr }},

		{name: "alert rule PATCH success", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":false,"threshold":95,"duration_sec":600,"scope_node_ids":["node-a"]}`, token: "admin-pass"},
		{name: "alert rule wrong method before authorization", endpoint: adminAPIDedupAlertRule, method: http.MethodGet, path: "/api/admin/v1/alert-rules/cpu_high"},
		{name: "alert rule missing resource", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/", body: `{"enabled":false}`, token: "admin-pass"},
		{name: "alert rule unauthorized", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":false}`},
		{name: "alert rule invalid JSON", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":`, token: "admin-pass"},
		{name: "alert rule unknown JSON field", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":false,"unknown":true}`, token: "admin-pass"},
		{name: "alert rule business error", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":false}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.alertRuleErr = errInvalidAdminAlertRuleUpdate }},
		{name: "alert rule internal error", endpoint: adminAPIDedupAlertRule, method: http.MethodPatch, path: "/api/admin/v1/alert-rules/cpu_high", body: `{"enabled":false}`, token: "admin-pass", configure: func(store *adminAPIDedupStore) { store.alertRuleErr = internalErr }},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			want := runAdminAPIDedupCase(t, testCase, true)
			got := runAdminAPIDedupCase(t, testCase, false)
			if got.status != want.status {
				t.Fatalf("status = %d, want pre-refactor %d; got body=%s want body=%s", got.status, want.status, got.body, want.body)
			}
			if !reflect.DeepEqual(got.header, want.header) {
				t.Fatalf("headers = %#v, want pre-refactor %#v", got.header, want.header)
			}
			if !bytes.Equal(got.body, want.body) {
				t.Fatalf("body = %q, want pre-refactor %q", got.body, want.body)
			}
			if !reflect.DeepEqual(got.trace, want.trace) {
				t.Fatalf("store/auth trace = %#v, want pre-refactor %#v", got.trace, want.trace)
			}
			t.Logf("BASE\t%s\t%d\t%s\ttrace=%v", testCase.name, got.status, strings.TrimSpace(string(got.body)), got.trace)
		})
	}
}

func runAdminAPIDedupCase(t *testing.T, testCase adminAPIDedupCase, reference bool) adminAPIDedupResult {
	t.Helper()
	store := newAdminAPIDedupStore()
	if testCase.configure != nil {
		testCase.configure(store)
	}
	requestTarget := testCase.path
	if testCase.secure {
		requestTarget = "https://zeno.test" + testCase.path
	}
	request := httptest.NewRequest(testCase.method, requestTarget, strings.NewReader(testCase.body))
	if testCase.cookie {
		request.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: testCase.token})
	} else if testCase.token != "" {
		request.Header.Set("X-Admin-Token", testCase.token)
	}
	recorder := httptest.NewRecorder()
	h := &handler{store: store, adminPasswordHash: testAdminPasswordHash("admin-pass")}
	switch testCase.endpoint {
	case adminAPIDedupSettings:
		if reference {
			referenceHandleAdminSettings(h, recorder, request)
		} else {
			h.handleAdminSettings(recorder, request)
		}
	case adminAPIDedupChannels:
		if reference {
			referenceHandleAdminNotificationChannels(h, recorder, request)
		} else {
			h.handleAdminNotificationChannels(recorder, request)
		}
	case adminAPIDedupNotificationType:
		if reference {
			referenceHandleAdminNotificationTypeResource(h, recorder, request)
		} else {
			h.handleAdminNotificationTypeResource(recorder, request)
		}
	case adminAPIDedupAlertRule:
		if reference {
			referenceHandleAdminAlertRuleResource(h, recorder, request)
		} else {
			h.handleAdminAlertRuleResource(recorder, request)
		}
	default:
		t.Fatalf("unknown endpoint %q", testCase.endpoint)
	}
	return adminAPIDedupResult{status: recorder.Code, header: recorder.Header().Clone(), body: append([]byte(nil), recorder.Body.Bytes()...), trace: append([]string(nil), store.trace...)}
}

func referenceHandleAdminSettings(h *handler, w http.ResponseWriter, r *http.Request) {
	store, ok := h.authorizeAdminRequest(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := store.AdminSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, AdminSettingsResponse{Settings: settings})
	case http.MethodPatch:
		var update AdminSettingsUpdateRequest
		if !decodeJSONBody(w, r, &update, adminJSONBodyLimit, true) {
			return
		}
		settings, err := store.UpdateAdminSettings(r.Context(), update)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, AdminSettingsResponse{Settings: settings})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func referenceHandleAdminNotificationChannels(h *handler, w http.ResponseWriter, r *http.Request) {
	store, ok := h.authorizeAdminRequest(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		channels, err := store.AdminNotificationChannels(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, AdminNotificationChannelsResponse{Channels: channels})
	case http.MethodPost:
		var create AdminNotificationChannelCreateRequest
		if !decodeJSONBody(w, r, &create, adminJSONBodyLimit, true) {
			return
		}
		channel, err := store.CreateAdminNotificationChannel(r.Context(), create)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, AdminNotificationChannelResponse{Channel: channel})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func referenceHandleAdminNotificationTypeResource(h *handler, w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/v1/notification-types/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	store, ok := h.authorizeAdminRequest(w, r)
	if !ok {
		return
	}
	var update AdminNotificationTypeUpdateRequest
	if !decodeJSONBody(w, r, &update, adminJSONBodyLimit, true) {
		return
	}
	notificationType, err := store.UpdateAdminNotificationType(r.Context(), parts[0], update)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, AdminNotificationTypeResponse{Type: notificationType})
}

func referenceHandleAdminAlertRuleResource(h *handler, w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/v1/alert-rules/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	store, ok := h.authorizeAdminRequest(w, r)
	if !ok {
		return
	}
	var update AdminAlertRuleUpdateRequest
	if !decodeJSONBody(w, r, &update, adminJSONBodyLimit, true) {
		return
	}
	rule, err := store.UpdateAdminAlertRule(r.Context(), parts[0], update)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, AdminAlertRuleResponse{Rule: rule})
}
