import { describe, expect, it } from 'vitest'
import { appearancePresets, defaultSettings, shellStyleForSettings } from './appearance'

describe('appearance presets', () => {
  it('keeps the default theme balanced over configured backgrounds', () => {
    expect(appearancePresets.default).toMatchObject({
      appearancePreset: 'default',
      cardOpacity: 0.7,
      cardBlur: 0,
      cardRadius: 20,
      borderStrength: 0.3,
      shadowStrength: 0.2,
      backgroundOverlay: 0,
    })
    expect(defaultSettings).toMatchObject(appearancePresets.default)
  })

  it('keeps the Gaussian theme aligned with the current interface geometry and accent', () => {
    expect(appearancePresets.gaussian_blur).toMatchObject({
      appearancePreset: 'gaussian_blur',
      cardRadius: appearancePresets.default.cardRadius,
      themeColor: appearancePresets.default.themeColor,
      cardOpacity: 0.5,
      cardBlur: 15,
      borderStrength: 0.3,
      shadowStrength: 0.3,
      backgroundOverlay: 0.05,
    })
  })

  it('uses one balanced unblurred overlay surface for the default appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'light', backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--zeno-overlay-surface']).toBe('rgba(255, 255, 255, 0.800)')
    expect(style['--zeno-overlay-filter']).toBe('none')
  })

  it('uses the same balanced blurred overlay surface for the Gaussian appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'dark', appearancePreset: 'gaussian_blur', cardOpacity: 0.5, cardBlur: 15, backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--zeno-overlay-surface']).toBe('rgba(15, 23, 42, 0.640)')
    expect(style['--zeno-overlay-filter']).toBe('blur(15px) saturate(1.08)')
  })
})
