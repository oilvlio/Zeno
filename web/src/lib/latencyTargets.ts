import type { LatencyPoint } from '../types'

export interface LatencyTargetSummary {
  targetId: string
  targetName: string
  sampleCount: number
  avgMs: number | null
  medianMs: number | null
  minMs: number | null
  maxMs: number | null
  lossPercent: number
}

interface Accumulator {
  targetId: string
  targetName: string
  sampleCount: number
  latestDelay: number | null
  latestMedian: number | null
  minDelay: number | null
  maxDelay: number | null
  lossTotal: number
  lossCount: number
}

export function summarizeLatencyTargets(points: LatencyPoint[]): LatencyTargetSummary[] {
  const byTarget = new Map<string, Accumulator>()

  for (const point of points) {
    const existing = byTarget.get(point.targetId)
    const acc = existing ?? {
      targetId: point.targetId,
      targetName: point.targetName,
      sampleCount: 0,
      latestDelay: null,
      latestMedian: null,
      minDelay: null,
      maxDelay: null,
      lossTotal: 0,
      lossCount: 0,
    }

    acc.targetName = point.targetName
    const delay = point.avgMs
    const hasDelay = typeof delay === 'number' && Number.isFinite(delay)
    const missingBucket = !hasDelay && point.lossPercent === 0
    if (!missingBucket) acc.sampleCount += 1
    if (hasDelay) {
      acc.latestDelay = delay
      acc.minDelay = acc.minDelay === null ? delay : Math.min(acc.minDelay, delay)
      acc.maxDelay = acc.maxDelay === null ? delay : Math.max(acc.maxDelay, delay)
    }
    const median = point.medianMs
    if (typeof median === 'number' && Number.isFinite(median)) {
      acc.latestMedian = median
    }
    if (!missingBucket && Number.isFinite(point.lossPercent)) {
      acc.lossTotal += point.lossPercent
      acc.lossCount += 1
    }

    if (!existing) byTarget.set(point.targetId, acc)
  }

  return [...byTarget.values()].map((acc) => ({
    targetId: acc.targetId,
    targetName: acc.targetName,
    sampleCount: acc.sampleCount,
    avgMs: acc.latestDelay !== null ? round2(acc.latestDelay) : null,
    medianMs: acc.latestMedian !== null ? round2(acc.latestMedian) : null,
    minMs: acc.minDelay !== null ? round2(acc.minDelay) : null,
    maxMs: acc.maxDelay !== null ? round2(acc.maxDelay) : null,
    lossPercent: acc.lossCount > 0 ? round2(acc.lossTotal / acc.lossCount) : 0,
  }))
}

function round2(value: number): number {
  return Math.round(value * 100) / 100
}
