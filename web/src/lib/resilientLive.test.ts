import { afterEach, describe, expect, it, vi } from 'vitest'
import { startResilientLiveData } from './resilientLive'
import type { LiveWebSocketStatus } from '../api/publicClient'

afterEach(() => {
  vi.useRealTimers()
})

describe('startResilientLiveData', () => {
  it('starts sustained HTTP fallback after 90s of websocket reconnecting and applies HTTP after prior WS frames', async () => {
    vi.useFakeTimers()
    let pushLive = (_data: string) => {}
    let pushStatus = (_status: LiveWebSocketStatus) => {}
    const fetch = vi.fn().mockResolvedValue('http-after-drop')
    const applyData = vi.fn()

    const stop = startResilientLiveData<string>({
      subscribe: (onData, _onError, onStatus) => {
        pushLive = onData
        pushStatus = onStatus ?? pushStatus
        onStatus?.('open')
        return vi.fn()
      },
      fetch,
      applyData,
    })

    pushLive('ws-frame')
    expect(applyData).toHaveBeenLastCalledWith('ws-frame', 'ws')

    pushStatus('reconnecting')
    await vi.advanceTimersByTimeAsync(89_999)
    expect(fetch).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(applyData).toHaveBeenLastCalledWith('http-after-drop', 'http')

    stop()
  })

  it('aborts a stalled HTTP fallback and allows the next interval to retry', async () => {
    vi.useFakeTimers()
    const signals: AbortSignal[] = []
    const fetch = vi.fn((signal?: AbortSignal) => {
      if (signal) signals.push(signal)
      return new Promise<string>((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true })
      })
    })

    const stop = startResilientLiveData<string>({
      subscribe: null,
      fetch,
      applyData: vi.fn(),
      httpFallbackTimeoutMs: 10_000,
      httpFallbackIntervalMs: 15_000,
    })

    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(signals[0]?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(5_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    stop()
  })

  it('ignores a late HTTP fallback success after timeout aborts the request', async () => {
    vi.useFakeTimers()
    const signals: AbortSignal[] = []
    const resolvers: Array<(value: string) => void> = []
    const fetch = vi.fn((signal?: AbortSignal) => {
      if (signal) signals.push(signal)
      return new Promise<string>((resolve) => resolvers.push(resolve))
    })
    const applyData = vi.fn()

    const stop = startResilientLiveData<string>({
      subscribe: null,
      fetch,
      applyData,
      httpFallbackTimeoutMs: 10_000,
      httpFallbackIntervalMs: 15_000,
    })

    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(signals[0]?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(5_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    resolvers[0]?.('late-stale-http')
    await Promise.resolve()
    expect(applyData).not.toHaveBeenCalled()

    resolvers[1]?.('fresh-http')
    await Promise.resolve()
    expect(applyData).toHaveBeenCalledTimes(1)
    expect(applyData).toHaveBeenLastCalledWith('fresh-http', 'http')

    stop()
  })

  it('keeps HTTP fallback through a bare websocket handshake and stops only after a fresh frame', async () => {
    vi.useFakeTimers()
    let pushLive = (_data: string) => {}
    let pushStatus = (_status: LiveWebSocketStatus) => {}
    const fetch = vi.fn().mockResolvedValue('http-fallback')
    const applyData = vi.fn()

    const stop = startResilientLiveData<string>({
      subscribe: (onData, _onError, onStatus) => {
        pushLive = onData
        pushStatus = onStatus ?? pushStatus
        onStatus?.('open')
        return vi.fn()
      },
      fetch,
      applyData,
    })

    pushLive('ws-frame')
    pushStatus('reconnecting')
    await vi.advanceTimersByTimeAsync(90_000)
    expect(fetch).toHaveBeenCalledTimes(1)

    pushStatus('open')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    pushLive('ws-recovered')
    expect(applyData).toHaveBeenLastCalledWith('ws-recovered', 'ws')
    await vi.advanceTimersByTimeAsync(45_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    stop()
  })

  it('does not let a stale aborted request unlock a newer fallback request', async () => {
    vi.useFakeTimers()
    let pushLive = (_data: string) => {}
    let pushStatus = (_status: LiveWebSocketStatus) => {}
    const resolvers: Array<(value: string) => void> = []
    const fetch = vi.fn(() => new Promise<string>((resolve) => resolvers.push(resolve)))

    const stop = startResilientLiveData<string>({
      subscribe: (onData, _onError, onStatus) => {
        pushLive = onData
        pushStatus = onStatus ?? pushStatus
        onStatus?.('open')
        return vi.fn()
      },
      fetch,
      applyData: vi.fn(),
      httpFallbackTimeoutMs: 60_000,
    })

    await vi.advanceTimersByTimeAsync(1_800)
    expect(fetch).toHaveBeenCalledTimes(1)
    pushLive('ws-frame')
    pushStatus('reconnecting')
    await vi.advanceTimersByTimeAsync(90_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    resolvers[0]?.('stale-http')
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(fetch).toHaveBeenCalledTimes(2)

    stop()
  })

  it('starts HTTP fallback when an open websocket silently stops producing frames', async () => {
    vi.useFakeTimers()
    let pushLive = (_data: string) => {}
    const fetch = vi.fn().mockResolvedValue('http-after-stall')
    const applyData = vi.fn()
    const stop = startResilientLiveData<string>({
      subscribe: (onData, _onError, onStatus) => {
        pushLive = onData
        onStatus?.('open')
        return vi.fn()
      },
      fetch,
      applyData,
    })

    pushLive('initial-frame')
    await vi.advanceTimersByTimeAsync(89_999)
    expect(fetch).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(applyData).toHaveBeenLastCalledWith('http-after-stall', 'http')

    stop()
  })
})

describe('startResilientLiveData immediate HTTP priming', () => {
  it('fetches over HTTP straight away instead of waiting out the websocket handshake', async () => {
    vi.useFakeTimers()
    const fetch = vi.fn().mockResolvedValue('http-primed')
    const applyData = vi.fn()

    const stop = startResilientLiveData<string>({
      subscribe: () => vi.fn(),
      fetch,
      applyData,
      fetchImmediately: true,
    })

    // No timer advance: the request must already be in flight.
    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(0)
    expect(applyData).toHaveBeenCalledWith('http-primed', 'http')

    stop()
  })

  it('does not prime over HTTP unless asked, so summary views keep socket-first behaviour', () => {
    vi.useFakeTimers()
    const fetch = vi.fn().mockResolvedValue('unused')

    const stop = startResilientLiveData<string>({
      subscribe: () => vi.fn(),
      fetch,
      applyData: vi.fn(),
    })

    expect(fetch).not.toHaveBeenCalled()
    stop()
  })

  // The whole point of the race is that it may not lose. A websocket frame is
  // the fresher source, so a priming response that resolves later must be
  // discarded rather than overwrite live data with an older snapshot.
  it('drops a late priming response once a websocket frame has arrived', async () => {
    vi.useFakeTimers()
    let resolveFetch: (value: string) => void = () => {}
    const fetch = vi.fn().mockReturnValue(new Promise<string>((resolve) => { resolveFetch = resolve }))
    const applyData = vi.fn()
    let pushLive = (_data: string) => {}

    const stop = startResilientLiveData<string>({
      subscribe: (onData) => {
        pushLive = onData
        return vi.fn()
      },
      fetch,
      applyData,
      fetchImmediately: true,
    })

    pushLive('ws-frame')
    expect(applyData).toHaveBeenLastCalledWith('ws-frame', 'ws')

    resolveFetch('stale-http')
    await vi.advanceTimersByTimeAsync(0)

    expect(applyData).toHaveBeenCalledTimes(1)
    expect(applyData).toHaveBeenLastCalledWith('ws-frame', 'ws')

    stop()
  })

  it('reports no error when the priming request fails, because the socket still owns the view', async () => {
    vi.useFakeTimers()
    const fetch = vi.fn().mockRejectedValue(new Error('primed request failed'))
    const onError = vi.fn()

    const stop = startResilientLiveData<string>({
      subscribe: () => vi.fn(),
      fetch,
      applyData: vi.fn(),
      onError,
      fetchImmediately: true,
    })

    await vi.advanceTimersByTimeAsync(0)
    expect(onError).not.toHaveBeenCalled()

    stop()
  })

  it('aborts an in-flight priming request when the view is torn down', () => {
    vi.useFakeTimers()
    let capturedSignal: AbortSignal | undefined
    const fetch = vi.fn().mockImplementation((signal?: AbortSignal) => {
      capturedSignal = signal
      return new Promise<string>(() => {})
    })

    const stop = startResilientLiveData<string>({
      subscribe: () => vi.fn(),
      fetch,
      applyData: vi.fn(),
      fetchImmediately: true,
    })

    expect(capturedSignal?.aborted).toBe(false)
    stop()
    expect(capturedSignal?.aborted).toBe(true)
  })
})
