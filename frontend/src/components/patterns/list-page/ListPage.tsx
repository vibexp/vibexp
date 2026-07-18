import type { ReactNode } from 'react'

import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import type { ListPageCount, ListPagePagination, ListPageStatus } from './types'

/**
 * Compound `<ListPage>` component implementing the gold-standard list-page
 * layout used across Prompts, Artifacts, Blueprints, Memories, Agents, and
 * Settings → Projects.
 *
 * Usage:
 *
 * ```tsx
 * <ListPage>
 *   <ListPage.Header title="Prompts" actions={<Button>New</Button>} />
 *   <ListPage.Container>
 *     <ListPage.Filters>
 *       <YourFilters … />
 *     </ListPage.Filters>
 *     <ListPage.Body status={status} empty={<EmptyState … />}>
 *       <ListTable … />
 *     </ListPage.Body>
 *     <ListPage.Footer
 *       count={{ visible, total, noun: 'prompt' }}
 *       pagination={{ page, totalPages, onPageChange }}
 *     />
 *   </ListPage.Container>
 * </ListPage>
 * ```
 *
 * Designed to live in a future design-system package: no domain imports, no
 * data fetching, only semantic Tailwind tokens.
 */

interface ListPageRootProps {
  children: ReactNode
  className?: string
}

function Root({ children, className }: Readonly<ListPageRootProps>) {
  return <div className={cn('space-y-6', className)}>{children}</div>
}

interface ListPageHeaderProps {
  title: string
  description?: ReactNode
  actions?: ReactNode
}

function Header({
  title,
  description,
  actions,
}: Readonly<ListPageHeaderProps>) {
  return (
    <PageHeader title={title} description={description} actions={actions} />
  )
}

interface ListPageContainerProps {
  children: ReactNode
  className?: string
}

/**
 * The Card that wraps Filters / Body / Footer. Borders between zones are
 * applied by `<ListPage.Filters>` and `<ListPage.Footer>` themselves.
 */
function Container({ children, className }: Readonly<ListPageContainerProps>) {
  return <Card className={cn('overflow-hidden', className)}>{children}</Card>
}

interface ListPageFiltersProps {
  children: ReactNode
}

function Filters({ children }: Readonly<ListPageFiltersProps>) {
  return <div className="border-b px-4 py-3">{children}</div>
}

interface ListPageBodyProps {
  status: ListPageStatus
  errorTitle?: string
  errorMessage?: string | null
  loadingRows?: number
  empty?: ReactNode
  children: ReactNode
}

/**
 * Renders the appropriate state for the body region:
 * - `loading` → skeleton rows
 * - `error` → destructive alert
 * - `empty` → caller-provided empty-state element
 * - `ready` → caller-provided table/list
 */
function Body({
  status,
  errorTitle = 'Failed to load',
  errorMessage,
  loadingRows = 6,
  empty,
  children,
}: Readonly<ListPageBodyProps>) {
  if (status === 'loading') {
    return (
      <div className="space-y-3 p-4">
        {Array.from({ length: loadingRows }, (_, i) => `row-${String(i)}`).map(
          slot => (
            <Skeleton
              key={slot}
              data-testid="list-page-skeleton-row"
              className="h-10 w-full"
            />
          )
        )}
      </div>
    )
  }
  if (status === 'error') {
    return (
      <div className="p-4">
        <Alert variant="destructive">
          <AlertTitle>{errorTitle}</AlertTitle>
          {errorMessage && <AlertDescription>{errorMessage}</AlertDescription>}
        </Alert>
      </div>
    )
  }
  if (status === 'empty') {
    return <div className="p-4">{empty}</div>
  }
  return <>{children}</>
}

interface ListPageFooterProps {
  count?: ListPageCount
  pagination?: ListPagePagination
  /** Optional small note below the count line (e.g. sort scope disclaimer). */
  note?: ReactNode
  /** Suppress the "Showing X of Y" line (e.g. during loading). */
  hideCount?: boolean
}

function countNoun(count: ListPageCount): string {
  if (count.total === 1) return count.noun
  return count.nounPlural ?? `${count.noun}s`
}

function Footer({
  count,
  pagination,
  note,
  hideCount,
}: Readonly<ListPageFooterProps>) {
  const noun = count ? countNoun(count) : ''
  const showCount = count && !hideCount
  // Pagination controls only render when there's more than one page. Single-page
  // lists shouldn't carry always-disabled Prev/Next buttons.
  const showPagination = pagination !== undefined && pagination.totalPages > 1

  return (
    <div className="text-muted-foreground flex items-center justify-between gap-3 border-t px-4 py-3 text-xs">
      <div className="flex flex-col gap-0.5">
        {showCount ? (
          <span>
            Showing {count.visible} of {count.total} {noun}
          </span>
        ) : (
          // Empty span keeps the flex layout stable while count is hidden.
          <span />
        )}
        {note && <span className="text-xs italic">{note}</span>}
      </div>
      {showPagination && (
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={pagination.page <= 1}
            onClick={() => {
              pagination.onPageChange(Math.max(1, pagination.page - 1))
            }}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={pagination.page >= pagination.totalPages}
            onClick={() => {
              pagination.onPageChange(pagination.page + 1)
            }}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  )
}

export const ListPage = Object.assign(Root, {
  Header,
  Container,
  Filters,
  Body,
  Footer,
})
