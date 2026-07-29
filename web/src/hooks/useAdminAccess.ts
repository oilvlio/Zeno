import { useCallback, useState } from 'react'
import { clearStoredAdminTokenIfCurrent, loadStoredAdminToken, type AdminTokenIdentity } from '../lib/adminToken'

export function useAdminAccess() {
  const [adminToken, setAdminToken] = useState(loadStoredAdminToken)
  const expireAdminSession = useCallback((identity: AdminTokenIdentity): boolean => {
    if (!clearStoredAdminTokenIfCurrent(identity)) return false
    setAdminToken((current) => current === identity.token ? '' : current)
    return true
  }, [])

  return { adminToken, setAdminToken, expireAdminSession }
}
