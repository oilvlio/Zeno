import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { HomeCardNode, LatencyPoint, StatePoint } from '../types'
import { LatencyDetail } from './LatencyDetail'

const node = {
  id: 'example-node-a',
  displayName: 'Example Node A',
  status: 'online',
  os: 'debian',
  arch: 'aarch64',
  osVersion: '13',
  kernel: '6.12.0',
  virtualization: 'kvm',
  cpuModel: 'AMD EPYC 7B13',
  countryCode: 'HK',
  bootTime: '2026-07-02T01:00:00Z',
  cpuCores: 2,
  cpuPercent: 12,
  memoryUsedBytes: 1024,
  memoryTotalBytes: 4096,
  diskUsedBytes: 2048,
  diskTotalBytes: 8192,
  load1: 0.11,
  load5: 0.12,
  load15: 0.13,
  uptimeSeconds: 120,
  netInSpeedBps: 128,
  netOutSpeedBps: 256,
  netInTotalBytes: 1024,
  netOutTotalBytes: 2048,
  monthlyBillableBytes: 3072,
  monthlyQuotaBytes: null,
} as HomeCardNode

const statePoints: StatePoint[] = [
  {
    ts: '2026-07-02T12:01:00Z',
    cpuPercent: 18.75,
    load1: 0.42,
    load5: 0.35,
    load15: 0.28,
    memoryUsedBytes: 2048,
    memoryTotalBytes: 4096,
    swapUsedBytes: 512,
    swapTotalBytes: 2048,
    diskUsedBytes: 4096,
    diskTotalBytes: 8192,
    netInTotalBytes: 12 * 1024,
    netOutTotalBytes: 24 * 1024,
    netInSpeedBps: 64 * 1024,
    netOutSpeedBps: 512 * 1024,
    processCount: 88,
    tcpConnectionCount: 34,
    udpConnectionCount: 12,
    uptimeSeconds: 3660,
  },
]

const latencyPoints: LatencyPoint[] = [
  { ts: '2026-07-02T12:01:00Z', targetId: 'telegram', targetName: 'Telegram', medianMs: 32.5, avgMs: 33.1, lossPercent: 0 },
]

describe('LatencyDetail', () => {
  it('includes the agent state history panel without removing the latency monitor', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        statePoints={statePoints}
        stateLoading={false}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('detail-hero')
    expect(html).toContain('detail-status-pill')
    expect(html).toContain('detail-fact-strip')
    expect(html).toContain('Example Node A')
    expect(html).toContain('AMD EPYC 7B13')
    expect(html).toContain('2 Virtual Cores')
    expect(html).not.toContain('2 Cores')
    expect(html).not.toContain('kvm')
    expect(html).not.toContain('Standard PC')
    expect(html).toContain('运行时间')
    expect(html).toContain('1 小时 1 分钟')
    expect(html).toContain('负载')
    expect(html).not.toContain('is-emphasis')
    expect(html.match(/is-adaptive/g)).toHaveLength(2)
    expect(html).toContain('0.42 / 0.35 / 0.28')
    expect(html).toContain('debian 13')
    expect(html).toContain('6.12.0')
    expect(html).not.toContain('debian 13 · aarch64 · 6.12.0 · HK')
    expect(html).not.toContain('12.0% · AMD EPYC 7B13')
    expect(html).toContain('2.00 KB / 4.00 KB')
    expect(html).toContain('4.00 KB / 8.00 KB')
    expect(html).toContain('18.8%')
    expect(html).not.toContain('CPU 使用')
    expect(html).not.toContain('规格')
    expect(html).toContain('内存')
    expect(html).toContain('磁盘')
    expect(html).toContain('开机时间')
    expect(html).toContain('2026/7/2 09:00:00')
    expect(html).not.toContain('↑256 B/s ↓128 B/s')
    expect(html).toContain('detail-fact--traffic')
    expect(html).toContain('detail-traffic-values')
    expect(html).toContain('detail-traffic-value')
    expect(html).toContain('累计流量: ↑ 2.00 KB  ↓ 1.00 KB')
    expect(html).not.toContain('detail-info-card')
    expect(html).toContain('系统资源趋势')
    expect(html).not.toContain('实时 · 1 个状态采样')
    expect(html).not.toContain('个状态采样')
    expect(html).not.toContain('Example Node A 网络延迟')
    expect(html).toContain('<h3>网络延迟</h3>')
    expect(html).not.toContain('1 天 · 0 个监控服务')
    expect(html).toContain('monitor services')
    expect(html).toContain('lumina-probe-grid')
    expect(html).toContain('lumina-probe-name')
    expect(html).toContain('lumina-probe-color')
    expect(html).not.toContain('data-desktop-corners')
    expect(html).not.toContain('latency-legend')
    expect(html).not.toContain('latency-target-toolbar')
  })

  it('renders one airy Lumina probe card per target without edge bookkeeping', () => {
    const count = 8
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={Array.from({ length: count }, (_, index) => ({
          ts: `2026-07-02T12:${String(index + 1).padStart(2, '0')}:00Z`,
          targetId: `target-${index}`,
          targetName: `Target ${index}`,
          medianMs: 20 + index,
          avgMs: 21 + index,
          lossPercent: index,
        }))}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )
    expect(html.match(/class="lumina-probe-card"/g)).toHaveLength(count)
    expect(html).toContain('Target 0')
    expect(html).toContain('>21.0 ms</span>')
    expect(html).toContain('均值 28.0 ms')
    expect(html).toContain('中位 27.0 ms')
    expect(html).toContain('丢包 7.00%')
    expect(html).toContain('样本 1')
    expect(html).toContain('min 21.0 ms')
    expect(html).toContain('max 28.0 ms')
  })

  it('labels detail CPU cores as physical when the host does not look virtualized', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={{ ...node, virtualization: 'PowerEdge R740' }}
        points={latencyPoints}
        statePoints={statePoints}
        stateLoading={false}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('2 Physical Cores')
  })

  it('keeps a neutral core label when virtualization is unknown', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={{ ...node, virtualization: '' }}
        points={latencyPoints}
        statePoints={statePoints}
        stateLoading={false}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('2 Cores')
  })

  it('keeps detail facts and latency area reserved while live data is loading', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={[]}
        loading
        statePoints={[]}
        stateLoading
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('运行时间')
    expect(html).toContain('负载')
    expect(html).toContain('2 分钟')
    expect(html).toContain('0.11 / 0.12 / 0.13')
    expect(html).toContain('累计流量')
    expect(html).toContain('1.00 KB / 4.00 KB')
    expect(html).toContain('2.00 KB / 8.00 KB')
    expect(html).toContain('latency-target-grid is-loading')
    expect(html).toContain('latency-panel-skeleton')
    expect(html).toContain('class="sr-only" role="status" data-state="loading" aria-live="polite">正在读取网络延迟…</div>')
  })

  it('keeps warning detail status distinct from offline', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={{ ...node, status: 'warning' }}
        points={[]}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />
    )

    expect(html).toContain('detail-status-pill status-warning')
    expect(html).toContain('异常')
    expect(html).not.toContain('detail-status-pill status-offline')
    expect(html).not.toContain('>离线</span>')
  })

  it('renders the detail page in the wide Lumina layout', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('kulin-container detail-container detail-lumina')
  })

  it('shows resource and ping views behind a Lumina-style tab switch defaulting to ping', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('role="tablist"')
    expect(html).toContain('data-active="false">系统资源</button>')
    expect(html).toContain('data-active="true">Ping 历史</button>')
  })

  it('opens the resource view first when requested', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        range="1d"
        initialView="resource"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('data-active="true">系统资源</button>')
    expect(html).toContain('data-active="false">Ping 历史</button>')
  })

  it('shows latency and loss metric tabs with latency rendered first', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('aria-label="Ping 图表指标"')
    expect(html).toContain('data-active="true">延迟</button>')
    expect(html).toContain('data-active="false">丢包率</button>')
    expect(html).toContain('aria-label="latency chart"')
    // 一次只渲染一张图：丢包率视图默认不在 DOM 里，切换才挂载。
    expect(html).not.toContain('aria-label="丢包率曲线"')
    expect(html).not.toContain('packet-loss-area')
  })

  it('renders Lumina-style probe cards with average, median, loss and range', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={latencyPoints}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('>33.1 ms</span>')
    expect(html).toContain('均值 33.1 ms')
    expect(html).toContain('中位 32.5 ms')
    expect(html).toContain('丢包 0.00%')
    expect(html).toContain('样本 1')
    expect(html).toContain('min 33.1 ms')
    expect(html).toContain('max 33.1 ms')
    // Lumina 热力色：延迟五阶绿，丢包连续渐变。
    expect(html).toContain('style="color:#2fc66e"')
    expect(html).toContain('style="color:hsl(145 62% 48%)"')
  })

  it('includes range controls with the latency chart actions', () => {
    const html = renderToStaticMarkup(
      <LatencyDetail
        node={node}
        points={[]}
        range="1d"
        onBack={vi.fn()}
        onRangeChange={vi.fn()}
      />,
    )

    expect(html).toContain('monitor-heading-actions')
    expect(html).toContain('monitor-title-row')
    const latencyRangeMarkup = html.match(/aria-label="latency range selector"[\s\S]*?<\/div>/)?.[0] ?? ''
    expect(latencyRangeMarkup).toContain('实时')
    expect(latencyRangeMarkup).toContain('1 天')
    expect(latencyRangeMarkup).not.toContain('7 天')
    expect(latencyRangeMarkup).not.toContain('30 天')
    expect(html).toContain('平')
    expect(html).not.toContain('削峰')
  })
})
