import { Bookmark, Check, Pencil, Save } from 'lucide-react'
import { useId, useState } from 'react'

import { badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type {
  AdminSavedFilterListName,
  AdminSavedFilterPreset,
} from '@/services/adminService'

import { MAX_PRESETS, sameQuery, validatePresetName } from './presetValidation'
import type { PresetQuery } from './useAdminSavedFilters'
import { useAdminSavedFilters } from './useAdminSavedFilters'

export const CAP_MESSAGE = `${String(MAX_PRESETS)} of ${String(MAX_PRESETS)} presets — delete one to save another`

export interface AdminSavedFiltersMenuProps {
  list: AdminSavedFilterListName
  /** The page's filters as a preset query (`useAdminListFilters().currentQuery`). */
  currentQuery: PresetQuery
  /** Replaces the page's filters with a preset's (`useAdminListFilters().applyQuery`). */
  onApply: (query: PresetQuery) => void
  /** Whether any filter is applied; saving an empty preset is pointless. */
  canSave: boolean
}

/**
 * The "Presets" control in the admin list filter bar (#1148): apply one of the
 * admin's saved filter presets in one click, save the current filters as a new
 * one, or rename/delete them in the manage dialog.
 *
 * Applying replaces the URL query, so the resulting view stays shareable
 * without the preset. Rename and delete live in a dialog rather than
 * per-item submenus, which keeps the apply list flat.
 */
export function AdminSavedFiltersMenu({
  list,
  currentQuery,
  onApply,
  canSave,
}: Readonly<AdminSavedFiltersMenuProps>) {
  const saved = useAdminSavedFilters(list)
  const [saveOpen, setSaveOpen] = useState(false)
  const [manageOpen, setManageOpen] = useState(false)

  const ready = saved.status === 'ready'
  const atCap = saved.presets.length >= MAX_PRESETS

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm">
            <Bookmark className="size-4" />
            Presets
            {saved.presets.length > 0 && (
              <span
                data-testid="saved-filters-count"
                className={badgeVariants({ variant: 'secondary' })}
              >
                {saved.presets.length}
                <span className="sr-only"> saved</span>
              </span>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-72">
          <DropdownMenuLabel>Saved presets</DropdownMenuLabel>
          {saved.status === 'loading' && (
            <p className="text-muted-foreground px-2 py-1.5 text-sm">
              Loading presets…
            </p>
          )}
          {saved.status === 'error' && (
            <>
              <p className="text-destructive px-2 py-1.5 text-sm">
                Couldn&apos;t load presets
              </p>
              <DropdownMenuItem
                onSelect={() => {
                  void saved.reload()
                }}
              >
                Retry
              </DropdownMenuItem>
            </>
          )}
          {ready && saved.presets.length === 0 && (
            <p className="text-muted-foreground px-2 py-1.5 text-sm">
              No saved presets yet
            </p>
          )}
          {ready &&
            saved.presets.map(preset => {
              const active = sameQuery(preset.query, currentQuery)
              return (
                <DropdownMenuItem
                  key={preset.id}
                  aria-current={active ? 'true' : undefined}
                  onSelect={() => {
                    onApply(preset.query)
                  }}
                >
                  <Check
                    className={active ? 'size-4' : 'invisible size-4'}
                    aria-hidden
                  />
                  <span className="truncate">{preset.name}</span>
                </DropdownMenuItem>
              )
            })}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={!ready || atCap || !canSave || saved.saving}
            onSelect={() => {
              setSaveOpen(true)
            }}
          >
            <Save className="size-4" aria-hidden />
            Save current filters…
          </DropdownMenuItem>
          {ready && atCap && (
            <p className="text-muted-foreground px-2 pb-1.5 text-xs">
              {CAP_MESSAGE}
            </p>
          )}
          {ready && !atCap && !canSave && (
            <p className="text-muted-foreground px-2 pb-1.5 text-xs">
              Apply some filters first
            </p>
          )}
          <DropdownMenuItem
            disabled={!ready || saved.presets.length === 0}
            onSelect={() => {
              setManageOpen(true)
            }}
          >
            <Pencil className="size-4" aria-hidden />
            Manage presets…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {saveOpen && (
        <SavePresetDialog
          presets={saved.presets}
          saving={saved.saving}
          onSave={name => saved.save(name, currentQuery)}
          onClose={() => {
            setSaveOpen(false)
          }}
        />
      )}

      <Dialog open={manageOpen} onOpenChange={setManageOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Manage presets</DialogTitle>
            <DialogDescription>
              Rename or delete your saved filter presets for this list.
            </DialogDescription>
          </DialogHeader>
          {saved.presets.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No saved presets yet
            </p>
          ) : (
            <ul className="space-y-3">
              {saved.presets.map(preset => (
                <PresetRow
                  key={preset.id}
                  preset={preset}
                  presets={saved.presets}
                  saving={saved.saving}
                  onRename={name => saved.rename(preset.id, name)}
                  onDelete={() => saved.remove(preset.id)}
                />
              ))}
            </ul>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}

interface SavePresetDialogProps {
  presets: readonly AdminSavedFilterPreset[]
  saving: boolean
  onSave: (name: string) => Promise<boolean>
  onClose: () => void
}

/** Names the current filters. Mounted only while open, so it starts empty. */
function SavePresetDialog({
  presets,
  saving,
  onSave,
  onClose,
}: Readonly<SavePresetDialogProps>) {
  const inputId = useId()
  const [name, setName] = useState('')
  const [touched, setTouched] = useState(false)
  const error = touched ? validatePresetName(name, presets) : null

  const submit = async () => {
    setTouched(true)
    if (validatePresetName(name, presets) !== null) return
    if (await onSave(name)) onClose()
  }

  return (
    <Dialog
      open
      onOpenChange={open => {
        if (!open) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Save current filters</DialogTitle>
          <DialogDescription>
            Save this list&apos;s filters and sort as a preset you can apply in
            one click.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={event => {
            event.preventDefault()
            void submit()
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor={inputId}>Preset name</Label>
            <Input
              id={inputId}
              value={name}
              onChange={event => {
                setName(event.target.value)
                setTouched(true)
              }}
              placeholder="Dormant teams"
              autoComplete="off"
              aria-invalid={error !== null}
            />
            {error && <p className="text-destructive text-sm">{error}</p>}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={saving}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saving}>
              Save preset
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface PresetRowProps {
  preset: AdminSavedFilterPreset
  presets: readonly AdminSavedFilterPreset[]
  saving: boolean
  onRename: (name: string) => Promise<boolean>
  onDelete: () => Promise<boolean>
}

/** One preset in the manage dialog: inline rename and a two-step delete. */
function PresetRow({
  preset,
  presets,
  saving,
  onRename,
  onDelete,
}: Readonly<PresetRowProps>) {
  const [draft, setDraft] = useState(preset.name)
  const [confirming, setConfirming] = useState(false)
  const changed = draft.trim() !== preset.name
  const error = changed ? validatePresetName(draft, presets, preset.id) : null

  return (
    <li className="space-y-1">
      <div className="flex items-center gap-2">
        <Input
          value={draft}
          onChange={event => {
            setDraft(event.target.value)
          }}
          aria-label={`Name for preset ${preset.name}`}
          aria-invalid={error !== null}
          autoComplete="off"
        />
        <Button
          variant="outline"
          size="sm"
          disabled={!changed || error !== null || saving}
          onClick={() => {
            void onRename(draft)
          }}
        >
          Rename
        </Button>
        <Button
          variant={confirming ? 'destructive' : 'ghost'}
          size="sm"
          disabled={saving}
          onClick={() => {
            if (confirming) {
              void onDelete()
            } else {
              setConfirming(true)
            }
          }}
        >
          {confirming ? 'Confirm delete' : 'Delete'}
        </Button>
      </div>
      {error && <p className="text-destructive text-sm">{error}</p>}
    </li>
  )
}
