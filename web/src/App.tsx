import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { LatencyDetail } from './components/LatencyDetail'
import { ServerCard } from './components/ServerCard'
import { ServiceDetail } from './components/ServiceDetail'
import { applyCustomCode } from './lib/customCode'
import { availableCurrencyOptions, billingCycleMonths, convertCurrencyAmount, normalizeCurrencyRates, rememberHomeCurrency, storedHomeCurrency, type CurrencyCode, type CurrencyRates } from './lib/currency'
import type { AdminTheme, HomeCardNode, LatencyPoint } from './types'
import { DashboardHeader } from './components/DashboardHeader'
import { AdminDashboardLoadError, AdminModuleErrorBoundary } from './components/admin/AdminDashboardBoundary'
import { applyDocumentBranding, settingsForChrome, shellStyleForSettings, storedBackgroundEnabled, storedThemeOverride, useDocumentTheme } from './lib/appearance'
import { useAdminAccess } from './hooks/useAdminAccess'
import { usePublicSettings } from './hooks/usePublicSettings'
import { useDashboardRouter } from './hooks/useDashboardRouter'
import { homeRealtimeSnapshotForNodes, useSummaryController } from './hooks/useSummaryController'
import { useNodeDetailController } from './hooks/useNodeDetailController'
import { useServiceDetailController } from './hooks/useServiceDetailController'
import { HomeRegionFilter, HomeTopPanel } from './components/HomeOverviewPanel'

export { applyCustomCode, extractSafeCustomCSS } from './lib/customCode'
export { availableHistoryRanges, coerceHistoryRange, rangeRequiresAdmin } from './lib/historyRange'
export { loadStoredSummary, rememberSummary } from './lib/summaryCache'
export { adminTokenMaxAgeMs } from './lib/adminToken'
export { applyDocumentBranding, documentBrandingForSettings, shellStyleForSettings } from './lib/appearance'
export { isAdminUnauthorizedError } from './lib/adminSettings'
export { shouldRefreshHomeRealtimeSnapshot } from './hooks/useSummaryController'
export { HomeOverviewPanel, HomeRegionFilter, HomeTopPanel } from './components/HomeOverviewPanel'

const LazyAdminDashboard = lazy(() => import('./components/admin/AdminDashboard').then((module) => ({ default: module.AdminDashboardContainer })))

function sum(values: Array<number | null | undefined>): number {
  return values.reduce<number>((total, value) => total + (value ?? 0), 0)
}

export function homeTrafficTotalsForNodes(nodes: HomeCardNode[]): { totalUp: number; totalDown: number } {
  return {
    totalUp: sum(nodes.map((node) => node.netOutLifetimeBytes ?? node.netOutTotalBytes)),
    totalDown: sum(nodes.map((node) => node.netInLifetimeBytes ?? node.netInTotalBytes)),
  }
}

export function homeMonthlyCostForNodes(nodes: HomeCardNode[], displayCurrency: CurrencyCode, inputExchangeRates: CurrencyRates): number {
  const exchangeRates = normalizeCurrencyRates(inputExchangeRates)
  return sum(nodes.map((node) => {
    if (node.monthlyCostCny === null || node.monthlyCostCny === undefined) return 0
    const cycleMonths = billingCycleMonths(node.billingCycle)
    const convertedRenewal = cycleMonths > 0
      ? convertCurrencyAmount(node.renewalAmount, node.renewalCurrency, displayCurrency, exchangeRates)
      : null
    if (convertedRenewal !== null) return convertedRenewal / cycleMonths
    return convertCurrencyAmount(node.monthlyCostCny, 'CNY', displayCurrency, exchangeRates) ?? 0
  }))
}

function summaryLatencyPoints(node: HomeCardNode | undefined): LatencyPoint[] {
  return (node?.latencySummaries ?? [])
    .filter((summary) => summary.updatedAt)
    .map((summary) => ({
      ts: summary.updatedAt,
      targetId: summary.targetId,
      targetName: summary.targetName,
      medianMs: summary.medianMs,
      avgMs: summary.avgMs,
      lossPercent: summary.lossPercent ?? 0,
    }))
}

export function orderHomeNodes(nodes: HomeCardNode[]): HomeCardNode[] {
  return nodes.map((node, index) => ({ node, index }))
    .sort((left, right) => {
      const leftOffline = left.node.status === 'online' ? 0 : 1
      const rightOffline = right.node.status === 'online' ? 0 : 1
      if (leftOffline !== rightOffline) return leftOffline - rightOffline
      return left.index - right.index
    })
    .map((entry) => entry.node)
}

function normalizeHomeRegion(countryCode: string | undefined): string {
  const code = (countryCode ?? '').trim().toUpperCase()
  return /^[A-Z]{2}$/.test(code) ? code : ''
}

export function homeRegionOptions(nodes: HomeCardNode[]): string[] {
  const seen = new Set<string>()
  const regions: string[] = []
  nodes.forEach((node) => {
    const region = normalizeHomeRegion(node.countryCode)
    if (region === '' || seen.has(region)) return
    seen.add(region)
    regions.push(region)
  })
  return regions
}

export function filterHomeNodesByRegion(nodes: HomeCardNode[], region: string): HomeCardNode[] {
  if (region === 'ALL') return nodes
  return nodes.filter((node) => normalizeHomeRegion(node.countryCode) === region)
}

export function App() {
  const { state, summaryRef, homeRealtimeSnapshot } = useSummaryController()
  const [homeRegion, setHomeRegion] = useState('ALL')
  const [homeCurrency, setHomeCurrency] = useState<CurrencyCode>(storedHomeCurrency)
  const { route, navigateHome, navigateAdmin, navigateNode: navigateNodeRoute } = useDashboardRouter()
  const { settings, settingsReady, setSettings } = usePublicSettings()
  const { adminToken, setAdminToken, expireAdminSession } = useAdminAccess()
  const { nodeLatencyRange, stateRange, latencyState, stateHistoryState, setNodeLatencyRange, setStateRange, resetNodeRanges } = useNodeDetailController({
    nodeId: route.kind === 'node' ? route.nodeId : null,
    summary: summaryRef.current,
    adminToken,
    expireAdminSession,
  })
  const { serviceLatencyRange, serviceLatencyState, setServiceLatencyRange } = useServiceDetailController({
    targetId: route.kind === 'service' ? route.targetId : null,
    adminToken,
    expireAdminSession,
  })
  const navigateNode = (nodeId: string) => {
    resetNodeRanges()
    navigateNodeRoute(nodeId)
  }
  const [backgroundAssetsReady, setBackgroundAssetsReady] = useState(false)
  const [themeOverride, setThemeOverride] = useState<AdminTheme | null>(() => storedThemeOverride())
  const [backgroundEnabled, setBackgroundEnabled] = useState(() => storedBackgroundEnabled())
  const backgroundEnabledRef = useRef(backgroundEnabled)
  const effectiveSettings = settingsForChrome(settings, themeOverride, backgroundEnabled)
  useDocumentTheme(effectiveSettings)

  useEffect(() => {
    applyDocumentBranding(settings)
  }, [settings.siteTitle, settings.logoUrl])

  useEffect(() => {
    applyCustomCode(settings)
  }, [settings.customCode])

  useEffect(() => {
    if (!settingsReady || typeof Image === 'undefined') return undefined
    const urls = [...new Set([settings.desktopBackgroundUrl || settings.backgroundUrl, settings.mobileBackgroundUrl].map((value) => value.trim()).filter(Boolean))]
    if (urls.length === 0) {
      setBackgroundAssetsReady(true)
      return undefined
    }
    let active = true
    let remaining = urls.length
    const timers: number[] = []
    setBackgroundAssetsReady(false)
    const images = urls.map((url) => {
      const image = new Image()
      image.decoding = 'async'
      let settled = false
      const finish = () => {
        if (settled) return
        settled = true
        window.clearTimeout(timeoutID)
        remaining -= 1
        if (active && remaining === 0) setBackgroundAssetsReady(true)
      }
      const timeoutID = window.setTimeout(finish, 8000)
      timers.push(timeoutID)
      image.onload = finish
      image.onerror = finish
      image.src = url
      if (image.complete) queueMicrotask(finish)
      return image
    })
    return () => {
      active = false
      timers.forEach((timerID) => window.clearTimeout(timerID))
      images.forEach((image) => {
        image.onload = null
        image.onerror = null
      })
    }
  }, [settingsReady, settings.backgroundUrl, settings.desktopBackgroundUrl, settings.mobileBackgroundUrl])

  const setThemeMode = (nextTheme: AdminTheme) => {
    window.localStorage.setItem('zeno_theme_override', nextTheme)
    setThemeOverride(nextTheme)
  }

  const toggleBackground = () => {
    const nextValue = !backgroundEnabledRef.current
    backgroundEnabledRef.current = nextValue
    window.localStorage.setItem('zeno_background_enabled', String(nextValue))
    setBackgroundEnabled(nextValue)
  }

  const backgroundConfigured = (settings.desktopBackgroundUrl || settings.backgroundUrl || settings.mobileBackgroundUrl).trim() !== ''
  const backgroundToggle = settingsReady && backgroundConfigured && (!backgroundEnabled || backgroundAssetsReady) ? toggleBackground : undefined
  const nodes = state.kind === 'ready' ? state.data.nodes : []
  const homeRealtimeNodes = homeRealtimeSnapshot?.nodes ?? nodes
  const homeNodes = orderHomeNodes(homeRealtimeNodes)
  const homeRegions = homeRegionOptions(homeNodes)
  const activeHomeRegion = homeRegion === 'ALL' || homeRegions.includes(homeRegion) ? homeRegion : 'ALL'
  const visibleHomeNodes = filterHomeNodesByRegion(homeNodes, activeHomeRegion)
  const services = state.kind === 'ready' ? state.data.services : []
  const exchangeRates = normalizeCurrencyRates(state.kind === 'ready' ? state.data.exchangeRates : null)
  const homeCurrencyOptions = availableCurrencyOptions(exchangeRates)
  const activeHomeCurrency = homeCurrencyOptions.some((option) => option.value === homeCurrency) ? homeCurrency : 'CNY'
  const selectedNode = route.kind === 'node' ? nodes.find((node) => node.id === route.nodeId) : undefined
  const selectedNodeLatencyPoints = latencyState.kind === 'ready' ? latencyState.data.points : summaryLatencyPoints(selectedNode)
  const selectedService = route.kind === 'service' ? services.find((service) => service.id === route.targetId) : undefined
  const totalCount = homeRealtimeNodes.length
  const onlineCount = homeRealtimeNodes.filter((node) => node.status === 'online').length
  const offlineCount = homeRealtimeNodes.filter((node) => node.status === 'offline').length
  const { totalUp, totalDown } = homeTrafficTotalsForNodes(homeRealtimeNodes)
  const monthlyCost = homeMonthlyCostForNodes(homeRealtimeNodes, activeHomeCurrency, exchangeRates)
  const currentRealtimeSnapshot = homeRealtimeSnapshot ?? homeRealtimeSnapshotForNodes(homeRealtimeNodes)
  const upSpeed = currentRealtimeSnapshot.upSpeed
  const downSpeed = currentRealtimeSnapshot.downSpeed
  const hasBackgroundImage = (effectiveSettings.desktopBackgroundUrl || effectiveSettings.backgroundUrl || effectiveSettings.mobileBackgroundUrl).trim() !== ''
  const hasAdminToken = adminToken !== ''
  const changeHomeCurrency = (currency: CurrencyCode) => {
    rememberHomeCurrency(currency)
    setHomeCurrency(currency)
  }

  return (
    <main className="kulin-shell" data-theme={effectiveSettings.theme} data-background={hasBackgroundImage ? 'on' : 'off'} style={shellStyleForSettings(effectiveSettings)}>
      {route.kind === 'admin' && (
        <AdminModuleErrorBoundary fallback={<AdminDashboardLoadError />}>
          <Suspense fallback={<section className="state-panel">加载中…</section>}>
            <LazyAdminDashboard
              onHome={navigateHome}
              settings={settings}
              chromeSettings={effectiveSettings}
              onAdminTokenChange={setAdminToken}
              onSettingsChange={setSettings}
              onThemeChange={setThemeMode}
              onBackgroundToggle={backgroundToggle}
              backgroundEnabled={hasBackgroundImage}
            />
          </Suspense>
        </AdminModuleErrorBoundary>
      )}

      {route.kind !== 'admin' && state.kind === 'loading' && <section className="state-panel">正在读取 Controller API…</section>}
      {route.kind !== 'admin' && state.kind === 'error' && <section className="state-panel is-error">API 读取失败：{state.message}</section>}

      {state.kind === 'ready' && route.kind === 'node' && selectedNode && (
        <LatencyDetail
          node={selectedNode}
          points={selectedNodeLatencyPoints}
          statePoints={stateHistoryState.kind === 'ready' ? stateHistoryState.data.points : []}
          range={nodeLatencyRange}
          stateRange={stateRange}
          loading={latencyState.kind === 'loading'}
          error={latencyState.kind === 'error' ? latencyState.message : undefined}
          stateLoading={stateHistoryState.kind === 'loading'}
          stateError={stateHistoryState.kind === 'error' ? stateHistoryState.message : undefined}
          canUseExtendedRanges={hasAdminToken}
          onBack={navigateHome}
          onRangeChange={setNodeLatencyRange}
          onStateRangeChange={setStateRange}
          topHeader={<DashboardHeader settings={effectiveSettings} onHome={navigateHome} onAdmin={navigateAdmin} onThemeChange={setThemeMode} onBackgroundToggle={backgroundToggle} backgroundEnabled={hasBackgroundImage} />}
        />
      )}

      {state.kind === 'ready' && route.kind === 'node' && !selectedNode && (
        <section className="state-panel is-error">没有找到这台服务器：{route.nodeId}</section>
      )}

      {state.kind === 'ready' && route.kind === 'service' && (selectedService || serviceLatencyState.kind === 'ready') && (
        <ServiceDetail
          target={serviceLatencyState.kind === 'ready' ? serviceLatencyState.data.target : selectedService!}
          points={serviceLatencyState.kind === 'ready' ? serviceLatencyState.data.points : []}
          range={serviceLatencyRange}
          loading={serviceLatencyState.kind === 'loading'}
          error={serviceLatencyState.kind === 'error' ? serviceLatencyState.message : undefined}
          canUseExtendedRanges={hasAdminToken}
          onBack={navigateHome}
          onRangeChange={setServiceLatencyRange}
          topHeader={<DashboardHeader settings={effectiveSettings} onHome={navigateHome} onAdmin={navigateAdmin} onThemeChange={setThemeMode} onBackgroundToggle={backgroundToggle} backgroundEnabled={hasBackgroundImage} />}
        />
      )}

      {state.kind === 'ready' && route.kind === 'service' && !selectedService && serviceLatencyState.kind === 'error' && (
        <section className="state-panel is-error">没有找到这个监控服务：{route.targetId}</section>
      )}

      {state.kind === 'ready' && route.kind === 'home' && (
        <div className="kulin-container">
          <HomeTopPanel
            settings={effectiveSettings}
            totalCount={totalCount}
            onlineCount={onlineCount}
            offlineCount={offlineCount}
            monthlyCost={monthlyCost}
            displayCurrency={activeHomeCurrency}
            exchangeRates={exchangeRates}
            currencyOptions={homeCurrencyOptions}
            onCurrencyChange={changeHomeCurrency}
            totalUp={totalUp}
            totalDown={totalDown}
            upSpeed={upSpeed}
            downSpeed={downSpeed}
            onHome={navigateHome}
            onAdmin={navigateAdmin}
            onThemeChange={setThemeMode}
            onBackgroundToggle={backgroundToggle}
            backgroundEnabled={hasBackgroundImage}
          />

          <HomeRegionFilter regions={homeRegions} activeRegion={activeHomeRegion} onChange={setHomeRegion} />

          <section className="server-card-list" aria-label="server cards">
            {visibleHomeNodes.map((node) => <ServerCard key={node.id} node={node} displayCurrency={activeHomeCurrency} exchangeRates={exchangeRates} onOpen={navigateNode} />)}
          </section>
        </div>
      )}
    </main>
  )
}
