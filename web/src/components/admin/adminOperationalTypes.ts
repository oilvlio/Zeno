import type {
  AdminAlertRuleUpdateInput,
  AdminNodeCreateInput,
  AdminNodeUpdateInput,
  AdminNotificationChannelCreateInput,
  AdminNotificationChannelUpdateInput,
  AdminProbeTargetInput,
  AdminProbeTargetUpdateInput,
} from '../../api/adminClient'
import type { AdminAlertRule, AdminNode, AdminNodeInstallCommand, AdminNotificationChannel, AdminProbeTarget } from '../../types'

export type MaybePromise<T = void> = T | Promise<T>
export type AdminOperationalSection = 'nodes' | 'targets' | 'notifications'

export interface AdminNodeWorkspaceProps {
  nodes: AdminNode[]
  targets: AdminProbeTarget[]
  onCreate: (input: AdminNodeCreateInput) => Promise<AdminNode | void>
  onUpdate: (nodeId: string, input: AdminNodeUpdateInput) => MaybePromise
  onDelete: (nodeId: string) => MaybePromise
  onInstallCommand: (nodeId: string) => Promise<AdminNodeInstallCommand>
}

export interface AdminTargetWorkspaceProps {
  targets: AdminProbeTarget[]
  nodes: AdminNode[]
  onCreate: (input: AdminProbeTargetInput) => MaybePromise
  onUpdate: (targetId: string, input: AdminProbeTargetUpdateInput) => MaybePromise
  onDelete: (targetId: string) => MaybePromise
}

export interface AdminNotificationsWorkspaceProps {
  channels: AdminNotificationChannel[]
  rules: AdminAlertRule[]
  nodes: AdminNode[]
  onChannelCreate: (input: AdminNotificationChannelCreateInput) => MaybePromise
  onChannelUpdate: (channelId: string, input: AdminNotificationChannelUpdateInput) => MaybePromise
  onChannelDelete: (channelId: string) => MaybePromise
  onChannelTest: (channelId: string) => void
  onRuleUpdate: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise
}

export interface AdminOperationalWorkspaceProps {
  activeSection: AdminOperationalSection
  nodes: AdminNode[]
  targets: AdminProbeTarget[]
  notificationChannels: AdminNotificationChannel[]
  alertRules: AdminAlertRule[]
  onNodeCreate: AdminNodeWorkspaceProps['onCreate']
  onNodeUpdate: AdminNodeWorkspaceProps['onUpdate']
  onNodeDelete: AdminNodeWorkspaceProps['onDelete']
  onInstallCommand: AdminNodeWorkspaceProps['onInstallCommand']
  onProbeTargetCreate: AdminTargetWorkspaceProps['onCreate']
  onProbeTargetUpdate: AdminTargetWorkspaceProps['onUpdate']
  onProbeTargetDelete: AdminTargetWorkspaceProps['onDelete']
  onNotificationChannelCreate: AdminNotificationsWorkspaceProps['onChannelCreate']
  onNotificationChannelUpdate: AdminNotificationsWorkspaceProps['onChannelUpdate']
  onNotificationChannelDelete: AdminNotificationsWorkspaceProps['onChannelDelete']
  onNotificationChannelTest: AdminNotificationsWorkspaceProps['onChannelTest']
  onAlertRuleUpdate: AdminNotificationsWorkspaceProps['onRuleUpdate']
}
