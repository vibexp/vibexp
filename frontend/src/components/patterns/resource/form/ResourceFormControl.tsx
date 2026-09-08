import type { ReactNode } from 'react'
import { useEffect, useRef, useState } from 'react'

import { MetadataEditor } from '@/components/metadata/MetadataEditor'
import { ProjectPicker } from '@/components/ProjectPicker'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useTypes } from '@/hooks/useTypes'

import { fieldValues } from '../statusTone'
import type { FieldSpec, FormFieldSpec } from '../types'
import { TaxonomyInput } from './TaxonomyInput'

/** What a `body` control is handed, so #914's editor can replace the textarea. */
export interface BodySlotProps {
  value: string
  onChange: (next: string) => void
  disabled: boolean
  placeholder?: string
  label: string
  'data-testid'?: string
}

export interface ResourceFormControlProps {
  spec: FormFieldSpec
  /** The descriptor field the control edits — its label and value vocabulary. */
  field: FieldSpec | undefined
  label: string
  /** The team resource type whose registered types a `types` select lists. */
  resourceType: string
  value: unknown
  onChange: (next: unknown) => void
  disabled: boolean
  /** The `metadata` editor blocks submit while a row is invalid. */
  onMetadataValidityChange?: (valid: boolean) => void
  /** Keys the `metadata` editor must not let the user delete or rename. */
  metadataRequiredKeys?: string[]
  /** Keys another control owns, so the metadata editor hides them. */
  metadataReservedKeys?: string[]
  /** Replaces the default body textarea (#914). */
  renderBody?: (props: BodySlotProps) => ReactNode
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function asRecord(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }
  return value as Record<string, unknown>
}

interface MetadataControlProps {
  value: unknown
  onChange: (next: Record<string, unknown>) => void
  disabled: boolean
  requiredKeys?: string[]
  reservedKeys?: string[]
  onValidityChange?: (valid: boolean) => void
}

/**
 * The `metadata` control, with the form value pinned by CONTENT rather than by
 * identity.
 *
 * react-hook-form's `Controller` hands back a deep **clone** of an object value
 * on every render, so its identity changes constantly. `MetadataEditor` keeps
 * derived state (its ordered rows) and re-seeds whenever `value` is not the map
 * it last emitted — which, driven straight off a `Controller`, re-seeds on
 * every render and so deletes the blank-keyed row the user just added the
 * instant they add it (`recombineMetadata` drops blank keys, by design). That
 * is why the three hand-written forms held metadata in their own `useState`
 * beside the form rather than in it.
 *
 * Holding the map here instead keeps metadata a normal form field — one schema,
 * one submit payload — while an echoed clone is recognised as an echo and a
 * genuine external change (a `reset` when the resource resolves) still re-seeds.
 */
function MetadataControl({
  value,
  onChange,
  disabled,
  requiredKeys,
  reservedKeys,
  onValidityChange,
}: Readonly<MetadataControlProps>) {
  const [current, setCurrent] = useState(() => asRecord(value))
  const signature = JSON.stringify(asRecord(value))
  const lastSignature = useRef(signature)

  useEffect(() => {
    if (lastSignature.current === signature) return
    lastSignature.current = signature
    setCurrent(asRecord(value))
  }, [signature, value])

  return (
    <MetadataEditor
      value={current}
      disabled={disabled}
      requiredKeys={requiredKeys}
      reservedKeys={reservedKeys}
      onValidityChange={onValidityChange}
      onChange={next => {
        lastSignature.current = JSON.stringify(next)
        setCurrent(next)
        onChange(next)
      }}
    />
  )
}

interface OptionSelectProps {
  spec: FormFieldSpec
  label: string
  value: string
  onChange: (next: string) => void
  disabled: boolean
  options: readonly { value: string; label: string }[]
}

/** The shared shape of every option Select on a generated form. */
function OptionSelect({
  spec,
  label,
  value,
  onChange,
  disabled,
  options,
}: Readonly<OptionSelectProps>) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger aria-label={label} data-testid={spec.testId}>
        <SelectValue placeholder={spec.placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map(option => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/**
 * A select over the team's registered types. Its own component so the catalog
 * is fetched only on a form whose descriptor declares one — the same reasoning
 * as `ResourceFilterBar`'s `TypeCatalogFilter`.
 */
function TypeCatalogSelect(
  props: Readonly<Omit<OptionSelectProps, 'options'> & { resourceType: string }>
) {
  const { resourceType, ...rest } = props
  const { types } = useTypes(resourceType)
  return (
    <OptionSelect
      {...rest}
      options={types.map(type => ({ value: type.slug, label: type.name }))}
    />
  )
}

/**
 * One control of a generated resource form.
 *
 * Kept out of `ResourceFormPage` so the page stays a layout: the switch here is
 * over the closed `FormControl` union, never over a resource kind, which is
 * what lets a descriptor the SPA has never seen render a working form.
 */
export function ResourceFormControl({
  spec,
  field,
  label,
  resourceType,
  value,
  onChange,
  disabled,
  onMetadataValidityChange,
  metadataRequiredKeys,
  metadataReservedKeys,
  renderBody,
}: Readonly<ResourceFormControlProps>) {
  switch (spec.control) {
    case 'text':
      return (
        <Input
          value={asString(value)}
          disabled={disabled}
          placeholder={spec.placeholder}
          data-testid={spec.testId}
          onChange={event => {
            onChange(event.target.value)
          }}
        />
      )
    case 'textarea':
      return (
        <Textarea
          value={asString(value)}
          disabled={disabled}
          placeholder={spec.placeholder}
          data-testid={spec.testId}
          rows={3}
          onChange={event => {
            onChange(event.target.value)
          }}
        />
      )
    case 'body': {
      const bodyProps: BodySlotProps = {
        value: asString(value),
        onChange,
        disabled,
        placeholder: spec.placeholder,
        label,
        'data-testid': spec.testId,
      }
      if (renderBody) return renderBody(bodyProps)
      return (
        <Textarea
          value={bodyProps.value}
          disabled={disabled}
          placeholder={spec.placeholder}
          data-testid={spec.testId}
          rows={22}
          className="font-mono text-sm"
          onChange={event => {
            onChange(event.target.value)
          }}
        />
      )
    }
    case 'select': {
      const shared = {
        spec,
        label,
        value: asString(value),
        onChange,
        disabled,
      }
      if (spec.optionsFrom === 'types') {
        return <TypeCatalogSelect {...shared} resourceType={resourceType} />
      }
      const labels = new Map(Object.entries(field?.valueLabels ?? {}))
      return (
        <OptionSelect
          {...shared}
          options={fieldValues(field).map(option => ({
            value: option,
            label: labels.get(option) ?? option,
          }))}
        />
      )
    }
    case 'project':
      return (
        <ProjectPicker
          value={asString(value)}
          disabled={disabled}
          placeholder={spec.placeholder}
          data-testid={spec.testId}
          onChange={projectId => {
            onChange(projectId ?? '')
          }}
        />
      )
    case 'taxonomy':
      return (
        <TaxonomyInput
          value={Array.isArray(value) ? (value as string[]) : []}
          disabled={disabled}
          placeholder={spec.placeholder}
          aria-label={label}
          data-testid={spec.testId}
          onChange={onChange}
        />
      )
    case 'metadata':
      return (
        <MetadataControl
          value={value}
          disabled={disabled}
          requiredKeys={metadataRequiredKeys}
          reservedKeys={metadataReservedKeys}
          onValidityChange={onMetadataValidityChange}
          onChange={onChange}
        />
      )
  }
}
