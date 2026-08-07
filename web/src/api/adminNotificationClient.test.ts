import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAdminNotificationChannel, deleteAdminNotificationChannel, fetchAdminAlertRules, fetchAdminNotificationChannels, normalizeAdminAlertRules, testAdminNotificationChannel, updateAdminAlertRule, updateAdminNotificationChannel } from './client'

describe('fetchAdminNotificationChannels', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('fetches notification channels with X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      if (String(url).includes('notification-channels')) {
        return new Response(JSON.stringify({ channels: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      return new Response(JSON.stringify({ types: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await fetchAdminNotificationChannels('admin-pass')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/v1/notification-channels', {
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})
describe('admin alert rules', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('normalizes notification types and fetches them with X-Admin-Token only', async () => {
    const apiPayload = {
      rules: [
        {
          id: 'cpu_high',
          name: 'CPU 使用率',
          category: 'resource',
          metric: 'cpu_percent',
          comparator: '>=',
          threshold: 90,
          threshold_unit: '%',
          duration_sec: 300,
          enabled: true,
          notification_event_type: 'probe_unhealthy',
          notification_label: '异常',
          description: '',
          scope_node_ids: ['example-node-a'],
          created_at: '2026-07-03T00:00:00Z',
          updated_at: '2026-07-03T00:00:00Z',
        },
      ],
    }
    const normalized = normalizeAdminAlertRules(apiPayload)
    expect(normalized.rules[0].thresholdUnit).toBe('%')
    expect(normalized.rules[0].durationSec).toBe(300)
    expect(normalized.rules[0].notificationLabel).toBe('异常')
    expect(normalized.rules[0].scopeNodeIds).toEqual(['example-node-a'])
    expect(normalized.rules[0].renewalDays).toEqual([])

    const fetchMock = vi.fn(async () => new Response(JSON.stringify(apiPayload), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const fetched = await fetchAdminAlertRules('admin-pass')
    expect(fetched.rules[0].metric).toBe('cpu_percent')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/alert-rules', {
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })

  it('updates notification type enablement threshold and duration without putting admin token in the URL', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({
      rule: {
        id: 'cpu_high',
        name: 'CPU 使用率',
        category: 'resource',
        metric: 'cpu_percent',
        comparator: '>=',
        threshold: 95.5,
        threshold_unit: '%',
        duration_sec: 600,
        enabled: false,
        notification_event_type: 'probe_unhealthy',
        notification_label: '异常',
        description: '',
        scope_node_ids: ['example-node-a', 'backup'],
        created_at: '2026-07-03T00:00:00Z',
        updated_at: '2026-07-03T00:10:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const rule = await updateAdminAlertRule('admin-pass', 'cpu_high', { enabled: false, threshold: 95.5, durationSec: 600, scopeNodeIds: ['example-node-a', 'backup'] })

    expect(rule.enabled).toBe(false)
    expect(rule.threshold).toBe(95.5)
    expect(rule.durationSec).toBe(600)
    expect(rule.scopeNodeIds).toEqual(['example-node-a', 'backup'])
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/alert-rules/cpu_high', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({ enabled: false, threshold: 95.5, duration_sec: 600, scope_node_ids: ['example-node-a', 'backup'] }),
    })
    const calls = fetchMock.mock.calls as unknown as Array<[RequestInfo | URL, RequestInit?]>
    expect(String(calls[0]?.[0])).not.toContain('admin-pass')
  })

  it('serializes multiple renewal reminder days', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      rule: {
        id: 'renewal_due',
        name: '续费提醒',
        category: 'billing',
        metric: 'expiry_days',
        comparator: '<=',
        threshold: 7,
        renewal_days: [1, 3, 7],
        threshold_unit: 'd',
        duration_sec: 0,
        enabled: true,
        notification_event_type: 'renewal_due',
        notification_label: '续费',
        description: '',
        scope_node_ids: [],
        created_at: '2026-07-03T00:00:00Z',
        updated_at: '2026-08-07T00:00:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const rule = await updateAdminAlertRule('admin-pass', 'renewal_due', { renewalDays: [1, 3, 7] })

    expect(rule.renewalDays).toEqual([1, 3, 7])
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/alert-rules/renewal_due', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({ renewal_days: [1, 3, 7] }),
    })
  })

})
describe('notification writes', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('creates and toggles notification config without placing credentials in URLs', async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      if (String(url).endsWith('/notification-channels')) {
        return new Response(JSON.stringify({
          channel: {
            id: 'zeno-telegram',
            name: 'Zeno Telegram',
              destination: '7579942307',
            credential_set: true,
            enabled: true,
            created_at: '2026-07-03T00:00:00Z',
            updated_at: '2026-07-03T00:00:00Z',
          },
        }), { status: 201, headers: { 'Content-Type': 'application/json' } })
      }
      if (String(url).includes('/notification-channels/')) {
        return new Response(JSON.stringify({
          channel: {
            id: 'zeno-telegram',
            name: 'Zeno Telegram',
              destination: '7579942307',
            credential_set: true,
            enabled: false,
            created_at: '2026-07-03T00:00:00Z',
            updated_at: '2026-07-03T00:00:00Z',
          },
        }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      return new Response(JSON.stringify({ type: { event_type: 'node_offline', label: '离线', enabled: true, updated_at: '2026-07-03T00:00:00Z' } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const created = await createAdminNotificationChannel('admin-pass', {
      name: 'Zeno Telegram',
      destination: '7579942307',
      credential: 'telegram-bot-secret',
      enabled: true,
    })
    const updated = await updateAdminNotificationChannel('admin-pass', 'zeno-telegram', { enabled: false })
    expect(created.credentialSet).toBe(true)
    expect(updated.enabled).toBe(false)
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/v1/notification-channels', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Zeno Telegram',
          destination: '7579942307',
        credential: 'telegram-bot-secret',
        enabled: true,
      }),
    })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/v1/notification-channels/zeno-telegram', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({ enabled: false }),
    })
    const calls = fetchMock.mock.calls as unknown as Array<[RequestInfo | URL, RequestInit?]>
    expect(String(calls[0]?.[0])).not.toContain('telegram-bot-secret')
  })

  it('tests a notification channel with the admin token and returns a sanitized delivery', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      delivery: {
        id: 9,
        event_type: 'test_notification',
        label: '测试发送',
        node_id: 'admin-test',
        node_name: 'Zeno',
        previous_status: 'test',
        status: 'test',
        channel_id: 'zeno-telegram',
        channel_name: 'Zeno Telegram',
        success: true,
        created_at: '2026-07-03T00:10:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const delivery = await testAdminNotificationChannel('admin-pass', 'zeno-telegram')

    expect(delivery.eventType).toBe('test_notification')
    expect(delivery.label).toBe('测试发送')
    expect(delivery.success).toBe(true)
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/notification-channels/zeno-telegram/test', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })

  it('omits a blank notification credential on channel updates to preserve the write-only credential', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      channel: {
        id: 'zeno-telegram',
        name: 'Zeno Telegram Updated',
          destination: '7579942307',
        credential_set: true,
        enabled: true,
        created_at: '2026-07-03T00:00:00Z',
        updated_at: '2026-07-03T00:20:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await updateAdminNotificationChannel('admin-pass', 'zeno-telegram', {
      name: 'Zeno Telegram Updated',
      destination: '7579942307',
      credential: '   ',
      enabled: true,
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/notification-channels/zeno-telegram', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        name: 'Zeno Telegram Updated',
          destination: '7579942307',
        enabled: true,
      }),
    })
  })

  it('deletes notification channels with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await deleteAdminNotificationChannel('admin-pass', 'zeno-telegram')

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/notification-channels/zeno-telegram', {
      method: 'DELETE',
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})
