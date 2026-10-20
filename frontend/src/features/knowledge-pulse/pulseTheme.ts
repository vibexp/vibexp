import type { PulseEventKind, PulseResourceType } from './types'

/**
 * Resource-type colours. Deliberately avoids the three event hues (blue
 * read, amber write, green link) so a pulse always stands out against the
 * dot it lands on. All tokens; the rose is a mix of two.
 */
export const RESOURCE_TYPE_COLOR: Record<PulseResourceType, string> = {
  memory: 'var(--chart-5)',
  artifact: 'var(--chart-1)',
  prompt: 'var(--chart-3)',
  blueprint: 'color-mix(in oklch, var(--destructive) 60%, var(--chart-5))',
  feed_item: 'var(--muted-foreground)',
}

export const RESOURCE_TYPE_LABEL: Record<PulseResourceType, string> = {
  memory: 'Memories',
  artifact: 'Artifacts',
  prompt: 'Prompts',
  blueprint: 'Blueprints',
  feed_item: 'Feed',
}

export const RESOURCE_TYPE_SINGULAR: Record<PulseResourceType, string> = {
  memory: 'memory',
  artifact: 'artifact',
  prompt: 'prompt',
  blueprint: 'blueprint',
  feed_item: 'feed post',
}

/** Event colours from the semantic status tokens: blue read, amber write, green link. */
export const EVENT_KIND_COLOR: Record<PulseEventKind, string> = {
  read: 'var(--info)',
  write: 'var(--warning)',
  link: 'var(--success)',
  feed_post: 'var(--foreground)',
}

export const EVENT_KIND_LABEL: Record<PulseEventKind, string> = {
  read: 'Read',
  write: 'Write',
  link: 'Link',
  feed_post: 'Feed post',
}
