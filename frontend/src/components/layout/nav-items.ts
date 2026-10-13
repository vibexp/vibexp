import {
  BookOpen,
  FileText,
  HardDrive,
  Home,
  type LucideIcon,
  Package,
  Plug,
  Rss,
  Search as SearchIcon,
  Settings as SettingsIcon,
  Users,
  UsersRound,
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
      // One entry for every way of connecting a tool (MCP, CLI, REST API, API
      // client): setup pages visited once, which used to take four rows of
      // their own. The tabs live on the page.
      { label: 'Connect', href: '/connect', icon: Plug },
      // `UsersRound`, not `Users`: the Agents entry above already uses `Users`,
      // and the collapsed icon rail (md-lg) shows icons only - two identical
      // glyphs would be indistinguishable there.
      { label: 'Teams', href: '/teams', icon: UsersRound },
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
