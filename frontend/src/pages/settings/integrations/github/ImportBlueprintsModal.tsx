import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { GitHubRepository } from '@/types/github'

interface ImportBlueprintsModalProps {
  isOpen: boolean
  repository: GitHubRepository
  onClose: () => void
  onConfirm: () => void
  isLoading: boolean
}

export function ImportBlueprintsModal({
  isOpen,
  repository,
  onClose,
  onConfirm,
  isLoading,
}: ImportBlueprintsModalProps) {
  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open && !isLoading) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Import Blueprints</DialogTitle>
        </DialogHeader>

        <p className="text-muted-foreground text-sm">
          This will scan the repository{' '}
          <strong className="text-foreground">{repository.name}</strong> for AI
          assistant configuration files and import them as blueprints.
        </p>

        <div className="bg-muted rounded-md p-3">
          <p className="mb-2 text-sm font-medium">Files to be scanned:</p>
          <ul className="text-muted-foreground list-inside list-disc space-y-1 text-sm">
            <li>.claude directory (markdown files only)</li>
            <li>.cursor directory (markdown files only)</li>
            <li>.codex directory (markdown files only)</li>
            <li>.agents directory (markdown files only)</li>
            <li>CLAUDE.md (root file)</li>
            <li>AGENTS.md (root file)</li>
            <li>CURSOR.md (root file)</li>
          </ul>
          <p className="text-muted-foreground mt-2 text-xs">
            Only markdown (.md) files will be imported. Other file types will be
            skipped.
          </p>
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={onClose} disabled={isLoading}>
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={isLoading}>
            {isLoading ? 'Importing…' : 'Yes, Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
