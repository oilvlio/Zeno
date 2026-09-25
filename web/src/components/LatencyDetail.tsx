import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type { HomeCardNode, LatencyPoint, StatePoint } from '../types'
import { formatLatency } from '../lib/format'
import { summarizeLatencyTargets } from '../lib/latencyTargets'
import { decimateLatencyPoints, LOSS_CHART_RENDER_BUDGET } from '../lib/chartDecimate'
import { latencyHeatColor, lossHeatColor } from '../lib/metricHeat'
import { latencySeriesColor, LatencyChart } from './LatencyChart'
import { LossChart } from './LossChart'
import { ServerFlag } from './ServerFlag'
import { StateHistoryPanel } from './StateHistoryPanel'
import { availableHistoryRanges } from '../lib/historyRange'
import { HistoryRangeSelector } from './HistoryRangeSelector'

interface LatencyDetailProps {
  node: HomeCardNode
  points: LatencyPoint[]
  statePoints?: StatePoint[]
  range: string
  stateRange?: string
  loading?: boolean
  error?: string
  stateLoading?: boolean
  stateError?: string
  canUseExtendedRanges?: boolean
  initialView?: 'ping' | 'resource'
  onBack: () => void
  onRangeChange: (range: string) => void
  onStateRangeChange?: (range: string) => void
  topHeader?: ReactNode
}

function targetCardTitle(target: { targetName: string; avgMs: number | null; medianMs: number | null; minMs: number | null; maxMs: number | null; lossPercent: number | null | undefined; sampleCount: number }): string {
  const parts = [
    target.targetName,
    `平均 ${formatLatency(target.avgMs)}`,
    target.medianMs !== null ? `中位 ${formatLatency(target.medianMs)}` : null,
    target.minMs !== null && target.maxMs !== null ? `区间 ${formatLatency(target.minMs)} ~ ${formatLatency(target.maxMs)}` : null,
    `丢包 ${formatLossPercent(target.lossPercent)}`,
    `${target.sampleCount} 个样本`,
  ]
  return parts.filter((part) => part !== null).join(' · ')
}

export function LatencyDetail({
  node,
  points,
  statePoints = [],
  range,
  stateRange = '1h',
  loading = false,
  error,
  stateLoading = false,
  stateError,
  canUseExtendedRanges = false,
  initialView = 'ping',
  onBack,
  onRangeChange,
  onStateRangeChange = () => {},
  topHeader,
}: LatencyDetailProps) {
  const targetSummaries = useMemo(() => summarizeLatencyTargets(points), [points])
  // 图表压点只喂图：统计汇总继续用原始全量点。丢包单独更小的预算，
  // 1 天 1440 点压成 360（约 4 分钟一步），线更顺。
  const chartPoints = useMemo(() => decimateLatencyPoints(points), [points])
  const lossChartPoints = useMemo(() => decimateLatencyPoints(points, LOSS_CHART_RENDER_BUDGET), [points])
  const [activeTargetIds, setActiveTargetIds] = useState<string[]>([])
  const [peakCut, setPeakCut] = useState(false)
  const [view, setView] = useState<'ping' | 'resource'>(initialView)
  const [chartMetric, setChartMetric] = useState<'latency' | 'loss'>('latency')
  const rangeOptions = availableHistoryRanges(canUseExtendedRanges)
  const rangeLabel = rangeOptions.find((option) => option.value === range)?.label ?? range
  const latestState = latestStatePoint(statePoints)
  const visualStatus = node.status === 'warning' ? 'warning' : node.status === 'online' ? 'online' : 'offline'
  const summaryUptimeSeconds = node.uptimeSeconds ?? null
  const uptimeValue = latestState?.uptimeSeconds !== null && latestState?.uptimeSeconds !== undefined
    ? formatUptime(latestState.uptimeSeconds)
    : summaryUptimeSeconds !== null
      ? formatUptime(summaryUptimeSeconds)
      : formatUptimeFromBootTime(node.bootTime)
  const loadValue = latestState && latestState.load1 !== null && latestState.load5 !== null && latestState.load15 !== null
    ? `${formatFixed(latestState.load1, 2)} / ${formatFixed(latestState.load5, 2)} / ${formatFixed(latestState.load15, 2)}`
    : node.load1 !== null && node.load1 !== undefined && node.load5 !== null && node.load5 !== undefined && node.load15 !== null && node.load15 !== undefined
      ? `${formatFixed(node.load1, 2)} / ${formatFixed(node.load5, 2)} / ${formatFixed(node.load15, 2)}`
    : '-- / -- / --'
  const memoryUsageValue = formatBinaryUsage(
    latestState?.memoryUsedBytes ?? node.memoryUsedBytes,
    latestState?.memoryTotalBytes ?? node.memoryTotalBytes,
  )
  const diskUsageValue = formatBinaryUsage(
    latestState?.diskUsedBytes ?? node.diskUsedBytes,
    latestState?.diskTotalBytes ?? node.diskTotalBytes,
  )
  const hasLatencyData = points.length > 0 || targetSummaries.length > 0
  const showLatencySkeleton = loading && !hasLatencyData && !error
  const toggleTarget = (targetId: string) => {
    setActiveTargetIds((current) => (
      current.includes(targetId) ? current.filter((id) => id !== targetId) : [...current, targetId]
    ))
  }

  return (
    <div className="kulin-container detail-container detail-lumina">
      <section className="home-top-card detail-top-card" aria-label={`${node.displayName} server overview`}>
        {topHeader}
        <section className="detail-hero">
          <div className="detail-hero__main">
            <button className="detail-title-button" type="button" onClick={onBack}>
              <BackIcon />
              <ServerFlag countryCode={node.countryCode} className="detail-title-flag" />
              <span>{node.displayName}</span>
            </button>
            <div className="detail-hero__badges" aria-label="server live status">
              <span className={`detail-status-pill status-${visualStatus}`}>{formatStatusLabel(node.status)}</span>
            </div>
          </div>
          <section className="detail-fact-strip detail-fact-strip--server" aria-label={`${node.displayName} server facts`}>
            <InfoFact label="系统" value={formatSystemSpec(node)} wide />
            <InfoFact label="CPU" value={formatCpuSpec(node)} wide />
            <InfoFact label="内存" value={memoryUsageValue} />
            <InfoFact label="磁盘" value={diskUsageValue} />
            <InfoFact label="开机时间" value={formatBootTime(node.bootTime)} />
            <InfoFact label="运行时间" value={uptimeValue ?? '--'} adaptive pending={!uptimeValue} />
            <InfoFact label="负载" value={loadValue} adaptive pending={loadValue === '-- / -- / --'} />
            <TrafficFact sent={formatBinaryBytes(node.netOutTotalBytes)} received={formatBinaryBytes(node.netInTotalBytes)} />
          </section>
        </section>
      </section>

      <div className="lumina-view-tabs" role="tablist" aria-label="详情视图">
        <button
          type="button"
          role="tab"
          aria-selected={view === 'resource'}
          data-active={view === 'resource'}
          onClick={() => setView('resource')}
        >
          系统资源
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={view === 'ping'}
          data-active={view === 'ping'}
          onClick={() => setView('ping')}
        >
          Ping 历史
        </button>
      </div>

      <div hidden={view !== 'ping'}>
      <section className="monitor-panel" aria-label={`${node.displayName} network latency`}>
        <header className="monitor-heading latency-monitor-heading">
          <div className="monitor-heading-title">
            <div className="monitor-title-row">
              <h3>网络延迟</h3>
              <button
                type="button"
                className={`peak-switch sliding-selector--compact${peakCut ? ' is-active' : ''}`}
                aria-pressed={peakCut}
                onClick={() => setPeakCut((current) => !current)}
              >
                <span aria-hidden="true" />
                <b>平滑</b>
              </button>
            </div>
            <p>{showLatencySkeleton ? '同步监控服务…' : `${targetSummaries.length} 个监控服务`}</p>
          </div>
          <div className="monitor-heading-actions">
            <HistoryRangeSelector
              ariaLabel="latency range selector"
              options={rangeOptions}
              value={range}
              onChange={onRangeChange}
              commitDelayMs={190}
            />
          </div>
        </header>

        {showLatencySkeleton && <LatencyLoadingSkeleton />}
        {error && <div className="detail-state is-error" data-state="error" role="alert">网络延迟读取失败：{error}</div>}

        {!showLatencySkeleton && !error && hasLatencyData && (
          <>
            <div
              className="lumina-probe-grid"
              aria-label="monitor services"
            >
              {targetSummaries.map((target, index) => (
                <button
                    key={target.targetId}
                    type="button"
                    className="lumina-probe-card"
                    title={targetCardTitle(target)}
                    data-active={activeTargetIds.includes(target.targetId)}
                    onClick={() => toggleTarget(target.targetId)}
                  >
                    <div className="lumina-probe-head">
                      <span className="lumina-probe-name">
                        <i className="lumina-probe-color" style={{ backgroundColor: latencySeriesColor(index) }} aria-hidden="true" />
                        <span>{target.targetName}</span>
                      </span>
                      <span className="lumina-probe-primary" style={{ color: latencyHeatColor(target.avgMs) }}>{formatLatency(target.avgMs)}</span>
                    </div>
                    <div className="lumina-probe-stats">
                      <span>均值 {formatLatency(target.avgMs)}</span>
                      <span>中位 {target.medianMs === null ? '--' : formatLatency(target.medianMs)}</span>
                      <span style={{ color: lossHeatColor(target.lossPercent) }}>丢包 {formatLossPercent(target.lossPercent)}</span>
                      <span>样本 {target.sampleCount}</span>
                    </div>
                    <div className="lumina-probe-meta">
                      <span>min {target.minMs === null ? '--' : formatLatency(target.minMs)}</span>
                      <span>max {target.maxMs === null ? '--' : formatLatency(target.maxMs)}</span>
                    </div>
                </button>
              ))}
            </div>

            <div className="lumina-metric-tabs" role="tablist" aria-label="Ping 图表指标">
              <button
                type="button"
                role="tab"
                aria-selected={chartMetric === 'latency'}
                data-active={chartMetric === 'latency'}
                onClick={() => setChartMetric('latency')}
              >
                延迟
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={chartMetric === 'loss'}
                data-active={chartMetric === 'loss'}
                onClick={() => setChartMetric('loss')}
              >
                丢包率
              </button>
            </div>

            {chartMetric === 'latency' ? (
              <LatencyChart
                points={chartPoints}
                title={`${node.displayName} 网络延迟`}
                eyebrow={`${rangeLabel} · ${targetSummaries.length} 个监控服务${peakCut ? ' · 平滑' : ''}`}
                compactHeader
                hideHeader
                hideLegend
                peakCut={peakCut}
                activeTargetIds={activeTargetIds}
                hidePacketLossArea
              />
            ) : (
              <LossChart
                points={lossChartPoints}
                activeTargetIds={activeTargetIds}
                eyebrow={`${rangeLabel} · ${targetSummaries.length} 个监控服务`}
              />
            )}
          </>
        )}
        {!showLatencySkeleton && !error && !hasLatencyData && <div className="detail-state" data-state="empty" role="status" aria-live="polite">暂无网络延迟历史</div>}
      </section>
      </div>

      <div hidden={view !== 'resource'}>
      <StateHistoryPanel
        points={statePoints}
        range={stateRange}
        onRangeChange={onStateRangeChange}
        loading={stateLoading}
        error={stateError}
        canUseExtendedRanges={canUseExtendedRanges}
      />
      </div>
    </div>
  )
}

function LatencyLoadingSkeleton() {
  return (
    <>
      <div className="sr-only" role="status" data-state="loading" aria-live="polite">正在读取网络延迟…</div>
      <div className="latency-target-grid is-loading" aria-label="monitor services loading" aria-hidden="true">
        {Array.from({ length: 7 }).map((_, index) => (
          <button key={index} type="button" disabled>
            <span>同步中</span>
            <strong>-- ms</strong>
            <em>丢包 --</em>
          </button>
        ))}
      </div>
      <section className="latency-panel latency-panel-skeleton" aria-hidden="true">
        <div className="latency-chart-skeleton" />
      </section>
    </>
  )
}

function InfoFact({ label, value, wide = false, adaptive = false, pending = false }: { label: string; value: string; wide?: boolean; adaptive?: boolean; pending?: boolean }) {
  const valueRef = useRef<HTMLElement>(null)
  const [multiline, setMultiline] = useState(false)

  useEffect(() => {
    if (!adaptive) return undefined
    const element = valueRef.current
    if (!element) return undefined
    let active = true

    const measure = () => {
      if (!active) return
      const lineHeight = Number.parseFloat(window.getComputedStyle(element).lineHeight)
      const height = element.getBoundingClientRect().height
      setMultiline(Number.isFinite(lineHeight) && height > lineHeight * 1.5)
    }
    measure()

    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(element)
    if (element.parentElement) observer?.observe(element.parentElement)
    window.addEventListener('resize', measure)
    void document.fonts?.ready.then(measure)

    return () => {
      active = false
      observer?.disconnect()
      window.removeEventListener('resize', measure)
    }
  }, [adaptive, value])

  return (
    <article className={`detail-fact${wide ? ' is-wide' : ''}${adaptive ? ' is-adaptive' : ''}${multiline ? ' is-multiline' : ''}${pending ? ' is-pending' : ''}`} title={`${label}: ${value}`}>
      <p>{label}</p>
      <strong>{adaptive ? <span ref={valueRef}>{value}</span> : value}</strong>
    </article>
  )
}

function TrafficFact({ sent, received }: { sent: string; received: string }) {
  return (
    <article className="detail-fact detail-fact--traffic" title={`累计流量: ↑ ${sent}  ↓ ${received}`}>
      <p>累计流量</p>
      <strong className="detail-traffic-values">
        <span className="detail-traffic-value"><span aria-hidden="true">↑</span><span>{sent}</span></span>
        <span className="detail-traffic-value"><span aria-hidden="true">↓</span><span>{received}</span></span>
      </strong>
    </article>
  )
}

function formatUptimeFromBootTime(value: string | undefined): string | null {
  if (!value) return null
  const startedAt = Date.parse(value)
  if (!Number.isFinite(startedAt)) return null
  const seconds = Math.max(0, Math.floor((Date.now() - startedAt) / 1000))
  return formatUptime(seconds)
}

function formatStatusLabel(status: HomeCardNode['status']): string {
  if (status === 'online') return '在线'
  if (status === 'warning') return '异常'
  return '离线'
}

function formatOSLabel(node: HomeCardNode): string {
  return [node.os, node.osVersion].filter(Boolean).join(' ') || '--'
}

function formatSystemSpec(node: HomeCardNode): string {
  return [formatOSLabel(node), node.arch || '--', node.kernel || '--'].filter(Boolean).join(' · ')
}

function formatCpuSpec(node: HomeCardNode): string {
  return [node.cpuModel || '--', formatCores(node.cpuCores, node.virtualization)].filter(Boolean).join(' · ')
}

function formatCores(value: number | null | undefined, virtualization?: string): string {
  const label = coreTypeLabel(virtualization)
  if (value === null || value === undefined) return `-- ${label.plural}`
  const formatted = Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)
  return `${formatted} ${value === 1 ? label.singular : label.plural}`
}

function coreTypeLabel(virtualization?: string): { singular: string; plural: string } {
  const value = virtualization?.trim().toLowerCase() ?? ''
  if (value === '') return { singular: 'Core', plural: 'Cores' }
  const virtualMarkers = [
    'virtual', 'kvm', 'qemu', 'standard pc', 'i440fx', 'piix', 'vmware', 'xen', 'hyper-v', 'bochs',
    'parallels', 'bhyve', 'openvz', 'lxc', 'docker', 'container', 'cloud', 'ec2', 'compute engine',
    'digitalocean', 'vultr', 'linode', 'example-transit', 'tencent', 'huawei', 'azure', 'google', 'amazon',
  ]
  if (virtualMarkers.some((marker) => value.includes(marker))) {
    return { singular: 'Virtual Core', plural: 'Virtual Cores' }
  }
  return { singular: 'Physical Core', plural: 'Physical Cores' }
}

function formatBootTime(value: string | undefined): string {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatBinaryBytes(value: number | null | undefined): string {
  if (value === null || value === undefined) return '--'
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`
}

function formatBinaryUsage(used: number | null | undefined, total: number | null | undefined): string {
  if ((used === null || used === undefined) && (total === null || total === undefined)) return '--'
  return `${formatBinaryBytes(used)} / ${formatBinaryBytes(total)}`
}

function formatLossPercent(value: number | null | undefined): string {
  if (value === null || value === undefined) return 'No data'
  return `${value.toFixed(2)}%`
}

function latestStatePoint(points: StatePoint[]): StatePoint | null {
  return points.length > 0 ? points[points.length - 1] : null
}

function formatUptime(seconds: number): string {
  const safeSeconds = Math.max(0, Math.floor(seconds))
  const days = Math.floor(safeSeconds / 86_400)
  const hours = Math.floor((safeSeconds % 86_400) / 3_600)
  const minutes = Math.floor((safeSeconds % 3_600) / 60)

  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  return `${Math.max(1, minutes)} 分钟`
}

function formatFixed(value: number, digits: number): string {
  return value.toFixed(digits)
}

function BackIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="m15 18-6-6 6-6" />
    </svg>
  )
}
