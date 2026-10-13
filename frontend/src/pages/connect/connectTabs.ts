/**
 * The Connect page's tabs — the four ways to plug a tool into VibeXP — in the
 * order they are offered. MCP is first and the default: connecting an AI tool
 * over MCP is what starts the read / write-back loop.
 *
 * The tab is part of the URL (`/connect/<slug>`) so each can be linked to, and
 * every in-app link goes through {@link CONNECT_PATHS} rather than spelling a
 * path, so a tab can never be linked by a stale address.
 */
export const CONNECT_TABS = [
  { slug: 'mcp', label: 'MCP' },
  { slug: 'cli', label: 'CLI' },
  { slug: 'rest-api', label: 'REST API' },
  { slug: 'api-client', label: 'API client' },
] as const

export type ConnectTabSlug = (typeof CONNECT_TABS)[number]['slug']

export const CONNECT_BASE_PATH = '/connect'
export const DEFAULT_CONNECT_TAB: ConnectTabSlug = 'mcp'

export const connectPath = (slug: ConnectTabSlug): string =>
  `${CONNECT_BASE_PATH}/${slug}`

export const CONNECT_PATHS = {
  mcp: connectPath('mcp'),
  cli: connectPath('cli'),
  restApi: connectPath('rest-api'),
  apiClient: connectPath('api-client'),
} as const

export const isConnectTab = (
  value: string | undefined
): value is ConnectTabSlug => CONNECT_TABS.some(tab => tab.slug === value)
