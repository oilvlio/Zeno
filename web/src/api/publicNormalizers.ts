import type { AdminSettings, HomeCardNode, LatencyPoint, ServiceTarget, StatePoint } from '../types'
import { normalizeCurrencyRates } from '../lib/currency'
import type { ApiLatencyPoint, ApiLatencyResponse, ApiLatencySeries, ApiLatencySummary, ApiNode, ApiServiceLatencyPoint, ApiServiceLatencyResponse, ApiServiceLatencySeries, ApiServiceTarget, ApiSettings, ApiStatePoint, ApiStateResponse, ApiStateSeries, NodeLatencyData, NodeStateData, ServiceLatencyData, SummaryData, ApiSummaryResponse } from './apiTypes'

export function normalizeSettings(input: ApiSettings): AdminSettings {
  const logoUrl = input.logo_url
  const desktopBackgroundUrl = input.desktop_background_url ?? input.background_url
  return {
    siteTitle: input.site_title,
    siteSubtitle: input.site_subtitle,
    logoUrl,
    theme: input.theme ?? 'system',
    agentControllerUrl: input.agent_controller_url ?? '',
    backgroundUrl: desktopBackgroundUrl,
    desktopBackgroundUrl,
    mobileBackgroundUrl: input.mobile_background_url ?? '',
    appearancePreset: input.appearance_preset ?? 'default',
    cardOpacity: input.card_opacity ?? 0.72,
    cardBlur: input.card_blur ?? 0,
    cardRadius: input.card_radius ?? 20,
    borderStrength: input.border_strength ?? 0.26,
    shadowStrength: input.shadow_strength ?? 0.22,
    backgroundOverlay: input.background_overlay ?? 0,
    themeColor: input.theme_color ?? '#2563eb',
    customCode: input.custom_code ?? '',
    updatedAt: input.updated_at,
  }
}

export function normalizeSummary(input: ApiSummaryResponse): SummaryData {
  return {
    nodes: (input.nodes ?? []).map(normalizeNode),
    services: (input.services ?? []).map(normalizeServiceTarget),
    latencyPoints: (input.latency_points ?? []).map(normalizeLatencyPoint),
    exchangeRates: normalizeCurrencyRates(input.exchange_rates),
  }
}

export function normalizeNodeLatency(input: ApiLatencyResponse): NodeLatencyData {
  return {
    nodeId: input.node_id,
    range: input.range,
    points: normalizeNodeLatencyPoints(input),
  }
}

export function normalizeServiceLatency(input: ApiServiceLatencyResponse): ServiceLatencyData {
  return {
    target: normalizeServiceTarget(input.target),
    range: input.range,
    points: normalizeServiceLatencyPoints(input),
  }
}

export function normalizeNodeState(input: ApiStateResponse): NodeStateData {
  return {
    nodeId: input.node_id,
    range: input.range,
    points: normalizeNodeStatePoints(input),
  }
}

export function normalizeNode(node: ApiNode): HomeCardNode {
  return {
    id: node.id,
    displayName: node.display_name,
    status: node.status,
    os: node.os,
    osVersion: node.os_version,
    kernel: node.kernel,
    arch: node.arch,
    virtualization: node.virtualization,
    cpuModel: node.cpu_model,
    countryCode: node.country_code,
    subtitle: node.subtitle,
    cpuCores: node.cpu_cores ?? null,
    expiryLabel: node.expiry_label,
    renewalAmount: node.renewal_amount ?? null,
    renewalCurrency: node.renewal_currency,
    billingCycle: node.billing_cycle,
    monthlyCostCny: node.monthly_cost_cny ?? null,
    cpuPercent: node.cpu_percent,
    memoryUsedBytes: node.memory_used_bytes,
    memoryTotalBytes: node.memory_total_bytes,
    diskUsedBytes: node.disk_used_bytes,
    diskTotalBytes: node.disk_total_bytes,
    bootTime: node.boot_time ?? undefined,
    load1: node.load1 ?? null,
    load5: node.load5 ?? null,
    load15: node.load15 ?? null,
    uptimeSeconds: node.uptime_seconds ?? null,
    netInSpeedBps: node.net_in_speed_bps,
    netOutSpeedBps: node.net_out_speed_bps,
    netInTotalBytes: node.net_in_total_bytes,
    netOutTotalBytes: node.net_out_total_bytes,
    netInLifetimeBytes: node.net_in_lifetime_bytes,
    netOutLifetimeBytes: node.net_out_lifetime_bytes,
    billingMode: node.billing_mode,
    monthlyResetDay: node.monthly_reset_day,
    monthlyPeriodStart: node.monthly_period_start,
    monthlyPeriodEnd: node.monthly_period_end,
    monthlyBillableBytes: node.monthly_billable_bytes,
    monthlyQuotaBytes: node.monthly_quota_bytes,
    latencySummary: node.latency_summary ? normalizeLatencySummary(node.latency_summary) : undefined,
    latencySummaries: (node.latency_summaries ?? []).map(normalizeLatencySummary),
  }
}

export function normalizeLatencySummary(summary: ApiLatencySummary) {
  return {
    targetId: summary.target_id,
    targetName: summary.target_name,
    medianMs: summary.median_ms,
    avgMs: summary.avg_ms ?? null,
    lossPercent: summary.loss_percent,
    updatedAt: summary.updated_at,
  }
}

export function normalizeLatencyPoint(point: ApiLatencyPoint): LatencyPoint {
  return {
    ts: point.ts,
    targetId: point.target_id,
    targetName: point.target_name,
    medianMs: point.median_ms,
    avgMs: point.avg_ms ?? null,
    lossPercent: point.loss_percent,
  }
}

export function normalizeNodeLatencyPoints(input: ApiLatencyResponse): LatencyPoint[] {
  if (input.points) return input.points.map(normalizeLatencyPoint)
  const sharedCreatedAt = input.created_at ?? []
  return (input.series ?? []).flatMap((series) => {
    const medianValues = series.median_ms ?? []
    const avgValues = series.avg_ms ?? []
    const lossValues = series.loss_percent ?? []
    return (series.created_at ?? sharedCreatedAt).map((createdAt, index) => {
      const medianMs = medianValues[index] ?? null
      return {
        ts: normalizeSeriesTimestamp(createdAt),
        targetId: series.target_id,
        targetName: series.target_name,
        medianMs,
        avgMs: avgValues[index] ?? null,
        lossPercent: lossValues[index] ?? 0,
      }
    })
  })
}

export function normalizeServiceLatencyPoints(input: ApiServiceLatencyResponse): LatencyPoint[] {
  if (input.points) return input.points.map(normalizeServiceLatencyPoint)
  const sharedCreatedAt = input.created_at ?? []
  return (input.series ?? []).flatMap((series) => {
    const medianValues = series.median_ms ?? []
    const avgValues = series.avg_ms ?? []
    const lossValues = series.loss_percent ?? []
    return (series.created_at ?? sharedCreatedAt).map((createdAt, index) => {
      const medianMs = medianValues[index] ?? null
      return {
        ts: normalizeSeriesTimestamp(createdAt),
        targetId: series.node_id,
        targetName: series.node_name,
        medianMs,
        avgMs: avgValues[index] ?? null,
        lossPercent: lossValues[index] ?? 0,
      }
    })
  })
}

export function normalizeSeriesTimestamp(value: number): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return new Date(0).toISOString()
  return date.toISOString()
}

export function normalizeServiceTarget(target: ApiServiceTarget): ServiceTarget {
  return {
    id: target.id,
    name: target.name,
    type: target.type,
    assignedNodeCount: target.assigned_node_count,
    reportingNodeCount: target.reporting_node_count,
    medianMs: target.median_ms,
    avgMs: target.avg_ms ?? null,
    lossPercent: target.loss_percent,
    updatedAt: target.updated_at,
  }
}

export function normalizeServiceLatencyPoint(point: ApiServiceLatencyPoint): LatencyPoint {
  return {
    ts: point.ts,
    targetId: point.node_id,
    targetName: point.node_name,
    medianMs: point.median_ms,
    avgMs: point.avg_ms ?? null,
    lossPercent: point.loss_percent,
  }
}

export function normalizeStatePoint(point: ApiStatePoint): StatePoint {
  return {
    ts: point.ts,
    cpuPercent: point.cpu_percent,
    load1: point.load1 ?? null,
    load5: point.load5 ?? null,
    load15: point.load15 ?? null,
    memoryUsedBytes: point.memory_used_bytes,
    memoryTotalBytes: point.memory_total_bytes,
    swapUsedBytes: point.swap_used_bytes ?? null,
    swapTotalBytes: point.swap_total_bytes ?? null,
    diskUsedBytes: point.disk_used_bytes,
    diskTotalBytes: point.disk_total_bytes,
    netInTotalBytes: point.net_in_total_bytes,
    netOutTotalBytes: point.net_out_total_bytes,
    netInSpeedBps: point.net_in_speed_bps,
    netOutSpeedBps: point.net_out_speed_bps,
    processCount: point.process_count ?? null,
    tcpConnectionCount: point.tcp_connection_count ?? null,
    udpConnectionCount: point.udp_connection_count ?? null,
    uptimeSeconds: point.uptime_seconds,
  }
}

export function normalizeNodeStatePoints(input: ApiStateResponse): StatePoint[] {
  if (input.points) return input.points.map(normalizeStatePoint)
  const timestamps = input.created_at ?? []
  const series = input.series ?? {}
  return timestamps.map((createdAt, index) => ({
    ts: normalizeSeriesTimestamp(createdAt),
    cpuPercent: stateSeriesValue(series.cpu_percent, index),
    load1: stateSeriesValue(series.load1, index),
    load5: stateSeriesValue(series.load5, index),
    load15: stateSeriesValue(series.load15, index),
    memoryUsedBytes: stateSeriesValue(series.memory_used_bytes, index),
    memoryTotalBytes: stateSeriesValue(series.memory_total_bytes, index),
    swapUsedBytes: stateSeriesValue(series.swap_used_bytes, index),
    swapTotalBytes: stateSeriesValue(series.swap_total_bytes, index),
    diskUsedBytes: stateSeriesValue(series.disk_used_bytes, index),
    diskTotalBytes: stateSeriesValue(series.disk_total_bytes, index),
    netInTotalBytes: stateSeriesValue(series.net_in_total_bytes, index),
    netOutTotalBytes: stateSeriesValue(series.net_out_total_bytes, index),
    netInSpeedBps: stateSeriesValue(series.net_in_speed_bps, index),
    netOutSpeedBps: stateSeriesValue(series.net_out_speed_bps, index),
    processCount: stateSeriesValue(series.process_count, index),
    tcpConnectionCount: stateSeriesValue(series.tcp_connection_count, index),
    udpConnectionCount: stateSeriesValue(series.udp_connection_count, index),
    uptimeSeconds: stateSeriesValue(series.uptime_seconds, index),
  }))
}

export function stateSeriesValue(values: Array<number | null> | null | undefined, index: number): number | null {
  if (!values || index < 0 || index >= values.length) return null
  const value = values[index]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}
