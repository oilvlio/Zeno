import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { ServerFlag } from './ServerFlag'

describe('ServerFlag', () => {
  it('renders SVG fallback markup during server render', () => {
    const html = renderToStaticMarkup(<ServerFlag countryCode="HK" className="node-flag" />)

    expect(html).toContain('class="server-flag node-flag"')
    expect(html).toContain('aria-label="HK flag"')
    expect(html).toContain('class="server-flag__image"')
    expect(html).toContain('src="/assets/flags/hk.svg"')
  })

  it('renders TW as Taiwan instead of mapping to CN', () => {
    const html = renderToStaticMarkup(<ServerFlag countryCode="TW" />)

    expect(html).toContain('aria-label="TW flag"')
    expect(html).toContain('src="/assets/flags/tw.svg"')
  })
})
