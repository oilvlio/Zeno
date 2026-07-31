package api

import (
	"testing"
)

// connectionCount builds the optional counter pointer used by AgentStateRequest.
func connectionCount(value int64) *int64 { return &value }

// An invalid counter group must be stored as NULL, not zero. Zero would be
// indistinguishable from a real idle sample and would corrupt both public
// summaries and the traffic baseline.
func TestResolveAgentStateOptionalCountersNullsInvalidGroups(t *testing.T) {
	invalid := false
	valid := true

	t.Run("net totals invalid", func(t *testing.T) {
		counters := resolveAgentStateOptionalCounters(AgentStateRequest{
			NetInTotalBytes:  10,
			NetOutTotalBytes: 20,
			NetTotalsValid:   &invalid,
		})
		if counters.netInTotalBytes != nil || counters.netOutTotalBytes != nil {
			t.Fatalf("invalid net totals must become NULL: %+v", counters)
		}
	})

	t.Run("connection counts invalid", func(t *testing.T) {
		counters := resolveAgentStateOptionalCounters(AgentStateRequest{
			TCPConnectionCount:    connectionCount(5),
			UDPConnectionCount:    connectionCount(6),
			ConnectionCountsValid: &invalid,
		})
		if counters.tcpConnectionCount != nil || counters.udpConnectionCount != nil {
			t.Fatalf("invalid connection counts must become NULL: %+v", counters)
		}
	})

	t.Run("groups are independent", func(t *testing.T) {
		counters := resolveAgentStateOptionalCounters(AgentStateRequest{
			NetInTotalBytes:       10,
			NetOutTotalBytes:      20,
			TCPConnectionCount:    connectionCount(5),
			UDPConnectionCount:    connectionCount(6),
			NetTotalsValid:        &invalid,
			ConnectionCountsValid: &valid,
		})
		if counters.netInTotalBytes != nil {
			t.Fatal("net totals should be NULL")
		}
		if counters.tcpConnectionCount == nil || counters.udpConnectionCount == nil {
			t.Fatal("valid connection counts must be preserved")
		}
	})

	// Legacy agents omit the validity flags entirely; their values must be kept.
	t.Run("absent flags mean valid", func(t *testing.T) {
		counters := resolveAgentStateOptionalCounters(AgentStateRequest{
			NetInTotalBytes:    10,
			NetOutTotalBytes:   20,
			TCPConnectionCount: connectionCount(5),
			UDPConnectionCount: connectionCount(6),
		})
		if counters.netInTotalBytes == nil || counters.netOutTotalBytes == nil ||
			counters.tcpConnectionCount == nil || counters.udpConnectionCount == nil {
			t.Fatalf("absent validity flags must preserve values: %+v", counters)
		}
	})

	// An explicit true must behave the same as an absent flag.
	t.Run("explicit valid preserves values", func(t *testing.T) {
		counters := resolveAgentStateOptionalCounters(AgentStateRequest{
			NetInTotalBytes:       10,
			NetOutTotalBytes:      20,
			NetTotalsValid:        &valid,
			ConnectionCountsValid: &valid,
		})
		if counters.netInTotalBytes == nil || counters.netOutTotalBytes == nil {
			t.Fatalf("explicitly valid totals must be preserved: %+v", counters)
		}
	})
}
