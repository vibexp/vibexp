import { useCallback, useEffect, useState } from 'react'

import { toast } from '@/lib/toast'
import { authService } from '@/services/authService'
import {
  type AdminAuthPolicy,
  type AdminAuthRegistrationMode,
  authSettingsService,
} from '@/services/authSettingsService'

import { isVersionConflict } from '../instanceSettingsForm'
import {
  lockoutRiskAccountCount,
  lockoutRiskReason,
  lockoutRiskReasons,
  messageOf,
} from './authSettingsForm'

/** The two settings of the authentication policy, as the form holds them. */
export interface AuthPolicyForm {
  passwordLogin: boolean
  registrationMode: AdminAuthRegistrationMode
}

/** A save the server refused as a lockout risk, waiting for the admin. */
export interface PendingPolicyLockout {
  reason: string
  /** Every reason that applies, `reason` first (#1473). */
  reasons: string[]
  /** Accounts whose only sign-in method is a password, when that is the risk. */
  accounts: number | null
}

function toForm(policy: AdminAuthPolicy): AuthPolicyForm {
  return {
    passwordLogin: policy.password_login_enabled,
    registrationMode: policy.registration_mode,
  }
}

/**
 * State and actions of the authentication policy card (#1442): whether email
 * and password sign-in is on, and who may get an account.
 *
 * The two settings are one stored row with one `version`, so they are saved
 * together. A save sends the loaded version (0 when nothing is stored). A 409
 * is never retried on its own:
 * - a version conflict sets `conflict` until `reload`, and the admin's
 *   choices stay in the form;
 * - a `lockout_risk` is held as `pendingLockout` until the admin confirms
 *   (the same change re-sent with `confirm_lockout_risk`) or cancels.
 *
 * `resetByAdmin` says a forgotten password needs an admin-issued link because
 * no email provider is configured; it is read from the public sign-in
 * discovery and a failure to read it only hides the note.
 */
export function useAuthPolicy(onSaved?: () => void) {
  const [stored, setStored] = useState<AdminAuthPolicy | null>(null)
  const [form, setForm] = useState<AuthPolicyForm>({
    passwordLogin: false,
    registrationMode: 'open',
  })
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [pendingLockout, setPendingLockout] =
    useState<PendingPolicyLockout | null>(null)
  const [resetByAdmin, setResetByAdmin] = useState(false)

  const readResetChannel = useCallback(async (): Promise<void> => {
    try {
      const methods = await authService.getSignInMethods()
      setResetByAdmin(methods.password.reset === 'admin')
    } catch {
      setResetByAdmin(false)
    }
  }, [])

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      const next = await authSettingsService.getPolicy()
      setStored(next)
      setForm(toForm(next))
      setConflict(false)
      setFormError(null)
    } catch (err) {
      setLoadError(messageOf(err, 'Failed to load the sign-in policy'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
    void readResetChannel()
  }, [load, readResetChannel])

  const dirty =
    stored !== null &&
    (form.passwordLogin !== stored.password_login_enabled ||
      form.registrationMode !== stored.registration_mode)

  const setPasswordLogin = (passwordLogin: boolean) => {
    setForm(current => ({ ...current, passwordLogin }))
  }

  const setRegistrationMode = (registrationMode: AdminAuthRegistrationMode) => {
    setForm(current => ({ ...current, registrationMode }))
  }

  const store = async (confirmLockoutRisk: boolean) => {
    if (!stored) return
    try {
      setBusy(true)
      setFormError(null)
      const next = await authSettingsService.updatePolicy({
        password_login_enabled: form.passwordLogin,
        registration_mode: form.registrationMode,
        // 0 means "nothing stored yet", so a first save is checked too.
        expected_version: stored.version ?? 0,
        confirm_lockout_risk: confirmLockoutRisk,
      })
      setStored(next)
      setForm(toForm(next))
      setPendingLockout(null)
      toast.success('Sign-in policy saved')
      onSaved?.()
    } catch (err) {
      const reason = lockoutRiskReason(err)
      if (reason !== null) {
        setPendingLockout({
          reason,
          reasons: lockoutRiskReasons(err),
          accounts: lockoutRiskAccountCount(err),
        })
        return
      }
      setPendingLockout(null)
      if (isVersionConflict(err)) {
        setConflict(true)
        return
      }
      setFormError(messageOf(err, 'Failed to save the sign-in policy'))
    } finally {
      setBusy(false)
    }
  }

  const save = async () => {
    if (conflict || !dirty) return
    await store(false)
  }

  const confirmLockout = async () => {
    if (!pendingLockout) return
    await store(true)
  }

  const cancelLockout = () => {
    setPendingLockout(null)
  }

  return {
    stored,
    form,
    loading,
    loadError,
    busy,
    conflict,
    dirty,
    formError,
    pendingLockout,
    resetByAdmin,
    setPasswordLogin,
    setRegistrationMode,
    save,
    confirmLockout,
    cancelLockout,
    reload: load,
  }
}
