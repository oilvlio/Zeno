import { sortAdminNodes, sortAdminProbeTargets } from './adminCollections'
import type { AdminLoadState } from './adminModel'
import type { AdminAlertRule, AdminNode, AdminNotificationChannel, AdminProbeTarget } from '../types'

export type AdminStateAction =
  | { type: 'state.idle' }
  | { type: 'state.loading' }
  | { type: 'state.error'; message: string }
  | { type: 'nodes.loaded'; nodes: AdminNode[] }
  | { type: 'details.loaded'; account?: { username: string }; targets?: AdminProbeTarget[]; notificationChannels?: AdminNotificationChannel[]; alertRules?: AdminAlertRule[] }
  | { type: 'account.updated'; username: string }
  | { type: 'node.created'; node: AdminNode }
  | { type: 'node.updated'; node: AdminNode; probeTargetIds?: string[] }
  | { type: 'node.deleted'; nodeId: string }
  | { type: 'target.created'; target: AdminProbeTarget }
  | { type: 'target.updated'; target: AdminProbeTarget }
  | { type: 'target.deleted'; targetId: string }
  | { type: 'channel.created'; channel: AdminNotificationChannel }
  | { type: 'channel.updated'; channel: AdminNotificationChannel }
  | { type: 'channel.deleted'; channelId: string }
  | { type: 'rule.updated'; rule: AdminAlertRule }

const emptyReadyState = (): Extract<AdminLoadState, { kind: 'ready' }> => ({
  kind: 'ready',
  account: { username: 'admin' },
  nodes: [],
  targets: [],
  notificationChannels: [],
  alertRules: [],
})

export function adminStateReducer(state: AdminLoadState, action: AdminStateAction): AdminLoadState {
  if (action.type === 'state.idle') return { kind: 'idle' }
  if (action.type === 'state.loading') return { kind: 'loading' }
  if (action.type === 'state.error') return { kind: 'error', message: action.message }
  if (action.type === 'nodes.loaded') {
    const ready = state.kind === 'ready' ? state : emptyReadyState()
    return { ...ready, nodes: action.nodes }
  }
  if (action.type === 'details.loaded') {
    if (state.kind !== 'ready') return state
    return {
      ...state,
      account: action.account ?? state.account,
      targets: action.targets ?? state.targets,
      notificationChannels: action.notificationChannels ?? state.notificationChannels,
      alertRules: action.alertRules ?? state.alertRules,
    }
  }
  if (action.type === 'account.updated') {
    return state.kind === 'ready' ? { ...state, account: { username: action.username } } : state
  }
  if (action.type === 'node.created') {
    if (state.kind !== 'ready') return state
    const ready = state
    return { ...ready, nodes: sortAdminNodes([...ready.nodes, action.node]) }
  }
  if (action.type === 'node.updated') {
    if (state.kind !== 'ready') return state
    const ready = state
    const selectedTargetIds = action.probeTargetIds === undefined ? null : new Set(action.probeTargetIds)
    return {
      ...ready,
      nodes: sortAdminNodes(ready.nodes.some((node) => node.id === action.node.id)
        ? ready.nodes.map((node) => node.id === action.node.id ? action.node : node)
        : [...ready.nodes, action.node]),
      targets: selectedTargetIds === null ? ready.targets : ready.targets.map((target) => {
        const existing = target.assignments.find((assignment) => assignment.nodeId === action.node.id)
        const enabled = selectedTargetIds.has(target.id)
        const nextAssignment = { nodeId: action.node.id, nodeDisplayName: action.node.displayName, enabled }
        return {
          ...target,
          assignments: existing
            ? target.assignments.map((assignment) => assignment.nodeId === action.node.id ? nextAssignment : assignment)
            : enabled ? [...target.assignments, nextAssignment] : target.assignments,
        }
      }),
    }
  }
  if (action.type === 'node.deleted') {
    if (state.kind !== 'ready') return state
    return {
      ...state,
      nodes: state.nodes.filter((node) => node.id !== action.nodeId),
      targets: state.targets.map((target) => ({
        ...target,
        assignments: target.assignments.filter((assignment) => assignment.nodeId !== action.nodeId),
      })),
    }
  }
  if (action.type === 'target.created' || action.type === 'target.updated') {
    if (state.kind !== 'ready') return state
    const ready = state
    const target = action.target
    const targets = action.type === 'target.created' || !ready.targets.some((item) => item.id === target.id)
      ? [...ready.targets, target]
      : ready.targets.map((item) => item.id === target.id ? target : item)
    return { ...ready, targets: sortAdminProbeTargets(targets) }
  }
  if (action.type === 'target.deleted') {
    return state.kind === 'ready' ? { ...state, targets: state.targets.filter((target) => target.id !== action.targetId) } : state
  }
  if (action.type === 'channel.created' || action.type === 'channel.updated') {
    if (state.kind !== 'ready') return state
    const ready = state
    const channel = action.channel
    const channels = action.type === 'channel.created' || !ready.notificationChannels.some((item) => item.id === channel.id)
      ? [...ready.notificationChannels, channel]
      : ready.notificationChannels.map((item) => item.id === channel.id ? channel : item)
    return { ...ready, notificationChannels: channels }
  }
  if (action.type === 'channel.deleted') {
    return state.kind === 'ready' ? { ...state, notificationChannels: state.notificationChannels.filter((channel) => channel.id !== action.channelId) } : state
  }
  if (action.type === 'rule.updated') {
    if (state.kind !== 'ready') return state
    const ready = state
    const rules = ready.alertRules.some((rule) => rule.id === action.rule.id)
      ? ready.alertRules.map((rule) => rule.id === action.rule.id ? action.rule : rule)
      : [...ready.alertRules, action.rule]
    return { ...ready, alertRules: rules }
  }
  return state
}
