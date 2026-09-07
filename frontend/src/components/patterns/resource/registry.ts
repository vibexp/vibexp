import { artifactDescriptor } from './descriptors/artifact'
import { blueprintDescriptor } from './descriptors/blueprint'
import { galleryPromptDescriptor } from './descriptors/galleryPrompt'
import { memoryDescriptor } from './descriptors/memory'
import { promptDescriptor } from './descriptors/prompt'
import type { ResourceDescriptor } from './types'

/**
 * Every resource kind the SPA knows about. The first four are team resources
 * and their keys are also the API's resource-type discriminators (they match
 * `ResourceKind` in `components/resource-detail/ResourceReadingPage`, which the
 * registry test pins); `gallery-prompt` is descriptor-only — the public gallery
 * has no team-scoped resource id.
 */
const descriptors = {
  prompt: promptDescriptor,
  artifact: artifactDescriptor,
  blueprint: blueprintDescriptor,
  memory: memoryDescriptor,
  'gallery-prompt': galleryPromptDescriptor,
}

/** The kinds `resourceRegistry` is keyed by. */
export type ResourceKindKey = keyof typeof descriptors

/**
 * `Object.freeze` is shallow, and the registry is a singleton every page reads —
 * a stray write to one descriptor's `fields` or `capabilities` would corrupt the
 * app globally. The `readonly` members on `ResourceDescriptor` stop that at
 * compile time; this stops it at runtime too.
 */
function deepFreeze<T>(value: T): T {
  if (value !== null && typeof value === 'object') {
    Object.values(value).forEach(deepFreeze)
    Object.freeze(value)
  }
  return value
}

export const resourceRegistry: Readonly<
  Record<ResourceKindKey, ResourceDescriptor>
> = deepFreeze(descriptors)

/** Looks a descriptor up by kind. Total over `ResourceKindKey`, so it cannot miss. */
export function getResourceDescriptor(
  kind: ResourceKindKey
): ResourceDescriptor {
  return resourceRegistry[kind]
}
