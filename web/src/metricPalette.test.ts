// @ts-nocheck — Node-side stylesheet contract, matching styles.test.ts.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const css = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')

describe('bright semantic metric palette', () => {
  it('preserves vivid light colors and restores original saturated dark metrics', () => {
    expect(css).toContain('--metric-blue: var(--blue);')
    for (const [name, light, dark] of [
      ['purple', '#8b5cf6', 'var(--purple)'],
      ['red', '#ef4444', '#CD5555'],
      ['gold', '#ca8a04', '#EEAD0E'],
      ['green', '#10b981', 'var(--green)'],
      ['orange', '#f97316', 'var(--orange)'],
    ]) expect(css).toContain(`--metric-${name}: light-dark(${light}, ${dark});`)
  })

  it('preserves the hardware icons under server names and meter status colors', () => {
    expect(css).toContain('.spec-cpu svg { color: #0ea5e9; }')
    expect(css).toContain('.spec-memory svg { color: #10b981; }')
    expect(css).toContain('.spec-disk svg { color: #8b5cf6; }')
    expect(css).toContain('.usage-fill.is-good { background: #22c55e; }')
    expect(css).toContain('.usage-fill.is-warning { background: #f59e0b; }')
    expect(css).toContain('.usage-fill.is-danger { background: #ef4444; }')
  })
})
