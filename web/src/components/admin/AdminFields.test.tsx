import { describe, expect, it } from 'vitest'
// @ts-ignore -- Node-only acceptance test; the app intentionally has no @types/node.
import { existsSync, readFileSync } from 'node:fs'

// Geometry is also exercised against real Chromium in acceptance/touch-worker probes.
describe('mobile touch stylesheet contract', () => {
  it('loads a scoped touch layer without changing the base theme', () => {
    const url = new URL('../../styles/touch.css', import.meta.url)
    expect(existsSync(url)).toBe(true)
    const css = readFileSync(url, 'utf8')
    expect(css).toMatch(/@media\s*\(pointer:\s*coarse\),\s*\(max-width:\s*767px\)/)
    expect(css).toMatch(/min-height:\s*44px/)
    expect(readFileSync(new URL('../../main.tsx', import.meta.url), 'utf8')).toContain("import './styles/touch.css'")
  })
})
import { adminPopoverExpanded, calculateAnchoredPopoverStyle, measureAnchoredPopoverHeight } from './AdminFields'

const trigger = { top: 100, right: 380, bottom: 140, left: 200, width: 180, height: 40 }

describe('calculateAnchoredPopoverStyle', () => {
  it('places a popover below the trigger when it fits', () => {
    expect(calculateAnchoredPopoverStyle(trigger, { width: 1000, height: 800 }, { width: 240, height: 200 })).toEqual({
      position: 'fixed',
      top: 148,
      left: 200,
      width: 240,
    })
  })

  it('places a popover above the trigger when the lower space is insufficient', () => {
    const lowerTrigger = { ...trigger, top: 600, bottom: 640 }
    expect(calculateAnchoredPopoverStyle(lowerTrigger, { width: 1000, height: 700 }, { width: 240, height: 220 })).toMatchObject({
      top: 372,
      left: 200,
    })
  })

  it('keeps a popover inside the horizontal viewport margin', () => {
    const rightTrigger = { ...trigger, left: 950, right: 1050 }
    expect(calculateAnchoredPopoverStyle(rightTrigger, { width: 1000, height: 800 }, { width: 240, height: 200 })).toMatchObject({
      left: 748,
      width: 240,
    })
  })

  it('keeps a compact mobile calendar inset at 320px and respects scrollbar space', () => {
    const source = readFileSync(new URL('./AdminFields.tsx', import.meta.url), 'utf8')
    expect(source).toContain('const margin = 12')
    expect(source).toContain('touchCalendar ? 312 : 340')
    const mobile = calculateAnchoredPopoverStyle(trigger, { width: 320, height: 568 }, { width: 312, height: 284 })
    expect(mobile).toMatchObject({ left: 12, width: 296 })
    const gutter = calculateAnchoredPopoverStyle(trigger, { width: 305, height: 568 }, { width: 312, height: 284 })
    expect(Number(gutter.left) + Number(gutter.width)).toBeLessThanOrEqual(293)
  })

  it('clamps both dimensions inside a very small viewport', () => {
    expect(calculateAnchoredPopoverStyle(trigger, { width: 280, height: 200 }, { width: 340, height: 354 })).toEqual({
      position: 'fixed',
      top: 12,
      left: 12,
      width: 256,
      maxHeight: 176,
      overflowY: 'auto',
    })
  })
})

describe('measureAnchoredPopoverHeight', () => {
  it('uses the visible border box rather than the scrollable content height', () => {
    expect(typeof measureAnchoredPopoverHeight).toBe('function')
    const menu = { offsetHeight: 320, scrollHeight: 392, style: { maxHeight: '' } } as unknown as HTMLDivElement
    const height = measureAnchoredPopoverHeight(menu, 260)
    expect(height).toBe(320)
    const lowerTrigger = { ...trigger, top: 500, bottom: 544 }
    const style = calculateAnchoredPopoverStyle(lowerTrigger, { width: 390, height: 844 }, { width: 180, height })
    expect(lowerTrigger.top - (Number(style.top) + height)).toBe(8)
  })

  it('removes only the previous viewport cap during measurement and restores it', () => {
    const style = { maxHeight: '176px' }
    const menu = { style, get offsetHeight() { return style.maxHeight === '' ? 320 : 176 } } as unknown as HTMLDivElement
    expect(measureAnchoredPopoverHeight(menu, 260)).toBe(320)
    expect(style.maxHeight).toBe('176px')
  })

  it('uses a fallback only before the element has a measurable layout', () => {
    expect(measureAnchoredPopoverHeight(null, 124)).toBe(124)
    const menu = { style: { maxHeight: '' }, offsetHeight: 0 } as unknown as HTMLDivElement
    expect(measureAnchoredPopoverHeight(menu, 124)).toBe(124)
  })
})

describe('adminPopoverExpanded', () => {
  it('never exposes a disabled popover as expanded', () => {
    expect(adminPopoverExpanded(true, false)).toBe(true)
    expect(adminPopoverExpanded(true, true)).toBe(false)
    expect(adminPopoverExpanded(false, false)).toBe(false)
  })
})
