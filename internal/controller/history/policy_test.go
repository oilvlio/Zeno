package history

import (
	"strings"
	"testing"
	"time"
)

// Retention boundaries decide what data is deleted, so each one is pinned
// explicitly rather than derived in the test from the same expression.
func TestCutoffsAt(t *testing.T) {
	now := time.Date(2026, 8, 1, 3, 16, 45, 0, time.UTC)
	cutoffs := CutoffsAt(now)

	if want := now.Add(-RawRetention).Unix(); cutoffs.Raw != want {
		t.Fatalf("raw cutoff = %d, want %d", cutoffs.Raw, want)
	}
	if want := now.Add(-RollupRetention).Unix(); cutoffs.LegacyRaw != want {
		t.Fatalf("legacy raw cutoff = %d, want %d", cutoffs.LegacyRaw, want)
	}
	if want := now.Add(-StalePendingNotificationDeliveryAfter).Unix(); cutoffs.StalePendingNotification != want {
		t.Fatalf("stale pending cutoff = %d, want %d", cutoffs.StalePendingNotification, want)
	}
	if cutoffs.Now != now.Unix() {
		t.Fatalf("now = %d, want %d", cutoffs.Now, now.Unix())
	}
	// Rollup cutoffs must land on bucket boundaries, otherwise a partially
	// filled bucket could be deleted mid-accumulation.
	if cutoffs.LatencyRollup%int64(LatencyRollupStep/time.Second) != 0 {
		t.Fatalf("latency rollup cutoff %d is not bucket aligned", cutoffs.LatencyRollup)
	}
	if cutoffs.StateRollup%int64(StateRollupStep/time.Second) != 0 {
		t.Fatalf("state rollup cutoff %d is not bucket aligned", cutoffs.StateRollup)
	}
	if cutoffs.LatencyRollup > cutoffs.LegacyRaw || cutoffs.StateRollup > cutoffs.LegacyRaw {
		t.Fatal("rollup cutoff is newer than the retention boundary")
	}
}

func TestCutoffsAtNormalizesLocation(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	instant := time.Date(2026, 8, 1, 11, 16, 45, 0, shanghai)
	if CutoffsAt(instant) != CutoffsAt(instant.UTC()) {
		t.Fatal("cutoffs depend on the caller's location")
	}
}

func TestBucketFloor(t *testing.T) {
	if got := BucketFloor(125, time.Minute); got != 120 {
		t.Fatalf("BucketFloor(125, 1m) = %d, want 120", got)
	}
	if got := BucketFloor(120, time.Minute); got != 120 {
		t.Fatalf("BucketFloor(120, 1m) = %d, want 120", got)
	}
	if got := BucketFloor(125, 30*time.Second); got != 120 {
		t.Fatalf("BucketFloor(125, 30s) = %d, want 120", got)
	}
	// A zero step would divide by zero; the input must pass through instead.
	if got := BucketFloor(125, 0); got != 125 {
		t.Fatalf("BucketFloor(125, 0) = %d, want 125", got)
	}
}

// Hourly retention is offset so it does not collide with the renewal scan that
// shares the SQLite writer; sub-hourly intervals keep their exact cadence.
func TestFirstDelay(t *testing.T) {
	if got := FirstDelay(time.Hour); got != time.Hour+ScheduleOffset {
		t.Fatalf("FirstDelay(1h) = %v, want %v", got, time.Hour+ScheduleOffset)
	}
	if got := FirstDelay(2 * time.Hour); got != 2*time.Hour+ScheduleOffset {
		t.Fatalf("FirstDelay(2h) = %v, want %v", got, 2*time.Hour+ScheduleOffset)
	}
	if got := FirstDelay(time.Minute); got != time.Minute {
		t.Fatalf("FirstDelay(1m) = %v, want unchanged", got)
	}
}

func TestStepSeconds(t *testing.T) {
	if got := StepSeconds(LatencyRollupStep); got != 60 {
		t.Fatalf("latency step = %d, want 60", got)
	}
	if got := StepSeconds(StateRollupStep); got != 30 {
		t.Fatalf("state step = %d, want 30", got)
	}
}

// A batch that is not bounded, or a cycle budget that is not finite, is how
// maintenance previously exceeded the write timeout on large databases.
func TestBatchBudgetsAreBounded(t *testing.T) {
	if BatchSize <= 0 || MaxBatchCycles <= 0 {
		t.Fatal("batch budgets must be positive")
	}
	if RawRetention >= RollupRetention {
		t.Fatal("raw retention must be shorter than rollup retention")
	}
	if BatchPause <= 0 {
		t.Fatal("batch pause must yield the writer")
	}
}

// The deletion statements must keep their INDEXED BY hints: without them
// SQLite falls back to a full scan and a single batch blows the write timeout.
func TestPruneStatementsForceCoveringIndex(t *testing.T) {
	if !strings.Contains(PruneExpiredProbeRoundsSQL, "INDEXED BY idx_probe_rounds_ts_target_node") {
		t.Fatal("probe round prune lost its covering index hint")
	}
	if !strings.Contains(PruneExpiredStateSamplesSQL, "INDEXED BY idx_state_samples_node_ts") {
		t.Fatal("state sample prune lost its covering index hint")
	}
	if !strings.Contains(PruneExpiredStateSamplesSQL, "CROSS JOIN") {
		t.Fatal("state sample prune lost its per-node seek")
	}
	for name, statement := range map[string]string{
		"probe rounds":    PruneExpiredProbeRoundsSQL,
		"state samples":   PruneExpiredStateSamplesSQL,
		"latency rollups": PruneExpiredLatencyRollupsSQL,
		"state rollups":   PruneExpiredStateRollupsSQL,
	} {
		if !strings.Contains(statement, "LIMIT ?") {
			t.Fatalf("%s prune is unbounded", name)
		}
	}
}

// Insert and delete run in one transaction over the same candidate window, so
// their ordering must match or compaction could drop unaggregated rows.
func TestRollupCandidateOrderingMatchesDeletion(t *testing.T) {
	if !strings.Contains(LatencyRollupInsertSQL, "ORDER BY ts, target_id, node_id, id") {
		t.Fatal("latency rollup candidate ordering changed")
	}
	if !strings.Contains(PruneExpiredProbeRoundsSQL, "ORDER BY ts, target_id, node_id, id") {
		t.Fatal("probe round deletion ordering changed")
	}
	if !strings.Contains(StateRollupInsertQuery, "ORDER BY samples.node_id, samples.ts, samples.id") {
		t.Fatal("state rollup candidate ordering changed")
	}
	if !strings.Contains(PruneExpiredStateSamplesSQL, "ORDER BY samples.node_id, samples.ts, samples.id") {
		t.Fatal("state sample deletion ordering changed")
	}
}

// Rollups accumulate: re-running a bucket must add to the stored sums rather
// than replace them, or repeated compaction would discard earlier samples.
func TestStateRollupUpsertAccumulates(t *testing.T) {
	query := StateRollupInsertQuery
	if !strings.Contains(query, "ON CONFLICT(node_id, bucket_start) DO UPDATE SET") {
		t.Fatal("state rollup lost its upsert clause")
	}
	for _, metric := range StateRollupMetrics {
		accumulate := metric + "_sum = state_history_rollups." + metric + "_sum + excluded." + metric + "_sum"
		if !strings.Contains(query, accumulate) {
			t.Fatalf("state rollup does not accumulate %s", metric)
		}
	}
	if !strings.Contains(LatencyRollupInsertSQL, "median_sum = latency_history_rollups.median_sum + excluded.median_sum") {
		t.Fatal("latency rollup does not accumulate median")
	}
}

// Column order is the scan contract shared with the reader.
func TestStateSourceAndAverageShareMetricOrder(t *testing.T) {
	source := StateSourceQuery
	average := StateAverageSelect
	previousSourceIndex := -1
	previousAverageIndex := -1
	for _, metric := range StateRollupMetrics {
		sourceIndex := strings.Index(source, metric+"_sum")
		if sourceIndex <= previousSourceIndex {
			t.Fatalf("metric %s is out of order in the source query", metric)
		}
		previousSourceIndex = sourceIndex

		averageIndex := strings.Index(average, "SUM("+metric+"_sum)")
		if averageIndex <= previousAverageIndex {
			t.Fatalf("metric %s is out of order in the average projection", metric)
		}
		previousAverageIndex = averageIndex
	}
	if strings.Count(average, "NULLIF(") != len(StateRollupMetrics) {
		t.Fatal("average projection must guard every metric against a zero count")
	}
	if !strings.Contains(source, "UNION ALL") {
		t.Fatal("source query must union raw samples with rollups")
	}
}

func TestStateRollupMetricsAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(StateRollupMetrics))
	for _, metric := range StateRollupMetrics {
		if _, duplicate := seen[metric]; duplicate {
			t.Fatalf("duplicate rollup metric %s", metric)
		}
		seen[metric] = struct{}{}
	}
	if len(StateRollupMetrics) == 0 {
		t.Fatal("rollup metric list is empty")
	}
}

// Generation is deterministic; the prebuilt vars must match a fresh build.
func TestPrebuiltQueriesMatchGenerators(t *testing.T) {
	if StateRollupInsertQuery != StateRollupInsertSQL() {
		t.Fatal("prebuilt state rollup insert drifted from its generator")
	}
	if StateSourceQuery != StateSourceSQL() {
		t.Fatal("prebuilt state source drifted from its generator")
	}
	if StateAverageSelect != StateAverageSelectSQL() {
		t.Fatal("prebuilt average projection drifted from its generator")
	}
}
