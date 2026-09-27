import { useCallback, useEffect, useMemo, useState } from 'react'

import { toast } from '@/lib/toast'

import {
  type FieldErrors,
  isVersionConflict,
  serverFieldErrors,
} from './instanceSettingsForm'

/** What every instance settings GET response carries besides its values. */
export interface InstanceSettingsEnvelope<V, L> {
  values: V
  built_in_defaults: V
  limits: L
  source: 'instance' | 'default'
  teams_with_override: number
  updated_at: string | null
  updated_by_name: string | null
  version: number | null
}

export interface InstanceSettingsEditorOptions<
  S extends InstanceSettingsEnvelope<S['values'], S['limits']>,
  F extends object,
  B,
> {
  get: () => Promise<S>
  /** The PUT; the editor adds `expected_version` to the body. */
  update: (body: B & { expected_version: number | null }) => Promise<S>
  reset: () => Promise<void>
  toForm: (values: S['values']) => F
  toUpdate: (form: F) => B
  validate: (form: F, limits: S['limits']) => FieldErrors<keyof F & string>
  /** The form's field names, which are also the wire field names. */
  fields: readonly (keyof F & string)[]
  /** e.g. "search settings", for messages. */
  noun: string
}

/**
 * State and actions of an instance settings page (#1202): load, edit, save
 * with optimistic locking, reset to the built-in defaults.
 *
 * Client-side errors are computed live from the form and the response's
 * `limits`; a server 400's field errors are kept until that field is edited.
 * A 409 version conflict is never retried or overwritten: the page shows a
 * reload prompt, and saving stays blocked until the admin reloads.
 *
 * `options` must be referentially stable (a module-level constant): its
 * functions are effect dependencies, so a fresh object per render would
 * refetch forever.
 */
export function useInstanceSettingsEditor<
  S extends InstanceSettingsEnvelope<S['values'], S['limits']>,
  F extends object,
  B,
>(options: InstanceSettingsEditorOptions<S, F, B>) {
  const { get, update, reset, toForm, toUpdate, validate, fields, noun } =
    options
  type Field = keyof F & string

  const [settings, setSettings] = useState<S | null>(null)
  const [form, setForm] = useState<F | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [serverErrors, setServerErrors] = useState<FieldErrors<Field>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [auditKey, setAuditKey] = useState(0)

  const apply = useCallback(
    (next: S) => {
      setSettings(next)
      setForm(toForm(next.values))
      setServerErrors({})
      setConflict(false)
    },
    [toForm]
  )

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      apply(await get())
    } catch (err) {
      setLoadError(
        err instanceof Error ? err.message : `Failed to load the ${noun}`
      )
    } finally {
      setLoading(false)
    }
  }, [apply, get, noun])

  useEffect(() => {
    void load()
  }, [load])

  const clientErrors = useMemo<FieldErrors<Field>>(
    () => (form && settings ? validate(form, settings.limits) : {}),
    [form, settings, validate]
  )

  const errors = useMemo<FieldErrors<Field>>(() => {
    const merged: FieldErrors<Field> = { ...clientErrors }
    for (const field of fields) {
      const message = serverErrors[field]
      if (message) merged[field] = message
    }
    return merged
  }, [clientErrors, fields, serverErrors])

  const hasErrors = Object.keys(errors).length > 0

  const dirty = useMemo(
    () =>
      form !== null &&
      settings !== null &&
      JSON.stringify(form) !== JSON.stringify(toForm(settings.values)),
    [form, settings, toForm]
  )

  const setField = useCallback(<K extends Field>(field: K, value: F[K]) => {
    setForm(current => (current ? { ...current, [field]: value } : current))
    setServerErrors(current =>
      field in current
        ? (Object.fromEntries(
            Object.entries(current).filter(([key]) => key !== field)
          ) as FieldErrors<Field>)
        : current
    )
  }, [])

  const save = async () => {
    if (!form || !settings || hasErrors || conflict) return
    try {
      setSaving(true)
      setFormError(null)
      apply(
        await update({
          ...toUpdate(form),
          expected_version: settings.version,
        })
      )
      setAuditKey(key => key + 1)
      toast.success(`Instance ${noun} saved`)
    } catch (err) {
      if (isVersionConflict(err)) {
        setConflict(true)
        return
      }
      const split = serverFieldErrors(err, fields)
      if (split) {
        setServerErrors(split.fields)
        setFormError(split.other.length > 0 ? split.other.join(' ') : null)
        return
      }
      setFormError(
        err instanceof Error ? err.message : `Failed to save the ${noun}`
      )
    } finally {
      setSaving(false)
    }
  }

  const resetToDefaults = async (): Promise<void> => {
    try {
      setResetting(true)
      setFormError(null)
      await reset()
    } catch (err) {
      setFormError(
        err instanceof Error ? err.message : `Failed to reset the ${noun}`
      )
      return
    } finally {
      setResetting(false)
    }
    setAuditKey(key => key + 1)
    toast.success(`Instance ${noun} reset to the built-in defaults`)
    await load()
  }

  const reload = async () => {
    setFormError(null)
    await load()
  }

  return {
    settings,
    form,
    loading,
    loadError,
    saving,
    resetting,
    conflict,
    errors,
    hasErrors,
    formError,
    dirty,
    auditKey,
    setField,
    save,
    resetToDefaults,
    reload,
  }
}

export type InstanceSettingsEditor<
  S extends InstanceSettingsEnvelope<S['values'], S['limits']>,
  F extends object,
  B,
> = ReturnType<typeof useInstanceSettingsEditor<S, F, B>>
