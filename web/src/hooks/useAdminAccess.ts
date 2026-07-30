import { useCallback, useEffect, useState } from 'react'
import { adminCookieSessionMarker, clearStoredAdminTokenIfCurrent, loadStoredAdminToken, rememberAdminToken, type AdminTokenIdentity } from '../lib/adminToken'

export function useAdminAccess() {
  const [adminToken, setAdminToken] = useState(loadStoredAdminToken)

  useEffect(() => {
    if (adminToken !== '') return undefined
    let active = true
    // Probe the HttpOnly cookie while the public dashboard is already visible.
    // A returning administrator can then enter the dashboard without paying an
    // extra authentication round trip after navigation. The API module remains
    // asynchronous, so it does not grow the public entry bundle.
    void import('../api/adminSession')
      .then(({ probeAdminCookieSession }) => probeAdminCookieSession())
      .then((authenticated) => {
        if (!active || !authenticated) return
        rememberAdminToken()
        setAdminToken(adminCookieSessionMarker)
      })
      .catch(() => {})
    return () => { active = false }
  }, [adminToken])

  const expireAdminSession = useCallback((identity: AdminTokenIdentity): boolean => {
    if (!clearStoredAdminTokenIfCurrent(identity)) return false
    setAdminToken((current) => current === identity.token ? '' : current)
    return true
  }, [])

  return { adminToken, setAdminToken, expireAdminSession }
}
