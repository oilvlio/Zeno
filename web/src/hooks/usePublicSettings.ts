import { useEffect, useState } from 'react'
import { fetchPublicSettings } from '../api/publicClient'
import { defaultSettings } from '../lib/appearance'
import type { AdminSettings } from '../types'

export function usePublicSettings() {
  const [settings, setSettings] = useState<AdminSettings>(defaultSettings)
  const [settingsReady, setSettingsReady] = useState(false)

  useEffect(() => {
    let cancelled = false
    fetchPublicSettings()
      .then((loadedSettings) => {
        if (!cancelled) setSettings(loadedSettings)
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setSettingsReady(true)
      })
    return () => { cancelled = true }
  }, [])

  return { settings, settingsReady, setSettings }
}
