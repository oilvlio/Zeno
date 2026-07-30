import type { ComponentType } from 'react'
import AdminNodeWorkspace from './AdminNodeWorkspace'
import AdminNotificationsWorkspace from './AdminNotificationsWorkspace'
import AdminTargetWorkspace from './AdminTargetWorkspace'
import type { AdminNodeWorkspaceProps, AdminNotificationsWorkspaceProps, AdminOperationalWorkspaceProps, AdminTargetWorkspaceProps } from './adminOperationalTypes'

export type AdminOperationalSectionComponents = {
  nodes: ComponentType<AdminNodeWorkspaceProps>
  targets: ComponentType<AdminTargetWorkspaceProps>
  notifications: ComponentType<AdminNotificationsWorkspaceProps>
}

export interface AdminOperationalWorkspaceRouterProps extends AdminOperationalWorkspaceProps {
  sectionComponents?: Partial<AdminOperationalSectionComponents>
}

export function AdminOperationalWorkspace({
  activeSection,
  nodes,
  targets,
  notificationChannels,
  alertRules,
  onNodeCreate,
  onNodeUpdate,
  onNodeDelete,
  onInstallCommand,
  onProbeTargetCreate,
  onProbeTargetUpdate,
  onProbeTargetDelete,
  onNotificationChannelCreate,
  onNotificationChannelUpdate,
  onNotificationChannelDelete,
  onNotificationChannelTest,
  onAlertRuleUpdate,
  sectionComponents,
}: AdminOperationalWorkspaceRouterProps) {
  if (activeSection === 'nodes') {
    const NodeWorkspace = sectionComponents?.nodes ?? AdminNodeWorkspace
    return <NodeWorkspace nodes={nodes} targets={targets} onCreate={onNodeCreate} onUpdate={onNodeUpdate} onDelete={onNodeDelete} onInstallCommand={onInstallCommand} />
  }
  if (activeSection === 'targets') {
    const TargetWorkspace = sectionComponents?.targets ?? AdminTargetWorkspace
    return <TargetWorkspace targets={targets} nodes={nodes} onCreate={onProbeTargetCreate} onUpdate={onProbeTargetUpdate} onDelete={onProbeTargetDelete} />
  }
  const NotificationsWorkspace = sectionComponents?.notifications ?? AdminNotificationsWorkspace
  return <NotificationsWorkspace channels={notificationChannels} rules={alertRules} nodes={nodes} onChannelCreate={onNotificationChannelCreate} onChannelUpdate={onNotificationChannelUpdate} onChannelDelete={onNotificationChannelDelete} onChannelTest={onNotificationChannelTest} onRuleUpdate={onAlertRuleUpdate} />
}

export type { AdminOperationalSection, AdminOperationalWorkspaceProps } from './adminOperationalTypes'
export { formatTargetAssignmentSummary, renewalCurrencyOptions } from './adminOperationalModel'
export default AdminOperationalWorkspace
