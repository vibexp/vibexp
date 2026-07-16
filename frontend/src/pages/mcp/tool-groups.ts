import type { MCPTool } from './mcp-tools'

/** Read vs. write classification shown as a badge on each tool row. */
export type ToolKind = 'read' | 'write'

export interface ToolGroup {
  id: string
  label: string
  /** lucide-react icon name resolved by the caller. */
  icon: 'package' | 'database' | 'layout' | 'users' | 'compass'
  tools: MCPTool[]
}

/**
 * Tools that mutate state. Everything else is treated as a read. Kept as an
 * explicit allow-list (rather than a name heuristic) so a new read tool that
 * happens to contain "post" or "set" in its name isn't mislabelled as a write.
 */
const WRITE_TOOLS = new Set([
  'vibexp_io_create_artifact',
  'vibexp_io_update_artifact',
  'vibexp_io_create_memory',
  'vibexp_io_update_memory',
  'vibexp_io_post_to_feed',
  'vibexp_io_reply_to_feed_item',
])

export function getToolKind(tool: MCPTool): ToolKind {
  return WRITE_TOOLS.has(tool.name) ? 'write' : 'read'
}

/**
 * First sentence of the tool description, used as the one-line summary shown on
 * a collapsed tool row. Falls back to a truncated slice when no sentence break
 * is found within a reasonable length.
 */
const SUMMARY_MAX = 120

export function getToolSummary(tool: MCPTool): string {
  const text = tool.description.trim()
  const sentenceEnd = text.indexOf('. ')
  if (sentenceEnd !== -1 && sentenceEnd <= SUMMARY_MAX) {
    return text.slice(0, sentenceEnd + 1)
  }
  return text.length > SUMMARY_MAX
    ? `${text.slice(0, SUMMARY_MAX).trimEnd()}…`
    : text
}

interface GroupDef {
  id: string
  label: string
  icon: ToolGroup['icon']
  match: (name: string) => boolean
}

/** Order here is the order the groups render in. */
const GROUP_DEFS: GroupDef[] = [
  {
    id: 'artifacts',
    label: 'Artifacts',
    icon: 'package',
    match: name => name.includes('_artifact'),
  },
  {
    id: 'memories',
    label: 'Memories',
    icon: 'database',
    match: name => name.includes('_memor'),
  },
  {
    id: 'resources',
    label: 'Resources',
    icon: 'package',
    match: name => name.includes('_resource'),
  },
  {
    id: 'projects-feeds',
    label: 'Projects & Feeds',
    icon: 'layout',
    match: name => name.includes('_project') || name.includes('_feed'),
  },
  {
    id: 'teams',
    label: 'Teams',
    icon: 'users',
    match: name => name.includes('_team'),
  },
  {
    id: 'account',
    label: 'Account & Search',
    icon: 'compass',
    match: () => true,
  },
]

/**
 * Buckets the flat tool list into ordered, labelled groups. Each tool lands in
 * the first matching group, so the catch-all "Account & Search" group captures
 * anything not claimed by a more specific group above it.
 */
export function groupTools(tools: MCPTool[]): ToolGroup[] {
  const remaining = new Set(tools)
  const groups: ToolGroup[] = []

  for (const def of GROUP_DEFS) {
    const claimed = [...remaining].filter(tool => def.match(tool.name))
    if (claimed.length === 0) continue
    claimed.forEach(tool => remaining.delete(tool))
    groups.push({
      id: def.id,
      label: def.label,
      icon: def.icon,
      tools: claimed,
    })
  }

  return groups
}
