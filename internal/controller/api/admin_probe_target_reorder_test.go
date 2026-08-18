package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func createProbeTargetForReorderTest(t *testing.T, store *SQLiteStore, id string) {
	t.Helper()
	if _, err := store.CreateAdminProbeTarget(context.Background(), AdminProbeTargetCreateRequest{
		ID: id, Name: id, Type: "ping", Address: "192.0.2.1", Count: 3, TimeoutMS: 1000, IntervalSec: 30,
	}); err != nil {
		t.Fatalf("create target %s: %v", id, err)
	}
}

func TestAdminProbeTargetCreateDefaultsToAppendOrder(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	createProbeTargetForReorderTest(t, store, "target-a")
	createProbeTargetForReorderTest(t, store, "target-b")

	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM probe_targets ORDER BY display_order ASC`)
	if err != nil {
		t.Fatalf("query target order: %v", err)
	}
	defer rows.Close()
	wantIDs := []string{"target-a", "target-b"}
	for index, wantID := range wantIDs {
		if !rows.Next() {
			t.Fatalf("missing target %d", index)
		}
		var id string
		var displayOrder int
		if err := rows.Scan(&id, &displayOrder); err != nil {
			t.Fatalf("scan target: %v", err)
		}
		if id != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("target %d = %s/%d, want %s/%d", index, id, displayOrder, wantID, (index+1)*10)
		}
	}
	if rows.Next() {
		t.Fatal("unexpected extra target")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read target order: %v", err)
	}
}

func TestAdminProbeTargetReorderUsesOneAtomicRequest(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	for _, id := range []string{"target-a", "target-b", "target-c"} {
		createProbeTargetForReorderTest(t, store, id)
	}
	beforeVersion, err := store.ProbeConfigVersion(context.Background())
	if err != nil {
		t.Fatalf("read probe config version: %v", err)
	}

	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/reorder", strings.NewReader(`{"target_ids":["target-c","target-a","target-b"]}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("reorder status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM probe_targets ORDER BY display_order`)
	if err != nil {
		t.Fatalf("query reordered targets: %v", err)
	}
	defer rows.Close()
	for index, wantID := range []string{"target-c", "target-a", "target-b"} {
		if !rows.Next() {
			t.Fatalf("missing reordered target %d", index)
		}
		var id string
		var displayOrder int
		if err := rows.Scan(&id, &displayOrder); err != nil {
			t.Fatalf("scan reordered target: %v", err)
		}
		if id != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("reordered target %d = %s/%d, want %s/%d", index, id, displayOrder, wantID, (index+1)*10)
		}
	}
	if rows.Next() {
		t.Fatal("unexpected extra reordered target")
	}
	afterVersion, err := store.ProbeConfigVersion(context.Background())
	if err != nil {
		t.Fatalf("read bumped probe config version: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("probe config version = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func TestAdminProbeTargetReorderRejectsIncompleteDuplicateAndUnauthorizedRequests(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	for _, id := range []string{"target-a", "target-b", "target-c"} {
		createProbeTargetForReorderTest(t, store, id)
	}

	for name, targetIDs := range map[string][]string{
		"empty":      {},
		"incomplete": {"target-a", "target-b"},
		"duplicate":  {"target-a", "target-a", "target-c"},
		"unknown":    {"target-a", "target-b", "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			err := store.ReorderAdminProbeTargets(context.Background(), AdminProbeTargetReorderRequest{TargetIDs: targetIDs})
			if !errors.Is(err, errInvalidAdminTargetWrite) {
				t.Fatalf("reorder error = %v, want %v", err, errInvalidAdminTargetWrite)
			}
		})
	}

	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/probe-targets/reorder", strings.NewReader(`{"target_ids":["target-c","target-b","target-a"]}`))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized reorder status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM probe_targets ORDER BY display_order`)
	if err != nil {
		t.Fatalf("query unchanged order: %v", err)
	}
	defer rows.Close()
	for index, wantID := range []string{"target-a", "target-b", "target-c"} {
		if !rows.Next() {
			t.Fatalf("missing unchanged target %d", index)
		}
		var id string
		var displayOrder int
		if err := rows.Scan(&id, &displayOrder); err != nil {
			t.Fatalf("scan unchanged target: %v", err)
		}
		if id != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("unchanged target %d = %s/%d, want %s/%d", index, id, displayOrder, wantID, (index+1)*10)
		}
	}
}

func TestAdminProbeTargetReorderRollsBackEveryOrderOnWriteFailure(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	for _, id := range []string{"target-a", "target-b", "target-c"} {
		createProbeTargetForReorderTest(t, store, id)
	}
	if _, err := store.db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_target_b_reorder
		BEFORE UPDATE OF display_order ON probe_targets
		WHEN NEW.id = 'target-b'
		BEGIN
			SELECT RAISE(ABORT, 'forced target reorder failure');
		END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if err := store.ReorderAdminProbeTargets(context.Background(), AdminProbeTargetReorderRequest{TargetIDs: []string{"target-c", "target-b", "target-a"}}); err == nil {
		t.Fatal("reorder succeeded, want forced write failure")
	}
	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM probe_targets ORDER BY display_order`)
	if err != nil {
		t.Fatalf("query rolled back order: %v", err)
	}
	defer rows.Close()
	for index, wantID := range []string{"target-a", "target-b", "target-c"} {
		if !rows.Next() {
			t.Fatalf("missing rolled back target %d", index)
		}
		var id string
		var displayOrder int
		if err := rows.Scan(&id, &displayOrder); err != nil {
			t.Fatalf("scan rolled back target: %v", err)
		}
		if id != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("rolled back target %d = %s/%d, want %s/%d", index, id, displayOrder, wantID, (index+1)*10)
		}
	}
}
