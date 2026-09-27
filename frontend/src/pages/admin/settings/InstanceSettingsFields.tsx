import type { ReactNode } from 'react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { fieldDescribedBy } from './instanceSettingsForm'

/**
 * Field rows shared by the instance search and AI summary settings pages
 * (#1202): a label, the control, a hint, and the field's error (client-side
 * or from the server's `validation_errors`) wired up through
 * `aria-describedby` / `aria-invalid`.
 */

interface FieldFrameProps {
  id: string
  label: string
  hint?: ReactNode
  error?: string
  children: ReactNode
}

/** A field's hint and error, with the ids `fieldDescribedBy` points at. */
function FieldMessages({
  id,
  hint,
  error,
}: Readonly<Pick<FieldFrameProps, 'id' | 'hint' | 'error'>>) {
  return (
    <>
      {hint && (
        <p id={`${id}-hint`} className="text-muted-foreground text-xs">
          {hint}
        </p>
      )}
      {error && (
        <p id={`${id}-error`} className="text-destructive text-xs" role="alert">
          {error}
        </p>
      )}
    </>
  )
}

export function FieldFrame({
  id,
  label,
  hint,
  error,
  children,
}: Readonly<FieldFrameProps>) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      {children}
      <FieldMessages id={id} hint={hint} error={error} />
    </div>
  )
}

interface NumberSettingFieldProps {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  hint?: ReactNode
  error?: string
  min?: number
  max?: number
  /** `1` for whole numbers, `'any'` for decimals. */
  step: number | 'any'
  disabled?: boolean
}

export function NumberSettingField({
  id,
  label,
  value,
  onChange,
  hint,
  error,
  min,
  max,
  step,
  disabled,
}: Readonly<NumberSettingFieldProps>) {
  return (
    <FieldFrame id={id} label={label} hint={hint} error={error}>
      <Input
        id={id}
        type="number"
        inputMode={step === 1 ? 'numeric' : 'decimal'}
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        aria-invalid={error ? true : undefined}
        aria-describedby={fieldDescribedBy(id, hint, error)}
        onChange={e => {
          onChange(e.target.value)
        }}
      />
    </FieldFrame>
  )
}

interface SwitchSettingFieldProps {
  id: string
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
  hint?: ReactNode
  error?: string
  disabled?: boolean
}

export function SwitchSettingField({
  id,
  label,
  checked,
  onChange,
  hint,
  error,
  disabled,
}: Readonly<SwitchSettingFieldProps>) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor={id}>{label}</Label>
        <FieldMessages id={id} hint={hint} error={error} />
      </div>
      <Switch
        id={id}
        checked={checked}
        disabled={disabled}
        aria-invalid={error ? true : undefined}
        aria-describedby={fieldDescribedBy(id, hint, error)}
        onCheckedChange={onChange}
      />
    </div>
  )
}
