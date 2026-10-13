import type { ComponentType } from 'react'
import { Navigate, NavLink, useParams } from 'react-router'

import { cn } from '@/lib/utils'
import { ApiClient } from '@/pages/integrations/ApiClient'
import { CliPage } from '@/pages/integrations/CLI'
import { RestApis } from '@/pages/integrations/RestApis'
import { VibeXPMCP } from '@/pages/mcp/VibeXPMCP'

import {
  CONNECT_TABS,
  connectPath,
  type ConnectTabSlug,
  DEFAULT_CONNECT_TAB,
  isConnectTab,
} from './connectTabs'

const TAB_CONTENT: Record<ConnectTabSlug, ComponentType> = {
  mcp: VibeXPMCP,
  cli: CliPage,
  'rest-api': RestApis,
  'api-client': ApiClient,
}

/**
 * One page for every way of connecting a tool to VibeXP: MCP, the CLI, the
 * REST API and the generated API clients. They are setup pages a team visits
 * once, so they share one navigation entry instead of four.
 *
 * Each tab is the page it used to be, unchanged — including its own `<h1>`,
 * so the page has exactly one. The tabs are real links to `/connect/<tab>`:
 * a tab can be bookmarked, opened in a new tab, and reached by the redirects
 * from the four retired URLs.
 */
export function Connect() {
  const { tab } = useParams<{ tab: string }>()
  if (!isConnectTab(tab)) {
    return <Navigate to={connectPath(DEFAULT_CONNECT_TAB)} replace />
  }
  const Content = TAB_CONTENT[tab]

  return (
    // No width of its own: like every non-reading page it fills the shell's
    // one container, so its left edge is where every other page's is.
    <div data-testid="connect-page">
      <nav aria-label="Connect" className="mb-8 flex gap-1 border-b">
        {CONNECT_TABS.map(({ slug, label }) => (
          <NavLink
            key={slug}
            to={connectPath(slug)}
            // An underlined row of links, not a tablist: each goes to a URL.
            className={({ isActive }) =>
              cn(
                '-mb-px shrink-0 border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'border-foreground text-foreground'
                  : 'text-muted-foreground hover:text-foreground border-transparent'
              )
            }
          >
            {label}
          </NavLink>
        ))}
      </nav>
      <Content />
    </div>
  )
}
