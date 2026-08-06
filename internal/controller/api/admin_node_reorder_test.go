package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedAdminNodeOrderTest(t *testing.T, store *SQLiteStore) {
	t.Helper()
	now := time.Now().UTC().Unix()
	for index, nodeID := range []string{"node-a", "node-b", "node-c"} {
		if _, err := store.db.ExecContext(context.Background(), `
			INSERT INTO nodes (id, display_name, token_hash, status, display_order, disabled, created_at, updated_at)
			VALUES (?, ?, ?, 'offline', ?, 0, ?, ?)
		`, nodeID, nodeID, HashAdminToken(nodeID), (index+1)*10, now, now); err != nil {
			t.Fatalf("seed %s: %v", nodeID, err)
		}
	}
}

func TestAdminNodeReorderUsesOneAtomicRequest(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	seedAdminNodeOrderTest(t, store)
	handler := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash("admin-pass")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/nodes/reorder", strings.NewReader(`{"node_ids":["node-c","node-a","node-b"]}`))
	request.Header.Set("X-Admin-Token", "admin-pass")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("reorder status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM nodes ORDER BY display_order`)
	if err != nil {
		t.Fatalf("query reordered nodes: %v", err)
	}
	defer rows.Close()
	wantIDs := []string{"node-c", "node-a", "node-b"}
	index := 0
	for rows.Next() {
		var nodeID string
		var displayOrder int
		if err := rows.Scan(&nodeID, &displayOrder); err != nil {
			t.Fatalf("scan reordered node: %v", err)
		}
		if nodeID != wantIDs[index] || displayOrder != (index+1)*10 {
			t.Fatalf("reordered node %d = %s/%d, want %s/%d", index, nodeID, displayOrder, wantIDs[index], (index+1)*10)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read reordered nodes: %v", err)
	}
	if index != len(wantIDs) {
		t.Fatalf("reordered row count = %d, want %d", index, len(wantIDs))
	}
}

func TestAdminNodeReorderRollsBackEveryOrderOnWriteFailure(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	seedAdminNodeOrderTest(t, store)
	if _, err := store.db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_node_c_reorder
		BEFORE UPDATE OF display_order ON nodes
		WHEN NEW.id = 'node-c'
		BEGIN
			SELECT RAISE(ABORT, 'forced reorder failure');
		END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if err := store.ReorderAdminNodes(context.Background(), AdminNodeReorderRequest{NodeIDs: []string{"node-b", "node-c", "node-a"}}); err == nil {
		t.Fatal("reorder succeeded, want forced write failure")
	}
	rows, err := store.db.QueryContext(context.Background(), `SELECT id, display_order FROM nodes ORDER BY display_order`)
	if err != nil {
		t.Fatalf("query rolled back order: %v", err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var nodeID string
		var displayOrder int
		if err := rows.Scan(&nodeID, &displayOrder); err != nil {
			t.Fatalf("scan rolled back order: %v", err)
		}
		wantID := []string{"node-a", "node-b", "node-c"}[index]
		if nodeID != wantID || displayOrder != (index+1)*10 {
			t.Fatalf("rolled back node %d = %s/%d, want %s/%d", index, nodeID, displayOrder, wantID, (index+1)*10)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read rolled back order: %v", err)
	}
	if index != 3 {
		t.Fatalf("rolled back row count = %d, want 3", index)
	}
}
