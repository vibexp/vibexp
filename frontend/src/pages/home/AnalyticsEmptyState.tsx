import { BarChart3 } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

const RESOURCE_ENTRY_POINTS: { label: string; to: string }[] = [
  { label: 'Artifact', to: '/artifacts/new' },
  { label: 'Memory', to: '/memories/new' },
  { label: 'Blueprint', to: '/blueprints/new' },
  { label: 'Prompt', to: '/prompts/new' },
]

/**
 * The Analytics section's empty state for a brand-new workspace: a single panel
 * inviting the user to create their first resource, replacing the four charts
 * until there is data to show.
 */
export function AnalyticsEmptyState() {
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-4 py-16 text-center">
        <div className="bg-muted text-muted-foreground flex size-12 items-center justify-center rounded-full">
          <BarChart3 className="size-6" />
        </div>
        <div className="space-y-1">
          <h3 className="text-lg font-semibold">
            Start building your workspace
          </h3>
          <p className="text-muted-foreground mx-auto max-w-md text-sm">
            Create your first resources and your access, creation and growth
            trends will appear here.
          </p>
        </div>
        <div className="flex flex-wrap justify-center gap-2">
          {RESOURCE_ENTRY_POINTS.map(entry => (
            <Button key={entry.label} variant="outline" size="sm" asChild>
              <Link to={entry.to}>+ {entry.label}</Link>
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
