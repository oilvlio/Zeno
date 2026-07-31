package api

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// Every rejection message is part of the agent-facing contract: agents log it
// verbatim, so a reworded or reordered check would change observable behaviour.
// These cases pin both the message and the precedence between checks.
func TestValidateAgentProbeRoundsRejectionMessages(t *testing.T) {
	sample := func(seq int) AgentProbeSample {
		return AgentProbeSample{Seq: seq, Success: true}
	}
	round := func() AgentProbeRound {
		return AgentProbeRound{TargetID: "target-a", TS: 1000, Type: "tcp", Samples: []AgentProbeSample{sample(1)}}
	}
	nan := math.NaN()
	negative := -1.0

	cases := []struct {
		name    string
		request AgentProbeResultsRequest
		message string
	}{
		{
			name:    "no rounds",
			request: AgentProbeResultsRequest{},
			message: "rounds required",
		},
		{
			name:    "too many rounds",
			request: AgentProbeResultsRequest{Rounds: make([]AgentProbeRound, maxAgentProbeRounds+1)},
			message: "too many rounds",
		},
		{
			name: "invalid round id",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{RoundID: "bad id!", TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{sample(1)}},
			}},
			message: "invalid probe round id",
		},
		{
			name: "blank target",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "   ", TS: 1000, Samples: []AgentProbeSample{sample(1)}},
			}},
			message: "unknown target",
		},
		{
			name: "duplicate target",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				round(), round(),
			}},
			message: "duplicate target in probe batch",
		},
		{
			name: "non-positive timestamp",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 0, Samples: []AgentProbeSample{sample(1)}},
			}},
			message: "invalid timestamp",
		},
		{
			name: "too many samples",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: make([]AgentProbeSample, maxAgentProbeSamplesPerRound+1)},
			}},
			message: "too many samples",
		},
		{
			name: "negative sequence",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{{Seq: -1}}},
			}},
			message: "invalid sample sequence",
		},
		{
			name: "duplicate sequence",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{sample(2), sample(2)}},
			}},
			message: "duplicate sample sequence",
		},
		{
			name: "NaN latency",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{{Seq: 1, LatencyMS: &nan}}},
			}},
			message: "invalid sample latency",
		},
		{
			name: "negative latency",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{{Seq: 1, LatencyMS: &negative}}},
			}},
			message: "invalid sample latency",
		},
		{
			name: "sample error too long",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{
					{Seq: 1, Error: strings.Repeat("x", maxProbeErrorBytes+1)},
				}},
			}},
			message: "sample error too long",
		},
		{
			name: "no samples",
			request: AgentProbeResultsRequest{Rounds: []AgentProbeRound{
				{TargetID: "target-a", TS: 1000, Samples: nil},
			}},
			message: "samples required",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := validateAgentProbeRounds(testCase.request)
			if err == nil {
				t.Fatalf("expected rejection %q", testCase.message)
			}
			if err.message != testCase.message {
				t.Fatalf("message = %q, want %q", err.message, testCase.message)
			}
			if err.status != 400 {
				t.Fatalf("status = %d, want 400", err.status)
			}
		})
	}
}

// A duplicate round id must be rejected, but only when an id is supplied:
// omitting it is legal and must not collide with other rounds that also omit it.
func TestValidateAgentProbeRoundsRoundIDDeduplication(t *testing.T) {
	duplicate := AgentProbeResultsRequest{Rounds: []AgentProbeRound{
		{RoundID: "round-1", TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{{Seq: 1}}},
		{RoundID: "round-1", TargetID: "target-b", TS: 1000, Samples: []AgentProbeSample{{Seq: 1}}},
	}}
	if _, err := validateAgentProbeRounds(duplicate); err == nil || err.message != "duplicate probe round id" {
		t.Fatalf("want duplicate probe round id, got %v", err)
	}

	omitted := AgentProbeResultsRequest{Rounds: []AgentProbeRound{
		{TargetID: "target-a", TS: 1000, Samples: []AgentProbeSample{{Seq: 1}}},
		{TargetID: "target-b", TS: 1000, Samples: []AgentProbeSample{{Seq: 1}}},
	}}
	if _, err := validateAgentProbeRounds(omitted); err != nil {
		t.Fatalf("omitted round ids must be allowed: %v", err)
	}
}

// The request-wide error budget must be enforced across rounds, not just within
// one round, so a batch cannot smuggle a large payload past it by splitting.
func TestValidateAgentProbeRoundsEnforcesRequestWideErrorBudget(t *testing.T) {
	// Fill each round to just under its own budget, so no single round trips the
	// per-round limit and only the accumulated request total can fail.
	samplesPerRound := maxProbeErrorBytesPerRound / maxProbeErrorBytes
	if samplesPerRound < 1 {
		t.Skip("per-round budget is smaller than one sample error")
	}
	bytesPerRound := samplesPerRound * maxProbeErrorBytes
	roundsNeeded := (maxAgentProbeErrorBytesPerRequest / bytesPerRound) + 1
	if roundsNeeded > maxAgentProbeRounds {
		t.Skipf("request budget cannot be exceeded within %d rounds", maxAgentProbeRounds)
	}

	rounds := make([]AgentProbeRound, 0, roundsNeeded)
	for index := 0; index < roundsNeeded; index++ {
		samples := make([]AgentProbeSample, 0, samplesPerRound)
		for seq := 1; seq <= samplesPerRound; seq++ {
			samples = append(samples, AgentProbeSample{Seq: seq, Error: strings.Repeat("x", maxProbeErrorBytes)})
		}
		rounds = append(rounds, AgentProbeRound{
			TargetID: fmt.Sprintf("target-%d", index),
			TS:       1000,
			Samples:  samples,
		})
	}
	if _, err := validateAgentProbeRounds(AgentProbeResultsRequest{Rounds: rounds}); err == nil ||
		err.message != "probe error budget exceeded" {
		t.Fatalf("want probe error budget exceeded, got %v", err)
	}
}

// A valid batch must produce prepared rounds with normalised fields, a filled
// sequence for legacy agents, and a payload hash for idempotency.
func TestValidateAgentProbeRoundsPreparesRounds(t *testing.T) {
	latency := 12.5
	prepared, err := validateAgentProbeRounds(AgentProbeResultsRequest{Rounds: []AgentProbeRound{
		{
			RoundID:  " round-1 ",
			TargetID: " target-a ",
			TS:       1700000000,
			Type:     " tcp ",
			Samples: []AgentProbeSample{
				{Success: true, LatencyMS: &latency, Error: "  "},
				{Success: false, Error: "  timeout  "},
			},
		},
	}})
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if len(prepared) != 1 {
		t.Fatalf("prepared %d rounds, want 1", len(prepared))
	}
	round := prepared[0]
	if round.targetID != "target-a" || round.targetType != "tcp" || round.agentRoundID != "round-1" {
		t.Fatalf("fields were not trimmed: %+v", round)
	}
	if round.ts.Unix() != 1700000000 || round.ts.Location().String() != "UTC" {
		t.Fatalf("timestamp not normalised to UTC: %v", round.ts)
	}
	if len(round.samples) != 2 {
		t.Fatalf("sample count = %d, want 2", len(round.samples))
	}
	// Omitted sequences are filled from position, keeping ordering stable.
	if round.samples[0].Seq != 1 || round.samples[1].Seq != 2 {
		t.Fatalf("sequences not filled from position: %+v", round.samples)
	}
	if round.samples[0].Error != "" || round.samples[1].Error != "timeout" {
		t.Fatalf("sample errors not trimmed: %+v", round.samples)
	}
	if round.payloadHash == "" {
		t.Fatal("payload hash must be computed for idempotency")
	}
}
