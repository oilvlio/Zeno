import type { AdminSettings } from '../types'
import { adminCookieSessionMarker } from '../lib/adminToken'
import { normalizeNodeLatency, normalizeNodeState, normalizeServiceLatency, normalizeSettings, normalizeSummary } from './publicNormalizers'
import type { ApiLatencyResponse, ApiServiceLatencyResponse, ApiSettings, ApiStateResponse, ApiSummaryResponse, NodeLatencyData, NodeStateData, ServiceLatencyData, SummaryData } from './apiTypes'
export type { ApiLatencyResponse, ApiServiceLatencyResponse, ApiStateResponse, ApiSummaryResponse, NodeLatencyData, NodeStateData, ServiceLatencyData, SummaryData } from './apiTypes'
export { normalizeNodeLatency, normalizeNodeState, normalizeServiceLatency, normalizeSettings, normalizeSummary } from './publicNormalizers'

export type LiveWebSocketStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

export async function fetchPublicSettings(): Promise<AdminSettings> {
  const response = await fetch('/api/public/v1/settings', { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`settings request failed: ${response.status}`)
  }
  return normalizeSettings(await response.json() as ApiSettings)
}

export async function fetchSummary(signal?: AbortSignal): Promise<SummaryData> {
  const response = await fetch('/api/public/v1/summary', { signal, headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`summary request failed: ${response.status}`)
  }
  return normalizeSummary(await response.json() as ApiSummaryResponse)
}

function liveWebSocketURL(path: string): string {
  const baseURL = typeof window === 'undefined' ? 'http://localhost/' : window.location.href
  const url = new URL(path, baseURL)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

function subscribeLiveWebSocket<T>(path: string, normalize: (payload: unknown) => T, onData: (data: T) => void, onError?: (error: Error) => void, onStatus?: (status: LiveWebSocketStatus) => void): (() => void) | null {
  if (typeof WebSocket === 'undefined') return null
  let closedByClient = false
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempts = 0

  const clearReconnectTimer = () => {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const connect = () => {
    if (closedByClient) return
    onStatus?.(reconnectAttempts > 0 ? 'reconnecting' : 'connecting')
    socket = new WebSocket(liveWebSocketURL(path))
    socket.onopen = () => {
      onStatus?.('open')
    }
    socket.onmessage = (event) => {
      try {
        if (typeof event.data !== 'string') throw new Error('live websocket message must be text')
        reconnectAttempts = 0
        onData(normalize(JSON.parse(event.data) as unknown))
      } catch (error) {
        onError?.(error instanceof Error ? error : new Error('live websocket parse failed'))
      }
    }
    socket.onerror = () => {
      socket?.close()
    }
    socket.onclose = () => {
      if (closedByClient) return
      reconnectAttempts += 1
      onStatus?.('reconnecting')
      clearReconnectTimer()
      reconnectTimer = setTimeout(connect, Math.min(1000 + reconnectAttempts * 250, 30_000))
    }
  }

  connect()
  return () => {
    closedByClient = true
    clearReconnectTimer()
    socket?.close()
    socket = null
  }
}

export function subscribeSummary(onSummary: (summary: SummaryData) => void, onError?: (error: Error) => void, onStatus?: (status: LiveWebSocketStatus) => void): (() => void) | null {
  return subscribeLiveWebSocket('/api/public/v1/summary/ws', (payload) => normalizeSummary(payload as ApiSummaryResponse), onSummary, onError, onStatus)
}

export function subscribeNodeLatency(nodeId: string, range: string, onLatency: (latency: NodeLatencyData) => void, onError?: (error: Error) => void, onStatus?: (status: LiveWebSocketStatus) => void): (() => void) | null {
  return subscribeLiveWebSocket(`/api/public/v1/nodes/${encodeURIComponent(nodeId)}/latency/ws?range=${encodeURIComponent(range)}`, (payload) => normalizeNodeLatency(payload as ApiLatencyResponse), onLatency, onError, onStatus)
}

export function subscribeNodeState(nodeId: string, range: string, onState: (state: NodeStateData) => void, onError?: (error: Error) => void, onStatus?: (status: LiveWebSocketStatus) => void): (() => void) | null {
  return subscribeLiveWebSocket(`/api/public/v1/nodes/${encodeURIComponent(nodeId)}/state/ws?range=${encodeURIComponent(range)}`, (payload) => normalizeNodeState(payload as ApiStateResponse), onState, onError, onStatus)
}

export function subscribeServiceLatency(targetId: string, range: string, onLatency: (latency: ServiceLatencyData) => void, onError?: (error: Error) => void, onStatus?: (status: LiveWebSocketStatus) => void): (() => void) | null {
  return subscribeLiveWebSocket(`/api/public/v1/services/${encodeURIComponent(targetId)}/latency/ws?range=${encodeURIComponent(range)}`, (payload) => normalizeServiceLatency(payload as ApiServiceLatencyResponse), onLatency, onError, onStatus)
}

function optionalAdminHeaders(adminToken?: string): HeadersInit {
  return adminToken && adminToken !== adminCookieSessionMarker ? { Accept: 'application/json', 'X-Admin-Token': adminToken } : { Accept: 'application/json' }
}

export async function fetchNodeLatency(nodeId: string, range = '1h', adminToken?: string, signal?: AbortSignal): Promise<NodeLatencyData> {
  const response = await fetch(`/api/public/v1/nodes/${encodeURIComponent(nodeId)}/latency?range=${encodeURIComponent(range)}`, { signal, headers: optionalAdminHeaders(adminToken) })
  if (!response.ok) {
    throw new Error(`latency request failed: ${response.status}`)
  }
  return normalizeNodeLatency(await response.json() as ApiLatencyResponse)
}

export async function fetchServiceLatency(targetId: string, range = '1h', adminToken?: string, signal?: AbortSignal): Promise<ServiceLatencyData> {
  const response = await fetch(`/api/public/v1/services/${encodeURIComponent(targetId)}/latency?range=${encodeURIComponent(range)}`, { signal, headers: optionalAdminHeaders(adminToken) })
  if (!response.ok) {
    throw new Error(`service latency request failed: ${response.status}`)
  }
  return normalizeServiceLatency(await response.json() as ApiServiceLatencyResponse)
}

export async function fetchNodeState(nodeId: string, range = '1h', adminToken?: string, signal?: AbortSignal): Promise<NodeStateData> {
  const response = await fetch(`/api/public/v1/nodes/${encodeURIComponent(nodeId)}/state?range=${encodeURIComponent(range)}`, { signal, headers: optionalAdminHeaders(adminToken) })
  if (!response.ok) {
    throw new Error(`state request failed: ${response.status}`)
  }
  return normalizeNodeState(await response.json() as ApiStateResponse)
}
