import { describe, expect, it } from 'vitest'
import { adminStateReducer, type AdminStateAction } from './adminStateReducer'
import { alertRules, backupNode, exampleNodeANode, exampleNodeATarget, pingTarget, telegramChannel } from '../components/admin/adminTestFixtures'
import type { AdminLoadState } from './adminModel'

function readyState(): Extract<AdminLoadState, { kind: 'ready' }> {
  return {
    kind: 'ready',
    account: { username: 'admin' },
    nodes: [exampleNodeANode, backupNode],
    targets: [exampleNodeATarget, pingTarget],
    notificationChannels: [telegramChannel],
    alertRules,
  }
}

describe('adminStateReducer', () => {
  it('updates a node and its target assignments without mutating the previous snapshot', () => {
    const previous = readyState()
    const updatedNode = { ...exampleNodeANode, displayName: 'Renamed', displayOrder: 99 }
    const next = adminStateReducer(previous, { type: 'node.updated', node: updatedNode, probeTargetIds: ['google-icmp'] })
    expect(next.kind).toBe('ready')
    if (next.kind !== 'ready') return
    expect(previous.nodes[0].displayName).toBe('Example Node A')
    expect(next.nodes.find((node) => node.id === updatedNode.id)?.displayName).toBe('Renamed')
    expect(next.targets.find((target) => target.id === 'google-icmp')?.assignments.find((assignment) => assignment.nodeId === updatedNode.id)).toMatchObject({ enabled: true, nodeDisplayName: 'Renamed' })
    expect(next.targets.find((target) => target.id === 'example-node-a-local')?.assignments.find((assignment) => assignment.nodeId === updatedNode.id)).toMatchObject({ enabled: false, nodeDisplayName: 'Renamed' })
  })

  it('composes independent concurrent mutation results without dropping prior fields', () => {
    const previous = readyState()
    const withNode = adminStateReducer(previous, { type: 'node.created', node: { ...exampleNodeANode, id: 'third', displayName: 'Third' } })
    const next = adminStateReducer(withNode, { type: 'channel.updated', channel: { ...telegramChannel, name: 'Renamed channel' } })
    expect(next.kind).toBe('ready')
    if (next.kind !== 'ready') return
    expect(next.nodes.some((node) => node.id === 'third')).toBe(true)
    expect(next.notificationChannels[0].name).toBe('Renamed channel')
    expect(previous.notificationChannels[0].name).toBe('Zeno Telegram')
  })

  it('removes deleted nodes from inventories and every target assignment', () => {
    const next = adminStateReducer(readyState(), { type: 'node.deleted', nodeId: 'example-node-a' })
    expect(next.kind).toBe('ready')
    if (next.kind !== 'ready') return
    expect(next.nodes.some((node) => node.id === 'example-node-a')).toBe(false)
    expect(next.targets.every((target) => target.assignments.every((assignment) => assignment.nodeId !== 'example-node-a'))).toBe(true)
  })

  it('keeps non-ready state unchanged when any stale mutation result arrives', () => {
    const loading: AdminLoadState = { kind: 'loading' }
    const staleActions: AdminStateAction[] = [
      { type: 'account.updated', username: 'stale' },
      { type: 'node.created', node: exampleNodeANode },
      { type: 'node.updated', node: exampleNodeANode },
      { type: 'node.deleted', nodeId: exampleNodeANode.id },
      { type: 'target.created', target: pingTarget },
      { type: 'target.updated', target: pingTarget },
      { type: 'target.deleted', targetId: pingTarget.id },
      { type: 'channel.created', channel: telegramChannel },
      { type: 'channel.updated', channel: telegramChannel },
      { type: 'channel.deleted', channelId: telegramChannel.id },
      { type: 'rule.updated', rule: alertRules[0] },
    ]
    for (const action of staleActions) expect(adminStateReducer(loading, action)).toBe(loading)
  })
})
