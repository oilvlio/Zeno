import { lazy, Suspense, type ComponentType } from 'react'
import type { AdminNodeWorkspaceProps, AdminNotificationsWorkspaceProps, AdminOperationalWorkspaceProps, AdminTargetWorkspaceProps } from './adminOperationalTypes'

const LazyAdminNodeWorkspace = lazy(() => import('./AdminNodeWorkspace'))
const LazyAdminTargetWorkspace = lazy(() => import('./AdminTargetWorkspace'))
const LazyAdminNotificationsWorkspace = lazy(() => import('./AdminNotificationsWorkspace'))

export type AdminOperationalSectionComponents = {
  nodes: ComponentType<AdminNodeWorkspaceProps>
  targets: ComponentType<AdminTargetWorkspaceProps>
  notifications: ComponentType<AdminNotificationsWorkspaceProps>
}

export interface AdminOperationalWorkspaceRouterProps extends AdminOperationalWorkspaceProps {
  sectionComponents?: Partial<AdminOperationalSectionComponents>
}

function AdminWorkspaceLoading() {
  return <div className="admin-state-card">加载中…</div>
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
    const NodeWorkspace = sectionComponents?.nodes ?? LazyAdminNodeWorkspace
    return <Suspense fallback={<AdminWorkspaceLoading />}><NodeWorkspace nodes={nodes} targets={targets} onCreate={onNodeCreate} onUpdate={onNodeUpdate} onDelete={onNodeDelete} onInstallCommand={onInstallCommand} /></Suspense>
  }
  if (activeSection === 'targets') {
    const TargetWorkspace = sectionComponents?.targets ?? LazyAdminTargetWorkspace
    return <Suspense fallback={<AdminWorkspaceLoading />}><TargetWorkspace targets={targets} nodes={nodes} onCreate={onProbeTargetCreate} onUpdate={onProbeTargetUpdate} onDelete={onProbeTargetDelete} /></Suspense>
  }
  const NotificationsWorkspace = sectionComponents?.notifications ?? LazyAdminNotificationsWorkspace
  return <Suspense fallback={<AdminWorkspaceLoading />}><NotificationsWorkspace channels={notificationChannels} rules={alertRules} nodes={nodes} onChannelCreate={onNotificationChannelCreate} onChannelUpdate={onNotificationChannelUpdate} onChannelDelete={onNotificationChannelDelete} onChannelTest={onNotificationChannelTest} onRuleUpdate={onAlertRuleUpdate} /></Suspense>
}

export type { AdminOperationalSection, AdminOperationalWorkspaceProps } from './adminOperationalTypes'
export { formatTargetAssignmentSummary, renewalCurrencyOptions } from './adminOperationalModel'
export default AdminOperationalWorkspace
