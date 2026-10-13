import type { Channel } from '@/components/channelSeries'

/**
 * Whether a last write is one a row marks: it came from MCP, the CLI or the
 * API. A person editing in the web app is the unmarked case, and `null` (no
 * recorded write) is never shown as a channel. For a caller that has to know
 * before it renders, to place a separator or to add the line at all.
 */
export function marksLastWrite(
  source: Channel | null | undefined
): source is Exclude<Channel, 'web'> {
  return source != null && source !== 'web'
}

/** The two last-write fields of a prompt, artifact, blueprint or memory. */
export interface LastWrite {
  last_write_source: Channel | null
  last_write_assistant_name: string | null
}
