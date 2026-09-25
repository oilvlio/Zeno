import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { LatencyPoint } from '../types'
import { LossChart } from './LossChart'

const points: LatencyPoint[] = [
  { ts: '2026-07-02T12:00:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 12, avgMs: 12, lossPercent: 0 },
  { ts: '2026-07-02T12:01:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 42, avgMs: 42, lossPercent: 10 },
  { ts: '2026-07-02T12:00:00Z', targetId: 'beta', targetName: 'Beta', medianMs: 20, avgMs: 20, lossPercent: 5 },
  { ts: '2026-07-02T12:01:00Z', targetId: 'beta', targetName: 'Beta', medianMs: 22, avgMs: 22, lossPercent: 0 },
]

describe('LossChart', () => {
  it('draws one colored loss line per target with percent axis and legend', () => {
    const html = renderToStaticMarkup(<LossChart points={points} eyebrow="1 天 · 2 个监控服务" />)

    expect(html).toContain('aria-label="丢包率曲线"')
    expect(html).toContain('<h4>丢包率</h4>')
    expect(html).toContain('1 天 · 2 个监控服务')
    expect(html).toContain('>100%</text>')
    expect(html).toContain('>25%</text>')
    // Alpha 用第一顺位颜色，与延迟图一致；图例已去掉，颜色对应关系由上方探测卡承担。
    expect(html).toContain('stroke="#22c55e"')
    // 纯线条：不画任何圆点，数值靠悬停 tooltip 读取。
    expect(html).not.toContain('<circle')
    // 初始无悬停：没有十字线与 tooltip。
    expect(html).not.toContain('lumina-loss-guide')
    expect(html).not.toContain('lumina-loss-tooltip')
    expect(html).not.toContain('lumina-loss-legend')
  })

  it('draws severe loss runs as plain elevated lines without markers', () => {
    const severe: LatencyPoint[] = [
      { ts: '2026-07-02T12:00:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 12, avgMs: 12, lossPercent: 0 },
      { ts: '2026-07-02T12:01:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 42, avgMs: 42, lossPercent: 100 },
      { ts: '2026-07-02T12:02:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 43, avgMs: 43, lossPercent: 100 },
      { ts: '2026-07-02T12:03:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 44, avgMs: 44, lossPercent: 0 },
      { ts: '2026-07-02T12:04:00Z', targetId: 'alpha', targetName: 'Alpha', medianMs: 45, avgMs: 45, lossPercent: 100 },
    ]
    const html = renderToStaticMarkup(<LossChart points={severe} />)
    expect(html).not.toContain('<circle')
    expect(html).toContain('<path')
  })

  it('filters to the selected targets when activeTargetIds is set', () => {
    const html = renderToStaticMarkup(<LossChart points={points} activeTargetIds={['beta']} />)

    expect(html).toContain('stroke="#38bdf8"')
    expect(html).not.toContain('stroke="#22c55e"')
  })

  it('renders nothing without drawable samples', () => {
    expect(renderToStaticMarkup(<LossChart points={[]} />)).toBe('')
  })
})
