import { describe, expect, it } from 'vitest'
import { clampAxisTickX, pruneCollidingAxisTicks } from './LatencyChart'

// Exact geometry captured from https://shuijiao.li/server/datawave-hk at a
// 1440px viewport: viewBox 960x320, 14 axis ticks, real rendered char width
// 6.904px at 12px font. This is the case 水饺 reported as overlapping.
const PROD = [
  { txt: '18:44', x: 52, anchor: 'start' as const },
  { txt: '20:00', x: 98.68797776233495, anchor: 'middle' as const },
  { txt: '22:00', x: 172.40583738707437, anchor: 'middle' as const },
  { txt: '0:00', x: 246.12369701181376, anchor: 'middle' as const },
  { txt: '2:00', x: 319.84155663655315, anchor: 'middle' as const },
  { txt: '4:00', x: 393.5594162612926, anchor: 'middle' as const },
  { txt: '6:00', x: 467.277275886032, anchor: 'middle' as const },
  { txt: '8:00', x: 540.9951355107714, anchor: 'middle' as const },
  { txt: '10:00', x: 614.7129951355107, anchor: 'middle' as const },
  { txt: '12:00', x: 688.4308547602502, anchor: 'middle' as const },
  { txt: '14:00', x: 762.1487143849896, anchor: 'middle' as const },
  { txt: '16:00', x: 835.866574009729, anchor: 'middle' as const },
  { txt: '18:00', x: 909.5844336344684, anchor: 'middle' as const },
  { txt: '18:43', x: 936, anchor: 'end' as const },
]

const WIDTH = 960
const PAD = { left: 52, right: 24 }
const CHAR = 7.2

function extent(centre: number, text: string, anchor: 'start' | 'middle' | 'end') {
  const w = text.length * CHAR
  if (anchor === 'start') return { left: centre, right: centre + w }
  if (anchor === 'end') return { left: centre - w, right: centre }
  return { left: centre - w / 2, right: centre + w / 2 }
}

function gapsFor(indices: number[]) {
  const gaps: { between: [string, string]; gap: number }[] = []
  for (let i = 1; i < indices.length; i += 1) {
    const p = PROD[indices[i - 1]]
    const c = PROD[indices[i]]
    gaps.push({
      between: [p.txt, c.txt],
      gap: +(extent(c.x, c.txt, c.anchor).left - extent(p.x, p.txt, p.anchor).right).toFixed(1),
    })
  }
  return gaps
}

describe('production axis geometry that 水饺 reported', () => {
  it('confirms the unfixed axis really does overlap at both ends', () => {
    const overlaps = gapsFor(PROD.map((_, i) => i)).filter((g) => g.gap < 0)
    expect(overlaps.map((o) => o.between.join('/'))).toEqual(['18:44/20:00', '18:00/18:43'])
  })

  it('removes every overlap while keeping the newest label', () => {
    const ticks = PROD.map((_, i) => i)
    const kept = pruneCollidingAxisTicks(
      ticks,
      (t) => clampAxisTickX(PROD[t].x, WIDTH, PAD),
      (t) => PROD[t].txt,
      CHAR,
      (t) => PROD[t].anchor,
    )
    const gaps = gapsFor(kept)
    const worst = Math.min(...gaps.map((g) => g.gap))

    expect(gaps.filter((g) => g.gap < 0)).toEqual([])
    expect(worst).toBeGreaterThanOrEqual(6)
    // Both endpoints state the axis range and must survive; only interior
    // round-hour marks may be sacrificed.
    expect(PROD[kept[0]].txt).toBe('18:44')
    expect(PROD[kept[kept.length - 1]].txt).toBe('18:43')
  })
})
