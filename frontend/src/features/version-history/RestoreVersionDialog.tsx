import * as AlertDialogPrimitive from '@radix-ui/react-alert-dialog'
import { Info, RotateCcw } from 'lucide-react'

import { Button } from '@/components/ui/button'

interface RestoreVersionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The snapshot version the user is restoring.
  versionNumber: number | null
  // The version the live draft will be snapshotted as before the restore.
  nextVersionNumber: number
  loading?: boolean
  onConfirm: () => void
}

// Non-destructive restore confirmation (design `RestoreModal` / `.vhd-*`).
// Restore must ALWAYS route through this — the backend snapshots the current
// draft as a new version first, and the copy says so explicitly.
export function RestoreVersionDialog({
  open,
  onOpenChange,
  versionNumber,
  nextVersionNumber,
  loading,
  onConfirm,
}: Readonly<RestoreVersionDialogProps>) {
  return (
    <AlertDialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <AlertDialogPrimitive.Portal>
        <AlertDialogPrimitive.Overlay className="vhd-overlay data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
        <AlertDialogPrimitive.Content
          className="vh-root vhd-modal fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
          data-testid="restore-version-dialog"
        >
          <div className="vhd-ico" aria-hidden="true">
            <RotateCcw />
          </div>
          <AlertDialogPrimitive.Title className="vhd-title">
            Restore Version {versionNumber ?? ''}?
          </AlertDialogPrimitive.Title>
          <AlertDialogPrimitive.Description className="vhd-text">
            This makes the content of <b>Version {versionNumber ?? ''}</b> the
            live draft. Your current content won&rsquo;t be lost — VibeXP saves
            it as <b>Version {nextVersionNumber}</b> first, so you can always
            come back.
          </AlertDialogPrimitive.Description>
          <div className="vhd-note">
            <Info aria-hidden="true" />
            <span>
              Restoring is non-destructive. Every restore is itself a new
              version in the timeline.
            </span>
          </div>
          <div className="vhd-actions">
            <AlertDialogPrimitive.Cancel asChild>
              <Button variant="ghost" size="sm" disabled={loading}>
                Cancel
              </Button>
            </AlertDialogPrimitive.Cancel>
            <Button
              size="sm"
              disabled={loading}
              data-testid="confirm-restore-button"
              onClick={onConfirm}
            >
              <RotateCcw />
              {loading ? 'Restoring…' : 'Restore as new version'}
            </Button>
          </div>
        </AlertDialogPrimitive.Content>
      </AlertDialogPrimitive.Portal>
    </AlertDialogPrimitive.Root>
  )
}
