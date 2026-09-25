import type { LatencyPoint } from '../types'

// 图表渲染压点：单条线超过预算时按桶取中值（延迟）/取均值（丢包），
// 点数下来、线变波浪。严重丢包事件不靠线保留——LossChart 会在均值线
// 冲上 25% 的 rising-edge 处打点标记。统计汇总继续用原始全量点，这里只喂图。
export const CHART_RENDER_BUDGET = 720
// 丢包率 curve 单独更小的预算：1 天 1440 点压成 360（约 4 分钟一步），
// 单桶 100% 稀释成 25% 正好落在打点阈值上，事件不丢。
export const LOSS_CHART_RENDER_BUDGET = 360

function medianOf(values: number[]): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((left, right) => left - right)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 1 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

function meanOf(values: number[]): number | null {
  if (values.length === 0) return null
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function finiteValues(values: Array<number | null | undefined>): number[] {
  return values.filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
}

export function decimateLatencyPoints(points: LatencyPoint[], budget = CHART_RENDER_BUDGET): LatencyPoint[] {
  // 总数不超预算时没有任何分组能超，直接原样返回（引用稳定，下游 memo 不失效）。
  if (points.length === 0 || budget < 1 || points.length <= budget) return points
  const order: string[] = []
  const groups = new Map<string, LatencyPoint[]>()
  for (const point of points) {
    let group = groups.get(point.targetId)
    if (!group) {
      group = []
      groups.set(point.targetId, group)
      order.push(point.targetId)
    }
    group.push(point)
  }

  const thinned: LatencyPoint[] = []
  for (const targetId of order) {
    const group = groups.get(targetId)!
    if (group.length <= budget) {
      thinned.push(...group)
      continue
    }
    const bucketSize = Math.ceil(group.length / budget)
    for (let start = 0; start < group.length; start += bucketSize) {
      const bucket = group.slice(start, start + bucketSize)
      const last = bucket[bucket.length - 1]
      thinned.push({
        ts: last.ts,
        targetId,
        targetName: bucket[0].targetName,
        medianMs: medianOf(finiteValues(bucket.map((point) => point.medianMs))),
        avgMs: medianOf(finiteValues(bucket.map((point) => point.avgMs))),
        lossPercent: meanOf(finiteValues(bucket.map((point) => point.lossPercent))) ?? 0,
      })
    }
  }

  return thinned
    .map((point, index) => ({ point, index, time: Date.parse(point.ts) }))
    .sort((left, right) => {
      if (left.time !== right.time) return left.time - right.time
      return left.index - right.index
    })
    .map(({ point }) => point)
}
