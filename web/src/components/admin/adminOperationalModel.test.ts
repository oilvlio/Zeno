import { describe, expect, it } from 'vitest'
import { formatRenewalDayOption, normalizeRenewalThreshold, parseRenewalThreshold, renewalDayOptions } from './adminOperationalModel'

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
})
