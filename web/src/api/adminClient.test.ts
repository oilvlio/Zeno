import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAdminNode, createAdminNotificationChannel, createAdminProbeTarget, deleteAdminNode, deleteAdminNotificationChannel, deleteAdminProbeTarget, fetchAdminAccount, fetchAdminAlertRules, fetchAdminNodes, fetchAdminNotificationChannels, fetchAdminProbeTargets, fetchAdminSettings, fetchPublicSettings, fetchServiceLatency, loginAdmin, logoutAdmin, normalizeAdminAlertRules, normalizeAdminNodes, normalizeAdminNotificationChannels, normalizeAdminProbeTargets, normalizeSettings, normalizeNodeLatency, normalizeNodeState, normalizeServiceLatency, normalizeSummary, requestAdminNodeInstallCommand, subscribeNodeLatency, subscribeNodeState, subscribeServiceLatency, subscribeSummary, testAdminNotificationChannel, updateAdminAccount, updateAdminAlertRule, updateAdminNode, updateAdminNotificationChannel, updateAdminProbeTarget, updateAdminSettings } from './client'
import { adminCookieSessionMarker } from '../lib/adminToken'

describe('normalizeAdminNodes', () => {
  it('maps authenticated admin node inventory without requiring token fields', () => {
    const data = normalizeAdminNodes({
      nodes: [
        {
          id: 'example-node-a',
          display_name: 'Example Node A',
          status: 'online',
          country_code: 'HK',
          region: 'Hong Kong',
          disabled: false,
          billing_mode: 'both',
          monthly_reset_day: 15,
          expiry_date: '2026-08-01',
          billing_cycle: '月付',
          renewal_amount: 20,
          renewal_currency: 'USD',
          display_order: 10,
          public_ipv4: '198.51.100.8',
          public_ipv6: '2001:db8::8',
          monthly_quota_bytes: 1099511627776,
          last_seen_at: '2026-07-03T00:00:00Z',
          created_at: '2026-07-02T00:00:00Z',
          updated_at: '2026-07-03T00:00:00Z',
          hostname: 'example-node-a-real',
          os_name: 'debian',
          os_version: '13',
          kernel: '6.12.0',
          arch: 'x86_64',
          virtualization: 'kvm',
          cpu_model: 'AMD EPYC',
          cpu_cores: 2,
          memory_total_bytes: 2147483648,
          disk_total_bytes: 42949672960,
          boot_time: '2026-07-02T01:00:00Z',
          agent_version: 'd206817',
        },
      ],
    })

    expect(data.nodes[0].id).toBe('example-node-a')
    expect(data.nodes[0].displayName).toBe('Example Node A')
    expect(data.nodes[0].disabled).toBe(false)
    expect(data.nodes[0].agentVersion).toBe('d206817')
    expect(data.nodes[0].expiryDate).toBe('2026-08-01')
    expect(data.nodes[0].billingCycle).toBe('月付')
    expect(data.nodes[0].renewalAmount).toBe(20)
    expect(data.nodes[0].renewalCurrency).toBe('USD')
    expect(data.nodes[0].monthlyResetDay).toBe(15)
    expect(data.nodes[0].displayOrder).toBe(10)
    expect(data.nodes[0].publicIPv4).toBe('198.51.100.8')
    expect(data.nodes[0].publicIPv6).toBe('2001:db8::8')
    expect(data.nodes[0].monthlyQuotaBytes).toBe(1099511627776)
  })

  it('normalizes null node collections from fresh controller installs', () => {
    const data = normalizeAdminNodes({ nodes: null })

    expect(data.nodes).toEqual([])
  })
})

describe('normalizeAdminProbeTargets', () => {
  it('maps authenticated probe target inventory and node assignments', () => {
    const data = normalizeAdminProbeTargets({
      targets: [
        {
          id: 'example-node-a-local',
          name: 'Example Node A',
          type: 'tcping',
          address: '127.0.0.1',
          port: 18980,
          count: 3,
          timeout_ms: 1200,
          interval_sec: 60,
          display_order: 20,
          assignments: [
            { node_id: 'example-node-a', node_display_name: 'Example Node A', enabled: true },
          ],
        },
      ],
    })

    expect(data.targets[0].id).toBe('example-node-a-local')
    expect(data.targets[0].timeoutMs).toBe(1200)
    expect(data.targets[0].intervalSec).toBe(60)
    expect(data.targets[0].displayOrder).toBe(20)
    expect(data.targets[0].assignments[0].nodeDisplayName).toBe('Example Node A')
  })

  it('normalizes HTTP GET targets without a port', () => {
    const data = normalizeAdminProbeTargets({
      targets: [
        {
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
      ],
    })

    expect(data.targets[0].type).toBe('http_get')
    expect(data.targets[0].port).toBeNull()
    expect(data.targets[0].address).toBe('https://example.com/health')
  })

  it('normalizes null assignment lists to an empty array', () => {
    const data = normalizeAdminProbeTargets({
      targets: [
        {
          id: 'orphan-target',
          name: 'Orphan Target',
          type: 'tcping',
          address: 'example.com',
          port: 443,
          count: 3,
          timeout_ms: 1200,
          interval_sec: 60,
          display_order: 40,
          assignments: null as never,
        },
      ],
    })

    expect(data.targets[0].assignments).toEqual([])
  })
})

describe('normalizeAdminNotifications', () => {
  it('maps channels with write-only credential state for admin forms', () => {
    const channels = normalizeAdminNotificationChannels({
      channels: [
        {
          id: 'zeno-telegram',
          name: 'Zeno Telegram',
          destination: '7579942307',
          credential_set: true,
          enabled: false,
          created_at: '2026-07-03T00:00:00Z',
          updated_at: '2026-07-03T00:00:00Z',
        },
      ],
    })

    expect(channels.channels[0].credentialSet).toBe(true)
    expect(channels.channels[0]).not.toHaveProperty('credential')
  })
})

describe('fetchAdminNodes', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('sends the admin token in X-Admin-Token and never in the URL', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ nodes: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await fetchAdminNodes('admin-pass')

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/nodes', {
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})

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

describe('admin auth client', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('uses the HttpOnly cookie marker and CSRF header for browser auth without retaining response tokens', async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const textUrl = String(url)
      if (textUrl.endsWith('/login')) return new Response(JSON.stringify({ username: 'admin' }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      if (textUrl.endsWith('/account') && !init?.method) return new Response(JSON.stringify({ account: { username: 'admin' } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      if (textUrl.endsWith('/account')) return new Response(JSON.stringify({ username: 'zeno-admin' }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      return new Response(null, { status: 204 })
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await expect(loginAdmin('admin', 'admin-pass')).resolves.toEqual({ username: 'admin', token: adminCookieSessionMarker })
    await expect(fetchAdminAccount(adminCookieSessionMarker)).resolves.toEqual({ username: 'admin' })
    await expect(updateAdminAccount(adminCookieSessionMarker, 'zeno-admin', 'admin-pass', 'new-admin-pass')).resolves.toEqual({ username: 'zeno-admin', token: adminCookieSessionMarker })
    await expect(logoutAdmin(adminCookieSessionMarker)).resolves.toBeUndefined()

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/v1/login', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-Zeno-CSRF': '1' },
      body: JSON.stringify({ username: 'admin', password: 'admin-pass' }),
    })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/v1/account', {
      headers: { Accept: 'application/json', 'X-Zeno-CSRF': '1' },
    })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/v1/account', {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-Zeno-CSRF': '1' },
      body: JSON.stringify({ username: 'zeno-admin', current_password: 'admin-pass', new_password: 'new-admin-pass' }),
    })
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/admin/v1/logout', {
      method: 'POST',
      headers: { 'X-Zeno-CSRF': '1' },
    })
  })

  it('rejects failed logout responses instead of reporting a local logout success', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 500 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await expect(logoutAdmin('account-session-token')).rejects.toThrow('admin logout failed: 500')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/logout', {
      method: 'POST',
      headers: { 'X-Admin-Token': 'account-session-token' },
    })
  })

  it('surfaces logout 401s so the UI can run the expired-session cleanup path', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 401 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await expect(logoutAdmin('expired-session-token')).rejects.toThrow('admin logout failed: 401')
  })
})

describe('fetchSettings', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('fetches public settings without admin credentials', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      site_title: '水饺监控',
      site_subtitle: 'VPS 状态总览',
      logo_url: '/assets/logo/custom.png',
      theme: 'dark',
      agent_controller_url: 'https://zeno.example.com',
      background_url: 'https://example.com/desktop-bg.webp',
      desktop_background_url: 'https://example.com/desktop-bg.webp',
      mobile_background_url: 'https://example.com/mobile-bg.webp',
      appearance_preset: 'gaussian_blur',
      card_opacity: 0.58,
      card_blur: 18,
      card_radius: 24,
      border_strength: 0.34,
      shadow_strength: 0.34,
      background_overlay: 0.08,
      theme_color: '#6366f1',
      custom_code: '<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>',
      updated_at: '2026-07-04T12:00:00Z',
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const settings = await fetchPublicSettings()

    expect(settings.siteTitle).toBe('水饺监控')
    expect(settings.logoUrl).toBe('/assets/logo/custom.png')
    expect(settings).not.toHaveProperty('avatarUrl')
    expect(settings.desktopBackgroundUrl).toBe('https://example.com/desktop-bg.webp')
    expect(settings.mobileBackgroundUrl).toBe('https://example.com/mobile-bg.webp')
    expect(settings.appearancePreset).toBe('gaussian_blur')
    expect(settings.cardBlur).toBe(18)
    expect(settings.themeColor).toBe('#6366f1')
    expect(settings.customCode).toBe('<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>')
    expect(fetchMock).toHaveBeenCalledWith('/api/public/v1/settings', {
      headers: { Accept: 'application/json' },
    })
  })

  it('fetches and updates admin settings with X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request) => new Response(JSON.stringify({
      settings: {
        site_title: String(url).includes('admin') ? '水饺监控' : 'Zeno',
        site_subtitle: 'VPS 状态总览',
        logo_url: '/assets/logo/custom.png',
        theme: 'dark',
        agent_controller_url: 'https://zeno.example.com',
        background_url: 'https://example.com/desktop-bg.webp',
        desktop_background_url: 'https://example.com/desktop-bg.webp',
        mobile_background_url: 'https://example.com/mobile-bg.webp',
        appearance_preset: 'gaussian_blur',
        card_opacity: 0.58,
        card_blur: 18,
        card_radius: 24,
        border_strength: 0.34,
        shadow_strength: 0.34,
        background_overlay: 0.08,
        theme_color: '#6366f1',
        custom_code: '<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>',
        updated_at: '2026-07-04T12:00:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await fetchAdminSettings('admin-pass')
    const settings = await updateAdminSettings('admin-pass', {
      siteTitle: '水饺监控',
      siteSubtitle: 'VPS 状态总览',
      logoUrl: '/assets/logo/custom.png',
      theme: 'dark',
      agentControllerUrl: 'https://zeno.example.com',
      backgroundUrl: 'https://example.com/desktop-bg.webp',
      desktopBackgroundUrl: 'https://example.com/desktop-bg.webp',
      mobileBackgroundUrl: 'https://example.com/mobile-bg.webp',
      appearancePreset: 'gaussian_blur',
      cardOpacity: 0.58,
      cardBlur: 18,
      cardRadius: 24,
      borderStrength: 0.34,
      shadowStrength: 0.34,
      backgroundOverlay: 0.08,
      themeColor: '#6366f1',
      customCode: '<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>',
    })

    expect(settings.backgroundUrl).toBe('https://example.com/desktop-bg.webp')
    expect(settings.logoUrl).toBe('/assets/logo/custom.png')
    expect(settings.agentControllerUrl).toBe('https://zeno.example.com')
    expect(settings).not.toHaveProperty('avatarUrl')
    expect(settings.desktopBackgroundUrl).toBe('https://example.com/desktop-bg.webp')
    expect(settings.mobileBackgroundUrl).toBe('https://example.com/mobile-bg.webp')
    expect(settings.appearancePreset).toBe('gaussian_blur')
    expect(settings.cardOpacity).toBe(0.58)
    expect(settings.cardBlur).toBe(18)
    expect(settings.cardRadius).toBe(24)
    expect(settings.borderStrength).toBe(0.34)
    expect(settings.shadowStrength).toBe(0.34)
    expect(settings.backgroundOverlay).toBe(0.08)
    expect(settings.themeColor).toBe('#6366f1')
    expect(settings.customCode).toBe('<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/v1/settings', {
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/v1/settings', {
      method: 'PATCH',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        site_title: '水饺监控',
        site_subtitle: 'VPS 状态总览',
        logo_url: '/assets/logo/custom.png',
        theme: 'dark',
        agent_controller_url: 'https://zeno.example.com',
        background_url: 'https://example.com/desktop-bg.webp',
        desktop_background_url: 'https://example.com/desktop-bg.webp',
        mobile_background_url: 'https://example.com/mobile-bg.webp',
        appearance_preset: 'gaussian_blur',
        card_opacity: 0.58,
        card_blur: 18,
        card_radius: 24,
        border_strength: 0.34,
        shadow_strength: 0.34,
        background_overlay: 0.08,
        theme_color: '#6366f1',
        custom_code: '<style>.home-top-card { border-color: #2563eb; }</style><script>window.ZenoCustomLoaded = true;</script>',
      }),
    })
  })
})

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

describe('updateAdminNode', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('patches editable node fields with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      node: {
        id: 'example-node-a',
        display_name: 'Example Node A Edited',
        status: 'disabled',
        country_code: 'HK',
        region: 'Hong Kong',
        home_probe_target_id: 'cloudflare',
        disabled: true,
        billing_mode: 'max',
        monthly_reset_day: 20,
        expiry_date: '2026-08-01',
        billing_cycle: '月付',
        display_order: 10,
        public_ipv4: '198.51.100.8',
        public_ipv6: '2001:db8::8',
        monthly_quota_bytes: 123456789,
        created_at: '2026-07-02T00:00:00Z',
        updated_at: '2026-07-03T00:00:00Z',
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const controller = new AbortController()
    const node = await updateAdminNode('admin-pass', 'example-node-a', {
      displayName: 'Example Node A Edited',
      countryCode: 'HK',
      region: 'Hong Kong',
      homeProbeTargetId: 'cloudflare',
      expiryDate: '2026-08-01',
      billingCycle: '月付',
      renewalAmount: 20,
      renewalCurrency: 'USD',
      billingMode: 'max',
      monthlyResetDay: 20,
      displayOrder: 10,
      publicIPv4: '198.51.100.8',
      publicIPv6: '2001:db8::8',
      monthlyQuotaBytes: 123456789,
      disabled: true,
      probeTargetIds: ['cloudflare', 'google'],
    }, controller.signal)

    expect(node.displayName).toBe('Example Node A Edited')
    expect(node.disabled).toBe(true)
    expect(node.monthlyQuotaBytes).toBe(123456789)
    expect(node.homeProbeTargetId).toBe('cloudflare')
    expect(node.expiryDate).toBe('2026-08-01')
    expect(node.billingCycle).toBe('月付')
    expect(node.billingMode).toBe('max')
    expect(node.monthlyResetDay).toBe(20)
    expect(node.displayOrder).toBe(10)
    expect(node.publicIPv4).toBe('198.51.100.8')
    expect(node.publicIPv6).toBe('2001:db8::8')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/nodes/example-node-a', {
      method: 'PATCH',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        display_name: 'Example Node A Edited',
        country_code: 'HK',
        region: 'Hong Kong',
        home_probe_target_id: 'cloudflare',
        expiry_date: '2026-08-01',
        billing_cycle: '月付',
        renewal_amount: 20,
        renewal_currency: 'USD',
        billing_mode: 'max',
        monthly_reset_day: 20,
        display_order: 10,
        public_ipv4: '198.51.100.8',
        public_ipv6: '2001:db8::8',
        monthly_quota_bytes: 123456789,
        disabled: true,
        probe_target_ids: ['cloudflare', 'google'],
      }),
    })
  })
})

describe('deleteAdminNode', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('deletes a node with the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await deleteAdminNode('admin-pass', 'example-node-a')

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/nodes/example-node-a', {
      method: 'DELETE',
      headers: {
        Accept: 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
    })
  })
})

describe('createAdminNode', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('creates a backend-first node with editable fields and the admin token in X-Admin-Token only', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      node: {
        id: 'new-server-a1b2c3d4',
        display_name: 'New Server',
        status: 'no_data',
        country_code: 'US',
        region: 'Los Angeles',
        disabled: false,
        billing_mode: 'out',
        monthly_reset_day: 10,
        expiry_date: '2026-09-01',
        billing_cycle: '月付',
        display_order: 20,
        public_ipv4: '203.0.113.20',
        public_ipv6: '2001:db8::20',
        monthly_quota_bytes: 1099511627776,
        created_at: '2026-07-03T00:00:00Z',
        updated_at: '2026-07-03T00:00:00Z',
      },
    }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const node = await createAdminNode('admin-pass', {
      displayName: 'New Server',
      countryCode: 'US',
      region: 'Los Angeles',
      expiryDate: '2026-09-01',
      billingCycle: '月付',
      renewalAmount: 88,
      renewalCurrency: 'CNY',
      billingMode: 'out',
      monthlyResetDay: 10,
      displayOrder: 20,
      publicIPv4: '203.0.113.20',
      publicIPv6: '2001:db8::20',
      monthlyQuotaBytes: 1099511627776,
    })

    expect(node.id).toBe('new-server-a1b2c3d4')
    expect(node.status).toBe('no_data')
    expect(node.displayOrder).toBe(20)
    expect(node.publicIPv4).toBe('203.0.113.20')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/nodes', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({
        display_name: 'New Server',
        country_code: 'US',
        region: 'Los Angeles',
        expiry_date: '2026-09-01',
        billing_cycle: '月付',
        renewal_amount: 88,
        renewal_currency: 'CNY',
        billing_mode: 'out',
        monthly_reset_day: 10,
        display_order: 20,
        public_ipv4: '203.0.113.20',
        public_ipv6: '2001:db8::20',
        monthly_quota_bytes: 1099511627776,
      }),
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

describe('requestAdminNodeInstallCommand', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('requests an install command from the node edit context without putting the admin token in the URL', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      node_id: 'example-node-a',
      command: "ZENO_INSTALL_URL='https://zeno.shuijiao.de/agent/install.sh' ZENO_CONTROLLER_URL='https://probe.example.com' ZENO_NODE_ID='example-node-a' bash -o pipefail -c 'curl -fsSL \"$ZENO_INSTALL_URL\" | sudo env ZENO_CONTROLLER_URL=\"$ZENO_CONTROLLER_URL\" ZENO_NODE_ID=\"$ZENO_NODE_ID\" bash'",
      commands: {
        linux: "ZENO_INSTALL_URL='https://zeno.shuijiao.de/agent/install.sh' ZENO_CONTROLLER_URL='https://probe.example.com' ZENO_NODE_ID='example-node-a' bash -o pipefail -c 'curl -fsSL \"$ZENO_INSTALL_URL\" | sudo env ZENO_CONTROLLER_URL=\"$ZENO_CONTROLLER_URL\" ZENO_NODE_ID=\"$ZENO_NODE_ID\" bash'",
        macos: "ZENO_INSTALL_URL='https://zeno.shuijiao.de/agent/install.sh' ZENO_CONTROLLER_URL='https://probe.example.com' ZENO_NODE_ID='example-node-a' bash -o pipefail -c 'curl -fsSL \"$ZENO_INSTALL_URL\" | sudo env ZENO_CONTROLLER_URL=\"$ZENO_CONTROLLER_URL\" ZENO_NODE_ID=\"$ZENO_NODE_ID\" bash'",
        windows: "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$env:ZENO_CONTROLLER_URL='https://probe.example.com'; irm 'https://zeno.shuijiao.de/agent/install.ps1' | iex\"",
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const result = await requestAdminNodeInstallCommand('admin-pass', 'example-node-a', 'https://probe.example.com')

    expect(result.nodeId).toBe('example-node-a')
    expect(result.command).toContain('zeno.shuijiao.de/agent/install.sh')
    expect(result.command).toContain('bash -o pipefail')
    expect(result.commands.windows).toContain('zeno.shuijiao.de/agent/install.ps1')
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/nodes/example-node-a/install-command', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-Admin-Token': 'admin-pass',
      },
      body: JSON.stringify({ controller_url: 'https://probe.example.com' }),
    })
  })

  it('explains how to fix a missing Agent controller URL without exposing a raw 409', async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({
      error: 'configure agent controller url before generating install commands',
    }), { status: 409, headers: { 'Content-Type': 'application/json' } })) as unknown as typeof fetch

    await expect(requestAdminNodeInstallCommand('admin-pass', 'mechrevo')).rejects.toThrow(
      '当前后台访问地址无法用于 Agent 接入，请在系统设置中填写 Agent 可访问的接入地址。',
    )
  })
})
