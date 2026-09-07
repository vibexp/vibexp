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

export const resourceRegistry: Readonly<
  Record<ResourceKindKey, ResourceDescriptor>
> = Object.freeze(descriptors)

/** Looks a descriptor up by kind. Total over `ResourceKindKey`, so it cannot miss. */
export function getResourceDescriptor(
  kind: ResourceKindKey
): ResourceDescriptor {
  return resourceRegistry[kind]
}
