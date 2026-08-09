import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { moduleEntryAssetURLs, resolveDistAssetPath, staticModuleAssetURLs } from './performance-budget-assets.mjs'

describe('performance budget initial JavaScript graph', () => {
  it('collects the module entry and every modulepreload regardless of attribute order', () => {
    const html = `
      <script crossorigin src="/assets/index.js" type="module"></script>
      <link href='/assets/shared.js' crossorigin rel='modulepreload'>
      <link rel="preload modulepreload" href="/assets/vendor.js">
      <link rel="stylesheet" href="/assets/index.css">
      <script src="/assets/legacy.js"></script>
      <link rel="modulepreload" href="/assets/shared.js">
    `
    expect(staticModuleAssetURLs(html)).toEqual([
      '/assets/index.js',
      '/assets/shared.js',
      '/assets/vendor.js',
    ])
    expect(moduleEntryAssetURLs(html)).toEqual(['/assets/index.js'])
  })

  it('resolves same-origin assets inside dist and rejects external assets', () => {
    expect(resolveDistAssetPath('/tmp/zeno-dist', '/assets/index.js?v=1'))
      .toBe(path.resolve('/tmp/zeno-dist/assets/index.js'))
    expect(() => resolveDistAssetPath('/tmp/zeno-dist', 'https://example.com/index.js'))
      .toThrow(/external initial JavaScript asset/)
  })
})
