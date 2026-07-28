import { describe, expect, it } from 'vitest'
import { availableCurrencyOptions, billingCycleMonths, convertCurrencyAmount, formatCurrencyAmount, homeCurrencyStorageKey, normalizeCurrencyCode, normalizeCurrencyRates, rememberHomeCurrency, storedHomeCurrency } from './currency'

describe('currency display helpers', () => {
  it('normalizes public rates and always keeps CNY as the base display currency', () => {
    expect(normalizeCurrencyRates({ CNY: 9, USD: 7.2, EUR: 0, BAD: 4, JPY: Number.NaN })).toEqual({ CNY: 1, USD: 7.2 })
    expect(availableCurrencyOptions({ CNY: 1, USD: 7.2 }).map((option) => option.value)).toEqual(['CNY', 'USD'])
    expect(availableCurrencyOptions({ CNY: 1, USD: 7.2 }).map((option) => option.flagCode)).toEqual(['CN', 'US'])
  })

  it('does not offer SGD or KRW in the homepage currency menu', () => {
    const rates = { CNY: 1, USD: 7.2, HKD: 0.92, EUR: 8.4, GBP: 9.7, JPY: 0.05, SGD: 5.4, AUD: 4.8, CAD: 5.2, KRW: 0.005 }
    expect(availableCurrencyOptions(rates).map((option) => option.value)).toEqual(['CNY', 'USD', 'HKD', 'EUR', 'GBP', 'JPY', 'AUD', 'CAD'])
  })

  it('converts through CNY-per-unit rates and formats compact card prices', () => {
    const rates = { CNY: 1, USD: 8, EUR: 10 }
    expect(convertCurrencyAmount(20, 'USD', 'CNY', rates)).toBe(160)
    expect(convertCurrencyAmount(20, 'USD', 'EUR', rates)).toBe(16)
    expect(convertCurrencyAmount(20, 'USD', 'JPY', rates)).toBeNull()
    expect(formatCurrencyAmount(160, 'CNY')).toBe('¥160')
    expect(formatCurrencyAmount(11.0625, 'USD', { fixed: true, spaced: true })).toBe('$ 11.06')
    expect(billingCycleMonths('年付')).toBe(12)
    expect(billingCycleMonths('半年')).toBe(6)
    expect(billingCycleMonths('三年')).toBe(36)
  })

  it('defaults invalid selections to CNY and persists a valid browser preference', () => {
    const values = new Map<string, string>()
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    }

    expect(normalizeCurrencyCode('unknown')).toBe('CNY')
    expect(storedHomeCurrency(storage)).toBe('CNY')
    rememberHomeCurrency('EUR', storage)
    expect(values.get(homeCurrencyStorageKey)).toBe('EUR')
    expect(storedHomeCurrency(storage)).toBe('EUR')
  })
})
