import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router'

import { Button } from '@/components/ui/button'

import { DashboardSection } from './DashboardSection'

export const MCP_SETUP_PATH = '/mcp-servers/vibexp-mcp'

const CREATE_ENTRY_POINTS: { label: string; to: string }[] = [
  { label: 'Prompt', to: '/prompts/new' },
  { label: 'Blueprint', to: '/blueprints/new' },
  { label: 'Memory', to: '/memories/new' },
  { label: 'Artifact', to: '/artifacts/new' },
]

/**
 * What an empty workspace shows instead of the usage and change blocks: the
 * two steps that start the loop, in the order that matters. Connecting an AI
 * tool comes first — it is what makes the workspace get read and written to —
 * and creating something by hand second.
 */
export function GettingStarted() {
  return (
    <DashboardSection
      title="Get started"
      description="This workspace is empty. Two steps start the loop."
    >
      <ol className="divide-y rounded-lg border">
        <li className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium">1. Connect an AI tool</p>
            <p className="text-muted-foreground text-sm">
              Claude Code, Cursor and other MCP tools read this workspace before
              a task and write back what they learn.
            </p>
          </div>
          <Button size="sm" asChild className="shrink-0">
            <Link to={MCP_SETUP_PATH}>
              Set up MCP
              <ArrowRight className="ml-1 size-4" aria-hidden="true" />
            </Link>
          </Button>
        </li>
        <li className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium">2. Add your first resource</p>
            <p className="text-muted-foreground text-sm">
              Give it something to start from, or let the connected tool create
              it.
            </p>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            {CREATE_ENTRY_POINTS.map(entry => (
              <Button key={entry.label} variant="outline" size="sm" asChild>
                <Link to={entry.to}>+ {entry.label}</Link>
              </Button>
            ))}
          </div>
        </li>
      </ol>
    </DashboardSection>
  )
}
