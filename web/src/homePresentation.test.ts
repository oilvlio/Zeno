// @ts-expect-error Node APIs are available in Vitest, not the browser tsconfig.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import postcss from 'postcss'

const root = postcss.parse(readFileSync(new URL('./styles.css', import.meta.url), 'utf8'))
function declarations(selector: string, media?: string) {
  const values: Record<string, string> = {}
  root.walkRules(selector, rule => {
    const parent = rule.parent
    const condition = parent?.type === 'atrule' ? parent.params : undefined
    if (condition !== media) return
    rule.walkDecls(decl => { values[decl.prop] = decl.value })
  })
  return values
}

describe('homepage icon-led presentation', () => {
  it('restores the original compact node name and keeps server reading sizes', () => {
    expect(declarations('.node-title-line p')['font-size']).toBe('13px')
    expect(declarations('.node-title-line p')['font-weight']).toBe('600')
    for (const selector of ['.node-uptime', '.node-specs', '.node-metric', '.node-health-metric', '.node-metric strong', '.health-history-heading strong']) {
      expect(declarations(selector)['font-size'], selector).toBe('11px')
    }
    expect(declarations('.usage-row__meta')['font-size']).toBe('12px')
  })

  it('matches Chinese labels to cards and keeps overview values compact', () => {
    expect(declarations('.home-summary__metric')['min-height']).toBe('40px')
    expect(declarations('.home-summary__metric')['flex-direction']).toBeUndefined()
    expect(declarations('.home-summary__metric-label')['font-size']).toBe(declarations('.node-metric')['font-size'])
    expect(declarations('.home-summary__metric-value strong')['font-size']).toBe('13px')
    expect(declarations('.home-summary__metric-value > span')['font-size']).toBe('11px')
    expect(declarations('.home-summary__metric', '(max-width: 767px)')['min-height']).toBe('34px')
    expect(declarations('.home-summary__metric-label', '(max-width: 360px)')['font-size']).toBeUndefined()
    expect(declarations('.home-summary__metric-value strong', '(max-width: 360px)')['font-size']).toBe('12px')
    expect(declarations('.home-summary__metric-value > span', '(max-width: 360px)')['font-size']).toBe('10px')
    expect(declarations('.home-top-card .home-summary', '(min-width: 768px) and (max-width: 1023px)')['grid-template-columns']).toBe('repeat(3, minmax(0, 1fr))')
  })

  it('uses theme-neutral text and readings while leaving color on the icons', () => {
    for (const selector of ['.home-summary__metric-label', '.home-summary__metric-value', '.home-summary__metric-value strong', '.home-summary__metric-value > span', '.home-summary__metric-value--rate > span', '.metric-label', '.node-metric strong', '.health-history-heading strong', '.usage-row__meta span', '.kulin-node-card.is-capsule .usage-row__meta strong', '.kulin-node-card.is-capsule .usage-row__detail']) {
      expect(declarations(selector).color, selector).toBe('var(--foreground)')
    }
    expect(declarations('.home-summary__metric-icon').color).toBe('var(--summary-accent, var(--orange))')
    expect(declarations('.node-metric').color).toBe('var(--metric-accent)')
    expect(declarations('.health-history-heading').color).toBe('var(--metric-accent)')
    expect(declarations('.home-summary__metric-value--total strong').color).toBeUndefined()
    expect(declarations('.home-summary__metric--total .home-summary__metric-label').color).toBeUndefined()
  })

  it('makes overview icons match card metrics at every viewport', () => {
    for (const selector of ['.home-summary__metric-icon', '.home-summary__metric-icon svg', '.home-summary__status-dot']) {
      expect(declarations(selector).width, selector).toBe('18px')
      expect(declarations(selector).height, selector).toBe('18px')
    }
    for (const selector of ['.metric-icon', '.metric-icon svg']) {
      expect(declarations(selector).width, selector).toBe('18px')
      expect(declarations(selector).height, selector).toBe('18px')
    }
    expect(declarations('.node-spec svg').width).toBe('16px')
    expect(declarations('.node-spec svg').height).toBe('16px')
    expect(declarations('.home-summary__metric-icon,\n  .home-summary__metric-icon svg', '(max-width: 767px)').width).toBeUndefined()
    expect(declarations('.home-summary__status-dot', '(max-width: 767px)').width).toBeUndefined()
    expect(declarations('.home-summary__metric-icon,\n  .home-summary__metric-icon svg', '(max-width: 360px)').width).toBeUndefined()
    expect(declarations('.node-metric').padding).toBe('8px 6px')
  })

  it('adds a little icon-to-text space on cards and the overview', () => {
    expect(declarations('.node-spec').gap).toBe('6px')
    expect(declarations('.metric-heading').gap).toBe('6px')
    expect(declarations('.home-summary__metric-label').gap).toBe('7px')
    expect(declarations('.home-summary__metric-label', '(max-width: 767px)').gap).toBe('6px')
    expect(declarations('.home-summary__metric-label', '(max-width: 360px)').gap).toBe('5px')
    expect(declarations('.home-summary__metric--rate > .home-summary__metric-label', '(max-width: 767px)').width).toBe('calc(2em + 24px)')
    expect(declarations('.home-summary__metric--rate > .home-summary__metric-label', '(max-width: 360px)').width).toBe('calc(2em + 23px)')
  })

  it('preserves light server-only shadows and green-online/red-offline status', () => {
    expect(declarations('.kulin-node-card')['--zeno-node-shadow']).toBe('0 4px 14px -4px rgba(0, 0, 0, .06)')
    expect(declarations('.kulin-node-card')['box-shadow']).toBe('var(--zeno-node-shadow)')
    expect(declarations(':root')['--zeno-node-shadow']).toBeUndefined()
    expect(declarations('.home-top-card')['--zeno-node-shadow']).toBeUndefined()
    expect(declarations('.kulin-node-card.is-offline .node-uptime').color).toBe('var(--metric-red)')
    expect(declarations('.node-uptime').color).toBe('var(--metric-green)')
    expect(declarations(':root')['--background']).toBe('light-dark(#f5f6f8, #07111f)')
  })
})
