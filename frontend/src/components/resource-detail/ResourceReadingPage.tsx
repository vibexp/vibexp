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

import {
  type ResourceHeaderAddress,
  ResourceHeaderMeta,
  type ResourceHeaderStatus,
} from './ResourceHeaderMeta'

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
  /** Lifecycle state, rendered as the header's status badge. */
  status?: ResourceHeaderStatus
  /** The identifier a reader copies — the slug, for kinds that have one. */
  address?: ResourceHeaderAddress
  /** ISO timestamp of the last edit — "Updated <relative>" in the header. */
  updatedAt?: string
  /** Lead paragraph beneath the header's badge row. */
  summary?: ReactNode
  /** Kind-specific header badges (the prompt's Shared badge). */
  headerExtra?: ReactNode
}

/**
 * `ReadingPage` plus the standard resource header (#902) and the section set
 * every team resource shares — Metadata, Attachments, Access activity,
 * Comments, Relations — in a fixed order, so each resource type (and any
 * resource type added later) gets the identical header and details panel by
 * describing itself rather than laying itself out.
 */
export function ResourceReadingPage({
  resource,
  metadata,
  attachments = true,
  extraSections = [],
  status,
  address,
  updatedAt,
  summary,
  headerExtra,
  description,
  ...pageProps
}: Readonly<ResourceReadingPageProps>) {
  // The standard header, built from data. An explicit `description` still wins,
  // so the inherited `ReadingPageProps` escape hatch keeps working for a page
  // that genuinely needs a bespoke node. `.some(Boolean)` rather than a chain
  // of `??`, which would stop at the first present-but-falsy prop.
  const hasHeaderMeta = [status, address, updatedAt, summary, headerExtra].some(
    Boolean
  )
  const headerMeta = hasHeaderMeta ? (
    <ResourceHeaderMeta
      status={status}
      address={address}
      updatedAt={updatedAt}
      summary={summary}
      extra={headerExtra}
    />
  ) : undefined
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

  return (
    <ReadingPage
      {...pageProps}
      description={description ?? headerMeta}
      sections={sections}
    />
  )
}
