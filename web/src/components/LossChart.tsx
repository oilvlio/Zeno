import { useEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent } from 'react'
import type { LatencyPoint } from '../types'
import { axisTicksForTimestamps, clampAxisTickX, latencySeriesColor, pruneCollidingAxisTicks } from './LatencyChart'
import { OverlaySurface } from './OverlaySurface'

// 丢包率独立曲线图：每个探测一条有色折线，与 LatencyChart 用同一套调色板与
// 目标顺序，颜色一一对应。悬停显示十字线与各目标数值。
interface LossChartProps {
  points: LatencyPoint[]
  activeTargetIds?: string[]
  eyebrow?: string
}

const viewWidth = 960
const viewHeight = 300
const pad = { left: 52, right: 24, top: 20, bottom: 40 }

interface LossSample {
  ts: number
  loss: number
  label: string
}

interface LossSeries {
  targetId: string
  targetName: string
  color: string
  samples: LossSample[]
}

function finiteLoss(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : null
}

function formatTickTime(createdAt: number, spanHours: number): string {
  const date = new Date(createdAt)
  if (Number.isNaN(date.getTime())) return '--:--'
  const time = `${date.getHours()}:${date.getMinutes().toString().padStart(2, '0')}`
  if (spanHours > 36) return `${date.getMonth() + 1}/${date.getDate()} ${time}`
  return time
}

function formatSampleTime(createdAt: number): string {
  const date = new Date(createdAt)
  if (Number.isNaN(date.getTime())) return '--'
  return date.toLocaleString('zh-CN', { hour12: false })
}

function nearestSample(samples: LossSample[], ts: number): LossSample | null {
  if (samples.length === 0) return null
  let low = 0
  let high = samples.length - 1
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (samples[middle].ts < ts) low = middle + 1
    else high = middle
  }
  const right = samples[low]
  const left = samples[Math.max(0, low - 1)]
  return Math.abs(right.ts - ts) < Math.abs(ts - left.ts) ? right : left
}

export function LossChart({ points, activeTargetIds = [], eyebrow }: LossChartProps) {
  const [hoverTs, setHoverTs] = useState<number | null>(null)
  const hoverFrameRef = useRef(0)

  useEffect(() => () => {
    if (hoverFrameRef.current) cancelAnimationFrame(hoverFrameRef.current)
  }, [])

  const series = useMemo<LossSeries[]>(() => {
    const order: string[] = []
    const buckets = new Map<string, { name: string; samples: LossSample[] }>()
    for (const point of points) {
      const ts = Date.parse(point.ts)
      const loss = finiteLoss(point.lossPercent)
      if (!Number.isFinite(ts) || loss === null) continue
      let bucket = buckets.get(point.targetId)
      if (!bucket) {
        bucket = { name: point.targetName, samples: [] }
        buckets.set(point.targetId, bucket)
        order.push(point.targetId)
      }
      bucket.name = point.targetName
      bucket.samples.push({ ts, loss, label: formatSampleTime(ts) })
    }
    const visible = activeTargetIds.length > 0 ? order.filter((id) => activeTargetIds.includes(id)) : order
    return visible.map((targetId) => {
      const bucket = buckets.get(targetId)!
      const samples = [...bucket.samples].sort((left, right) => left.ts - right.ts)
      return {
        targetId,
        targetName: bucket.name,
        color: latencySeriesColor(order.indexOf(targetId)),
        samples,
      }
    })
  }, [points, JSON.stringify(activeTargetIds)])

  const allTs = useMemo(() => {
    const set = new Set<number>()
    for (const item of series) for (const sample of item.samples) set.add(sample.ts)
    return [...set].sort((left, right) => left - right)
  }, [series])

  const timeStart = allTs[0] ?? 0
  const timeEnd = allTs.at(-1) ?? timeStart
  const timeSpan = Math.max(0, timeEnd - timeStart)
  const spanHours = timeSpan / 3_600_000
  const plotWidth = viewWidth - pad.left - pad.right
  const plotHeight = viewHeight - pad.top - pad.bottom

  const x = (ts: number) => (timeSpan <= 0 ? pad.left : pad.left + ((ts - timeStart) / timeSpan) * plotWidth)
  const y = (loss: number) => pad.top + (1 - loss / 100) * plotHeight

  const ticks = useMemo(() => {
    const candidates = axisTicksForTimestamps(allTs, 8)
    return pruneCollidingAxisTicks(
      candidates,
      (tick) => clampAxisTickX(x(tick), viewWidth, pad),
      (tick) => formatTickTime(tick, spanHours),
      7,
      (tick) => {
        const xx = x(tick)
        if (xx <= pad.left + 8) return 'start'
        if (xx >= viewWidth - pad.right - 8) return 'end'
        return 'middle'
      },
    )
  }, [allTs.join(','), timeStart, timeEnd])

  const hoverRows = useMemo(() => {
    if (hoverTs === null) return null
    const rows = series
      .map((item) => ({ item, sample: nearestSample(item.samples, hoverTs) }))
      .filter((row): row is { item: LossSeries; sample: LossSample } => row.sample !== null)
    if (rows.length === 0) return null
    return { label: formatSampleTime(hoverTs), rows }
  }, [hoverTs, series])

  const handleHoverMove = (event: ReactMouseEvent<SVGRectElement>) => {
    const svg = event.currentTarget.ownerSVGElement
    const ctm = svg?.getScreenCTM()
    if (!svg || !ctm || allTs.length === 0) return
    const point = svg.createSVGPoint()
    point.x = event.clientX
    point.y = event.clientY
    const svgPoint = point.matrixTransform(ctm.inverse())
    const ratio = plotWidth > 0 ? Math.max(0, Math.min(1, (svgPoint.x - pad.left) / plotWidth)) : 0
    const targetTs = timeStart + ratio * timeSpan
    let low = 0
    let high = allTs.length - 1
    while (low < high) {
      const middle = Math.floor((low + high) / 2)
      if (allTs[middle] < targetTs) low = middle + 1
      else high = middle
    }
    const right = allTs[low]
    const left = allTs[Math.max(0, low - 1)]
    const next = Math.abs(right - targetTs) < Math.abs(targetTs - left) ? right : left
    // 拖动时 mousemove 高频触发，用 rAF 节流到每帧最多一次 setState，
    // 否则每次都要 reconcile 全图圆点，延迟图那边是按 path 重绘所以没这个问题。
    if (hoverFrameRef.current) return
    hoverFrameRef.current = requestAnimationFrame(() => {
      hoverFrameRef.current = 0
      setHoverTs((current) => (current === next ? current : next))
    })
  }

  const linePath = (samples: LossSample[]): string => {
    const parts: string[] = []
    // 采样按时间排序后是连续的，直接连线即可。
    for (const sample of samples) {
      parts.push(`${parts.length > 0 ? 'L' : 'M'} ${x(sample.ts).toFixed(2)} ${y(sample.loss).toFixed(2)}`)
    }
    return parts.join(' ')
  }

  if (series.length === 0) return null

  return (
    <section className="lumina-loss-chart" aria-label="丢包率曲线">
      <div className="lumina-loss-chart-head">
        <h4>丢包率</h4>
        {eyebrow ? <span>{eyebrow}</span> : null}
      </div>
      <div className="lumina-loss-chart-wrap">
        <svg className="lumina-loss-chart-svg" viewBox={`0 0 ${viewWidth} ${viewHeight}`} role="img" aria-label="packet loss chart" onMouseLeave={() => setHoverTs(null)}>
          {[0, 25, 50, 75, 100].map((value) => {
            const yy = y(value)
            return (
              <g key={value}>
                <line x1={pad.left} x2={viewWidth - pad.right} y1={yy} y2={yy} className="lumina-loss-grid" />
                <text x={12} y={yy + 4} className="lumina-loss-axis">{value}%</text>
              </g>
            )
          })}
          {ticks.map((tick) => {
            const xx = x(tick)
            const anchor = xx <= pad.left + 8 ? 'start' : xx >= viewWidth - pad.right - 8 ? 'end' : 'middle'
            return (
              <text
                key={tick}
                x={Math.min(viewWidth - pad.right, Math.max(pad.left, xx))}
                y={viewHeight - 12}
                className="lumina-loss-axis"
                textAnchor={anchor}
              >
                {formatTickTime(tick, spanHours)}
              </text>
            )
          })}
          {series.map((item) => (
            <path key={item.targetId} d={linePath(item.samples)} fill="none" stroke={item.color} strokeWidth={1.8} strokeLinejoin="round" strokeLinecap="round" />
          ))}
          {hoverTs !== null && (
            <line x1={x(hoverTs)} x2={x(hoverTs)} y1={pad.top} y2={viewHeight - pad.bottom} className="lumina-loss-guide" />
          )}
          <rect
            x={pad.left}
            y={pad.top}
            width={plotWidth}
            height={plotHeight}
            fill="transparent"
            onMouseMove={handleHoverMove}
            onMouseEnter={handleHoverMove}
          />
        </svg>
        {hoverRows && (
          <OverlaySurface className="lumina-loss-tooltip" style={{ left: `${Math.min(72, Math.max(2, ((x(hoverTs!) - pad.left) / plotWidth) * 100))}%` }}>
            <time>{hoverRows.label}</time>
            {hoverRows.rows.map(({ item, sample }) => (
              <span key={item.targetId} className="lumina-loss-tooltip-row">
                <i style={{ background: item.color }} aria-hidden="true" />
                <b>{item.targetName}</b>
                <strong>{sample.loss.toFixed(1)}%</strong>
              </span>
            ))}
          </OverlaySurface>
        )}
      </div>
    </section>
  )
}
