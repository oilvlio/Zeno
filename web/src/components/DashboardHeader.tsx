import { type ReactNode, useEffect, useRef, useState } from 'react'
import { defaultSettings, fallbackLogoUrl, resolvedTheme, themeOptions } from '../lib/appearance'
import type { AdminSettings, AdminTheme } from '../types'

export interface DashboardHeaderProps {
  settings?: AdminSettings
  onHome: () => void
  onAdmin: () => void
  onAdminIntent?: () => void
  adminLabel?: string
  leadingAction?: ReactNode
  trailingAction?: ReactNode
  onThemeChange?: (theme: AdminTheme) => void
  onBackgroundToggle?: () => void
  backgroundEnabled?: boolean
}

function BrandLogo({ logoUrl, siteTitle }: { logoUrl?: string; siteTitle?: string }) {
  const source = (logoUrl ?? '').trim()
  const [currentSource, setCurrentSource] = useState(source)
  const [showLetterFallback, setShowLetterFallback] = useState(source === '')

  useEffect(() => {
    setCurrentSource(source)
    setShowLetterFallback(source === '')
  }, [source])

  if (showLetterFallback) {
    return <span className="brand-logo-fallback" role="img" aria-label={`${siteTitle || 'Zeno'} logo`}>Z</span>
  }

  return (
    <img
      src={currentSource}
      width="32"
      height="32"
      decoding="async"
      alt={`${siteTitle || 'Zeno'} logo`}
      onError={() => {
        if (currentSource !== defaultSettings.logoUrl) setCurrentSource(defaultSettings.logoUrl)
        else if (currentSource !== fallbackLogoUrl) setCurrentSource(fallbackLogoUrl)
        else setShowLetterFallback(true)
      }}
    />
  )
}

export function DashboardHeader({ settings = defaultSettings, onHome, onAdmin, onAdminIntent, adminLabel = '后台', leadingAction, trailingAction, onThemeChange, onBackgroundToggle, backgroundEnabled = false }: DashboardHeaderProps) {
  const [themeMenuOpen, setThemeMenuOpen] = useState(false)
  const themeMenuRef = useRef<HTMLDivElement>(null)
  const themeMode = settings.theme
  const currentTheme = resolvedTheme(themeMode)
  const currentThemeLabel = themeOptions.find((option) => option.value === themeMode)?.label ?? '跟随系统'
  const backgroundControlLabel = onBackgroundToggle
    ? (backgroundEnabled ? '关闭背景图' : '开启背景图')
    : (backgroundEnabled ? '背景图加载中' : '背景图未配置')

  useEffect(() => {
    if (!themeMenuOpen || typeof window === 'undefined') return undefined
    const handlePointerDown = (event: PointerEvent) => {
      if (themeMenuRef.current?.contains(event.target as Node)) return
      setThemeMenuOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setThemeMenuOpen(false)
    }
    window.addEventListener('pointerdown', handlePointerDown)
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown)
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [themeMenuOpen])

  const selectTheme = (nextTheme: AdminTheme) => {
    onThemeChange?.(nextTheme)
    setThemeMenuOpen(false)
  }

  return (
    <header className="kulin-nav">
      <button className="brand" type="button" onClick={onHome}>
        <span className="brand-logo"><BrandLogo logoUrl={settings.logoUrl} siteTitle={settings.siteTitle} /></span>
        <span>{settings.siteTitle || 'Zeno'}</span>
      </button>
      <nav className="nav-actions" aria-label="dashboard actions">
        {leadingAction}
        <div className="theme-menu" ref={themeMenuRef}>
          <button className="nav-icon-button" type="button" aria-label={`主题：${currentThemeLabel}`} aria-haspopup="menu" aria-expanded={themeMenuOpen} onClick={() => setThemeMenuOpen((open) => !open)}>{themeMode === 'system' ? <MonitorIcon /> : currentTheme === 'dark' ? <MoonIcon /> : <SunIcon />}<span className="sr-only">切换深浅色</span></button>
          {themeMenuOpen && (
            <div className="theme-menu-popover" role="menu">
              {themeOptions.map((option) => (
                <button key={option.value} type="button" role="menuitemradio" aria-checked={themeMode === option.value} data-active={themeMode === option.value} onClick={() => selectTheme(option.value)}>
                  <span>{option.label}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <button className={`nav-icon-button${backgroundEnabled ? ' is-solid' : ''}`} type="button" aria-label={backgroundControlLabel} aria-pressed={backgroundEnabled} disabled={!onBackgroundToggle} onClick={onBackgroundToggle}><ImageMinusIcon /><span className="sr-only">开关背景图</span></button>
        <button className="login-link" type="button" onPointerEnter={onAdminIntent} onPointerDown={onAdminIntent} onFocus={onAdminIntent} onClick={onAdmin}>{adminLabel}</button>
        {trailingAction}
      </nav>
    </header>
  )
}

function SunIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
  )
}

function MoonIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M20.99 12.58A8.5 8.5 0 1 1 11.42 3a6.6 6.6 0 0 0 9.57 9.57Z" />
    </svg>
  )
}

function MonitorIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <rect x="3" y="4" width="18" height="12" rx="2" />
      <path d="M8 20h8M12 16v4" />
    </svg>
  )
}

function ImageMinusIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M21 9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7" />
      <path d="M16 5h6" />
      <circle cx="9" cy="9" r="2" />
      <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
    </svg>
  )
}
