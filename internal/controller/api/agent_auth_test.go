package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentProbeTargetsRequiresBearerToken(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	handler := NewHandler(HandlerOptions{Store: store})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/probe-targets", nil)
	request.Header.Set("X-Node-ID", "example-node-a")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("token")) || bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("secret")) {
		t.Fatalf("auth failure body should not leak token/secret wording: %s", recorder.Body.String())
	}
}
func TestAgentStateResponseDoesNotWaitForLiveDetailRefresh(t *testing.T) {
	sqliteStore, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer sqliteStore.Close()
	if err := sqliteStore.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	store := &blockingNodeStateStore{SQLiteStore: sqliteStore, started: make(chan struct{}), release: make(chan struct{})}
	httpHandler := NewHandler(HandlerOptions{Store: store})
	h := httpHandler.(*handler)
	_, unsubscribe := h.liveHub.subscribe(nodeStateLiveTopic("example-node-a", "1h"))
	defer unsubscribe()

	postAgentState(t, httpHandler.ServeHTTP, time.Now().UTC().Unix(), 12.5)
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("background live detail refresh did not start")
	}
	close(store.release)
	cleanupTestHandler(t, httpHandler)
}
func TestAgentProbeTargetsReturnsEnabledTargetsAfterAuth(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/probe-targets", nil)
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	NewHandler(HandlerOptions{Store: store}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response AgentProbeTargetsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode targets response: %v", err)
	}
	if len(response.Targets) != len(DefaultPreviewProbeTargets()) {
		t.Fatalf("targets len = %d, want %d", len(response.Targets), len(DefaultPreviewProbeTargets()))
	}
	if response.Version <= 0 {
		t.Fatalf("probe config version = %d, want positive version", response.Version)
	}
	if response.Targets[0].ID == "" || response.Targets[0].Name == "" || response.Targets[0].Address == "" {
		t.Fatalf("first target missing required public agent fields: %+v", response.Targets[0])
	}
	raw := recorder.Body.String()
	if bytes.Contains(bytes.ToLower([]byte(raw)), []byte("token")) || bytes.Contains(bytes.ToLower([]byte(raw)), []byte("secret")) {
		t.Fatalf("agent targets response leaked token/secret wording: %s", raw)
	}
}
func TestAgentProbeTargetsVersionErrorReturns500InsteadOfZero(t *testing.T) {
	handler := NewHandler(HandlerOptions{Store: probeTargetsVersionErrorStore{}})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/probe-targets", nil)
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when config version lookup fails; body=%s", recorder.Code, recorder.Body.String())
	}
}
func TestAgentInternalErrorsEmitStructuredSafeLog(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	handler := NewHandler(HandlerOptions{Store: agentAuthorizeErrorStore{}})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/heartbeat", strings.NewReader(`{"ts":1782990000,"status":"online"}`))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer super-secret-token")
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	logText := logs.String()
	for _, want := range []string{"agent_api_error", "endpoint=heartbeat", "node_id=example-node-a", "stage=authorize", "error=redacted"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log %q missing %q", logText, want)
		}
	}
	if strings.Contains(logText, "super-secret-token") {
		t.Fatalf("agent error log leaked token: %s", logText)
	}
}
