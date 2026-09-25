import { describe, expect, it } from 'vitest'
import type { LatencyPoint } from '../types'
import { CHART_RENDER_BUDGET, decimateLatencyPoints } from './chartDecimate'

function point(targetId: string, ts: string, avgMs: number | null, lossPercent: number): LatencyPoint {
  return { ts, targetId, targetName: targetId, medianMs: avgMs, avgMs, lossPercent }
}

describe('decimateLatencyPoints', () => {
  it('passes small series through untouched', () => {
    const points = [point('a', '2026-07-02T12:00:00Z', 10, 0), point('a', '2026-07-02T12:01:00Z', 20, 0)]
    expect(decimateLatencyPoints(points)).toBe(points)
    expect(decimateLatencyPoints([])).toEqual([])
  })

  it('caps each target at the budget with bucket medians and mean loss', () => {
    const points: LatencyPoint[] = []
    for (let index = 0; index < CHART_RENDER_BUDGET + 80; index += 1) {
      const ts = `2026-07-02T${String(Math.floor(index / 60)).padStart(2, '0')}:${String(index % 60).padStart(2, '0')}:00Z`
      points.push(point('a', ts, index, index % 50 === 0 ? 10 : 0))
    }
    const thinned = decimateLatencyPoints(points)
    expect(thinned.length).toBeLessThanOrEqual(CHART_RENDER_BUDGET)
    expect(thinned.length).toBeGreaterThan(0)
    // 含丢包的桶取均值：10 混在 0 里变成 5，线呈波浪而非竖线。
    expect(thinned.some((item) => item.lossPercent === 5)).toBe(true)
    expect(thinned.every((item) => item.lossPercent <= 10)).toBe(true)
    // 时间单调。
    const times = thinned.map((item) => Date.parse(item.ts))
    expect([...times].sort((left, right) => left - right)).toEqual(times)
  })

  it('buckets each target separately and merges back in time order', () => {
    const points = [
      point('b', '2026-07-02T12:02:00Z', 30, 0),
      point('a', '2026-07-02T12:00:00Z', 10, 0),
      point('a', '2026-07-02T12:01:00Z', 20, 0),
    ]
    const thinned = decimateLatencyPoints(points, 1)
    expect(thinned.map((item) => item.targetId)).toEqual(['a', 'b'])
    expect(thinned[0].targetName).toBe('a')
    expect(thinned[0].avgMs).toBe(15)
  })
})
