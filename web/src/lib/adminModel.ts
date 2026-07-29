import type { AdminAccountData } from '../api/adminSession'
import type { AdminAlertRule, AdminNode, AdminNotificationChannel, AdminProbeTarget } from '../types'

export type AdminLoadState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; account: AdminAccountData; nodes: AdminNode[]; targets: AdminProbeTarget[]; notificationChannels: AdminNotificationChannel[]; alertRules: AdminAlertRule[] }
  | { kind: 'error'; message: string }

export type AdminAuthState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
