import { describe, expect, it } from 'vitest'
import { shouldApplyNodeLatencySnapshot } from './useNodeDetailController'

describe('shouldApplyNodeLatencySnapshot', () => {
  it('suppresses an identical HTTP/WebSocket snapshot but accepts changed data', () => {
    const seen = new Map<string, string>()

    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-a')).toBe(true)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-a')).toBe(false)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-a:1d', 'snapshot-b')).toBe(true)
    expect(shouldApplyNodeLatencySnapshot(seen, 'node-b:1d', 'snapshot-a')).toBe(true)
  })

  it('keeps legacy uncached payloads applicable', () => {
    expect(shouldApplyNodeLatencySnapshot(new Map(), 'node-a:1d')).toBe(true)
  })
})
