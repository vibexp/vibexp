import {
  BookOpen,
  FileText,
  HardDrive,
  Home,
  type LucideIcon,
  Package,
  Rss,
  Search as SearchIcon,
  Server,
  Settings as SettingsIcon,
  Users,
  Wrench,
} from 'lucide-react'

export interface NavItem {
  label: string
  href: string
  icon: LucideIcon
  children?: { label: string; href: string }[]
}

/**
 * Sidebar navigation, grouped into design-system style sections. Each group
 * renders an uppercase muted label above its links (mirrors the DS docs
 * sidebar: "GET STARTED / FOUNDATIONS / LIBRARY"). Group labels are hidden in
 * the collapsed icon rail (md–lg) where there's no room for them.
 */
export interface NavGroup {
  label: string
  items: NavItem[]
}

export const NAV_GROUPS: NavGroup[] = [
  {
    label: 'General',
    items: [
      { label: 'Dashboard', href: '/', icon: Home },
      { label: 'Search', href: '/search', icon: SearchIcon },
      { label: 'AI Feeds', href: '/feeds', icon: Rss },
    ],
  },
  {
    label: 'Workspace',
    items: [
      {
        label: 'AI Tools',
        href: '/ai-tools/overview',
        icon: Wrench,
        children: [
          { label: 'Claude Code', href: '/ai-tools/claude-code/overview' },
          { label: 'Cursor IDE', href: '/ai-tools/cursor-ide/overview' },
        ],
      },
      {
        label: 'Prompts',
        href: '/prompts',
        icon: FileText,
        children: [
          { label: 'My Prompts', href: '/prompts' },
          { label: 'Prompt Gallery', href: '/prompt-gallery' },
        ],
      },
      { label: 'Artifacts', href: '/artifacts', icon: Package },
      { label: 'Blueprints', href: '/blueprints', icon: BookOpen },
      { label: 'Memories', href: '/memories', icon: HardDrive },
      { label: 'Agents', href: '/agents', icon: Users },
    ],
  },
  {
    label: 'System',
    items: [
      { label: 'MCP Server', href: '/mcp-servers/vibexp-mcp', icon: Server },
      { label: 'Settings', href: '/settings', icon: SettingsIcon },
    ],
  },
]

/**
 * Flattened list of all nav items, preserving group order. Kept for consumers
 * that need a single sequence (e.g. breadcrumb route matching) and for
 * backwards compatibility with components that don't care about grouping.
 */
export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap(g => g.items)
