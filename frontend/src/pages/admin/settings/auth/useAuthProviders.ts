import { useCallback, useEffect, useState } from 'react'

import {
  type AdminAuthProvider,
  authSettingsService,
} from '@/services/authSettingsService'

import { isVersionConflict } from '../instanceSettingsForm'
import {
  lockoutRiskReason,
  moveProvider,
  sortProviders,
  toProviderUpdate,
} from './authSettingsForm'

/**
 * One provider change. It is handed the settings `version` to send as
 * `expected_version` and whether the admin confirmed a lockout risk.
 */
export type ProviderWrite = (
  version: number,
  confirmLockoutRisk: boolean
) => Promise<void>

/** How a provider change ended. `error` carries what the server said. */
export type ProviderWriteResult =
  | { status: 'ok' }
  | { status: 'conflict' }
  | { status: 'lockout' }
  | { status: 'error'; error: unknown }

interface PendingLockout {
  reason: string
  write: ProviderWrite
  onOk?: () => void
}

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

/**
 * The sign-in providers and the rules every change to them follows (#1239):
 *
 * - every write sends the list's shared `version` as `expected_version`, and
 *   the list is re-read afterwards, so the next write sends the new one;
 * - a 409 version conflict is never retried: `conflict` is set and stays set
 *   until `reload`;
 * - a 409 `lockout_risk` is held as `pendingLockout` until the admin confirms
 *   (the same change re-sent with `confirm_lockout_risk`) or cancels.
 *
 * `refreshKey` re-reads the list when it changes: an allowlist save bumps the
 * shared version too, and a stale one would turn the next provider change into
 * a false conflict. `onChanged` runs after each stored change.
 */
export function useAuthProviders(refreshKey: number, onChanged?: () => void) {
  const [providers, setProviders] = useState<AdminAuthProvider[] | null>(null)
  const [version, setVersion] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [pendingLockout, setPendingLockout] = useState<PendingLockout | null>(
    null
  )

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      const list = await authSettingsService.listProviders()
      setProviders(sortProviders(list.providers))
      setVersion(list.version)
      setConflict(false)
    } catch (err) {
      setLoadError(messageOf(err, 'Failed to load the sign-in providers'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load, refreshKey])

  const attempt = async (
    write: ProviderWrite,
    confirmed: boolean,
    onOk?: () => void
  ): Promise<ProviderWriteResult> => {
    if (conflict) return { status: 'conflict' }
    setBusy(true)
    setActionError(null)
    try {
      await write(version, confirmed)
    } catch (err) {
      if (isVersionConflict(err)) {
        setPendingLockout(null)
        setConflict(true)
        return { status: 'conflict' }
      }
      const reason = lockoutRiskReason(err)
      if (reason !== null && !confirmed) {
        setPendingLockout({ reason, write, onOk })
        return { status: 'lockout' }
      }
      setPendingLockout(null)
      // A change of several steps (a reorder) may have half-applied.
      await load()
      return { status: 'error', error: err }
    } finally {
      setBusy(false)
    }
    setPendingLockout(null)
    await load()
    onChanged?.()
    onOk?.()
    return { status: 'ok' }
  }

  /** Runs a change; `onOk` also runs when it only succeeds after a confirm. */
  const run = (write: ProviderWrite, onOk?: () => void) =>
    attempt(write, false, onOk)

  /** Runs a change whose failure is shown at the section level. */
  const runShowingError = async (write: ProviderWrite, fallback: string) => {
    const result = await run(write)
    if (result.status === 'error') {
      setActionError(messageOf(result.error, fallback))
    }
  }

  const confirmLockout = async () => {
    if (!pendingLockout) return
    const { write, onOk } = pendingLockout
    const result = await attempt(write, true, onOk)
    if (result.status === 'error') {
      setActionError(messageOf(result.error, 'Failed to apply the change'))
    }
  }

  const setEnabled = (provider: AdminAuthProvider, enabled: boolean) =>
    runShowingError(
      async (expected, confirm) => {
        await authSettingsService.updateProvider(
          provider.id,
          toProviderUpdate(provider, { enabled }, expected, confirm)
        )
      },
      `Failed to ${enabled ? 'enable' : 'disable'} ${provider.display_name}`
    )

  const remove = (provider: AdminAuthProvider) =>
    runShowingError(async (expected, confirm) => {
      await authSettingsService.deleteProvider(provider.id, {
        expected_version: expected,
        ...(confirm ? { confirm_lockout_risk: true } : {}),
      })
    }, `Failed to delete ${provider.display_name}`)

  /**
   * Moves a provider one place: one PUT per provider whose position changes,
   * each sending the version the previous one returned.
   */
  const move = (index: number, direction: -1 | 1) => {
    const reordered = moveProvider(providers ?? [], index, direction)
    return runShowingError(async expected => {
      let current = expected
      for (const [position, provider] of reordered.entries()) {
        if (provider.sort_order === position) continue
        const saved = await authSettingsService.updateProvider(
          provider.id,
          toProviderUpdate(provider, { sort_order: position }, current, false)
        )
        current = saved.version
      }
    }, 'Failed to reorder the providers')
  }

  return {
    providers,
    version,
    loading,
    loadError,
    busy,
    conflict,
    actionError,
    pendingLockout,
    reload: load,
    run,
    confirmLockout,
    cancelLockout: () => {
      setPendingLockout(null)
    },
    setEnabled,
    remove,
    move,
  }
}

export type AuthProvidersState = ReturnType<typeof useAuthProviders>
