package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAgentHostUpsertUpdatesPublicSummaryTotals(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	body := map[string]any{
		"hostname":           "example-node-a-real",
		"os_name":            "debian",
		"os_version":         "13",
		"kernel":             "6.12.0",
		"arch":               "x86_64",
		"virtualization":     "kvm",
		"cpu_model":          "AMD EPYC",
		"cpu_cores":          4,
		"memory_total_bytes": int64(8 * 1024 * 1024 * 1024),
		"disk_total_bytes":   int64(160 * 1024 * 1024 * 1024),
		"boot_time":          int64(1782980000),
		"agent_version":      "agent-test",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal host body: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/host", bytes.NewReader(payload))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set("Content-Type", "application/json")
	NewHandler(HandlerOptions{Store: store}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(summary.Nodes))
	}
	node := summary.Nodes[0]
	if node.OS != "debian" || node.CPUCores == nil || *node.CPUCores != 4 || node.MemoryTotalBytes == nil || *node.MemoryTotalBytes != float64(8*1024*1024*1024) || node.DiskTotalBytes == nil || *node.DiskTotalBytes != float64(160*1024*1024*1024) {
		t.Fatalf("summary node after host = %+v, want host totals and cores", node)
	}
}
func TestAgentHostUpsertAutoFillsPublicNetworkIdentity(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	body := map[string]any{
		"hostname":           "example-node-a-real",
		"os_name":            "debian",
		"os_version":         "13",
		"kernel":             "6.12.0",
		"arch":               "x86_64",
		"virtualization":     "kvm",
		"cpu_model":          "AMD EPYC",
		"cpu_cores":          4,
		"memory_total_bytes": int64(8 * 1024 * 1024 * 1024),
		"disk_total_bytes":   int64(160 * 1024 * 1024 * 1024),
		"boot_time":          int64(1782980000),
		"agent_version":      "agent-test",
		"public_ipv4":        " 198.51.100.8 ",
		"public_ipv6":        "2001:db8::8",
		"country_code":       " jp ",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal host body: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/host", bytes.NewReader(payload))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set("Content-Type", "application/json")
	NewHandler(HandlerOptions{Store: store}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}

	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].PublicIPv4 != "198.51.100.8" || nodes[0].PublicIPv6 != "2001:db8::8" || nodes[0].CountryCode != "JP" {
		t.Fatalf("node network identity = %+v, want auto-filled normalized IPs and country", nodes[0])
	}
}
func TestAgentHostUpsertKeepsNetworkIdentityWhenDiscoveryOmitted(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE nodes SET public_ipv4 = '198.51.100.8', public_ipv6 = '2001:db8::8', country_code = 'JP' WHERE id = 'example-node-a'`); err != nil {
		t.Fatalf("seed network identity: %v", err)
	}

	body := []byte(`{"hostname":"example-node-a-real","os_name":"debian","os_version":"13","kernel":"6.12.0","arch":"x86_64","virtualization":"kvm","cpu_model":"AMD EPYC","cpu_cores":4,"memory_total_bytes":8589934592,"disk_total_bytes":171798691840,"boot_time":1782980000,"agent_version":"agent-test"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/host", bytes.NewReader(body))
	request.Header.Set("X-Node-ID", "example-node-a")
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set("Content-Type", "application/json")
	NewHandler(HandlerOptions{Store: store}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}

	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if nodes[0].PublicIPv4 != "198.51.100.8" || nodes[0].PublicIPv6 != "2001:db8::8" || nodes[0].CountryCode != "JP" {
		t.Fatalf("node network identity after omitted discovery = %+v, want existing values preserved", nodes[0])
	}
}
