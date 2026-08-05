import { describe, expect, it } from 'vitest'
import { appearancePresets, defaultSettings, shellStyleForSettings } from './appearance'

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

  it('uses an opaque unblurred overlay surface for the default appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'light', backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--zeno-overlay-surface']).toBe('rgba(255, 255, 255, 0.920)')
    expect(style['--zeno-menu-surface']).toBe('transparent')
    expect(style['--zeno-overlay-filter']).toBe('none')
  })

  it('uses a theme-colored blurred overlay surface for the Gaussian appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'dark', appearancePreset: 'gaussian_blur', cardOpacity: 0.66, cardBlur: 18, backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--zeno-overlay-surface']).toBe('rgba(15, 23, 42, 0.502)')
    expect(style['--zeno-menu-surface']).toBe('rgba(15, 23, 42, 0.580)')
    expect(style['--zeno-overlay-filter']).toBe('blur(18px) saturate(1.08)')
  })
})
