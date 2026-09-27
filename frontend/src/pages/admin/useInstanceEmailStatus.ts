import { useCallback, useEffect, useRef, useState } from 'react'

import { onInstanceEmailChanged } from '@/pages/admin/instanceEmailEvents'
import {
  type AdminInstanceEmailSettings,
  adminService,
} from '@/services/adminService'

/** The admin page that configures the instance email provider (#1191). */
export const INSTANCE_EMAIL_SETTINGS_PATH = '/admin/settings/email'

/**
 * Where instance email stands (#1192):
 * - `unconfigured`: nothing stored, so the instance's mail is discarded;
 * - `failing`: configured, and the last send failed;
 * - `healthy`: configured, and the last send succeeded or none was made yet;
 * - `unknown`: the status could not be read (never shown as unconfigured);
 * - `loading`: the first read has not settled yet.
 */
export type InstanceEmailState =
  'loading' | 'healthy' | 'unconfigured' | 'failing' | 'unknown'

/**
 * Maps the settings payload to its state. `is_healthy` is the server's verdict
 * (it compares `last_error_at` with `last_success_at`, and is true for a
 * provider that has never sent), so the timestamps are not compared again here.
 */
export function instanceEmailState(
  settings: AdminInstanceEmailSettings
): Exclude<InstanceEmailState, 'loading' | 'unknown'> {
  if (!settings.configured) return 'unconfigured'
  return settings.is_healthy === false ? 'failing' : 'healthy'
}

export interface InstanceEmailStatus {
  state: InstanceEmailState
  /** The last payload read; null while loading or after a failed read. */
  settings: AdminInstanceEmailSettings | null
}

/**
 * Reads `GET /api/v1/admin/settings/email` and refetches whenever the settings
 * page reports a change. A failed read resolves to `unknown` and never throws,
 * so an admin page never breaks over the mail status.
 */
export function useInstanceEmailStatus(): InstanceEmailStatus {
  const [status, setStatus] = useState<InstanceEmailStatus>({
    state: 'loading',
    settings: null,
  })
  // Each read takes a ticket; only the newest one may set state, so a slow
  // mount read can never overwrite a refetch that a save triggered after it.
  const latest = useRef(0)

  const load = useCallback(async (isActive: () => boolean) => {
    const ticket = ++latest.current
    const current = () => isActive() && ticket === latest.current
    try {
      const settings = await adminService.getInstanceEmailSettings()
      if (current()) {
        setStatus({ state: instanceEmailState(settings), settings })
      }
    } catch (err) {
      console.error('Failed to load the instance email status:', err)
      if (current()) setStatus({ state: 'unknown', settings: null })
    }
  }, [])

  useEffect(() => {
    let active = true
    const isActive = () => active
    void load(isActive)
    const unsubscribe = onInstanceEmailChanged(() => {
      void load(isActive)
    })
    return () => {
      active = false
      unsubscribe()
    }
  }, [load])

  return status
}
