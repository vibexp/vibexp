import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import type { Team } from '@/services/teamService'

import { SourceTeamPicker } from './SourceTeamPicker'

interface CopyFromTeamDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /**
   * The DESTINATION team. Always a prop, never `useTeam()` — a settings page's
   * effects run before `TeamScopeLayout` syncs the ambient team, so on a cold
   * deep-link `useTeam().currentTeam` is a different team than the URL's (#584).
   */
  team: Team
  title: string
  description: ReactNode
  /** Owned by the page, which makes the service call. */
  submitting: boolean
  onConfirm: (sourceTeam: Team) => void | Promise<void>
  /**
   * Notified whenever the chosen source changes (and with `null` when the
   * dialog resets), so the page can load whatever it needs to preview the
   * copy. The preview itself is the page's, since what a copy will do differs
   * per surface.
   */
  onSourceChange?: (sourceTeam: Team | null) => void
  /** Rendered between the picker and the footer once a source is chosen. */
  preview?: ReactNode
  /** See {@link SourceTeamPicker}'s `canCopyFrom`. */
  canCopyFrom?: (team: Team) => boolean
  confirmLabel?: string
  submittingLabel?: string
}

/**
 * The shared "Copy from another team…" affordance for the team settings pages
 * (epic #827): pick one of your other teams, see what the copy will do, confirm.
 *
 * The dialog owns only the picker and the chosen source. Open/submitting state
 * and the service call stay with the page — the three copy endpoints return
 * different shapes and only the page knows what a successful copy should say
 * or reload.
 */
export function CopyFromTeamDialog({
  open,
  onOpenChange,
  team,
  title,
  description,
  submitting,
  onConfirm,
  onSourceChange,
  preview,
  canCopyFrom,
  confirmLabel = 'Copy',
  submittingLabel = 'Copying…',
}: Readonly<CopyFromTeamDialogProps>) {
  const [sourceTeam, setSourceTeam] = useState<Team | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  // Reset on OPEN, not on close: the page may close the dialog itself after a
  // successful copy, and resetting from `open`'s falsy edge alone would then
  // depend on the page's callback identity inside the effect.
  useEffect(() => {
    if (open) {
      setSourceTeam(null)
      setPickerOpen(false)
    }
  }, [open])

  const handleSelect = (selected: Team) => {
    setSourceTeam(selected)
    setPickerOpen(false)
    onSourceChange?.(selected)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="copy-source-team">Copy from</Label>
            <SourceTeamPicker
              id="copy-source-team"
              destinationTeam={team}
              value={sourceTeam?.id ?? null}
              onChange={handleSelect}
              canCopyFrom={canCopyFrom}
              disabled={submitting}
              open={pickerOpen}
              onOpenChange={setPickerOpen}
            />
          </div>

          {sourceTeam && preview}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={submitting}
            onClick={() => {
              onOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            data-testid="confirm-copy-from-team-button"
            disabled={submitting || !sourceTeam}
            onClick={() => {
              if (sourceTeam) void onConfirm(sourceTeam)
            }}
          >
            {submitting ? submittingLabel : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
