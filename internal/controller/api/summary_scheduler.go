package api

import (
	"context"
	"time"
)

func (h *handler) publishSummary(_ context.Context) {
	if h == nil || h.liveHub == nil || !h.liveHub.hasClients(summaryLiveTopic) {
		return
	}
	h.scheduleSummaryPublish()
}

func (h *handler) scheduleSummaryPublish() {
	if h == nil {
		return
	}
	h.scheduleSummaryTimer(
		&h.summaryPublishTimer,
		func(now time.Time) time.Duration {
			wait := summaryPublishCoalesceDelay
			if !h.summaryLastPublished.IsZero() {
				if minWait := h.summaryLastPublished.Add(summaryPublishMinInterval).Sub(now); minWait > wait {
					wait = minWait
				}
			}
			return wait
		},
		func() { h.summaryLastPublished = time.Now() },
		h.publishSummaryNow,
	)
}

func (h *handler) scheduleSummaryRefreshAfter(delay time.Duration) {
	if h == nil {
		return
	}
	h.scheduleSummaryTimer(
		&h.summaryRefreshTimer,
		func(time.Time) time.Duration { return delay },
		nil,
		h.refreshSummaryCacheNow,
	)
}

// scheduleSummaryTimer owns the common timer/background lifecycle while each
// caller keeps an independent timer slot. Cache refreshes can therefore run on
// their short SWR deadline without changing the WebSocket publication cadence.
func (h *handler) scheduleSummaryTimer(slot **time.Timer, delay func(time.Time) time.Duration, fired func(), run func(context.Context)) {
	if h == nil || slot == nil || delay == nil || run == nil || h.backgroundContext().Err() != nil {
		return
	}
	h.summaryScheduleMu.Lock()
	if *slot != nil {
		h.summaryScheduleMu.Unlock()
		return
	}
	wait := delay(time.Now())
	if wait < 0 {
		wait = 0
	}
	timer := time.NewTimer(wait)
	*slot = timer
	backgroundCtx, ok := h.beginBackground()
	if !ok {
		timer.Stop()
		*slot = nil
		h.summaryScheduleMu.Unlock()
		return
	}
	h.summaryScheduleMu.Unlock()

	go func() {
		defer h.backgroundWG.Done()
		if !waitForSummaryTimer(backgroundCtx, timer) {
			h.summaryScheduleMu.Lock()
			if *slot == timer {
				*slot = nil
			}
			h.summaryScheduleMu.Unlock()
			return
		}

		h.summaryScheduleMu.Lock()
		if *slot != timer {
			h.summaryScheduleMu.Unlock()
			return
		}
		*slot = nil
		if fired != nil {
			fired()
		}
		h.summaryScheduleMu.Unlock()

		ctx, cancel := context.WithTimeout(backgroundCtx, 5*time.Second)
		defer cancel()
		run(ctx)
	}()
}

func waitForSummaryTimer(ctx context.Context, timer *time.Timer) bool {
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		return false
	case <-timer.C:
		return true
	}
}

func (h *handler) refreshSummaryCacheNow(ctx context.Context) {
	_, _ = h.loadSummaryJSON(ctx, summaryCacheHTTPFreshFor, true)
}

func (h *handler) publishSummaryNow(ctx context.Context) {
	payload, err := h.summaryJSON(ctx)
	if err != nil {
		return
	}
	if h.liveHub != nil && h.liveHub.hasClients(summaryLiveTopic) {
		h.liveHub.publish(summaryLiveTopic, payload)
	}
}

func (h *handler) publishSummaryNowFresh(ctx context.Context) {
	h.invalidateSummaryAggregates()
	h.invalidateSummaryCache()
	// Cache invalidation must be synchronous so the next HTTP read is fresh, but
	// rebuilding and broadcasting the summary should not hold up an admin save.
	h.publishSummary(ctx)
}
