import type { AdminAlertRule, AdminNode, AdminNodeInstallCommand, AdminNotificationChannel, AdminNotificationDelivery, AdminProbeTarget, AdminSettings } from '../types'
import { adminHeaders } from './adminSession'
import { normalizeSettings } from './publicNormalizers'
import { normalizeAdminAlertRule, normalizeAdminAlertRules, normalizeAdminNode, normalizeAdminNodes, normalizeAdminNotificationChannel, normalizeAdminNotificationChannels, normalizeAdminNotificationDelivery, normalizeAdminProbeTarget, normalizeAdminProbeTargets, serializeAdminAlertRuleUpdate, serializeAdminNodeCreate, serializeAdminNodeUpdate, serializeAdminNotificationChannelCreate, serializeAdminNotificationChannelUpdate, serializeAdminProbeTargetCreate, serializeAdminProbeTargetUpdate, serializeAdminSettingsUpdate } from './adminNormalizers'
import type { AdminAlertRuleUpdateInput, AdminAlertRulesData, AdminNodeCreateInput, AdminNodesData, AdminNodeUpdateInput, AdminNotificationChannelCreateInput, AdminNotificationChannelsData, AdminNotificationChannelUpdateInput, AdminProbeTargetInput, AdminProbeTargetsData, AdminProbeTargetUpdateInput, AdminSettingsUpdateInput, ApiAdminAlertRuleResponse, ApiAdminAlertRulesResponse, ApiAdminNodeInstallCommandResponse, ApiAdminNodeResponse, ApiAdminNodesResponse, ApiAdminNotificationChannelResponse, ApiAdminNotificationChannelsResponse, ApiAdminNotificationTestResponse, ApiAdminProbeTargetResponse, ApiAdminProbeTargetsResponse, ApiAdminSettingsResponse } from './apiTypes'
export type { AdminAlertRuleUpdateInput, AdminAlertRulesData, AdminNodeCreateInput, AdminNodesData, AdminNodeUpdateInput, AdminNotificationChannelCreateInput, AdminNotificationChannelsData, AdminNotificationChannelUpdateInput, AdminProbeTargetInput, AdminProbeTargetsData, AdminProbeTargetUpdateInput, AdminSettingsUpdateInput, ApiAdminAlertRuleResponse, ApiAdminNodeInstallCommandResponse, ApiAdminNodeResponse, ApiAdminNotificationChannelResponse, ApiAdminNotificationTestResponse, ApiAdminProbeTargetResponse, ApiAdminSettingsResponse, ApiAdminNodesResponse, ApiAdminProbeTargetsResponse, ApiAdminNotificationChannelsResponse, ApiAdminAlertRulesResponse } from './apiTypes'
export { fetchAdminAccount, loginAdmin, logoutAdmin, updateAdminAccount } from './adminSession'
export type { AdminAccountData, AdminLoginData } from './adminSession'
export { fetchPublicSettings } from './publicClient'
export { normalizeAdminAlertRules, normalizeAdminNodes, normalizeAdminNotificationChannels, normalizeAdminProbeTargets } from './adminNormalizers'

export async function fetchAdminSettings(adminToken: string, signal?: AbortSignal): Promise<AdminSettings> {
  const response = await fetch('/api/admin/v1/settings', {
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin settings request failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminSettingsResponse
  return normalizeSettings(data.settings)
}

export class AdminSettingsConflictError extends Error {
  constructor(readonly latestSettings: AdminSettings) {
    super('服务端设置已更新，请载入最新设置。')
    this.name = 'AdminSettingsConflictError'
  }
}

export async function updateAdminSettings(adminToken: string, input: AdminSettingsUpdateInput, signal?: AbortSignal): Promise<AdminSettings> {
  const response = await fetch('/api/admin/v1/settings', {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminSettingsUpdate(input)),
  })
  if (response.status === 409) {
    throw new AdminSettingsConflictError(await fetchAdminSettings(adminToken, signal))
  }
  if (!response.ok) {
    throw new Error(`admin settings update failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminSettingsResponse
  return normalizeSettings(data.settings)
}

export async function fetchAdminNodes(adminToken: string, signal?: AbortSignal): Promise<AdminNodesData> {
  const response = await fetch('/api/admin/v1/nodes', {
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin nodes request failed: ${response.status}`)
  }
  return normalizeAdminNodes(await response.json() as ApiAdminNodesResponse)
}

export async function reorderAdminNodes(adminToken: string, nodeIds: string[], signal?: AbortSignal): Promise<void> {
  const response = await fetch('/api/admin/v1/nodes/reorder', {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify({ node_ids: nodeIds }),
  })
  if (!response.ok) {
    throw new Error(`admin node reorder failed: ${response.status}`)
  }
}

export async function reorderAdminProbeTargets(adminToken: string, targetIds: string[], signal?: AbortSignal): Promise<void> {
  const response = await fetch('/api/admin/v1/probe-targets/reorder', {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify({ target_ids: targetIds }),
  })
  if (!response.ok) {
    throw new Error(`admin probe target reorder failed: ${response.status}`)
  }
}

export async function fetchAdminProbeTargets(adminToken: string, signal?: AbortSignal): Promise<AdminProbeTargetsData> {
  const response = await fetch('/api/admin/v1/probe-targets', {
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin probe targets request failed: ${response.status}`)
  }
  return normalizeAdminProbeTargets(await response.json() as ApiAdminProbeTargetsResponse)
}

export async function fetchAdminNotificationChannels(adminToken: string, signal?: AbortSignal): Promise<AdminNotificationChannelsData> {
  const response = await fetch('/api/admin/v1/notification-channels', {
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin notification channels request failed: ${response.status}`)
  }
  return normalizeAdminNotificationChannels(await response.json() as ApiAdminNotificationChannelsResponse)
}

export async function fetchAdminAlertRules(adminToken: string, signal?: AbortSignal): Promise<AdminAlertRulesData> {
  const response = await fetch('/api/admin/v1/alert-rules', {
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin alert rules request failed: ${response.status}`)
  }
  return normalizeAdminAlertRules(await response.json() as ApiAdminAlertRulesResponse)
}

export async function createAdminNode(adminToken: string, input: AdminNodeCreateInput, signal?: AbortSignal): Promise<AdminNode> {
  const response = await fetch('/api/admin/v1/nodes', {
    method: 'POST',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminNodeCreate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin node create failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNodeResponse
  return normalizeAdminNode(data.node)
}

export async function createAdminProbeTarget(adminToken: string, input: AdminProbeTargetInput, signal?: AbortSignal): Promise<AdminProbeTarget> {
  const response = await fetch('/api/admin/v1/probe-targets', {
    method: 'POST',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminProbeTargetCreate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin probe target create failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminProbeTargetResponse
  return normalizeAdminProbeTarget(data.target)
}

export async function updateAdminProbeTarget(adminToken: string, targetId: string, input: AdminProbeTargetUpdateInput, signal?: AbortSignal): Promise<AdminProbeTarget> {
  const response = await fetch(`/api/admin/v1/probe-targets/${encodeURIComponent(targetId)}`, {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminProbeTargetUpdate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin probe target update failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminProbeTargetResponse
  return normalizeAdminProbeTarget(data.target)
}

export async function deleteAdminProbeTarget(adminToken: string, targetId: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/api/admin/v1/probe-targets/${encodeURIComponent(targetId)}`, {
    method: 'DELETE',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin probe target delete failed: ${response.status}`)
  }
}

export async function createAdminNotificationChannel(adminToken: string, input: AdminNotificationChannelCreateInput, signal?: AbortSignal): Promise<AdminNotificationChannel> {
  const response = await fetch('/api/admin/v1/notification-channels', {
    method: 'POST',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminNotificationChannelCreate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin notification channel create failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNotificationChannelResponse
  return normalizeAdminNotificationChannel(data.channel)
}

export async function updateAdminNotificationChannel(adminToken: string, channelId: string, input: AdminNotificationChannelUpdateInput, signal?: AbortSignal): Promise<AdminNotificationChannel> {
  const response = await fetch(`/api/admin/v1/notification-channels/${encodeURIComponent(channelId)}`, {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminNotificationChannelUpdate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin notification channel update failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNotificationChannelResponse
  return normalizeAdminNotificationChannel(data.channel)
}

export async function deleteAdminNotificationChannel(adminToken: string, channelId: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/api/admin/v1/notification-channels/${encodeURIComponent(channelId)}`, {
    method: 'DELETE',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin notification channel delete failed: ${response.status}`)
  }
}

export async function testAdminNotificationChannel(adminToken: string, channelId: string, signal?: AbortSignal): Promise<AdminNotificationDelivery> {
  const response = await fetch(`/api/admin/v1/notification-channels/${encodeURIComponent(channelId)}/test`, {
    method: 'POST',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin notification channel test failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNotificationTestResponse
  return normalizeAdminNotificationDelivery(data.delivery)
}

export async function updateAdminAlertRule(adminToken: string, ruleId: string, input: AdminAlertRuleUpdateInput, signal?: AbortSignal): Promise<AdminAlertRule> {
  const response = await fetch(`/api/admin/v1/alert-rules/${encodeURIComponent(ruleId)}`, {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminAlertRuleUpdate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin alert rule update failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminAlertRuleResponse
  return normalizeAdminAlertRule(data.rule)
}

export async function requestAdminNodeInstallCommand(adminToken: string, nodeId: string, controllerURL = typeof window === 'undefined' ? '' : window.location.origin, signal?: AbortSignal): Promise<AdminNodeInstallCommand> {
  const response = await fetch(`/api/admin/v1/nodes/${encodeURIComponent(nodeId)}/install-command`, {
    method: 'POST',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify({ controller_url: controllerURL }),
  })
  if (!response.ok) {
    if (response.status === 409) {
      throw new Error('当前后台访问地址无法用于 Agent 接入，请在系统设置中填写 Agent 可访问的接入地址。')
    }
    throw new Error(`admin node install command failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNodeInstallCommandResponse
  return { nodeId: data.node_id, command: data.command, commands: data.commands ?? { linux: data.command } }
}

export async function updateAdminNode(adminToken: string, nodeId: string, input: AdminNodeUpdateInput, signal?: AbortSignal): Promise<AdminNode> {
  const response = await fetch(`/api/admin/v1/nodes/${encodeURIComponent(nodeId)}`, {
    method: 'PATCH',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json', 'Content-Type': 'application/json' }),
    body: JSON.stringify(serializeAdminNodeUpdate(input)),
  })
  if (!response.ok) {
    throw new Error(`admin node update failed: ${response.status}`)
  }
  const data = await response.json() as ApiAdminNodeResponse
  return normalizeAdminNode(data.node)
}

export async function deleteAdminNode(adminToken: string, nodeId: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/api/admin/v1/nodes/${encodeURIComponent(nodeId)}`, {
    method: 'DELETE',
    signal,
    headers: adminHeaders(adminToken, { Accept: 'application/json' }),
  })
  if (!response.ok) {
    throw new Error(`admin node delete failed: ${response.status}`)
  }
}
