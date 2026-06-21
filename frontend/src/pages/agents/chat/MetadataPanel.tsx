import { X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

import type { ExecutionMetadata } from './types'

interface MetadataPanelProps {
  metadata: ExecutionMetadata
  onClose: () => void
}

export function MetadataPanel({ metadata, onClose }: MetadataPanelProps) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="grid flex-1 grid-cols-1 gap-3 md:grid-cols-3">
            <div>
              <div className="text-muted-foreground mb-0.5 text-xs font-medium">
                Task ID
              </div>
              <div className="font-mono text-sm">{metadata.taskId}</div>
            </div>
            {metadata.status && (
              <div>
                <div className="text-muted-foreground mb-0.5 text-xs font-medium">
                  Status
                </div>
                <div className="text-sm capitalize">{metadata.status}</div>
              </div>
            )}
            {metadata.started && (
              <div>
                <div className="text-muted-foreground mb-0.5 text-xs font-medium">
                  Started
                </div>
                <div className="text-sm">
                  {new Date(metadata.started).toLocaleString()}
                </div>
              </div>
            )}
            {metadata.duration !== undefined && (
              <div>
                <div className="text-muted-foreground mb-0.5 text-xs font-medium">
                  Duration
                </div>
                <div className="text-sm">{metadata.duration.toFixed(2)}s</div>
              </div>
            )}
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="Close metadata"
          >
            <X className="size-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
