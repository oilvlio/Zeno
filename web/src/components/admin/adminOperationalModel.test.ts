import { describe, expect, it } from 'vitest'
import { formatCorrectionGB, formatRenewalDayOption, isValidCorrectionGB, normalizeRenewalDays, normalizeRenewalThreshold, parseCorrectionBytes, parseRenewalThreshold, renewalDayOptions } from './adminOperationalModel'

describe('renewal reminder options', () => {
  it('removes same-day reminders and preserves supported lead times', () => {
    expect(renewalDayOptions).toEqual([1, 3, 7, 15, 30])
    expect(parseRenewalThreshold('0')).toBeNull()
    expect(normalizeRenewalThreshold(0)).toBe(1)
  })

  it('labels the calendar-month option separately from day durations', () => {
    expect(formatRenewalDayOption(1)).toBe('提前1天')
    expect(formatRenewalDayOption(15)).toBe('提前半个月')
    expect(formatRenewalDayOption(30)).toBe('提前1个月')
  })

  it('normalizes multiple reminder days without duplicates and keeps the legacy threshold fallback', () => {
    expect(normalizeRenewalDays([7, 1, 7, 2], 3)).toEqual([1, 7])
    expect(normalizeRenewalDays([], 15)).toEqual([15])
  })
})

describe('traffic correction values', () => {
  it('formats stored bytes as GB and leaves empty values blank', () => {
    expect(formatCorrectionGB(1073741824)).toBe('1')
    expect(formatCorrectionGB(1610612736)).toBe('1.5')
    expect(formatCorrectionGB(null)).toBe('')
    expect(formatCorrectionGB(0)).toBe('')
  })

  it('parses GB input to bytes and treats blank as unchanged', () => {
    expect(parseCorrectionBytes('')).toBeUndefined()
    expect(parseCorrectionBytes('   ')).toBeUndefined()
    expect(parseCorrectionBytes('1.5')).toBe(1610612736)
    expect(parseCorrectionBytes('0')).toBe(0)
  })

  it('rejects negative, oversized, and non-numeric input', () => {
    expect(isValidCorrectionGB('')).toBe(true)
    expect(isValidCorrectionGB('0')).toBe(true)
    expect(isValidCorrectionGB('1000000')).toBe(true)
    expect(isValidCorrectionGB('-1')).toBe(false)
    expect(isValidCorrectionGB('1000000.1')).toBe(false)
    expect(isValidCorrectionGB('abc')).toBe(false)
    expect(parseCorrectionBytes('-1')).toBeUndefined()
    expect(parseCorrectionBytes('1000001')).toBeUndefined()
  })
})
