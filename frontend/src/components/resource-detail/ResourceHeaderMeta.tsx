import type { ReactNode } from 'react'

import { COPY_CHIP_CLASS, CopyChip } from '@/components/CopyChip'
import { RelativeTime } from '@/components/RelativeTime'
import { StatusBadge, type StatusTone } from '@/components/StatusBadge'
import { cn } from '@/lib/utils'

/** The resource's lifecycle state, rendered as the header's status badge. */
export interface ResourceHeaderStatus {
  /** Display text — already humanised by the caller (e.g. "Active"). */
  value: string
  tone?: StatusTone
}

/**
 * How the resource is addressed — the identifier a reader copies to use it
 * elsewhere (a prompt/artifact/blueprint slug). Kinds without one omit it.
 */
export interface ResourceHeaderAddress {
  /** Names the identifier in the copy affordance. Defaults to "Slug". */
  label?: string
  value: string
  /** `false` renders a plain chip with no copy button. Defaults to `true`. */
  copyable?: boolean
}

export interface ResourceHeaderMetaProps {
  status?: ResourceHeaderStatus
  address?: ResourceHeaderAddress
  /** ISO timestamp of the last edit, rendered as "Updated <relative>". */
  updatedAt?: string
  /** Lead paragraph under the badge row (the resource's description). */
  summary?: ReactNode
  /** Kind-specific badges slotted after the status (the prompt's Shared badge). */
  extra?: ReactNode
}

/**
 * The one reading-page header treatment every resource uses (#902): status
 * badge → kind-specific extras → address chip → "Updated <relative>", with the
 * summary on its own line beneath.
 *
 * Pages describe themselves with data (`ResourceReadingPage`'s `status` /
 * `address` / `updatedAt` / `summary` props) instead of hand-building a
 * `description` node, which is what keeps the five detail pages identical —
 * and `ReadingPage` itself domain-free.
 */
export function ResourceHeaderMeta({
  status,
  address,
  updatedAt,
  summary,
  extra,
}: Readonly<ResourceHeaderMetaProps>) {
  // `.some(Boolean)`, not `a ?? b ?? …`: `??` stops at the first non-nullish
  // value, so one present-but-falsy prop would suppress the whole row.
  const hasBadgeRow = [status, address, updatedAt, extra].some(Boolean)

  return (
    <div data-testid="resource-header-meta">
      {hasBadgeRow && (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          {status && (
            <StatusBadge tone={status.tone}>{status.value}</StatusBadge>
          )}
          {extra}
          {address && <HeaderAddress address={address} />}
          {updatedAt && (
            <span className="text-muted-foreground inline-flex items-center gap-1">
              Updated <RelativeTime value={updatedAt} />
            </span>
          )}
        </div>
      )}
      {summary && (
        <p className="text-muted-foreground mt-2 text-sm">{summary}</p>
      )}
    </div>
  )
}

/** The address chip — click-to-copy by default, a plain code chip when not. */
function HeaderAddress({
  address,
}: Readonly<{ address: ResourceHeaderAddress }>) {
  if (address.copyable === false) {
    return (
      <code className={cn(COPY_CHIP_CLASS, 'truncate')}>{address.value}</code>
    )
  }
  return <CopyChip label={address.label} value={address.value} />
}
