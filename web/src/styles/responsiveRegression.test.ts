import postcss from 'postcss'
import { expect, it, vi } from 'vitest'

const { readFileSync } = await vi.importActual<{
  readFileSync(path: URL, encoding: 'utf8'): string
}>('node:fs')

function baseDeclarations(file: string, selector: string) {
  const result: Record<string, string> = {}
  postcss.parse(readFileSync(new URL(file, import.meta.url), 'utf8')).walkRules((rule) => {
    if (rule.parent?.type !== 'root' || !rule.selectors.includes(selector)) return
    rule.walkDecls((declaration) => { result[declaration.prop] = declaration.value })
  })
  return result
}

it('keeps the navigation title inside its available width without shrinking action controls', () => {
  expect(baseDeclarations('../styles.css', '.brand > span:last-child')).toMatchObject({
    'min-width': '0', overflow: 'hidden', 'text-overflow': 'ellipsis', 'white-space': 'nowrap',
  })
  expect(baseDeclarations('../styles.css', '.brand-logo')).toMatchObject({ flex: 'none' })
  expect(baseDeclarations('../styles.css', '.nav-actions')).toMatchObject({ flex: 'none' })
})

it('wraps complete traffic values instead of shrinking and clipping their units', () => {
  expect(baseDeclarations('./detail.css', '.detail-traffic-values')).toMatchObject({
    'flex-wrap': 'wrap', 'row-gap': '3px',
  })
  expect(baseDeclarations('./detail.css', '.detail-traffic-value')).toMatchObject({ flex: 'none' })
})
