import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { GitHubRepository } from '@/services/githubIntegrationService'

interface ImportProjectModalProps {
  isOpen: boolean
  repository: GitHubRepository
  onClose: () => void
  onConfirm: () => void
  isLoading: boolean
}

export function ImportProjectModal({
  isOpen,
  repository,
  onClose,
  onConfirm,
  isLoading,
}: ImportProjectModalProps) {
  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open && !isLoading) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Import as Project</DialogTitle>
        </DialogHeader>

        <p className="text-muted-foreground text-sm">
          This will create a new project in your team workspace with the
          following details:
        </p>

        <div className="space-y-3">
          <div>
            <p className="text-muted-foreground mb-0.5 text-xs font-medium">
              Name
            </p>
            <p className="text-sm">{repository.name}</p>
          </div>
          <div>
            <p className="text-muted-foreground mb-0.5 text-xs font-medium">
              Description
            </p>
            <p className="text-sm">
              {repository.description ?? (
                <span className="text-muted-foreground italic">
                  No description
                </span>
              )}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground mb-0.5 text-xs font-medium">
              Repository URL
            </p>
            <p className="text-sm break-all">{repository.html_url}</p>
          </div>
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
