import { describe, expect, it } from 'vitest'
import { axisTicksForTimestamps, clampAxisTickX, pruneCollidingAxisTicks } from './LatencyChart'

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

// The reason the axis overlapped was that ticks were chosen by wall-clock hour,
// which makes spacing depend on where the window happens to start. These pin the
// even-interval contract so a future change cannot quietly go back to that.
describe('axis ticks are spaced evenly rather than picked by wall clock', () => {
  const minute = 60_000
  const width = 960
  const pad = { left: 52, right: 24 }
  const charWidth = 7.2

  // A 24h window ending at :57 -- neither endpoint is on an hour boundary, which
  // is exactly the case that used to collide.
  const end = new Date('2026-07-31T18:57:00+08:00').getTime()
  const timestamps = Array.from({ length: 1440 }, (_, i) => end - (1439 - i) * minute)

  const xOf = (t: number) => {
    const span = timestamps[timestamps.length - 1] - timestamps[0]
    return pad.left + ((t - timestamps[0]) / span) * (width - pad.left - pad.right)
  }
  const labelOf = (t: number) => {
    const d = new Date(t)
    return `${d.getHours()}:${d.getMinutes().toString().padStart(2, '0')}`
  }
  const anchorOf = (t: number) => {
    const x = xOf(t)
    if (x <= pad.left + 8) return 'start' as const
    if (x >= width - pad.right - 8) return 'end' as const
    return 'middle' as const
  }

  it('gives every interior gap exactly the same width', () => {
    const ticks = axisTicksForTimestamps(timestamps, 12)
    const interior = ticks.slice(1, -1)
    expect(interior.length).toBeGreaterThan(2)

    // Strict equality is the contract. An earlier version positioned ticks evenly
    // and then snapped each to its nearest hour, which let neighbours round in
    // opposite directions and produced a 2h gap next to a 3h gap. Deriving the
    // positions from one chosen step is what makes this exact.
    const gaps = interior.slice(1).map((t, i) => t - interior[i])
    expect(new Set(gaps).size).toBe(1)

    // Interior marks land on round hours, so the labels read as a sequence.
    expect(interior.every((t) => new Date(t).getMinutes() === 0)).toBe(true)
  })

  it('keeps the true window endpoints even though they are not on the step', () => {
    const ticks = axisTicksForTimestamps(timestamps, 12)
    expect(ticks[0]).toBe(timestamps[0])
    expect(ticks[ticks.length - 1]).toBe(timestamps[timestamps.length - 1])
  })

  it('chooses a step that keeps the tick count within budget', () => {
    // A one-hour window cannot use the same step as a 24h one; the step has to
    // shrink or the axis would show only its endpoints.
    const shortWindow = Array.from({ length: 60 }, (_, i) => end - (59 - i) * minute)
    const ticks = axisTicksForTimestamps(shortWindow, 12)
    expect(ticks.length).toBeLessThanOrEqual(12)
    expect(ticks.length).toBeGreaterThan(2)
  })

  it('drops the mark an endpoint sits on top of, and only that mark', () => {
    // An evenly spaced axis cannot avoid this: the endpoints are not on the step,
    // so whichever mark is nearest can end up only minutes away. Here the window
    // ends at 18:57 and the last step mark is 18:00 -- 57 minutes apart, which
    // overlaps by 19px at this width. Pruning exists for exactly this case.
    const ticks = axisTicksForTimestamps(timestamps, 12)
    const kept = pruneCollidingAxisTicks(ticks, xOf, labelOf, charWidth, anchorOf)

    const removed = ticks.filter((t) => !kept.includes(t))
    expect(removed.map(labelOf)).toEqual(['18:00'])

    // Both endpoints survive; a dropped endpoint would silently misstate the range.
    expect(kept[0]).toBe(timestamps[0])
    expect(kept[kept.length - 1]).toBe(timestamps[timestamps.length - 1])

    // And nothing that remains overlaps.
    const extentOf = (t: number) => {
      const centre = xOf(t)
      const w = labelOf(t).length * charWidth
      const anchor = anchorOf(t)
      if (anchor === 'start') return { left: centre, right: centre + w }
      if (anchor === 'end') return { left: centre - w, right: centre }
      return { left: centre - w / 2, right: centre + w / 2 }
    }
    const extents = kept.map(extentOf)
    for (let i = 1; i < extents.length; i += 1) {
      expect(extents[i].left).toBeGreaterThanOrEqual(extents[i - 1].right)
    }
  })
})
