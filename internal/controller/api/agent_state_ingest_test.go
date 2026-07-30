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

func TestAgentStateIdempotencyAndRateLimit(t *testing.T) {
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
	defer cleanupTestHandler(t, handler)
	post := func(body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal state: %v", err)
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/state", bytes.NewReader(payload))
		request.Header.Set("X-Node-ID", "example-node-a")
		request.Header.Set("Authorization", "Bearer test-agent-token")
		request.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	base := map[string]any{
		"sample_id":           "state-sample-1",
		"ts":                  time.Now().UTC().Truncate(time.Second).Unix(),
		"cpu_percent":         12.5,
		"memory_used_bytes":   int64(4 * 1024),
		"memory_total_bytes":  int64(8 * 1024),
		"disk_used_bytes":     int64(40 * 1024),
		"disk_total_bytes":    int64(160 * 1024),
		"net_in_total_bytes":  int64(1_000_000),
		"net_out_total_bytes": int64(2_000_000),
		"net_in_speed_bps":    2048.5,
		"net_out_speed_bps":   1024.25,
		"uptime_seconds":      int64(3600),
	}
	if recorder := post(base); recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"accepted":true`) {
		t.Fatalf("first state status=%d body=%s, want accepted", recorder.Code, recorder.Body.String())
	}
	if recorder := post(base); recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"accepted":false`) {
		t.Fatalf("duplicate state status=%d body=%s, want idempotent no-op", recorder.Code, recorder.Body.String())
	}
	conflicting := cloneMap(base)
	conflicting["cpu_percent"] = 22.5
	if recorder := post(conflicting); recorder.Code != http.StatusBadRequest {
		t.Fatalf("conflicting state id status=%d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
	rateLimited := cloneMap(base)
	rateLimited["sample_id"] = "state-sample-2"
	rateLimited["ts"] = base["ts"].(int64)
	if recorder := post(rateLimited); recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"accepted":false`) {
		t.Fatalf("rate-limited state status=%d body=%s, want accepted=false", recorder.Code, recorder.Body.String())
	}
	olderButImmediate := cloneMap(base)
	olderButImmediate["sample_id"] = "state-sample-older"
	olderButImmediate["ts"] = base["ts"].(int64) - 1
	olderButImmediate["cpu_percent"] = 13.5
	if recorder := post(olderButImmediate); recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"accepted":false`) {
		t.Fatalf("older immediate state status=%d body=%s, want monotonic sample limit", recorder.Code, recorder.Body.String())
	}
	var samples int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM state_samples`).Scan(&samples); err != nil {
		t.Fatalf("count state samples: %v", err)
	}
	if samples != 1 {
		t.Fatalf("state samples=%d, want only first accepted row", samples)
	}
}
