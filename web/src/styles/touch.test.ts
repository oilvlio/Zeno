import postcss from 'postcss'
import { expect, it, vi } from 'vitest'

// Load the real Node module without requiring Node ambient types in the web app.
const { readFileSync } = await vi.importActual<{
  readFileSync(path: URL, encoding: 'utf8'): string
}>('node:fs')

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
