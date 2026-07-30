import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { HomeCardNode, HourlyLatencyPoint } from '../types'
import { ServerCard } from './ServerCard'

const hourlyHistory: HourlyLatencyPoint[] = [20, 40, 60, 79, 80, 100, 149, 150, 180, 220, null, 50].map((latencyMs, index) => ({
  startedAt: new Date(Date.UTC(2026, 6, 29, index)).toISOString(),
  latencyMs,
  lossPercent: [0, 0.1, 0.5, 0.9, 1, 2, 4.9, 5, 10, 25, null, 0][index],
}))

const baseNode: HomeCardNode = {
  id: 'example-node-a',
  displayName: 'Example Node A',
  status: 'online',
  os: 'debian',
  osVersion: '13',
  kernel: '6.12.0',
  arch: 'x86_64',
  cpuModel: 'AMD EPYC',
  countryCode: 'HK',
  cpuCores: 2,
  expiryLabel: '永 久',
  cpuPercent: 12.5,
  load1: 0.42,
  load5: 0.35,
  load15: 0.28,
  memoryUsedBytes: 1024,
  memoryTotalBytes: 4096,
  diskUsedBytes: 2048,
  diskTotalBytes: 8192,
  netInSpeedBps: 128,
  netOutSpeedBps: 256,
  netInTotalBytes: 1024,
  netOutTotalBytes: 2048,
  monthlyBillableBytes: 1024,
  monthlyQuotaBytes: 4096,
  latencySummary: {
    targetId: 'google',
    targetName: 'Google',
    medianMs: 70,
    avgMs: 72,
    lossPercent: 1.25,
    updatedAt: '2026-07-29T12:00:00Z',
    hourlyHistory,
  },
}

describe('ServerCard', () => {
  it('renders non-online nodes as frozen metric cards with a concise offline status', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, status: 'warning' }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('kulin-node-card is-offline')
    expect(html).toContain('node-head')
    expect(html).toContain('node-usage-grid')
    expect(html).not.toContain('node-specs')
    expect(html).toContain('<p>Example Node A</p>')
    expect(html).not.toContain('node-offline-watermark')
    expect(html).toContain('node-status is-offline')
    expect(html).toContain('node-status-dot')
    expect(html).toContain('>离线</span>')
  })

  it('shows a green concise online status in the card header', () => {
    const html = renderToStaticMarkup(<ServerCard node={baseNode} onOpen={vi.fn()} />)

    expect(html).toContain('node-status is-online')
    expect(html).toContain('node-status-dot')
    expect(html).toContain('>在线</span>')
  })

  it('lays out four resource bars as CPU-memory then disk-traffic with details below each bar', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, monthlyPeriodStart: '2026-07-01', monthlyPeriodEnd: '2026-07-31', monthlyResetDay: 1 }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('node-usage-grid')
    expect(html).not.toContain('node-specs')
    expect(html.match(/class="usage-row"/g)).toHaveLength(4)
    expect(html).toContain('usage-row__icon')
    expect(html).toMatch(/>CPU<\/span>[\s\S]*<strong>12.50%<\/strong>[\s\S]*>内存<\/span>[\s\S]*<strong>25.00%<\/strong>[\s\S]*>存储<\/span>[\s\S]*<strong>25.00%<\/strong>[\s\S]*>流量<\/span>[\s\S]*<strong>25.00%<\/strong>/)
    expect(html).toContain('>负载</span>')
    expect(html).toContain('>0.42 / 0.35 / 0.28</span>')
    expect(html.match(/>占用<\/span>/g)).toHaveLength(3)
    expect(html).toContain('1.00 KB / 4.00 KB')
    expect(html).toContain('2.00 KB / 8.00 KB')
  })

  it('uses a compact two-column grid for upload, download, expiry and billing', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, renewalAmount: 20, renewalCurrency: 'USD', billingCycle: '年' }} displayCurrency="CNY" exchangeRates={{ CNY: 1, USD: 8 }} onOpen={vi.fn()} />,
    )

    expect(html).toMatch(/class="node-footer-grid"[\s\S]*metric-up[\s\S]*metric-down[\s\S]*metric-expiry is-safe[\s\S]*metric-billing/)
    expect(html.match(/class="node-metric /g)).toHaveLength(4)
    expect(html).not.toContain('metric-latency')
    expect(html).not.toContain('metric-loss')
    expect(html).toContain('>¥ 160 / 年</strong>')
    expect(html).toMatch(/metric-up[\s\S]*>上传<\/span>[\s\S]*>256 B\/s<\/strong>/)
    expect(html).toMatch(/metric-down[\s\S]*>下载<\/span>[\s\S]*>128 B\/s<\/strong>/)
  })

  it('renders latency and packet loss as separate bottom rows with twelve hourly cells each', () => {
    const html = renderToStaticMarkup(<ServerCard node={baseNode} onOpen={vi.fn()} />)

    expect(html).toMatch(/class="node-health-history"[\s\S]*health-latency[\s\S]*health-loss/)
    expect(html.match(/class="history-cell history-latency /g)).toHaveLength(12)
    expect(html.match(/class="history-cell history-loss /g)).toHaveLength(12)
    expect(html).toContain('history-latency is-good')
    expect(html).toContain('history-latency is-warning')
    expect(html).toContain('history-latency is-danger')
    expect(html).toContain('history-latency is-empty')
    expect(html).toContain('history-loss is-good')
    expect(html).toContain('history-loss is-warning')
    expect(html).toContain('history-loss is-danger')
    expect(html).toContain('history-loss is-empty')
    expect(html).toContain('>72.0 ms</strong>')
    expect(html).toContain('>1.25%</strong>')
    expect(html).toContain('2026-07-29 00:00 · 延迟 20.00 ms')
    expect(html).toContain('2026-07-29 00:00 · 丢包 0.00%')
  })

  it('keeps twelve neutral hourly cells when history is unavailable', () => {
    const html = renderToStaticMarkup(<ServerCard node={{ ...baseNode, latencySummary: undefined }} onOpen={vi.fn()} />)

    expect(html.match(/class="history-cell history-latency is-empty"/g)).toHaveLength(12)
    expect(html.match(/class="history-cell history-loss is-empty"/g)).toHaveLength(12)
    expect(html).toContain('>--ms</strong>')
    expect(html).toContain('>--%</strong>')
  })

  it('keeps expiry and billing slots when values are unavailable', () => {
    const html = renderToStaticMarkup(<ServerCard node={{ ...baseNode, expiryLabel: '', renewalAmount: null }} onOpen={vi.fn()} />)

    expect(html.match(/class="node-metric /g)).toHaveLength(4)
    expect(html).toMatch(/metric-expiry[\s\S]*>剩余<\/span>[\s\S]*>--<\/strong>/)
    expect(html).toMatch(/metric-billing[\s\S]*>账单<\/span>[\s\S]*>--<\/strong>/)
  })

  it('styles precomputed recurring expiry labels by urgency', () => {
    const html = renderToStaticMarkup(<ServerCard node={{ ...baseNode, expiryLabel: '余 3 天' }} onOpen={vi.fn()} />)

    expect(html).toContain('metric-expiry is-urgent')
    expect(html).toContain('>3 天</strong>')
  })

  it('updates the renewal amount when a non-default display currency is selected', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, renewalAmount: 20, renewalCurrency: 'USD', billingCycle: '年' }} displayCurrency="EUR" exchangeRates={{ CNY: 1, USD: 8, EUR: 10 }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('>€ 16 / 年</strong>')
  })
})
