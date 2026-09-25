import { describe, expect, it } from 'vitest'
import { latencyHeatColor, lossHeatColor } from './metricHeat'

describe('latencyHeatColor', () => {
  it('uses the five discrete steps', () => {
    expect(latencyHeatColor(0)).toBe('#2fc66e')
    expect(latencyHeatColor(60)).toBe('#2fc66e')
    expect(latencyHeatColor(61)).toBe('#9fe339')
    expect(latencyHeatColor(100)).toBe('#9fe339')
    expect(latencyHeatColor(101)).toBe('#cbd83a')
    expect(latencyHeatColor(160)).toBe('#cbd83a')
    expect(latencyHeatColor(161)).toBe('#e2a928')
    expect(latencyHeatColor(200)).toBe('#e2a928')
    expect(latencyHeatColor(201)).toBe('#dc2626')
  })

  it('falls back to muted without a real sample', () => {
    expect(latencyHeatColor(null)).toBe('var(--muted)')
    expect(latencyHeatColor(undefined)).toBe('var(--muted)')
    expect(latencyHeatColor(Number.NaN)).toBe('var(--muted)')
    expect(latencyHeatColor(-1)).toBe('var(--muted)')
  })
})

describe('lossHeatColor', () => {
  it('ramps green to red through the same bounds', () => {
    expect(lossHeatColor(0)).toBe('hsl(145 62% 48%)')
    expect(lossHeatColor(5)).toBe('hsl(50 82% 53%)')
    expect(lossHeatColor(100)).toBe('hsl(6 84% 44%)')
  })

  it('falls back to muted without a real sample', () => {
    expect(lossHeatColor(null)).toBe('var(--muted)')
    expect(lossHeatColor(undefined)).toBe('var(--muted)')
    expect(lossHeatColor(Number.NaN)).toBe('var(--muted)')
  })
})
