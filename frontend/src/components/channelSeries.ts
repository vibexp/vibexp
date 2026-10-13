/** The four channels a read or a write can arrive through. */
export type Channel = 'web' | 'cli' | 'mcp' | 'api'

/**
 * One colour per channel, the same wherever channels are drawn: the access
 * chart's stack and the dashboard's read and write figures.
 */
export const CHANNEL_FILL: Readonly<Record<Channel, string>> = {
  web: 'var(--chart-1)',
  cli: 'var(--chart-2)',
  mcp: 'var(--chart-3)',
  api: 'var(--chart-4)',
}

export const CHANNEL_LABEL: Readonly<Record<Channel, string>> = {
  web: 'Web',
  cli: 'CLI',
  mcp: 'MCP',
  api: 'API',
}
