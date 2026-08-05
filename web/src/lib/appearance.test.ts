import { describe, expect, it } from 'vitest'
import { appearancePresets, defaultSettings } from './appearance'

describe('appearance presets', () => {
  it('keeps the default theme comfortably opaque over configured backgrounds', () => {
    expect(appearancePresets.default).toMatchObject({
      appearancePreset: 'default',
      cardOpacity: 0.82,
      cardBlur: 0,
    })
    expect(defaultSettings).toMatchObject(appearancePresets.default)
  })

  it('keeps the Gaussian theme aligned with the current interface geometry and accent', () => {
    expect(appearancePresets.gaussian_blur).toMatchObject({
      appearancePreset: 'gaussian_blur',
      cardRadius: appearancePresets.default.cardRadius,
      themeColor: appearancePresets.default.themeColor,
      cardOpacity: 0.66,
      cardBlur: 18,
      borderStrength: 0.34,
      shadowStrength: 0.34,
      backgroundOverlay: 0.08,
    })
  })
})
