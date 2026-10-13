import { Activity, History, MessageSquare, Workflow } from 'lucide-react'
import type { ReactNode } from 'react'
import { useNavigate } from 'react-router'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import { CommentsPanel } from '@/components/comments/CommentsPanel'
import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingPageProps,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { RelationsPanel } from '@/components/relations/RelationsPanel'

import {
  ResourceHeaderMeta,
  type ResourceHeaderStatus,
} from './ResourceHeaderMeta'
import {
  RESOURCE_SECTION_CHROME,
  RESOURCE_SECTION_IDS,
} from './resourceSections'
import { UsageDisclosure } from './UsageDisclosure'

/** The resource kinds every standard side panel understands. */
export type ResourceKind = 'artifact' | 'prompt' | 'blueprint' | 'memory'

export interface ResourceRef {
  kind: ResourceKind
  id: string
  teamId: string
}

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
  /** The resource's type, where the kind has one — shown in the meta line. */
  kindLabel?: string
  /**
   * The resource's version history. With more than one version the header
   * shows a version chip linking to it, and the toolbar's "More" menu a
   * "Version history" item.
   */
  versionHistory?: VersionHistoryMeta
  /** ISO timestamp of the last edit — "Updated <relative>" in the header. */
  updatedAt?: string
  /** Lead paragraph beneath the header's badge row. */
  summary?: ReactNode
  /** Kind-specific header badges (the prompt's Shared badge). */
  headerExtra?: ReactNode
}

/**
 * `ReadingPage` plus the standard resource header (#902) and the section set
 * every team resource shares — Metadata, Relations, Comments, Attachments and
 * a collapsed Usage — in a fixed order, so each resource type (and any
 * resource type added later) gets the identical header and details panel by
 * describing itself rather than laying itself out.
 */
export function ResourceReadingPage({
  resource,
  metadata,
  attachments = true,
  extraSections = [],
  status,
  kindLabel,
  versionHistory,
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
  const version =
    versionHistory?.currentVersion === undefined
      ? undefined
      : {
          current: versionHistory.currentVersion,
          count: versionHistory.count,
          to: versionHistory.to,
        }
  const hasHeaderMeta = [
    status,
    kindLabel,
    version,
    updatedAt,
    summary,
    headerExtra,
  ].some(Boolean)
  const headerMeta = hasHeaderMeta ? (
    <ResourceHeaderMeta
      status={status}
      kindLabel={kindLabel}
      version={version}
      updatedAt={updatedAt}
      summary={summary}
      extra={headerExtra}
    />
  ) : undefined

  // "Version history" is a place a reader goes now and then, so it is a menu
  // item, appended to whatever the page asked for.
  const navigate = useNavigate()
  const actions: readonly ReadingAction[] = versionHistory
    ? [
        ...(pageProps.actions ?? []),
        {
          id: 'version-history',
          label: versionHistory.label ?? 'Version history',
          icon: History,
          testId: 'version-history-action',
          onClick: () => {
            void navigate(versionHistory.to)
          },
        },
      ]
    : (pageProps.actions ?? [])

  // Ordered by what a reader comes to the rail for: the resource's details,
  // then the parts that carry knowledge (relations, comments), then files, the
  // kind's own sections, and last — collapsed until asked for — usage. Nothing
  // is removed; page views just no longer sit above comments and relations.
  const sections: ReadingSection[] = [
    {
      id: RESOURCE_SECTION_IDS.metadata,
      ...RESOURCE_SECTION_CHROME.metadata,
      content: metadata,
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
      id: RESOURCE_SECTION_IDS.attachments,
      ...RESOURCE_SECTION_CHROME.attachments,
      content: resource && attachments && (
        <ResourceAttachments
          teamId={resource.teamId}
          ownerType={resource.kind}
          ownerId={resource.id}
        />
      ),
    },
    ...extraSections,
    {
      id: RESOURCE_SECTION_IDS.activity,
      label: 'Usage',
      icon: Activity,
      content: resource && (
        <UsageDisclosure>
          <AccessActivityPanel
            teamId={resource.teamId}
            resourceType={resource.kind}
            resourceId={resource.id}
          />
        </UsageDisclosure>
      ),
    },
  ]

  return (
    <ReadingPage
      {...pageProps}
      actions={actions}
      // Only a page that has its resource names it in the breadcrumb; a
      // loading or not-found page has nothing to name.
      breadcrumb={
        pageProps.breadcrumb ?? (resource ? pageProps.title : undefined)
      }
      description={description ?? headerMeta}
      sections={sections}
    />
  )
}
