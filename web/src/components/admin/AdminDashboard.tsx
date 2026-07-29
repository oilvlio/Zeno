import { type ComponentType, type CSSProperties, type FormEvent, useEffect, useState } from 'react'
import type { AdminAlertRuleUpdateInput, AdminNodeCreateInput, AdminNodeUpdateInput, AdminNotificationChannelCreateInput, AdminNotificationChannelUpdateInput, AdminProbeTargetInput, AdminProbeTargetUpdateInput, AdminSettingsUpdateInput } from '../../api/adminClient'
import type { AdminAccountData } from '../../api/adminSession'
import { DashboardHeader } from '../DashboardHeader'
import { AdminSegmentedField } from './AdminFields'
import AdminOperationalWorkspace, { type AdminOperationalWorkspaceProps } from './AdminOperationalWorkspace'
import { AdminModuleErrorBoundary, AdminOperationalWorkspaceLoadError } from './AdminDashboardBoundary'
import { AdminFormSection, AdminModalActions } from './AdminPrimitives'
import { appearancePresetOptions, appearancePresets, appearanceValuesForSettings, defaultSettings, themeOptions, type AppearanceValues } from '../../lib/appearance'
import { validateAdminSettingsInput } from '../../lib/adminSettings'
import type { AdminNode, AdminNodeInstallCommand, AdminSettings, AdminTheme, AppearancePreset } from '../../types'
import type { AdminAuthState, AdminLoadState } from '../../lib/adminModel'
import { useAdminController } from '../../hooks/useAdminController'
import '../../styles/admin.css'

export type AdminSection = 'nodes' | 'targets' | 'notifications' | 'account' | 'settings'
type MaybePromise<T = void> = T | Promise<T>

export interface AdminDashboardProps {
  onHome: () => void
  settings?: AdminSettings
  chromeSettings?: AdminSettings
  hasAdminToken?: boolean
  authState?: AdminAuthState
  adminState?: AdminLoadState
  showAdminLoading?: boolean
  initialSection?: AdminSection
  onAdminLogin?: (username: string, password: string) => void
  onAdminTokenClear?: () => void
  onAdminAccountUpdate?: (username: string, currentPassword: string, newPassword: string) => Promise<void>
  onAdminNodeCreate?: (input: AdminNodeCreateInput) => Promise<AdminNode | void>
  onAdminNodeUpdate?: (nodeId: string, input: AdminNodeUpdateInput) => MaybePromise
  onAdminNodeDelete?: (nodeId: string) => MaybePromise
  onAdminInstallCommand?: (nodeId: string) => Promise<AdminNodeInstallCommand>
  onAdminProbeTargetCreate?: (input: AdminProbeTargetInput) => MaybePromise
  onAdminProbeTargetUpdate?: (targetId: string, input: AdminProbeTargetUpdateInput) => MaybePromise
  onAdminProbeTargetDelete?: (targetId: string) => MaybePromise
  onAdminNotificationChannelCreate?: (input: AdminNotificationChannelCreateInput) => MaybePromise
  onAdminNotificationChannelUpdate?: (channelId: string, input: AdminNotificationChannelUpdateInput) => MaybePromise
  onAdminNotificationChannelDelete?: (channelId: string) => MaybePromise
  onAdminNotificationChannelTest?: (channelId: string) => void
  onAdminAlertRuleUpdate?: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise
  onAdminSettingsUpdate?: (input: AdminSettingsUpdateInput) => MaybePromise
  onThemeChange?: (theme: AdminTheme) => void
  onBackgroundToggle?: () => void
  backgroundEnabled?: boolean
  operationalWorkspace?: ComponentType<AdminOperationalWorkspaceProps>
}

export interface AdminDashboardContainerProps {
  onHome: () => void
  settings?: AdminSettings
  chromeSettings?: AdminSettings
  onAdminTokenChange?: (token: string) => void
  onSettingsChange?: (settings: AdminSettings) => void
  onThemeChange?: (theme: AdminTheme) => void
  onBackgroundToggle?: () => void
  backgroundEnabled?: boolean
}

export function AdminDashboardContainer({
  onHome,
  settings = defaultSettings,
  chromeSettings = settings,
  onAdminTokenChange,
  onSettingsChange,
  onThemeChange,
  onBackgroundToggle,
  backgroundEnabled = true,
}: AdminDashboardContainerProps) {
  const controller = useAdminController(true, {
    initialSettings: settings,
    onTokenChange: onAdminTokenChange,
    onSettingsChange,
  })
  return (
    <AdminDashboard
      onHome={onHome}
      settings={controller.settings}
      chromeSettings={chromeSettings}
      hasAdminToken={controller.adminToken !== ''}
      authState={controller.adminAuthState}
      adminState={controller.adminState}
      showAdminLoading={controller.showAdminLoading}
      onAdminLogin={controller.submitAdminLogin}
      onAdminTokenClear={controller.clearAdminToken}
      onAdminAccountUpdate={controller.updateAdminAccountDetails}
      onAdminNodeCreate={controller.createAdminNodeDetails}
      onAdminNodeUpdate={controller.updateAdminNodeDetails}
      onAdminNodeDelete={controller.deleteAdminNodeDetails}
      onAdminInstallCommand={controller.requestAdminInstallCommand}
      onAdminProbeTargetCreate={controller.createAdminProbeTargetDetails}
      onAdminProbeTargetUpdate={controller.updateAdminProbeTargetDetails}
      onAdminProbeTargetDelete={controller.deleteAdminProbeTargetDetails}
      onAdminNotificationChannelCreate={controller.createAdminNotificationChannelDetails}
      onAdminNotificationChannelUpdate={controller.updateAdminNotificationChannelDetails}
      onAdminNotificationChannelDelete={controller.deleteAdminNotificationChannelDetails}
      onAdminNotificationChannelTest={controller.testAdminNotificationChannelDetails}
      onAdminAlertRuleUpdate={controller.updateAdminAlertRuleDetails}
      onAdminSettingsUpdate={controller.updateAdminSettingsDetails}
      onThemeChange={onThemeChange}
      onBackgroundToggle={onBackgroundToggle}
      backgroundEnabled={backgroundEnabled}
    />
  )
}

export function AdminDashboard({
  onHome,
  settings = defaultSettings,
  chromeSettings = settings,
  hasAdminToken = false,
  authState = { kind: 'idle' },
  adminState = { kind: 'idle' },
  showAdminLoading = false,
  initialSection = 'nodes',
  onAdminLogin = () => {},
  onAdminTokenClear = () => {},
  onAdminAccountUpdate = () => Promise.reject(new Error('account update unavailable')),
  onAdminNodeCreate = () => Promise.resolve(),
  onAdminNodeUpdate = () => {},
  onAdminNodeDelete = () => {},
  onAdminInstallCommand = () => Promise.reject(new Error('install command unavailable')),
  onAdminProbeTargetCreate = () => {},
  onAdminProbeTargetUpdate = () => {},
  onAdminProbeTargetDelete = () => {},
  onAdminNotificationChannelCreate = () => {},
  onAdminNotificationChannelUpdate = () => {},
  onAdminNotificationChannelDelete = () => {},
  onAdminNotificationChannelTest = () => {},
  onAdminAlertRuleUpdate = () => {},
  onAdminSettingsUpdate = () => {},
  onThemeChange,
  onBackgroundToggle,
  backgroundEnabled = true,
  operationalWorkspace,
}: AdminDashboardProps) {
  const [activeSection, setActiveSection] = useState<AdminSection>(initialSection)
  const OperationalWorkspace = operationalWorkspace ?? AdminOperationalWorkspace
  const handleTokenSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = event.currentTarget
    const formData = new FormData(form)
    const username = String(formData.get('admin-username') ?? '').trim()
    const password = String(formData.get('admin-password') ?? '').trim()
    if (username === '' || password === '') return
    onAdminLogin(username, password)
  }

  return (
    <div className="kulin-container admin-container">
      <section className={`home-top-card admin-panel${hasAdminToken ? '' : ' admin-panel--login'}`} aria-label="admin dashboard">
        <DashboardHeader
          settings={chromeSettings}
          onHome={onHome}
          onAdmin={onHome}
          adminLabel="前台"
          trailingAction={hasAdminToken ? <button className="nav-logout-button" type="button" onClick={onAdminTokenClear}>退出</button> : undefined}
          onThemeChange={onThemeChange}
          onBackgroundToggle={onBackgroundToggle}
          backgroundEnabled={backgroundEnabled}
        />

        {!hasAdminToken && (
          <form className="admin-login-card" aria-label="admin login form" onSubmit={handleTokenSubmit}>
              <div className="admin-login-title">
                <strong>后台登录</strong>
              </div>
              <label>
                <span>账号</span>
                <input name="admin-username" autoComplete="username" placeholder="admin" aria-label="后台账号" />
              </label>
              <label>
                <span>密码</span>
                <input name="admin-password" type="password" autoComplete="current-password" placeholder="admin" aria-label="后台密码" />
              </label>
              <button type="submit" disabled={authState.kind === 'loading'}>{authState.kind === 'loading' ? '登录中…' : '登录后台'}</button>
              {authState.kind === 'error' && <p className="admin-login-error">{authState.message}</p>}
          </form>
        )}

        {hasAdminToken && (
          <>
            <div className="admin-toolbar">
              <AdminSectionNav
                activeSection={activeSection}
                onSectionChange={setActiveSection}
              />
            </div>

            {adminState.kind === 'loading' && showAdminLoading && <div className="admin-state-card">加载中…</div>}
            {authState.kind === 'error' && <div className="admin-state-card is-error">{authState.message}</div>}
            {adminState.kind === 'error' && <div className="admin-state-card is-error">Admin API 读取失败：{adminState.message}</div>}

            {adminState.kind === 'ready' && (activeSection === 'nodes' || activeSection === 'targets' || activeSection === 'notifications') && (
              <AdminModuleErrorBoundary key={activeSection} fallback={<AdminOperationalWorkspaceLoadError />}>
                <OperationalWorkspace
                  activeSection={activeSection}
                  nodes={adminState.nodes}
                  targets={adminState.targets}
                  notificationChannels={adminState.notificationChannels}
                  alertRules={adminState.alertRules}
                  onNodeCreate={onAdminNodeCreate}
                  onNodeUpdate={onAdminNodeUpdate}
                  onNodeDelete={onAdminNodeDelete}
                  onInstallCommand={onAdminInstallCommand}
                  onProbeTargetCreate={onAdminProbeTargetCreate}
                  onProbeTargetUpdate={onAdminProbeTargetUpdate}
                  onProbeTargetDelete={onAdminProbeTargetDelete}
                  onNotificationChannelCreate={onAdminNotificationChannelCreate}
                  onNotificationChannelUpdate={onAdminNotificationChannelUpdate}
                  onNotificationChannelDelete={onAdminNotificationChannelDelete}
                  onNotificationChannelTest={onAdminNotificationChannelTest}
                  onAlertRuleUpdate={onAdminAlertRuleUpdate}
                />
              </AdminModuleErrorBoundary>
            )}

            {adminState.kind === 'ready' && activeSection === 'account' && (
              <AdminAccountSection account={adminState.account} onUpdate={onAdminAccountUpdate} />
            )}

            {adminState.kind === 'ready' && activeSection === 'settings' && (
              <AdminSettingsSection settings={settings} onUpdate={onAdminSettingsUpdate} />
            )}


          </>
        )}
      </section>
    </div>
  )
}

function AdminSectionNav({ activeSection, onSectionChange }: { activeSection: AdminSection; onSectionChange: (section: AdminSection) => void }) {
  const sections: Array<{ id: AdminSection; label: string }> = [
    { id: 'nodes', label: '服务器' },
    { id: 'targets', label: '延迟监控' },
    { id: 'notifications', label: '通知' },
    { id: 'account', label: '账户' },
    { id: 'settings', label: '设置' },
  ]

  return (
    <nav className="admin-section-nav" aria-label="后台导航">
      {sections.map((section) => (
        <button
          key={section.id}
          type="button"
          data-active={activeSection === section.id}
          aria-current={activeSection === section.id ? 'page' : undefined}
          onClick={() => onSectionChange(section.id)}
        >
          <span>{section.label}</span>
        </button>
      ))}
    </nav>
  )
}

function AdminAccountSection({ account, onUpdate }: { account: AdminAccountData; onUpdate: (username: string, currentPassword: string, newPassword: string) => Promise<void> }) {
  const [message, setMessage] = useState<{ kind: 'error' | 'success'; text: string } | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    const username = String(formData.get('account-username') ?? '').trim()
    const currentPassword = String(formData.get('current-password') ?? '').trim()
    const newPassword = String(formData.get('new-password') ?? '').trim()
    const confirmPassword = String(formData.get('confirm-password') ?? '').trim()
    if (!validAdminAccountUsername(username)) {
      setMessage({ kind: 'error', text: '账号只能使用 3-64 位字母、数字、点、短横线或下划线。' })
      return
    }
    if (currentPassword === '') {
      setMessage({ kind: 'error', text: '请输入当前密码确认修改。' })
      return
    }
    if (newPassword !== '' && newPassword.length < 8) {
      setMessage({ kind: 'error', text: '新密码至少 8 位；不改密码可留空。' })
      return
    }
    if (newPassword !== confirmPassword) {
      setMessage({ kind: 'error', text: '两次输入的新密码不一致。' })
      return
    }
    setSubmitting(true)
    setMessage(null)
    onUpdate(username, currentPassword, newPassword)
      .then(() => setMessage({ kind: 'success', text: '账户已更新。' }))
      .catch((error: unknown) => setMessage({ kind: 'error', text: error instanceof Error ? error.message : '账户更新失败。' }))
      .finally(() => setSubmitting(false))
  }

  return (
    <section className="admin-account-section admin-workspace-panel" aria-label="账户设置">
      <header className="admin-section-heading">
        <div>
          <h3>账户</h3>
        </div>
      </header>
      <form className="admin-account-form admin-node-edit-form is-sectioned" aria-label="修改账号和密码" onSubmit={handleSubmit}>
        <AdminFormSection title="登录信息">
          <div className="admin-form-grid">
            <label>
              <span>账号</span>
              <input name="account-username" autoComplete="username" defaultValue={account.username} />
            </label>
            <label>
              <span>当前密码</span>
              <input name="current-password" type="password" autoComplete="current-password" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="修改密码">
          <div className="admin-form-grid">
            <label>
              <span>新密码</span>
              <input name="new-password" type="password" autoComplete="new-password" placeholder="留空则不修改" />
            </label>
            <label>
              <span>确认新密码</span>
              <input name="confirm-password" type="password" autoComplete="new-password" placeholder="留空则不修改" />
            </label>
          </div>
        </AdminFormSection>
        <AdminModalActions>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存账户'}</button>
        </AdminModalActions>
        {message && <p className={`admin-install-error${message.kind === 'success' ? ' is-success' : ''}`}>{message.text}</p>}
      </form>
    </section>
  )
}

function validAdminAccountUsername(username: string): boolean {
  return /^[A-Za-z0-9._-]{3,64}$/.test(username.trim())
}

function AdminSettingsSection({ settings, onUpdate }: { settings: AdminSettings; onUpdate: (input: AdminSettingsUpdateInput) => MaybePromise }) {
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [appearance, setAppearance] = useState<AppearanceValues>(() => appearanceValuesForSettings(settings))
  useEffect(() => {
    setAppearance(appearanceValuesForSettings(settings))
  }, [settings.appearancePreset, settings.cardOpacity, settings.cardBlur, settings.cardRadius, settings.borderStrength, settings.shadowStrength, settings.backgroundOverlay, settings.themeColor])
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
      siteSubtitle: String(formData.get('site-subtitle') ?? '').trim(),
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
      <header className="admin-section-heading">
        <div>
          <h3>站点设置</h3>
        </div>
      </header>
      <form className="admin-settings-form admin-node-edit-form is-sectioned" aria-label="外观配置" onSubmit={handleSubmit}>
        <AdminFormSection title="站点信息">
          <div className="admin-form-grid">
            <label>
              <span>站点标题</span>
              <input name="site-title" autoComplete="off" defaultValue={settings.siteTitle} />
            </label>
            <label>
              <span>站点副标题</span>
              <input name="site-subtitle" autoComplete="off" defaultValue={settings.siteSubtitle} />
            </label>
            <label>
              <span>头像 / Logo URL</span>
              <input name="logo-url" autoComplete="off" defaultValue={settings.logoUrl} placeholder="可留空" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="主题与背景">
          <div className="admin-form-grid">
            <AdminSegmentedField name="theme" label="主题" defaultValue={settings.theme} options={themeOptions} />
            <label>
              <span>电脑端背景图 URL</span>
              <input name="desktop-background-url" autoComplete="off" defaultValue={settings.desktopBackgroundUrl || settings.backgroundUrl} placeholder="可留空" />
            </label>
            <label>
              <span>手机端背景图 URL</span>
              <input name="mobile-background-url" autoComplete="off" defaultValue={settings.mobileBackgroundUrl} placeholder="可留空，默认跟随电脑端" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="外观样式">
          <div className="admin-appearance-layout">
            <div className="admin-appearance-main">
              <div className="admin-appearance-top">
                <AdminAppearancePresetCards value={appearance.appearancePreset} onChange={updateAppearancePreset} />
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
            <AdminAppearancePreview appearance={appearance} />
          </div>
        </AdminFormSection>
        <AdminFormSection title="Agent 接入">
          <div className="admin-form-grid">
            <label>
              <span>Agent 接入 URL</span>
              <input name="agent-controller-url" autoComplete="off" defaultValue={settings.agentControllerUrl} placeholder="留空则使用当前后台访问地址" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="自定义 CSS">
          <div className="admin-form-grid">
            <label className="admin-form-span-2">
              <span>自定义 CSS</span>
              <textarea className="admin-code-field" name="custom-code" defaultValue={settings.customCode} spellCheck={false} placeholder={'<style>\n.home-top-card { border-color: #2563eb; }\n</style>'} />
            </label>
          </div>
        </AdminFormSection>
        {settingsError && <p className="admin-install-error">{settingsError}</p>}
        <AdminModalActions>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存设置'}</button>
        </AdminModalActions>
      </form>
    </section>
  )
}

function AdminAppearancePresetCards({ value, onChange }: { value: AppearancePreset; onChange: (value: string) => void }) {
  return (
    <div className="admin-appearance-presets" role="radiogroup" aria-label="外观模板">
      <input type="hidden" name="appearance-preset" value={value} />
      {appearancePresetOptions.map((option) => {
        const preset = appearancePresets[option.value]
        const active = value === option.value
        return (
          <button key={option.value} type="button" role="radio" aria-checked={active} data-active={active} onClick={() => onChange(option.value)}>
            <span>{option.label}</span>
            <small>{Math.round(preset.cardOpacity * 100)}% · {preset.cardBlur}px</small>
          </button>
        )
      })}
    </div>
  )
}

function AdminAppearancePreview({ appearance }: { appearance: AppearanceValues }) {
  const previewStyle = {
    '--appearance-preview-color': appearance.themeColor,
    '--appearance-preview-bg': `rgba(var(--appearance-preview-surface-rgb), ${appearance.cardOpacity.toFixed(3)})`,
    '--appearance-preview-radius': `${Math.max(10, appearance.cardRadius - 4)}px`,
    '--appearance-preview-blur': `${appearance.cardBlur}px`,
    '--appearance-preview-shadow': `0 12px 28px -22px rgba(var(--appearance-preview-shadow-rgb), ${(0.08 + appearance.shadowStrength * 0.28).toFixed(3)})`,
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
      <span className="admin-style-range__head">
        <span>{label}</span>
        <strong>{formatValue(value)}</strong>
      </span>
      <input name={name} type="range" min={min} max={max} step={step} value={value} onChange={(event) => onChange(Number.parseFloat(event.currentTarget.value))} />
    </label>
  )
}

function parseSettingsNumber(formData: FormData, name: string, fallback: number): number {
  const parsed = Number.parseFloat(String(formData.get(name) ?? ''))
  return Number.isFinite(parsed) ? parsed : fallback
}


export default AdminDashboard
