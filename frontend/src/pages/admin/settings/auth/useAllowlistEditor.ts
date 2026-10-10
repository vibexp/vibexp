import { useCallback, useEffect, useMemo, useState } from 'react'

import { toast } from '@/lib/toast'
import {
  type AdminAuthAllowlist,
  type AdminAuthAllowlistImpact,
  authSettingsService,
} from '@/services/authSettingsService'

import {
  type FieldErrors,
  isVersionConflict,
  serverFieldErrors,
} from '../instanceSettingsForm'
import {
  ALLOWLIST_FORM_FIELDS,
  type AllowlistField,
  type AllowlistForm,
  normalizeAllowlistEntries,
  sameAllowlistEntries,
  validateAllowlistForm,
} from './authSettingsForm'

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

/**
 * State and actions of the access allowlist editor (#1239).
 *
 * A save asks the server first who it would shut out
 * (`POST …/allowlist/preview`): with nobody affected it saves directly,
 * otherwise `impact` is set and the admin confirms it (`store`) or cancels.
 * The save sends the loaded `version` (0 when nothing is stored), and a 409 is
 * never retried: `conflict` stays set until `reload`.
 *
 * `onSaved` runs after each stored change: an allowlist change bumps the
 * shared authentication settings version the providers are saved against.
 */
export function useAllowlistEditor(onSaved?: () => void) {
  const [stored, setStored] = useState<AdminAuthAllowlist | null>(null)
  const [form, setForm] = useState<AllowlistForm>({ domains: [], emails: [] })
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [serverErrors, setServerErrors] = useState<FieldErrors<AllowlistField>>(
    {}
  )
  const [formError, setFormError] = useState<string | null>(null)
  const [impact, setImpact] = useState<AdminAuthAllowlistImpact | null>(null)
  const [confirmReset, setConfirmReset] = useState(false)

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      const next = await authSettingsService.getAllowlist()
      setStored(next)
      setForm({ domains: next.domains, emails: next.emails })
      setServerErrors({})
      setConflict(false)
    } catch (err) {
      setLoadError(messageOf(err, 'Failed to load the access allowlist'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const clientErrors = useMemo(() => validateAllowlistForm(form), [form])
  const errors: FieldErrors<AllowlistField> = {
    ...serverErrors,
    ...clientErrors,
  }
  const hasErrors = Object.keys(errors).length > 0
  const dirty =
    stored !== null &&
    !(
      sameAllowlistEntries(form.domains, stored.domains) &&
      sameAllowlistEntries(form.emails, stored.emails)
    )

  const candidate = () => ({
    domains: normalizeAllowlistEntries(form.domains),
    emails: normalizeAllowlistEntries(form.emails),
  })

  const setList = (field: AllowlistField, value: string[]) => {
    setForm(current => ({ ...current, [field]: value }))
    setServerErrors(current =>
      Object.fromEntries(
        Object.entries(current).filter(([key]) => key !== field)
      )
    )
  }

  const showError = (err: unknown, fallback: string) => {
    if (isVersionConflict(err)) {
      setConflict(true)
      return
    }
    const split = serverFieldErrors(err, ALLOWLIST_FORM_FIELDS)
    if (split) {
      setServerErrors(split.fields)
      setFormError(split.other.length > 0 ? split.other.join(' ') : null)
      return
    }
    setFormError(messageOf(err, fallback))
  }

  const store = async () => {
    if (!stored) return
    try {
      setBusy(true)
      setFormError(null)
      const next = await authSettingsService.updateAllowlist({
        ...candidate(),
        // 0 means "nothing stored yet", so a first save is checked too.
        expected_version: stored.version ?? 0,
      })
      setStored(next)
      setForm({ domains: next.domains, emails: next.emails })
      setServerErrors({})
      toast.success('Access allowlist saved')
      onSaved?.()
    } catch (err) {
      showError(err, 'Failed to save the access allowlist')
    } finally {
      setBusy(false)
      setImpact(null)
    }
  }

  const save = async () => {
    if (hasErrors || conflict || !dirty) return
    let preview: AdminAuthAllowlistImpact
    try {
      setBusy(true)
      setFormError(null)
      preview = await authSettingsService.previewAllowlist(candidate())
    } catch (err) {
      showError(err, 'Failed to check who the allowlist would sign out')
      return
    } finally {
      setBusy(false)
    }
    if (preview.count > 0) {
      setImpact(preview)
      return
    }
    await store()
  }

  const reset = async () => {
    try {
      setBusy(true)
      setFormError(null)
      await authSettingsService.resetAllowlist()
    } catch (err) {
      setFormError(messageOf(err, 'Failed to reset the access allowlist'))
      return
    } finally {
      setBusy(false)
      setConfirmReset(false)
    }
    toast.success('Access allowlist reset to open access')
    onSaved?.()
    await load()
  }

  return {
    stored,
    form,
    loading,
    loadError,
    busy,
    conflict,
    errors,
    hasErrors,
    dirty,
    formError,
    impact,
    confirmReset,
    setConfirmReset,
    setList,
    save,
    store,
    reset,
    reload: load,
    cancelImpact: () => {
      setImpact(null)
    },
  }
}
