import type { LucideIcon } from 'lucide-react'
import {
  Cpu,
  Database,
  FolderGit2,
  Mail,
  SlidersHorizontal,
  Sparkles,
  Timer,
} from 'lucide-react'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import type { AdminTeamListItem } from '@/services/adminService'

type TeamConfiguration = AdminTeamListItem['configuration']

interface SetupIndicator {
  key: keyof TeamConfiguration
  label: string
  icon: LucideIcon
  onText: string
  offText: string
}

/** Fixed order, so an admin can scan one indicator down a column of rows. */
const SETUP_INDICATORS: readonly SetupIndicator[] = [
  {
    key: 'embedding_configured',
    label: 'Embedding',
    icon: Database,
    onText: 'configured',
    offText: 'not configured',
  },
  {
    key: 'llm_configured',
    label: 'LLM',
    icon: Cpu,
    onText: 'configured',
    offText: 'not configured',
  },
  {
    key: 'ai_summary_enabled',
    label: 'AI summary',
    icon: Sparkles,
    onText: 'enabled',
    offText: 'disabled',
  },
  {
    key: 'email_configured',
    label: 'Email',
    icon: Mail,
    onText: 'configured',
    offText: 'not configured',
  },
  {
    key: 'github_configured',
    label: 'GitHub',
    icon: FolderGit2,
    onText: 'configured',
    offText: 'not configured',
  },
  {
    key: 'search_settings_customized',
    label: 'Search settings',
    icon: SlidersHorizontal,
    onText: 'customized',
    offText: 'instance defaults',
  },
  {
    key: 'freshness_enabled',
    label: 'Freshness',
    icon: Timer,
    onText: 'enabled',
    offText: 'disabled',
  },
]

/**
 * The "Setup" cell of the admin teams list (#1139): one icon per configuration
 * flag, every one always rendered.
 *
 * State is carried by text first — each icon's accessible name and tooltip read
 * e.g. "Embedding: not configured" — and visually by opacity, never by colour
 * alone. Each trigger is a real inline-flex box: a `display: contents` trigger
 * has no box for floating-ui to measure and anchors the tooltip on the viewport
 * origin (#891).
 */
export function TeamSetupIndicators({
  configuration,
}: Readonly<{ configuration: TeamConfiguration }>) {
  return (
    <TooltipProvider>
      <div className="flex items-center gap-1">
        {SETUP_INDICATORS.map(({ key, label, icon: Icon, onText, offText }) => {
          const on = configuration[key]
          const text = `${label}: ${on ? onText : offText}`
          return (
            <Tooltip key={key}>
              <TooltipTrigger asChild>
                <span
                  role="img"
                  aria-label={text}
                  className={cn(
                    'inline-flex size-5 items-center justify-center',
                    on ? 'text-foreground' : 'text-muted-foreground opacity-30'
                  )}
                >
                  <Icon className="size-4" aria-hidden="true" />
                </span>
              </TooltipTrigger>
              <TooltipContent>{text}</TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    </TooltipProvider>
  )
}
