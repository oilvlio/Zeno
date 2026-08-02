import { describe, expect, it } from 'vitest'
import { seedNodeLatencyFromSummary, shouldApplyNodeLatencySnapshot } from './useNodeDetailController'

describe('shouldApplyNodeLatencySnapshot', () => {
  it('suppresses an identical HTTP/WebSocket snapshot but accepts changed data', () => {
    const seen = new Map<string, string>()

    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-a')).toBe(true)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-a')).toBe(false)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-b')).toBe(true)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-b:1d', 'snapshot-a')).toBe(true)
  })

  it('keeps legacy uncached payloads applicable', () => {
    expect(shouldApplyNodeLatencySnapshot(new Map(), 'node-a:1d')).toBe(true)
  })
})

describe('seedNodeLatencyFromSummary', () => {
  it('uses the homepage hourly history as an immediately drawable chart preview', () => {
    const seeded = seedNodeLatencyFromSummary({
      nodes: [{
        id: 'node-a',
        latencySummary: {
          targetId: 'primary',
          targetName: 'Primary',
          medianMs: 20,
          lossPercent: 0,
          updatedAt: '2026-08-02T08:00:00Z',
          hourlyHistory: [
            { startedAt: '2026-08-02T06:00:00Z', latencyMs: 18, lossPercent: 0 },
            { startedAt: '2026-08-02T07:00:00Z', latencyMs: 19, lossPercent: 0 },
          ],
        },
        latencySummaries: [
          { targetId: 'primary', targetName: 'Primary', medianMs: 20, lossPercent: 0, updatedAt: '2026-08-02T08:00:00Z' },
          { targetId: 'secondary', targetName: 'Secondary', medianMs: 30, lossPercent: 0, updatedAt: '2026-08-02T08:00:00Z' },
        ],
      }],
      services: [],
    } as never, 'node-a', '1d')

    expect(seeded?.points.filter((point) => point.targetId === 'primary')).toHaveLength(2)
    expect(seeded?.points.find((point) => point.targetId === 'primary')?.medianMs).toBe(18)
    expect(seeded?.points.find((point) => point.targetId === 'secondary')?.medianMs).toBe(30)
  })
})
