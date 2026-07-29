import type { AdminNode, AdminProbeTarget } from '../types'

export function sortAdminNodes(nodes: AdminNode[]): AdminNode[] {
  return [...nodes].sort((left, right) => left.displayOrder - right.displayOrder || left.id.localeCompare(right.id, 'zh-CN'))
}

export function sortAdminProbeTargets(targets: AdminProbeTarget[]): AdminProbeTarget[] {
  return [...targets].sort((left, right) => left.displayOrder - right.displayOrder || left.id.localeCompare(right.id, 'zh-CN'))
}
