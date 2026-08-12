import type { CSSProperties } from 'react'

type SlidingSelectorStyle = CSSProperties & {
  '--slider-columns': number
  '--slider-width': string
  '--slider-shift': string
}

export function slidingSelectorStyle(optionCount: number, activeIndex: number): SlidingSelectorStyle {
  const count = Math.max(1, optionCount)
  const index = Math.min(Math.max(0, activeIndex), count - 1)
  return {
    '--slider-columns': count,
    '--slider-width': `calc(100% / ${count})`,
    // Anchor the offset to the selector itself. Percentage translateX values are
    // relative to the slider pseudo-element, so any width mismatch compounds
    // as the active option moves to the right.
    '--slider-shift': `${(index * 100) / count}%`,
  }
}
