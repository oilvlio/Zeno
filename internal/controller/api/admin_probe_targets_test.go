package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminProbeTargetsListsTargetsAndAssignmentsWithoutSecrets(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/probe-targets", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("agent-super-secret")) {
		t.Fatalf("admin probe targets response leaked sensitive fields: %s", raw)
	}
	var response struct {
		Targets []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Port        *int   `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
			Assignments []struct {
				NodeID          string `json:"node_id"`
				NodeDisplayName string `json:"node_display_name"`
				Enabled         bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode admin probe targets: %v", err)
	}
	if len(response.Targets) != len(DefaultPreviewProbeTargets()) {
		t.Fatalf("targets len = %d, want %d", len(response.Targets), len(DefaultPreviewProbeTargets()))
	}
	findTarget := func(id string) struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		Address     string `json:"address"`
		Port        *int   `json:"port"`
		Count       int    `json:"count"`
		TimeoutMS   int    `json:"timeout_ms"`
		IntervalSec int    `json:"interval_sec"`
		Assignments []struct {
			NodeID          string `json:"node_id"`
			NodeDisplayName string `json:"node_display_name"`
			Enabled         bool   `json:"enabled"`
		} `json:"assignments"`
	} {
		for _, target := range response.Targets {
			if target.ID == id {
				return target
			}
		}
		return struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Port        *int   `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
			Assignments []struct {
				NodeID          string `json:"node_id"`
				NodeDisplayName string `json:"node_display_name"`
				Enabled         bool   `json:"enabled"`
			} `json:"assignments"`
		}{}
	}
	exampleNodeA := findTarget("example-node-a-local")
	if exampleNodeA.ID == "" || exampleNodeA.Name != "Example Node A" || exampleNodeA.Type != "tcping" || exampleNodeA.Address != "192.0.2.1" || exampleNodeA.Port == nil || *exampleNodeA.Port != 443 || exampleNodeA.Count != 3 || exampleNodeA.TimeoutMS != 1000 || exampleNodeA.IntervalSec != 30 {
		t.Fatalf("example-node-a target = %+v, want full target config", exampleNodeA)
	}
	if len(exampleNodeA.Assignments) != 1 || exampleNodeA.Assignments[0].NodeID != "example-node-a" || exampleNodeA.Assignments[0].NodeDisplayName != "Example Node A" || !exampleNodeA.Assignments[0].Enabled {
		t.Fatalf("example-node-a assignments = %+v, want enabled example-node-a assignment", exampleNodeA.Assignments)
	}
	if google := findTarget("google-dns"); google.ID == "" {
		t.Fatalf("google-dns target missing from admin inventory: %+v", response.Targets)
	}
	var wireResponse struct {
		Targets []map[string]json.RawMessage `json:"targets"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&wireResponse); err != nil {
		t.Fatalf("decode probe target wire shape: %v", err)
	}
	for _, target := range wireResponse.Targets {
		if _, exists := target["enabled"]; exists {
			t.Fatalf("probe target response still exposes removed global enabled field: %s", raw)
		}
	}
}
func TestAdminProbeTargetsReturnsEmptyAssignmentArrayForUnassignedTargets(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM node_probe_targets WHERE target_id = 'google-dns'`); err != nil {
		t.Fatalf("delete google-dns assignments: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/probe-targets", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Targets []struct {
			ID          string          `json:"id"`
			Assignments json.RawMessage `json:"assignments"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode admin probe targets: %v", err)
	}
	for _, target := range response.Targets {
		if target.ID == "google-dns" {
			if string(target.Assignments) != "[]" {
				t.Fatalf("google-dns assignments JSON = %s, want []", string(target.Assignments))
			}
			return
		}
	}
	t.Fatalf("google-dns target not found in admin response: %+v", response.Targets)
}
func TestAdminProbeTargetCreateDefaultsToNoAssignedServersWithoutSecrets(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/probe-targets", bytes.NewBufferString(`{
		"name": "  Example HTTPS  ",
		"type": "tcping",
		"address": "  example.com  ",
		"port": 443,
		"count": 5,
		"timeout_ms": 1500,
		"interval_sec": 90
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("agent-super-secret")) {
		t.Fatalf("admin probe target create response leaked sensitive fields: %s", raw)
	}
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Port        int    `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
			Assignments []struct {
				NodeID  string `json:"node_id"`
				Enabled bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode created target: %v", err)
	}
	if response.Target.ID == "" || response.Target.Name != "Example HTTPS" || response.Target.Type != "tcping" || response.Target.Address != "example.com" || response.Target.Port != 443 || response.Target.Count != 5 || response.Target.TimeoutMS != 1500 || response.Target.IntervalSec != 90 {
		t.Fatalf("created target = %+v, want trimmed tcping target", response.Target)
	}
	if len(response.Target.Assignments) != 0 {
		t.Fatalf("created target assignments = %+v, want no server enabled by default", response.Target.Assignments)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	for _, target := range targets {
		if target.ID == response.Target.ID {
			t.Fatalf("created target %q unexpectedly assigned to example-node-a enabled target set", response.Target.ID)
		}
	}
}
func TestAdminProbeTargetCreateAcceptsExplicitServerAssignments(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/probe-targets", bytes.NewBufferString(`{
		"name": "Assigned HTTPS",
		"type": "http_get",
		"address": "https://example.com/health",
		"count": 2,
		"timeout_ms": 1500,
		"interval_sec": 30,
		"assignments": [{"node_id":"example-node-a","enabled":true}]
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Assignments []struct {
				NodeID  string `json:"node_id"`
				Enabled bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode created target: %v", err)
	}
	if len(response.Target.Assignments) != 1 || response.Target.Assignments[0].NodeID != "example-node-a" || !response.Target.Assignments[0].Enabled {
		t.Fatalf("created assignments = %+v, want explicit example-node-a enabled", response.Target.Assignments)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	found := false
	for _, target := range targets {
		if target.ID == response.Target.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("created target %q not assigned to example-node-a enabled target set", response.Target.ID)
	}
}
func TestAdminProbeTargetCreateAcceptsPingWithoutPort(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/probe-targets", bytes.NewBufferString(`{
		"name": "  Example ICMP  ",
		"type": "icmp",
		"address": "  8.8.8.8  ",
		"count": 4,
		"timeout_ms": 900,
		"interval_sec": 45
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoSensitiveAdminProbeTargetLeak(t, recorder.Body.String())
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Port        *int   `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
			Assignments []struct {
				NodeID  string `json:"node_id"`
				Enabled bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode created ping target: %v", err)
	}
	if response.Target.ID == "" || response.Target.Name != "Example ICMP" || response.Target.Type != "ping" || response.Target.Address != "8.8.8.8" || response.Target.Port != nil || response.Target.Count != 4 || response.Target.TimeoutMS != 900 || response.Target.IntervalSec != 45 {
		t.Fatalf("created ping target = %+v, want normalized ping target without port", response.Target)
	}
	if len(response.Target.Assignments) != 0 {
		t.Fatalf("created ping assignments = %+v, want no server enabled by default", response.Target.Assignments)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	found := false
	for _, target := range targets {
		if target.ID == response.Target.ID {
			found = true
			if target.Type != "ping" || target.Port != nil {
				t.Fatalf("agent target = %+v, want ping target without port", target)
			}
		}
	}
	if found {
		t.Fatalf("created ping target %q unexpectedly assigned to example-node-a enabled target set", response.Target.ID)
	}
}
func TestAdminProbeTargetCreateAcceptsHTTPGETWithoutPort(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/probe-targets", bytes.NewBufferString(`{
		"name": "  Zeno Health  ",
		"type": "http_get",
		"address": "  https://example.com/health  ",
		"count": 2,
		"timeout_ms": 1500,
		"interval_sec": 30
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoSensitiveAdminProbeTargetLeak(t, recorder.Body.String())
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Port        *int   `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
			Assignments []struct {
				NodeID  string `json:"node_id"`
				Enabled bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode created http_get target: %v", err)
	}
	if response.Target.ID == "" || response.Target.Name != "Zeno Health" || response.Target.Type != "http_get" || response.Target.Address != "https://example.com/health" || response.Target.Port != nil || response.Target.Count != 2 || response.Target.TimeoutMS != 1500 || response.Target.IntervalSec != 30 {
		t.Fatalf("created http_get target = %+v, want normalized HTTP GET target without port", response.Target)
	}
	if len(response.Target.Assignments) != 0 {
		t.Fatalf("created http_get assignments = %+v, want no server enabled by default", response.Target.Assignments)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	found := false
	for _, target := range targets {
		if target.ID == response.Target.ID {
			found = true
			if target.Type != "http_get" || target.Port != nil {
				t.Fatalf("agent target = %+v, want http_get target without port", target)
			}
		}
	}
	if found {
		t.Fatalf("created http_get target %q unexpectedly assigned to example-node-a enabled target set", response.Target.ID)
	}
}
func TestAdminProbeTargetPatchCanSwitchToPingAndClearPort(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/example-node-a-local", bytes.NewBufferString(`{
		"type": "icmp",
		"address": "  1.1.1.1  "
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoSensitiveAdminProbeTargetLeak(t, recorder.Body.String())
	var response struct {
		Target struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Address string `json:"address"`
			Port    *int   `json:"port"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode updated ping target: %v", err)
	}
	if response.Target.ID != "example-node-a-local" || response.Target.Type != "ping" || response.Target.Address != "1.1.1.1" || response.Target.Port != nil {
		t.Fatalf("updated target = %+v, want ping target with cleared port", response.Target)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	for _, target := range targets {
		if target.ID == "example-node-a-local" && (target.Type != "ping" || target.Port != nil) {
			t.Fatalf("agent target = %+v, want ping target without port", target)
		}
	}
}
func TestAdminProbeTargetPatchCanSwitchToHTTPGETAndClearPort(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/example-node-a-local", bytes.NewBufferString(`{
		"type": "http_get",
		"address": "  https://example.com/health  "
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoSensitiveAdminProbeTargetLeak(t, recorder.Body.String())
	var response struct {
		Target struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Address string `json:"address"`
			Port    *int   `json:"port"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(recorder.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode updated http_get target: %v", err)
	}
	if response.Target.ID != "example-node-a-local" || response.Target.Type != "http_get" || response.Target.Address != "https://example.com/health" || response.Target.Port != nil {
		t.Fatalf("updated target = %+v, want http_get target with cleared port", response.Target)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	for _, target := range targets {
		if target.ID == "example-node-a-local" && (target.Type != "http_get" || target.Port != nil) {
			t.Fatalf("agent target = %+v, want http_get target without port", target)
		}
	}
}
func TestAdminProbeTargetPatchRejectsHTTPGETWithoutFullURL(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/example-node-a-local", bytes.NewBufferString(`{
		"type": "http_get"
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for http_get target without full URL; body=%s", recorder.Code, recorder.Body.String())
	}
}
func TestAdminProbeTargetPatchUpdatesEditableFieldsAndAffectsAgentTargets(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/example-node-a-local", bytes.NewBufferString(`{
		"name": "  Local Controller  ",
		"address": "  127.0.0.1  ",
		"port": 18981,
		"count": 4,
		"timeout_ms": 900,
		"interval_sec": 30
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("agent-super-secret")) {
		t.Fatalf("admin probe target update response leaked sensitive fields: %s", raw)
	}
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Address     string `json:"address"`
			Port        int    `json:"port"`
			Count       int    `json:"count"`
			TimeoutMS   int    `json:"timeout_ms"`
			IntervalSec int    `json:"interval_sec"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode updated target: %v", err)
	}
	if response.Target.ID != "example-node-a-local" || response.Target.Name != "Local Controller" || response.Target.Address != "127.0.0.1" || response.Target.Port != 18981 || response.Target.Count != 4 || response.Target.TimeoutMS != 900 || response.Target.IntervalSec != 30 {
		t.Fatalf("updated target = %+v, want edited target", response.Target)
	}
	targets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	found := false
	for _, target := range targets {
		if target.ID == "example-node-a-local" {
			found = true
		}
	}
	if !found {
		t.Fatalf("edited target should remain in the assigned Agent target set: %+v", targets)
	}
}
func TestAdminProbeTargetPatchUpdatesNodeAssignments(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.CreateAdminNode(ctx, AdminNodeCreateRequest{ID: "backup", DisplayName: "Backup", CountryCode: "US"}); err != nil {
		t.Fatalf("create backup node: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/example-node-a-local", bytes.NewBufferString(`{
		"assignments": [
			{"node_id": "example-node-a", "enabled": false},
			{"node_id": "backup", "enabled": true}
		]
	}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	raw := recorder.Body.String()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("agent-super-secret")) {
		t.Fatalf("admin probe target assignment response leaked sensitive fields: %s", raw)
	}
	var response struct {
		Target struct {
			ID          string `json:"id"`
			Assignments []struct {
				NodeID  string `json:"node_id"`
				Enabled bool   `json:"enabled"`
			} `json:"assignments"`
		} `json:"target"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(raw)).Decode(&response); err != nil {
		t.Fatalf("decode updated target assignments: %v", err)
	}
	if response.Target.ID != "example-node-a-local" {
		t.Fatalf("target id = %q, want example-node-a-local", response.Target.ID)
	}
	assignmentEnabled := map[string]bool{}
	for _, assignment := range response.Target.Assignments {
		assignmentEnabled[assignment.NodeID] = assignment.Enabled
	}
	if assignmentEnabled["example-node-a"] || !assignmentEnabled["backup"] {
		t.Fatalf("assignments = %+v, want example-node-a disabled and backup enabled", response.Target.Assignments)
	}

	exampleNodeATargets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("example-node-a enabled probe targets: %v", err)
	}
	for _, target := range exampleNodeATargets {
		if target.ID == "example-node-a-local" {
			t.Fatalf("example-node-a-local should be removed from example-node-a agent targets after assignment disable")
		}
	}
	backupTargets, err := store.EnabledProbeTargets(ctx, "backup")
	if err != nil {
		t.Fatalf("backup enabled probe targets: %v", err)
	}
	backupHasTarget := false
	for _, target := range backupTargets {
		if target.ID == "example-node-a-local" {
			backupHasTarget = true
		}
	}
	if !backupHasTarget {
		t.Fatalf("example-node-a-local should remain enabled for backup agent targets")
	}
}
func TestAdminProbeTargetDisplayOrderControlsInventoryAndAgentOrder(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})
	for targetID, order := range map[string]int{"google-dns": 5, "example-node-a-local": 250} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/"+targetID, bytes.NewBufferString(fmt.Sprintf(`{"display_order": %d}`, order)))
		request.Header.Set("X-Admin-Token", "admin-pass")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("patch %s status = %d, want 200; body=%s", targetID, recorder.Code, recorder.Body.String())
		}
		assertNoSensitiveAdminProbeTargetLeak(t, recorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/probe-targets", nil)
	listRequest.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listResponse struct {
		Targets []struct {
			ID           string `json:"id"`
			DisplayOrder int    `json:"display_order"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(listRecorder.Body.String())).Decode(&listResponse); err != nil {
		t.Fatalf("decode target list: %v", err)
	}
	if len(listResponse.Targets) == 0 || listResponse.Targets[0].ID != "google-dns" || listResponse.Targets[0].DisplayOrder != 5 {
		t.Fatalf("first admin target = %+v, want google-dns display_order 5", listResponse.Targets)
	}

	agentTargets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets: %v", err)
	}
	if len(agentTargets) == 0 || agentTargets[0].ID != "google-dns" {
		t.Fatalf("first agent target = %+v, want google-dns by display_order", agentTargets)
	}
}
func TestAdminProbeTargetDeleteRemovesTargetAndAssignments(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "agent-super-secret"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	if _, err := store.CreateAdminNode(ctx, AdminNodeCreateRequest{ID: "backup", DisplayName: "Backup", CountryCode: "US"}); err != nil {
		t.Fatalf("create backup node: %v", err)
	}
	if _, err := store.UpdateAdminProbeTarget(ctx, "example-node-a-local", AdminProbeTargetUpdateRequest{Assignments: []AdminProbeTargetAssignmentUpdate{
		{NodeID: "example-node-a", Enabled: true},
		{NodeID: "backup", Enabled: true},
	}}); err != nil {
		t.Fatalf("seed assignments: %v", err)
	}
	roundResult, err := store.db.ExecContext(ctx, `
		INSERT INTO probe_rounds (node_id, target_id, ts, type, sent, received, loss_percent)
		VALUES ('example-node-a', 'example-node-a-local', ?, 'tcping', 1, 1, 0)
	`, time.Now().UTC().Unix())
	if err != nil {
		t.Fatalf("seed probe round: %v", err)
	}
	roundID, err := roundResult.LastInsertId()
	if err != nil {
		t.Fatalf("seed probe round id: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO probe_samples (round_id, seq, success, latency_ms)
		VALUES (?, 1, 1, 0.42)
	`, roundID); err != nil {
		t.Fatalf("seed probe sample: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/probe-targets/example-node-a-local", nil)
	request.Header.Set("X-Admin-Token", "admin-pass")
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "" {
		t.Fatalf("delete body = %q, want empty", recorder.Body.String())
	}
	targets, err := store.AdminProbeTargets(ctx)
	if err != nil {
		t.Fatalf("admin targets after delete: %v", err)
	}
	for _, target := range targets {
		if target.ID == "example-node-a-local" {
			t.Fatalf("deleted target still visible in admin inventory: %+v", target)
		}
	}
	for _, nodeID := range []string{"example-node-a", "backup"} {
		enabledTargets, err := store.EnabledProbeTargets(ctx, nodeID)
		if err != nil {
			t.Fatalf("enabled targets for %s after delete: %v", nodeID, err)
		}
		for _, target := range enabledTargets {
			if target.ID == "example-node-a-local" {
				t.Fatalf("deleted target still assigned to %s agent targets", nodeID)
			}
		}
	}
	waitForAdminDeletionCompleted(t, store, "probe_target", "example-node-a-local", 10*time.Second)
	var remainingRounds, remainingSamples int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM probe_rounds WHERE target_id = 'example-node-a-local'`).Scan(&remainingRounds); err != nil {
		t.Fatalf("count remaining probe rounds: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM probe_samples WHERE round_id = ?`, roundID).Scan(&remainingSamples); err != nil {
		t.Fatalf("count remaining probe samples: %v", err)
	}
	if remainingRounds != 0 || remainingSamples != 0 {
		t.Fatalf("deleted target history remains: rounds=%d samples=%d", remainingRounds, remainingSamples)
	}
}
func TestAdminProbeTargetWritesRejectUnauthorizedUnknownAndInvalidRequests(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	handler := NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")})

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		adminToken string
		wantStatus int
	}{
		{name: "create missing token", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":443,"count":3,"timeout_ms":1000,"interval_sec":30}`, wantStatus: http.StatusUnauthorized},
		{name: "create blank name", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"   ","type":"tcping","address":"example.com","port":443,"count":3,"timeout_ms":1000,"interval_sec":30}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create ping option-looking address", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"ping","address":"-f","count":3,"timeout_ms":1000,"interval_sec":30}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create bad port", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":70000,"count":3,"timeout_ms":1000,"interval_sec":30}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create count above resource cap", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":443,"count":33,"timeout_ms":1000,"interval_sec":60}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create timeout below resource floor", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":443,"count":3,"timeout_ms":50,"interval_sec":30}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create exceeds single round budget", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":443,"count":32,"timeout_ms":5000,"interval_sec":60}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "create rejects removed global enabled field", method: http.MethodPost, path: "/api/admin/v1/probe-targets", body: `{"name":"A","type":"tcping","address":"example.com","port":443,"count":3,"timeout_ms":1000,"interval_sec":30,"enabled":false}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "patch unknown target", method: http.MethodPatch, path: "/api/admin/v1/probe-targets/missing", body: `{"name":"Changed"}`, adminToken: "admin-pass", wantStatus: http.StatusNotFound},
		{name: "patch negative count", method: http.MethodPatch, path: "/api/admin/v1/probe-targets/example-node-a-local", body: `{"count":0}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "patch interval too small for final budget", method: http.MethodPatch, path: "/api/admin/v1/probe-targets/example-node-a-local", body: `{"count":32}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "patch rejects removed global enabled field", method: http.MethodPatch, path: "/api/admin/v1/probe-targets/example-node-a-local", body: `{"enabled":false}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "patch unknown assignment node", method: http.MethodPatch, path: "/api/admin/v1/probe-targets/example-node-a-local", body: `{"assignments":[{"node_id":"missing","enabled":false}]}`, adminToken: "admin-pass", wantStatus: http.StatusBadRequest},
		{name: "delete missing token", method: http.MethodDelete, path: "/api/admin/v1/probe-targets/example-node-a-local", adminToken: "", wantStatus: http.StatusUnauthorized},
		{name: "delete unknown target", method: http.MethodDelete, path: "/api/admin/v1/probe-targets/missing", adminToken: "admin-pass", wantStatus: http.StatusNotFound},
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
			if bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("token")) || bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("secret")) {
				t.Fatalf("error body should not leak sensitive wording: %s", recorder.Body.String())
			}
		})
	}
}
func TestAdminProbeTargetAssignmentRejectsNodeTargetCountOverflow(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	existingTargets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets before fill: %v", err)
	}
	if len(existingTargets) >= maxProbeTargetsPerNode {
		t.Fatalf("seeded target count=%d unexpectedly already at cap %d", len(existingTargets), maxProbeTargetsPerNode)
	}

	for index := 0; index < maxProbeTargetsPerNode-len(existingTargets); index++ {
		_, err := store.CreateAdminProbeTarget(ctx, AdminProbeTargetCreateRequest{
			Name:        fmt.Sprintf("Bulk %02d", index),
			Type:        "tcping",
			Address:     "203.0.113.10",
			Port:        adminOptionalInt64{Set: true, Valid: true, Value: 443},
			Count:       1,
			TimeoutMS:   minProbeTargetTimeoutMS,
			IntervalSec: minProbeTargetIntervalSec,
			Assignments: []AdminProbeTargetAssignmentUpdate{{NodeID: "example-node-a", Enabled: true}},
		})
		if err != nil {
			t.Fatalf("create filler target %d: %v", index, err)
		}
	}
	filledTargets, err := store.EnabledProbeTargets(ctx, "example-node-a")
	if err != nil {
		t.Fatalf("enabled probe targets after fill: %v", err)
	}
	if len(filledTargets) != maxProbeTargetsPerNode {
		t.Fatalf("enabled target count after fill=%d, want cap %d", len(filledTargets), maxProbeTargetsPerNode)
	}
	_, err = store.CreateAdminProbeTarget(ctx, AdminProbeTargetCreateRequest{
		Name:        "Overflow",
		Type:        "tcping",
		Address:     "203.0.113.11",
		Port:        adminOptionalInt64{Set: true, Valid: true, Value: 443},
		Count:       1,
		TimeoutMS:   minProbeTargetTimeoutMS,
		IntervalSec: minProbeTargetIntervalSec,
		Assignments: []AdminProbeTargetAssignmentUpdate{{NodeID: "example-node-a", Enabled: true}},
	})
	if err != errInvalidAdminTargetWrite {
		t.Fatalf("overflow create error=%v, want errInvalidAdminTargetWrite", err)
	}
}
func TestAdminProbeTargetAssignmentRejectsNodeRoundBudgetOverflow(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SeedPreviewData(ctx, PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}
	heavyCreate := func(name, address string) error {
		_, err := store.CreateAdminProbeTarget(ctx, AdminProbeTargetCreateRequest{
			Name:        name,
			Type:        "tcping",
			Address:     address,
			Port:        adminOptionalInt64{Set: true, Valid: true, Value: 443},
			Count:       12,
			TimeoutMS:   maxProbeTargetTimeoutMS,
			IntervalSec: 60,
			Assignments: []AdminProbeTargetAssignmentUpdate{{NodeID: "example-node-a", Enabled: true}},
		})
		return err
	}
	if err := heavyCreate("Heavy A", "203.0.113.20"); err != nil {
		t.Fatalf("create first heavy target: %v", err)
	}
	if err := heavyCreate("Heavy B", "203.0.113.21"); err != errInvalidAdminTargetWrite {
		t.Fatalf("second heavy target error=%v, want errInvalidAdminTargetWrite", err)
	}
}
func TestAdminProbeTargetsRequiresAdminToken(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	if err := store.SeedPreviewData(context.Background(), PreviewSeedOptions{NodeID: "example-node-a", DisplayName: "Example Node A", CountryCode: "HK", AgentToken: "test-agent-token"}); err != nil {
		t.Fatalf("seed preview data: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/probe-targets", nil)
	NewHandler(HandlerOptions{Store: store, AdminTokenHash: HashAdminToken("admin-pass")}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("token")) || bytes.Contains(bytes.ToLower(recorder.Body.Bytes()), []byte("secret")) {
		t.Fatalf("admin target auth failure body should not leak token/secret wording: %s", recorder.Body.String())
	}
}
