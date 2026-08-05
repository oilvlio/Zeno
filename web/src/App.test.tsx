import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { DashboardRouteState, HomeRegionFilter, HomeTopPanel, adminTokenMaxAgeMs, documentBrandingForSettings, filterHomeNodesByRegion, homeMonthlyCostForNodes, homeRegionOptions, homeTrafficTotalsForNodes, isAdminUnauthorizedError, orderHomeNodes, preloadAdminRoute, preloadNodeDetailRoute, shellStyleForSettings, shouldPreloadAdminRoute, shouldPreloadNodeDetailRoute, shouldRefreshHomeRealtimeSnapshot } from './App'
import type { HomeCardNode } from './types'
import { settings } from './components/admin/adminTestFixtures'

const overviewProps = {
  totalCount: 11,
  onlineCount: 9,
  offlineCount: 2,
  monthlyCost: 88.5,
  totalUp: 1024,
  totalDown: 2048,
  upSpeed: 128,
  downSpeed: 256,
}

const trafficNode: HomeCardNode = {
  id: 'traffic-node',
  displayName: 'Traffic Node',
  status: 'online',
  os: 'debian',
  cpuPercent: 1,
  memoryUsedBytes: 1,
  memoryTotalBytes: 2,
  diskUsedBytes: 1,
  diskTotalBytes: 2,
  netInSpeedBps: 1,
  netOutSpeedBps: 1,
  netInTotalBytes: 100,
  netOutTotalBytes: 200,
  netInLifetimeBytes: 1_000,
  netOutLifetimeBytes: 2_000,
  monthlyBillableBytes: 1,
  monthlyQuotaBytes: 2,
}

describe('homeTrafficTotalsForNodes', () => {
  it('uses controller-persisted lifetime traffic instead of reboot-scoped interface counters', () => {
    expect(homeTrafficTotalsForNodes([trafficNode, { ...trafficNode, id: 'second', netInLifetimeBytes: 3_000, netOutLifetimeBytes: 4_000 }])).toEqual({
      totalUp: 6_000,
      totalDown: 4_000,
    })
  })

  it('falls back to raw counters for a cached summary created before lifetime totals existed', () => {
    const legacyNode = { ...trafficNode, netInLifetimeBytes: undefined, netOutLifetimeBytes: undefined }
    expect(homeTrafficTotalsForNodes([legacyNode])).toEqual({ totalUp: 200, totalDown: 100 })
  })

  it('derives the selected-currency monthly total from original renewal amounts without double rounding', () => {
    const nodes = [
      { ...trafficNode, renewalAmount: 10, renewalCurrency: 'USD', billingCycle: '月', monthlyCostCny: 67.66 },
      { ...trafficNode, id: 'yearly', renewalAmount: 120, renewalCurrency: 'USD', billingCycle: '年', monthlyCostCny: 67.66 },
      { ...trafficNode, id: 'permanent', renewalAmount: 10, renewalCurrency: 'USD', billingCycle: '月', monthlyCostCny: null },
    ]
    const rates = { CNY: 1, USD: 6.766, KRW: 0.004602 }

    expect(homeMonthlyCostForNodes(nodes, 'USD', rates)).toBe(20)
    expect(homeMonthlyCostForNodes(nodes, 'KRW', rates)).toBeCloseTo(29_404.61, 2)
  })
})

describe('HomeTopPanel', () => {
  it('keeps local admin tokens for at most one day', () => {
    expect(adminTokenMaxAgeMs).toBe(24 * 60 * 60 * 1000)
  })

  it('preloads the complete admin surface before navigation', async () => {
    await expect(preloadAdminRoute()).resolves.toBeUndefined()
  })

  it('warms the complete admin surface only for a returning authenticated administrator', () => {
    expect(shouldPreloadAdminRoute('home', true, '__cookie_session__')).toBe(true)
    expect(shouldPreloadAdminRoute('home', true, '')).toBe(false)
    expect(shouldPreloadAdminRoute('home', false, '__cookie_session__')).toBe(false)
    expect(shouldPreloadAdminRoute('admin', true, '__cookie_session__')).toBe(false)
  })

  it('preloads the shared server detail surface after the home summary is ready', async () => {
    expect(shouldPreloadNodeDetailRoute('home', true)).toBe(true)
    expect(shouldPreloadNodeDetailRoute('home', false)).toBe(true)
    expect(shouldPreloadNodeDetailRoute('node', true)).toBe(false)
    await expect(preloadNodeDetailRoute()).resolves.toBeUndefined()
  })

  it('keeps route loading content inside the same top-card shell used by destination pages', () => {
    const html = renderToStaticMarkup(
      <DashboardRouteState
        settings={settings}
        message="加载中…"
        isAdmin
        onHome={() => {}}
        onAdmin={() => {}}
        onThemeChange={() => {}}
        backgroundEnabled
      />,
    )

    expect(html).toContain('class="kulin-container route-state-container"')
    expect(html).toContain('class="home-top-card route-state-panel is-admin"')
    expect(html).toContain('class="route-state-card">加载中…</div>')
    expect(html).toContain('>前台</button>')
    expect(html).not.toContain('class="state-panel"')
  })

  it('keeps a direct backend load on the stable shell without exposing loading copy', () => {
    const html = renderToStaticMarkup(
      <DashboardRouteState
        settings={settings}
        message=""
        isAdmin
        onHome={() => {}}
        onAdmin={() => {}}
        onThemeChange={() => {}}
        backgroundEnabled
      />,
    )
    expect(html).toContain('home-top-card route-state-panel is-admin')
    expect(html).not.toContain('route-state-card')
    expect(html).not.toContain('>加载中…<')
  })

  it('recognizes expired admin session API responses', () => {
    expect(isAdminUnauthorizedError(new Error('admin nodes request failed: 401'))).toBe(true)
    expect(isAdminUnauthorizedError(new Error('admin settings update failed: 401'))).toBe(true)
    expect(isAdminUnauthorizedError(new Error('admin logout failed: 401'))).toBe(true)
    expect(isAdminUnauthorizedError(new Error('admin nodes request failed: 500'))).toBe(false)
    expect(isAdminUnauthorizedError(new Error('missing admin token'))).toBe(false)
  })

  it('paces aggregate homepage realtime refreshes on live summary frames', () => {
    expect(shouldRefreshHomeRealtimeSnapshot(null, 100, 100)).toBe(true)
    expect(shouldRefreshHomeRealtimeSnapshot(100, 800, 100)).toBe(true)
    expect(shouldRefreshHomeRealtimeSnapshot(100, 1800, 100)).toBe(false)
    expect(shouldRefreshHomeRealtimeSnapshot(100, 1950, 100)).toBe(true)
    expect(shouldRefreshHomeRealtimeSnapshot(100, 2100, 100)).toBe(true)
  })

  it('keeps configured order inside online/offline groups but moves offline homepage cards last', () => {
    const nodes = [
      { id: 'offline-first', status: 'offline' },
      { id: 'online-middle', status: 'online' },
      { id: 'warning-last', status: 'warning' },
    ] as HomeCardNode[]

    expect(orderHomeNodes(nodes).map((node) => node.id)).toEqual(['online-middle', 'offline-first', 'warning-last'])
    expect(nodes.map((node) => node.id)).toEqual(['offline-first', 'online-middle', 'warning-last'])
  })

  it('builds country filters in server order and keeps 全部 as the unfiltered view', () => {
    const nodes = [
      { id: 'hk-a', countryCode: 'hk' },
      { id: 'unknown' },
      { id: 'jp', countryCode: 'JP' },
      { id: 'tw', countryCode: 'TW' },
      { id: 'cn', countryCode: 'cn' },
      { id: 'hk-b', countryCode: 'HK' },
      { id: 'invalid', countryCode: 'Hong Kong' },
    ] as HomeCardNode[]

    expect(homeRegionOptions(nodes)).toEqual(['HK', 'JP', 'CN'])
    expect(filterHomeNodesByRegion(nodes, 'ALL').map((node) => node.id)).toEqual(['hk-a', 'unknown', 'jp', 'tw', 'cn', 'hk-b', 'invalid'])
    expect(filterHomeNodesByRegion(nodes, 'HK').map((node) => node.id)).toEqual(['hk-a', 'hk-b'])
    expect(filterHomeNodesByRegion(nodes, 'CN').map((node) => node.id)).toEqual(['tw', 'cn'])

    const html = renderToStaticMarkup(<HomeRegionFilter regions={['HK', 'JP']} activeRegion="ALL" onChange={() => {}} />)
    expect(html).toContain('aria-label="服务器地区筛选"')
    expect(html).toContain('aria-pressed="true"><span class="region-all-text">全部</span></button>')
    expect(html).toContain('title="HK"')
    expect(html).toContain('title="JP"')
    expect(html).not.toContain('香港')
    expect(html).not.toContain('日本')
  })

  it('uses the configured logo as the browser favicon source', () => {
    expect(documentBrandingForSettings(settings)).toEqual({
      title: '水饺监控',
      iconHref: '/assets/logo/custom.png',
    })
  })

  it('turns configured desktop and mobile background images into safe shell variables', () => {
    expect(shellStyleForSettings(settings)).toEqual({
      '--zeno-desktop-background-image': 'url("https://example.com/desktop-bg.webp")',
      '--zeno-mobile-background-image': 'url("https://example.com/mobile-bg.webp")',
      '--blue': '#6366f1',
      '--foreground': '#f8fafc',
      '--muted': '#cbd5e1',
      '--border': 'rgba(99, 102, 241, 0.340)',
      '--metric-shadow': 'rgba(99, 102, 241, 0.075)',
      '--page-surface': 'rgba(15, 23, 42, 0.580)',
      '--admin-secondary-surface': 'rgb(15, 23, 42)',
      '--surface-strong': 'transparent',
      '--surface': 'transparent',
      '--surface-soft': 'transparent',
      '--secondary': 'transparent',
      '--metric-bg': 'transparent',
      '--field-bg': 'transparent',
      '--control-bg': 'transparent',
      '--usage-track-bg': 'rgba(148, 163, 184, 0.22)',
      '--usage-track-border': 'rgba(203, 213, 225, 0.24)',
      '--zeno-overlay-surface': 'rgba(15, 23, 42, 0.460)',
      '--zeno-menu-surface': 'rgba(15, 23, 42, 0.580)',
      '--zeno-overlay-filter': 'blur(18px) saturate(1.08)',
      '--radius-panel': '24px',
      '--radius-card': '20px',
      '--radius-field': '16px',
      '--zeno-card-blur': '18px',
      '--zeno-card-highlight': 'rgba(255, 255, 255, 0.081)',
      '--zeno-card-shadow': '0 10px 26px -24px rgba(0, 0, 0, 0.190), 0 1px 2px rgba(0, 0, 0, 0.037)',
      '--zeno-background-overlay-color': 'rgba(0, 0, 0, 0.080)',
      '--zeno-theme-rgb': '99, 102, 241',
      backgroundSize: 'cover',
      backgroundAttachment: 'fixed',
    })
    expect(shellStyleForSettings({ ...settings, backgroundUrl: '', desktopBackgroundUrl: '', mobileBackgroundUrl: '' })).toMatchObject({
      '--zeno-desktop-background-image': 'none',
      '--zeno-card-blur': '18px',
      '--page-surface': 'rgb(15, 23, 42)',
      '--admin-secondary-surface': 'rgb(15, 23, 42)',
      '--zeno-overlay-surface': 'rgba(15, 23, 42, 0.880)',
    })
    expect(shellStyleForSettings({ ...settings, mobileBackgroundUrl: '' })).toMatchObject({
      '--zeno-mobile-background-image': 'url("https://example.com/desktop-bg.webp")',
    })
    const defaultAppearanceSettings = {
      ...settings,
      theme: 'light' as const,
      appearancePreset: 'default' as const,
      cardOpacity: 0.82,
      cardBlur: 0,
      cardRadius: 20,
      borderStrength: 0.26,
      shadowStrength: 0.22,
      backgroundOverlay: 0,
      themeColor: '#2563eb',
    }
    const defaultWithBackgroundStyle = shellStyleForSettings(defaultAppearanceSettings)
    expect(defaultWithBackgroundStyle).toMatchObject({
      '--page-surface': 'rgba(255, 255, 255, 0.820)',
      '--admin-secondary-surface': 'rgb(255, 255, 255)',
      '--zeno-overlay-surface': 'rgba(255, 255, 255, 0.920)',
      '--surface-strong': 'transparent',
      '--surface': 'transparent',
    })
    expect(shellStyleForSettings({ ...defaultAppearanceSettings, theme: 'dark' })).toMatchObject({ '--admin-secondary-surface': 'rgb(15, 23, 42)' })
    expect(shellStyleForSettings({ ...defaultAppearanceSettings, cardOpacity: 0.8 })).toMatchObject({ '--admin-secondary-surface': 'rgb(255, 255, 255)' })
    expect(shellStyleForSettings({ ...defaultAppearanceSettings, backgroundUrl: '', desktopBackgroundUrl: '', mobileBackgroundUrl: '' })).toMatchObject({
      '--page-surface': 'rgb(255, 255, 255)',
      '--admin-secondary-surface': 'rgb(255, 255, 255)',
      '--zeno-overlay-surface': 'rgba(255, 255, 255, 0.960)',
    })
  })

  it('keeps the homepage top controls inside one compact six-column summary row', () => {
    const html = renderToStaticMarkup(
      <HomeTopPanel
        {...overviewProps}
        settings={settings}
        onHome={() => {}}
        onAdmin={() => {}}
      />,
    )

    expect(html).toContain('home-top-card home-overview-card')
    expect(html).toContain('dashboard actions')
    expect(html).toContain('后台')
    expect(html).not.toContain('aria-label="language"')
    expect(html).not.toContain('Zeno Overview')
    expect(html).toContain('水饺监控')
    expect(html).toContain('/assets/logo/custom.png')
    expect(html).not.toContain('/assets/avatar/custom.webp')
    expect(html).not.toContain('VPS 状态总览')
    expect(html).toContain('home-summary')
    expect(html.match(/home-summary__metric(?: |")/g)).toHaveLength(6)
    expect(html).toContain('home-summary__metric--status')
    expect(html).toContain('home-summary__metric--cost')
    expect(html).not.toContain('home-summary__status-track')
    expect(html).not.toContain('home-summary__snapshot')
    expect(html).not.toContain('home-summary__network-board')
    expect(html).not.toContain('home-summary__tile')
    expect(html).toContain('home-summary__status-dot')
    expect(html).toMatch(/home-summary__metric--status[\s\S]*?home-summary__metric--cost[\s\S]*?home-summary__metric--total home-summary__metric--upload[\s\S]*?home-summary__metric--total home-summary__metric--download[\s\S]*?home-summary__metric--rate home-summary__metric--upload[\s\S]*?home-summary__metric--rate home-summary__metric--download/)
    expect(html).toContain('在线节点')
    expect(html).toContain('月均消费')
    expect(html).toContain('¥ 88.50')
    expect(html).not.toContain('home-summary__status-body')
    expect(html).not.toContain('home-summary__status-cost')
    expect(html).toContain('class="home-currency-menu"')
    const currencyControl = html.indexOf('class="home-currency-menu"')
    const themeControl = html.indexOf('class="theme-menu"')
    const backgroundControl = html.indexOf('aria-pressed=')
    const adminControl = html.indexOf('class="login-link"')
    expect(currencyControl).toBeGreaterThan(-1)
    expect(currencyControl).toBeLessThan(themeControl)
    expect(themeControl).toBeLessThan(backgroundControl)
    expect(backgroundControl).toBeLessThan(adminControl)
    expect(html).toContain('aria-label="金额单位：人民币 CNY"')
    expect(html).toContain('aria-haspopup="listbox"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain('home-currency-select__flag')
    expect(html).not.toContain('class="fi fi-cn"')
    expect(html).toContain('home-currency-select__value')
    expect(html).toContain('>CNY</span>')
    expect(html).toMatch(/<button class="home-currency-select"[\s\S]*?<span class="home-currency-select__value">CNY<\/span><\/button>/)
    expect(html).not.toContain('<select')
    expect(html).not.toContain('<option')
    expect(html).not.toContain('home-summary__status-meta')
    expect(html).not.toContain('运行中')
    expect(html).not.toContain('全部在线')
    expect(html).toContain('home-summary__metric--upload')
    expect(html).toContain('home-summary__metric--download')
    expect(html).not.toContain('Zeno Overview')
    expect(html).not.toContain('服务器运行概览')
    expect(html).not.toContain('在线率')
    expect(html).not.toContain('11 台服务器')
    expect(html).toContain('累计发送')
    expect(html).toContain('累计接收')
    expect(html).toContain('上传')
    expect(html).toContain('下载')
    expect(html).not.toContain('上传速率')
    expect(html).not.toContain('下载速率')
    expect(html).toContain('home-summary__metric-icon')
    expect(html).toMatch(/home-summary__status-dot[^>]*><\/span><span>在线节点<\/span>/)
    expect(html).toMatch(/home-summary__metric-icon[\s\S]*?月均消费/)
    expect(html).toMatch(/aria-label="total sent"[\s\S]*?home-summary__metric-icon home-summary__metric-icon--total[\s\S]*?累计发送/)
    expect(html).toMatch(/aria-label="total received"[\s\S]*?home-summary__metric-icon home-summary__metric-icon--total[\s\S]*?累计接收/)
    expect(html).toMatch(/home-summary__metric-icon[\s\S]*?上传<\/span>/)
    expect(html).not.toContain('实时')
    expect(html).not.toContain('累计上传')
    expect(html).not.toContain('累计下载')
    expect(html).not.toContain('服务器总数')
    expect(html).not.toContain('在线服务器')
    expect(html).not.toContain('离线服务器')
    expect(html).not.toContain('overview-card--combined')
    expect(html).not.toContain('overview-metric')
    expect(html).not.toContain(['service', 'status', 'panel'].join('-'))
  })

  it('converts the top monthly cost and selector to the chosen currency', () => {
    const html = renderToStaticMarkup(
      <HomeTopPanel
        {...overviewProps}
        settings={settings}
        monthlyCost={11.0625}
        displayCurrency="USD"
        exchangeRates={{ CNY: 1, USD: 8, EUR: 9 }}
        onCurrencyChange={() => {}}
        onHome={() => {}}
        onAdmin={() => {}}
      />,
    )

    expect(html).toContain('aria-label="金额单位：美元 USD"')
    expect(html).not.toContain('class="fi fi-us"')
    expect(html).toContain('>USD</span>')
    expect(html).not.toContain('<select')
    expect(html).toContain('$ 11.06')
    expect(html).not.toContain('¥ 88.50')
  })

  it('renders a circular Z brand avatar when the logo URL is blank', () => {
    const html = renderToStaticMarkup(
      <HomeTopPanel
        {...overviewProps}
        settings={{ ...settings, logoUrl: '' }}
        onHome={() => {}}
        onAdmin={() => {}}
      />,
    )

    expect(html).toContain('class="brand-logo-fallback"')
    expect(html).toContain('role="img"')
    expect(html).toContain('aria-label="水饺监控 logo"')
    expect(html).toContain('>Z</span>')
  })

  it('always renders the background control and enables it only when it is ready', () => {
    const waiting = renderToStaticMarkup(<HomeTopPanel {...overviewProps} settings={settings} onHome={() => {}} onAdmin={() => {}} />)
    expect(waiting).toContain('aria-label="背景图未配置"')
    expect(waiting).toContain('aria-pressed="false"')
    expect(waiting).toContain('disabled=""')
    expect(waiting).not.toContain('nav-icon-button-placeholder')

    const loading = renderToStaticMarkup(
      <HomeTopPanel {...overviewProps} settings={settings} onHome={() => {}} onAdmin={() => {}} backgroundEnabled />,
    )
    expect(loading).toContain('aria-label="背景图加载中"')
    expect(loading).toContain('aria-pressed="true"')
    expect(loading).toContain('disabled=""')
    expect(loading).toContain('nav-icon-button is-solid')

    const ready = renderToStaticMarkup(
      <HomeTopPanel {...overviewProps} settings={settings} onHome={() => {}} onAdmin={() => {}} onBackgroundToggle={() => {}} backgroundEnabled={false} />,
    )
    expect(ready).toContain('aria-label="开启背景图"')
    expect(ready).toContain('aria-pressed="false"')
    expect(ready).not.toContain('disabled=""')
    expect(ready).not.toContain('nav-icon-button-placeholder')
  })

  it('keeps the no-data count without duplicate status copy', () => {
    const html = renderToStaticMarkup(
      <HomeTopPanel
        {...overviewProps}
        onlineCount={0}
        offlineCount={0}
        settings={settings}
        onHome={() => {}}
        onAdmin={() => {}}
      />,
    )

    expect(html).toContain('home-summary__metric-value--status')
    expect(html).toContain('<strong>0</strong><span>/ 11</span>')
    expect(html).not.toContain('home-summary__status-meta')
    expect(html).not.toContain('运行中')
    expect(html).not.toContain('等待连接')
    expect(html).not.toContain('11 台未在线')
    expect(html).not.toContain('11 台服务器')
    expect(html).not.toContain('全部在线')
  })
})
