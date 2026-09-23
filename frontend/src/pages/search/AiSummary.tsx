import { ChevronDown, RefreshCw, Settings, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { Link, useNavigate } from 'react-router'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Skeleton } from '@/components/ui/skeleton'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useLocalStorage } from '@/hooks/useLocalStorage'
import { cn } from '@/lib/utils'
import type { SummaryState } from '@/pages/search/aiSummary'
import { renderSummaryHtml, sourceUrl } from '@/pages/search/aiSummary'
import { TYPE_LABEL } from '@/pages/search/resourceUrl'
import type {
  SearchAISummaryAvailability,
  SearchSummaryResponse,
} from '@/services/searchService'

interface AiSummaryProps {
  /** `ai_summary` from the search response; absent → the section is hidden. */
  availability: SearchAISummaryAvailability | undefined
  /** Whether the user may configure model providers (`team.update`). */
  canConfigure: boolean
  teamId: string
  /** The summary for the current search, held by the page. */
  state: SummaryState | undefined
  /** Generate the summary for the current search (idempotent per search). */
  onGenerate: () => void
  /** Regenerate after an error. */
  onRetry: () => void
  /** Whether a result for this resource is on the visible page. */
  isOnPage: (resourceId: string) => boolean
  /** Scroll to and highlight the visible result for this resource. */
  onShowResult: (resourceId: string) => void
  /**
   * Open regardless of the stored preference — the user arrived through the
   * header search dialog's "See full results" (#1079). The stored preference
   * is left untouched.
   */
  preExpanded?: boolean
  /** The user toggled the section, ending any `preExpanded` override. */
  onToggle?: () => void
}

/**
 * Collapsed-by-default AI Summary section on the search page (#1078).
 *
 * Collapsed makes no network call; expanding asks the page to generate the
 * summary once per search. The summary state lives in the page, so collapsing
 * (which unmounts the panel's children) never loses it.
 */
export function AiSummary({
  availability,
  canConfigure,
  teamId,
  state,
  onGenerate,
  onRetry,
  isOnPage,
  onShowResult,
  preExpanded = false,
  onToggle,
}: Readonly<AiSummaryProps>) {
  const [storedOpen, setOpen] = useLocalStorage(
    STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED,
    false
  )
  const open = preExpanded || storedOpen
  const active = availability?.enabled === true && availability.available

  useEffect(() => {
    if (active && open && state === undefined) onGenerate()
  }, [active, open, state, onGenerate])

  if (!availability?.enabled) return null

  if (!availability.available) {
    if (!canConfigure) return null
    return (
      <Card className="text-muted-foreground flex items-center gap-2 p-4 text-sm">
        <Sparkles className="size-4 shrink-0" />
        <Link
          to={`/teams/${encodeURIComponent(teamId)}/settings/model-providers`}
          className="text-primary inline-flex items-center gap-1 font-medium hover:underline"
        >
          <Settings className="size-3.5" />
          Configure a model provider to enable AI Summary
        </Link>
      </Card>
    )
  }

  return (
    <Card className="p-0">
      <Collapsible
        open={open}
        onOpenChange={next => {
          onToggle?.()
          setOpen(next)
        }}
      >
        <CollapsibleTrigger className="flex w-full items-center gap-2 p-4 text-left font-medium">
          <Sparkles className="size-4 shrink-0" />
          <span className="flex-1">AI Summary</span>
          <ChevronDown
            className={cn(
              'size-4 shrink-0 transition-transform',
              open && 'rotate-180'
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent className="px-4 pb-4">
          <SummaryBody
            state={state}
            onRetry={onRetry}
            isOnPage={isOnPage}
            onShowResult={onShowResult}
          />
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}

function SummaryBody({
  state,
  onRetry,
  isOnPage,
  onShowResult,
}: Readonly<
  Pick<AiSummaryProps, 'state' | 'onRetry' | 'isOnPage' | 'onShowResult'>
>) {
  if (state === undefined || state.status === 'loading') {
    return <SummarySkeleton />
  }
  if (state.status === 'error') {
    return (
      <div role="alert" className="flex flex-col items-start gap-2 text-sm">
        <p className="text-destructive">{state.message}</p>
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw className="size-3.5" />
          Retry
        </Button>
      </div>
    )
  }
  return (
    <SummaryReady
      data={state.data}
      isOnPage={isOnPage}
      onShowResult={onShowResult}
    />
  )
}

function SummarySkeleton() {
  return (
    <div
      data-testid="ai-summary-skeleton"
      aria-busy="true"
      aria-label="Generating summary"
      className="space-y-2"
    >
      <Skeleton className="h-4 w-1/3" />
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-2/3" />
      <Skeleton className="mt-3 h-3 w-1/2" />
    </div>
  )
}

function SummaryReady({
  data,
  isOnPage,
  onShowResult,
}: Readonly<
  { data: SearchSummaryResponse } & Pick<
    AiSummaryProps,
    'isOnPage' | 'onShowResult'
  >
>) {
  const navigate = useNavigate()
  const html = useMemo(
    () => renderSummaryHtml(data.summary, data.sources),
    [data.summary, data.sources]
  )

  // Citation links carry the resource id: a cited result on the visible page
  // is scrolled to and highlighted; otherwise the link opens the resource
  // in-app rather than reloading the SPA. A native delegated listener, since
  // the links are injected HTML rather than React elements.
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const content = contentRef.current
    if (!content) return
    const handleClick = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return
      const link = event.target.closest<HTMLAnchorElement>('a[data-citation]')
      if (!link) return
      event.preventDefault()
      const resourceId = link.dataset.resourceId ?? ''
      if (isOnPage(resourceId)) {
        onShowResult(resourceId)
      } else {
        void navigate(link.getAttribute('href') ?? '')
      }
    }
    content.addEventListener('click', handleClick)
    return () => {
      content.removeEventListener('click', handleClick)
    }
  }, [isOnPage, onShowResult, navigate])

  return (
    <div className="space-y-3">
      <div
        className="prose prose-sm dark:prose-invert max-w-none"
        ref={contentRef}
        // Sanitized by renderSummaryHtml (DOMPurify) — the summary is
        // untrusted LLM output.
        dangerouslySetInnerHTML={{ __html: html }}
      />
      {data.sources.length > 0 && (
        <div className="border-t pt-3">
          <h3 className="text-muted-foreground mb-1 text-xs font-medium uppercase">
            Sources
          </h3>
          <ol className="space-y-1 text-sm">
            {data.sources.map(source => {
              const url = sourceUrl(source)
              return (
                <li key={source.index} className="flex items-baseline gap-2">
                  <span className="text-muted-foreground shrink-0">
                    [{source.index}]
                  </span>
                  <span className="min-w-0">
                    {url ? (
                      <Link to={url} className="hover:underline">
                        {source.title || TYPE_LABEL[source.type]}
                      </Link>
                    ) : (
                      source.title || TYPE_LABEL[source.type]
                    )}{' '}
                    <Badge variant="secondary" className="ml-1">
                      {TYPE_LABEL[source.type]}
                    </Badge>
                    {source.project_name && (
                      <span className="text-muted-foreground ml-1 text-xs">
                        {source.project_name}
                      </span>
                    )}
                    {source.truncated && (
                      <span className="text-muted-foreground ml-1 text-xs italic">
                        (truncated)
                      </span>
                    )}
                  </span>
                </li>
              )
            })}
          </ol>
        </div>
      )}
      <p className="text-muted-foreground text-xs">Generated by {data.model}</p>
    </div>
  )
}
