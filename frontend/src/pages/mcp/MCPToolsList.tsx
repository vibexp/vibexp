import {
  ChevronRight,
  Compass,
  Database,
  Layout,
  type LucideIcon,
  Package,
  Search,
  Users,
} from 'lucide-react'
import { useMemo, useState } from 'react'

import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import type { MCPTool } from './mcp-tools'
import {
  getToolKind,
  getToolSummary,
  groupTools,
  type ToolGroup,
} from './tool-groups'

interface MCPToolsListProps {
  tools: MCPTool[]
  expandedTools: Set<string>
  onToggleTool: (toolName: string) => void
}

const GROUP_ICONS: Record<ToolGroup['icon'], LucideIcon> = {
  package: Package,
  database: Database,
  layout: Layout,
  users: Users,
  compass: Compass,
}

function KindBadge({ kind }: Readonly<{ kind: 'read' | 'write' }>) {
  return (
    <span
      className={cn(
        'shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold uppercase tracking-wide',
        kind === 'write'
          ? 'bg-warning-subtle text-warning'
          : 'bg-info-subtle text-info'
      )}
    >
      {kind}
    </span>
  )
}

function ToolRow({
  tool,
  open,
  onToggle,
}: Readonly<{
  tool: MCPTool
  open: boolean
  onToggle: () => void
}>) {
  const params = Object.entries(tool.inputSchema.properties)

  return (
    <div
      className={cn(
        'bg-card overflow-hidden rounded-lg border transition-colors',
        open && 'border-ring/50'
      )}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full items-center gap-3 px-4 py-3.5 text-left"
      >
        <ChevronRight
          className={cn(
            'text-muted-foreground size-4 shrink-0 transition-transform',
            open && 'rotate-90'
          )}
        />
        <span className="text-foreground font-mono text-sm font-semibold">
          {tool.name}
        </span>
        {!open && (
          <span className="text-muted-foreground min-w-0 flex-1 truncate text-sm">
            {getToolSummary(tool)}
          </span>
        )}
        <span className={cn('ml-auto', open && 'ml-0')}>
          <KindBadge kind={getToolKind(tool)} />
        </span>
      </button>

      {open && (
        <div className="pb-[18px] pl-11 pr-4 pt-0.5">
          <p className="text-foreground text-sm">{tool.description}</p>
          {params.length > 0 && (
            <>
              <div className="text-muted-foreground mb-2 mt-4 text-xs font-semibold uppercase tracking-wide">
                Parameters
              </div>
              <div className="flex flex-col gap-[7px]">
                {params.map(([name, param]) => (
                  <div
                    key={name}
                    className="bg-muted/45 rounded-md border px-[13px] py-[11px]"
                  >
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-sm font-semibold">
                        {name}
                      </span>
                      {tool.inputSchema.required.includes(name) && (
                        <span className="bg-destructive/15 text-destructive rounded-full px-1.5 py-px text-xs font-bold uppercase tracking-wide">
                          required
                        </span>
                      )}
                      <span className="text-muted-foreground ml-auto font-mono text-xs">
                        {param.type}
                      </span>
                    </div>
                    <p className="text-muted-foreground mt-[5px] text-sm">
                      {param.description}
                    </p>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

export function MCPToolsList({
  tools,
  expandedTools,
  onToggleTool,
}: Readonly<MCPToolsListProps>) {
  const [query, setQuery] = useState('')

  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = q
      ? tools.filter(
          t =>
            t.name.toLowerCase().includes(q) ||
            t.description.toLowerCase().includes(q)
        )
      : tools
    return groupTools(filtered)
  }, [tools, query])

  return (
    <div>
      <div className="relative mb-[18px]">
        <Search className="text-muted-foreground absolute left-3 top-1/2 size-4 -translate-y-1/2" />
        <Input
          type="search"
          placeholder="Filter tools…"
          value={query}
          onChange={e => {
            setQuery(e.target.value)
          }}
          className="pl-[38px]"
          aria-label="Filter tools"
        />
      </div>

      {groups.map(group => {
        const Icon = GROUP_ICONS[group.icon]
        return (
          <div key={group.id}>
            <div className="text-muted-foreground mb-[10px] mt-[22px] flex items-center gap-[9px] text-xs font-semibold uppercase tracking-wide">
              <Icon className="size-3.5" />
              {group.label}
              <span className="text-muted-foreground font-medium normal-case tracking-normal">
                · {group.tools.length}
              </span>
            </div>
            <div className="flex flex-col gap-2">
              {group.tools.map(tool => (
                <ToolRow
                  key={tool.name}
                  tool={tool}
                  open={expandedTools.has(tool.name)}
                  onToggle={() => {
                    onToggleTool(tool.name)
                  }}
                />
              ))}
            </div>
          </div>
        )
      })}

      {groups.length === 0 && (
        <p className="text-muted-foreground text-sm">
          No tools match “{query}”.
        </p>
      )}
    </div>
  )
}
