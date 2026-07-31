import { afterEach, describe, expect, it, vi } from 'vitest'
import { extractSafeCustomCSS } from './customCode'
import { availableHistoryRanges, coerceHistoryRange } from './historyRange'
import { flushScheduledSummaryWrite, loadStoredSummary, rememberSummary, resetScheduledSummaryWriteForTests, scheduleRememberSummary, summaryCacheWriteDelay, summaryCacheWriteIntervalMs, summaryFreshTtlMs } from './summaryCache'
import type { SummaryData } from '../api/publicClient'

const summary: SummaryData = {
  nodes: [],
  services: [],
  latencyPoints: [],
  exchangeRates: { CNY: 1 },
}

function installWindowStorage() {
  const storage = new Map<string, string>()
  let writes = 0
  const windowStub = {
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => { writes += 1; storage.set(key, value) },
      removeItem: (key: string) => storage.delete(key),
    },
    setTimeout: globalThis.setTimeout,
    clearTimeout: globalThis.clearTimeout,
  }
  const previousWindow = globalThis.window
  Object.defineProperty(globalThis, 'window', { value: windowStub, configurable: true })
  return {
    restore: () => Object.defineProperty(globalThis, 'window', { value: previousWindow, configurable: true }),
    writes: () => writes,
  }
}

describe('realtime reliability helpers', () => {
  afterEach(() => {
    // Ensure tests that stubbed window do not leak a storage object.
    if (typeof window !== 'undefined' && !('document' in window)) {
      Reflect.deleteProperty(globalThis, 'window')
    }
  })

  it('marks stored summary data stale after the short freshness TTL', () => {
    const { restore } = installWindowStorage()
    try {
      rememberSummary(summary, 1_000)
      expect(loadStoredSummary(1_000 + summaryFreshTtlMs - 1)).toMatchObject({ data: summary, stale: false, storedAt: 1_000 })
      expect(loadStoredSummary(1_000 + summaryFreshTtlMs + 1)).toMatchObject({ data: summary, stale: true, storedAt: 1_000 })
    } finally {
      restore()
    }
  })

  it('limits live summary persistence to one write per freshness window', () => {
    expect(summaryCacheWriteDelay(0, summaryCacheWriteIntervalMs)).toBe(0)
    expect(summaryCacheWriteDelay(10_000, 10_001)).toBe(summaryCacheWriteIntervalMs - 1)
    expect(summaryCacheWriteDelay(10_000, 10_000 + summaryCacheWriteIntervalMs + 1)).toBe(0)
  })

  it('coalesces live frames and persists only the newest summary', () => {
    vi.useFakeTimers()
    vi.setSystemTime(30_000)
    const { restore, writes } = installWindowStorage()
    resetScheduledSummaryWriteForTests()
    try {
      scheduleRememberSummary(summary, 30_000)
      scheduleRememberSummary({ ...summary, exchangeRates: { CNY: 1, USD: 2 } }, 30_001)
      vi.runOnlyPendingTimers()
      expect(writes()).toBe(1)
      expect(loadStoredSummary(30_001)?.data.exchangeRates.USD).toBe(2)

      vi.setSystemTime(30_002)
      scheduleRememberSummary({ ...summary, exchangeRates: { CNY: 1, USD: 3 } }, 30_002)
      vi.advanceTimersByTime(summaryCacheWriteIntervalMs - 3)
      expect(writes()).toBe(1)
      vi.advanceTimersByTime(1)
      vi.runOnlyPendingTimers()
      expect(writes()).toBe(2)
      expect(loadStoredSummary(60_001)?.data.exchangeRates.USD).toBe(3)
    } finally {
      flushScheduledSummaryWrite()
      resetScheduledSummaryWriteForTests()
      restore()
      vi.useRealTimers()
    }
  })

  it('limits unauthenticated history ranges to realtime and one day', () => {
    expect(availableHistoryRanges(false).map((option) => option.value)).toEqual(['1h', '1d'])
    expect(availableHistoryRanges(true).map((option) => option.value)).toEqual(['1h', '1d', '7d', '30d'])
    expect(coerceHistoryRange('30d', false, '1d')).toBe('1d')
    expect(coerceHistoryRange('30d', true, '1d')).toBe('30d')
  })

  it('keeps appearance CSS but strips executable custom code', () => {
    const css = extractSafeCustomCSS('<style>.home-top-card { border-color: #2563eb; background: url(javascript:alert(1)); }</style><img onerror="alert(1)"><script>alert(1)</script>')
    expect(css).toContain('.home-top-card { border-color: #2563eb;')
    expect(css).not.toContain('<script>')
    expect(css).not.toContain('onerror')
    expect(css).not.toContain('javascript:alert')
    expect(css).toContain('url(about:blank)')
  })
})
