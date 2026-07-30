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
	"time"
)

func TestAdminNodesRequiresAdminToken(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("token")) || bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("secret")) {
		t.Fatalf("admin auth failure body should not leak token/secret wording: %s", recorder.Body.String())
	}
}
func TestAdminNodesEmptyStoreReturnsEmptyList(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	nodes, err := store.AdminNodes(context.Background())
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if nodes == nil || len(nodes) != 0 {
		t.Fatalf("empty admin nodes = %#v, want non-nil empty slice", nodes)
	}

	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"nodes":[]`) {
		t.Fatalf("empty admin nodes response = %s, want nodes:[]", recorder.Body.String())
	}
}
func TestAdminLoginCreatesSessionAndPasswordUpdateInvalidatesOldPassword(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	publicURL := "https://zeno.example.com"
	if _, err := store.UpdateAdminSettings(context.Background(), AdminSettingsUpdateRequest{AgentControllerURL: &publicURL}); err != nil {
		t.Fatalf("set agent controller URL: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/login", strings.NewReader(`{"username":"admin","password":"admin-pass"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginResponse AdminLoginResponse
	if err := json.NewDecoder(loginRecorder.Body).Decode(&loginResponse); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if loginResponse.Username != "admin" || loginResponse.Token == "" || loginResponse.Token == "admin-pass" {
		t.Fatalf("login response = %+v, want opaque admin session", loginResponse)
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	listRequest.Header.Set("X-Admin-Token", loginResponse.Token)
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("session list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	passwordRecorder := httptest.NewRecorder()
	passwordRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/account", strings.NewReader(`{"username":"admin","current_password":"admin-pass","new_password":"new-admin-pass"}`))
	passwordRequest.Header.Set("X-Admin-Token", loginResponse.Token)
	handler.ServeHTTP(passwordRecorder, passwordRequest)
	if passwordRecorder.Code != http.StatusOK {
		t.Fatalf("account password status = %d, want 200; body=%s", passwordRecorder.Code, passwordRecorder.Body.String())
	}
	var passwordResponse AdminLoginResponse
	if err := json.NewDecoder(passwordRecorder.Body).Decode(&passwordResponse); err != nil {
		t.Fatalf("decode account password response: %v", err)
	}
	if passwordResponse.Token == "" || passwordResponse.Token == loginResponse.Token {
		t.Fatalf("account password response = %+v, want rotated session", passwordResponse)
	}

	oldTokenRecorder := httptest.NewRecorder()
	oldTokenRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	oldTokenRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(oldTokenRecorder, oldTokenRequest)
	if oldTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("old bootstrap token status = %d, want 401", oldTokenRecorder.Code)
	}

	oldLoginRecorder := httptest.NewRecorder()
	oldLoginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/login", strings.NewReader(`{"username":"admin","password":"admin-pass"}`))
	oldLoginRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(oldLoginRecorder, oldLoginRequest)
	if oldLoginRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("old password status = %d, want 401", oldLoginRecorder.Code)
	}

	newLoginRecorder := httptest.NewRecorder()
	newLoginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/login", strings.NewReader(`{"username":"admin","password":"new-admin-pass"}`))
	newLoginRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(newLoginRecorder, newLoginRequest)
	if newLoginRecorder.Code != http.StatusOK {
		t.Fatalf("new password status = %d, want 200; body=%s", newLoginRecorder.Code, newLoginRecorder.Body.String())
	}
	var newLoginResponse AdminLoginResponse
	if err := json.NewDecoder(newLoginRecorder.Body).Decode(&newLoginResponse); err != nil {
		t.Fatalf("decode new login response: %v", err)
	}

	accountRecorder := httptest.NewRecorder()
	accountRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/account", strings.NewReader(`{"username":"zeno-admin","current_password":"new-admin-pass","new_password":""}`))
	accountRequest.Header.Set("X-Admin-Token", newLoginResponse.Token)
	handler.ServeHTTP(accountRecorder, accountRequest)
	if accountRecorder.Code != http.StatusOK {
		t.Fatalf("account status = %d, want 200; body=%s", accountRecorder.Code, accountRecorder.Body.String())
	}
	var accountResponse AdminLoginResponse
	if err := json.NewDecoder(accountRecorder.Body).Decode(&accountResponse); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if accountResponse.Username != "zeno-admin" || accountResponse.Token == "" || accountResponse.Token == newLoginResponse.Token {
		t.Fatalf("account response = %+v, want renamed admin and rotated session", accountResponse)
	}

	oldUsernameRecorder := httptest.NewRecorder()
	oldUsernameRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/login", strings.NewReader(`{"username":"admin","password":"new-admin-pass"}`))
	oldUsernameRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(oldUsernameRecorder, oldUsernameRequest)
	if oldUsernameRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("old username status = %d, want 401", oldUsernameRecorder.Code)
	}

	accountGetRecorder := httptest.NewRecorder()
	accountGetRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/account", nil)
	accountGetRequest.Header.Set("X-Admin-Token", accountResponse.Token)
	handler.ServeHTTP(accountGetRecorder, accountGetRequest)
	if accountGetRecorder.Code != http.StatusOK {
		t.Fatalf("account get status = %d, want 200; body=%s", accountGetRecorder.Code, accountGetRecorder.Body.String())
	}
	var accountGetResponse AdminAccountResponse
	if err := json.NewDecoder(accountGetRecorder.Body).Decode(&accountGetResponse); err != nil {
		t.Fatalf("decode account get response: %v", err)
	}
	if accountGetResponse.Account.Username != "zeno-admin" {
		t.Fatalf("account get = %+v, want zeno-admin", accountGetResponse)
	}
}
func TestAdminSessionExpiresAfterOneDay(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/login", strings.NewReader(`{"username":"admin","password":"admin-pass"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginResponse AdminLoginResponse
	if err := json.NewDecoder(loginRecorder.Body).Decode(&loginResponse); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(context.Background(), `UPDATE admin_sessions SET created_at = ?, last_seen_at = ? WHERE token_hash = ?`, now-int64((23*time.Hour).Seconds()), now, HashAdminToken(loginResponse.Token)); err != nil {
		t.Fatalf("age fresh session: %v", err)
	}
	freshRecorder := httptest.NewRecorder()
	freshRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	freshRequest.Header.Set("X-Admin-Token", loginResponse.Token)
	handler.ServeHTTP(freshRecorder, freshRequest)
	if freshRecorder.Code != http.StatusOK {
		t.Fatalf("fresh one-day session status = %d, want 200; body=%s", freshRecorder.Code, freshRecorder.Body.String())
	}

	if _, err := store.db.ExecContext(context.Background(), `UPDATE admin_sessions SET created_at = ?, last_seen_at = ? WHERE token_hash = ?`, now-int64((25*time.Hour).Seconds()), now, HashAdminToken(loginResponse.Token)); err != nil {
		t.Fatalf("age expired session: %v", err)
	}
	expiredRecorder := httptest.NewRecorder()
	expiredRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	expiredRequest.Header.Set("X-Admin-Token", loginResponse.Token)
	handler.ServeHTTP(expiredRecorder, expiredRequest)
	if expiredRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expired one-day session status = %d, want 401; body=%s", expiredRecorder.Code, expiredRecorder.Body.String())
	}
}
