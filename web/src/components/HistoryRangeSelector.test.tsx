import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { slidingSelectorGeometry, slidingSelectorStyle } from '../lib/slidingSelector'
import { HistoryRangeSelector } from './HistoryRangeSelector'

const options = [
  { value: '1h', label: '实时' },
  { value: '1d', label: '1 天' },
  { value: '3d', label: '3 天' },
  { value: '7d', label: '7 天' },
]

describe('sliding history range selector', () => {
  it('clamps selector geometry to valid columns and indices', () => {
    expect(slidingSelectorStyle(3, 2)).toEqual({
      '--slider-columns': 3,
      '--slider-grid-columns': 3,
      '--slider-row': 0,
      '--slider-width': 'calc(100% / 3)',
      '--slider-shift': `${(2 * 100) / 3}%`,
    })
    expect(slidingSelectorStyle(0, 8)).toEqual({
      '--slider-columns': 1,
      '--slider-grid-columns': 1,
      '--slider-row': 0,
      '--slider-width': 'calc(100% / 1)',
      '--slider-shift': '0%',
    })
    expect(slidingSelectorStyle(9, 8, 8)).toEqual({
      '--slider-columns': 9,
      '--slider-grid-columns': 5,
      '--slider-row': 1,
      '--slider-width': 'calc(100% / 5)',
      '--slider-shift': '60%',
    })
    expect(slidingSelectorStyle(12, 11, 8)).toEqual({
      '--slider-columns': 12,
      '--slider-grid-columns': 6,
      '--slider-row': 1,
      '--slider-width': 'calc(100% / 6)',
      '--slider-shift': `${(5 * 100) / 6}%`,
    })
  })

  it('maps the active button real box into selector-local slider geometry', () => {
    expect(slidingSelectorGeometry(
      { left: 20, top: 100 },
      { left: 112.5, top: 134, width: 46.25, height: 34 },
    )).toEqual({
      '--slider-left': '92.5px',
      '--slider-top': '34px',
      '--slider-width': '46.25px',
      '--slider-height': '34px',
    })
  })

  it('renders one pressed option and falls back to the first option for stale values', () => {
    const html = renderToStaticMarkup(
      <HistoryRangeSelector ariaLabel="history range" options={options} value="missing" onChange={() => {}} />,
    )

    expect(html).toContain('role="group"')
    expect(html).toContain('aria-label="history range"')
    expect(html.match(/aria-pressed="true"/g)).toHaveLength(1)
    expect(html).toMatch(/data-active="true"[^>]*aria-pressed="true"[^>]*>实时<\/button>/)
    expect(html).toContain('--slider-columns:4')
    expect(html).toContain('--slider-shift:0%')
  })
})
