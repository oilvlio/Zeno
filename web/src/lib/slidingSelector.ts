import type { CSSProperties } from 'react'

export type SlidingSelectorGeometry = CSSProperties & {
  '--slider-left': string
  '--slider-top': string
  '--slider-width': string
  '--slider-height': string
}

type SlidingSelectorStyle = CSSProperties & {
  '--slider-columns': number
  '--slider-grid-columns': number
  '--slider-row': number
  '--slider-width': string
  '--slider-shift': string
}

interface SelectorRect {
  left: number
  top: number
}

interface OptionRect extends SelectorRect {
  width: number
  height: number
}

function pixel(value: number): string {
  return `${Math.round(value * 1000) / 1000}px`
}

export function slidingSelectorGeometry(selector: SelectorRect, option: OptionRect): SlidingSelectorGeometry {
  return {
    '--slider-left': pixel(option.left - selector.left),
    '--slider-top': pixel(option.top - selector.top),
    '--slider-width': pixel(option.width),
    '--slider-height': pixel(option.height),
  }
}

export function slidingSelectorStyle(optionCount: number, activeIndex: number, maxColumns = optionCount): SlidingSelectorStyle {
  const count = Math.max(1, optionCount)
  const columnLimit = Math.max(1, maxColumns)
  const rows = Math.ceil(count / columnLimit)
  const columns = Math.ceil(count / rows)
  const index = Math.min(Math.max(0, activeIndex), count - 1)
  const column = index % columns
  const row = Math.floor(index / columns)
  return {
    '--slider-columns': count,
    '--slider-grid-columns': columns,
    '--slider-row': row,
    '--slider-width': `calc(100% / ${columns})`,
    '--slider-shift': `${(column * 100) / columns}%`,
  }
}
