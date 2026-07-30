import { describe, expect, it } from 'vitest'
import { normalizeAdminNodes, normalizeAdminNotificationChannels, normalizeAdminProbeTargets } from './client'

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
