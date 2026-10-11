import type { components } from '@vibexp/api-client'

// Generated wire types for the resource lock (epic #1285). Prompts, blueprints
// and artifacts carry the same `lock` shape and share one history event type.
export type ResourceLock = components['schemas']['ResourceLock']
export type ResourceLockEvent = components['schemas']['ResourceLockEvent']

/** The resource kinds that can be locked. Memories cannot. */
export type LockableKind = 'prompt' | 'blueprint' | 'artifact'
