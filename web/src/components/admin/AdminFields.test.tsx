import { describe, expect, it } from 'vitest'
import { adminPopoverExpanded, calculateAnchoredPopoverStyle } from './AdminFields'

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

describe('adminPopoverExpanded', () => {
  it('never exposes a disabled popover as expanded', () => {
    expect(adminPopoverExpanded(true, false)).toBe(true)
    expect(adminPopoverExpanded(true, true)).toBe(false)
    expect(adminPopoverExpanded(false, false)).toBe(false)
  })
})
