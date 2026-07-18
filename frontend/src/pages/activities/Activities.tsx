import { Activity as ActivityIcon, ChevronDown, Search } from 'lucide-react'
import { type ReactNode, useCallback, useEffect, useState } from 'react'

import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useAlerts } from '@/hooks'
import { formatDateTime, formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import type {
  Activity as ActivityType,
  ActivityFilters,
} from '@/services/activityService'
import { activityService } from '@/services/activityService'
import { getErrorMessage } from '@/utils/errorHandling'

interface State {
  activities: ActivityType[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Activities() {
  const { showError } = useAlerts()

  const [state, setState] = useState<State>({
    activities: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })
  const [filters, setFilters] = useState<ActivityFilters>({
    search: '',
    page: 1,
    limit: 25,
  })
  const [searchInput, setSearchInput] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  const fetchActivities = useCallback(
    async (current: ActivityFilters) => {
      try {
        setState(prev => ({ ...prev, loading: true, error: null }))
        const response = await activityService.getActivities(current)
        setState({
          activities: response.data.activities,
          loading: false,
          error: null,
          totalPages: response.data.total_pages,
          currentPage: current.page ?? 1,
          total: response.data.total_count,
        })
      } catch (error) {
        const message = getErrorMessage(error, 'Failed to fetch activities')
        setState(prev => ({ ...prev, loading: false, error: message }))
        showError(message, 'Error')
      }
    },
    [showError]
  )

  useEffect(() => {
    void fetchActivities(filters)
  }, [fetchActivities, filters])

  useEffect(() => {
    const t = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : { ...prev, search: searchInput, page: 1 }
      )
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput])

  // Loading / error / empty pre-empt the list; null means render the list.
  let listStateContent: ReactNode = null
  if (state.loading) {
    listStateContent = (
      <div className="space-y-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  } else if (state.error) {
    listStateContent = (
      <Alert variant="destructive">
        <AlertTitle>Failed to load activities</AlertTitle>
        <AlertDescription>{state.error}</AlertDescription>
      </Alert>
    )
  } else if (state.activities.length === 0) {
    listStateContent = (
      <EmptyState
        icon={ActivityIcon}
        title={
          filters.search
            ? 'No activities match your search'
            : 'No activities yet'
        }
        description={
          filters.search
            ? 'Try a different search term.'
            : 'Platform events and session activity will appear here.'
        }
      />
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Activities"
        description="Monitor account activity and platform events."
      />

      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="relative max-w-sm">
            <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
            <Input
              value={searchInput}
              onChange={e => {
                setSearchInput(e.target.value)
              }}
              placeholder="Search activities…"
              className="pl-8"
            />
          </div>

          {listStateContent ?? (
            <TooltipProvider>
              <div className="space-y-2">
                {state.activities.map(activity => (
                  <Collapsible
                    key={activity.id}
                    open={expanded[activity.id] ?? false}
                    onOpenChange={open => {
                      setExpanded(prev => ({ ...prev, [activity.id]: open }))
                    }}
                  >
                    <CollapsibleTrigger className="hover:bg-muted/40 flex w-full items-center gap-3 rounded-md border p-3 text-left transition-colors">
                      <ActivityIcon className="text-muted-foreground size-4 shrink-0" />
                      <div className="flex-1 min-w-0 space-y-0.5">
                        <p className="truncate text-sm font-medium">
                          {activity.description}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          {activity.activity_type}
                          {activity.entity_type && ` · ${activity.entity_type}`}
                        </p>
                      </div>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="text-muted-foreground shrink-0 text-xs">
                            {formatRelativeTime(activity.created_at)}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <p>{formatDateTime(activity.created_at)}</p>
                        </TooltipContent>
                      </Tooltip>
                      <ChevronDown
                        className={cn(
                          'text-muted-foreground size-4 shrink-0 transition-transform',
                          expanded[activity.id] && 'rotate-180'
                        )}
                      />
                    </CollapsibleTrigger>
                    <CollapsibleContent>
                      <div className="bg-muted/30 mt-1 rounded-md border p-3 text-xs">
                        <pre className="overflow-x-auto font-mono">
                          {JSON.stringify(activity, null, 2)}
                        </pre>
                      </div>
                    </CollapsibleContent>
                  </Collapsible>
                ))}
              </div>
            </TooltipProvider>
          )}

          {state.totalPages > 1 && (
            <div className="flex items-center justify-between gap-2">
              <div className="text-muted-foreground text-sm">
                Page {state.currentPage} of {state.totalPages}
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={state.currentPage <= 1}
                  onClick={() => {
                    setFilters(prev => ({
                      ...prev,
                      page: state.currentPage - 1,
                    }))
                  }}
                >
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={state.currentPage >= state.totalPages}
                  onClick={() => {
                    setFilters(prev => ({
                      ...prev,
                      page: state.currentPage + 1,
                    }))
                  }}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
