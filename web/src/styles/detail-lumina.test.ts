import postcss from 'postcss'
import { describe, expect, it, vi } from 'vitest'

const { readFileSync } = await vi.importActual<{
  readFileSync(path: URL, encoding: 'utf8'): string
}>('node:fs')

const luminaStyles = readFileSync(new URL('./detail-lumina.css', import.meta.url), 'utf8')

function luminaDeclarations(selector: string) {
  const result: Record<string, string> = {}
  postcss.parse(luminaStyles).walkRules((rule) => {
    if (rule.parent?.type !== 'root' || !rule.selectors.includes(selector)) return
    rule.walkDecls((declaration) => { result[declaration.prop] = declaration.value })
  })
  return result
}

describe('detail Lumina overrides', () => {
  it('widens only the detail container instead of the shared content width', () => {
    expect(luminaDeclarations('.detail-container.detail-lumina')).toMatchObject({
      'max-width': 'min(1560px, 100%)',
    })
  })

  it('gives top card and chart panels a bordered Lumina-style surface', () => {
    expect(luminaStyles).toContain('.detail-container.detail-lumina .detail-top-card')
    expect(luminaStyles).toContain('.detail-container.detail-lumina .monitor-panel')
    expect(luminaStyles).toContain('border-radius: 20px')
  })

  it('lays probe cards out as an airy auto-fill grid with large type', () => {
    expect(luminaStyles).toContain('.detail-container.detail-lumina .lumina-probe-grid')
    expect(luminaStyles).toContain('repeat(auto-fill, minmax(250px, 1fr))')
    expect(luminaStyles).toContain('.detail-container.detail-lumina .lumina-probe-primary')
    expect(luminaStyles).toContain('.detail-container.detail-lumina .lumina-loss-chart')
  })

  it('keeps the overlay gradient-free like the rest of the app styles', () => {
    expect(luminaStyles).not.toMatch(/(?:linear|radial)-gradient\(/)
  })
})
