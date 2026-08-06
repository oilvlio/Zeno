import { useEffect, useRef, useState } from 'react'
import { fetchServiceLatency, subscribeServiceLatency, type ServiceLatencyData } from '../api/publicClient'
import { captureAdminTokenIdentity, type AdminTokenIdentity } from '../lib/adminToken'
import { DetailMemoryCache, loadCachedDetailData, rememberDetailData, serviceLatencyCachePrefix } from '../lib/detailCache'
import { detailHttpFallbackDelayMs } from '../lib/detailTiming'
import { coerceHistoryRange, isHTTPUnauthorizedError, rangeRequiresAdmin } from '../lib/historyRange'
import { startResilientLiveData } from '../lib/resilientLive'

export type ServiceLatencyLoadState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; data: ServiceLatencyData }
  | { kind: 'error'; message: string }

function validateServiceLatencyData(value: unknown): ServiceLatencyData | null {
  const data = value as Partial<ServiceLatencyData> | null
  if (!data || !data.target || typeof data.range !== 'string' || !Array.isArray(data.points)) return null
  return data as ServiceLatencyData
}

interface ServiceDetailControllerOptions {
  targetId: string | null
  adminToken: string
  expireAdminSession: (identity: AdminTokenIdentity) => boolean
}

export function useServiceDetailController({ targetId, adminToken, expireAdminSession }: ServiceDetailControllerOptions) {
  const [serviceLatencyRange, setServiceLatencyRange] = useState('1h')
  const [serviceLatencyState, setServiceLatencyState] = useState<ServiceLatencyLoadState>({ kind: 'idle' })
  const serviceLatencyCacheRef = useRef(new DetailMemoryCache<ServiceLatencyData>())

  useEffect(() => {
    const nextRange = coerceHistoryRange(serviceLatencyRange, adminToken !== '', '1h')
    if (nextRange !== serviceLatencyRange) setServiceLatencyRange(nextRange)
  }, [adminToken, serviceLatencyRange])

  useEffect(() => {
    if (targetId === null) {
      setServiceLatencyState({ kind: 'idle' })
      return
    }
    let cancelled = false
    const cacheKey = `${targetId}:${serviceLatencyRange}`
    const memoryCached = serviceLatencyCacheRef.current.getCached(cacheKey)
    const sessionCached = memoryCached ? null : loadCachedDetailData(serviceLatencyCachePrefix, targetId, serviceLatencyRange, validateServiceLatencyData)
    const cached = memoryCached?.data ?? sessionCached?.data ?? null
    if (cached) {
      if (sessionCached) serviceLatencyCacheRef.current.set(cacheKey, sessionCached.data, sessionCached.storedAt)
      setServiceLatencyState({ kind: 'ready', data: cached })
    } else {
      setServiceLatencyState({ kind: 'loading' })
    }
    const applyData = (data: ServiceLatencyData) => {
      serviceLatencyCacheRef.current.set(cacheKey, data)
      rememberDetailData(serviceLatencyCachePrefix, targetId, serviceLatencyRange, data)
      if (!cancelled) setServiceLatencyState({ kind: 'ready', data })
    }
    const useLiveStream = !rangeRequiresAdmin(serviceLatencyRange)
    const requestToken = useLiveStream ? undefined : adminToken
    const requestTokenIdentity = requestToken ? captureAdminTokenIdentity(requestToken) : null
    const stop = startResilientLiveData<ServiceLatencyData>({
      subscribe: useLiveStream ? (onData, onError, onStatus) => subscribeServiceLatency(targetId, serviceLatencyRange, onData, onError, onStatus) : null,
      fetch: (signal) => fetchServiceLatency(targetId, serviceLatencyRange, requestToken, signal),
      applyData,
      // Same cold-open race as the node detail view.
      fetchImmediately: true,
      initialFallbackDelayMs: detailHttpFallbackDelayMs,
      onError: (error) => {
        if (cancelled) return
        const unauthorized = isHTTPUnauthorizedError(error)
        if (unauthorized && requestTokenIdentity && !expireAdminSession(requestTokenIdentity)) return
        setServiceLatencyState((current) => current.kind === 'ready' ? current : { kind: 'error', message: unauthorized ? '登录已过期，请重新登录。' : error instanceof Error ? error.message : 'service latency request failed' })
      },
    })
    return () => {
      cancelled = true
      stop()
    }
  }, [targetId, serviceLatencyRange, adminToken, expireAdminSession])

  return { serviceLatencyRange, serviceLatencyState, setServiceLatencyRange }
}
