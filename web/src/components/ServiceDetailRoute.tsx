import type { ReactNode } from 'react'
import { useServiceDetailController } from '../hooks/useServiceDetailController'
import type { AdminTokenIdentity } from '../lib/adminToken'
import type { ServiceTarget } from '../types'
import { ServiceDetail } from './ServiceDetail'
import '../styles/detail.css'

interface ServiceDetailRouteProps {
  targetId: string
  target?: ServiceTarget
  adminToken: string
  expireAdminSession: (identity: AdminTokenIdentity) => boolean
  onBack: () => void
  topHeader: ReactNode
  loadingFallback: ReactNode
  notFoundFallback: ReactNode
}

export function ServiceDetailRoute({ targetId, target, adminToken, expireAdminSession, onBack, topHeader, loadingFallback, notFoundFallback }: ServiceDetailRouteProps) {
  const { serviceLatencyRange, serviceLatencyState, setServiceLatencyRange } = useServiceDetailController({
    targetId,
    adminToken,
    expireAdminSession,
  })
  const resolvedTarget = serviceLatencyState.kind === 'ready' ? serviceLatencyState.data.target : target
  if (!resolvedTarget) return serviceLatencyState.kind === 'error' ? notFoundFallback : loadingFallback

  return (
    <ServiceDetail
      target={resolvedTarget}
      points={serviceLatencyState.kind === 'ready' ? serviceLatencyState.data.points : []}
      range={serviceLatencyRange}
      loading={serviceLatencyState.kind === 'loading'}
      error={serviceLatencyState.kind === 'error' ? serviceLatencyState.message : undefined}
      canUseExtendedRanges={adminToken !== ''}
      onBack={onBack}
      onRangeChange={setServiceLatencyRange}
      topHeader={topHeader}
    />
  )
}
