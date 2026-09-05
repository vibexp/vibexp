import {
  Activity,
  Info,
  MessageSquare,
  Paperclip,
  Workflow,
} from 'lucide-react'
import type { ReactNode } from 'react'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import { CommentsPanel } from '@/components/comments/CommentsPanel'
import {
  ReadingPage,
  type ReadingPageProps,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { RelationsPanel } from '@/components/relations/RelationsPanel'

/** The resource kinds every standard side panel understands. */
export type ResourceKind = 'artifact' | 'prompt' | 'blueprint' | 'memory'

export interface ResourceRef {
  kind: ResourceKind
  id: string
  teamId: string
}

/** Section ids, exported so tests and rail deep-links can address them. */
export const RESOURCE_SECTION_IDS = {
  metadata: 'metadata',
  attachments: 'attachments',
  activity: 'activity',
  comments: 'comments',
  relations: 'relations',
} as const

export interface ResourceReadingPageProps extends Omit<
  ReadingPageProps,
  'sections'
> {
  /**
   * The resource the standard panels are about. Omit while loading or when
   * the resource could not be found — the page then renders without them.
   */
  resource?: ResourceRef
  /** Content of the Metadata section (MetadataPanel, tags, custom metadata, …). */
  metadata?: ReactNode
  /** Whether the resource supports attachments. Defaults to true. */
  attachments?: boolean
  /** Resource-specific sections appended after the standard ones. */
  extraSections?: readonly ReadingSection[]
}

/**
 * `ReadingPage` plus the section set every team resource shares — Metadata,
 * Attachments, Access activity, Comments, Relations — in a fixed order, so
 * each resource type (and any resource type added later) gets the identical
 * details panel by describing itself rather than laying itself out.
 */
export function ResourceReadingPage({
  resource,
  metadata,
  attachments = true,
  extraSections = [],
  ...pageProps
}: Readonly<ResourceReadingPageProps>) {
  const sections: ReadingSection[] = [
    {
      id: RESOURCE_SECTION_IDS.metadata,
      label: 'Metadata',
      icon: Info,
      content: metadata,
    },
    {
      id: RESOURCE_SECTION_IDS.attachments,
      label: 'Attachments',
      icon: Paperclip,
      content: resource && attachments && (
        <ResourceAttachments
          teamId={resource.teamId}
          ownerType={resource.kind}
          ownerId={resource.id}
        />
      ),
    },
    {
      id: RESOURCE_SECTION_IDS.activity,
      label: 'Access activity',
      icon: Activity,
      content: resource && (
        <AccessActivityPanel
          teamId={resource.teamId}
          resourceType={resource.kind}
          resourceId={resource.id}
        />
      ),
    },
    {
      id: RESOURCE_SECTION_IDS.comments,
      label: 'Comments',
      icon: MessageSquare,
      content: resource && (
        <CommentsPanel
          teamId={resource.teamId}
          resourceType={resource.kind}
          resourceId={resource.id}
        />
      ),
    },
    {
      id: RESOURCE_SECTION_IDS.relations,
      label: 'Relations',
      icon: Workflow,
      content: resource && (
        <RelationsPanel
          teamId={resource.teamId}
          resourceType={resource.kind}
          resourceId={resource.id}
        />
      ),
    },
    ...extraSections,
  ]

  return <ReadingPage {...pageProps} sections={sections} />
}
