import { type CSSProperties, type FormEvent, useEffect, useState } from 'react'
import type { AdminSettingsUpdateInput } from '../../api/adminClient'
import { appearancePresetOptions, appearancePresets, appearanceValuesForSettings, resolvedTheme, shellStyleForSettings, themeOptions, type AppearanceValues } from '../../lib/appearance'
import { validateAdminSettingsInput } from '../../lib/adminSettings'
import type { AdminSettings, AppearancePreset } from '../../types'
import { SlidingSelector } from '../SlidingSelector'
import { AdminSegmentedField } from './AdminFields'
import { AdminFormSection, AdminActionFooter, AdminWorkspaceHeading } from './AdminPrimitives'

export interface AdminSettingsSectionProps {
  settings: AdminSettings
  onUpdate: (input: AdminSettingsUpdateInput) => void | Promise<void>
}

export default function AdminSettingsSection({ settings, onUpdate }: AdminSettingsSectionProps) {
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [appearance, setAppearance] = useState<AppearanceValues>(() => appearanceValuesForSettings(settings))
  const [previewTheme, setPreviewTheme] = useState<AdminSettings['theme']>(settings.theme)
  useEffect(() => {
    setAppearance(appearanceValuesForSettings(settings))
    setPreviewTheme(settings.theme)
  }, [settings.theme, settings.appearancePreset, settings.cardOpacity, settings.cardBlur, settings.cardRadius, settings.borderStrength, settings.shadowStrength, settings.backgroundOverlay, settings.themeColor])
  const updateAppearance = (patch: Partial<AppearanceValues>) => setAppearance((current) => ({ ...current, ...patch }))
  const updateAppearancePreset = (value: string) => {
    const preset = value === 'gaussian_blur' ? 'gaussian_blur' : 'default'
    setAppearance(appearancePresets[preset])
  }
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    const theme = String(formData.get('theme') ?? 'system') as AdminSettings['theme']
    const input: AdminSettingsUpdateInput = {
      siteTitle: String(formData.get('site-title') ?? '').trim(),
      logoUrl: String(formData.get('logo-url') ?? '').trim(),
      theme,
      agentControllerUrl: String(formData.get('agent-controller-url') ?? '').trim(),
      backgroundUrl: String(formData.get('desktop-background-url') ?? '').trim(),
      desktopBackgroundUrl: String(formData.get('desktop-background-url') ?? '').trim(),
      mobileBackgroundUrl: String(formData.get('mobile-background-url') ?? '').trim(),
      appearancePreset: String(formData.get('appearance-preset') ?? appearance.appearancePreset) as AppearancePreset,
      cardOpacity: parseSettingsNumber(formData, 'card-opacity', appearance.cardOpacity),
      cardBlur: parseSettingsNumber(formData, 'card-blur', appearance.cardBlur),
      cardRadius: parseSettingsNumber(formData, 'card-radius', appearance.cardRadius),
      borderStrength: parseSettingsNumber(formData, 'border-strength', appearance.borderStrength),
      shadowStrength: parseSettingsNumber(formData, 'shadow-strength', appearance.shadowStrength),
      backgroundOverlay: parseSettingsNumber(formData, 'background-overlay', appearance.backgroundOverlay),
      themeColor: String(formData.get('theme-color') ?? appearance.themeColor).trim(),
      customCode: String(formData.get('custom-code') ?? '').trim(),
    }
    const validationError = validateAdminSettingsInput(input)
    if (validationError) {
      setSettingsError(validationError)
      return
    }
    setSettingsError(null)
    setSubmitting(true)
    Promise.resolve(onUpdate(input))
      .catch((error: unknown) => setSettingsError(error instanceof Error ? error.message : '设置保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <section className="admin-settings-section admin-workspace-panel" aria-label="admin settings">
      <AdminWorkspaceHeading title="站点设置" />
      <form className="admin-settings-form admin-node-edit-form is-sectioned admin-workspace-form" aria-label="外观配置" onSubmit={handleSubmit}>
        <div className="admin-workspace-card admin-settings-card">
          <section className="admin-settings-card-section" aria-label="站点设置">
            <div className="admin-form-grid">
              <label><span>站点标题</span><input name="site-title" autoComplete="off" defaultValue={settings.siteTitle} /></label>
              <label><span>头像 / Logo URL</span><input name="logo-url" autoComplete="off" defaultValue={settings.logoUrl} placeholder="可留空" /></label>
              <label className="admin-form-span-2"><span>Agent 接入 URL</span><input name="agent-controller-url" autoComplete="off" defaultValue={settings.agentControllerUrl} placeholder="留空则使用当前后台访问地址" /></label>
              <label className="admin-form-span-2">
                <span>自定义 CSS</span>
                <textarea className="admin-code-field" name="custom-code" defaultValue={settings.customCode} spellCheck={false} placeholder={'<style>\n.home-top-card { border-color: #2563eb; }\n</style>'} />
              </label>
            </div>
          </section>
          <AdminFormSection className="admin-settings-card-section admin-settings-appearance-card" title="界面外观">
            <div className="admin-form-grid">
              <AdminSegmentedField name="theme" label="主题" value={previewTheme} options={themeOptions} onChange={(value) => setPreviewTheme(value === 'light' || value === 'dark' ? value : 'system')} />
              <label><span>电脑端背景图 URL</span><input name="desktop-background-url" autoComplete="off" defaultValue={settings.desktopBackgroundUrl || settings.backgroundUrl} placeholder="可留空" /></label>
              <label><span>手机端背景图 URL</span><input name="mobile-background-url" autoComplete="off" defaultValue={settings.mobileBackgroundUrl} placeholder="可留空，默认跟随电脑端" /></label>
            </div>
            <div className="admin-appearance-layout">
              <div className="admin-appearance-main">
                <div className="admin-appearance-top">
                  <AdminAppearancePresetSlider value={appearance.appearancePreset} onChange={updateAppearancePreset} />
                  <label className="admin-color-field">
                    <span>主题色</span>
                    <span className="admin-color-field__row">
                      <input name="theme-color" type="color" value={appearance.themeColor} onChange={(event) => updateAppearance({ themeColor: event.currentTarget.value })} />
                      <strong>{appearance.themeColor.toUpperCase()}</strong>
                    </span>
                  </label>
                </div>
                <div className="admin-style-grid">
                  <AdminStyleRangeField name="card-opacity" label="卡片透明度" value={appearance.cardOpacity} min={0.2} max={1} step={0.01} onChange={(value) => updateAppearance({ cardOpacity: value })} formatValue={(value) => `${Math.round(value * 100)}%`} />
                  <AdminStyleRangeField name="card-blur" label="卡片模糊度" value={appearance.cardBlur} min={0} max={40} step={1} onChange={(value) => updateAppearance({ cardBlur: value })} formatValue={(value) => `${Math.round(value)}px`} />
                  <AdminStyleRangeField name="card-radius" label="卡片圆角" value={appearance.cardRadius} min={8} max={36} step={1} onChange={(value) => updateAppearance({ cardRadius: value })} formatValue={(value) => `${Math.round(value)}px`} />
                  <AdminStyleRangeField name="border-strength" label="边框强度" value={appearance.borderStrength} min={0} max={1} step={0.01} onChange={(value) => updateAppearance({ borderStrength: value })} formatValue={(value) => `${Math.round(value * 100)}%`} />
                  <AdminStyleRangeField name="shadow-strength" label="阴影强度" value={appearance.shadowStrength} min={0} max={1} step={0.01} onChange={(value) => updateAppearance({ shadowStrength: value })} formatValue={(value) => `${Math.round(value * 100)}%`} />
                  <AdminStyleRangeField name="background-overlay" label="背景遮罩" value={appearance.backgroundOverlay} min={0} max={0.8} step={0.01} onChange={(value) => updateAppearance({ backgroundOverlay: value })} formatValue={(value) => `${Math.round(value * 100)}%`} />
                </div>
              </div>
              <AdminAppearancePreview appearance={appearance} settings={settings} theme={previewTheme} />
            </div>
            {settingsError && <p className="admin-install-error">{settingsError}</p>}
            <AdminActionFooter><button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存设置'}</button></AdminActionFooter>
          </AdminFormSection>
        </div>
      </form>
    </section>
  )
}

function AdminAppearancePresetSlider({ value, onChange }: { value: AppearancePreset; onChange: (value: string) => void }) {
  return (
    <div className="admin-appearance-preset-field">
      <span>主题样式</span>
      <input type="hidden" name="appearance-preset" value={value} />
      <SlidingSelector
        ariaLabel="外观模板"
        role="group"
        className="admin-appearance-preset-slider sliding-selector--large"
        options={appearancePresetOptions.map((option) => {
          return {
            value: option.value,
            content: (
              <span className="admin-appearance-preset-option">
                <strong>{option.label}</strong>
              </span>
            ),
          }
        })}
        value={value}
        onChange={onChange}
      />
    </div>
  )
}

function AdminAppearancePreview({ appearance, settings, theme }: { appearance: AppearanceValues; settings: AdminSettings; theme: AdminSettings['theme'] }) {
  const previewTheme = resolvedTheme(theme)
  const previewShellStyle = shellStyleForSettings({
    ...settings,
    ...appearance,
    theme,
    backgroundUrl: 'preview',
    desktopBackgroundUrl: 'preview',
    mobileBackgroundUrl: '',
  }) as Record<string, string | number> | undefined
  const previewStyle = {
    '--appearance-preview-color': String(previewShellStyle?.['--blue'] ?? appearance.themeColor),
    '--appearance-preview-radius': String(previewShellStyle?.['--radius-card'] ?? `${Math.max(10, appearance.cardRadius - 4)}px`),
    '--appearance-preview-surface': String(previewShellStyle?.['--page-surface'] ?? 'rgba(255, 255, 255, 0.82)'),
    '--appearance-preview-border': String(previewShellStyle?.['--border'] ?? appearance.themeColor),
    '--appearance-preview-shadow': String(previewShellStyle?.['--zeno-card-shadow'] ?? 'none'),
    '--appearance-preview-filter': appearance.cardBlur > 0 ? `blur(${appearance.cardBlur}px) saturate(1.06)` : 'none',
    '--appearance-preview-overlay': String(previewShellStyle?.['--zeno-background-overlay-color'] ?? 'transparent'),
    '--appearance-preview-foreground': String(previewShellStyle?.['--foreground'] ?? 'var(--foreground)'),
    '--appearance-preview-muted': String(previewShellStyle?.['--muted'] ?? 'var(--muted)'),
    '--appearance-preview-canvas': previewTheme === 'dark' ? '#0f172a' : '#f8fafc',
    '--appearance-preview-canvas-soft': previewTheme === 'dark' ? '#475569' : '#cbd5e1',
  } as CSSProperties
  return (
    <div className="admin-appearance-preview" style={previewStyle} aria-hidden="true">
      <div className="admin-appearance-preview__card">
        <span />
        <strong>预览卡片</strong>
        <em>{Math.round(appearance.cardOpacity * 100)}% · {appearance.cardBlur}px · {Math.round(appearance.borderStrength * 100)}%</em>
      </div>
    </div>
  )
}

function AdminStyleRangeField({ name, label, value, min, max, step, onChange, formatValue }: { name: string; label: string; value: number; min: number; max: number; step: number; onChange: (value: number) => void; formatValue: (value: number) => string }) {
  return (
    <label className="admin-style-range">
      <span className="admin-style-range__head"><span>{label}</span><strong>{formatValue(value)}</strong></span>
      <input name={name} type="range" min={min} max={max} step={step} value={value} onChange={(event) => onChange(Number.parseFloat(event.currentTarget.value))} />
    </label>
  )
}

function parseSettingsNumber(formData: FormData, name: string, fallback: number): number {
  const parsed = Number.parseFloat(String(formData.get(name) ?? ''))
  return Number.isFinite(parsed) ? parsed : fallback
}
