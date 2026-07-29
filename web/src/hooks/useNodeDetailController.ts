import { useEffect, useRef, useState } from 'react'
import { fetchNodeLatency, fetchNodeState, subscribeNodeLatency, subscribeNodeState, type NodeLatencyData, type NodeStateData, type SummaryData } from '../api/publicClient'
import { captureAdminTokenIdentity, type AdminTokenIdentity } from '../lib/adminToken'
import { DetailMemoryCache, loadCachedDetailData, nodeLatencyCachePrefix, nodeStateCachePrefix, rememberDetailData } from '../lib/detailCache'
import { coerceHistoryRange, isHTTPUnauthorizedError, rangeRequiresAdmin } from '../lib/historyRange'
import { startResilientLiveData } from '../lib/resilientLive'
import type { StatePoint } from '../types'

export type LatencyLoadState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; data: NodeLatencyData }
  | { kind: 'error'; message: string }

export type StateHistoryLoadState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; data: NodeStateData }
  | { kind: 'error'; message: string }

const detailHttpFallbackDelayMs = 1800

function validateNodeLatencyData(value: unknown): NodeLatencyData | null {
  const data = value as Partial<NodeLatencyData> | null
  if (!data || typeof data.nodeId !== 'string' || typeof data.range !== 'string' || !Array.isArray(data.points)) return null
  return data as NodeLatencyData
}

function validateNodeStateData(value: unknown): NodeStateData | null {
  const data = value as Partial<NodeStateData> | null
  if (!data || typeof data.nodeId !== 'string' || typeof data.range !== 'string' || !Array.isArray(data.points)) return null
  return data as NodeStateData
}

function seedNodeLatencyFromSummary(summary: SummaryData | null, nodeId: string, range: string): NodeLatencyData | null {
  const node = summary?.nodes.find((item) => item.id === nodeId)
  if (!node) return null
  const summaries = node.latencySummaries && node.latencySummaries.length > 0 ? node.latencySummaries : node.latencySummary ? [node.latencySummary] : []
  if (summaries.length === 0) return null
  return {
    nodeId,
    range,
    points: summaries.map((item) => ({
      ts: item.updatedAt || new Date().toISOString(),
      targetId: item.targetId,
      targetName: item.targetName,
      medianMs: item.medianMs,
      avgMs: item.avgMs ?? null,
      lossPercent: item.lossPercent ?? 0,
    })),
  }
}

function seedNodeStateFromSummary(summary: SummaryData | null, nodeId: string, range: string): NodeStateData | null {
  const node = summary?.nodes.find((item) => item.id === nodeId)
  if (!node) return null
  const point: StatePoint = {
    ts: new Date().toISOString(),
    cpuPercent: node.cpuPercent,
    load1: node.load1 ?? null,
    load5: node.load5 ?? null,
    load15: node.load15 ?? null,
    memoryUsedBytes: node.memoryUsedBytes,
    memoryTotalBytes: node.memoryTotalBytes,
    swapUsedBytes: null,
    swapTotalBytes: null,
    diskUsedBytes: node.diskUsedBytes,
    diskTotalBytes: node.diskTotalBytes,
    netInTotalBytes: node.netInTotalBytes,
    netOutTotalBytes: node.netOutTotalBytes,
    netInSpeedBps: node.netInSpeedBps,
    netOutSpeedBps: node.netOutSpeedBps,
    processCount: null,
    tcpConnectionCount: null,
    udpConnectionCount: null,
    uptimeSeconds: node.uptimeSeconds ?? null,
  }
  return { nodeId, range, points: [point] }
}

interface NodeDetailControllerOptions {
  nodeId: string | null
  summary: SummaryData | null
  adminToken: string
  expireAdminSession: (identity: AdminTokenIdentity) => boolean
}

export function useNodeDetailController({ nodeId, summary, adminToken, expireAdminSession }: NodeDetailControllerOptions) {
  const [nodeLatencyRange, setNodeLatencyRange] = useState('1d')
  const [stateRange, setStateRange] = useState('1h')
  const [latencyState, setLatencyState] = useState<LatencyLoadState>({ kind: 'idle' })
  const [stateHistoryState, setStateHistoryState] = useState<StateHistoryLoadState>({ kind: 'idle' })
  const nodeLatencyCacheRef = useRef(new DetailMemoryCache<NodeLatencyData>())
  const nodeStateCacheRef = useRef(new DetailMemoryCache<NodeStateData>())

  useEffect(() => {
    const hasToken = adminToken !== ''
    const nextNodeRange = coerceHistoryRange(nodeLatencyRange, hasToken, '1d')
    const nextStateRange = coerceHistoryRange(stateRange, hasToken, '1h')
    if (nextNodeRange !== nodeLatencyRange) setNodeLatencyRange(nextNodeRange)
    if (nextStateRange !== stateRange) setStateRange(nextStateRange)
  }, [adminToken, nodeLatencyRange, stateRange])

  useEffect(() => {
    if (nodeId === null) {
      setLatencyState({ kind: 'idle' })
      return
    }
    let cancelled = false
    const cacheKey = `${nodeId}:${nodeLatencyRange}`
    const memoryCached = nodeLatencyCacheRef.current.getCached(cacheKey)
    const sessionCached = memoryCached ? null : loadCachedDetailData(nodeLatencyCachePrefix, nodeId, nodeLatencyRange, validateNodeLatencyData)
    const cached = memoryCached?.data ?? sessionCached?.data ?? null
    const seeded = cached ?? seedNodeLatencyFromSummary(summary, nodeId, nodeLatencyRange)
    if (sessionCached) nodeLatencyCacheRef.current.set(cacheKey, sessionCached.data, sessionCached.storedAt)
    if (seeded) setLatencyState({ kind: 'ready', data: seeded })
    else setLatencyState((current) => current.kind === 'ready' && current.data.nodeId === nodeId ? current : { kind: 'loading' })
    const applyData = (data: NodeLatencyData) => {
      nodeLatencyCacheRef.current.set(cacheKey, data)
      rememberDetailData(nodeLatencyCachePrefix, nodeId, nodeLatencyRange, data)
      if (!cancelled) setLatencyState({ kind: 'ready', data })
    }
    const useLiveStream = !rangeRequiresAdmin(nodeLatencyRange)
    const requestToken = useLiveStream ? undefined : adminToken
    const requestTokenIdentity = requestToken ? captureAdminTokenIdentity(requestToken) : null
    const stop = startResilientLiveData<NodeLatencyData>({
      subscribe: useLiveStream ? (onData, onError, onStatus) => subscribeNodeLatency(nodeId, nodeLatencyRange, onData, onError, onStatus) : null,
      fetch: (signal) => fetchNodeLatency(nodeId, nodeLatencyRange, requestToken, signal),
      applyData,
      initialFallbackDelayMs: detailHttpFallbackDelayMs,
      onError: (error) => {
        if (cancelled) return
        const unauthorized = isHTTPUnauthorizedError(error)
        if (unauthorized && requestTokenIdentity && !expireAdminSession(requestTokenIdentity)) return
        setLatencyState((current) => current.kind === 'ready' ? current : { kind: 'error', message: unauthorized ? '登录已过期，请重新登录。' : error instanceof Error ? error.message : 'latency request failed' })
      },
    })
    return () => {
      cancelled = true
      stop()
    }
  }, [nodeId, nodeLatencyRange, adminToken, expireAdminSession])

  useEffect(() => {
    if (nodeId === null) {
      setStateHistoryState({ kind: 'idle' })
      return
    }
    let cancelled = false
    const cacheKey = `${nodeId}:${stateRange}`
    const memoryCached = nodeStateCacheRef.current.getCached(cacheKey)
    const sessionCached = memoryCached ? null : loadCachedDetailData(nodeStateCachePrefix, nodeId, stateRange, validateNodeStateData)
    const cached = memoryCached?.data ?? sessionCached?.data ?? null
    const seeded = cached ?? seedNodeStateFromSummary(summary, nodeId, stateRange)
    if (sessionCached) nodeStateCacheRef.current.set(cacheKey, sessionCached.data, sessionCached.storedAt)
    setStateHistoryState(seeded ? { kind: 'ready', data: seeded } : { kind: 'loading' })
    const applyData = (data: NodeStateData) => {
      nodeStateCacheRef.current.set(cacheKey, data)
      rememberDetailData(nodeStateCachePrefix, nodeId, stateRange, data)
      if (!cancelled) setStateHistoryState({ kind: 'ready', data })
    }
    const useLiveStream = !rangeRequiresAdmin(stateRange)
    const requestToken = useLiveStream ? undefined : adminToken
    const requestTokenIdentity = requestToken ? captureAdminTokenIdentity(requestToken) : null
    const stop = startResilientLiveData<NodeStateData>({
      subscribe: useLiveStream ? (onData, onError, onStatus) => subscribeNodeState(nodeId, stateRange, onData, onError, onStatus) : null,
      fetch: (signal) => fetchNodeState(nodeId, stateRange, requestToken, signal),
      applyData,
      initialFallbackDelayMs: detailHttpFallbackDelayMs,
      onError: (error) => {
        if (cancelled) return
        const unauthorized = isHTTPUnauthorizedError(error)
        if (unauthorized && requestTokenIdentity && !expireAdminSession(requestTokenIdentity)) return
        setStateHistoryState((current) => current.kind === 'ready' ? current : { kind: 'error', message: unauthorized ? '登录已过期，请重新登录。' : error instanceof Error ? error.message : 'state request failed' })
      },
    })
    return () => {
      cancelled = true
      stop()
    }
  }, [nodeId, stateRange, adminToken, expireAdminSession])

  useEffect(() => {
    if (nodeId === null || summary === null) return
    const latencySeed = seedNodeLatencyFromSummary(summary, nodeId, nodeLatencyRange)
    if (latencySeed) setLatencyState((current) => current.kind === 'loading' || current.kind === 'idle' ? { kind: 'ready', data: latencySeed } : current)
    const stateSeed = seedNodeStateFromSummary(summary, nodeId, stateRange)
    if (stateSeed) setStateHistoryState((current) => current.kind === 'loading' || current.kind === 'idle' ? { kind: 'ready', data: stateSeed } : current)
  }, [summary, nodeId, nodeLatencyRange, stateRange])

  return {
    nodeLatencyRange,
    stateRange,
    latencyState,
    stateHistoryState,
    setNodeLatencyRange,
    setStateRange,
    resetNodeRanges: () => {
      setNodeLatencyRange('1d')
      setStateRange('1h')
    },
  }
}
