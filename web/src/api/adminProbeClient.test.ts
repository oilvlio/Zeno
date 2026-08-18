import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAdminProbeTarget, deleteAdminProbeTarget, fetchAdminProbeTargets, reorderAdminProbeTargets, updateAdminProbeTarget } from './client'

describe('fetchAdminProbeTargets', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('sends the admin token in X-Admin-Token and never in the URL', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ targets: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await fetchAdminProbeTargets('admin-pass')

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets', {
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})
describe('reorderAdminProbeTargets', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('sends the complete target order in one PATCH request', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await reorderAdminProbeTargets('admin-pass', ['target-c', 'target-a', 'target-b'])

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets/reorder', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({ target_ids: ['target-c', 'target-a', 'target-b'] }),
    })
  })
})
describe('createAdminProbeTarget', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('creates a probe target with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      target: {
        id: 'example-https-a1b2c3d4',
        name: 'Example HTTPS',
        type: 'tcping',
        address: 'example.com',
        port: 443,
        count: 5,
        timeout_ms: 1500,
        interval_sec: 90,
        display_order: 20,
        assignments: [{ node_id: 'example-node-a', node_display_name: 'Example Node A', enabled: true }],
      },
    }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const target = await createAdminProbeTarget('admin-pass', {
      name: 'Example HTTPS',
      type: 'tcping',
      address: 'example.com',
      port: 443,
      count: 5,
      timeoutMs: 1500,
      intervalSec: 90,
      displayOrder: 20,
    })

    expect(target.id).toBe('example-https-a1b2c3d4')
    expect(target.assignments[0].nodeId).toBe('example-node-a')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Example HTTPS',
        type: 'tcping',
        address: 'example.com',
        port: 443,
        count: 5,
        timeout_ms: 1500,
        interval_sec: 90,
        display_order: 20,
      }),
    })
  })

  it('creates an HTTP GET probe target without a separate port', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      target: {
        id: 'zeno-health-http',
        name: 'Zeno Health HTTP',
        type: 'http_get',
        address: 'https://example.com/health',
        port: null,
        count: 2,
        timeout_ms: 1500,
        interval_sec: 60,
        display_order: 30,
        assignments: [],
      },
    }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const target = await createAdminProbeTarget('admin-pass', {
      name: 'Zeno Health HTTP',
      type: 'http_get',
      address: 'https://example.com/health',
      port: null,
      count: 2,
      timeoutMs: 1500,
      intervalSec: 60,
      displayOrder: 30,
    })

    expect(target.type).toBe('http_get')
    expect(target.port).toBeNull()
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Zeno Health HTTP',
        type: 'http_get',
        address: 'https://example.com/health',
        port: null,
        count: 2,
        timeout_ms: 1500,
        interval_sec: 60,
        display_order: 30,
      }),
    })
  })
})
describe('updateAdminProbeTarget', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('patches editable probe target fields with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      target: {
        id: 'example-node-a-local',
        name: 'Local Controller',
        type: 'tcping',
        address: '127.0.0.1',
        port: 18981,
        count: 4,
        timeout_ms: 900,
        interval_sec: 30,
        display_order: 40,
        assignments: [{ node_id: 'example-node-a', node_display_name: 'Example Node A', enabled: true }],
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const target = await updateAdminProbeTarget('admin-pass', 'example-node-a-local', {
      name: 'Local Controller',
      address: '127.0.0.1',
      port: 18981,
      count: 4,
      timeoutMs: 900,
      intervalSec: 30,
      displayOrder: 40,
      assignments: [
        { nodeId: 'example-node-a', enabled: false },
        { nodeId: 'backup', enabled: true },
      ],
    })

    expect(target.name).toBe('Local Controller')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets/example-node-a-local', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Local Controller',
        address: '127.0.0.1',
        port: 18981,
        count: 4,
        timeout_ms: 900,
        interval_sec: 30,
        display_order: 40,
        assignments: [
          { node_id: 'example-node-a', enabled: false },
          { node_id: 'backup', enabled: true },
        ],
      }),
    })
  })

  it('patches HTTP GET probe targets with a null port to clear TCP state', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      target: {
        id: 'example-node-a-local',
        name: 'Zeno Health HTTP',
        type: 'http_get',
        address: 'https://example.com/health',
        port: null,
        count: 2,
        timeout_ms: 1500,
        interval_sec: 60,
        display_order: 50,
        assignments: [{ node_id: 'example-node-a', node_display_name: 'Example Node A', enabled: true }],
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const target = await updateAdminProbeTarget('admin-pass', 'example-node-a-local', {
      name: 'Zeno Health HTTP',
      type: 'http_get',
      address: 'https://example.com/health',
      port: null,
      count: 2,
      timeoutMs: 1500,
      intervalSec: 60,
      displayOrder: 50,
    })

    expect(target.type).toBe('http_get')
    expect(target.port).toBeNull()
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets/example-node-a-local', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Zeno Health HTTP',
        type: 'http_get',
        address: 'https://example.com/health',
        port: null,
        count: 2,
        timeout_ms: 1500,
        interval_sec: 60,
        display_order: 50,
      }),
    })
  })
})
describe('deleteAdminProbeTarget', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('deletes a probe target with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await deleteAdminProbeTarget('admin-pass', 'example-node-a-local')

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/probe-targets/example-node-a-local', {
      method: 'DELETE',
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})
