import type { SummaryData } from '../api/publicClient'
import { normalizeCurrencyRates } from './currency'

export const summaryCacheKey = 'zeno_summary_cache_v2'
export const legacySummaryCacheKey = 'zeno_summary_cache_v1'
export const summaryFreshTtlMs = 30 * 1000
export const summaryCacheMaxAgeMs = 24 * 60 * 60 * 1000
export const summaryCacheWriteIntervalMs = 30 * 1000

type SummaryCachePayload = {
  storedAt: number
  data: SummaryData
}

export type StoredSummary = {
  data: SummaryData
  storedAt: number
  stale: boolean
}

function validateSummaryData(value: unknown): SummaryData | null {
  const data = value as Partial<SummaryData> | null
  if (!data || !Array.isArray(data.nodes) || !Array.isArray(data.services)) return null
  return {
    nodes: data.nodes as SummaryData['nodes'],
    services: data.services as SummaryData['services'],
    latencyPoints: Array.isArray(data.latencyPoints) ? data.latencyPoints as SummaryData['latencyPoints'] : [],
    exchangeRates: normalizeCurrencyRates(data.exchangeRates),
  }
}

function parseSummaryPayload(raw: string, now: number): StoredSummary | null {
  const parsed = JSON.parse(raw) as Partial<SummaryCachePayload> | Partial<SummaryData>
  const hasStoredAt = typeof (parsed as Partial<SummaryCachePayload>).storedAt === 'number'
  const storedAt = hasStoredAt ? Number((parsed as Partial<SummaryCachePayload>).storedAt) : 0
  const data = validateSummaryData(hasStoredAt ? (parsed as Partial<SummaryCachePayload>).data : parsed)
  if (!data) return null
  if (storedAt > 0 && now - storedAt > summaryCacheMaxAgeMs) return null
  return { data, storedAt, stale: storedAt <= 0 || now - storedAt > summaryFreshTtlMs }
}

export function loadStoredSummary(now = Date.now()): StoredSummary | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(summaryCacheKey)
    if (raw) return parseSummaryPayload(raw, now)
    const legacyRaw = window.localStorage.getItem(legacySummaryCacheKey)
    return legacyRaw ? parseSummaryPayload(legacyRaw, now) : null
  } catch {
    return null
  }
}

export function rememberSummary(summary: SummaryData, now = Date.now()) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(summaryCacheKey, JSON.stringify({ storedAt: now, data: summary } satisfies SummaryCachePayload))
    window.localStorage.removeItem(legacySummaryCacheKey)
  } catch {}
}

let pendingSummary: SummaryData | null = null
let summaryWriteTimer: number | null = null
let summaryIdleCallback: number | null = null
let lastSummaryWriteAt = 0

function clearScheduledSummaryWrite() {
  if (typeof window === 'undefined') return
  if (summaryWriteTimer !== null) {
    window.clearTimeout(summaryWriteTimer)
    summaryWriteTimer = null
  }
  if (summaryIdleCallback !== null) {
    if (typeof window.cancelIdleCallback === 'function') window.cancelIdleCallback(summaryIdleCallback)
    summaryIdleCallback = null
  }
}

function persistPendingSummary() {
  clearScheduledSummaryWrite()
  if (!pendingSummary) return
  const summary = pendingSummary
  pendingSummary = null
  const now = Date.now()
  rememberSummary(summary, now)
  lastSummaryWriteAt = now
}

function schedulePendingSummaryForIdle() {
  if (typeof window === 'undefined') return
  if (typeof window.requestIdleCallback === 'function') {
    summaryIdleCallback = window.requestIdleCallback(() => {
      summaryIdleCallback = null
      persistPendingSummary()
    }, { timeout: 1000 })
    return
  }
  summaryWriteTimer = window.setTimeout(persistPendingSummary, 0)
}

// Live Summary frames arrive every few seconds. Persist only the newest frame
// once per cache freshness window, and move stringify/localStorage work off the
// active render path. The in-memory dashboard remains fully realtime.
export function scheduleRememberSummary(summary: SummaryData, now = Date.now()) {
  if (typeof window === 'undefined') return
  pendingSummary = summary
  if (summaryWriteTimer !== null || summaryIdleCallback !== null) return
  const wait = summaryCacheWriteDelay(lastSummaryWriteAt, now)
  if (wait === 0) {
    schedulePendingSummaryForIdle()
    return
  }
  summaryWriteTimer = window.setTimeout(() => {
    summaryWriteTimer = null
    schedulePendingSummaryForIdle()
  }, wait)
}

export function summaryCacheWriteDelay(lastWriteAt: number, now: number): number {
  return Math.max(0, summaryCacheWriteIntervalMs - (now - lastWriteAt))
}

export function flushScheduledSummaryWrite() {
  persistPendingSummary()
}

export function resetScheduledSummaryWriteForTests() {
  clearScheduledSummaryWrite()
  pendingSummary = null
  lastSummaryWriteAt = 0
}
