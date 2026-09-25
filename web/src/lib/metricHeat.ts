// 延迟/丢包热力配色，数值语义与 Monitor-Theme-LuminaPlus 的 metricTone 对齐：
// 延迟用离散五阶（巡检一眼可分），丢包用连续 HSL 渐变。
function clamp01(value: number): number {
  return Math.max(0, Math.min(1, value))
}

function hsl(h: number, s: number, l: number): string {
  const round1 = (value: number) => Math.round(value * 10) / 10
  return `hsl(${round1(h)} ${round1(s)}% ${round1(l)}%)`
}

const HEAT_RAMP_SEGMENTS = [
  (t: number) => hsl(145 - 18 * t, 62 + 8 * t, 48 + 3 * t),
  (t: number) => hsl(127 - 47 * t, 70 + 6 * t, 51 + 1 * t),
  (t: number) => hsl(80 - 30 * t, 76 + 6 * t, 52 + 1 * t),
  (t: number) => hsl(50 - 20 * t, 82 + 4 * t, 53 - 1 * t),
  (t: number) => hsl(30 - 24 * t, 86 - 2 * t, 52 - 8 * t),
]

function heatRamp(value: number, bounds: [number, number, number, number], tailSpan: number): string {
  const [b0, b1, b2, b3] = bounds
  if (value <= b0) return HEAT_RAMP_SEGMENTS[0](clamp01(value / b0))
  if (value <= b1) return HEAT_RAMP_SEGMENTS[1](clamp01((value - b0) / (b1 - b0)))
  if (value <= b2) return HEAT_RAMP_SEGMENTS[2](clamp01((value - b1) / (b2 - b1)))
  if (value <= b3) return HEAT_RAMP_SEGMENTS[3](clamp01((value - b2) / (b3 - b2)))
  return HEAT_RAMP_SEGMENTS[4](clamp01((value - b3) / tailSpan))
}

export function latencyHeatColor(ms: number | null | undefined): string {
  if (ms == null || !Number.isFinite(ms) || ms < 0) return 'var(--muted)'
  if (ms <= 60) return '#2fc66e'
  if (ms <= 100) return '#9fe339'
  if (ms <= 160) return '#cbd83a'
  if (ms <= 200) return '#e2a928'
  return '#dc2626'
}

export function lossHeatColor(pct: number | null | undefined): string {
  if (pct == null || !Number.isFinite(pct) || pct < 0) return 'var(--muted)'
  return heatRamp(pct, [1, 3, 5, 10], 20)
}
