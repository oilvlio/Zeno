import { describe, expect, it, vi } from 'vitest'
import { consumeAnchoredPopoverEscape, createAnchoredPopoverStack } from './useDismissibleAnchoredPopover'

describe('dismissible anchored popover lifecycle', () => {
  it('dismisses the previous popover and keeps only the latest one topmost', () => {
    const stack = createAnchoredPopoverStack()
    const dismissFirst = vi.fn()
    stack.push(1, dismissFirst)
    expect(stack.isTopmost(1)).toBe(true)
    stack.push(2, vi.fn())
    expect(dismissFirst).toHaveBeenCalledOnce()
    expect(stack.isTopmost(1)).toBe(false)
    expect(stack.isTopmost(2)).toBe(true)
    stack.remove(2)
    expect(stack.isTopmost(1)).toBe(false)
  })

  it('consumes Escape before a parent modal can observe it', () => {
    const event = {
      key: 'Escape',
      preventDefault: vi.fn(),
      stopPropagation: vi.fn(),
      stopImmediatePropagation: vi.fn(),
    }
    expect(consumeAnchoredPopoverEscape(event)).toBe(true)
    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(event.stopPropagation).toHaveBeenCalledOnce()
    expect(event.stopImmediatePropagation).toHaveBeenCalledOnce()
  })

  it('does not consume unrelated keys', () => {
    const event = {
      key: 'Enter',
      preventDefault: vi.fn(),
      stopPropagation: vi.fn(),
      stopImmediatePropagation: vi.fn(),
    }
    expect(consumeAnchoredPopoverEscape(event)).toBe(false)
    expect(event.stopPropagation).not.toHaveBeenCalled()
  })
})
