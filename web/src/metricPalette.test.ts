// @ts-nocheck — Node-side stylesheet contract, matching styles.test.ts.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const css = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')

describe('bright semantic metric palette', () => {
  it('uses vivid light colors without black mixing and preserves dark colors', () => {
    expect(css).toContain('--metric-blue: light-dark(var(--blue), color-mix(in srgb, var(--blue) 36%, white));')
    for (const [name, light, dark] of [
      ['purple', '#8b5cf6', '#ddc0ff'],
      ['red', '#ef4444', '#ffb4b4'],
      ['gold', '#ca8a04', '#ffe08a'],
      ['green', '#10b981', '#9aefc7'],
      ['orange', '#f97316', '#ffcca3'],
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
