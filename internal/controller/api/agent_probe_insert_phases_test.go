package api

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shui1iao/zeno/internal/shared/probe"
)

// resolveAgentProbeRound decides whether a batch is accepted at all, so each
// rejection reason is pinned separately: a regression here would either drop
// valid Agent data or accept rounds that do not match the current config.
func TestResolveAgentProbeRoundRejections(t *testing.T) {
	target := ProbeTarget{ID: "target-a", Type: "tcp", Count: 3, TimeoutMS: 1000}
	targets := map[string]ProbeTarget{target.ID: target}
	receivedAt := time.Unix(1700000000, 0).UTC()

	valid := preparedAgentProbeRound{
		targetID:   "target-a",
		targetType: "tcp",
		ts:         receivedAt,
		samples:    probeSamples(2),
	}
	if _, err := resolveAgentProbeRound(valid, targets, receivedAt); err != nil {
		t.Fatalf("a matching round must be accepted: %v", err)
	}

	cases := map[string]preparedAgentProbeRound{
		// An unknown or disabled target means the Agent is probing something the
		// controller no longer assigns to it.
		"unknown target": {targetID: "target-missing", ts: receivedAt, samples: probeSamples(1)},
		// A type mismatch means the target was reconfigured after the snapshot.
		"type mismatch": {targetID: "target-a", targetType: "icmp", ts: receivedAt, samples: probeSamples(1)},
		"no samples":    {targetID: "target-a", ts: receivedAt, samples: nil},
		// More samples than the target's configured count cannot come from the
		// current configuration.
		"too many samples for target": {targetID: "target-a", ts: receivedAt, samples: probeSamples(target.Count + 1)},
		// A timestamp far outside the accepted window indicates clock drift.
		"timestamp skew": {targetID: "target-a", ts: receivedAt.Add(-365 * 24 * time.Hour), samples: probeSamples(1)},
	}
	for name, round := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveAgentProbeRound(round, targets, receivedAt); !errors.Is(err, errInvalidAgentProbeResults) {
				t.Fatalf("want errInvalidAgentProbeResults, got %v", err)
			}
		})
	}
}

// The idempotency key must prefer the Agent-supplied round id and fall back to a
// content hash, so Agents that predate round ids still get replay protection.
func TestResolveAgentProbeRoundIdempotencyKey(t *testing.T) {
	target := ProbeTarget{ID: "target-a", Type: "tcp", Count: 3, TimeoutMS: 1000}
	targets := map[string]ProbeTarget{target.ID: target}
	receivedAt := time.Unix(1700000000, 0).UTC()

	withID, err := resolveAgentProbeRound(preparedAgentProbeRound{
		targetID:     "target-a",
		agentRoundID: "  round-7  ",
		ts:           receivedAt,
		samples:      probeSamples(2),
	}, targets, receivedAt)
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if withID.idempotencyKey != "agent:round-7" {
		t.Fatalf("idempotency key = %q, want agent:round-7 (trimmed)", withID.idempotencyKey)
	}

	withoutID, err := resolveAgentProbeRound(preparedAgentProbeRound{
		targetID: "target-a",
		ts:       receivedAt,
		samples:  probeSamples(2),
	}, targets, receivedAt)
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if len(withoutID.idempotencyKey) <= len("legacy:") || withoutID.idempotencyKey[:7] != "legacy:" {
		t.Fatalf("idempotency key = %q, want a legacy: content hash", withoutID.idempotencyKey)
	}
	// The resolved target must be attached for the insert to record it.
	if withoutID.target.ID != target.ID {
		t.Fatalf("target not attached: %+v", withoutID.target)
	}
}

// probeSamples builds a successful sample run of the requested length.
func probeSamples(count int) []probe.Sample {
	samples := make([]probe.Sample, 0, count)
	latency := 10.0
	for seq := 1; seq <= count; seq++ {
		samples = append(samples, probe.Sample{Seq: seq, Success: true, LatencyMS: &latency})
	}
	return samples
}

// A zero config version is the legacy/unknown snapshot value and must skip the
// staleness comparison; a mismatching non-zero version must be rejected so a
// batch measured against replaced config is never stored.
func TestCheckAgentProbeConfigVersion(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := t.Context()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM probe_config_meta WHERE id = 1`).Scan(&current); err != nil {
		t.Fatalf("read version: %v", err)
	}

	if err := checkAgentProbeConfigVersion(ctx, tx, 0); err != nil {
		t.Fatalf("legacy version 0 must skip the check: %v", err)
	}
	if err := checkAgentProbeConfigVersion(ctx, tx, current); err != nil {
		t.Fatalf("matching version must pass: %v", err)
	}
	if err := checkAgentProbeConfigVersion(ctx, tx, current+1); !errors.Is(err, errAgentProbeConfigStale) {
		t.Fatalf("want errAgentProbeConfigStale, got %v", err)
	}
}
