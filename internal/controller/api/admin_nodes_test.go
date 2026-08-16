package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminNodesListsEnabledAndDisabledNodesWithoutTokenHashes(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if err := store.RecordAgentHeartbeat(ctx, "example-node-a", time.Now().UTC().Truncate(time.Second), "online", "agent-test"); err != nil {
		t.Fatalf("record heartbeat: %v", err)
	}
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (id, display_name, token_hash, status, country_code, disabled, created_at, updated_at)
		VALUES ('disabled-node', 'Disabled Node', 'disabled-token-hash', 'no_data', 'US', 1, ?, ?)
	`, now, now); err != nil {
		t.Fatalf("insert disabled node: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	if bytes.Contains(bytes.ToLower([]byte(raw)), []byte("token")) || bytes.Contains(bytes.ToLower([]byte(raw)), []byte("secret")) || bytes.Contains([]byte(raw), []byte("disabled-token-hash")) {
		t.Fatalf("admin nodes response leaked sensitive fields: %s", raw)
	}

	var response struct {
		Nodes []struct {
			ID           string  `json:"id"`
			DisplayName  string  `json:"display_name"`
			Status       string  `json:"status"`
			CountryCode  string  `json:"country_code"`
			Disabled     bool    `json:"disabled"`
			LastSeenAt   *string `json:"last_seen_at"`
			AgentVersion string  `json:"agent_version"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode admin nodes: %v", err)
	}
	if len(response.Nodes) != 2 {
		t.Fatalf("admin nodes len = %d, want both enabled and disabled nodes", len(response.Nodes))
	}
	if response.Nodes[0].ID != "disabled-node" || !response.Nodes[0].Disabled {
		t.Fatalf("first admin node = %+v, want disabled-node visible with disabled=true", response.Nodes[0])
	}
	if response.Nodes[1].ID != "example-node-a" || response.Nodes[1].DisplayName != "Example Node A" || response.Nodes[1].Status != "online" || response.Nodes[1].CountryCode != "HK" || response.Nodes[1].LastSeenAt == nil || response.Nodes[1].AgentVersion != "agent-test" {
		t.Fatalf("example-node-a admin node = %+v, want persisted management fields", response.Nodes[1])
	}
}
func TestAdminNodePatchUpdatesEditableFieldsAndReturnsSafeDTO(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "US", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/example-node-a", bytes.NewBufferString(`{
		"display_name": "  Example Node A Edited  ",
		"country_code": " hk ",
		"region": "  Hong Kong  ",
		"billing_mode": "max",
		"monthly_reset_day": 15,
		"monthly_quota_bytes": 123456789,
		"disabled": true
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	if bytes.Contains(bytes.ToLower([]byte(raw)), []byte("token")) || bytes.Contains(bytes.ToLower([]byte(raw)), []byte("secret")) || bytes.Contains([]byte(raw), []byte("test-agent-token")) {
		t.Fatalf("admin node update response leaked sensitive fields: %s", raw)
	}
	var response struct {
		Node struct {
			ID                string `json:"id"`
			DisplayName       string `json:"display_name"`
			Status            string `json:"status"`
			CountryCode       string `json:"country_code"`
			Region            string `json:"region"`
			Disabled          bool   `json:"disabled"`
			BillingMode       string `json:"billing_mode"`
			MonthlyResetDay   int    `json:"monthly_reset_day"`
			MonthlyQuotaBytes int64  `json:"monthly_quota_bytes"`
		} `json:"node"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode updated admin node: %v", err)
	}
	if response.Node.ID != "example-node-a" || response.Node.DisplayName != "Example Node A Edited" || response.Node.Status != "disabled" || response.Node.CountryCode != "HK" || response.Node.Region != "Hong Kong" || !response.Node.Disabled || response.Node.BillingMode != "max" || response.Node.MonthlyResetDay != 15 || response.Node.MonthlyQuotaBytes != 123456789 {
		t.Fatalf("updated admin node = %+v, want trimmed editable fields and disabled status", response.Node)
	}

	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary after disabling node: %v", err)
	}
	if len(summary.Nodes) != 0 {
		t.Fatalf("public summary should hide disabled node, got %+v", summary.Nodes)
	}
}
func TestAdminNodePatchReplacesProbeAssignmentsInOneRequest(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.CreateAdminProbeTarget(ctx, AdminProbeTargetCreateRequest{
		ID: "batch-target-a", Name: "Batch A", Type: "ping", Address: "1.1.1.1", Count: 3, TimeoutMS: 1000, IntervalSec: 30,
		Assignments: []AdminProbeTargetAssignmentUpdate{{NodeID: "example-node-a", Enabled: true}},
	}); err != nil {
		t.Fatalf("create assigned target: %v", err)
	}
	if _, err := store.CreateAdminProbeTarget(ctx, AdminProbeTargetCreateRequest{
		ID: "batch-target-b", Name: "Batch B", Type: "ping", Address: "8.8.8.8", Count: 3, TimeoutMS: 1000, IntervalSec: 30,
	}); err != nil {
		t.Fatalf("create unassigned target: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/example-node-a", bytes.NewBufferString(`{
		"display_name":"Example Node A Fast",
		"home_probe_target_id":"batch-target-b",
		"probe_target_ids":["batch-target-b"]
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}

	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled targets: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "batch-target-b" {
		t.Fatalf("enabled targets = %+v, want only batch-target-b", targets)
	}
	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].DisplayName != "Example Node A Fast" || nodes[0].HomeProbeTargetID != "batch-target-b" {
		t.Fatalf("updated node = %+v, want node fields and home target committed together", nodes)
	}
}
func TestAdminNodePatchRejectsHomeTargetOutsideBatchSelection(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	for _, target := range []AdminProbeTargetCreateRequest{
		{ID: "batch-target-a", Name: "Batch A", Type: "ping", Address: "1.1.1.1", Count: 3, TimeoutMS: 1000, IntervalSec: 30},
		{ID: "batch-target-b", Name: "Batch B", Type: "ping", Address: "8.8.8.8", Count: 3, TimeoutMS: 1000, IntervalSec: 30},
	} {
		if _, err := store.CreateAdminProbeTarget(ctx, target); err != nil {
			t.Fatalf("create target %s: %v", target.ID, err)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/example-node-a", bytes.NewBufferString(`{
		"display_name":"Must Not Persist",
		"home_probe_target_id":"batch-target-a",
		"probe_target_ids":["batch-target-b"]
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	nodes, err := store.AdminNodes(ctx)
	if err != nil {
		t.Fatalf("admin nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].DisplayName != "Example Node A" || nodes[0].HomeProbeTargetID != "" {
		t.Fatalf("invalid batch partially persisted node = %+v", nodes)
	}
}
func TestAdminNodeBillingIPAndDisplayOrderFieldsFlowThroughAdminAndPublicSummary(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})

	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/nodes", bytes.NewBufferString(`{
		"id": "backup",
		"display_name": "Backup",
		"country_code": " jp ",
		"expiry_date": "2026-12-31",
		"billing_cycle": "年付",
		"renewal_amount": 120,
		"renewal_currency": "CNY",
		"billing_mode": "in",
		"monthly_reset_day": 10,
		"display_order": 30,
		"public_ipv4": "203.0.113.10",
		"public_ipv6": "2001:db8::10"
	}`))
	createRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", createRecorder.Code, createRecorder.Body.String())
	}

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/example-node-a", bytes.NewBufferString(`{
		"expiry_date": "2026-08-01",
		"billing_cycle": "月付",
		"renewal_amount": 60,
		"renewal_currency": "CNY",
		"billing_mode": "max",
		"monthly_reset_day": 15,
		"display_order": 10,
		"public_ipv4": "198.51.100.8",
		"public_ipv6": "2001:db8::8"
	}`))
	patchRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/nodes", nil)
	listRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	raw := listRecorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains(lower, []byte("credential")) || bytes.Contains(lower, []byte("hash")) {
		t.Fatalf("admin node metadata response leaked sensitive wording: %s", raw)
	}
	var response struct {
		Nodes []struct {
			ID              string   `json:"id"`
			ExpiryDate      string   `json:"expiry_date"`
			BillingCycle    string   `json:"billing_cycle"`
			RenewalAmount   *float64 `json:"renewal_amount"`
			RenewalCurrency string   `json:"renewal_currency"`
			BillingMode     string   `json:"billing_mode"`
			ResetDay        int      `json:"monthly_reset_day"`
			DisplayOrder    int      `json:"display_order"`
			PublicIPv4      string   `json:"public_ipv4"`
			PublicIPv6      string   `json:"public_ipv6"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode admin nodes metadata: %v", err)
	}
	if len(response.Nodes) != 2 {
		t.Fatalf("nodes len = %d, want 2", len(response.Nodes))
	}
	if response.Nodes[0].ID != "example-node-a" || response.Nodes[0].DisplayOrder != 10 || response.Nodes[0].ExpiryDate != "2026-08-01" || response.Nodes[0].BillingCycle != "月付" || response.Nodes[0].RenewalAmount == nil || *response.Nodes[0].RenewalAmount != 60 || response.Nodes[0].RenewalCurrency != "CNY" || response.Nodes[0].BillingMode != "max" || response.Nodes[0].ResetDay != 15 || response.Nodes[0].PublicIPv4 != "198.51.100.8" || response.Nodes[0].PublicIPv6 != "2001:db8::8" {
		t.Fatalf("example-node-a metadata = %+v, want edited billing/IP/order fields", response.Nodes[0])
	}
	if response.Nodes[1].ID != "backup" || response.Nodes[1].DisplayOrder != 30 || response.Nodes[1].RenewalAmount == nil || *response.Nodes[1].RenewalAmount != 120 || response.Nodes[1].RenewalCurrency != "CNY" || response.Nodes[1].BillingMode != "in" || response.Nodes[1].ResetDay != 10 {
		t.Fatalf("second node = %+v, want display-order sorted backup", response.Nodes[1])
	}

	summary, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Nodes) != 2 || summary.Nodes[0].ID != "example-node-a" || summary.Nodes[1].ID != "backup" {
		t.Fatalf("summary nodes order = %+v, want display_order order", summary.Nodes)
	}
	expectedExampleNodeAExpiry := expiryLabelValue(sql.NullString{String: "2026-08-01", Valid: true}, sql.NullString{String: "月付", Valid: true}, false, time.Now())
	expectedBackupExpiry := expiryLabelValue(sql.NullString{String: "2026-12-31", Valid: true}, sql.NullString{String: "年付", Valid: true}, false, time.Now())
	if summary.Nodes[0].ExpiryLabel != expectedExampleNodeAExpiry || summary.Nodes[1].ExpiryLabel != expectedBackupExpiry {
		t.Fatalf("summary expiry labels = %q/%q, want %q/%q", summary.Nodes[0].ExpiryLabel, summary.Nodes[1].ExpiryLabel, expectedExampleNodeAExpiry, expectedBackupExpiry)
	}
	if summary.Nodes[0].MonthlyCostCNY == nil || *summary.Nodes[0].MonthlyCostCNY != 60 || summary.Nodes[1].MonthlyCostCNY == nil || *summary.Nodes[1].MonthlyCostCNY != 10 {
		t.Fatalf("summary monthly costs = %v/%v, want 60/10", summary.Nodes[0].MonthlyCostCNY, summary.Nodes[1].MonthlyCostCNY)
	}
	expectedPeriod := billingPeriodFor(time.Now(), 15)
	if summary.Nodes[0].BillingMode != "max" || summary.Nodes[0].MonthlyResetDay != 15 || summary.Nodes[0].MonthlyPeriodStart != expectedPeriod.StartDate || summary.Nodes[0].MonthlyPeriodEnd != expectedPeriod.EndDate {
		t.Fatalf("summary billing period = %+v, want billing mode/reset day and current period", summary.Nodes[0])
	}
}
func TestAdminNodePatchRefreshesCachedPublicSummaryImmediately(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})

	initialRecorder := httptest.NewRecorder()
	handler.ServeHTTP(initialRecorder, httptest.NewRequest(http.MethodGet, "/api/public/v1/summary", nil))
	if initialRecorder.Code != http.StatusOK {
		t.Fatalf("initial summary status = %d, want 200; body=%s", initialRecorder.Code, initialRecorder.Body.String())
	}

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/example-node-a", strings.NewReader(`{"expiry_date":"2026-09-09","monthly_quota_bytes":987654321}`))
	patchRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}

	refreshedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(refreshedRecorder, httptest.NewRequest(http.MethodGet, "/api/public/v1/summary", nil))
	if refreshedRecorder.Code != http.StatusOK {
		t.Fatalf("refreshed summary status = %d, want 200; body=%s", refreshedRecorder.Code, refreshedRecorder.Body.String())
	}
	var summary SummaryResponse
	if err := json.NewDecoder(refreshedRecorder.Body).Decode(&summary); err != nil {
		t.Fatalf("decode refreshed summary: %v", err)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("summary nodes len = %d, want 1", len(summary.Nodes))
	}
	if summary.Nodes[0].ExpiryLabel != "2026-09-09" {
		t.Fatalf("expiry label = %q, want patched value", summary.Nodes[0].ExpiryLabel)
	}
	if summary.Nodes[0].MonthlyQuotaBytes == nil || *summary.Nodes[0].MonthlyQuotaBytes != 987654321 {
		t.Fatalf("monthly quota = %v, want patched value", summary.Nodes[0].MonthlyQuotaBytes)
	}
}
func TestAdminNodePatchRejectsUnauthorizedUnknownAndInvalidRequests(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})

	cases := []struct {
		name       string
		nodeID     string
		body       string
		adminToken string
		wantStatus int
	}{
		{name: "missing token", nodeID: "example-node-a", body: `{"display_name":"Changed"}`, wantStatus: http.StatusUnauthorized},
		{name: "unknown node", nodeID: "missing", body: `{"display_name":"Changed"}`, adminToken: "admin-pass", wantStatus: http.StatusNotFound},
		{name: "blank display name", nodeID: "example-node-a", body: `{"display_name":"   "}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "negative monthly quota", nodeID: "example-node-a", body: `{"monthly_quota_bytes":-1}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "invalid billing mode", nodeID: "example-node-a", body: `{"billing_mode":"95th"}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "negative renewal amount", nodeID: "example-node-a", body: `{"renewal_amount":-1}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "invalid renewal currency", nodeID: "example-node-a", body: `{"renewal_currency":"BTC"}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "zero monthly reset day", nodeID: "example-node-a", body: `{"monthly_reset_day":0}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "invalid monthly reset day", nodeID: "example-node-a", body: `{"monthly_reset_day":32}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/"+tc.nodeID, bytes.NewBufferString(tc.body))
			if tc.adminToken != "" {
				request.Header.Set("X-Admin-Token", tc.adminToken)
			}
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("token")) || bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("secret")) {
				t.Fatalf("error body should not leak sensitive wording: %s", recorder.Body.String())
			}
		})
	}
}
func TestAdminNodeDeleteRemovesNodeAndDependentData(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.CreateAdminNode(ctx, AdminNodeCreateRequest{ID: "backup", DisplayName: "Backup", CountryCode: "US"}); err != nil {
		t.Fatalf("create backup node: %v", err)
	}
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO host_info (node_id, hostname, updated_at)
		VALUES ('backup', 'backup-host', ?)
	`, now); err != nil {
		t.Fatalf("seed backup host info: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO state_samples (node_id, ts, cpu_percent)
		VALUES ('backup', ?, 42.5)
	`, now); err != nil {
		t.Fatalf("seed backup state sample: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO traffic_monthly (node_id, month, in_bytes, out_bytes, billable_bytes, updated_at)
		VALUES ('backup', '2026-07', 1, 2, 3, ?)
	`, now); err != nil {
		t.Fatalf("seed backup traffic: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO traffic_lifetime (node_id, in_bytes, out_bytes, updated_at)
		VALUES ('backup', 4, 5, ?)
	`, now); err != nil {
		t.Fatalf("seed backup lifetime traffic: %v", err)
	}
	roundResult, err := store.db.ExecContext(ctx, `
		INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent)
		VALUES ('backup', 'example-node-a-local', ?, 'tcping', 1, 1, 0)
	`, now)
	if err != nil {
		t.Fatalf("seed backup probe round: %v", err)
	}
	roundID, err := roundResult.LastInsertId()
	if err != nil {
		t.Fatalf("backup probe round id: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO probe_samples (round_id, seq, success, latency_ms)
		VALUES (?, 1, 1, 0.42)
	`, roundID); err != nil {
		t.Fatalf("seed backup probe sample: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO alert_rule_states (node_id, rule_id, active, updated_at)
		VALUES ('backup', 'cpu_high', 1, ?)
	`, now); err != nil {
		t.Fatalf("seed backup alert state: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO alert_rule_node_scopes (rule_id, node_id, created_at)
		VALUES ('cpu_high', 'backup', ?)
	`, now); err != nil {
		t.Fatalf("seed backup alert scope: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/nodes/backup", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "" {
		t.Fatalf("delete body = %q, want empty", recorder.Body.String())
	}
	waitForAdminDeletionCompleted(t, store, "node", "backup", 10*time.Second)
	checks := []struct {
		name  string
		query string
	}{
		{name: "nodes", query: `SELECT COUNT(*) FROM nodes WHERE id = 'backup'`},
		{name: "host_info", query: `SELECT COUNT(*) FROM host_info WHERE node_id = 'backup'`},
		{name: "state_samples", query: `SELECT COUNT(*) FROM state_samples WHERE node_id = 'backup'`},
		{name: "traffic_monthly", query: `SELECT COUNT(*) FROM traffic_monthly WHERE node_id = 'backup'`},
		{name: "traffic_lifetime", query: `SELECT COUNT(*) FROM traffic_lifetime WHERE node_id = 'backup'`},
		{name: "node_probe_targets", query: `SELECT COUNT(*) FROM node_probe_targets WHERE node_id = 'backup'`},
		{name: "probe_rounds", query: `SELECT COUNT(*) FROM probe_rounds WHERE node_id = 'backup'`},
		{name: "probe_samples", query: `SELECT COUNT(*) FROM probe_samples WHERE round_id = ?`},
		{name: "alert_rule_states", query: `SELECT COUNT(*) FROM alert_rule_states WHERE node_id = 'backup'`},
	}
	for _, check := range checks {
		var count int
		var err error
		if check.name == "probe_samples" {
			err = store.db.QueryRowContext(ctx, check.query, roundID).Scan(&count)
		} else {
			err = store.db.QueryRowContext(ctx, check.query).Scan(&count)
		}
		if err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after node delete = %d, want 0", check.name, count)
		}
	}
	var preservedScopes int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_rule_node_scopes WHERE rule_id = 'cpu_high' AND node_id = 'backup'`).Scan(&preservedScopes); err != nil {
		t.Fatalf("count preserved scopes: %v", err)
	}
	if preservedScopes != 0 {
		t.Fatalf("alert rule scope rows = %d, want foreign-key cascade cleanup", preservedScopes)
	}
	var cpuRuleEnabled int
	if err := store.db.QueryRowContext(ctx, `SELECT enabled FROM alert_rules WHERE id = 'cpu_high'`).Scan(&cpuRuleEnabled); err != nil {
		t.Fatalf("query cpu rule enabled: %v", err)
	}
	if cpuRuleEnabled != 0 {
		t.Fatalf("cpu_high enabled = %d, want disabled after deleting its last scoped node", cpuRuleEnabled)
	}
}
func TestCreateAdminNodeAddsNodeToEveryRestrictedNotificationType(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO alert_rule_node_scopes (rule_id, node_id, created_at)
		VALUES ('cpu_high', 'example-node-a', ?),
		       ('renewal_due', 'example-node-a', ?)
	`, now, now); err != nil {
		t.Fatalf("seed restricted notification types: %v", err)
	}

	if _, err := store.CreateAdminNode(ctx, AdminNodeCreateRequest{ID: "new-node", DisplayName: "New Node", CountryCode: "US"}); err != nil {
		t.Fatalf("create admin node: %v", err)
	}

	rows, err := store.db.QueryContext(ctx, `
		SELECT rule_id
		FROM alert_rule_node_scopes
		WHERE node_id = 'new-node'
		ORDER BY rule_id
	`)
	if err != nil {
		t.Fatalf("query new node notification scopes: %v", err)
	}
	defer rows.Close()
	var ruleIDs []string
	for rows.Next() {
		var ruleID string
		if err := rows.Scan(&ruleID); err != nil {
			t.Fatalf("scan new node notification scope: %v", err)
		}
		ruleIDs = append(ruleIDs, ruleID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate new node notification scopes: %v", err)
	}
	if got := strings.Join(ruleIDs, ","); got != "cpu_high,renewal_due" {
		t.Fatalf("new node restricted notification types = %q, want %q", got, "cpu_high,renewal_due")
	}

	var globalRuleCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_rules WHERE id = 'memory_high'`).Scan(&globalRuleCount); err != nil {
		t.Fatalf("count global notification type: %v", err)
	}
	if globalRuleCount != 1 {
		t.Fatalf("global notification type count = %d, want 1", globalRuleCount)
	}
	var globalRuleScopes int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_rule_node_scopes WHERE rule_id = 'memory_high'`).Scan(&globalRuleScopes); err != nil {
		t.Fatalf("count global notification type scopes: %v", err)
	}
	if globalRuleScopes != 0 {
		t.Fatalf("global notification type scope rows = %d, want 0 so it remains global", globalRuleScopes)
	}
}

func TestCreateAdminNodeRollsBackWhenNotificationScopeAssignmentFails(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	now := time.Now().UTC().Unix()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO alert_rule_node_scopes (rule_id, node_id, created_at)
		VALUES ('cpu_high', 'example-node-a', ?);
		CREATE TRIGGER fail_new_node_notification_scope
		BEFORE INSERT ON alert_rule_node_scopes
		WHEN NEW.node_id = 'rollback-node'
		BEGIN
			SELECT RAISE(ABORT, 'forced notification scope failure');
		END;
	`, now); err != nil {
		t.Fatalf("seed restricted notification type and failure trigger: %v", err)
	}

	if _, err := store.CreateAdminNode(ctx, AdminNodeCreateRequest{ID: "rollback-node", DisplayName: "Rollback Node", CountryCode: "US"}); err == nil {
		t.Fatal("create admin node error = nil, want forced notification scope failure")
	}

	var nodeCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE id = 'rollback-node'`).Scan(&nodeCount); err != nil {
		t.Fatalf("count rolled back node: %v", err)
	}
	if nodeCount != 0 {
		t.Fatalf("rolled back node count = %d, want 0", nodeCount)
	}
}

func TestAdminNodeCreateAddsEditableNodeWithoutReturningSecrets(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/nodes", bytes.NewBufferString(`{
		"display_name": "  New Server  ",
		"country_code": " us ",
		"region": "  Los Angeles  ",
		"billing_mode": "out",
		"monthly_reset_day": 20,
		"monthly_quota_bytes": 1099511627776
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) {
		t.Fatalf("admin node create response leaked sensitive wording: %s", raw)
	}
	var response struct {
		Node struct {
			ID                string `json:"id"`
			DisplayName       string `json:"display_name"`
			Status            string `json:"status"`
			CountryCode       string `json:"country_code"`
			Region            string `json:"region"`
			Disabled          bool   `json:"disabled"`
			BillingMode       string `json:"billing_mode"`
			MonthlyResetDay   int    `json:"monthly_reset_day"`
			MonthlyQuotaBytes int64  `json:"monthly_quota_bytes"`
		} `json:"node"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode created admin node: %v", err)
	}
	if response.Node.ID == "" || response.Node.DisplayName != "New Server" || response.Node.Status != "no_data" || response.Node.CountryCode != "US" || response.Node.Region != "Los Angeles" || response.Node.Disabled || response.Node.BillingMode != "out" || response.Node.MonthlyResetDay != 20 || response.Node.MonthlyQuotaBytes != 1099511627776 {
		t.Fatalf("created admin node = %+v, want trimmed editable no-data node", response.Node)
	}

	targets, err := store.EnabledProbeTargets(ctx, response.Node.ID)
	if err != nil {
		t.Fatalf("enabled probe targets for created node: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("created node targets = %d, want no default enabled target assignment", len(targets))
	}
}
