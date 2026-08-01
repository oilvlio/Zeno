import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { HomeCardNode, HourlyLatencyPoint } from '../types'
import { ServerCard } from './ServerCard'

const hourlyHistory: HourlyLatencyPoint[] = [0, 50, 99.99, 100, 150, 199.99, 200, 250, -1, null, 75, 175].map((latencyMs, index) => ({
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
  uptimeSeconds: 262800,
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
  it('renders non-online nodes as frozen cards with a diagonal offline watermark', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, status: 'warning' }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('class="kulin-node-card is-offline"')
    expect(html).toContain('node-offline-watermark')
    expect(html).toContain('>离线</span>')
    expect(html).toContain('node-head')
    expect(html).toContain('node-usage-grid')
    expect(html).toContain('node-specs')
    expect(html).toContain('<p>Example Node A</p>')
    expect(html).toContain('class="node-uptime">在线 3 天</span>')
    expect(html).not.toContain('node-status-dot')
  })

  it('renders offline nodes with the watermark as well', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, status: 'offline' }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('class="kulin-node-card is-offline"')
    expect(html).toContain('node-offline-watermark')
  })

  it('keeps online cards clean without offline overlay markup', () => {
    const html = renderToStaticMarkup(<ServerCard node={{ ...baseNode, uptimeSeconds: null }} onOpen={vi.fn()} />)

    expect(html).toContain('class="kulin-node-card"')
    expect(html).toContain('class="node-uptime">在线 -- 天</span>')
    expect(html).not.toContain('is-offline')
    expect(html).not.toContain('node-offline-watermark')
    expect(html).not.toContain('node-status-dot')
    expect(html).not.toContain('>离线</span>')
  })

  it('renders GitHub-style specs below the name and four full-width bars without details underneath', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, monthlyPeriodStart: '2026-07-01', monthlyPeriodEnd: '2026-07-31', monthlyResetDay: 1 }} onOpen={vi.fn()} />,
    )

    expect(html).toMatch(/class="node-head"[\s\S]*class="node-specs"[\s\S]*class="node-usage"/)
    expect(html).toMatch(/class="node-spec spec-cpu"[\s\S]*>2 Cores<\/span>/)
    expect(html).toMatch(/class="node-spec spec-memory"[\s\S]*>4.00 KB<\/span>/)
    expect(html).toMatch(/class="node-spec spec-disk"[\s\S]*>8.00 KB<\/span>/)
    expect(html.match(/class="usage-row usage-row--/g)).toHaveLength(4)
    expect(html).toContain('usage-row--cpu')
    expect(html).toContain('usage-row--memory')
    expect(html).toContain('usage-row--disk')
    expect(html).toContain('usage-row--traffic')
    expect(html).not.toContain('usage-row__icon')
    expect(html).toMatch(/>CPU<\/span>[\s\S]*<strong>12.50%<\/strong>[\s\S]*>内存<\/span>[\s\S]*<strong>25.00%<\/strong>[\s\S]*>存储<\/span>[\s\S]*<strong>25.00%<\/strong>[\s\S]*>流量<\/span>[\s\S]*<strong>1.00KB \/ 4.00KB<\/strong>/)
    expect(html).not.toContain('usage-row__detail')
    expect(html).not.toContain('>负载<')
    expect(html).not.toContain('>占用<')
  })

  it('uses original-size individual frames for upload, download, expiry and billing', () => {
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

  it('renders latency and packet loss in separate original-size frames with twelve hourly cells each', () => {
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
    expect(html).toMatch(/history-latency is-good[^>]*title="2026-07-29 00:00 · 延迟 0.00 ms"/)
    expect(html).toMatch(/history-latency is-good[^>]*title="2026-07-29 02:00 · 延迟 99.99 ms"/)
    expect(html).toMatch(/history-latency is-warning[^>]*title="2026-07-29 03:00 · 延迟 100.00 ms"/)
    expect(html).toMatch(/history-latency is-warning[^>]*title="2026-07-29 05:00 · 延迟 199.99 ms"/)
    expect(html).toMatch(/history-latency is-danger[^>]*title="2026-07-29 06:00 · 延迟 200.00 ms"/)
    expect(html).toMatch(/history-latency is-empty[^>]*title="2026-07-29 08:00 · 延迟 -1.00 ms"/)
    expect(html).toMatch(/history-loss is-good[^>]*title="2026-07-29 03:00 · 丢包 0.90%"/)
    expect(html).toMatch(/history-loss is-warning[^>]*title="2026-07-29 04:00 · 丢包 1.00%"/)
    expect(html).toMatch(/history-loss is-danger[^>]*title="2026-07-29 07:00 · 丢包 5.00%"/)
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

  it('shows a due-today remaining value as zero days', () => {
    const html = renderToStaticMarkup(<ServerCard node={{ ...baseNode, expiryLabel: '今天到期' }} onOpen={vi.fn()} />)

    expect(html).toContain('metric-expiry is-urgent')
    expect(html).toContain('>0 天</strong>')
    expect(html).not.toContain('>今天到期</strong>')
  })

  it('updates the renewal amount when a non-default display currency is selected', () => {
    const html = renderToStaticMarkup(
      <ServerCard node={{ ...baseNode, renewalAmount: 20, renewalCurrency: 'USD', billingCycle: '年' }} displayCurrency="EUR" exchangeRates={{ CNY: 1, USD: 8, EUR: 10 }} onOpen={vi.fn()} />,
    )

    expect(html).toContain('>€ 16 / 年</strong>')
  })
})
