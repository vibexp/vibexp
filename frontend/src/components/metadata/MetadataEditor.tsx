import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  MAX_METADATA_PAIRS,
  type MetadataPair,
  recombineMetadata,
  splitMetadata,
  validateMetadataRows,
} from './metadataPairs'

/**
 * A controlled editor for the string-valued subset of a resource's `metadata`
 * (`Record<string, unknown>`). The host owns the full map; this component keeps
 * an ordered list of string rows internally, emits the recombined map via
 * `onChange` on every edit, and reports validity via `onValidityChange` so the
 * host can block submit. Non-string metadata (arrays / nested objects) is split
 * off once and preserved untouched — it never appears as a row and round-trips
 * through the update payload verbatim.
 *
 * `reservedKeys` bars a key from being added as a string row (e.g. Memory's
 * `tags`, owned by its own chip UI); `requiredKeys` marks rows that cannot be
 * deleted and whose key is fixed (e.g. the sub-agents `model` key).
 */
interface MetadataEditorProps {
  /** The resource's current metadata. String entries become editable rows. */
  value?: Record<string, unknown>
  /** Fires with the recombined metadata map on every edit. */
  onChange: (value: Record<string, unknown>) => void
  /** Fires whenever overall validity changes, so the host can gate submit. */
  onValidityChange?: (valid: boolean) => void
  /** Keys that may not be added as string rows. */
  reservedKeys?: string[]
  /** Keys whose row cannot be deleted and whose key input is read-only. */
  requiredKeys?: string[]
  /** Disable all inputs (e.g. while the host form is submitting). */
  disabled?: boolean
  className?: string
}

interface Row {
  id: string
  key: string
  value: string
}

const EMPTY_STRINGS: string[] = []

export function MetadataEditor({
  value,
  onChange,
  onValidityChange,
  reservedKeys = EMPTY_STRINGS,
  requiredKeys = EMPTY_STRINGS,
  disabled = false,
  className,
}: Readonly<MetadataEditorProps>) {
  const idCounter = useRef(0)
  const nextId = () => {
    idCounter.current += 1
    return `metadata-row-${String(idCounter.current)}`
  }

  const [rows, setRows] = useState<Row[]>(() =>
    splitMetadata(value).pairs.map(p => ({ id: nextId(), ...p }))
  )
  const extrasRef = useRef<Record<string, unknown>>(splitMetadata(value).extras)
  // The last map we handed to onChange. When the host echoes it straight back
  // as `value` we skip the resync effect; a genuinely different `value` (a form
  // reset / async resource load) is an external change and re-seeds the rows.
  const lastEmittedRef = useRef<Record<string, unknown> | undefined>(value)

  useEffect(() => {
    if (value === lastEmittedRef.current) return
    const { pairs, extras } = splitMetadata(value)
    extrasRef.current = extras
    setRows(pairs.map(p => ({ id: nextId(), ...p })))
    lastEmittedRef.current = value
  }, [value])

  const emit = (nextRows: Row[]) => {
    setRows(nextRows)
    const pairs: MetadataPair[] = nextRows.map(({ key, value: v }) => ({
      key,
      value: v,
    }))
    const next = recombineMetadata(pairs, extrasRef.current)
    lastEmittedRef.current = next
    onChange(next)
  }

  const validation = useMemo(
    () =>
      validateMetadataRows(
        rows.map(({ key, value: v }) => ({ key, value: v })),
        {
          extrasKeys: Object.keys(extrasRef.current),
          reservedKeys,
          requiredKeys,
        }
      ),
    [rows, reservedKeys, requiredKeys]
  )

  const lastValidRef = useRef<boolean | null>(null)
  useEffect(() => {
    if (lastValidRef.current === validation.valid) return
    lastValidRef.current = validation.valid
    onValidityChange?.(validation.valid)
  }, [validation.valid, onValidityChange])

  const updateRow = (
    id: string,
    patch: Partial<Pick<Row, 'key' | 'value'>>
  ) => {
    emit(rows.map(row => (row.id === id ? { ...row, ...patch } : row)))
  }

  const deleteRow = (id: string) => {
    const target = rows.find(row => row.id === id)
    if (target && requiredKeys.includes(target.key.trim())) return
    emit(rows.filter(row => row.id !== id))
  }

  const addRow = () => {
    if (rows.length >= MAX_METADATA_PAIRS) return
    emit([...rows, { id: nextId(), key: '', value: '' }])
  }

  const atCap = rows.length >= MAX_METADATA_PAIRS

  return (
    <div className={cn('space-y-2', className)} data-testid="metadata-editor">
      {rows.length === 0 && (
        <p className="text-muted-foreground text-xs">
          No metadata yet. Add a key-value pair below.
        </p>
      )}

      {rows.map((row, index) => {
        const error = validation.rowErrors[index]
        const isRequired = requiredKeys.includes(row.key.trim())
        return (
          <div key={row.id} className="space-y-1" data-testid="metadata-row">
            <div className="flex items-start gap-2">
              <Input
                aria-label={`Metadata key ${String(index + 1)}`}
                data-testid={`metadata-key-${String(index)}`}
                value={row.key}
                readOnly={isRequired}
                disabled={disabled}
                placeholder="Key"
                aria-invalid={error !== null}
                className={cn(
                  'flex-1 font-mono text-sm',
                  isRequired && 'bg-muted',
                  error !== null && 'border-destructive'
                )}
                onChange={e => {
                  updateRow(row.id, { key: e.target.value })
                }}
              />
              <Input
                aria-label={`Metadata value ${String(index + 1)}`}
                data-testid={`metadata-value-${String(index)}`}
                value={row.value}
                disabled={disabled}
                placeholder="Value"
                aria-invalid={error !== null}
                className={cn(
                  'flex-1 text-sm',
                  error !== null && 'border-destructive'
                )}
                onChange={e => {
                  updateRow(row.id, { value: e.target.value })
                }}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Remove ${row.key.trim() || `pair ${String(index + 1)}`}`}
                data-testid={`metadata-delete-${String(index)}`}
                disabled={disabled || isRequired}
                className={cn(isRequired && 'invisible')}
                onClick={() => {
                  deleteRow(row.id)
                }}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
            {error !== null && (
              <p
                className="text-destructive text-xs"
                data-testid={`metadata-error-${String(index)}`}
              >
                {error}
              </p>
            )}
          </div>
        )
      })}

      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled || atCap}
        data-testid="metadata-add-pair"
        onClick={addRow}
      >
        <Plus className="size-4" />
        Add pair
      </Button>

      {validation.formError !== null && (
        <p
          className="text-destructive text-xs"
          data-testid="metadata-form-error"
        >
          {validation.formError}
        </p>
      )}
    </div>
  )
}
