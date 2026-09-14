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
    expect(defaultSettings.logoUrl).toBe('/assets/logo/id.png')
    expect(JSON.stringify(defaultSettings)).not.toContain('cdn.jsdelivr.net')
  })

  it('tunes only the Gaussian preset material while preserving existing geometry', () => {
    expect(appearancePresets.gaussian_blur).toEqual({
      appearancePreset: 'gaussian_blur',
      cardRadius: 20,
      themeColor: '#0071e3',
      cardOpacity: 0.8,
      cardBlur: 20,
      borderStrength: 0.08,
      shadowStrength: 0.08,
      backgroundOverlay: 0.08,
    })
    expect(appearancePresets.default.themeColor).toBe('#2563eb')
  })

  it.each([
    ['light', '255, 255, 255', '#1d1d1f', '#48484a', '0, 0, 0'],
    ['dark', '15, 23, 42', '#f8fafc', '#cbd5e1', '37, 99, 235'],
  ] as const)('keeps light Gaussian colors and restores original slate tokens in %s mode', (theme, base, foreground, muted, border) => {
    const settings = { ...defaultSettings, ...appearancePresets.gaussian_blur, theme, backgroundUrl: '/wallpaper.webp' }
    expect(shellStyleForSettings(settings)).toMatchObject({
      '--foreground': foreground,
      '--muted': muted,
      '--border': `rgba(${border}, 0.080)`,
      '--page-surface': `rgba(${base}, 0.800)`,
      '--admin-secondary-surface': `rgb(${base})`,
      '--zeno-card-filter': 'blur(20px) saturate(1.2)',
      '--zeno-card-shadow': '0 5px 30px rgba(0, 0, 0, 0.080)',
      '--zeno-overlay-shadow': '0 5px 30px rgba(0, 0, 0, 0.080)',
      '--zeno-modal-filter': 'none',
      '--zeno-modal-backdrop-filter': 'blur(20px) saturate(1.2)',
      '--radius-panel': '20px',
      '--radius-card': '16px',
      '--radius-field': '12px',
    })
    expect(shellStyleForSettings({ ...settings, backgroundUrl: '' })).toMatchObject({ '--page-surface': `rgb(${base})` })
  })

  it('honors custom Gaussian sliders, including literal none at zero blur and shadow', () => {
    const settings = { ...defaultSettings, ...appearancePresets.gaussian_blur, theme: 'light' as const, backgroundUrl: '/wallpaper.webp', cardOpacity: 0.6, cardBlur: 0, shadowStrength: 0, borderStrength: 0, backgroundOverlay: 0.2, themeColor: '#aabbcc' }
    expect(shellStyleForSettings(settings)).toMatchObject({
      '--page-surface': 'rgba(255, 255, 255, 0.600)',
      '--blue': '#aabbcc',
      '--border': 'rgba(0, 0, 0, 0.000)',
      '--zeno-card-filter': 'none',
      '--zeno-overlay-filter': 'none',
      '--zeno-modal-filter': 'none',
      '--zeno-modal-backdrop-filter': 'none',
      '--zeno-card-shadow': 'none',
      '--zeno-background-overlay-color': 'rgba(255, 255, 255, 0.200)',
    })
  })

  it('restores the original dark accent without changing light or custom colors', () => {
    const settings = { ...defaultSettings, ...appearancePresets.gaussian_blur }
    expect(shellStyleForSettings({ ...settings, theme: 'dark' })).toMatchObject({ '--blue': '#2563eb' })
    expect(shellStyleForSettings({ ...settings, theme: 'light' })).toMatchObject({ '--blue': '#0071e3' })
    expect(shellStyleForSettings({ ...settings, theme: 'dark', themeColor: '#aabbcc' })).toMatchObject({ '--blue': '#aabbcc', '--border': 'rgba(170, 187, 204, 0.080)' })
  })

  it('does not add Gaussian material overrides to the default theme', () => {
    const style = shellStyleForSettings(defaultSettings) as Record<string, string>
    for (const name of ['--zeno-card-filter', '--zeno-overlay-shadow', '--zeno-modal-filter', '--zeno-modal-backdrop-filter']) {
      expect(style).not.toHaveProperty(name)
    }
  })

  it('uses one balanced unblurred overlay surface for the default appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'light', backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--usage-track-bg']).toBe('rgba(148, 163, 184, 0.12)')
    expect(style['--zeno-overlay-surface']).toBe('rgba(255, 255, 255, 0.800)')
    expect(style['--zeno-overlay-filter']).toBe('none')
  })

  it('uses the same balanced blurred overlay surface for the Gaussian appearance', () => {
    const style = shellStyleForSettings({ ...defaultSettings, theme: 'dark', appearancePreset: 'gaussian_blur', cardOpacity: 0.5, cardBlur: 15, backgroundUrl: '/wallpaper.webp', desktopBackgroundUrl: '/wallpaper.webp' }) as unknown as Record<string, string>
    expect(style['--usage-track-bg']).toBe('rgba(226, 232, 240, 0.17)')
    expect(style['--zeno-overlay-surface']).toBe('rgba(15, 23, 42, 0.640)')
    expect(style['--zeno-overlay-filter']).toBe('blur(15px) saturate(1.08)')
  })
})
