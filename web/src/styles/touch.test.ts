import postcss from 'postcss'
import { expect, it, vi } from 'vitest'

// Load the real Node module without requiring Node ambient types in the web app.
const { readFileSync } = await vi.importActual<{
  readFileSync(path: URL, encoding: 'utf8'): string
}>('node:fs')

it('keeps calendar controls at their compact base size instead of stretching the painted boxes', () => {
  const touch = postcss.parse(readFileSync(new URL('./touch.css', import.meta.url), 'utf8'))
  const oversizedCalendarRules: string[] = []
  touch.walkRules((rule) => {
    if (!rule.selector.includes('.admin-date-popover')) return
    rule.walkDecls((declaration) => {
      if ((['height', 'min-height', 'width'].includes(declaration.prop) && declaration.value === '44px')
        || (declaration.prop === 'padding' && declaration.value === '2px')
        || (declaration.prop === 'gap' && declaration.value === '0')) {
        oversizedCalendarRules.push(`${rule.selector}: ${declaration.prop}: ${declaration.value}`)
      }
    })
  })
  expect(oversizedCalendarRules).toEqual([])
  const admin = postcss.parse(readFileSync(new URL('./admin.css', import.meta.url), 'utf8'))
  const heights: string[] = []
  admin.walkRules((rule) => {
    if (!rule.selectors.some((selector) => ['.admin-date-current-button', '.admin-date-grid button', '.admin-date-option-panel button'].includes(selector))) return
    rule.walkDecls('height', (declaration) => { heights.push(declaration.value) })
  })
  expect(heights).toEqual(['32px', '30px'])
})

it('keeps home and admin currency menus compact instead of enlarging their rows for touch', () => {
  const touch = postcss.parse(readFileSync(new URL('./touch.css', import.meta.url), 'utf8'))
  const enlargedSelectors: string[] = []
  touch.walkRules((rule) => {
    rule.walkDecls('min-height', (declaration) => {
      if (declaration.value === '44px') enlargedSelectors.push(...rule.selectors)
    })
  })
  expect(enlargedSelectors).not.toContain('.home-currency-popover button')
  expect(enlargedSelectors).not.toContain('.admin-select-popover button')
  expect(enlargedSelectors).toContain('.admin-select-popover:not([data-field$="currency"]) button')
  const fieldSource = readFileSync(new URL('../components/admin/AdminFields.tsx', import.meta.url), 'utf8')
  expect(fieldSource).toContain('className="admin-select-popover" data-field={name}')
})

it.each([
  '.admin-row-actions:has(.admin-row-action.is-icon)',
  '.admin-notification-list .admin-row-actions:has(.admin-row-action.is-icon)',
])('preserves mobile spacing and separates wide-coarse hit targets for %s', (selector) => {
  const touch = postcss.parse(readFileSync(new URL('./touch.css', import.meta.url), 'utf8'))
  const gaps: { media: string; value: string }[] = []
  // The notification ancestor wins over the lazy admin rule even when it loads
  // later (.admin-notification-list .admin-row-actions.admin-icon-actions).
  touch.walkRules((rule) => {
    if (!rule.selectors.includes(selector)) return
    const parent = rule.parent
    if (parent?.type !== 'atrule' || parent.name !== 'media') {
      throw new Error('Action-row touch spacing must be scoped to a media query')
    }
    rule.walkDecls('gap', (declaration) => {
      gaps.push({ media: parent.params, value: declaration.value })
    })
  })
  // Keep 10px on narrow screens (including fine pointers); only wide coarse
  // pointers need 16px because their painted buttons remain 30px wide.
  expect(gaps).toEqual([
    { media: '(pointer: coarse), (max-width: 767px)', value: '10px' },
    { media: '(pointer: coarse) and (min-width: 768px)', value: '16px' },
  ])
})
