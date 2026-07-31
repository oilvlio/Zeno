package api

import (
	"crypto/sha256"
	"math"
	"math/rand"
	"strings"
	"testing"
)

// legacyValidAgentStateValues is the pre-refactor condition, copied verbatim.
// It exists so the rule list can be proven equivalent over generated input:
// this check runs on every state report from every agent, and it is the only
// thing standing between a malformed payload and stored telemetry.
func legacyValidAgentStateValues(request AgentStateRequest) bool {
	return !(invalidFloat(request.CPUPercent) || request.CPUPercent < 0 || request.CPUPercent > 100 ||
		optionalFloatInvalidOrNegative(request.Load1) || optionalFloatInvalidOrNegative(request.Load5) ||
		optionalFloatInvalidOrNegative(request.Load15) || request.MemoryUsedBytes < 0 ||
		request.MemoryTotalBytes < 0 || optionalIntNegative(request.SwapUsedBytes) ||
		optionalIntNegative(request.SwapTotalBytes) || request.DiskUsedBytes < 0 ||
		request.DiskTotalBytes < 0 || request.NetInTotalBytes < 0 || request.NetOutTotalBytes < 0 ||
		!validNetworkCounterSource(request.NetCounterSource) || invalidFloat(request.NetInSpeedBps) ||
		request.NetInSpeedBps < 0 || invalidFloat(request.NetOutSpeedBps) || request.NetOutSpeedBps < 0 ||
		optionalIntNegative(request.ProcessCount) || optionalIntNegative(request.TCPConnectionCount) ||
		optionalIntNegative(request.UDPConnectionCount) || request.UptimeSeconds < 0)
}

// The rule list must accept and reject exactly what the original expression did.
func TestValidAgentStateValuesMatchesLegacyExpression(t *testing.T) {
	random := rand.New(rand.NewSource(20260731))

	nan := math.NaN()
	posInf := math.Inf(1)
	negInf := math.Inf(-1)
	negative := -1.0
	zero := 0.0
	normal := 1.5
	optionalFloats := []*float64{nil, &zero, &normal, &negative, &nan, &posInf, &negInf}

	negativeInt := int64(-1)
	zeroInt := int64(0)
	normalInt := int64(42)
	optionalInts := []*int64{nil, &zeroInt, &normalInt, &negativeInt}

	floats := []float64{0, 1.5, 100, 100.1, -0.1, nan, posInf, negInf}
	ints := []int64{0, 1, -1, 1 << 40}
	// The counter source must be empty or a hex sha256 digest; anything else is
	// rejected outright, so the valid forms have to be represented explicitly.
	validSource := strings.Repeat("ab", sha256.Size)
	sources := []string{"", validSource, "proc", "unknown-source", "  ", strings.ToUpper(validSource)}

	// Each field is usually valid so that whole-request acceptance is reachable;
	// with 19 independent fields, uniform sampling would reject essentially every
	// generated request and the comparison would only ever exercise one branch.
	const validBias = 4
	pickFloat := func() float64 {
		if random.Intn(validBias) != 0 {
			return []float64{0, 1.5, 100}[random.Intn(3)]
		}
		return floats[random.Intn(len(floats))]
	}
	pickInt := func() int64 {
		if random.Intn(validBias) != 0 {
			return []int64{0, 1, 1 << 40}[random.Intn(3)]
		}
		return ints[random.Intn(len(ints))]
	}
	pickOptionalFloat := func() *float64 {
		if random.Intn(validBias) != 0 {
			return []*float64{nil, &zero, &normal}[random.Intn(3)]
		}
		return optionalFloats[random.Intn(len(optionalFloats))]
	}
	pickOptionalInt := func() *int64 {
		if random.Intn(validBias) != 0 {
			return []*int64{nil, &zeroInt, &normalInt}[random.Intn(3)]
		}
		return optionalInts[random.Intn(len(optionalInts))]
	}
	pickSource := func() string {
		if random.Intn(validBias) != 0 {
			return []string{"", validSource}[random.Intn(2)]
		}
		return sources[random.Intn(len(sources))]
	}

	const iterations = 5000
	accepted := 0
	for iteration := 0; iteration < iterations; iteration++ {
		request := AgentStateRequest{
			CPUPercent:         pickFloat(),
			Load1:              pickOptionalFloat(),
			Load5:              pickOptionalFloat(),
			Load15:             pickOptionalFloat(),
			MemoryUsedBytes:    pickInt(),
			MemoryTotalBytes:   pickInt(),
			SwapUsedBytes:      pickOptionalInt(),
			SwapTotalBytes:     pickOptionalInt(),
			DiskUsedBytes:      pickInt(),
			DiskTotalBytes:     pickInt(),
			NetInTotalBytes:    pickInt(),
			NetOutTotalBytes:   pickInt(),
			NetCounterSource:   pickSource(),
			NetInSpeedBps:      pickFloat(),
			NetOutSpeedBps:     pickFloat(),
			ProcessCount:       pickOptionalInt(),
			TCPConnectionCount: pickOptionalInt(),
			UDPConnectionCount: pickOptionalInt(),
			UptimeSeconds:      pickInt(),
		}
		want := legacyValidAgentStateValues(request)
		got := validAgentStateValues(request)
		if got != want {
			t.Fatalf("iteration %d: got %v, want %v for %+v", iteration, got, want, request)
		}
		if want {
			accepted++
		}
	}
	// Guard against a generator that only ever produces rejections, which would
	// make the comparison above pass without exercising the accept path.
	if accepted == 0 {
		t.Fatal("no generated request was accepted; the comparison proved nothing")
	}
	t.Logf("compared %d requests, %d accepted", iterations, accepted)
}

// A fully valid report must be accepted, and every single rule must be able to
// reject on its own so no rule is dead.
func TestAgentStateValueRulesRejectIndividually(t *testing.T) {
	valid := AgentStateRequest{
		CPUPercent:       12.5,
		MemoryUsedBytes:  1024,
		MemoryTotalBytes: 4096,
		DiskUsedBytes:    2048,
		DiskTotalBytes:   8192,
		NetInTotalBytes:  100,
		NetOutTotalBytes: 200,
		NetInSpeedBps:    10,
		NetOutSpeedBps:   20,
		UptimeSeconds:    3600,
	}
	if !validAgentStateValues(valid) {
		t.Fatal("a report with only required fields must be accepted")
	}

	nan := math.NaN()
	negativeFloat := -1.0
	negativeInt := int64(-1)

	breaks := map[string]func(*AgentStateRequest){
		"cpu percent":          func(r *AgentStateRequest) { r.CPUPercent = 101 },
		"cpu nan":              func(r *AgentStateRequest) { r.CPUPercent = nan },
		"load1":                func(r *AgentStateRequest) { r.Load1 = &negativeFloat },
		"load5":                func(r *AgentStateRequest) { r.Load5 = &negativeFloat },
		"load15":               func(r *AgentStateRequest) { r.Load15 = &nan },
		"memory used":          func(r *AgentStateRequest) { r.MemoryUsedBytes = -1 },
		"memory total":         func(r *AgentStateRequest) { r.MemoryTotalBytes = -1 },
		"swap used":            func(r *AgentStateRequest) { r.SwapUsedBytes = &negativeInt },
		"swap total":           func(r *AgentStateRequest) { r.SwapTotalBytes = &negativeInt },
		"disk used":            func(r *AgentStateRequest) { r.DiskUsedBytes = -1 },
		"disk total":           func(r *AgentStateRequest) { r.DiskTotalBytes = -1 },
		"net in total":         func(r *AgentStateRequest) { r.NetInTotalBytes = -1 },
		"net out total":        func(r *AgentStateRequest) { r.NetOutTotalBytes = -1 },
		"net counter source":   func(r *AgentStateRequest) { r.NetCounterSource = "not-a-source" },
		"net in speed":         func(r *AgentStateRequest) { r.NetInSpeedBps = -1 },
		"net out speed":        func(r *AgentStateRequest) { r.NetOutSpeedBps = nan },
		"process count":        func(r *AgentStateRequest) { r.ProcessCount = &negativeInt },
		"tcp connection count": func(r *AgentStateRequest) { r.TCPConnectionCount = &negativeInt },
		"udp connection count": func(r *AgentStateRequest) { r.UDPConnectionCount = &negativeInt },
		"uptime":               func(r *AgentStateRequest) { r.UptimeSeconds = -1 },
	}
	for name, apply := range breaks {
		t.Run(name, func(t *testing.T) {
			request := valid
			apply(&request)
			if validAgentStateValues(request) {
				t.Fatalf("%s must be rejected", name)
			}
			if legacyValidAgentStateValues(request) {
				t.Fatalf("%s: legacy expression disagrees", name)
			}
		})
	}
}

// Absent optional fields mean "not collected" and must stay legal, otherwise
// agents that cannot read swap or connection counts would break.
func TestAgentStateValueRulesAllowAbsentOptionalFields(t *testing.T) {
	request := AgentStateRequest{CPUPercent: 1}
	if !validAgentStateValues(request) {
		t.Fatal("absent optional fields must be accepted")
	}
	for _, rule := range agentStateValueRules(request) {
		if !rule.valid() {
			t.Fatalf("rule %q rejected an all-absent report", rule.name)
		}
	}
}
