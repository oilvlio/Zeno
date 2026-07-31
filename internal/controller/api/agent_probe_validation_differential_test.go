package api

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/shui1iao/zeno/internal/shared/probe"
)

// legacyValidateAgentProbeRounds is the pre-refactor validation loop, copied
// verbatim from the handler. It exists only so the extracted validator can be
// compared against it over generated input: this endpoint is on the agent hot
// path and its rejection messages are an external contract, so "looks
// equivalent" is not sufficient evidence.
func legacyValidateAgentProbeRounds(request AgentProbeResultsRequest) ([]preparedAgentProbeRound, string) {
	if len(request.Rounds) == 0 {
		return nil, "rounds required"
	}
	if len(request.Rounds) > maxAgentProbeRounds {
		return nil, "too many rounds"
	}
	prepared := make([]preparedAgentProbeRound, 0, len(request.Rounds))
	seenTargetIDs := make(map[string]struct{}, len(request.Rounds))
	seenRoundIDs := make(map[string]struct{}, len(request.Rounds))
	totalErrorBytes := 0
	for _, round := range request.Rounds {
		roundID := strings.TrimSpace(round.RoundID)
		if roundID != "" && !validAgentProbeRoundID(roundID) {
			return nil, "invalid probe round id"
		}
		if roundID != "" {
			if _, duplicate := seenRoundIDs[roundID]; duplicate {
				return nil, "duplicate probe round id"
			}
			seenRoundIDs[roundID] = struct{}{}
		}
		targetID := strings.TrimSpace(round.TargetID)
		if targetID == "" {
			return nil, "unknown target"
		}
		if _, duplicate := seenTargetIDs[targetID]; duplicate {
			return nil, "duplicate target in probe batch"
		}
		seenTargetIDs[targetID] = struct{}{}
		targetType := strings.TrimSpace(round.Type)
		if round.TS <= 0 {
			return nil, "invalid timestamp"
		}
		roundTS := time.Unix(round.TS, 0).UTC()
		if len(round.Samples) > maxAgentProbeSamplesPerRound {
			return nil, "too many samples"
		}
		samples := make([]probe.Sample, 0, len(round.Samples))
		seenSequences := make(map[int]struct{}, len(round.Samples))
		roundErrorBytes := 0
		for index, sample := range round.Samples {
			seq := sample.Seq
			if seq == 0 {
				seq = index + 1
			}
			if seq < 1 {
				return nil, "invalid sample sequence"
			}
			if _, duplicate := seenSequences[seq]; duplicate {
				return nil, "duplicate sample sequence"
			}
			seenSequences[seq] = struct{}{}
			latency := sample.LatencyMS
			if latency != nil {
				if math.IsNaN(*latency) || math.IsInf(*latency, 0) || *latency < 0 {
					return nil, "invalid sample latency"
				}
			}
			errorText := strings.TrimSpace(sample.Error)
			if len(errorText) > maxProbeErrorBytes {
				return nil, "sample error too long"
			}
			roundErrorBytes += len(errorText)
			totalErrorBytes += len(errorText)
			if roundErrorBytes > maxProbeErrorBytesPerRound || totalErrorBytes > maxAgentProbeErrorBytesPerRequest {
				return nil, "probe error budget exceeded"
			}
			samples = append(samples, probe.Sample{Seq: seq, Success: sample.Success, LatencyMS: latency, Error: errorText})
		}
		if len(samples) == 0 {
			return nil, "samples required"
		}
		preparedRound := preparedAgentProbeRound{targetID: targetID, targetType: targetType, ts: roundTS, agentRoundID: roundID, samples: samples}
		preparedRound.payloadHash = agentProbeRoundPayloadHash(preparedRound)
		prepared = append(prepared, preparedRound)
	}
	return prepared, ""
}

// The extracted validator must agree with the original on both the accept/reject
// decision and the exact message, including which check fires first when a
// request violates several rules at once.
func TestAgentProbeValidationMatchesLegacyImplementation(t *testing.T) {
	random := rand.New(rand.NewSource(20260731))

	roundIDs := []string{"", "  ", "round-1", " round-1 ", "bad id!", "round-2"}
	targetIDs := []string{"", "   ", "target-a", " target-a ", "target-b", "target-c"}
	timestamps := []int64{-1, 0, 1, 1700000000}
	types := []string{"", "tcp", " icmp "}

	nan := math.NaN()
	posInf := math.Inf(1)
	negInf := math.Inf(-1)
	negative := -0.5
	fine := 7.25
	latencies := []*float64{nil, &fine, &nan, &posInf, &negInf, &negative}
	errorTexts := []string{
		"", "   ", "timeout", "  timeout  ",
		strings.Repeat("x", maxProbeErrorBytes),
		strings.Repeat("x", maxProbeErrorBytes+1),
	}

	const iterations = 3000
	for iteration := 0; iteration < iterations; iteration++ {
		roundCount := random.Intn(4)
		rounds := make([]AgentProbeRound, 0, roundCount)
		for roundIndex := 0; roundIndex < roundCount; roundIndex++ {
			sampleCount := random.Intn(4)
			samples := make([]AgentProbeSample, 0, sampleCount)
			for sampleIndex := 0; sampleIndex < sampleCount; sampleIndex++ {
				samples = append(samples, AgentProbeSample{
					// Include 0 (auto-fill from position) and a negative value.
					Seq:       []int{0, 1, 2, -1}[random.Intn(4)],
					Success:   random.Intn(2) == 0,
					LatencyMS: latencies[random.Intn(len(latencies))],
					Error:     errorTexts[random.Intn(len(errorTexts))],
				})
			}
			rounds = append(rounds, AgentProbeRound{
				RoundID:  roundIDs[random.Intn(len(roundIDs))],
				TargetID: targetIDs[random.Intn(len(targetIDs))],
				TS:       timestamps[random.Intn(len(timestamps))],
				Type:     types[random.Intn(len(types))],
				Samples:  samples,
			})
		}
		request := AgentProbeResultsRequest{Rounds: rounds}

		wantPrepared, wantMessage := legacyValidateAgentProbeRounds(request)
		gotPrepared, gotErr := validateAgentProbeRounds(request)

		gotMessage := ""
		if gotErr != nil {
			gotMessage = gotErr.message
		}
		if gotMessage != wantMessage {
			t.Fatalf("iteration %d: message = %q, want %q\nrequest: %+v", iteration, gotMessage, wantMessage, request)
		}
		if wantMessage != "" {
			continue
		}
		if len(gotPrepared) != len(wantPrepared) {
			t.Fatalf("iteration %d: prepared %d rounds, want %d", iteration, len(gotPrepared), len(wantPrepared))
		}
		for index := range wantPrepared {
			want, got := wantPrepared[index], gotPrepared[index]
			if got.targetID != want.targetID || got.targetType != want.targetType ||
				got.agentRoundID != want.agentRoundID || !got.ts.Equal(want.ts) ||
				got.payloadHash != want.payloadHash {
				t.Fatalf("iteration %d round %d: %+v != %+v", iteration, index, got, want)
			}
			if len(got.samples) != len(want.samples) {
				t.Fatalf("iteration %d round %d: %d samples, want %d", iteration, index, len(got.samples), len(want.samples))
			}
			for sampleIndex := range want.samples {
				wantSample, gotSample := want.samples[sampleIndex], got.samples[sampleIndex]
				if gotSample.Seq != wantSample.Seq || gotSample.Success != wantSample.Success ||
					gotSample.Error != wantSample.Error {
					t.Fatalf("iteration %d round %d sample %d: %+v != %+v",
						iteration, index, sampleIndex, gotSample, wantSample)
				}
				if (gotSample.LatencyMS == nil) != (wantSample.LatencyMS == nil) {
					t.Fatalf("iteration %d: latency nil-ness differs", iteration)
				}
				if gotSample.LatencyMS != nil && *gotSample.LatencyMS != *wantSample.LatencyMS {
					t.Fatalf("iteration %d: latency %v != %v", iteration, *gotSample.LatencyMS, *wantSample.LatencyMS)
				}
			}
		}
	}
}
