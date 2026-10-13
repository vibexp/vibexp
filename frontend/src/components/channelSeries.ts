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

/**
 * The one order channels are listed in, wherever they are listed: the ones an
 * AI tool uses first, then people. Figures, chart legends, chart stacks (MCP
 * at the bottom, so the AI share has a common baseline) and filters all read
 * it, so a reader never has to re-find a channel.
 */
export const CHANNEL_ORDER: readonly Channel[] = ['mcp', 'cli', 'api', 'web']

/** {@link CHANNEL_ORDER} as bar-chart series. */
export const CHANNEL_SERIES = CHANNEL_ORDER.map(key => ({
  key,
  label: CHANNEL_LABEL[key],
  fill: CHANNEL_FILL[key],
}))
