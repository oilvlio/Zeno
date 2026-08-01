package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shui1iao/zeno/internal/shared/probe"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingNodeStateStore struct {
	*SQLiteStore
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (store *blockingNodeStateStore) NodeState(ctx context.Context, nodeID string, window latencyWindow) (StateResponse, error) {
	store.once.Do(func() { close(store.started) })
	select {
	case <-store.release:
		return store.SQLiteStore.NodeState(ctx, nodeID, window)
	case <-ctx.Done():
		return StateResponse{}, ctx.Err()
	}
}

type probeTargetsVersionErrorStore struct{ mockStore }

func (probeTargetsVersionErrorStore) AuthorizeAgent(context.Context, string, string) (bool, error) {
	return true, nil
}
func (probeTargetsVersionErrorStore) EnabledProbeTargets(context.Context, string) ([]ProbeTarget, error) {
	return []ProbeTarget{{ID: "target", Name: "Target", Type: "tcping", Address: "127.0.0.1", Port: intPtrValue(80), Count: 1, TimeoutMS: 1000, IntervalSec: 30}}, nil
}
func (probeTargetsVersionErrorStore) ProbeConfigVersion(context.Context) (int64, error) {
	return 0, fmt.Errorf("database unavailable")
}
func (probeTargetsVersionErrorStore) BumpProbeConfigVersion(context.Context) (int64, error) {
	return 0, fmt.Errorf("database unavailable")
}
func (probeTargetsVersionErrorStore) InsertProbeRound(context.Context, string, ProbeTarget, time.Time, []probe.Sample) error {
	return nil
}
func (probeTargetsVersionErrorStore) RecordAgentHeartbeat(context.Context, string, time.Time, string, string) error {
	return nil
}
func (probeTargetsVersionErrorStore) UpsertAgentHost(context.Context, string, AgentHostRequest) error {
	return nil
}
func (probeTargetsVersionErrorStore) InsertAgentState(context.Context, string, AgentStateRequest) error {
	return nil
}

type agentAuthorizeErrorStore struct{ probeTargetsVersionErrorStore }

func (agentAuthorizeErrorStore) AuthorizeAgent(context.Context, string, string) (bool, error) {
	return false, fmt.Errorf("authorization failed for token super-secret-token")
}

func cloneMap(input map[string]any) map[string]any {
	copy := make(map[string]any, len(input))
	for key, value := range input {
		copy[key] = value
	}
	return copy
}

func postAgentHeartbeat(t *testing.T, handler http.Handler, ts int64, status string) *httptest.ResponseRecorder {
	t.Helper()
	payload := []byte(`{"ts":` + strconv.FormatInt(ts, 10) + `,"status":"` + status + `","agent_version":"agent-test"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/heartbeat", bytes.NewReader(payload))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("heartbeat status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	return recorder
}

func cleanupTestHandler(t *testing.T, httpHandler http.Handler) {
	t.Helper()
	cleanup, ok := httpHandler.(interface{ Cleanup(context.Context) error })
	if !ok {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cleanup.Cleanup(shutdownCtx); err != nil {
		t.Errorf("cleanup handler: %v", err)
	}
}

func postAgentState(t *testing.T, handle func(http.ResponseWriter, *http.Request), ts int64, cpuPercent float64) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{
		"ts":                  ts,
		"cpu_percent":         cpuPercent,
		"memory_used_bytes":   int64(4 * 1024 * 1024 * 1024),
		"memory_total_bytes":  int64(8 * 1024 * 1024 * 1024),
		"disk_used_bytes":     int64(40 * 1024 * 1024 * 1024),
		"disk_total_bytes":    int64(160 * 1024 * 1024 * 1024),
		"net_in_total_bytes":  int64(1_000_000),
		"net_out_total_bytes": int64(2_000_000),
		"net_in_speed_bps":    2048.5,
		"net_out_speed_bps":   1024.25,
		"uptime_seconds":      int64(3600),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal state body: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/state", bytes.NewReader(payload))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set("Content-Type", "application/json")
	handle(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("state status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	return recorder
}

func readAllString(t *testing.T, reader io.Reader) string {
	t.Helper()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type telegramTestCapture struct {
	server *httptest.Server
	mu     sync.Mutex
	paths  []string
	forms  []string
	errors []error
}

func newTelegramTestCapture(t *testing.T) *telegramTestCapture {
	t.Helper()
	capture := &telegramTestCapture{}
	capture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			capture.mu.Lock()
			capture.errors = append(capture.errors, err)
			capture.mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		capture.mu.Lock()
		capture.paths = append(capture.paths, r.URL.Path)
		capture.forms = append(capture.forms, r.Form.Encode())
		capture.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	t.Cleanup(capture.server.Close)
	return capture
}

func (capture *telegramTestCapture) handlerOptions(store Store) HandlerOptions {
	return HandlerOptions{Store: store, NotificationClient: capture.server.Client(), TelegramAPIBaseURL: capture.server.URL}
}

func (capture *telegramTestCapture) waitForCalls(t *testing.T, want int) ([]string, []string, []error) {
	t.Helper()
	waitUntil(t, time.Second, func() bool {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		return len(capture.paths)+len(capture.errors) >= want
	})
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]string(nil), capture.paths...), append([]string(nil), capture.forms...), append([]error(nil), capture.errors...)
}

func assertTelegramFormsDoNotLeakCredential(t *testing.T, forms []string, credential string) {
	t.Helper()
	for _, form := range forms {
		if strings.Contains(form, credential) {
			t.Fatalf("telegram form leaked credential: %s", form)
		}
	}
}
