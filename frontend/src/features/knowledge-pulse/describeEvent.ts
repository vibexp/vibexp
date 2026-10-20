import { RESOURCE_TYPE_SINGULAR } from './pulseTheme'
import type { PulseActor, PulseEvent } from './types'

const SOURCE_LABEL: Record<PulseActor['source'], string> = {
  mcp: 'MCP',
  cli: 'CLI',
  web: 'the web',
  api: 'the API',
}

export interface EventSentence {
  verb: string
  object: string
  /** Second resource of a link ("… to *Y*"). */
  target?: string
  /** "via MCP", "built-from", … */
  suffix?: string
}

/**
 * The live feed's sentence for one event, split into parts so the row can
 * style the resource titles: "Claude Code **read** *Deploy runbook* via MCP".
 */
export function describeEvent(event: PulseEvent): EventSentence {
  const title =
    event.resource.title !== ''
      ? event.resource.title
      : `a ${RESOURCE_TYPE_SINGULAR[event.resource.type]}`
  const suffix = `via ${SOURCE_LABEL[event.actor.source]}`
  switch (event.kind) {
    case 'read':
      return { verb: 'read', object: title, suffix }
    case 'write':
      return {
        verb: event.detail?.action === 'create' ? 'created' : 'updated',
        object: title,
        suffix,
      }
    case 'link':
      return {
        verb: 'linked',
        object: title,
        target: event.detail?.to?.title ?? 'another item',
        suffix: event.detail?.relation_type,
      }
    case 'feed_post':
      return { verb: 'posted', object: title, suffix: 'to the feed' }
  }
}

/** Short ticking age: "4s", "2m", "1h". */
export function shortAge(at: string, now: number): string {
  const s = Math.max(0, Math.floor((now - Date.parse(at)) / 1000))
  if (s < 60) return `${String(s)}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${String(m)}m`
  return `${String(Math.floor(m / 60))}h`
}
