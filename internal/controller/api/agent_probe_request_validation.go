package api

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/shui1iao/zeno/internal/shared/probe"
)

// agentProbeValidationError is a rejected probe batch, carrying the exact
// status and message the handler should return.
//
// Validation is separated from the handler so the rules can be tested directly
// against a request value instead of through an HTTP round trip. The message is
// part of the agent-facing contract, so it travels with the error rather than
// being reconstructed at the call site.
type agentProbeValidationError struct {
	status  int
	message string
}

func rejectAgentProbe(message string) *agentProbeValidationError {
	return &agentProbeValidationError{status: http.StatusBadRequest, message: message}
}

// validateAgentProbeRounds validates a probe batch and returns the prepared
// rounds ready for insertion.
//
// Checks run in the same order as the wire format is read (batch limits, then
// per round, then per sample) so a malformed request always reports its first
// problem, which keeps agent-side debugging deterministic.
func validateAgentProbeRounds(request AgentProbeResultsRequest) ([]preparedAgentProbeRound, *agentProbeValidationError) {
	if len(request.Rounds) == 0 {
		return nil, rejectAgentProbe("rounds required")
	}
	if len(request.Rounds) > maxAgentProbeRounds {
		return nil, rejectAgentProbe("too many rounds")
	}

	prepared := make([]preparedAgentProbeRound, 0, len(request.Rounds))
	seenTargetIDs := make(map[string]struct{}, len(request.Rounds))
	seenRoundIDs := make(map[string]struct{}, len(request.Rounds))
	// The error budget is enforced per round and across the whole request, so a
	// batch cannot smuggle in a large payload by spreading it over many rounds.
	totalErrorBytes := 0

	for _, round := range request.Rounds {
		preparedRound, updatedTotal, err := validateAgentProbeRound(round, seenRoundIDs, seenTargetIDs, totalErrorBytes)
		if err != nil {
			return nil, err
		}
		totalErrorBytes = updatedTotal
		prepared = append(prepared, preparedRound)
	}
	return prepared, nil
}

// validateAgentProbeRound validates one round, recording its identifiers in the
// caller's dedupe sets. It carries the request-wide error-byte total through so
// both budgets are enforced at the same point as the wire format is read.
func validateAgentProbeRound(
	round AgentProbeRound,
	seenRoundIDs, seenTargetIDs map[string]struct{},
	totalErrorBytes int,
) (preparedAgentProbeRound, int, *agentProbeValidationError) {
	roundID := strings.TrimSpace(round.RoundID)
	if roundID != "" {
		if !validAgentProbeRoundID(roundID) {
			return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("invalid probe round id")
		}
		if _, duplicate := seenRoundIDs[roundID]; duplicate {
			return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("duplicate probe round id")
		}
		seenRoundIDs[roundID] = struct{}{}
	}

	targetID := strings.TrimSpace(round.TargetID)
	if targetID == "" {
		return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("unknown target")
	}
	if _, duplicate := seenTargetIDs[targetID]; duplicate {
		return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("duplicate target in probe batch")
	}
	seenTargetIDs[targetID] = struct{}{}

	if round.TS <= 0 {
		return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("invalid timestamp")
	}
	if len(round.Samples) > maxAgentProbeSamplesPerRound {
		return preparedAgentProbeRound{}, totalErrorBytes, rejectAgentProbe("too many samples")
	}

	samples, updatedTotal, err := validateAgentProbeSamples(round.Samples, totalErrorBytes)
	if err != nil {
		return preparedAgentProbeRound{}, updatedTotal, err
	}
	if len(samples) == 0 {
		return preparedAgentProbeRound{}, updatedTotal, rejectAgentProbe("samples required")
	}

	preparedRound := preparedAgentProbeRound{
		targetID:     targetID,
		targetType:   strings.TrimSpace(round.Type),
		ts:           time.Unix(round.TS, 0).UTC(),
		agentRoundID: roundID,
		samples:      samples,
	}
	preparedRound.payloadHash = agentProbeRoundPayloadHash(preparedRound)
	return preparedRound, updatedTotal, nil
}

// validateAgentProbeSamples validates one round's samples and returns them
// alongside the updated request-wide error-byte total.
//
// Both the per-round and the request-wide budget are checked after each sample,
// matching the original single-pass loop: a batch that exceeds either budget is
// rejected at the sample that crosses it, so the reported error does not depend
// on how the rounds were split.
//
// A missing sequence number is filled from the sample's position so legacy
// agents that omit it still produce a stable, duplicate-free ordering.
func validateAgentProbeSamples(rawSamples []AgentProbeSample, totalErrorBytes int) ([]probe.Sample, int, *agentProbeValidationError) {
	samples := make([]probe.Sample, 0, len(rawSamples))
	seenSequences := make(map[int]struct{}, len(rawSamples))
	roundErrorBytes := 0

	for index, sample := range rawSamples {
		seq := sample.Seq
		if seq == 0 {
			seq = index + 1
		}
		if seq < 1 {
			return nil, totalErrorBytes, rejectAgentProbe("invalid sample sequence")
		}
		if _, duplicate := seenSequences[seq]; duplicate {
			return nil, totalErrorBytes, rejectAgentProbe("duplicate sample sequence")
		}
		seenSequences[seq] = struct{}{}

		latency := sample.LatencyMS
		if latency != nil && (math.IsNaN(*latency) || math.IsInf(*latency, 0) || *latency < 0) {
			return nil, totalErrorBytes, rejectAgentProbe("invalid sample latency")
		}

		errorText := strings.TrimSpace(sample.Error)
		if len(errorText) > maxProbeErrorBytes {
			return nil, totalErrorBytes, rejectAgentProbe("sample error too long")
		}
		roundErrorBytes += len(errorText)
		totalErrorBytes += len(errorText)
		if roundErrorBytes > maxProbeErrorBytesPerRound || totalErrorBytes > maxAgentProbeErrorBytesPerRequest {
			return nil, totalErrorBytes, rejectAgentProbe("probe error budget exceeded")
		}

		samples = append(samples, probe.Sample{
			Seq:       seq,
			Success:   sample.Success,
			LatencyMS: latency,
			Error:     errorText,
		})
	}
	return samples, totalErrorBytes, nil
}
