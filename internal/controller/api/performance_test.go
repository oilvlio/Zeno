package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAdminPerformanceReportsSummaryAndSQLiteSignals(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	const adminToken = "performance-admin"
	h := NewHandler(HandlerOptions{Store: store, AdminPasswordHash: testAdminPasswordHash(adminToken)})

	for index := 0; index < 2; index++ {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/public/v1/summary", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("summary request %d status = %d", index, recorder.Code)
		}
	}

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/admin/v1/performance", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized performance status = %d, want 401", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/performance", nil)
	request.Header.Set("X-Admin-Token", adminToken)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("performance status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var snapshot runtimePerformanceSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode performance response: %v", err)
	}
	if snapshot.Summary.CacheMisses != 1 || snapshot.Summary.FreshCacheHits != 1 || snapshot.Summary.Builds != 1 {
		t.Fatalf("summary performance = %#v, want one miss/build then one fresh hit", snapshot.Summary)
	}
	if snapshot.Summary.BuildFailures != 0 || snapshot.Summary.LastBytes == 0 {
		t.Fatalf("summary build health = %#v", snapshot.Summary)
	}
	if snapshot.SQLite.RollupEnabledAfter == "" || snapshot.SQLite.RollupReady {
		t.Fatalf("new database rollup grace = after %q ready=%t, want future/not-ready", snapshot.SQLite.RollupEnabledAfter, snapshot.SQLite.RollupReady)
	}
}

type scaleSummaryStore struct {
	mockStore
	summary SummaryResponse
}

func (store scaleSummaryStore) Summary(context.Context) (SummaryResponse, error) {
	return store.summary, nil
}

func summaryAtScale(nodeCount int) SummaryResponse {
	nodes := make([]Node, nodeCount)
	for index := range nodes {
		value := float64(index % 100)
		nodes[index] = Node{
			ID: fmt.Sprintf("node-%04d", index), DisplayName: fmt.Sprintf("Scale Node %04d", index),
			Status: "online", OS: "linux", CountryCode: "HK", CPUPercent: &value,
			MemoryUsedBytes: &value, MemoryTotalBytes: &value, DiskUsedBytes: &value, DiskTotalBytes: &value,
			NetInSpeedBps: &value, NetOutSpeedBps: &value, NetInTotalBytes: &value, NetOutTotalBytes: &value,
			NetInLifetimeBytes: &value, NetOutLifetimeBytes: &value, MonthlyBillableBytes: &value, MonthlyQuotaBytes: &value,
		}
	}
	return SummaryResponse{Nodes: nodes, Services: []ServiceTarget{}, LatencyPoints: []LatencyPoint{}, ExchangeRates: map[string]float64{"CNY": 1}}
}

func TestSummaryBuildScalesToFiveHundredNodesWithinBudget(t *testing.T) {
	h := NewHandler(HandlerOptions{Store: scaleSummaryStore{summary: summaryAtScale(500)}}).(*handler)
	started := time.Now()
	payload, err := h.summaryJSON(context.Background())
	if err != nil {
		t.Fatalf("build 500-node summary: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("500-node summary build took %s, budget 2s", elapsed)
	}
	if len(payload) > 1024*1024 {
		t.Fatalf("500-node summary bytes = %d, budget 1MiB", len(payload))
	}
}

func BenchmarkSummaryBuildAtScale(b *testing.B) {
	for _, nodeCount := range []int{100, 500} {
		b.Run(fmt.Sprintf("nodes-%d", nodeCount), func(b *testing.B) {
			h := NewHandler(HandlerOptions{Store: scaleSummaryStore{summary: summaryAtScale(nodeCount)}}).(*handler)
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				h.invalidateSummaryCache()
				if _, err := h.summaryJSON(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
