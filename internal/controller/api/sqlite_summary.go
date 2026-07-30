package api

import (
	"context"
	"time"
)

func (s *SQLiteStore) Summary(ctx context.Context) (SummaryResponse, error) {
	nodes, err := s.nodes(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	if nodes == nil {
		nodes = []Node{}
	}
	homeSummaries, services, latencySummaries, err := s.summaryAggregates(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	if services == nil {
		services = []ServiceTarget{}
	}
	exchangeRates, err := s.exchangeRateSnapshot(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	for index := range nodes {
		nodes[index].LatencySummary = homeSummaries[nodes[index].ID]
		if summaries, ok := latencySummaries[nodes[index].ID]; ok {
			nodes[index].LatencySummaries = summaries
		} else {
			nodes[index].LatencySummaries = []LatencySummary{}
		}
	}
	return SummaryResponse{Nodes: nodes, Services: services, LatencyPoints: []LatencyPoint{}, ExchangeRates: exchangeRates}, nil
}

func (s *SQLiteStore) summaryAggregates(ctx context.Context) (map[string]*LatencySummary, []ServiceTarget, map[string][]LatencySummary, error) {
	retries := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		now := time.Now()
		s.summaryAggregateMu.Lock()
		if !s.summaryAggregateUpdated.IsZero() && now.Sub(s.summaryAggregateUpdated) < summaryAggregateFreshFor {
			home := s.summaryAggregateHome
			services := s.summaryAggregateServices
			nodeLatency := s.summaryAggregateNodeLatency
			s.summaryAggregateMu.Unlock()
			return home, services, nodeLatency, nil
		}
		generation := s.summaryAggregateGeneration
		if flight := s.summaryAggregateFlight; flight != nil {
			s.summaryAggregateMu.Unlock()
			select {
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			case <-flight.done:
			}
			s.summaryAggregateMu.Lock()
			currentGeneration := s.summaryAggregateGeneration
			s.summaryAggregateMu.Unlock()
			if currentGeneration != flight.generation {
				if retries < summaryGenerationMaxRetries {
					retries++
					continue
				}
				return flight.home, flight.services, flight.nodeLatency, flight.err
			}
			return flight.home, flight.services, flight.nodeLatency, flight.err
		}
		flight := &summaryAggregateFlight{generation: generation, done: make(chan struct{})}
		s.summaryAggregateFlight = flight
		s.summaryAggregateMu.Unlock()

		// These are the expensive rolling queries. They intentionally run outside
		// summaryAggregateMu so Agent probe writes can return 202 without waiting
		// for a 24-hour scan.
		homeSummaries, err := s.latestHomeLatencySummaries(ctx)
		var services []ServiceTarget
		if err == nil {
			services, err = s.serviceTargets(ctx)
		}
		var latencySummaries map[string][]LatencySummary
		if err == nil {
			latencySummaries, err = s.latestLatencySummariesByNode(ctx)
		}

		s.summaryAggregateMu.Lock()
		currentGeneration := s.summaryAggregateGeneration
		if err == nil && currentGeneration == generation {
			s.summaryAggregateHome = homeSummaries
			s.summaryAggregateServices = services
			s.summaryAggregateNodeLatency = latencySummaries
			s.summaryAggregateUpdated = time.Now()
		}
		flight.home = homeSummaries
		flight.services = services
		flight.nodeLatency = latencySummaries
		flight.err = err
		if s.summaryAggregateFlight == flight {
			s.summaryAggregateFlight = nil
		}
		close(flight.done)
		s.summaryAggregateMu.Unlock()

		if currentGeneration != generation {
			// Invalidation raced this snapshot. Do not let it repopulate the cache;
			// retry once in the current generation. If writes continue, the second
			// completed query is still a usable point-in-time snapshot and must be
			// returned rather than allowing unbounded aggregate rebuilds.
			if retries < summaryGenerationMaxRetries {
				retries++
				continue
			}
			return homeSummaries, services, latencySummaries, err
		}
		return homeSummaries, services, latencySummaries, err
	}
}

func (s *SQLiteStore) invalidateSummaryAggregates() {
	s.summaryAggregateMu.Lock()
	s.summaryAggregateGeneration++
	s.summaryAggregateUpdated = time.Time{}
	s.summaryAggregateHome = nil
	s.summaryAggregateServices = nil
	s.summaryAggregateNodeLatency = nil
	s.summaryAggregateMu.Unlock()
}

func (s *SQLiteStore) NodeLatency(ctx context.Context, nodeID string, window latencyWindow) (LatencyResponse, error) {
	exists, err := s.nodeExists(ctx, nodeID)
	if err != nil {
		return LatencyResponse{}, err
	}
	if !exists {
		return LatencyResponse{}, errNodeNotFound
	}
	points, err := s.latencyPoints(ctx, nodeID, window)
	if err != nil {
		return LatencyResponse{}, err
	}
	return LatencyResponse{NodeID: nodeID, Range: window.Name, Points: points}, nil
}

func (s *SQLiteStore) ServiceTargetLatency(ctx context.Context, targetID string, window latencyWindow) (ServiceTargetLatencyResponse, error) {
	target, err := s.serviceTargetByID(ctx, targetID)
	if err != nil {
		return ServiceTargetLatencyResponse{}, err
	}
	points, err := s.serviceLatencyPoints(ctx, targetID, window)
	if err != nil {
		return ServiceTargetLatencyResponse{}, err
	}
	return ServiceTargetLatencyResponse{Target: target, Range: window.Name, Points: points}, nil
}

func (s *SQLiteStore) NodeState(ctx context.Context, nodeID string, window latencyWindow) (StateResponse, error) {
	exists, err := s.nodeExists(ctx, nodeID)
	if err != nil {
		return StateResponse{}, err
	}
	if !exists {
		return StateResponse{}, errNodeNotFound
	}
	points, err := s.statePoints(ctx, nodeID, window)
	if err != nil {
		return StateResponse{}, err
	}
	return StateResponse{NodeID: nodeID, Range: window.Name, Points: points}, nil
}

const activeAdminNodeExistsSQL = `
	SELECT 1 FROM nodes n
	WHERE n.id = ?
	  AND NOT EXISTS (
		SELECT 1 FROM admin_deletion_jobs deletion
		WHERE deletion.entity_kind = 'node'
		  AND deletion.entity_id = n.id
		  AND deletion.state IN ('pending', 'running')
	  )
`

const activeAdminProbeTargetExistsSQL = `
	SELECT 1 FROM probe_targets pt
	WHERE pt.id = ?
	  AND NOT EXISTS (
		SELECT 1 FROM admin_deletion_jobs deletion
		WHERE deletion.entity_kind = 'probe_target'
		  AND deletion.entity_id = pt.id
		  AND deletion.state IN ('pending', 'running')
	  )
`
