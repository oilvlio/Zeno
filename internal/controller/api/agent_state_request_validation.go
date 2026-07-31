package api

// validAgentStateValues reports whether every telemetry field is within range.
//
// This was one boolean expression with ~25 clauses, which made the handler's
// complexity dominated by a single condition and gave a rejected report no
// indication of which field was wrong. The checks are now a list of named rules
// evaluated in the same order.
//
// The result stays a plain bool because the wire contract is a single
// "invalid state values" message: an agent must not be able to probe which
// field it got wrong. The rule names exist for readability and testing.
func validAgentStateValues(request AgentStateRequest) bool {
	for _, rule := range agentStateValueRules(request) {
		if !rule.valid() {
			return false
		}
	}
	return true
}

// agentStateValueRule is one named range check.
type agentStateValueRule struct {
	name  string
	valid func() bool
}

// agentStateValueRules lists every range rule applied to a state report.
//
// Counters are monotonic byte/second totals, so negative values are always
// malformed rather than merely unusual. Optional fields are only checked when
// present: absent is "not collected", which is legal.
func agentStateValueRules(request AgentStateRequest) []agentStateValueRule {
	return []agentStateValueRule{
		// CPU is a percentage, so it is bounded on both sides; a value above 100
		// means the agent computed it wrong rather than that the host is busy.
		{"cpu percent", func() bool {
			return !invalidFloat(request.CPUPercent) && request.CPUPercent >= 0 && request.CPUPercent <= 100
		}},
		{"load1", func() bool { return !optionalFloatInvalidOrNegative(request.Load1) }},
		{"load5", func() bool { return !optionalFloatInvalidOrNegative(request.Load5) }},
		{"load15", func() bool { return !optionalFloatInvalidOrNegative(request.Load15) }},
		{"memory used", func() bool { return request.MemoryUsedBytes >= 0 }},
		{"memory total", func() bool { return request.MemoryTotalBytes >= 0 }},
		{"swap used", func() bool { return !optionalIntNegative(request.SwapUsedBytes) }},
		{"swap total", func() bool { return !optionalIntNegative(request.SwapTotalBytes) }},
		{"disk used", func() bool { return request.DiskUsedBytes >= 0 }},
		{"disk total", func() bool { return request.DiskTotalBytes >= 0 }},
		{"net in total", func() bool { return request.NetInTotalBytes >= 0 }},
		{"net out total", func() bool { return request.NetOutTotalBytes >= 0 }},
		// The counter source is recorded so traffic accounting can tell whether a
		// baseline may be carried across agent restarts.
		{"net counter source", func() bool { return validNetworkCounterSource(request.NetCounterSource) }},
		{"net in speed", func() bool {
			return !invalidFloat(request.NetInSpeedBps) && request.NetInSpeedBps >= 0
		}},
		{"net out speed", func() bool {
			return !invalidFloat(request.NetOutSpeedBps) && request.NetOutSpeedBps >= 0
		}},
		{"process count", func() bool { return !optionalIntNegative(request.ProcessCount) }},
		{"tcp connection count", func() bool { return !optionalIntNegative(request.TCPConnectionCount) }},
		{"udp connection count", func() bool { return !optionalIntNegative(request.UDPConnectionCount) }},
		{"uptime", func() bool { return request.UptimeSeconds >= 0 }},
	}
}
